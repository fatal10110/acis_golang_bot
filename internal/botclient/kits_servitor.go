package botclient

// Class ids of the servitor lines (docs/servitors.md section 5;
// data/xml/classes): the Dark Avenger, the Necromancer, the Warlock, the
// Elemental Summoner and the Phantom Summoner. The branch tables pick them
// only while the persona switch ServitorLines is on (servitorBranches).
const (
	classDarkAvenger       int32 = 6
	classNecromancer       int32 = 13
	classWarlock           int32 = 14
	classElementalSummoner int32 = 28
	classPhantomSummoner   int32 = 41
)

// Skill ids of the servitor rows DA WK NC EL PS (docs/servitors.md section
// 5; data/xml/skills): the Dark Avenger's Life Scavenge and Reflect Damage;
// the summons, newest first per line, each line's first-profession Crystal
// D summon (Kat the Cat, Boxer the Unicorn, Shadow) its fallback (D51
// answer 1); the Necromancer's corpse summons and corpse skills, which the
// corpse rule casts (#762); Servitor Heal, the servitor buffs and the
// summoners' Transfer Pain.
const (
	skillLifeScavenge           int32 = 46
	skillReflectDamage          int32 = 86
	skillSummonDarkPanther      int32 = 283
	skillSummonKatTheCat        int32 = 1111
	skillServitorHeal           int32 = 1127
	skillSummonShadow           int32 = 1128
	skillSummonReanimatedMan    int32 = 1129
	skillServitorMagicShield    int32 = 1139
	skillServitorPhysicalShield int32 = 1140
	skillServitorHaste          int32 = 1141
	skillFastServitor           int32 = 1144
	skillBrightServitor         int32 = 1145
	skillMightyServitor         int32 = 1146
	skillCorpseLifeDrain        int32 = 1151
	skillCorpseBurst            int32 = 1155
	skillSummonBoxerTheUnicorn  int32 = 1226
	skillTransferPain           int32 = 1262
	skillSummonKaiTheCat        int32 = 1276
	skillSummonMerrowTheUnicorn int32 = 1277
	skillSummonSoulless         int32 = 1278
	skillSummonFelineQueen      int32 = 1331
	skillSummonUnicornSeraphim  int32 = 1332
	skillSummonNightshade       int32 = 1333
	skillSummonCursedMan        int32 = 1334
)

// transferPain is the summoners' Transfer Pain: a combat toggle while the
// servitor is out and MP is above 30 % (rows WK EL PS, column 6).
var transferPain = Toggle{Skill: skillTransferPain, MPAbove: 30, Servitor: true}

// servitorBuffs are the buffs every mystic summoner keeps on its servitor
// (Servitor Physical Shield, Servitor Magic Shield and Servitor Haste from
// 44, as the trees grant them) followed by its line's own: Fast, Bright or
// Mighty Servitor from 48. Never 1299 Servitor Empowerment, which holds the
// servitor in place (docs/servitors.md section 3).
func servitorBuffs(own int32) []int32 {
	return []int32{skillServitorPhysicalShield, skillServitorMagicShield, skillServitorHaste, own}
}

// The summoner thresholds (rows WK EL, "Numbers and summary"): their first
// profession's but 20 % MP kept for Servitor Heal and a sit at 30 % MP.
var summonerThresholds = Thresholds{
	CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 20,
	SitHP: 50, SitMP: 30, StandHP: 90, StandMP: 90,
}

