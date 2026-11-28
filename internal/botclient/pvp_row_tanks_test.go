package botclient

import "testing"

// The tank second-profession PvP rows (docs/pvp/rows-second.md "Tests",
// X21): each test plays the row's real compiled build on the rows' skill
// fixture (rowSkills) against a player: its control against the named
// archetype, its Hold step, its Break step, and Aggression only as a Peel.

// TestPvPRowPL checks the paladin's PvP row: Shield Stun at reuse against
// a Warrior, held against a Cleric until its Heal; Shackle from 55 on a
// caster or healer (control ahead of the opener, docs/pvp/layer.md "PvP
// order"), not at 54 nor on a Warrior; Shackle to break off; never
// Aggression.
func TestPvPRowPL(t *testing.T) {
	all := []int32{skillShackle, skillShieldStun, skillDrainHealth, skillPowerStrike, skillAggression, skillUltimateDefense, skillDeflectArrow}
	brk := []int32{skillShackle, skillShieldStun, skillPowerStrike, skillAggression}
	playRow(t, classPaladin, []rowCase{
		{name: "a Warrior: Shield Stun at reuse", level: 55, weapon: "SWORD", shield: true, skills: all, opp: lyrWarrior, want: skillShieldStun},
		{
			name: "a Warrior, Shield Stun cooling: Power Strike, no Shackle, no Aggression", level: 55, weapon: "SWORD", shield: true, skills: all, opp: lyrWarrior,
			setup: cooling(skillShieldStun), want: skillPowerStrike, not: []int32{skillShackle, skillAggression},
		},
		{name: "a Cleric: Shield Stun held, Shackle", level: 55, weapon: "SWORD", shield: true, skills: all, opp: lyrCleric, want: skillShackle},
		{
			name: "the Cleric's Heal: Shield Stun", level: 55, weapon: "SWORD", shield: true, skills: all, opp: lyrCleric, heal: true,
			setup: func(t *testing.T, b *Brain) { foeOf(t, b, lyrCleric, Point{Y: 60}, 0) }, want: skillShieldStun,
		},
		{name: "a kiting Human Mystic: Shackle ahead of the opener", level: 55, weapon: "SWORD", shield: true, skills: all, opp: lyrMystic, want: skillShackle, not: []int32{skillAggression}},
		{name: "a Human Mystic, Shackle cooling: Drain Health opens", level: 55, weapon: "SWORD", shield: true, skills: all, opp: lyrMystic, setup: cooling(skillShackle), want: skillDrainHealth},
		{name: "at 54, a Human Mystic: Drain Health opens", level: 54, weapon: "SWORD", shield: true, skills: all, opp: lyrMystic, want: skillDrainHealth},
		{name: "at 54, a Human Mystic, opened: no Shackle", level: 54, weapon: "SWORD", shield: true, skills: all, opp: lyrMystic, setup: opened, want: skillPowerStrike},
		{name: "breaking off: Shackle", level: 55, weapon: "SWORD", shield: true, skills: brk, opp: lyrWarrior, setup: breakingOff, want: skillShackle},
		{name: "breaking off at 54: the flee", level: 54, weapon: "SWORD", shield: true, skills: brk, opp: lyrWarrior, setup: breakingOff, kind: actMove},
	})
}

