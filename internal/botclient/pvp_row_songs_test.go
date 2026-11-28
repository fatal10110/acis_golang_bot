package botclient

import (
	"slices"
	"testing"
	"time"
)

// TestPvPRowSongKits checks the song rows' kits (docs/pvp/rows-second.md,
// X22): SS and BD set their own lists and learn Arrest, the EK and PK
// lists stay as they are, and against a player Aggression is only a Peel
// (the M10 opener step of Aggression does not play).
func TestPvPRowSongKits(t *testing.T) {
	for _, tc := range []struct {
		name string
		kit  ClassKit
		pvp  []Step
	}{
		{"SS", swordSingerKit, swordSingerPvP},
		{"BD", bladedancerKit, bladedancerPvP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.EqualFunc(tc.kit.PvP, tc.pvp, func(a, b Step) bool { return a.Skill == b.Skill && a.Break == b.Break && a.Peel == b.Peel }) {
				t.Errorf("kit PvP %+v, want the %s list", tc.kit.PvP, tc.name)
			}
			if !slices.Contains(tc.kit.Skills, skillArrest) {
				t.Errorf("Skills %v without Arrest", tc.kit.Skills)
			}
			for _, st := range pvpSteps(tc.kit.PvP, tc.kit.Rotation) {
				if st.Skill == skillAggression && !st.Peel {
					t.Errorf("Aggression step %+v is not a Peel", st)
				}
			}
		})
	}
	for _, tc := range []struct {
		name string
		got  []Step
		want []int32
	}{
		{"EK", elvenKnightPvP, []int32{skillUltimateDefense, skillDeflectArrow, skillEntangle, skillEntangle, skillAggression}},
		{"PK", palusKnightPvP, []int32{skillUltimateDefense, skillDeflectArrow, skillPoison, skillPowerBreak, skillFreezingStrike, skillAggression}},
	} {
		got := make([]int32, len(tc.got))
		for i, st := range tc.got {
			got[i] = st.Skill
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s list %v, want %v unchanged", tc.name, got, tc.want)
		}
	}
}

