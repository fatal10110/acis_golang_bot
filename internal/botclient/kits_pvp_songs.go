package botclient

// skillArrest is 402 Arrest (TK, SK, SS, BD from 55; skills/0400-0499.xml):
// ROOT with TargetMe, power 70, hit 2000, reuse 8 s, root 8 s, cast range
// 150. M10 left it out, as it stops a kill against a monster; against a
// player it is the second tree's control (docs/pvp/rows-second.md O1, D50).
const skillArrest int32 = 402

// The song second-profession PvP rows (docs/pvp/rows-second.md, rows SS BD;
// X22): each a whole list set as its kit's PvP, in place of the first
// profession's (elvenKnightPvP, palusKnightPvP stay as they are). The
// steps replace the M10 steps of every skill they name (pvpSteps); the
// other M10 attacks play after them, in M10 order. Both break off at the
// normal bound (30) and interrupt with damage only.
var (
	// swordSingerPvP (SS, as TK without Tribunal): Ultimate Defense under
	// 20 % HP only vs M, Deflect Arrow vs A (Sighted), as KN; Entangle vs
	// C, A; Arrest from 55 vs C, H, A; Arrest from 55, then Entangle, to
	// break off; Aggression only as a Peel. Then M10: Power Strike (fixed),
	// Elemental Heal under 50 %, the songs out of combat.
	swordSingerPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillEntangle, From: 36, Vs: vsCA},
		{Skill: skillArrest, From: 55, Vs: vsCHA},
		{Skill: skillArrest, From: 55, Break: true},
		{Skill: skillEntangle, From: 36, Break: true},
		{Skill: skillAggression, From: 24, Peel: true},
	}
	// bladedancerPvP (BD): Ultimate Defense and Deflect Arrow as SS; Poison
	// vs C, H; Power Break vs M, A; Freezing Strike only vs M; Arrest from
	// 55 vs C, H, A; Arrest from 55, then Freezing Strike, to break off;
	// Aggression only as a Peel. Then M10: Drain Health as the opener and
	// under 60 %, the blade dances with duals and two attackers from 55
	// (under the area rule), Sting.
	bladedancerPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillPoison, Vs: vsCH},
		{Skill: skillPowerBreak, From: 32, Vs: vsMA},
		{Skill: skillFreezingStrike, From: 36, Vs: vsM},
		{Skill: skillArrest, From: 55, Vs: vsCHA},
		{Skill: skillArrest, From: 55, Break: true},
		{Skill: skillFreezingStrike, From: 36, Break: true},
		{Skill: skillAggression, From: 24, Peel: true},
	}
)
