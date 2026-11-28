package botclient

// The dagger second-profession PvP rows (docs/pvp/rows-second.md, rows TH
// PW AW; X24): PW and AW each a whole list set as its kit's PvP, in place
// of the first profession's (elvenScoutPvP, assassinPvP stay as they are);
// TH keeps roguePvP (O3). The steps replace the M10 steps of every skill
// they name (pvpSteps); the other M10 attacks play after them, in M10
// order, 321 Blinding Blow as a damage blow (this server's Bluff only
// turns the target). Against a player the M10 51 Lure opener does not
// play (stepState, monsterOnly). All three interrupt with damage only.
var (
	// plainsWalkerPvP (PW): Ultimate Evasion under 30 % only vs M, A;
	// Entangle vs C, A; Entangle, then Sprint, to break off. Then M10:
	// Deadly Blow, Blinding Blow from 66, Bleed above 50 % target HP,
	// Mortal Blow (fixed), Elemental Heal under 50 %.
	plainsWalkerPvP = []Step{
		{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30, Vs: vsMA},
		{Skill: skillEntangle, From: 36, Vs: vsCA},
		{Skill: skillEntangle, From: 36, Break: true},
		{Skill: skillSprint, From: 32, Target: TargetSelf, Break: true},
	}
	// abyssWalkerPvP (AW): Ultimate Evasion as PW; Poison vs C, H; Power
	// Break vs M; Freezing Strike (slow, hit 3000) only to break off. Then
	// M10: Drain Health as the opener and under 60 %, Deadly Blow,
	// Blinding Blow from 66, Bleed above 50 % target HP, Sting, Mortal
	// Blow (fixed).
	abyssWalkerPvP = []Step{
		{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30, Vs: vsMA},
		{Skill: skillPoison, Vs: vsCH},
		{Skill: skillPowerBreak, From: 32, Vs: vsM},
		{Skill: skillFreezingStrike, From: 36, Break: true},
	}
)
