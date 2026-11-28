package botclient

// Skill ids of the Treasure Hunter, Plains Walker and Abyss Walker rows
// (docs/strategy.md "Second professions (levels 40+)", rows TH PW AW;
// data/xml/skills): the blows (BLOW, DAGGER, castRange 40), Lure
// (castRange 400), Mortal Strike (a SELF buff, DAGGER) and Chameleon Rest
// (a TOGGLE that sits the bot, from 46).
const (
	skillLure          int32 = 51
	skillDeadlyBlow    int32 = 263
	skillChameleonRest int32 = 296
	skillBlindingBlow  int32 = 321
	skillCriticalBlow  int32 = 409
	skillMortalStrike  int32 = 410
)

// The dagger rows play a dagger alone: each blow's bracket gate is its
// weaponsAllowed (DAGGER), and Sand Bomb is dropped (owner decision 4 on
// #624). PW and AW play their own PvP rows (kits_pvp_daggers.go, X24); TH
// keeps the rogue's (docs/pvp/rows-second.md O3).
var (
	// treasureHunterKit (TH, from RO): Ultimate Evasion under 30 % HP;
	// Lure opens from 52 (before 52 it walks in), Critical Blow from 55,
	// Deadly Blow, Bleed above 50 % target HP, Mortal Blow (fixed); the
	// stance toggles; Dash to flee; rests with Relax.
	treasureHunterKit = ClassKit{
		Skills: []int32{
			skillLure, skillCriticalBlow, skillDeadlyBlow, skillBleed, skillMortalBlow,
			skillUltimateEvasion, skillAccuracy, skillViciousStance, skillDash, skillRelax, skillStunningShot,
		},
		Rotation: []Step{
			{Skill: skillUltimateEvasion, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillLure, From: 52, Opener: true},
			{Skill: skillCriticalBlow, From: 55, Weapons: daggerBranch},
			{Skill: skillDeadlyBlow, Weapons: daggerBranch},
			{Skill: skillBleed, Weapons: daggerBranch, MinTargetHP: 50},
			{Skill: skillMortalBlow, Weapons: daggerBranch},
		},
		PvP:     roguePvP,
		Toggles: stanceToggles,
		Flee:    skillDash,
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Fixed:   []int32{skillMortalBlow},
		Thresholds: Thresholds{
			CriticalHP: 20, CrowdedHP: 70, OutmatchedHP: 40, MPReserve: 10,
			SitHP: 40, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// plainsWalkerKit (PW, from ES): Elemental Heal under 50 % HP,
	// Ultimate Evasion under 30 %, Mortal Strike in combat from 58;
	// Entangle opens, else Lure from 52; Blinding Blow from 66, Deadly
	// Blow, Bleed above 50 % target HP, Mortal Blow (fixed); the auras and
	// Spirit Barrier; the stance toggles; Sprint to flee; rests with
	// Chameleon Rest from 46, plainly before.
	plainsWalkerKit = ClassKit{
		Skills: []int32{
			skillEntangle, skillLure, skillBlindingBlow, skillDeadlyBlow, skillBleed, skillMortalBlow,
			skillElementalHeal, skillUltimateEvasion, skillMortalStrike, skillAccuracy, skillViciousStance,
			skillSprint, skillDefenseAura, skillAttackAura, skillSpiritBarrier, skillChameleonRest, skillStunningShot,
		},
		Rotation: []Step{
			{Skill: skillElementalHeal, Target: TargetSelf, SelfHPBelow: 50},
			{Skill: skillUltimateEvasion, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillMortalStrike, From: 58, Weapons: daggerBranch, Target: TargetSelf},
			{Skill: skillEntangle, Opener: true},
			{Skill: skillLure, From: 52, Opener: true},
			{Skill: skillBlindingBlow, From: 66, Weapons: daggerBranch},
			{Skill: skillDeadlyBlow, Weapons: daggerBranch},
			{Skill: skillBleed, Weapons: daggerBranch, MinTargetHP: 50},
			{Skill: skillMortalBlow, Weapons: daggerBranch},
		},
		PvP:       plainsWalkerPvP,
		Buffs:     append(append([]Buff{}, fighterAuras...), Buff{Skill: skillSpiritBarrier, Self: true}),
		Toggles:   stanceToggles,
		Flee:      skillSprint,
		Relax:     skillChameleonRest,
		RelaxFrom: 46,
		Base:      []int32{skillDefenseAura, skillAttackAura},
		Fixed:     []int32{skillMortalBlow},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 20,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// abyssWalkerKit (AW, from AS): Drain Health under 60 % HP, Ultimate
	// Evasion under 30 %, Mortal Strike in combat from 58; Drain Health
	// opens, Blinding Blow from 66, Deadly Blow, Bleed above 50 % target
	// HP, Sting, Mortal Blow (fixed); the auras; the stance toggles; sits
	// plainly. It learns Freezing Strike for its PvP row's break-off.
	abyssWalkerKit = ClassKit{
		Skills: []int32{
			skillDrainHealth, skillBlindingBlow, skillDeadlyBlow, skillBleed, skillSting, skillMortalBlow,
			skillUltimateEvasion, skillMortalStrike, skillAccuracy, skillViciousStance, skillDefenseAura, skillAttackAura,
			skillPoison, skillPowerBreak, skillFreezingStrike, skillStunningShot,
		},
		Rotation: []Step{
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillUltimateEvasion, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillMortalStrike, From: 58, Weapons: daggerBranch, Target: TargetSelf},
			{Skill: skillDrainHealth, Opener: true},
			{Skill: skillBlindingBlow, From: 66, Weapons: daggerBranch},
			{Skill: skillDeadlyBlow, Weapons: daggerBranch},
			{Skill: skillBleed, Weapons: daggerBranch, MinTargetHP: 50},
			{Skill: skillSting, Weapons: stingWeapons},
			{Skill: skillMortalBlow, Weapons: daggerBranch},
		},
		PvP:     abyssWalkerPvP,
		Buffs:   fighterAuras,
		Toggles: stanceToggles,
		Base:    []int32{skillDefenseAura, skillAttackAura},
		Fixed:   []int32{skillMortalBlow},
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 20,
			SitHP: 50, SitMP: 25, StandHP: 95, StandMP: 80,
		},
		SpotMargin: marginGlassCannon,
	}
)
