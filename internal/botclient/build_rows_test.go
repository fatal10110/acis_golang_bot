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
// skill is in the class's merged tree by the row's end, every step starts at
// max(row start, the skill's first merged minLvl) (its "@N"), every weapon
// gate fits the skill's weaponsAllowed at each level the row reaches and a
// skill with weaponsAllowed has one, every charge gate equals the skill's
// charge condition, and the next profession is a direct child of the
// class.
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
	for _, st := range r.kit.Rotation {
		lvl, ok := known(st.Skill)
		if !ok {
			continue
		}
		if want, got := max(r.start, lvl), max(r.start, st.From); got != want {
			fail("skill %d starts at %d, want @%d", st.Skill, got, want)
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
			wantCharges, wantToMax := 0, sk.Charges.Not
			if !sk.Charges.Not {
				wantCharges = sk.Charges.Min
			}
			if st.Charges != wantCharges || st.ChargeToMax != wantToMax {
				fail("skill %d level %d charge condition %+v, the step has charges %d to max %v", g.ID, g.Level, sk.Charges, st.Charges, st.ChargeToMax)
			}
		}
	}
	for _, id := range r.kit.Skills {
		known(id)
	}
	for _, h := range r.kit.Heals {
		known(h.Skill)
	}
	for _, bf := range r.kit.Buffs {
		known(bf.Skill)
	}
	if r.kit.Flee != 0 {
		known(r.kit.Flee)
	}
	if r.next != 0 && classes[r.next].Parent != r.class {
		fail("profession %d is not a direct child of class %d", r.next, r.class)
	}
	return errs
}

// shippedRows are the rows of every base class (levels 1-19) and first
// profession (20-39) in classes, for every role. A line without a kit of its
// own plays supportKit, which is no row of its: only its professions are
// checked until its rows land (S6 #325).
func shippedRows(classes map[int32]datapack.Class) []behaviorRow {
	ids := make([]int32, 0, len(classes))
	for id := range classes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var rows []behaviorRow
	for _, id := range ids {
		c := classes[id]
		start, end := int32(1), int32(19)
		if c.Parent >= 0 {
			if classes[c.Parent].Parent >= 0 {
				continue
			}
			start, end = 20, 39
		}
		base, _ := datapack.BaseClass(id)
		for _, role := range []Role{RoleDamage, RoleSupport, RoleCrafter} {
			bd := CompileBuild(c.Race, id, end, role)
			if _, ok := lineKits[base]; !ok {
				bd.Kit.Rotation, bd.Kit.Heals, bd.Kit.Buffs = nil, nil, nil
			}
			rows = append(rows, behaviorRow{
				name: fmt.Sprintf("class %d %v", id, role), class: id, start: start, end: end,
				kit: bd.Kit, next: bd.Race.Professions[id],
			})
		}
	}
	return rows
}

// TestBehaviorRowsShipped checks the build's rows, their sets and dyes
// (gearRows) included, against the shipped datapack when BOT_DATAPACK_XML
// names its data/xml directory, and skips otherwise (docs/strategy.md,
// "Testing").
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
	rows := shippedRows(classes)
	if len(rows) != 3*(9+18) {
		t.Errorf("%d rows, want every role of the 9 base classes and 18 first professions", len(rows))
	}
	for _, r := range rows {
		for _, e := range checkRow(r, classes, skills) {
			t.Error(e)
		}
	}
	items, _, _, err := datapack.LoadItems(filepath.Join(dir, "items"))
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
		for _, e := range checkGearRow(class, r, classes, items, sets, hennas) {
			t.Error(e)
		}
	}
}

// TestBehaviorRowsCheck checks checkRow on a fixture tree: a right row
// passes; a wrong weapon gate, a missing one, a wrong level, a skill out of
// the tree, a wrong charge gate and a profession that is not a direct child
// each fail.
func TestBehaviorRowsCheck(t *testing.T) {
	const sword, plain, charged = int32(100), int32(101), int32(102)
	classes := map[int32]datapack.Class{
		10: {ID: 10, Parent: -1, Skills: []datapack.SkillGrant{
			{ID: sword, Level: 1, MinLevel: 1},
			{ID: plain, Level: 1, MinLevel: 7},
			{ID: plain, Level: 2, MinLevel: 14},
			{ID: charged, Level: 1, MinLevel: 5},
		}},
		11: {ID: 11, Parent: 10},
		12: {ID: 12, Parent: 11},
	}
	skills := map[datapack.SkillRef]datapack.Skill{
		{ID: sword, Level: 1}:   {WeaponsAllowed: []string{"SWORD", "BLUNT"}},
		{ID: plain, Level: 1}:   {},
		{ID: plain, Level: 2}:   {},
		{ID: charged, Level: 1}: {Charges: datapack.ChargeCond{Min: 2}},
	}
	row := func(next int32, steps ...Step) behaviorRow {
		return behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Rotation: steps, Heals: []Heal{{Skill: plain, HP: 50}}}, next: next}
	}
	tests := []struct {
		name string
		row  behaviorRow
		fail bool
	}{
		{"right", row(11, Step{Skill: sword, Weapons: []string{"SWORD"}}, Step{Skill: plain, From: 7}, Step{Skill: charged, From: 5, Charges: 2}), false},
		{"wrong weapon gate", row(11, Step{Skill: sword, Weapons: []string{"BOW"}}), true},
		{"no weapon gate", row(11, Step{Skill: sword}), true},
		{"no level", row(11, Step{Skill: plain}), true},
		{"wrong level", row(11, Step{Skill: plain, From: 9}), true},
		{"not in the tree", row(11, Step{Skill: 999}), true},
		{"heal not in the tree", behaviorRow{name: "fixture", class: 10, start: 1, end: 19, kit: ClassKit{Heals: []Heal{{Skill: 999}}}}, true},
		{"wrong charge gate", row(11, Step{Skill: charged, From: 5, Charges: 1}), true},
		{"not a direct child", row(12), true},
		{"profession row starts at 20", behaviorRow{name: "fixture", class: 10, start: 20, end: 39, kit: ClassKit{Rotation: []Step{{Skill: plain}}}}, false},
	}
	for _, tt := range tests {
		errs := checkRow(tt.row, classes, skills)
		if (len(errs) > 0) != tt.fail {
			t.Errorf("%s: errors %v, want failing %v", tt.name, errs, tt.fail)
		}
	}
}
