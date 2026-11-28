package botclient

// The archer second-profession PvP rows (docs/pvp/rows-second.md, rows HK
// SR PR; X25): SR and PR each a whole list set as its kit's PvP, in place
// of the first profession's (elvenScoutPvP, assassinPvP stay as they are);
// HK keeps roguePvP (O3). The rows play a bow alone, so no step carries a
// branch gate: Stunning Shot's bracket gate is its weaponsAllowed (BOW).
// The steps replace the M10 steps of every skill they name (pvpSteps); the
// other M10 attacks play after them, in M10 order. Stunning Shot, the Hold
// step, is also the interrupt, timed as RO's.
var (
	// silverRangerPvP (SR): Ultimate Evasion under 30 % only vs M, A;
	// Entangle vs M (a slow: the bow row kites on it); Entangle, then
	// Sprint, to break off; Stunning Shot held for a cast. Then M10:
	// Double Shot, Burst Shot from 46 with two attackers, Power Shot
	// (fixed), Elemental Heal under 50 %.
	silverRangerPvP = []Step{
		{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30, Vs: vsMA},
		{Skill: skillEntangle, From: 36, Vs: vsM},
		{Skill: skillEntangle, From: 36, Break: true},
		{Skill: skillSprint, From: 32, Target: TargetSelf, Break: true},
		{Skill: skillStunningShot, From: 36, Weapons: bowBranch, Hold: true},
	}
	// phantomRangerPvP (PR): Ultimate Evasion as SR; Freezing Strike vs M
	// (a slow: the bow row kites on it, as ES's bow branch on Entangle);
	// Poison vs C, H; Freezing Strike to break off; Stunning Shot held
	// for a cast. Then M10: Drain Health under 60 %, Fatal Counter from 60
	// under 50 %, Double Shot, Power Shot (fixed).
	phantomRangerPvP = []Step{
		{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30, Vs: vsMA},
		{Skill: skillFreezingStrike, From: 36, Vs: vsM},
		{Skill: skillPoison, Vs: vsCH},
		{Skill: skillFreezingStrike, From: 36, Break: true},
		{Skill: skillStunningShot, From: 36, Weapons: bowBranch, Hold: true},
	}
)
