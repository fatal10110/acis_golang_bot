package botclient

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// behaviorRow is one row of "Behavior per class" (docs/strategy.md) as the
// build plays it: a class from its row's start level to its end, the kit
// compiled for it and the profession it takes next.
type behaviorRow struct {
	name              string
	class, start, end int32
	kit               ClassKit
	next              int32
}

// firstMinLevel is the first level of class's merged skill tree that grants
// skill; ok is false when the tree holds none.
func firstMinLevel(c datapack.Class, skill int32) (int32, bool) {
	lvl, ok := int32(0), false
	for _, g := range c.Skills {
		if g.ID == skill && (!ok || int32(g.MinLevel) < lvl) {
			lvl, ok = int32(g.MinLevel), true
		}
	}
	return lvl, ok
}

// checkRow checks a row against the class trees and skill templates: every
// skill is in the class's merged tree by the row's end, every step and
// toggle starts at max(row start, the skill's first merged minLvl) (its
// "@N"), as does every buff with a From and a Relax with a RelaxFrom, every
// bracket gate equals the skill's weaponsAllowed and every branch gate
// (Step.Branch) fits it at each level the row reaches, a skill with
// weaponsAllowed has a gate, every
// charge gate equals the skill's charge condition, every Within gate equals
// the skill's castRange, every toggle is a TOGGLE skill, a Party buff and a
// party heal (Heal.Members) are exactly the PARTY-target skills among the
// buffs and heals, every SELF-target buff is a Self one, every skill tagged (base) (ClassKit.Base) is first
// granted under 20, every skill tagged (fixed) (ClassKit.Fixed) is in the
// merged tree and not in the class's own, and the next profession is a
// direct child of the class.
func checkRow(r behaviorRow, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) []string {
	var errs []string
	fail := func(format string, args ...any) { errs = append(errs, r.name+": "+fmt.Sprintf(format, args...)) }
	c, ok := classes[r.class]
	if !ok {
		return []string{fmt.Sprintf("%s: class %d not in the datapack", r.name, r.class)}
	}
	known := func(skill int32) (int32, bool) {
		lvl, ok := firstMinLevel(c, skill)
		if !ok || lvl > r.end {
			fail("skill %d not in class %d's tree by level %d", skill, r.class, r.end)
			return 0, false
		}
		return lvl, true
	}
	starts := func(skill, from int32) bool {
		lvl, ok := known(skill)
		if ok {
			if want, got := max(r.start, lvl), max(r.start, from); got != want {
				fail("skill %d starts at %d, want @%d", skill, got, want)
			}
		}
		return ok
	}
	// templates are skill's templates at each level the row reaches.
	templates := func(skill int32) []datapack.Skill {
		var ts []datapack.Skill
		for _, g := range c.Skills {
			if g.ID == skill && int32(g.MinLevel) <= r.end {
				ts = append(ts, skills[datapack.SkillRef{ID: g.ID, Level: int32(g.Level)}])
			}
		}
		return ts
	}
	party := func(kind string, skill int32, want bool) {
		for _, sk := range templates(skill) {
			if (sk.Target == "PARTY") != want {
				fail("%s %d targets %s, the row casts it as a party one %v", kind, skill, sk.Target, want)
				return
			}
		}
	}
	for _, tg := range r.kit.Toggles {
		starts(tg.Skill, tg.From)
		for _, sk := range templates(tg.Skill) {
			if sk.OperateType != "TOGGLE" {
				fail("toggle %d is operateType %s", tg.Skill, sk.OperateType)
				break
			}
			// A toggle's weapon gate fits its weaponsAllowed, as a branch
			// gate: Polearm Accuracy's comes from <using kind>, not from
			// weaponsAllowed.
			if len(sk.WeaponsAllowed) > 0 && len(tg.Weapons) == 0 {
				fail("toggle %d allows %v, it has no weapon gate", tg.Skill, sk.WeaponsAllowed)
			}
			for _, k := range tg.Weapons {
				if len(sk.WeaponsAllowed) > 0 && !slices.Contains(sk.WeaponsAllowed, k) {
					fail("toggle %d allows %v, its gate has %s", tg.Skill, sk.WeaponsAllowed, k)
				}
			}
		}
	}
	for _, st := range slices.Concat(r.kit.Rotation, r.kit.PvP) {
		if !starts(st.Skill, st.From) {
			continue
		}
		for _, g := range c.Skills {
			if g.ID != st.Skill || int32(g.MinLevel) > r.end {
				continue
			}
			sk := skills[datapack.SkillRef{ID: g.ID, Level: int32(g.Level)}]
			if len(sk.WeaponsAllowed) > 0 && len(st.Weapons) == 0 {
				fail("skill %d level %d allows %v, the step has no weapon gate", g.ID, g.Level, sk.WeaponsAllowed)
			}
			for _, k := range st.Weapons {
				if len(sk.WeaponsAllowed) > 0 && !slices.Contains(sk.WeaponsAllowed, k) {
					fail("skill %d level %d allows %v, the step's gate has %s", g.ID, g.Level, sk.WeaponsAllowed, k)
				}
			}
			if len(st.Weapons) > 0 && !st.Branch && !slices.Equal(slices.Sorted(slices.Values(st.Weapons)), slices.Sorted(slices.Values(sk.WeaponsAllowed))) {
				fail("skill %d level %d allows %v, the step's bracket gate is %v", g.ID, g.Level, sk.WeaponsAllowed, st.Weapons)
			}
			wantCharges, wantToMax := 0, sk.Charges.Not
			if !sk.Charges.Not {
				wantCharges = sk.Charges.Min
			}
			if st.Charges != wantCharges || st.ChargeToMax != wantToMax {
				fail("skill %d level %d charge condition %+v, the step has charges %d to max %v", g.ID, g.Level, sk.Charges, st.Charges, st.ChargeToMax)
			}
			// A Within is the castRange; an aura's (castRange 0) is its
			// radius, the OS Stun Attack's its padded reach (X8).
			aura := sk.CastRange <= 0 && st.Within == sk.Radius
			if st.Within != 0 && st.Within != sk.CastRange && !aura && (st.Skill != skillStunAttack || st.Within != stunAttackReach) {
				fail("skill %d level %d castRange %d, the step's Within is %d", g.ID, g.Level, sk.CastRange, st.Within)
			}
		}
	}
	for _, id := range r.kit.Skills {
		known(id)
	}
	for _, h := range r.kit.Heals {
		starts(h.Skill, h.From)
		party("heal", h.Skill, h.Members > 0)
	}
	for _, bf := range r.kit.Buffs {
		if bf.From != 0 {
			starts(bf.Skill, bf.From)
		} else {
			known(bf.Skill)
		}
		party("buff", bf.Skill, bf.Party)
		for _, sk := range templates(bf.Skill) {
			if sk.Target == "SELF" && !bf.Self {
				fail("buff %d targets SELF, the row does not cast it as a Self one", bf.Skill)
				break
			}
		}
	}
	for _, id := range r.kit.Base {
		if lvl, ok := known(id); ok && lvl >= 20 {
			fail("skill %d tagged (base) is first granted at %d, not by the base class", id, lvl)
		}
	}
	// The class's own tree is the head of its merged one: its own grants
	// in file order, then its parent's merged tree (datapack.Class.Skills).
	own := c.Skills
	if p, ok := classes[c.Parent]; ok {
		own = c.Skills[:max(0, len(c.Skills)-len(p.Skills))]
	}
	for _, id := range r.kit.Fixed {
		if _, ok := known(id); ok && slices.ContainsFunc(own, func(g datapack.SkillGrant) bool { return g.ID == id }) {
			fail("skill %d tagged (fixed) is in class %d's own tree", id, r.class)
		}
	}
	for _, id := range []int32{r.kit.Flee, r.kit.Sweep} {
		if id != 0 {
			known(id)
		}
	}
	switch {
	case r.kit.RelaxFrom != 0:
		starts(r.kit.Relax, r.kit.RelaxFrom)
	case r.kit.Relax != 0:
		known(r.kit.Relax)
	}
	if r.next != 0 && classes[r.next].Parent != r.class {
		fail("profession %d is not a direct child of class %d", r.next, r.class)
	}
	return errs
}

