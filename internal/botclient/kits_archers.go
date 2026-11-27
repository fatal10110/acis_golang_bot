package botclient

// Skill ids of the Hawkeye, Silver Ranger and Phantom Ranger rows
// (docs/strategy.md "Second professions (levels 40+)", rows HK SR PR;
// data/xml/skills): Double Shot (BOW, castRange 900), Burst Shot (BOW,
// AREA, castRange 500, from 46), Fatal Counter (BOW, FATAL, castRange 900,
// from 60), Rapid Fire and Dead Eye (SELF, BOW, from 60), Spirit of
// Sagittarius (SELF, from 58), and the SELF buffs Hawk Eye, Soul of
// Sagittarius (from 46) and Blessing of Sagittarius (from 58).
const (
	skillDoubleShot            int32 = 19
	skillBurstShot             int32 = 24
	skillHawkEye               int32 = 131
	skillSoulOfSagittarius     int32 = 303
	skillFatalCounter          int32 = 314
	skillRapidFire             int32 = 413
	skillDeadEye               int32 = 414
	skillSpiritOfSagittarius   int32 = 415
	skillBlessingOfSagittarius int32 = 416
)

// spiritOfSagittarius is Spirit of Sagittarius from 58, a self step in
// combat (rows HK SR PR, column 6).
var spiritOfSagittarius = Step{Skill: skillSpiritOfSagittarius, From: 58, Target: TargetSelf}

