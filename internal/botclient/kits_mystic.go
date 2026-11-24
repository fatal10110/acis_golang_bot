package botclient

import "slices"

// Skill ids of the mystic rows (docs/strategy.md, "Behavior per class", rows
// HM EM DM OM CL HW EO EW SO DW OS; data/xml/skills).
const (
	skillRobeMastery    int32 = 251
	skillSoulCry        int32 = 1001
	skillFlameChant     int32 = 1002
	skillChantFire      int32 = 1006
	skillChantShielding int32 = 1009
	skillGroupHeal      int32 = 1027
	skillHolyWeapon     int32 = 1043
	skillGreaterEmpower int32 = 1059
	skillFocus          int32 = 1077
	skillConcentration  int32 = 1078
	skillAcumen         int32 = 1085
	skillAgility        int32 = 1087
	skillFrostFlame     int32 = 1107
	skillVampiricTouch  int32 = 1147
	skillBodyToMind     int32 = 1157
	skillCursePoison    int32 = 1168
	skillAuraBurn       int32 = 1172
	skillAquaSwirl      int32 = 1175
	skillTwister        int32 = 1178
	skillIceBolt        int32 = 1184
	skillDryadRoot      int32 = 1201
	skillWindWalk       int32 = 1204
	skillWindShackle    int32 = 1206
	skillSelfHeal       int32 = 1216
	skillBlaze          int32 = 1220
	skillChantLife      int32 = 1229
	skillSolarSpark     int32 = 1264
	skillShadowSpark    int32 = 1266
	skillVampiricRage   int32 = 1268
	skillEnergyBolt     int32 = 1274
)

// Class ids of the wizard first professions (data/xml/classes).
const (
	classHumanWizard int32 = 11
	classElvenWizard int32 = 26
	classDarkWizard  int32 = 39
)

// The close-range nukes' gates: each skill's castRange (data/xml/skills,
// Energy Bolt 400, Aura Burn 150), so the server never walks a caster in
// to cast them (rows HW EW DW, "≤400" and "≤150").
var (
	energyBolt = Step{Skill: skillEnergyBolt, Within: 400}
	auraBurn   = Step{Skill: skillAuraBurn, Within: 150}
)

// healerHeals are the healers' heals in their order (rows HM CL EO SO,
// column 4): Battle Heal under 35 %, Group Heal with two members under
// 60 %, Heal under 60 %; patients leader first (Brain.patients).
var healerHeals = []Heal{
	{Skill: skillBattleHeal, HP: 35},
	{Skill: skillGroupHeal, HP: 60, Members: 2},
	{Skill: skillHeal, HP: 60},
}

// The human, elven and dark elven mystics' Support parts (rows HM EM DM):
// the healers' heals, Might and Shield on the party from 7, the Support
// role's 50 % MPReserve and 30 % Sit MP (both roles melee only under
// MPReserve), and its learn table's heals and buffs.
var (
	baseMysticHeals = func() []Heal {
		hs := make([]Heal, len(healerHeals))
		for i, h := range healerHeals {
			h.Role = RoleSupport
			hs[i] = h
		}
		return hs
	}()
	baseMysticBuffs = []Buff{
		{Skill: skillMight, From: 7, Role: RoleSupport},
		{Skill: skillShield, From: 7, Role: RoleSupport},
	}
	baseMysticSupportPart   = RolePart{Melee: MeleeUnderReserve, Thresholds: Thresholds{MPReserve: 50, SitMP: 30}}
	baseMysticSupportSkills = []int32{skillHeal, skillBattleHeal, skillGroupHeal, skillMight, skillShield}
)

// healerBuffs are Might and Shield (base) and Wind Walk, the party buffs
// every oracle and cleric keeps up (rows CL EO SO, column 6).
var healerBuffs = []Buff{{Skill: skillMight}, {Skill: skillShield}, {Skill: skillWindWalk}}

// selfConcentration is Concentration on the bot only (rows HW EW DW,
// column 6).
var selfConcentration = []Buff{{Skill: skillConcentration, Self: true}}

// healerThresholds are the cleric's and elven oracle's (rows CL EO,
// column 8).
var healerThresholds = Thresholds{
	CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 50,
	SitHP: 50, SitMP: 30, StandHP: 90, StandMP: 90,
}

