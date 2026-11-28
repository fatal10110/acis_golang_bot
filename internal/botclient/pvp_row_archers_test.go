package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// checkArcherPvPRows checks the archer second rows HK, SR and PR of the row
// table at 46, where 24 Burst Shot comes in for HK and SR, and at 60, where
// 314 Fatal Counter comes in for PR (docs/pvp/rows-second.md, X25);
// checkSecondPvPRows holds 40, 55 and 74.
func checkArcherPvPRows(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	want := map[int]map[string]string{
		46: {"HK": "101 19 24 56", "SR": "102/M 101 19 24 56", "PR": "105/M 129/CH 101 70 19 56"},
		60: {"HK": "101 19 24 56", "SR": "102/M 101 19 24 56", "PR": "105/M 129/CH 101 70 314 19 56"},
	}
	m := &TravelMap{classes: classes, skills: skills}
	for level, rows := range want {
		got := map[string]string{}
		for r, refs := range pvpRowTable(m, shippedKits, level) {
			if _, ok := rows[r.ID]; ok {
				got[r.ID] = refWords(t, skills, refs)
			}
		}
		for id, w := range rows {
			if got[id] != w {
				t.Errorf("level %d row %s: steps %q, want %q", level, id, got[id], w)
			}
		}
	}
}

// TestPvPRowArcherKits checks the archer rows' kits (docs/pvp/rows-second.md,
// X25): SR and PR set their own lists, PR learns Freezing Strike for its
// row, HK keeps the rogue's list (O3).
func TestPvPRowArcherKits(t *testing.T) {
	for _, tc := range []struct {
		name string
		kit  ClassKit
		want []int32
	}{
		{"HK", hawkeyeKit, stepSkills(roguePvP)},
		{"SR", silverRangerKit, []int32{skillUltimateEvasion, skillEntangle, skillEntangle, skillSprint, skillStunningShot}},
		{"PR", phantomRangerKit, []int32{skillUltimateEvasion, skillFreezingStrike, skillPoison, skillFreezingStrike, skillStunningShot}},
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
}

// archerKites checks that the bow row of class at 40, after setup, kites
// a Warrior 300 away on its slow: it casts skill (cast range 600, hit
// 3000), and once the slow lands walks to a stand point at the cast range
// less walkSlop before it casts next there, skill still in its reuse.
func archerKites(t *testing.T, class, skill int32, skills []int32, setup func(*testing.T, *Brain), next int32) {
	t.Helper()
	b := pvpRowBrain(t, class, 40, "BOW", lyrWarrior, skills...)
	// The worn weapon is a bow (Brain.bowEquipped): the bow row kites.
	b.cfg.Travel.arrows = map[int32]int32{rowWeaponID: 17}
	if setup != nil {
		setup(t, b)
	}
	play(t, b, []beat{
		{at: time.Second, kind: actCast, skill: skill},
		{at: time.Second + 100*time.Millisecond, ev: selfCast(skill, 3*time.Second), noDecide: true},
	})
	b.world.holdSkill(skill, at(time.Minute)) // its reuse
	if _, ok := b.kiting(pcID, at(4500*time.Millisecond)); !ok {
		t.Fatalf("skill %d does not count for the kite", skill)
	}
	now := at(4500 * time.Millisecond)
	_, a := b.decide(now)
	want := Point{Y: 300 - 584}
	if got := moveDest(t, a); got != want {
		t.Fatalf("kite to %v, want %v", got, want)
	}
	if _, err := b.send(a, now); err != nil {
		t.Fatal(err)
	}
	b.world.Self.move = stopAt(want)
	if got := castOf(t, b, at(6*time.Second)); got != next {
		t.Fatalf("cast %d at the stand point, want %d", got, next)
	}
}

// TestPvPRowHK checks the hawkeye against a player: it plays the rogue's
// row with a bow, Stunning Shot at reuse on a Warrior, held on a Cleric
// until its Heal.
func TestPvPRowHK(t *testing.T) {
	bow := []int32{skillStunningShot, skillPowerShot, skillDash, skillUltimateEvasion}
	playRow(t, classHawkeye, []rowCase{
		{name: "a Warrior: Stunning Shot at reuse", level: 40, weapon: "BOW", skills: bow, opp: lyrWarrior, want: skillStunningShot},
		{name: "a Cleric: Stunning Shot held", level: 40, weapon: "BOW", skills: bow, opp: lyrCleric, want: skillPowerShot},
		{name: "the Cleric's Heal: Stunning Shot", level: 40, weapon: "BOW", skills: bow, opp: lyrCleric, heal: true, want: skillStunningShot},
	})
}

// TestPvPRowSR checks the silver ranger's PvP row: Entangle on a Warrior,
// not on a caster, then the kite on it; Stunning Shot held on a Cleric
// until its Heal; Entangle, then Sprint, to break off; Ultimate Evasion
// under 30 % only vs M, A.
func TestPvPRowSR(t *testing.T) {
	all := []int32{skillEntangle, skillStunningShot, skillPowerShot, skillSprint, skillUltimateEvasion}
	low := func(_ *testing.T, b *Brain) { b.world.Self.HP = 250 }
	playRow(t, classSilverRanger, []rowCase{
		{name: "a Warrior: Entangle", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, want: skillEntangle},
		{name: "a Warrior, Entangle cooling: Stunning Shot at reuse", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, setup: cooling(skillEntangle), want: skillStunningShot},
		{name: "a Cleric: no Entangle, Stunning Shot held", level: 40, weapon: "BOW", skills: all, opp: lyrCleric, want: skillPowerShot},
		{name: "the Cleric's Heal: Stunning Shot", level: 40, weapon: "BOW", skills: all, opp: lyrCleric, heal: true, want: skillStunningShot},
		{name: "breaking off: Entangle", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, setup: breakingOff, want: skillEntangle},
		{name: "breaking off, Entangle cooling: Sprint", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, setup: both(breakingOff, cooling(skillEntangle)), want: skillSprint},
		{name: "under 30 % against a Warrior: Ultimate Evasion", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, setup: low, want: skillUltimateEvasion},
		{name: "under 30 % against a Human Mystic: no Ultimate Evasion", level: 40, weapon: "BOW", skills: all, opp: lyrMystic, setup: low, not: []int32{skillUltimateEvasion}},
	})
	t.Run("a Warrior: Entangle, then the kite", func(t *testing.T) { archerKites(t, classSilverRanger, skillEntangle, all, nil, skillStunningShot) })
}

// TestPvPRowPR checks the phantom ranger's PvP row: on a Warrior Stunning
// Shot at reuse (a control step, ahead of every damage step in the PvP
// order), else Freezing Strike, then the kite on its slow; no Freezing
// Strike on a caster; Poison on a Cleric and a caster, not on a Warrior
// nor an archer; Stunning Shot held on a Cleric until its Heal; Freezing
// Strike to break off.
func TestPvPRowPR(t *testing.T) {
	all := []int32{skillFreezingStrike, skillPoison, skillStunningShot, skillPowerShot, skillUltimateEvasion}
	playRow(t, classPhantomRanger, []rowCase{
		{name: "a Warrior: Stunning Shot at reuse", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, want: skillStunningShot},
		{name: "a Warrior, Stunning Shot cooling: Freezing Strike", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, setup: cooling(skillStunningShot), want: skillFreezingStrike},
		{
			name: "a Warrior, Stunning Shot and Freezing Strike cooling: no Poison", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior,
			setup: cooling(skillStunningShot, skillFreezingStrike), want: skillPowerShot,
		},
		{name: "a Cleric: Poison, no Freezing Strike", level: 40, weapon: "BOW", skills: all, opp: lyrCleric, want: skillPoison},
		{name: "a Human Mystic: Poison", level: 40, weapon: "BOW", skills: all, opp: lyrMystic, want: skillPoison},
		{
			name: "an archer: no Freezing Strike, no Poison", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior,
			setup: func(_ *testing.T, b *Brain) { asArcher(b) }, not: []int32{skillFreezingStrike, skillPoison},
		},
		{name: "a Cleric, Poison cooling: Stunning Shot held", level: 40, weapon: "BOW", skills: all, opp: lyrCleric, setup: cooling(skillPoison), want: skillPowerShot},
		{name: "the Cleric's Heal: Stunning Shot", level: 40, weapon: "BOW", skills: all, opp: lyrCleric, setup: cooling(skillPoison), heal: true, want: skillStunningShot},
		{name: "breaking off: Freezing Strike", level: 40, weapon: "BOW", skills: all, opp: lyrWarrior, setup: breakingOff, want: skillFreezingStrike},
	})
	t.Run("a Warrior: Freezing Strike, then the kite", func(t *testing.T) {
		archerKites(t, classPhantomRanger, skillFreezingStrike, all, cooling(skillStunningShot), skillPowerShot)
	})
}