// The archer rows play a bow alone: each bow step's bracket gate is its
// weaponsAllowed (BOW); Rapid Shot (no weaponsAllowed) keeps the bow
// branch gate of the first professions. Snipe, Pain of Sagittarius, Hex,
// the Quivers and Fatal Counter outside PR are left out (docs/strategy.md
// "Left out, and why"). The PvP row stays the first profession's until
// #421; its dagger steps never pass with a bow.
var (
	// hawkeyeKit (HK, from RO): Ultimate Evasion under 30 % HP; Rapid Shot
	// and Spirit of Sagittarius from 58 in combat; Stunning Shot opens,
	// Double Shot, Burst Shot from 46 with two attackers, Stunning Shot,
	// Power Shot (fixed); Hawk Eye, Soul of Sagittarius from 46 and
	// Blessing of Sagittarius from 58 out of combat; the stance toggles;
	// Dash to flee; rests with Relax.
	hawkeyeKit = ClassKit{
		Skills: []int32{
			skillStunningShot, skillDoubleShot, skillBurstShot, skillPowerShot, skillUltimateEvasion, skillRapidShot,
			skillSpiritOfSagittarius, skillHawkEye, skillSoulOfSagittarius, skillBlessingOfSagittarius,
			skillAccuracy, skillViciousStance, skillDash, skillRelax,
		},
		Rotation: []Step{
			{Skill: skillUltimateEvasion, Target: TargetSelf, SelfHPBelow: 30},
			rapidShot,
			spiritOfSagittarius,
			{Skill: skillStunningShot, Weapons: bowBranch, Opener: true},
			{Skill: skillDoubleShot, Weapons: bowBranch},
			{Skill: skillBurstShot, From: 46, Weapons: bowBranch, Attackers: 2},
			{Skill: skillStunningShot, Weapons: bowBranch},
			{Skill: skillPowerShot, Weapons: bowBranch},
		},
		PvP: roguePvP,
		Buffs: []Buff{
			{Skill: skillHawkEye, Self: true},
			{Skill: skillSoulOfSagittarius, From: 46, Self: true},
			{Skill: skillBlessingOfSagittarius, From: 58, Self: true},
		},
		Toggles: stanceToggles,
		Flee:    skillDash,
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Fixed:   []int32{skillPowerShot},
		Thresholds: Thresholds{
			CriticalHP: 20, CrowdedHP: 70, OutmatchedHP: 40, MPReserve: 10,
			SitHP: 40, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// silverRangerKit (SR, from ES): Elemental Heal under 50 % HP,
	// Ultimate Evasion under 30 %; Rapid Shot, Spirit of Sagittarius from
	// 58 and Rapid Fire from 60 in combat; Entangle opens, else Stunning
	// Shot, then as HK; the auras, Spirit Barrier, Soul of Sagittarius from
	// 46 and Blessing of Sagittarius from 58 out of combat; the stance
	// toggles; Sprint to flee; sits plainly.
	silverRangerKit = ClassKit{
		Skills: []int32{
			skillEntangle, skillStunningShot, skillDoubleShot, skillBurstShot, skillPowerShot,
			skillElementalHeal, skillUltimateEvasion, skillRapidShot, skillSpiritOfSagittarius, skillRapidFire,
			skillDefenseAura, skillAttackAura, skillSpiritBarrier, skillSoulOfSagittarius, skillBlessingOfSagittarius,
			skillAccuracy, skillViciousStance, skillSprint,
		},
		Rotation: []Step{
			{Skill: skillElementalHeal, Target: TargetSelf, SelfHPBelow: 50},
			{Skill: skillUltimateEvasion, Target: TargetSelf, SelfHPBelow: 30},
			rapidShot,
			spiritOfSagittarius,
			{Skill: skillRapidFire, From: 60, Weapons: bowBranch, Target: TargetSelf},
			{Skill: skillEntangle, Opener: true},
			{Skill: skillStunningShot, Weapons: bowBranch, Opener: true},
			{Skill: skillDoubleShot, Weapons: bowBranch},
			{Skill: skillBurstShot, From: 46, Weapons: bowBranch, Attackers: 2},
			{Skill: skillStunningShot, Weapons: bowBranch},
			{Skill: skillPowerShot, Weapons: bowBranch},
		},
		PvP: elvenScoutPvP,
		Buffs: append(append([]Buff{}, fighterAuras...),
			Buff{Skill: skillSpiritBarrier, Self: true},
			Buff{Skill: skillSoulOfSagittarius, From: 46, Self: true},
			Buff{Skill: skillBlessingOfSagittarius, From: 58, Self: true},
		),
		Toggles: stanceToggles,
		Flee:    skillSprint,
		Base:    []int32{skillDefenseAura, skillAttackAura},
		Fixed:   []int32{skillPowerShot},
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 20,
			SitHP: 45, SitMP: 25, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// phantomRangerKit (PR, from AS): Drain Health under 60 % HP first,
	// Ultimate Evasion under 30 %; Rapid Shot, Spirit of Sagittarius from
	// 58 and Dead Eye from 60 in combat; Stunning Shot opens, Fatal Counter
	// from 60 under 50 % HP, Double Shot, Stunning Shot, Power Shot
	// (fixed); the auras and Soul of Sagittarius from 46 out of combat; the
	// stance toggles; sits plainly, no flee skill.
	phantomRangerKit = ClassKit{
		Skills: []int32{
			skillStunningShot, skillFatalCounter, skillDoubleShot, skillPowerShot, skillDrainHealth,
			skillUltimateEvasion, skillRapidShot, skillSpiritOfSagittarius, skillDeadEye,
			skillDefenseAura, skillAttackAura, skillSoulOfSagittarius, skillAccuracy, skillViciousStance,
			skillPoison, skillPowerBreak,
		},
		Rotation: []Step{
			{Skill: skillDrainHealth, SelfHPBelow: 60},
			{Skill: skillUltimateEvasion, Target: TargetSelf, SelfHPBelow: 30},
			rapidShot,
			spiritOfSagittarius,
			{Skill: skillDeadEye, From: 60, Weapons: bowBranch, Target: TargetSelf},
			{Skill: skillStunningShot, Weapons: bowBranch, Opener: true},
			{Skill: skillFatalCounter, From: 60, Weapons: bowBranch, SelfHPBelow: 50},
			{Skill: skillDoubleShot, Weapons: bowBranch},
			{Skill: skillStunningShot, Weapons: bowBranch},
			{Skill: skillPowerShot, Weapons: bowBranch},
		},
		PvP:     assassinPvP,
		Buffs:   append(append([]Buff{}, fighterAuras...), Buff{Skill: skillSoulOfSagittarius, From: 46, Self: true}),
		Toggles: stanceToggles,
		Base:    []int32{skillDefenseAura, skillAttackAura},
		Fixed:   []int32{skillPowerShot},
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 20,
			SitHP: 50, SitMP: 25, StandHP: 95, StandMP: 80,
		},
		SpotMargin: marginGlassCannon,
	}
)