// The mystic base rows (docs/strategy.md, "Behavior per class", columns 3-8):
// the Damage role nukes down to its 10 % MPReserve (30 % for the orc line),
// the Support role heals and buffs first and stops nuking at 50 %
// (ClassKit.Roles). The learn tables list the role's attacks and sustain
// first for Damage, its heals and buffs first for Support. A role without
// a part of its own (Crafter) plays the row's role-neutral steps with the
// Damage role's thresholds and melee policy.
var (
	// humanMysticKit (HM): Ice Bolt from 7, then Wind Strike; Self Heal
	// under 40 % HP, Vampiric Touch under 60 % from 14.
	humanMysticKit = ClassKit{
		Skills: []int32{skillIceBolt, skillWindStrike, skillSelfHeal, skillVampiricTouch},
		Rotation: []Step{
			{Skill: skillSelfHeal, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillVampiricTouch, From: 14, SelfHPBelow: 60},
			{Skill: skillIceBolt, From: 7},
			{Skill: skillWindStrike},
		},
		Heals: baseMysticHeals,
		Buffs: baseMysticBuffs,
		Melee: MeleeUnderReserve,
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 10,
			SitHP: 50, SitMP: 20, StandHP: 90, StandMP: 90,
		},
		Roles: map[Role]RolePart{RoleSupport: supportPart(baseMysticSupportPart,
			skillIceBolt, skillWindStrike, skillSelfHeal, skillVampiricTouch)},
		SpotMargin: marginGlassCannon,
	}
	// elvenMysticKit (EM): the Damage role opens with Wind Shackle from 14
	// above 50 % MP; Ice Bolt, Wind Strike; Self Heal under 50 % HP, no
	// drain.
	elvenMysticKit = ClassKit{
		Skills: []int32{skillWindShackle, skillIceBolt, skillWindStrike, skillSelfHeal},
		Rotation: []Step{
			{Skill: skillSelfHeal, Target: TargetSelf, SelfHPBelow: 50},
			{Skill: skillWindShackle, From: 14, Opener: true, MPAbove: 50, Role: RoleDamage},
			{Skill: skillIceBolt, From: 7},
			{Skill: skillWindStrike},
		},
		Heals: baseMysticHeals,
		Buffs: baseMysticBuffs,
		Melee: MeleeUnderReserve,
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 10,
			SitHP: 50, SitMP: 20, StandHP: 90, StandMP: 90,
		},
		Roles: map[Role]RolePart{RoleSupport: supportPart(baseMysticSupportPart,
			skillIceBolt, skillWindStrike, skillSelfHeal)},
		SpotMargin: marginGlassCannon,
	}
	// darkMysticKit (DM): the Damage role opens with Curse: Poison from 7
	// above 60 % MP; Ice Bolt, Wind Strike; Vampiric Touch under 70 % HP
	// from 14, Self Heal under 40 %.
	darkMysticKit = ClassKit{
		Skills: []int32{skillCursePoison, skillIceBolt, skillWindStrike, skillVampiricTouch, skillSelfHeal},
		Rotation: []Step{
			{Skill: skillVampiricTouch, From: 14, SelfHPBelow: 70},
			{Skill: skillSelfHeal, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillCursePoison, From: 7, Opener: true, MPAbove: 60, Role: RoleDamage},
			{Skill: skillIceBolt, From: 7},
			{Skill: skillWindStrike},
		},
		Heals: baseMysticHeals,
		Buffs: baseMysticBuffs,
		Melee: MeleeUnderReserve,
		Thresholds: Thresholds{
			CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 10,
			SitHP: 55, SitMP: 20, StandHP: 95, StandMP: 90,
		},
		Roles: map[Role]RolePart{RoleSupport: supportPart(baseMysticSupportPart,
			skillIceBolt, skillWindStrike, skillVampiricTouch, skillSelfHeal)},
		SpotMargin: marginGlassCannon,
	}
	// orcMysticKit (OM): Chill Flame opens from 7, then Life Drain, which
	// also drains under 70 % HP; Soul Cry on in combat above 30 % MP; Soul
	// Shield from 7 and Chant of Battle on the party from 14. The Support
	// role learns the armor masteries (RaceProfile.MysticArmor), stands in
	// a fight (melee always) and stops nuking at 50 % MP; the Damage role
	// stays in a robe and melees only under its 30 % MPReserve.
	orcMysticKit = ClassKit{
		Skills: []int32{skillChillFlame, skillLifeDrain, skillSoulCry, skillSoulShield, skillChantBattle, skillRobeMastery},
		Rotation: []Step{
			{Skill: skillLifeDrain, From: 7, SelfHPBelow: 70},
			{Skill: skillChillFlame, From: 7, Opener: true},
			{Skill: skillLifeDrain, From: 7},
		},
		Toggles: []Toggle{{Skill: skillSoulCry, MPAbove: 30}},
		Buffs:   []Buff{{Skill: skillSoulShield, From: 7}, {Skill: skillChantBattle, From: 14, Party: true}},
		Melee:   MeleeUnderReserve,
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 30,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 90,
		},
		Roles: map[Role]RolePart{RoleSupport: {
			Skills: []int32{
				skillSoulShield, skillChantBattle, skillLightArmorMastery, skillHeavyArmorMastery, skillRobeMastery,
				skillSoulCry, skillChillFlame, skillLifeDrain,
			},
			Melee:      MeleeAlways,
			Thresholds: Thresholds{MPReserve: 50, SitMP: 30},
		}},
		SpotMargin: marginNormal,
	}
)