// rowSpan is the levels a row of class plays (docs/strategy.md "Behavior
// per class"): 1-19 for a base class, 20-39 for a first profession and
// 40-75 for a second; ok is false for a third profession (76+, no rows)
// and an id the profession table does not hold.
func rowSpan(class int32) (start, end int32, ok bool) {
	depth := 0
	p, known := datapack.ParentClass(class)
	for known && p >= 0 {
		depth++
		p, known = datapack.ParentClass(p)
	}
	if !known {
		return 0, 0, false
	}
	switch depth {
	case 0:
		return 1, 19, true
	case 1:
		return 20, 39, true
	case 2:
		return 40, 75, true
	}
	return 0, 0, false
}

// secondRows are the second professions with a row of their own: an entry
// in professionKits. The others play their first profession's row
// (TestSecondProfessionFallback).
func secondRows() []int32 {
	var ids []int32
	for id := range professionKits {
		if start, _, ok := rowSpan(id); ok && start == 40 {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// shippedRows are the rows of every base class (levels 1-19), first
// profession (20-39) and second profession with a row of its own (40-75,
// secondRows) in classes, for every role.
func shippedRows(classes map[int32]datapack.Class) []behaviorRow {
	ids := make([]int32, 0, len(classes))
	for id := range classes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var rows []behaviorRow
	for _, id := range ids {
		c := classes[id]
		start, end, ok := rowSpan(id)
		if !ok || start == 40 && !slices.Contains(secondRows(), id) {
			continue
		}
		for _, role := range []Role{RoleDamage, RoleSupport, RoleCrafter} {
			bd := CompileBuild(c.Race, id, end, role, BranchChoice{})
			rows = append(rows, behaviorRow{
				name: fmt.Sprintf("class %d %v", id, role), class: id, start: start, end: end,
				kit: bd.Kit, next: bd.Race.Professions[id],
			})
		}
	}
	return rows
}

// TestBehaviorRowsShipped checks the build's rows, their sets and dyes
// (gearRows) included, and that every class that learns a skill has a
// trainer (indexTrainers), against the shipped datapack when
// BOT_DATAPACK_XML names its data/xml directory, and skips otherwise
// (docs/strategy.md, "Testing").
func TestBehaviorRowsShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	classes, err := datapack.LoadClasses(filepath.Join(dir, "classes"))
	if err != nil {
		t.Fatal(err)
	}
	skills, err := datapack.LoadSkills(filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	npcs, err := datapack.LoadNPCs(filepath.Join(dir, "npcs"))
	if err != nil {
		t.Fatal(err)
	}
	trainers := indexTrainers(npcs)
	rows := shippedRows(classes)
	if second := secondRows(); len(rows) != 3*(9+18+len(second)) {
		t.Errorf("%d rows, want every role of the 9 base classes, 18 first professions and second professions %v", len(rows), second)
	}
	for _, r := range rows {
		for _, e := range checkRow(r, classes, skills) {
			t.Error(e)
		}
		if r.kit.SpotMargin != rowSpotMargins[r.class] {
			t.Errorf("%s: spot margin %v, want %v (column 10)", r.name, r.kit.SpotMargin, rowSpotMargins[r.class])
		}
		if len(r.kit.Skills) > 0 && len(trainers[r.class]) == 0 {
			t.Errorf("%s learns %v at no trainer", r.name, r.kit.Skills)
		}
		// A second profession takes no profession next: the third is out
		// of scope (branch_test.go).
		if r.next == 0 && r.start < 40 {
			t.Errorf("%s takes no profession next (branchTables)", r.name)
		}
	}
	// Every line of the branch tables goes from its base class to a
	// direct child and on to that one's direct child (#328).
	for role, tab := range branchTables {
		for base, ls := range tab {
			for _, l := range ls {
				first, ok1 := classes[l.first]
				second, ok2 := classes[l.second]
				if !ok1 || !ok2 || first.Parent != base || second.Parent != l.first || second.Race != classes[base].Race {
					t.Errorf("%v class %d: line %d -> %d is not in its tree", role, base, l.first, l.second)
				}
			}
		}
	}
	tables, err := datapack.LoadItems(filepath.Join(dir, "items"))
	if err != nil {
		t.Fatal(err)
	}
	sets, err := datapack.LoadArmorSets(filepath.Join(dir, "armorSets.xml"))
	if err != nil {
		t.Fatal(err)
	}
	hennas, err := datapack.LoadHennas(filepath.Join(dir, "hennas.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for class, r := range gearRows {
		for _, e := range checkGearRow(class, r, classes, tables.Items, sets, hennas) {
			t.Error(e)
		}
	}
}

// TestBehaviorRowsCheck checks checkRow on a fixture tree: a right row
// passes, as do a branch gate inside weaponsAllowed, a Within gate at
// castRange and PARTY skills cast as party ones; a wrong weapon gate, a
// missing one, a bracket gate narrower than weaponsAllowed, a Within gate
// off castRange, a wrong level of a step, a toggle or a buff, a toggle of a
// skill that is none, a party buff or heal of a single-target skill and the
// other way round, a skill out of the tree (a heal's, Relax), a heal off
// its level or with none, a wrong
// charge gate, a (base) tag on a skill first granted from 20 and a
// profession that is not a direct child each fail. In a second
// profession's row (40/75) a (fixed) tag on a skill of the parent's tree
// passes and one on a skill of the class's own tree or out of the tree
// fails; an untagged skill granted at 38 passes, one granted at 43 only
// with From 43.
func TestBehaviorRowsCheck(t *testing.T) {
	const sword, plain, charged, late, aura, chant = int32(100), int32(101), int32(102), int32(103), int32(104), int32(105)
	const first38, own40, own43, ward = int32(106), int32(107), int32(108), int32(109)
	classes := map[int32]datapack.Class{
		10: {ID: 10, Parent: -1, Skills: []datapack.SkillGrant{
			{ID: sword, Level: 1, MinLevel: 1},
			{ID: plain, Level: 1, MinLevel: 7},
			{ID: plain, Level: 2, MinLevel: 14},
			{ID: charged, Level: 1, MinLevel: 5},
			{ID: late, Level: 1, MinLevel: 20},
			{ID: aura, Level: 1, MinLevel: 7},
			{ID: chant, Level: 1, MinLevel: 1},
			{ID: ward, Level: 1, MinLevel: 1},
		}},
		11: {ID: 11, Parent: 10},
		12: {ID: 12, Parent: 11},
	}
	// 20 is a first profession of 10 and 21 a second, each tree its own
	// grants and then its parent's merged one.
	classes[20] = datapack.Class{ID: 20, Parent: 10, Skills: slices.Concat([]datapack.SkillGrant{
		{ID: first38, Level: 1, MinLevel: 38},
	}, classes[10].Skills)}
	classes[21] = datapack.Class{ID: 21, Parent: 20, Skills: slices.Concat([]datapack.SkillGrant{
		{ID: own40, Level: 1, MinLevel: 40},
		{ID: own43, Level: 1, MinLevel: 43},
	}, classes[20].Skills)}
	second := func(kit ClassKit) behaviorRow {
		return behaviorRow{name: "fixture", class: 21, start: 40, end: 75, kit: kit}
	}
	skills := map[datapack.SkillRef]datapack.Skill{
		{ID: sword, Level: 1}:   {WeaponsAllowed: []string{"SWORD", "BLUNT"}},
		{ID: plain, Level: 1}:   {CastRange: 600},
		{ID: plain, Level: 2}:   {CastRange: 600},
		{ID: charged, Level: 1}: {Charges: datapack.ChargeCond{Min: 2}},
		{ID: late, Level: 1}:    {},
		{ID: aura, Level: 1}:    {OperateType: "TOGGLE"},
		{ID: chant, Level: 1}:   {Target: "PARTY"},
		{ID: ward, Level: 1}:    {Target: "SELF"},
	}
	row := func(next int32, steps ...Step) behaviorRow {
		return behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Rotation: steps, Heals: []Heal{{Skill: plain, From: 7, HP: 50}}}, next: next}
	}
	tests := []struct {
		name string
		row  behaviorRow
		fail bool
	}{
		{"right", row(11, Step{Skill: sword, Weapons: []string{"BLUNT", "SWORD"}}, Step{Skill: plain, From: 7}, Step{Skill: charged, From: 5, Charges: 2}), false},
		{"branch gate inside", row(11, Step{Skill: sword, Weapons: []string{"SWORD"}, Branch: true}), false},
		{"bracket gate narrower", row(11, Step{Skill: sword, Weapons: []string{"SWORD"}}), true},
		{"wrong weapon gate", row(11, Step{Skill: sword, Weapons: []string{"BOW"}}), true},
		{"toggle level", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Toggles: []Toggle{{Skill: aura, From: 7}}}}, false},
		{"wrong toggle level", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Toggles: []Toggle{{Skill: aura}}}}, true},
		{"not a toggle", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Toggles: []Toggle{{Skill: plain, From: 7}}}}, true},
		{"within gate", row(11, Step{Skill: plain, From: 7, Within: 600}), false},
		{"wrong within gate", row(11, Step{Skill: plain, From: 7, Within: 400}), true},
		{"party buff and heal", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Buffs: []Buff{{Skill: chant, Party: true}}, Heals: []Heal{{Skill: chant, HP: 50, Members: 1}}}}, false},
		{"party buff of a single target", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Buffs: []Buff{{Skill: plain, From: 7, Party: true}}}}, true},
		{"member buff of a party target", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Buffs: []Buff{{Skill: chant}}}}, true},
		{"self buff of a self target", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Buffs: []Buff{{Skill: ward, Self: true}}}}, false},
		{"member buff of a self target", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Buffs: []Buff{{Skill: ward}}}}, true},
		{"party heal of a single target", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Heals: []Heal{{Skill: plain, HP: 60, Members: 2}}}}, true},
		{"single heal of a party target", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Heals: []Heal{{Skill: chant, HP: 60}}}}, true},
		{"relax not in the tree", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Relax: 999}}, true},
		{"relax level", second(ClassKit{Relax: own43, RelaxFrom: 43}), false},
		{"wrong relax level", second(ClassKit{Relax: own43, RelaxFrom: 44}), true},
		{"no weapon gate", row(11, Step{Skill: sword}), true},
		{"no level", row(11, Step{Skill: plain}), true},
		{"wrong level", row(11, Step{Skill: plain, From: 9}), true},
		{"not in the tree", row(11, Step{Skill: 999}), true},
		{"heal not in the tree", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Heals: []Heal{{Skill: 999}}}}, true},
		{"wrong charge gate", row(11, Step{Skill: charged, From: 5, Charges: 1}), true},
		{"not a direct child", row(12), true},
		{"buff level", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Buffs: []Buff{{Skill: plain, From: 7}}}}, false},
		{"wrong buff level", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Buffs: []Buff{{Skill: plain, From: 9}}}}, true},
		{"wrong heal level", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Heals: []Heal{{Skill: plain, From: 9, HP: 60}}}}, true},
		{"no heal level", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Heals: []Heal{{Skill: plain, HP: 60}}}}, true},
		{"second row: heal at its level", second(ClassKit{Heals: []Heal{{Skill: own43, From: 43, HP: 60}}}), false},
		{"base skill", behaviorRow{name: "fixture", class: 10, start: 20, end: 39, kit: ClassKit{Relax: plain, Base: []int32{plain}}}, false},
		{"base tag from 20", behaviorRow{name: "fixture", class: 10, start: 20, end: 39, kit: ClassKit{Buffs: []Buff{{Skill: late}}, Base: []int32{late}}}, true},
		{"profession row starts at 20", behaviorRow{name: "fixture", class: 10, start: 20, end: 39, kit: ClassKit{Rotation: []Step{{Skill: plain}}}}, false},
		{"fixed from the parent", second(ClassKit{Rotation: []Step{{Skill: first38}}, Fixed: []int32{first38}}), false},
		{"fixed from the base", second(ClassKit{Rotation: []Step{{Skill: plain}}, Fixed: []int32{plain}}), false},
		{"fixed in the own tree", second(ClassKit{Rotation: []Step{{Skill: own40}}, Fixed: []int32{own40}}), true},
		{"fixed out of the tree", second(ClassKit{Fixed: []int32{999}}), true},
		{"second row: untagged at 38", second(ClassKit{Rotation: []Step{{Skill: first38}}}), false},
		{"second row: untagged at 40", second(ClassKit{Rotation: []Step{{Skill: own40}}}), false},
		{"second row: 43 at its level", second(ClassKit{Rotation: []Step{{Skill: own43, From: 43}}}), false},
		{"second row: 43 untagged", second(ClassKit{Rotation: []Step{{Skill: own43}}}), true},
	}
	for _, tt := range tests {
		errs := checkRow(tt.row, classes, skills)
		if (len(errs) > 0) != tt.fail {
			t.Errorf("%s: errors %v, want failing %v", tt.name, errs, tt.fail)
		}
	}
}
