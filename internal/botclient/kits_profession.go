package botclient

import "slices"

// Skill ids of the fighter first-profession rows (docs/strategy.md,
// "Behavior per class", rows KN WA RO EK ES PK AS OR MO SC;
// data/xml/skills).
const (
	skillDash             int32 = 4
	skillAggression       int32 = 28
	skillDivineHeal       int32 = 45
	skillFocusedForce     int32 = 50
	skillForceBlaster     int32 = 54
	skillBearTotem        int32 = 76
	skillWarCry           int32 = 78
	skillMajesty          int32 = 82
	skillWolfTotem        int32 = 83
	skillShieldStun       int32 = 92
	skillRage             int32 = 94
	skillCripple          int32 = 95
	skillBleed            int32 = 96
	skillRapidShot        int32 = 99
	skillStunAttack       int32 = 100
	skillStunningShot     int32 = 101
	skillEntangle         int32 = 102
	skillFreezingStrike   int32 = 105
	skillUltimateDefense  int32 = 110
	skillUltimateEvasion  int32 = 111
	skillPowerBreak       int32 = 115
	skillStunningFist     int32 = 120
	skillBattleRoar       int32 = 121
	skillGuts             int32 = 139
	skillFrenzy           int32 = 176
	skillSting            int32 = 223
	skillSprint           int32 = 230
	skillWildSweep        int32 = 245
	skillSpoil            int32 = 254
	skillSweeper          int32 = 42
	skillPowerSmash       int32 = 255
	skillAccuracy         int32 = 256
	skillHurricaneAssault int32 = 284
	skillViciousStance    int32 = 312
)

// Weapon gates of the profession rows: each skill's weaponsAllowed
// (data/xml/skills), or the branch weapon of a "Dagger:" or "Bow:" prefix.
var (
	daggerBranch = []string{"DAGGER"}
	bowBranch    = []string{"BOW"}
	stunWeapons  = []string{"BLUNT", "BIGBLUNT"}
	smashWeapons = []string{"SWORD", "BLUNT", "BIGBLUNT", "BIGSWORD"}
	stingWeapons = []string{"SWORD", "DAGGER", "DUAL", "BIGSWORD"}
	fistWeapons  = []string{"DUALFIST"}
)

// stanceToggles are Accuracy from 24 and Vicious Stance from 20, combat
// toggles on while MP is above 30 % (rows WA RO ES AS OR, column 6).
var stanceToggles = []Toggle{
	{Skill: skillAccuracy, From: 24, MPAbove: 30},
	{Skill: skillViciousStance, MPAbove: 30},
}

// daggerSteps are the dagger branch's Bleed above 50 % target HP from 24,
// then Mortal Blow (rows RO ES, column 4); bowSteps the bow branch's
// Stunning Shot from 36, then Power Shot (rows RO ES AS).
var (
	daggerSteps = []Step{
		{Skill: skillBleed, From: 24, Weapons: daggerBranch, Branch: true, MinTargetHP: 50},
		{Skill: skillMortalBlow, Weapons: daggerBranch, Branch: true},
	}
	bowSteps = []Step{
		{Skill: skillStunningShot, From: 36, Weapons: bowBranch, Branch: true},
		{Skill: skillPowerShot, Weapons: bowBranch, Branch: true},
	}
)

// rapidShot is Rapid Shot from 32 with a bow, a self step in combat (rows
// RO ES AS, column 6).
var rapidShot = Step{Skill: skillRapidShot, From: 32, Weapons: bowBranch, Branch: true, Target: TargetSelf}

