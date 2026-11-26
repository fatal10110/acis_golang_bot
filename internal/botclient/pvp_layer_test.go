package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The PvP layer's fixtures (docs/pvp/layer.md "Tests"): test-built steps,
// the opponent classes (Human Fighter 0, Warrior 1, Human Mystic 10,
// Cleric 15) and the Cleric's Heal (1011, hit 5000).
const (
	lyrStun    = int32(9201) // a stun, hit 1000 at attack speed 333
	lyrSleep   = int32(9202) // a magic sleep, hit 4000
	lyrNuke    = int32(9203) // a magic damage step, hit 1000, power set per case
	lyrHeal    = skillSelfHeal
	lyrStunLow = int32(9205) // a stun landing at effect power 10
	lyrStunHi  = int32(9206) // a stun landing at effect power 90
	lyrBigNuke = int32(9207) // a magic damage step of power 1e4
	lyrWarrior = int32(1)
	lyrMystic  = int32(10)
	lyrCleric  = int32(15)
)

// layerSkills are the datapack entries the layer tests cast or see cast.
var layerSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: lyrStun, Level: 1}:    {Type: "STUN", Target: "ONE", Offensive: true, HitTime: time.Second, EffectPower: 90, Effects: []datapack.Effect{{Name: "Stun", Time: 9, EffectPower: -1}}},
	{ID: lyrSleep, Level: 1}:   {Type: "SLEEP", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Effects: []datapack.Effect{{Name: "Sleep", Time: 30}}},
	{ID: lyrNuke, Level: 1}:    {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second, Power: 1},
	{ID: lyrHeal, Level: 1}:    {Type: "HEAL", Target: "SELF", Magic: true, HitTime: 4 * time.Second, Power: 100},
	{ID: lyrStunLow, Level: 1}: {Type: "STUN", Target: "ONE", Offensive: true, HitTime: time.Second, EffectPower: 10, Effects: []datapack.Effect{{Name: "Stun", Time: 9, EffectPower: -1}}},
	{ID: lyrStunHi, Level: 1}:  {Type: "STUN", Target: "ONE", Offensive: true, HitTime: time.Second, EffectPower: 90, Effects: []datapack.Effect{{Name: "Stun", Time: 9, EffectPower: -1}}},
	{ID: lyrBigNuke, Level: 1}: {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second, Power: 1e4},
	{ID: skillHeal, Level: 1}:  {Type: "HEAL", Target: "ONE", Magic: true, HitTime: 5 * time.Second, Power: 100},
	// Dryad Root as the guards', landing at effect power 80.
	{ID: grdRoot, Level: 1}: {Type: "ROOT", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second, EffectPower: 80, Effects: []datapack.Effect{{Name: "Root", Time: 30, EffectPower: -1}}},
}

// layerClass is a test class template: id under parent, CON 40, MEN 39.
func layerClass(id, parent int32) datapack.Class {
	return datapack.Class{
		ID: id, Parent: parent, STR: 40, CON: 40, DEX: 30, INT: 20, WIT: 20, MEN: 39,
		PAtk: 4, PDef: 80, MAtk: 6, MDef: 47, RunSpeed: 120,
		HPTable: []float64{100}, MPTable: []float64{50}, CPTable: []float64{50},
	}
}

// layerBrain is guardBrain with the layer skills, the opponent classes, a
// level 20 bot of 1000 HP with P. Atk and M. Atk 100 running at 120, and the
// opponent pcID of class (300 away, flagged, hitting the bot).
func layerBrain(t *testing.T, class int32, steps ...Step) *Brain {
	t.Helper()
	b := guardBrain(t, false, 1, 0, steps...)
	for ref, s := range layerSkills {
		b.cfg.Travel.skills[ref] = s
	}
	b.cfg.Travel.classes = map[int32]datapack.Class{
		0: layerClass(0, -1), lyrWarrior: layerClass(lyrWarrior, 0),
		lyrMystic: layerClass(lyrMystic, -1), lyrCleric: layerClass(lyrCleric, lyrMystic),
	}
	s := &b.world.Self
	s.Level, s.HP, s.MaxHP, s.MP, s.MaxMP, s.PAtk, s.MAtk, s.RunSpeed = 20, 1000, 1000, 1000, 1000, 100, 100, 120
	withKit(b, ClassKit{Rotation: steps})
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: class, Pos: Point{Y: 300}, RunSpeed: 124, Running: true, PvPFlag: 1, CollisionRadius: 9}, t0)
	return b
}

