package botclient

// Skill ids of the Sword Singer and Bladedancer rows (docs/strategy.md
// "Second professions (levels 40+)", rows SS BD; data/xml/skills): the
// songs and dances (target PARTY, skillRadius 1000), the blade dances (AURA,
// skillRadius 150, DUAL only) and Spirit Barrier.
const (
	skillPoisonBladeDance   int32 = 84
	skillSpiritBarrier      int32 = 123
	skillSongOfEarth        int32 = 264
	skillSongOfWarding      int32 = 267
	skillSongOfWind         int32 = 268
	skillSongOfHunter       int32 = 269
	skillDanceOfWarrior     int32 = 271
	skillDanceOfInspiration int32 = 272
	skillDanceOfFire        int32 = 274
	skillDanceOfFury        int32 = 275
	skillSongOfVitality     int32 = 304
	skillDanceOfVampire     int32 = 310
	skillDemonicBladeDance  int32 = 408
)

// dualOnly is the weaponsAllowed of every dance and both blade dances.
var dualOnly = []string{"DUAL"}

// The songs and dances are Party buffs (the S6 stack rule; each its own
// stackType): out of combat the bot casts each on itself while it or a
// member within partyBuffReach misses it, and MPReserve (30) keeps the MP
// for them, as it gates attack steps only.
var (
	// swordSingerKit (SS, from EK): Elemental Heal under 50 % HP, Ultimate
	// Defense under 20 %; Aggression and Entangle open, Power Strike (fixed);
	// the auras, Spirit Barrier and the songs. Sword Symphony is dropped
	// (owner decision 4 on #624).
	swordSingerKit = ClassKit{
		Skills: []int32{
			skillAggression, skillEntangle, skillPowerStrike, skillElementalHeal, skillUltimateDefense,
			skillDefenseAura, skillAttackAura, skillDeflectArrow, skillSpiritBarrier,
			skillSongOfWarding, skillSongOfWind, skillSongOfHunter, skillSongOfEarth, skillSongOfVitality,
		},
		Rotation: []Step{
			{Skill: skillElementalHeal, Target: TargetSelf, SelfHPBelow: 50},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillAggression, Opener: true},
			{Skill: skillEntangle, Opener: true},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		PvP: elvenKnightPvP,
		Buffs: append(append([]Buff{}, fighterAuras...),
			Buff{Skill: skillSpiritBarrier, Self: true},
			Buff{Skill: skillSongOfWarding, Party: true},
			Buff{Skill: skillSongOfWind, Party: true, From: 46},
			Buff{Skill: skillSongOfHunter, Party: true, From: 49},
			Buff{Skill: skillSongOfEarth, Party: true, From: 55},
			Buff{Skill: skillSongOfVitality, Party: true, From: 66},
		),
		Base:  []int32{skillDefenseAura, skillAttackAura},
		Fixed: []int32{skillPowerStrike},
		Thresholds: Thresholds{
			CriticalHP: 20, CrowdedHP: 75, OutmatchedHP: 45, MPReserve: 30,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// bladedancerKit (BD, from PK): Drain Health under 60 % HP, Ultimate
	// Defense under 20 %; Drain Health and Power Break open, the blade dances
	// with duals and two attackers from 55, Sting, Freezing Strike; the
	// auras and, with duals, the dances.
	bladedancerKit = ClassKit{
		Skills: []int32{
			skillAggression, skillDrainHealth, skillPowerBreak, skillSting, skillFreezingStrike, skillPowerStrike,
			skillUltimateDefense, skillDefenseAura, skillAttackAura, skillDeflectArrow, skillPoison,
			skillPoisonBladeDance, skillDemonicBladeDance,
			skillDanceOfFire, skillDanceOfInspiration, skillDanceOfWarrior, skillDanceOfFury, skillDanceOfVampire,
		},
		Rotation: []Step{
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillDrainHealth, Opener: true},
			{Skill: skillPowerBreak, Opener: true},
			{Skill: skillPoisonBladeDance, From: 55, Attackers: 2, Weapons: dualOnly},
			{Skill: skillDemonicBladeDance, From: 55, Attackers: 2, Weapons: dualOnly},
			{Skill: skillSting, Weapons: stingWeapons},
			{Skill: skillFreezingStrike},
		},
		PvP: palusKnightPvP,
		Buffs: append(append([]Buff{}, fighterAuras...),
			Buff{Skill: skillDanceOfFire, Party: true, Weapons: dualOnly},
			Buff{Skill: skillDanceOfInspiration, Party: true, Weapons: dualOnly, From: 46},
			Buff{Skill: skillDanceOfWarrior, Party: true, Weapons: dualOnly, From: 55},
			Buff{Skill: skillDanceOfFury, Party: true, Weapons: dualOnly, From: 58},
			Buff{Skill: skillDanceOfVampire, Party: true, Weapons: dualOnly, From: 74},
		),
		Base: []int32{skillDefenseAura, skillAttackAura},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 30,
			SitHP: 50, SitMP: 25, StandHP: 95, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
)