// supportPart is part with the Support learn table: the base mystics'
// heals and buffs first, then the row's attacks and sustain.
func supportPart(part RolePart, attacks ...int32) RolePart {
	part.Skills = append(append([]int32{}, baseMysticSupportSkills...), attacks...)
	return part
}

// The mystic first-profession rows (docs/strategy.md, "Behavior per
// class", columns 3-8), registered in professionKits; a second profession
// plays its first profession's (#332). The healers (CL EO SO) heal the
// leader first and nuke the leader's target above their 50 % MPReserve,
// meleeing only alone and under it; the wizards (HW EW DW) nuke down to
// 10 % and melee only under it.
var (
	// clericKit (CL): the healers' heals; Wind Strike on the leader's
	// target; Might, Shield, Wind Walk, Acumen on the mystic members and
	// Concentration.
	clericKit = ClassKit{
		Skills: []int32{
			skillHeal, skillBattleHeal, skillGroupHeal, skillMight, skillShield, skillWindWalk,
			skillAcumen, skillConcentration, skillWindStrike,
		},
		Rotation: []Step{{Skill: skillWindStrike, Target: TargetLeaderTarget}},
		Heals:    healerHeals,
		Buffs: slices.Concat(healerBuffs, []Buff{
			{Skill: skillAcumen, Members: mysticClasses}, {Skill: skillConcentration},
		}),
		Melee:      MeleeAloneUnderReserve,
		Base:       []int32{skillMight, skillShield},
		Thresholds: healerThresholds,
		SpotMargin: marginGlassCannon,
	}
	// elvenOracleKit (EO): the healers' heals; Dryad Root from 25 on a
	// second attacker, Wind Strike on the leader's target; Might, Shield,
	// Wind Walk, Agility and Holy Weapon from 25.
	elvenOracleKit = ClassKit{
		Skills: []int32{
			skillHeal, skillBattleHeal, skillGroupHeal, skillMight, skillShield, skillWindWalk,
			skillAgility, skillHolyWeapon, skillDryadRoot, skillWindStrike,
		},
		Rotation: []Step{
			{Skill: skillDryadRoot, From: 25, Target: TargetOtherAttacker, Attackers: 2},
			{Skill: skillWindStrike, Target: TargetLeaderTarget},
		},
		Heals: healerHeals,
		Buffs: slices.Concat(healerBuffs, []Buff{
			{Skill: skillAgility, From: 25}, {Skill: skillHolyWeapon, From: 25},
		}),
		Melee:      MeleeAloneUnderReserve,
		Base:       []int32{skillMight, skillShield},
		Thresholds: healerThresholds,
		SpotMargin: marginGlassCannon,
	}
	// shillienOracleKit (SO): the healers' heals; Wind Shackle once on the
	// leader's target, then Wind Strike; Might, Shield, Wind Walk, Greater
	// Empower and Focus from 25, Vampiric Rage from 30.
	shillienOracleKit = ClassKit{
		Skills: []int32{
			skillHeal, skillBattleHeal, skillGroupHeal, skillMight, skillShield, skillWindWalk,
			skillGreaterEmpower, skillFocus, skillVampiricRage, skillWindShackle, skillWindStrike,
		},
		Rotation: []Step{
			{Skill: skillWindShackle, Target: TargetLeaderTarget, Opener: true},
			{Skill: skillWindStrike, Target: TargetLeaderTarget},
		},
		Heals: healerHeals,
		Buffs: slices.Concat(healerBuffs, []Buff{
			{Skill: skillGreaterEmpower, From: 25}, {Skill: skillFocus, From: 25}, {Skill: skillVampiricRage, From: 30},
		}),
		Melee: MeleeAloneUnderReserve,
		Base:  []int32{skillMight, skillShield},
		Thresholds: Thresholds{
			CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 50,
			SitHP: 55, SitMP: 30, StandHP: 95, StandMP: 90,
		},
		SpotMargin: marginGlassCannon,
	}
	// humanWizardKit (HW): Blaze from 750, Ice Bolt, Energy Bolt within
	// 400, Aura Burn within 150; Vampiric Touch under 60 % HP, Body To Mind
	// from 25 with MP under 30 % and HP above 70 %; Concentration.
	humanWizardKit = ClassKit{
		Skills: []int32{
			skillBlaze, skillIceBolt, skillEnergyBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind, skillConcentration,
		},
		Rotation: []Step{
			{Skill: skillVampiricTouch, SelfHPBelow: 60},
			{Skill: skillBodyToMind, From: 25, Target: TargetSelf, MPBelow: 30, SelfHPAbove: 70},
			{Skill: skillBlaze},
			{Skill: skillIceBolt},
			energyBolt,
			auraBurn,
		},
		Buffs: selfConcentration,
		Melee: MeleeUnderReserve,
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 10,
			SitHP: 50, SitMP: 20, StandHP: 90, StandMP: 90,
		},
		SpotMargin: marginGlassCannon,
	}
	// elvenWizardKit (EW): Wind Shackle opens above 50 % MP, else Solar
	// Spark from 25, Aqua Swirl, Ice Bolt, Energy Bolt within 400, Aura
	// Burn within 150; Self Heal under 40 % HP; Concentration.
	elvenWizardKit = ClassKit{
		Skills: []int32{
			skillWindShackle, skillSolarSpark, skillAquaSwirl, skillIceBolt, skillEnergyBolt, skillAuraBurn,
			skillSelfHeal, skillConcentration,
		},
		Rotation: []Step{
			{Skill: skillSelfHeal, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillWindShackle, Opener: true, MPAbove: 50},
			{Skill: skillSolarSpark, From: 25},
			{Skill: skillAquaSwirl},
			{Skill: skillIceBolt},
			energyBolt,
			auraBurn,
		},
		Buffs: selfConcentration,
		Melee: MeleeUnderReserve,
		Base:  []int32{skillSelfHeal},
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 10,
			SitHP: 50, SitMP: 20, StandHP: 90, StandMP: 90,
		},
		SpotMargin: marginGlassCannon,
	}
	// darkWizardKit (DW): Curse: Poison opens above 60 % MP, else Shadow
	// Spark from 25, Twister, Ice Bolt, Aura Burn within 150; Vampiric
	// Touch under 70 % HP, Body To Mind from 25; Concentration.
	darkWizardKit = ClassKit{
		Skills: []int32{
			skillCursePoison, skillShadowSpark, skillTwister, skillIceBolt, skillAuraBurn, skillVampiricTouch,
			skillBodyToMind, skillConcentration,
		},
		Rotation: []Step{
			{Skill: skillVampiricTouch, SelfHPBelow: 70},
			{Skill: skillBodyToMind, From: 25, Target: TargetSelf, MPBelow: 30, SelfHPAbove: 70},
			{Skill: skillCursePoison, Opener: true, MPAbove: 60},
			{Skill: skillShadowSpark, From: 25},
			{Skill: skillTwister},
			{Skill: skillIceBolt},
			auraBurn,
		},
		Buffs: selfConcentration,
		Melee: MeleeUnderReserve,
		Thresholds: Thresholds{
			CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 10,
			SitHP: 55, SitMP: 20, StandHP: 95, StandMP: 90,
		},
		SpotMargin: marginGlassCannon,
	}
	// orcShamanKit (OS): Frost Flame opens from 750, Life Drain (also under
	// 70 % HP), Stun Attack with a blunt, else melee; Chant of Life with a
	// member under 50 %. Stun Attack has no Within 40: the server pads its
	// cast range with both collision radii, which a centre distance cannot
	// see, and the row melees anyway; Soul Cry on in combat above 30 % MP;
	// the party chants (Flame Chant from 30) and Soul Shield. Both roles
	// learn the armor masteries; the Support role stops nuking at 50 % MP.
	orcShamanKit = ClassKit{
		Skills: []int32{
			skillChantBattle, skillChantShielding, skillChantFire, skillFlameChant, skillSoulShield, skillChantLife,
			skillLightArmorMastery, skillHeavyArmorMastery, skillRobeMastery,
			skillSoulCry, skillFrostFlame, skillLifeDrain, skillStunAttack,
		},
		Rotation: []Step{
			{Skill: skillLifeDrain, SelfHPBelow: 70},
			{Skill: skillFrostFlame, Opener: true},
			{Skill: skillLifeDrain},
			{Skill: skillStunAttack, Weapons: stunWeapons},
		},
		Toggles: []Toggle{{Skill: skillSoulCry, MPAbove: 30}},
		Heals:   []Heal{{Skill: skillChantLife, HP: 50, Members: 1}},
		Buffs: []Buff{
			{Skill: skillChantBattle, Party: true},
			{Skill: skillChantShielding, Party: true},
			{Skill: skillChantFire, Party: true},
			{Skill: skillFlameChant, From: 30, Party: true},
			{Skill: skillSoulShield},
		},
		Base: []int32{skillSoulCry, skillChantBattle, skillSoulShield},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 30,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 90,
		},
		Roles:      map[Role]RolePart{RoleSupport: {Melee: MeleeAlways, Thresholds: Thresholds{MPReserve: 50}}},
		SpotMargin: marginNormal,
	}
)