// professionKits are the kits of the first professions with a row of their
// own, by class id (docs/strategy.md, "Behavior per class", columns 3-8):
// CompileBuild plays them in place of the line's base row (lineKits). Each
// serves every branch of its class: the weapon gates pick the steps for the
// weapon the branch wears. Base lists the skills a row tags "(base)": the
// profession knows them from its base class (TestBehaviorRowsShipped).
var professionKits = map[int32]ClassKit{
	// KN: Aggression opens from 24 (Drain Health before it, and while it
	// cools), Shield Stun only with a shield, Power Strike; Drain Health
	// under 60 % HP, Divine Heal under 40 % from 28, Ultimate Defense under
	// 20 %; Majesty out of combat; rests with Relax.
	4: {
		Skills: []int32{skillAggression, skillDrainHealth, skillShieldStun, skillPowerStrike, skillDivineHeal, skillUltimateDefense, skillMajesty, skillRelax, skillDeflectArrow},
		Rotation: []Step{
			{Skill: skillDivineHeal, From: 28, Target: TargetSelf, SelfHPBelow: 40},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillAggression, From: 24, Opener: true},
			{Skill: skillDrainHealth, Opener: true},
			{Skill: skillShieldStun, Weapons: []string{"SHIELD"}},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		PvP:   knightPvP,
		Buffs: []Buff{{Skill: skillMajesty}},
		Relax: skillRelax,
		Base:  []int32{skillRelax},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30, MPReserve: 20,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginTank,
	},
	// WA: War Cry in combat, Battle Roar under 30 % HP from 28; Stun Attack
	// with a blunt, Power Smash, Wild Sweep with a pole and two attackers;
	// the stance toggles; rests with Relax.
	1: {
		Skills: []int32{skillStunAttack, skillPowerSmash, skillWildSweep, skillBattleRoar, skillWarCry, skillAccuracy, skillViciousStance, skillRelax, skillLionheart},
		Rotation: []Step{
			{Skill: skillBattleRoar, From: 28, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillWarCry, Target: TargetSelf},
			{Skill: skillStunAttack, Weapons: stunWeapons},
			{Skill: skillPowerSmash, Weapons: smashWeapons},
			{Skill: skillWildSweep, Weapons: []string{"POLE"}, Attackers: 2},
		},
		PvP:     warlordPvP,
		Toggles: stanceToggles,
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30,
			SitHP: 35, SitMP: 15, StandHP: 90, StandMP: 70,
		},
		SpotMargin: marginNormal,
	},
	// RO: Ultimate Evasion under 30 % HP from 28, Rapid Shot with a bow
	// from 32; the dagger and bow steps; the stance toggles; Dash to flee;
	// rests with Relax.
	7: {
		Skills: []int32{skillBleed, skillMortalBlow, skillStunningShot, skillPowerShot, skillUltimateEvasion, skillRapidShot, skillAccuracy, skillViciousStance, skillDash, skillRelax},
		Rotation: slices.Concat([]Step{
			{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30},
			rapidShot,
		}, daggerSteps, bowSteps),
		PvP:     roguePvP,
		Toggles: stanceToggles,
		Flee:    skillDash,
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Thresholds: Thresholds{
			CriticalHP: 20, CrowdedHP: 70, OutmatchedHP: 40, MPReserve: 10,
			SitHP: 40, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	},
	// EK: Elemental Heal under 50 % HP, Ultimate Defense under 20 %;
	// Aggression opens from 24, Entangle from 36, Power Strike; the auras.
	19: {
		Skills: []int32{skillAggression, skillEntangle, skillPowerStrike, skillElementalHeal, skillUltimateDefense, skillDefenseAura, skillAttackAura, skillDeflectArrow},
		Rotation: []Step{
			{Skill: skillElementalHeal, Target: TargetSelf, SelfHPBelow: 50},
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillAggression, From: 24, Opener: true},
			{Skill: skillEntangle, From: 36, Opener: true},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		PvP:   elvenKnightPvP,
		Buffs: fighterAuras,
		Base:  []int32{skillDefenseAura, skillAttackAura},
		Thresholds: Thresholds{
			CriticalHP: 20, CrowdedHP: 75, OutmatchedHP: 45, MPReserve: 25,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	},
	// ES: Elemental Heal under 50 % HP, Ultimate Evasion under 30 % from 28,
	// Rapid Shot with a bow from 32; Entangle opens from 36 for both
	// branches, then the dagger and bow steps; the stance toggles; Sprint to
	// flee; the auras.
	22: {
		Skills: []int32{skillEntangle, skillBleed, skillMortalBlow, skillStunningShot, skillPowerShot, skillElementalHeal, skillUltimateEvasion, skillRapidShot, skillAccuracy, skillViciousStance, skillSprint, skillDefenseAura, skillAttackAura},
		Rotation: slices.Concat([]Step{
			{Skill: skillElementalHeal, Target: TargetSelf, SelfHPBelow: 50},
			{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30},
			rapidShot,
			{Skill: skillEntangle, From: 36, Opener: true},
		}, daggerSteps, bowSteps),
		PvP:     elvenScoutPvP,
		Buffs:   fighterAuras,
		Toggles: stanceToggles,
		Flee:    skillSprint,
		Base:    []int32{skillDefenseAura, skillAttackAura},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 20,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	},
	// PK: Ultimate Defense under 20 % HP, Drain Health under 60 %;
	// Aggression opens from 24 in the Support role, Drain Health opens,
	// Power Break from 32; Sting with its weapons from 24, Freezing Strike
	// from 36, Power Strike; the auras.
	32: {
		Skills: []int32{skillAggression, skillDrainHealth, skillPowerBreak, skillSting, skillFreezingStrike, skillPowerStrike, skillUltimateDefense, skillDefenseAura, skillAttackAura, skillDeflectArrow, skillPoison},
		Rotation: []Step{
			{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20},
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillAggression, From: 24, Opener: true, Role: RoleSupport},
			{Skill: skillDrainHealth, Opener: true},
			{Skill: skillPowerBreak, From: 32, Opener: true},
			{Skill: skillSting, From: 24, Weapons: stingWeapons},
			{Skill: skillFreezingStrike, From: 36},
			{Skill: skillPowerStrike, Weapons: meleeWeapons},
		},
		PvP:   palusKnightPvP,
		Buffs: fighterAuras,
		Base:  []int32{skillDefenseAura, skillAttackAura},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 25,
			SitHP: 50, SitMP: 25, StandHP: 95, StandMP: 80,
		},
		SpotMargin: marginNormal,
	},
	// AS: Ultimate Evasion under 30 % HP from 28, Rapid Shot with a bow from
	// 32, Drain Health under 60 %; with a dagger Drain Health opens, then
	// Bleed, Mortal Blow and Sting from 24; the bow steps; the stance
	// toggles; the auras.
	35: {
		Skills: []int32{skillDrainHealth, skillBleed, skillMortalBlow, skillSting, skillStunningShot, skillPowerShot, skillUltimateEvasion, skillRapidShot, skillAccuracy, skillViciousStance, skillDefenseAura, skillAttackAura, skillPoison, skillPowerBreak},
		Rotation: slices.Concat([]Step{
			{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30},
			rapidShot,
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillDrainHealth, Weapons: daggerBranch, Branch: true, Opener: true},
		}, daggerSteps, []Step{
			{Skill: skillSting, From: 24, Weapons: daggerBranch, Branch: true},
		}, bowSteps),
		PvP:     assassinPvP,
		Buffs:   fighterAuras,
		Toggles: stanceToggles,
		Base:    []int32{skillDefenseAura, skillAttackAura},
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 20,
			SitHP: 50, SitMP: 25, StandHP: 95, StandMP: 80,
		},
		SpotMargin: marginGlassCannon,
	},
	// OR: Battle Roar under 30 % HP from 28, Frenzy under 30 % from 32,
	// else Guts from 36 (one pinch stack: the stack rule keeps Guts off while
	// Frenzy is on), Rage in combat from 24; Stun Attack with a blunt, Power
	// Smash; the stance toggles; rests with Relax.
	45: {
		Skills: []int32{skillStunAttack, skillPowerSmash, skillBattleRoar, skillRage, skillFrenzy, skillGuts, skillAccuracy, skillViciousStance, skillRelax, skillLionheart},
		Rotation: []Step{
			{Skill: skillBattleRoar, From: 28, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillFrenzy, From: 32, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillGuts, From: 36, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillRage, From: 24, Target: TargetSelf},
			{Skill: skillStunAttack, Weapons: stunWeapons},
			{Skill: skillPowerSmash, Weapons: smashWeapons},
		},
		PvP:     overlordPvP,
		Toggles: stanceToggles,
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Thresholds: Thresholds{
			CriticalHP: 10, CrowdedHP: 50, OutmatchedHP: 25,
			SitHP: 30, SitMP: 15, StandHP: 85, StandMP: 70,
		},
		SpotMargin: marginTank,
	},
	// MO: Focused Force out of combat below its maxCharges from 24; Force
	// Blaster opens with a charge, Cripple opens; Hurricane Assault with two
	// charges from 36, Force Blaster with one, Stunning Fist, Iron Punch;
	// Wolf Spirit Totem out of combat, replaced by Bear Spirit Totem from
	// 28; rests with Relax. Every step but Cripple takes DUALFIST.
	47: {
		Skills: []int32{skillFocusedForce, skillForceBlaster, skillCripple, skillHurricaneAssault, skillStunningFist, skillIronPunch, skillWolfTotem, skillBearTotem, skillRelax},
		Rotation: []Step{
			{Skill: skillFocusedForce, From: 24, Weapons: fistWeapons, Target: TargetSelf, Idle: true, ChargeToMax: true},
			{Skill: skillForceBlaster, From: 24, Weapons: fistWeapons, Charges: 1, Opener: true},
			{Skill: skillCripple, Weapons: []string{"FIST", "DUALFIST"}, Opener: true},
			{Skill: skillHurricaneAssault, From: 36, Weapons: fistWeapons, Charges: 2},
			{Skill: skillForceBlaster, From: 24, Weapons: fistWeapons, Charges: 1},
			{Skill: skillStunningFist, Weapons: fistWeapons},
			{Skill: skillIronPunch, Weapons: fistWeapons},
		},
		PvP: monkPvP,
		Buffs: []Buff{
			{Skill: skillWolfTotem, Weapons: fistWeapons},
			{Skill: skillBearTotem, From: 28, Weapons: fistWeapons, Replaces: skillWolfTotem},
		},
		Relax: skillRelax,
		Base:  []int32{skillRelax},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 50, OutmatchedHP: 25, MPReserve: 10,
			SitHP: 30, SitMP: 20, StandHP: 85, StandMP: 80,
		},
		SpotMargin: marginNormal,
	},
	// SC: Spoil opens on a fresh target, then Stun Attack with a blunt;
	// Sweeper on the spoiled corpse (ruleSweep).
	54: {
		Skills: []int32{skillSpoil, skillStunAttack, skillSweeper},
		Rotation: []Step{
			{Skill: skillSpoil, Spoil: true},
			{Skill: skillStunAttack, Weapons: stunWeapons},
		},
		PvP:   scavengerPvP,
		Sweep: skillSweeper,
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30, MPReserve: 10,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginTank,
	},
	// The mystic first professions (kits_mystic.go).
	classCleric:         clericKit,
	classHumanWizard:    humanWizardKit,
	classElvenOracle:    elvenOracleKit,
	classElvenWizard:    elvenWizardKit,
	classShillienOracle: shillienOracleKit,
	classDarkWizard:     darkWizardKit,
	classOrcShaman:      orcShamanKit,
	// The singers and dancers (kits_songs.go).
	classSwordSinger: swordSingerKit,
	classBladedancer: bladedancerKit,
}
