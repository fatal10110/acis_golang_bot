package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// checkDaggerPvPRows checks the dagger second rows TH, PW and AW of the
// row table at 66, where 321 Blinding Blow comes in as a damage blow
// (docs/pvp/rows-second.md, X24); checkSecondPvPRows holds 40, 55 and 74.
func checkDaggerPvPRows(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	want := map[string]string{
		"TH": "409 263 96 16",
		"PW": "102/CA 321 263 96 16",
		"AW": "129/CH 115/M 70 70 321 263 96 223 16",
	}
	m := &TravelMap{classes: classes, skills: skills}
	got := map[string]string{}
	for r, refs := range pvpRowTable(m, shippedKits, 66) {
		if _, ok := want[r.ID]; ok {
			got[r.ID] = refWords(t, skills, refs)
		}
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("level 66 row %s: steps %q, want %q", id, got[id], w)
		}
	}
}

// TestPvPRowDaggerKits checks the dagger rows' kits (docs/pvp/rows-second.md,
// X24): PW and AW set their own lists, AW learns Freezing Strike for its
// break-off, TH keeps the rogue's list (O3), and the RO, ES and AS lists
// stay as they are.
func TestPvPRowDaggerKits(t *testing.T) {
	for _, tc := range []struct {
		name string
		kit  ClassKit
		want []int32
	}{
		{"TH", treasureHunterKit, stepSkills(roguePvP)},
		{"PW", plainsWalkerKit, []int32{skillUltimateEvasion, skillEntangle, skillEntangle, skillSprint}},
		{"AW", abyssWalkerKit, []int32{skillUltimateEvasion, skillPoison, skillPowerBreak, skillFreezingStrike}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stepSkills(tc.kit.PvP); !slices.Equal(got, tc.want) {
				t.Errorf("kit PvP %v, want %v", got, tc.want)
			}
			for _, st := range tc.kit.PvP {
				if !slices.Contains(tc.kit.Skills, st.Skill) {
					t.Errorf("PvP step skill %d not in Skills %v", st.Skill, tc.kit.Skills)
				}
			}
		})
	}
	for _, tc := range []struct {
		name string
		got  []Step
		want []int32
	}{
		{"RO", roguePvP, []int32{skillUltimateEvasion, skillDash, skillDash, skillStunningShot}},
		{"ES", elvenScoutPvP, []int32{skillUltimateEvasion, skillEntangle, skillEntangle, skillEntangle, skillSprint, skillStunningShot}},
		{"AS", assassinPvP, []int32{skillUltimateEvasion, skillPoison, skillPowerBreak, skillStunningShot}},
	} {
		if got := stepSkills(tc.got); !slices.Equal(got, tc.want) {
			t.Errorf("%s list %v, want %v unchanged", tc.name, got, tc.want)
		}
	}
}

// lureOnMonster checks that the bot of class at level with a dagger, knowing
// skills, opens on a fresh monster with Lure: the player gate leaves the
// monster rotation as it is.
func lureOnMonster(t *testing.T, class, level int32, skills ...int32) {
	t.Helper()
	b := rowBrain(t, class, level, "DAGGER", skills...)
	lure := datapack.SkillRef{ID: skillLure, Level: 1}
	b.cfg.Travel.skills = map[datapack.SkillRef]datapack.Skill{lure: rowSkills[lure]}
	if a := b.fight(npcID, at(time.Second)); a.kind != actCast || a.skill != skillLure {
		t.Fatalf("action %v skill %d on a monster, want the cast of Lure", a.kind, a.skill)
	}
}

// TestPvPRowTH checks the treasure hunter against a player: it plays the
// rogue's row, and its M10 Lure opener does not go on a player at 52 (an
// AGGDAMAGE without TargetMe does nothing to one), while it still opens on
// a monster.
func TestPvPRowTH(t *testing.T) {
	all := []int32{skillLure, skillBleed, skillMortalBlow, skillDash, skillUltimateEvasion}
	far := func(t *testing.T, b *Brain) { foeOf(t, b, lyrMystic, Point{Y: 900}, 0) }
	playRow(t, classTreasureHunter, []rowCase{
		{name: "a Warrior at 52: no Lure, Bleed", level: 52, weapon: "DAGGER", skills: all, opp: lyrWarrior, want: skillBleed},
		{name: "a Human Mystic at 900: Dash (as RO)", level: 52, weapon: "DAGGER", skills: all, opp: lyrMystic, setup: far, want: skillDash},
		{name: "breaking off: Dash (as RO)", level: 52, weapon: "DAGGER", skills: all, opp: lyrWarrior, setup: breakingOff, want: skillDash},
	})
	t.Run("a monster at 52: Lure", func(t *testing.T) { lureOnMonster(t, classTreasureHunter, 52, all...) })
}