// The servitor rows (docs/servitors.md section 5, D51). A summoner plays
// glass cannon on its fixed first-profession nukes without a servitor and
// normal with one out or ready to summon (ServitorSpotMargin); the summon,
// servitor heal, buff and order rules read Summons, ServitorHeal,
// ServitorBuffs and ServitorLead (servitor_upkeep.go, servitor_orders.go).
// The corpse skills (Life Scavenge, Corpse Life Drain, Corpse Burst and the
// Necromancer's corpse summons) are learned here and cast by the corpse
// rule (#762): no step casts them yet. No row has a PvP row of its own
// (docs/pvp/rows-second.md).
var (
	// darkAvengerKit (DA, from KN): Divine Heal (fixed) under 40 % HP,
	// Ultimate Defense under 20 %, Drain Health under 60 %; Aggression opens
	// (from its 400 cast range), else Drain Health; Aura of Hate with two
	// attackers, Shield Stun with a shield, Judgment from 55, Power Strike
	// (fixed); Majesty, Iron Will from 43 and Reflect Damage out of combat;
	// Shield Fortress from 64; rests with Relax; summons its Dark Panther,
	// which it orders onto the target before the pull (no ServitorLead,
	// no servitor heal or buffs). Its Support role is a tank, as the
	// Paladin's (gearRole: MeleeAlways).
	darkAvengerKit = ClassKit{
		Skills: []int32{
			skillAggression, skillDrainHealth, skillAuraOfHate, skillShieldStun, skillJudgment, skillPowerStrike,
			skillDivineHeal, skillUltimateDefense, skillMajesty, skillIronWill, skillReflectDamage,
			skillShieldFortress, skillRelax, skillSummonDarkPanther, skillLifeScavenge,
		},
		Rotation: []Step{
			{Skill: skillDivineHeal, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillAggression, Opener: true},
			{Skill: skillDrainHealth, Opener: true},
			{Skill: skillAuraOfHate, Attackers: 2},
			{Skill: skillShieldStun, Weapons: []string{"SHIELD"}},
			{Skill: skillJudgment, From: 55, Weapons: smashWeapons},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		Buffs: []Buff{
			{Skill: skillMajesty, Self: true},
			{Skill: skillIronWill, From: 43, Self: true},
			{Skill: skillReflectDamage, Self: true},
		},
		Toggles: []Toggle{shieldFortress},
		Relax:   skillRelax,
		Summons: []int32{skillSummonDarkPanther},
		Base:    []int32{skillRelax},
		Fixed:   []int32{skillDivineHeal, skillPowerStrike},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30, MPReserve: 20,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginTank,
	}
	// warlockKit (WK, from HW): Vampiric Touch (fixed) under 60 % HP, Body
	// To Mind (fixed); after its servitor's lead, Blaze from 750, Ice Bolt
	// and Aura Burn within 150 (fixed); Concentration; Transfer Pain while
	// the servitor is out; summons the Feline Queen from 56, else Kai the
	// Cat, else Kat the Cat on Crystal D; heals and buffs it.
	warlockKit = ClassKit{
		Skills: []int32{
			skillSummonKaiTheCat, skillSummonFelineQueen, skillServitorHeal,
			skillServitorPhysicalShield, skillServitorMagicShield, skillServitorHaste, skillFastServitor, skillTransferPain,
			skillBlaze, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind, skillConcentration,
			skillSummonKatTheCat,
		},
		Rotation: []Step{
			{Skill: skillVampiricTouch, SelfHPBelow: 60},
			nukerBodyToMind,
			{Skill: skillBlaze},
			{Skill: skillIceBolt},
			auraBurn,
		},
		Buffs:              selfConcentration,
		Melee:              MeleeUnderReserve,
		ServitorLead:       true,
		Summons:            []int32{skillSummonFelineQueen, skillSummonKaiTheCat, skillSummonKatTheCat},
		ServitorHeal:       skillServitorHeal,
		ServitorBuffs:      servitorBuffs(skillFastServitor),
		Toggles:            []Toggle{transferPain},
		Fixed:              []int32{skillBlaze, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind},
		Thresholds:         summonerThresholds,
		SpotMargin:         marginGlassCannon,
		ServitorSpotMargin: marginNormal,
	}
	// necromancerKit (NC, from HW): Vampiric Claw under 70 % HP before
	// Vampiric Touch (fixed) under 60 %, Body To Mind; Curse: Poison opens
	// above 60 % MP, else Death Spike from 44 at 900, Vampiric Claw, Blaze,
	// Ice Bolt and Aura Burn within 150 (fixed); Concentration. A curse
	// nuker (D51 answer 4): its corpse summons need a fresh corpse in reach,
	// which the corpse rule casts (#762), so it lists no Summons the
	// summon rule would cast with none; its Servitor Heal (fixed) keeps a
	// corpse servitor up.
	necromancerKit = ClassKit{
		Skills: []int32{
			skillCursePoison, skillDeathSpike, skillVampiricClaw, skillBodyToMind, skillCorpseLifeDrain, skillCorpseBurst,
			skillSummonReanimatedMan, skillSummonCursedMan,
			skillBlaze, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillConcentration, skillServitorHeal,
		},
		Rotation: []Step{
			{Skill: skillVampiricClaw, SelfHPBelow: 70},
			{Skill: skillVampiricTouch, SelfHPBelow: 60},
			nukerBodyToMind,
			{Skill: skillCursePoison, Opener: true, MPAbove: 60},
			{Skill: skillDeathSpike, From: 44},
			{Skill: skillVampiricClaw},
			{Skill: skillBlaze},
			{Skill: skillIceBolt},
			auraBurn,
		},
		Buffs:        selfConcentration,
		Melee:        MeleeUnderReserve,
		ServitorHeal: skillServitorHeal,
		Fixed:        []int32{skillBlaze, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillServitorHeal},
		Thresholds: Thresholds{
			CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 10,
			SitHP: 55, SitMP: 20, StandHP: 95, StandMP: 90,
		},
		SpotMargin: marginGlassCannon,
	}
	// elementalSummonerKit (EL, from EW): Self Heal under 40 % HP; after
	// its servitor's lead, Wind Shackle opens above 50 % MP, else Solar
	// Spark from 750, Aqua Swirl, Ice Bolt and Aura Burn within 150
	// (fixed); Concentration; Transfer Pain while the servitor is out;
	// summons the Unicorn Seraphim from 56, else Merrow the Unicorn, else
	// Boxer the Unicorn on Crystal D; heals and buffs it.
	elementalSummonerKit = ClassKit{
		Skills: []int32{
			skillSummonMerrowTheUnicorn, skillSummonUnicornSeraphim, skillServitorHeal,
			skillServitorPhysicalShield, skillServitorMagicShield, skillServitorHaste, skillBrightServitor, skillTransferPain,
			skillWindShackle, skillSolarSpark, skillAquaSwirl, skillIceBolt, skillAuraBurn, skillSelfHeal, skillConcentration,
			skillSummonBoxerTheUnicorn,
		},
		Rotation: []Step{
			{Skill: skillSelfHeal, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillWindShackle, Opener: true, MPAbove: 50},
			{Skill: skillSolarSpark},
			{Skill: skillAquaSwirl},
			{Skill: skillIceBolt},
			auraBurn,
		},
		Buffs:              selfConcentration,
		Melee:              MeleeUnderReserve,
		ServitorLead:       true,
		Summons:            []int32{skillSummonUnicornSeraphim, skillSummonMerrowTheUnicorn, skillSummonBoxerTheUnicorn},
		ServitorHeal:       skillServitorHeal,
		ServitorBuffs:      servitorBuffs(skillBrightServitor),
		Toggles:            []Toggle{transferPain},
		Base:               []int32{skillSelfHeal},
		Fixed:              []int32{skillSolarSpark, skillAquaSwirl, skillIceBolt, skillAuraBurn},
		Thresholds:         summonerThresholds,
		SpotMargin:         marginGlassCannon,
		ServitorSpotMargin: marginNormal,
	}
	// phantomSummonerKit (PS, from DW): Vampiric Touch (fixed) under 70 %
	// HP, Body To Mind (fixed); after its servitor's lead, Wind Shackle
	// opens above 50 % MP, else Shadow Spark from 750, Twister, Ice Bolt and
	// Aura Burn within 150 (fixed); Concentration; Transfer Pain while the
	// servitor is out; summons Nightshade from 56, else Soulless, else
	// Shadow on Crystal D; heals and buffs it.
	phantomSummonerKit = ClassKit{
		Skills: []int32{
			skillSummonSoulless, skillSummonNightshade, skillServitorHeal,
			skillServitorPhysicalShield, skillServitorMagicShield, skillServitorHaste, skillMightyServitor, skillTransferPain,
			skillWindShackle, skillShadowSpark, skillTwister, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind,
			skillConcentration, skillSummonShadow,
		},
		Rotation: []Step{
			{Skill: skillVampiricTouch, SelfHPBelow: 70},
			nukerBodyToMind,
			{Skill: skillWindShackle, Opener: true, MPAbove: 50},
			{Skill: skillShadowSpark},
			{Skill: skillTwister},
			{Skill: skillIceBolt},
			auraBurn,
		},
		Buffs:         selfConcentration,
		Melee:         MeleeUnderReserve,
		ServitorLead:  true,
		Summons:       []int32{skillSummonNightshade, skillSummonSoulless, skillSummonShadow},
		ServitorHeal:  skillServitorHeal,
		ServitorBuffs: servitorBuffs(skillMightyServitor),
		Toggles:       []Toggle{transferPain},
		Fixed:         []int32{skillShadowSpark, skillTwister, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind},
		Thresholds: Thresholds{
			CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 20,
			SitHP: 55, SitMP: 30, StandHP: 95, StandMP: 90,
		},
		SpotMargin:         marginGlassCannon,
		ServitorSpotMargin: marginNormal,
	}
)
