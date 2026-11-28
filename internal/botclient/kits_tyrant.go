package botclient

// Skill ids of the tyrant row TY (docs/strategy.md "Second professions
// (levels 40+)"; data/xml/skills).
const (
	skillForceBurst  int32 = 17
	skillForceStorm  int32 = 35
	skillBurningFist int32 = 280
	skillSoulBreaker int32 = 281
	skillPumaTotem   int32 = 282
	skillBisonTotem  int32 = 292
)

// tyrantKit (TY, from MO): Focused Force out of combat below its
// maxCharges (3 at 40 up to 7 at 72); Force Blaster opens from 600 with a
// charge, Cripple opens; Force Storm from 49 and Force Burst from 43 with
// two attackers and a charge, Hurricane Assault with 2, Burning Fist, Soul
// Breaker, Force Blaster with one, Stunning Fist and Iron Punch (both
// fixed); with no attack ready, Focused Force on the bot (the filler).
// Focused Force costs 20 HP a charge (hpConsume); the plan writes no HP
// gate. Force Burst and Force Storm eat 2 Energy Stones each (castReady's
// reagent gate). Zealot from 58 under 30 % HP; Puma Spirit Totem out of
// combat, replaced by Bison Spirit Totem from 68 (one "possession" stack;
// no Wolf or Bear: Bear's x0.7 run is gone); War Frenzy from 58; rests
// with Relax. Every step but Cripple takes DUALFIST.
var tyrantKit = ClassKit{
	Skills: []int32{
		skillFocusedForce, skillForceBlaster, skillCripple, skillForceStorm, skillForceBurst,
		skillHurricaneAssault, skillBurningFist, skillSoulBreaker, skillStunningFist, skillIronPunch,
		skillZealot, skillPumaTotem, skillBisonTotem, skillWarFrenzy, skillRelax,
	},
	Rotation: []Step{
		{Skill: skillZealot, From: 58, Target: TargetSelf, SelfHPBelow: 30},
		{Skill: skillFocusedForce, Weapons: fistWeapons, Target: TargetSelf, Idle: true, ChargeToMax: true},
		{Skill: skillForceBlaster, Weapons: fistWeapons, Charges: 1, Opener: true},
		{Skill: skillCripple, Weapons: []string{"FIST", "DUALFIST"}, Opener: true},
		{Skill: skillForceStorm, From: 49, Weapons: fistWeapons, Attackers: 2, Charges: 1},
		{Skill: skillForceBurst, From: 43, Weapons: fistWeapons, Attackers: 2, Charges: 1},
		{Skill: skillHurricaneAssault, Weapons: fistWeapons, Charges: 2},
		{Skill: skillBurningFist, Weapons: fistWeapons},
		{Skill: skillSoulBreaker, Weapons: fistWeapons},
		{Skill: skillForceBlaster, Weapons: fistWeapons, Charges: 1},
		{Skill: skillStunningFist, Weapons: fistWeapons},
		{Skill: skillIronPunch, Weapons: fistWeapons},
		{Skill: skillFocusedForce, Weapons: fistWeapons, Target: TargetSelf, Filler: true, ChargeToMax: true},
	},
	PvP: monkPvP,
	Buffs: []Buff{
		{Skill: skillPumaTotem, Weapons: fistWeapons, Self: true},
		{Skill: skillBisonTotem, From: 68, Weapons: fistWeapons, Replaces: skillPumaTotem, Self: true},
	},
	Toggles: []Toggle{warFrenzy},
	Relax:   skillRelax,
	Base:    []int32{skillRelax},
	Fixed:   []int32{skillStunningFist, skillIronPunch},
	Thresholds: Thresholds{
		CriticalHP: 15, CrowdedHP: 50, OutmatchedHP: 25, MPReserve: 10,
		SitHP: 30, SitMP: 20, StandHP: 85, StandMP: 80,
	},
	SpotMargin: marginNormal,
}
