package botclient

// The tank second-profession PvP rows (docs/pvp/rows-second.md, rows PL,
// TK and SK; X21): each a whole list, repeating every step of the first
// profession's list it keeps (knightPvP, elvenKnightPvP, palusKnightPvP),
// with the second tree's own-target root (owner decision O1, D50): 403
// Shackle (PL) and 402 Arrest (TK, SK) from 55, against a caster, healer
// or archer (a kiting opponent stays for the walk in) and to break off.
// The M10 steps the lists do not name play after them (pvpSteps): 400
// Tribunal or 401 Judgment from 55, Power Strike, the heals and drains
// under the long-cast rule; SK's 279 Lightning Strike is a control step
// under MinLand against a player (Paralyze of power 25), so the layer
// skips it.
var (
	// paladinPvP (PL): as KN (Ultimate Defense under 20 % HP only vs M,
	// Deflect Arrow vs A and Sighted, Drain Health under 60 % and as the
	// opener vs C, A, Shield Stun held for a caster's or healer's cast and
	// at reuse otherwise), then Shackle from 55 vs C, H, A and to break
	// off; Aggression only as a Peel.
	paladinPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillDrainHealth, SelfHPBelow: 60},
		{Skill: skillDrainHealth, Opener: true, Vs: vsCA},
		{Skill: skillShieldStun, Weapons: []string{"SHIELD"}, Hold: true},
		{Skill: skillShackle, From: 55, Vs: vsCHA},
		{Skill: skillShackle, From: 55, Break: true},
		{Skill: skillAggression, From: 24, Peel: true},
	}
	// templeKnightPvP (TK): as EK (Ultimate Defense, Deflect Arrow,
	// Entangle vs C, A), then Arrest from 55 vs C, H, A; Arrest within
	// its 150 reach, then Entangle, to break off; Aggression only as a
	// Peel. Damage only for an interrupt.
	templeKnightPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillEntangle, From: 36, Vs: vsCA},
		{Skill: skillArrest, From: 55, Vs: vsCHA},
		{Skill: skillArrest, From: 55, Within: arrestReach, Break: true},
		{Skill: skillEntangle, From: 36, Break: true},
		{Skill: skillAggression, From: 24, Peel: true},
	}
	// shillienKnightPvP (SK): as PK (Ultimate Defense, Deflect Arrow,
	// Poison vs C, H, Power Break vs M, A, Freezing Strike vs M), then
	// Arrest from 55 vs C, H, A; Arrest within its 150 reach, then
	// Freezing Strike, to break off; Aggression only as a Peel. Damage
	// only for an interrupt.
	shillienKnightPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillPoison, Vs: vsCH},
		{Skill: skillPowerBreak, From: 32, Vs: vsMA},
		{Skill: skillFreezingStrike, From: 36, Vs: vsM},
		{Skill: skillArrest, From: 55, Vs: vsCHA},
		{Skill: skillArrest, From: 55, Within: arrestReach, Break: true},
		{Skill: skillFreezingStrike, From: 36, Break: true},
		{Skill: skillAggression, From: 24, Peel: true},
	}
)

// arrestReach is 402 Arrest's castRange: as a Break step it casts only
// on a foe that close, since a break-off cast never walks the bot back in
// (Entangle or Freezing Strike covers a farther foe).
const arrestReach = 150