// TestPvPRowPW checks the plains walker's PvP row: Entangle on a caster and
// an archer, not on a Warrior; no Lure on a player at 52, while it opens on
// a monster; Entangle, then Sprint, to break off; Ultimate Evasion under
// 30 % only vs M, A.
func TestPvPRowPW(t *testing.T) {
	all := []int32{skillEntangle, skillLure, skillBleed, skillMortalBlow, skillSprint, skillUltimateEvasion}
	low := func(_ *testing.T, b *Brain) { b.world.Self.HP = 250 }
	playRow(t, classPlainsWalker, []rowCase{
		{name: "a Human Mystic: Entangle", level: 52, weapon: "DAGGER", skills: all, opp: lyrMystic, want: skillEntangle},
		{name: "an archer: Entangle", level: 52, weapon: "DAGGER", skills: all, opp: lyrWarrior, setup: func(_ *testing.T, b *Brain) { asArcher(b) }, want: skillEntangle},
		{name: "a Warrior at 52: no Entangle, no Lure, Bleed", level: 52, weapon: "DAGGER", skills: all, opp: lyrWarrior, want: skillBleed},
		{name: "a Human Mystic, Entangle cooling: no Lure, Bleed", level: 52, weapon: "DAGGER", skills: all, opp: lyrMystic, setup: cooling(skillEntangle), want: skillBleed},
		{name: "breaking off: Entangle", level: 52, weapon: "DAGGER", skills: all, opp: lyrWarrior, setup: breakingOff, want: skillEntangle},
		{name: "breaking off, Entangle cooling: Sprint", level: 52, weapon: "DAGGER", skills: all, opp: lyrWarrior, setup: both(breakingOff, cooling(skillEntangle)), want: skillSprint},
		{name: "under 30 % against a Warrior: Ultimate Evasion", level: 52, weapon: "DAGGER", skills: all, opp: lyrWarrior, setup: low, want: skillUltimateEvasion},
		{name: "under 30 % against a Human Mystic: no Ultimate Evasion", level: 52, weapon: "DAGGER", skills: all, opp: lyrMystic, setup: both(low, cooling(skillEntangle)), not: []int32{skillUltimateEvasion}},
	})
	t.Run("a monster at 52, Entangle unknown: Lure", func(t *testing.T) {
		lureOnMonster(t, classPlainsWalker, 52, skillLure, skillBleed, skillMortalBlow)
	})
}

// TestPvPRowAW checks the abyss walker's PvP row: Drain Health opens (M10);
// then Poison on a Cleric and a caster, not on a Warrior; Power Break on a
// Warrior, not on a caster; Freezing Strike only to break off.
func TestPvPRowAW(t *testing.T) {
	all := []int32{skillPoison, skillPowerBreak, skillFreezingStrike, skillDrainHealth, skillBleed, skillSting, skillMortalBlow}
	playRow(t, classAbyssWalker, []rowCase{
		{name: "a Cleric: Drain Health opens", level: 40, weapon: "DAGGER", skills: all, opp: lyrCleric, want: skillDrainHealth},
		{name: "a Cleric, opened: Poison", level: 40, weapon: "DAGGER", skills: all, opp: lyrCleric, setup: opened, want: skillPoison},
		{name: "a Human Mystic, opened: Poison, no Power Break", level: 40, weapon: "DAGGER", skills: all, opp: lyrMystic, setup: opened, want: skillPoison},
		{
			name: "a Human Mystic, Poison cooling: no Power Break, no Freezing Strike", level: 40, weapon: "DAGGER", skills: all, opp: lyrMystic,
			setup: both(opened, cooling(skillPoison)), want: skillBleed,
		},
		{name: "a Warrior, opened: Power Break, no Poison", level: 40, weapon: "DAGGER", skills: all, opp: lyrWarrior, setup: opened, want: skillPowerBreak},
		{
			name: "a Warrior, Power Break cooling: no Freezing Strike", level: 40, weapon: "DAGGER", skills: all, opp: lyrWarrior,
			setup: both(opened, cooling(skillPowerBreak)), want: skillBleed,
		},
		{
			name: "breaking off: Freezing Strike", level: 40, weapon: "DAGGER", skills: all, opp: lyrWarrior,
			setup: both(opened, breakingOff, cooling(skillDrainHealth)), want: skillFreezingStrike,
		},
	})
}