// TestPvPRowTK checks the temple knight's PvP row: Entangle, then Arrest
// from 55, on a caster; Arrest on a healer; neither on a Warrior nor
// Arrest at 54; Arrest, then Entangle, to break off; never Aggression.
func TestPvPRowTK(t *testing.T) {
	all := []int32{skillEntangle, skillArrest, skillPowerStrike, skillAggression}
	playRow(t, classTempleKnight, []rowCase{
		{name: "a Human Mystic: Entangle", level: 55, weapon: "SWORD", skills: all, opp: lyrMystic, want: skillEntangle},
		{name: "a Human Mystic, Entangle cooling: Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrMystic, setup: cooling(skillEntangle), want: skillArrest},
		{name: "a Cleric: Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrCleric, want: skillArrest},
		{name: "at 54, a Cleric: no Arrest", level: 54, weapon: "SWORD", skills: all, opp: lyrCleric, want: skillPowerStrike},
		{
			name: "a Warrior: no Entangle, no Arrest, no Aggression", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior,
			want: skillPowerStrike, not: []int32{skillEntangle, skillArrest, skillAggression},
		},
		{name: "breaking off from a foe at 100: Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: both(warriorAt(100), breakingOff), want: skillArrest},
		{name: "breaking off, Arrest cooling: Entangle", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior, setup: both(breakingOff, cooling(skillArrest)), want: skillEntangle},
		{
			name: "breaking off from a foe at 450, beyond Arrest's 150: Entangle, no Arrest", level: 55, weapon: "SWORD", skills: all, opp: lyrWarrior,
			setup: both(warriorAt(450), breakingOff), want: skillEntangle, not: []int32{skillArrest},
		},
	})
}

// TestPvPRowSK checks the shillien knight's PvP row: Arrest from 55 on a
// healer ahead of Poison (control first), Poison once Arrest is out or
// before 55, Power Break on a Warrior; Lightning Strike (a Paralyze under MinLand) never on a player;
// Arrest within its reach, then Freezing Strike, to break off.
func TestPvPRowSK(t *testing.T) {
	all := []int32{
		skillPoison, skillPowerBreak, skillFreezingStrike, skillArrest, skillDrainHealth, skillLightningStrike,
		skillSting, skillPowerStrike, skillAggression,
	}
	brk := []int32{skillPoison, skillPowerBreak, skillFreezingStrike, skillArrest, skillSting, skillPowerStrike}
	playRow(t, classShillienKnight, []rowCase{
		{name: "a Cleric, opened: Arrest ahead of Poison", level: 55, weapon: "SWORD", skills: all, opp: lyrCleric, setup: opened, want: skillArrest},
		{name: "a Cleric, opened, Arrest cooling: Poison", level: 58, weapon: "SWORD", skills: all, opp: lyrCleric, setup: both(opened, cooling(skillArrest)), want: skillPoison},
		{name: "at 54, a Cleric, opened: Poison", level: 54, weapon: "SWORD", skills: all, opp: lyrCleric, setup: opened, want: skillPoison},
		{name: "a Warrior: Power Break, no Arrest", level: 58, weapon: "SWORD", skills: all, opp: lyrWarrior, want: skillPowerBreak},
		{name: "at 54, a Cleric, opened, Poison cooling: no Arrest", level: 54, weapon: "SWORD", skills: all, opp: lyrCleric, setup: both(opened, cooling(skillPoison)), want: skillSting},
		{
			name: "a Warrior at 58, opened, Power Break and Freezing Strike cooling: no Lightning Strike", level: 58, weapon: "SWORD", skills: all, opp: lyrWarrior,
			setup: both(opened, cooling(skillPowerBreak, skillFreezingStrike)), want: skillSting, not: []int32{skillLightningStrike, skillAggression},
		},
		{
			name: "a Human Mystic at 58, opened, Poison and Arrest cooling: no Lightning Strike", level: 58, weapon: "SWORD", skills: all, opp: lyrMystic,
			setup: both(opened, cooling(skillPoison, skillArrest)), want: skillSting, not: []int32{skillLightningStrike},
		},
		{name: "breaking off from a foe at 100: Arrest", level: 55, weapon: "SWORD", skills: brk, opp: lyrWarrior, setup: both(warriorAt(100), breakingOff), want: skillArrest},
		{name: "breaking off, Arrest cooling: Freezing Strike", level: 55, weapon: "SWORD", skills: brk, opp: lyrWarrior, setup: both(breakingOff, cooling(skillArrest)), want: skillFreezingStrike},
		{
			name: "breaking off from a foe at 450, beyond Arrest's 150: Freezing Strike, no Arrest", level: 55, weapon: "SWORD", skills: brk, opp: lyrWarrior,
			setup: both(warriorAt(450), breakingOff), want: skillFreezingStrike, not: []int32{skillArrest},
		},
	})
}

// warriorAt moves pcID, a Warrior, to y away: 100 is within 402 Arrest's 150,
// 450 beyond it but within Entangle's and Freezing Strike's reach.
func warriorAt(y int32) func(*testing.T, *Brain) {
	return func(t *testing.T, b *Brain) {
		t.Helper()
		foeOf(t, b, lyrWarrior, Point{Y: y}, 0)
	}
}
