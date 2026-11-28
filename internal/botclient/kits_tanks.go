package botclient

// Skill ids of the tank rows PL, TK and SK (docs/strategy.md "Second
// professions (levels 40+)"; data/xml/skills).
const (
	skillAuraOfHate      int32 = 18
	skillIronWill        int32 = 72
	skillHolyBlessing    int32 = 262
	skillLightningStrike int32 = 279
	skillGuardStance     int32 = 288
	skillShieldFortress  int32 = 322
	skillTribunal        int32 = 400
	skillJudgment        int32 = 401
	skillShackle         int32 = 403
)

// The tanks' combat toggles (docs/strategy.md, Design section 9): Guard
// Stance from 43 and Shield Fortress from 64, on at engage while MP is
// above 30 %.
var (
	guardStance    = Toggle{Skill: skillGuardStance, From: 43, MPAbove: 30}
	shieldFortress = Toggle{Skill: skillShieldFortress, From: 64, MPAbove: 30}
)

var (
	// paladinKit (PL, from KN): Divine Heal (fixed) under 40 % HP, Ultimate
	// Defense under 20 %, Drain Health (fixed) under 60 %; Aggression opens
	// (from its 400 cast range), Aura of Hate with two attackers, Shield
	// Stun with a shield, Tribunal from 55, Power Strike (fixed); Holy
	// Blessing on the first patient under 50 % HP (rule 3, itself among
	// them); Majesty and Iron Will from 43 out of combat; Shield Fortress
	// from 64; rests with Relax. Its Support role is a tank, not a healer
	// (gearRole: its melee policy is MeleeAlways).
	paladinKit = ClassKit{
		Skills: []int32{
			skillAggression, skillAuraOfHate, skillDrainHealth, skillShieldStun, skillTribunal, skillPowerStrike,
			skillHolyBlessing, skillDivineHeal, skillUltimateDefense, skillMajesty, skillIronWill,
			skillShieldFortress, skillRelax, skillDeflectArrow, skillShackle,
		},
		Rotation: []Step{
			{Skill: skillDivineHeal, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillAggression, Opener: true},
			{Skill: skillAuraOfHate, Attackers: 2},
			{Skill: skillShieldStun, Weapons: []string{"SHIELD"}},
			{Skill: skillTribunal, From: 55, Weapons: smashWeapons},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		PvP:     paladinPvP,
		Heals:   []Heal{{Skill: skillHolyBlessing, HP: 50}},
		Buffs:   []Buff{{Skill: skillMajesty, Self: true}, {Skill: skillIronWill, From: 43, Self: true}},
		Toggles: []Toggle{shieldFortress},
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Fixed:   []int32{skillDrainHealth, skillDivineHeal, skillPowerStrike},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30, MPReserve: 20,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginTank,
	}

	// templeKnightKit (TK, from EK): Elemental Heal under 50 % HP, Ultimate
	// Defense under 20 %; Aggression opens, else Entangle; Aura of Hate with
	// two attackers, Tribunal from 55, Power Strike (fixed); the auras and
	// Spirit Barrier; Guard Stance from 43 and Shield Fortress from 64;
	// sits plainly. No cubic (S17, #636).
	templeKnightKit = ClassKit{
		Skills: []int32{
			skillAggression, skillEntangle, skillAuraOfHate, skillTribunal, skillPowerStrike, skillElementalHeal,
			skillUltimateDefense, skillDefenseAura, skillAttackAura, skillSpiritBarrier, skillGuardStance,
			skillShieldFortress, skillDeflectArrow, skillArrest,
		},
		Rotation: []Step{
			{Skill: skillElementalHeal, Target: TargetSelf, SelfHPBelow: 50},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillAggression, Opener: true},
			{Skill: skillEntangle, Opener: true},
			{Skill: skillAuraOfHate, Attackers: 2},
			{Skill: skillTribunal, From: 55, Weapons: smashWeapons},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		PvP:     templeKnightPvP,
		Buffs:   append(append([]Buff{}, fighterAuras...), Buff{Skill: skillSpiritBarrier, Self: true}),
		Toggles: []Toggle{guardStance, shieldFortress},
		Base:    []int32{skillDefenseAura, skillAttackAura},
		Fixed:   []int32{skillPowerStrike},
		Thresholds: Thresholds{
			CriticalHP: 20, CrowdedHP: 75, OutmatchedHP: 45, MPReserve: 25,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginTank,
	}

	// shillienKnightKit (SK, from PK): Life Leech under 70 % HP, Drain
	// Health under 60 %, Ultimate Defense under 20 %; Aggression opens, else
	// Drain Health; Aura of Hate with two attackers, Life Leech, Judgment
	// from 55, Lightning Strike from 58, Sting with its weapons, Freezing
	// Strike, Power Strike (fixed); the auras; Guard Stance from 43 and
	// Shield Fortress from 64; sits plainly. No cubic (S17, #636).
	shillienKnightKit = ClassKit{
		Skills: []int32{
			skillAggression, skillDrainHealth, skillAuraOfHate, skillLifeLeech, skillJudgment, skillLightningStrike,
			skillSting, skillFreezingStrike, skillPowerStrike, skillUltimateDefense, skillDefenseAura, skillAttackAura,
			skillGuardStance, skillShieldFortress, skillDeflectArrow, skillPoison, skillPowerBreak, skillArrest,
		},
		Rotation: []Step{
			{Skill: skillLifeLeech, SelfHPBelow: 70},
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillAggression, Opener: true},
			{Skill: skillDrainHealth, Opener: true},
			{Skill: skillAuraOfHate, Attackers: 2},
			{Skill: skillLifeLeech},
			{Skill: skillJudgment, From: 55, Weapons: smashWeapons},
			{Skill: skillLightningStrike, From: 58},
			{Skill: skillSting, Weapons: stingWeapons},
			{Skill: skillFreezingStrike},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		PvP:     shillienKnightPvP,
		Buffs:   fighterAuras,
		Toggles: []Toggle{guardStance, shieldFortress},
		Base:    []int32{skillDefenseAura, skillAttackAura},
		Fixed:   []int32{skillPowerStrike},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 25,
			SitHP: 50, SitMP: 25, StandHP: 95, StandMP: 80,
		},
		SpotMargin: marginTank,
	}
)