// castOf is the decision of b at now, the skill of a cast or 0 for any
// other action.
func castOf(t *testing.T, b *Brain, now time.Time) int32 {
	t.Helper()
	_, a := b.decide(now)
	if a.kind != actCast {
		return 0
	}
	return a.skill
}

// TestPvPStepGates checks the PvP step fields' gates (docs/pvp/layer.md
// "Step fields") on a monster, a melee player and a caster.
func TestPvPStepGates(t *testing.T) {
	monster := stepState{}
	melee := stepState{player: true, vs: charmodel.Melee}
	caster := stepState{player: true, vs: charmodel.Caster}
	healer := stepState{player: true, vs: charmodel.Healer}
	answer := stepState{player: true, vs: charmodel.Caster, interrupt: true}
	tests := []struct {
		name string
		step Step
		st   stepState
		want bool
	}{
		{"Vs: no monster", Step{Vs: []charmodel.Archetype{charmodel.Melee}}, monster, false},
		{"Vs: a listed archetype", Step{Vs: []charmodel.Archetype{charmodel.Melee}}, melee, true},
		{"Vs: another archetype", Step{Vs: []charmodel.Archetype{charmodel.Melee}}, caster, false},
		{"PvP: no monster", Step{PvP: true}, monster, false},
		{"PvP: a player", Step{PvP: true}, melee, true},
		{"Interrupt: not at reuse", Step{Interrupt: true}, melee, false},
		{"Interrupt: as an answer", Step{Interrupt: true}, answer, true},
		{"Hold: kept against a caster", Step{Hold: true}, caster, false},
		{"Hold: kept against a healer", Step{Hold: true}, healer, false},
		{"Hold: as an answer", Step{Hold: true}, answer, true},
		{"Hold: at reuse against melee", Step{Hold: true}, melee, true},
		{"Hold: at reuse against a monster", Step{Hold: true}, monster, true},
		{"Break: only while breaking off", Step{Break: true}, melee, false},
		{"Break: breaking off", Step{Break: true}, stepState{player: true, breaking: true}, true},
		{"Peel: only on a peel", Step{Peel: true}, melee, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.step.passes(tc.st); got != tc.want {
				t.Errorf("passes %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPInterruptHeal checks the interrupt of a heal (docs/pvp/layer.md
// "Interrupts"): a Hold stun is not cast at reuse against a Cleric, goes
// before the Cleric's Heal (hit 5000) lands, once per cast, and is cast at
// reuse against a Warrior.
func TestPvPInterruptHeal(t *testing.T) {
	stun := Step{Skill: lyrStun, Hold: true}
	t.Run("kept against a Cleric, then the interrupt", func(t *testing.T) {
		b := layerBrain(t, lyrCleric, stun)
		if got := castOf(t, b, at(time.Second)); got == lyrStun {
			t.Fatal("stun cast at reuse against a Cleric")
		}
		heal := MagicSkillUse{CasterID: pcID, TargetID: pcID, SkillID: skillHeal, Level: 1, HitTime: 5 * time.Second}
		b.handleOne(t, heal, at(2*time.Second))
		if got := castOf(t, b, at(2600*time.Millisecond)); got != lyrStun {
			t.Fatalf("cast %d on the Heal, want the stun %d", got, lyrStun)
		}
		if got := castOf(t, b, at(2700*time.Millisecond)); got == lyrStun {
			t.Fatal("a second interrupt on the same cast")
		}
	})
	t.Run("at reuse against a Warrior", func(t *testing.T) {
		b := layerBrain(t, lyrWarrior, stun)
		if got := castOf(t, b, at(time.Second)); got != lyrStun {
			t.Fatalf("cast %d, want the stun %d at reuse", got, lyrStun)
		}
	})
}

// TestPvPInterruptTooLate checks the interrupts that would land too late
// (docs/pvp/layer.md "Interrupts"): the answer's hit time and the reaction
// delay (PvPReactMax, 600 ms) from the MagicSkillUse must end before the
// landing, a damage step's escLead before it on a magic cast and at
// MinBreak or more.
func TestPvPInterruptTooLate(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		power   float64
		cast    datapack.Skill
		castSpd int32
		auto    bool
		stack   bool
		want    bool
	}{
		{name: "stun ends after the landing", step: Step{Skill: lyrStun, Interrupt: true}, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 1500 * time.Millisecond}},
		{name: "stun ends before the landing", step: Step{Skill: lyrStun, Interrupt: true}, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 1700 * time.Millisecond}, want: true},
		{name: "damage under MinBreak", step: Step{Skill: lyrNuke, Interrupt: true}, power: 1, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 5 * time.Second}},
		{name: "damage in the cast's last 200 ms", step: Step{Skill: lyrNuke, Interrupt: true}, power: 1e5, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 1700 * time.Millisecond}},
		{name: "damage 200 ms before the landing", step: Step{Skill: lyrNuke, Interrupt: true}, power: 1e5, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 1800 * time.Millisecond}, want: true},
		{name: "damage on a non-magic cast", step: Step{Skill: lyrNuke, Interrupt: true}, power: 1e5, cast: datapack.Skill{Type: "HEAL", HitTime: 5 * time.Second}},
		{name: "sleep at 260, no auto-use (5.1 s)", step: Step{Skill: lyrSleep, Interrupt: true}, castSpd: 260, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 4500 * time.Millisecond}},
		{name: "sleep at 260, auto-use and a stack (3.6 s)", step: Step{Skill: lyrSleep, Interrupt: true}, castSpd: 260, auto: true, stack: true, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 4500 * time.Millisecond}, want: true},
		{name: "sleep at 260, auto-use, no stack", step: Step{Skill: lyrSleep, Interrupt: true}, castSpd: 260, auto: true, cast: datapack.Skill{Type: "HEAL", Magic: true, HitTime: 4500 * time.Millisecond}},
		{name: "a buff is no interrupt", step: Step{Skill: lyrStun, Interrupt: true}, cast: datapack.Skill{Type: "BUFF", Magic: true, HitTime: 5 * time.Second}},
		{name: "a short MDAM is no interrupt", step: Step{Skill: lyrStun, Interrupt: true}, cast: datapack.Skill{Type: "MDAM", Magic: true, HitTime: 2900 * time.Millisecond}},
	}
	const castSkill = int32(9290)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := layerBrain(t, lyrCleric, tc.step)
			if tc.power > 0 {
				nuke := layerSkills[datapack.SkillRef{ID: lyrNuke, Level: 1}]
				nuke.Power = tc.power
				b.cfg.Travel.skills[datapack.SkillRef{ID: lyrNuke, Level: 1}] = nuke
			}
			b.cfg.Travel.skills[datapack.SkillRef{ID: castSkill, Level: 1}] = tc.cast
			if tc.castSpd > 0 {
				b.world.Self.CastSpd = tc.castSpd
			}
			switch {
			case tc.stack:
				carryBlessed(b)
			case tc.auto:
				b.persona.AutoShots = false
				b.world.Apply(ExAutoSoulShot{TemplateID: grdBlessed, Enabled: true}, t0)
			}
			b.handleOne(t, MagicSkillUse{CasterID: pcID, TargetID: selfID, SkillID: castSkill, Level: 1, HitTime: tc.cast.HitTime}, at(time.Second))
			if got := castOf(t, b, at(time.Second)) == tc.step.Skill; got != tc.want {
				t.Errorf("interrupt %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPLongCastRule checks the long-cast rule (docs/pvp/layer.md "PvP
// order"): under its HP gate, no Self Heal (hit 4000) while a melee
// opponent stands in reach, unless its mask shows a stun; out of reach the
// heal goes.
func TestPvPLongCastRule(t *testing.T) {
	heal := Step{Skill: lyrHeal, Target: TargetSelf, SelfHPBelow: 50}
	tests := []struct {
		name     string
		pos      Point
		abnormal int32
		want     bool
	}{
		{"melee opponent in reach", Point{Y: 100}, 0, false},
		{"melee opponent stunned", Point{Y: 100}, 0x40, true},
		{"melee opponent rooted", Point{Y: 100}, rootAbnormal, true},
		{"melee opponent out of reach", Point{Y: 300}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := layerBrain(t, 0, heal)
			b.world.Self.HP = 400
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: 0, Pos: tc.pos, RunSpeed: 124, Running: true, PvPFlag: 1, Abnormal: tc.abnormal, CollisionRadius: 9}, at(time.Second))
			b.world.object(pcID).move = stopAt(tc.pos)
			if got := castOf(t, b, at(time.Second)) == lyrHeal; got != tc.want {
				t.Errorf("heal %v, want %v", got, tc.want)
			}
			if _, got := b.sustain(at(time.Second)); got != tc.want {
				t.Errorf("critical sustain %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPControlLandRate checks the control steps (docs/pvp/layer.md "PvP
// order"): a stun landing under MinLand on the reference opponent is
// skipped, one at MinLand or more cast; a root is cast again once it wore
// off and not after two 139s.
func TestPvPControlLandRate(t *testing.T) {
	for _, tc := range []struct {
		skill int32
		want  bool
	}{{lyrStunLow, false}, {lyrStunHi, true}} {
		b := layerBrain(t, lyrWarrior, Step{Skill: tc.skill})
		bot, opp, ok := b.pvpFighters(pcID)
		if !ok {
			t.Fatal("no fighters")
		}
		rate := charmodel.LandRate(b.cfg.Travel.skills[datapack.SkillRef{ID: tc.skill, Level: 1}], bot, opp)
		if rate >= MinLand != tc.want {
			t.Fatalf("skill %d lands at %.1f %%, the case needs the other side of %d", tc.skill, rate, MinLand)
		}
		if got := castOf(t, b, at(time.Second)) == tc.skill; got != tc.want {
			t.Errorf("skill %d (%.1f %%): cast %v, want %v", tc.skill, rate, got, tc.want)
		}
	}
	t.Run("root again once worn off", func(t *testing.T) {
		b := layerBrain(t, lyrWarrior, Step{Skill: grdRoot})
		rooted := CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, RunSpeed: 124, Running: true, PvPFlag: 1, Abnormal: rootAbnormal, CollisionRadius: 9}
		free := rooted
		free.Abnormal = 0
		play(t, b, []beat{
			{at: 0, kind: actCast, skill: grdRoot},
			{at: 100 * time.Millisecond, ev: selfCast(grdRoot, time.Second), noDecide: true},
			{at: 1200 * time.Millisecond, ev: rooted, noDecide: true},
		})
		if got := castOf(t, b, at(2*time.Second)); got == grdRoot {
			t.Fatal("root cast again while it holds")
		}
		b.handleOne(t, free, at(5*time.Second))
		if got := castOf(t, b, at(5*time.Second)); got != grdRoot {
			t.Fatalf("cast %d once the root wore off, want the root", got)
		}
	})
	t.Run("two 139s", func(t *testing.T) {
		b := layerBrain(t, lyrWarrior, Step{Skill: grdRoot})
		resist := SystemMessage{ID: msgResisted}
		play(t, b, []beat{
			{at: 0, kind: actCast, skill: grdRoot},
			{at: 100 * time.Millisecond, ev: selfCast(grdRoot, time.Second), noDecide: true},
			{at: 1100 * time.Millisecond, ev: resist, noDecide: true},
			{at: 5 * time.Second, kind: actCast, skill: grdRoot},
			{at: 5100 * time.Millisecond, ev: selfCast(grdRoot, time.Second), noDecide: true},
			{at: 6100 * time.Millisecond, ev: resist, noDecide: true},
		})
		if got := castOf(t, b, at(10*time.Second)); got == grdRoot {
			t.Fatal("a third root after two 139s")
		}
	})
}

// TestPvPSleepRule checks the sleep rule (docs/pvp/layer.md "Sleep and
// damage"): no holding sleep on an opponent showing an HP DOT, nor while a
// DOT the bot landed still ticks; else it may.
func TestPvPSleepRule(t *testing.T) {
	const venom = int32(9295)
	tests := []struct {
		name     string
		abnormal int32
		venomAt  time.Duration
		want     bool
	}{
		{name: "no DOT", want: true},
		{name: "poison bit", abnormal: 0x2},
		{name: "bleeding bit", abnormal: 0x1},
		{name: "flame bit", abnormal: 0x4000},
		{name: "own DOT ticking", venomAt: 25 * time.Second},
		{name: "own DOT over", venomAt: 35 * time.Second, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := layerBrain(t, lyrMystic)
			b.cfg.Travel.skills[datapack.SkillRef{ID: venom, Level: 1}] = datapack.Skill{Type: "POISON", Magic: true, Effects: []datapack.Effect{{Name: "DamOverTime", Time: 3, Count: 10}}}
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: lyrMystic, Pos: Point{Y: 300}, PvPFlag: 1, Abnormal: tc.abnormal}, t0)
			now := at(time.Second)
			if tc.venomAt > 0 {
				b.layerCast(MagicSkillUse{CasterID: selfID, TargetID: pcID, SkillID: venom, Level: 1}, t0)
				now = at(tc.venomAt)
			}
			if got := b.sleepHolds(pcID, now); got != tc.want {
				t.Errorf("sleep holds %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPFinisher checks the finishers (docs/pvp/layer.md "PvP order"): in
// row order while the pool estimate is unknown or above every step's
// expected damage, the step whose expected damage covers it first once it
// is under.
func TestPvPFinisher(t *testing.T) {
	steps := []Step{{Skill: lyrNuke}, {Skill: lyrBigNuke}}
	b := layerBrain(t, lyrWarrior, steps...)
	if got := castOf(t, b, at(time.Second)); got != lyrNuke {
		t.Fatalf("cast %d with the pool unknown, want %d", got, lyrNuke)
	}
	r := b.pvp.opponents[pcID]
	r.HP, r.MaxHP, r.CP = 2000, 2000, 0
	bot, opp, _ := b.pvpFighters(pcID)
	big := charmodel.SkillDamage(bot, layerSkills[datapack.SkillRef{ID: lyrBigNuke, Level: 1}], opp)
	if big < 2000 {
		t.Fatalf("big nuke deals %.0f, the case needs it over the pool", big)
	}
	r.HP, r.MaxHP = 1e9, 1e9
	if got := castOf(t, b, at(time.Second)); got != lyrNuke {
		t.Fatalf("cast %d with the pool above every step, want %d", got, lyrNuke)
	}
	r.HP, r.CP = int32(big)/2, int32(big)/4
	if got := castOf(t, b, at(time.Second)); got != lyrBigNuke {
		t.Fatalf("cast %d with the pool under the big nuke, want the finisher %d", got, lyrBigNuke)
	}
}

// TestPvPRowTable checks the PvP row table (docs/pvp/model.md "Row
// table") on a test kit: damage, DOT and control steps at the class tree's
// skill level, Vs copied, buffs and later steps left out, a scout's bow and
// dagger steps split to its branches.
func TestPvPRowTable(t *testing.T) {
	const (
		nuke, dot, root, buff = int32(9301), int32(9302), int32(9303), int32(9304)
		rogue                 = int32(7)
	)
	m := &TravelMap{
		skills: map[datapack.SkillRef]datapack.Skill{
			{ID: nuke, Level: 1}: {Type: "MDAM", Power: 10}, {ID: nuke, Level: 2}: {Type: "MDAM", Power: 20},
			{ID: dot, Level: 1}:  {Type: "POISON", Effects: []datapack.Effect{{Name: "DamOverTime", Time: 3, Count: 10}}},
			{ID: root, Level: 1}: {Type: "ROOT", Effects: []datapack.Effect{{Name: "Root", Time: 30}}},
			{ID: buff, Level: 1}: {Type: "BUFF"},
		},
		classes: map[int32]datapack.Class{
			lyrMystic: {ID: lyrMystic, Parent: -1, Skills: []datapack.SkillGrant{{ID: nuke, Level: 1, MinLevel: 1}, {ID: nuke, Level: 2, MinLevel: 10}, {ID: root, Level: 1, MinLevel: 5}, {ID: buff, Level: 1, MinLevel: 1}}},
			rogue:     {ID: rogue, Parent: 0, Skills: []datapack.SkillGrant{{ID: nuke, Level: 1, MinLevel: 1}, {ID: dot, Level: 1, MinLevel: 1}}},
		},
	}
	kits := []map[int32]ClassKit{{
		lyrMystic: {PvP: []Step{{Skill: nuke}, {Skill: root, Vs: []charmodel.Archetype{charmodel.Melee}}, {Skill: buff}, {Skill: dot, From: 30}}},
		rogue:     {PvP: []Step{{Skill: nuke, Weapons: []string{"BOW"}}, {Skill: dot, Weapons: []string{"DAGGER"}}}},
	}}
	got := pvpRowTable(m, kits, 12)
	hm, _ := charmodel.ClassRow(lyrMystic, false)
	ro, _ := charmodel.ClassRow(rogue, false)
	roBow, _ := charmodel.ClassRow(rogue, true)
	if steps := got[hm]; len(steps) != 2 || steps[0].Skill.Power != 20 || steps[0].Level != 1 ||
		steps[1].Skill.Type != "ROOT" || steps[1].Level != 5 || len(steps[1].Vs) != 1 {
		t.Errorf("HM row %+v, want the level 2 nuke from 1 and the root from 5 vs M", steps)
	}
	if steps := got[roBow]; len(steps) != 1 || steps[0].Skill.Type != "MDAM" {
		t.Errorf("RO bow row %+v, want the bow step", steps)
	}
	if steps := got[ro]; len(steps) != 1 || steps[0].Skill.Type != "POISON" {
		t.Errorf("RO dagger row %+v, want the dagger step", steps)
	}
	if len(pvpRowTable(nil, kits, 12)) != 0 {
		t.Error("a row table without a travel map")
	}
}
