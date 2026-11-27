package botclient

// Skill ids of the Sorcerer, Spellsinger, Spellhowler and Warcryer rows
// (docs/strategy.md "Second professions (levels 40+)", rows SN SG SH WC;
// data/xml/skills): the 900 nukes Prominence, Hydro Blast, Hurricane, Ice
// Dagger, Solar Flare, Death Spike (1 Cursed Bone) and Shadow Flare; Aura
// Bolt (castRange 400) and Aura Flare (150); the AREA nukes Rain of Fire,
// Aqua Splash and Tempest (castRange 500); Surrender To Fire, a 15 s
// armor_fire debuff on an 8 s reuse; the ONE-target self buffs Blazing Skin
// and Freezing Skin, the SELF buff Mana Regeneration (7-15 Spirit Ore);
// and the PARTY chants Chant of Vampire, Chant of Fury and War Chant (4-12
// Spirit Ore).
const (
	skillManaRegeneration int32 = 1047
	skillSurrenderToFire  int32 = 1083
	skillDeathSpike       int32 = 1148
	skillTempest          int32 = 1176
	skillProminence       int32 = 1230
	skillAuraFlare        int32 = 1231
	skillBlazingSkin      int32 = 1232
	skillHydroBlast       int32 = 1235
	skillIceDagger        int32 = 1237
	skillFreezingSkin     int32 = 1238
	skillHurricane        int32 = 1239
	skillChantFury        int32 = 1251
	skillSolarFlare       int32 = 1265
	skillShadowFlare      int32 = 1267
	skillAuraBolt         int32 = 1275
	skillAquaSplash       int32 = 1295
	skillRainOfFire       int32 = 1296
	skillChantVampire     int32 = 1310
	skillWarChant         int32 = 1390
)

// The second tree's short bolts' gates: each skill's castRange (Aura Bolt
// 400, Aura Flare 150), so the server never walks a caster in to cast them
// (rows SN SG, "≤400" and "≤150").
var (
	auraBolt  = Step{Skill: skillAuraBolt, Within: 400}
	auraFlare = Step{Skill: skillAuraFlare, Within: 150}
)

// nukerBodyToMind is Body To Mind with MP under 30 % and HP above 70 %
// (rows SN SH, column 5; "S≥70" as Step.SelfHPAbove, as rows HW DW).
var nukerBodyToMind = Step{Skill: skillBodyToMind, Target: TargetSelf, MPBelow: 30, SelfHPAbove: 70}