// TestPvPRowSS checks the sword singer's PvP row: Entangle on a caster,
// not on a Warrior; Arrest on a Cleric from 55, not at 54; Arrest within its
// 150 reach, then Entangle, to break off; never Aggression against a lone opponent.
func TestPvPRowSS(t *testing.T) {
	all := []int32{skillEntangle, skillArrest, skillPowerStrike, skillAggression}
	playRow(t, classSwordSinger, []rowCase{
		{name: "a Human Mystic: Entangle", level: 55, weapon: "SWORD", skills: all, opp: lyrMystic, want: skillEntangle},
		{name: "a Warrior: no Entangle, no Arrest, no Aggression", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior, want: skillPowerStrike},
		{name: "a Cleric at 55: Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrCleric, want: skillArrest},
		{name: "a Cleric at 54: no Arrest", level: 54, weapon: "SWORD", skills: all, opp: lyrCleric, want: skillPowerStrike},
		{name: "a Human Mystic, Entangle cooling: Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrMystic, setup: cooling(skillEntangle), want: skillArrest},
		{name: "an archer: Entangle", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: func(_ *testing.T, b *Brain) { asArcher(b) }, want: skillEntangle},
		{name: "breaking off from a foe at 100: Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: both(warriorAt(100), breakingOff), want: skillArrest},
		{
			name: "breaking off from a foe at 450, beyond Arrest's 150: Entangle, no Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior,
			setup: both(warriorAt(450), breakingOff), want: skillEntangle, not: []int32{skillArrest},
		},
		{name: "breaking off, Arrest cooling: Entangle", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: both(breakingOff, cooling(skillArrest)), want: skillEntangle},
		{name: "breaking off at 54: Entangle", level: 54, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: breakingOff, want: skillEntangle},
	})
}

// TestPvPRowBD checks the bladedancer's PvP row: Arrest on a Cleric from
// 55, not at 54 (a control step, it goes before the DOT: docs/pvp/layer.md
// "PvP order"); Poison on a Cleric, not on a Warrior; Power Break, then
// Freezing Strike, on a Warrior; Arrest within its 150 reach, then
// Freezing Strike, to break off (Drain Health, the sustain, cooling); Deflect Arrow against an
// archer.
func TestPvPRowBD(t *testing.T) {
	all := []int32{skillPoison, skillPowerBreak, skillFreezingStrike, skillArrest, skillDrainHealth, skillSting, skillAggression, skillDeflectArrow}
	playRow(t, classBladedancer, []rowCase{
		{name: "a Cleric, opened, at 55: Arrest (control before the DOT)", level: 55, weapon: "DUAL", skills: all, opp: lyrCleric, setup: opened, want: skillArrest},
		{name: "a Cleric, opened, at 54: Poison", level: 54, weapon: "DUAL", skills: all, opp: lyrCleric, setup: opened, want: skillPoison},
		{name: "a Cleric, opened, Arrest cooling: Poison", level: 55, weapon: "DUAL", skills: all, opp: lyrCleric, setup: both(opened, cooling(skillArrest)), want: skillPoison},
		{name: "a Warrior, opened: Power Break, no Poison", level: 55, weapon: "DUAL", skills: all, opp: lyrWarrior, setup: opened, want: skillPowerBreak},
		{name: "a Warrior, Power Break cooling: Freezing Strike", level: 55, weapon: "DUAL", skills: all, opp: lyrWarrior, setup: both(opened, cooling(skillPowerBreak)), want: skillFreezingStrike},
		{
			name: "a Warrior, Power Break and Freezing Strike cooling: no Poison, no Arrest, no Aggression", level: 55, weapon: "DUAL", skills: all, opp: lyrWarrior,
			setup: both(opened, cooling(skillPowerBreak, skillFreezingStrike)), want: skillSting,
		},
		{
			name: "breaking off from a foe at 100: Arrest", level: 55, weapon: "DUAL", skills: all, opp: lyrWarrior,
			setup: both(opened, warriorAt(100), breakingOff, cooling(skillDrainHealth)), want: skillArrest,
		},
		{
			name: "breaking off from a foe at 450, beyond Arrest's 150: Freezing Strike, no Arrest", level: 55, weapon: "DUAL", skills: all, opp: lyrWarrior,
			setup: both(opened, warriorAt(450), breakingOff, cooling(skillDrainHealth)), want: skillFreezingStrike, not: []int32{skillArrest},
		},
		{
			name: "breaking off, Arrest cooling: Freezing Strike", level: 55, weapon: "DUAL", skills: all, opp: lyrWarrior,
			setup: both(opened, breakingOff, cooling(skillDrainHealth, skillArrest)), want: skillFreezingStrike,
		},
		{name: "an archer: Deflect Arrow", level: 55, weapon: "DUAL", skills: all, opp: lyrWarrior, setup: func(_ *testing.T, b *Brain) { asArcher(b) }, want: skillDeflectArrow},
	})
}

// TestPvPRowBDBladeDance checks the bladedancer's blade dances against a
// player under the area rule (docs/pvp/guards.md "Area hits and
// bystanders"): at 55 with duals and two attackers Poison Blade Dance goes;
// with a flagged bystander in its aura's reach neither blade dance goes,
// Sting does.
func TestPvPRowBDBladeDance(t *testing.T) {
	dances := []int32{skillPoisonBladeDance, skillDemonicBladeDance, skillSting}
	for _, tc := range []struct {
		name      string
		bystander bool
		want      int32
	}{
		{"no bystander: Poison Blade Dance", false, skillPoisonBladeDance},
		{"a flagged bystander in reach: Sting", true, skillSting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, classBladedancer, 55, "DUAL", lyrWarrior, dances...)
			foeOf(t, b, lyrWarrior, Point{X: 40}, 0)
			b.opened = pcID
			b.world.Apply(NPCInfo{ObjectID: rowNPC2, NPCID: 20001, Attackable: true, Pos: Point{X: -30}}, t0)
			for _, id := range []int32{pcID, rowNPC2} {
				b.world.Apply(Attack{AttackerID: id, Pos: b.world.Objects[id].Position(t0), Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
			}
			if tc.bystander {
				player(t, b, pvpThird, "x", Point{Y: 100}, 1, 0, 9)
			}
			now := at(time.Second)
			for _, s := range []int32{skillPoisonBladeDance, skillDemonicBladeDance} {
				if held := b.areaHeld(pcID, s, now); held != tc.bystander {
					t.Errorf("skill %d held by the area rule %v, want %v", s, held, tc.bystander)
				}
			}
			_, a := b.decide(now)
			if a.kind != actCast || a.skill != tc.want {
				t.Fatalf("action %v skill %d, want the cast of %d", a.kind, a.skill, tc.want)
			}
		})
	}
}