// The nukers open with the first ready 900 nuke and rank the first tree's
// nukes, which their own tree does not raise, last (fixed). Corpse Life
// Drain, Curse Fear, Sleeping Cloud, Silence, Curse Death Link, Decay,
// Freezing Shackle, Freezing Flame, the seeds and symphonies, Cancellation
// and Resist Aqua are left out (docs/strategy.md "Left out, and why"). The
// PvP row stays the first profession's until #421.
var (
	// sorcererKit (SN, from HW): Vampiric Touch under 60 % HP, Body To
	// Mind; Prominence from 900, Surrender To Fire once per target at 70 %
	// target HP or more with MP above 50 % (owner decision 4 on #624: its
	// debuff outlasts its reuse), Rain of Fire from 58 with two attackers,
	// Aura Bolt within 400, Aura Flare within 150, Blaze and Ice Bolt
	// (fixed); Concentration and Blazing Skin on itself.
	sorcererKit = ClassKit{
		Skills: []int32{
			skillProminence, skillSurrenderToFire, skillRainOfFire, skillAuraBolt, skillAuraFlare, skillBlaze, skillIceBolt,
			skillVampiricTouch, skillBodyToMind, skillConcentration, skillBlazingSkin,
			skillEnergyBolt, skillAuraBurn, skillSlow, skillCurseChaos,
		},
		Rotation: []Step{
			{Skill: skillVampiricTouch, SelfHPBelow: 60},
			nukerBodyToMind,
			{Skill: skillProminence},
			{Skill: skillSurrenderToFire, MinTargetHP: 70, MPAbove: 50, Once: true},
			{Skill: skillRainOfFire, From: 58, Attackers: 2},
			auraBolt,
			auraFlare,
			{Skill: skillBlaze},
			{Skill: skillIceBolt},
		},
		Buffs: []Buff{{Skill: skillConcentration, Self: true}, {Skill: skillBlazingSkin, Self: true}},
		Melee: MeleeUnderReserve,
		Fixed: []int32{skillBlaze, skillIceBolt, skillVampiricTouch},
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 10,
			SitHP: 50, SitMP: 20, StandHP: 90, StandMP: 90,
		},
		PvP:        humanWizardPvP,
		SpotMargin: marginGlassCannon,
	}
	// spellsingerKit (SG, from EW): Self Heal under 40 % HP; Wind Shackle
	// opens above 50 % MP, else Hydro Blast from 900, Ice Dagger from 44,
	// Solar Flare, Aqua Splash from 58 with two attackers, Aura Bolt within
	// 400, Aura Flare within 150, Solar Spark and Aqua Swirl (fixed);
	// Concentration, Mana Regeneration and Freezing Skin on itself.
	spellsingerKit = ClassKit{
		Skills: []int32{
			skillHydroBlast, skillIceDagger, skillSolarFlare, skillAquaSplash, skillAuraBolt, skillAuraFlare,
			skillWindShackle, skillSolarSpark, skillAquaSwirl, skillSelfHeal,
			skillConcentration, skillManaRegeneration, skillFreezingSkin,
			skillIceBolt, skillEnergyBolt, skillAuraBurn,
		},
		Rotation: []Step{
			{Skill: skillSelfHeal, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillWindShackle, Opener: true, MPAbove: 50},
			{Skill: skillHydroBlast},
			{Skill: skillIceDagger, From: 44},
			{Skill: skillSolarFlare},
			{Skill: skillAquaSplash, From: 58, Attackers: 2},
			auraBolt,
			auraFlare,
			{Skill: skillSolarSpark},
			{Skill: skillAquaSwirl},
		},
		Buffs: []Buff{
			{Skill: skillConcentration, Self: true},
			{Skill: skillManaRegeneration, Self: true},
			{Skill: skillFreezingSkin, Self: true},
		},
		Melee: MeleeUnderReserve,
		Base:  []int32{skillSelfHeal},
		Fixed: []int32{skillWindShackle, skillSolarSpark, skillAquaSwirl},
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 10,
			SitHP: 50, SitMP: 20, StandHP: 90, StandMP: 90,
		},
		PvP:        elvenWizardPvP,
		SpotMargin: marginGlassCannon,
	}
	// spellhowlerKit (SH, from DW): Vampiric Claw under 70 % HP before
	// Vampiric Touch under 60 %, Body To Mind; Curse: Poison opens above
	// 60 % MP, else Hurricane from 900, Death Spike from 44, Shadow Flare,
	// Tempest from 48 with two attackers, Shadow Spark, Twister and Aura
	// Burn within 150 (fixed); Concentration.
	spellhowlerKit = ClassKit{
		Skills: []int32{
			skillHurricane, skillDeathSpike, skillShadowFlare, skillTempest, skillCursePoison,
			skillShadowSpark, skillTwister, skillAuraBurn, skillVampiricClaw, skillVampiricTouch, skillBodyToMind,
			skillConcentration, skillWindShackle, skillSlow, skillIceBolt,
		},
		Rotation: []Step{
			{Skill: skillVampiricClaw, SelfHPBelow: 70},
			{Skill: skillVampiricTouch, SelfHPBelow: 60},
			nukerBodyToMind,
			{Skill: skillCursePoison, Opener: true, MPAbove: 60},
			{Skill: skillHurricane},
			{Skill: skillDeathSpike, From: 44},
			{Skill: skillShadowFlare},
			{Skill: skillTempest, From: 48, Attackers: 2},
			{Skill: skillShadowSpark},
			{Skill: skillTwister},
			auraBurn,
		},
		Buffs: selfConcentration,
		Melee: MeleeUnderReserve,
		Fixed: []int32{skillShadowSpark, skillTwister, skillAuraBurn, skillVampiricTouch},
		Thresholds: Thresholds{
			CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 10,
			SitHP: 55, SitMP: 20, StandHP: 95, StandMP: 90,
		},
		PvP:        darkWizardPvP,
		SpotMargin: marginGlassCannon,
	}
	// warcryerKit (WC, from OS): Steal Essence and Life Drain (fixed) under
	// 70 % HP; Steal Essence opens from 900, Frost Flame (fixed), Hammer
	// Crush then Stun Attack (fixed) with a blunt, else melee; Chant of Life
	// with a member under 50 %; Soul Cry on in combat above 30 % MP; the OS
	// party chants, Chant of Vampire from 44, Chant of Fury from 48 and War
	// Chant from 58, and Soul Shield. Hammer Crush and Stun Attack have no
	// Within, as OS's Stun Attack (orcShamanKit). Both roles learn the
	// armor masteries; the Support role stops attacking at 50 % MP.
	warcryerKit = ClassKit{
		Skills: []int32{
			skillChantBattle, skillChantShielding, skillChantFire, skillFlameChant, skillChantVampire, skillChantFury,
			skillWarChant, skillSoulShield, skillChantLife,
			skillLightArmorMastery, skillHeavyArmorMastery, skillRobeMastery,
			skillSoulCry, skillStealEssence, skillFrostFlame, skillHammerCrush, skillStunAttack, skillLifeDrain,
			skillAuraSink, skillSealBinding, skillDreamingSpirit, skillFear,
		},
		Rotation: []Step{
			{Skill: skillStealEssence, SelfHPBelow: 70},
			{Skill: skillLifeDrain, SelfHPBelow: 70},
			{Skill: skillStealEssence, Opener: true},
			{Skill: skillFrostFlame},
			{Skill: skillHammerCrush, Weapons: stunWeapons},
			{Skill: skillStunAttack, Weapons: stunWeapons},
		},
		Toggles: []Toggle{{Skill: skillSoulCry, MPAbove: 30}},
		Heals:   []Heal{{Skill: skillChantLife, HP: 50, Members: 1}},
		Buffs: []Buff{
			{Skill: skillChantBattle, Party: true},
			{Skill: skillChantShielding, Party: true},
			{Skill: skillChantFire, Party: true},
			{Skill: skillFlameChant, Party: true},
			{Skill: skillChantVampire, From: 44, Party: true},
			{Skill: skillChantFury, From: 48, Party: true},
			{Skill: skillWarChant, From: 58, Party: true},
			{Skill: skillSoulShield},
		},
		Base:  []int32{skillSoulCry, skillSoulShield},
		Fixed: []int32{skillFrostFlame, skillStunAttack, skillLifeDrain},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 30,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 90,
		},
		Roles:      map[Role]RolePart{RoleSupport: {Melee: MeleeAlways, Thresholds: Thresholds{MPReserve: 50}}},
		PvP:        orcShamanPvP,
		SpotMargin: marginNormal,
	}
)
