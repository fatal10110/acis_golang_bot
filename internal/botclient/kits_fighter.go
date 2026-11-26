package botclient

// Skill ids of the fighter base rows (docs/strategy.md, "Behavior per
// class", rows HF EF DF OF WF; data/xml/skills).
const (
	skillPowerStrike   int32 = 3
	skillMortalBlow    int32 = 16
	skillIronPunch     int32 = 29
	skillPowerShot     int32 = 56
	skillElementalHeal int32 = 58
	skillDrainHealth   int32 = 70
	skillAttackAura    int32 = 77
	skillDefenseAura   int32 = 91
	skillRelax         int32 = 226
)

// meleeWeapons is Power Strike's weaponsAllowed (data/xml/skills/0000-0099.xml).
var meleeWeapons = []string{"SWORD", "BLUNT", "BIGSWORD", "BIGBLUNT"}

// fighterAttacks are the human, elven and dark elven fighters' attacks from
// 5: Power Shot with a bow above 20 % MP, Mortal Blow with a dagger, Power
// Strike with a one- or two-handed sword or blunt (rows HF EF DF, column 4).
var fighterAttacks = []Step{
	{Skill: skillPowerShot, From: 5, Weapons: []string{"BOW"}, MPAbove: 20},
	{Skill: skillMortalBlow, From: 5, Weapons: []string{"DAGGER"}},
	{Skill: skillPowerStrike, From: 5, Weapons: meleeWeapons},
}

// darkFighterSteps are the dark elven fighter's Drain Health under 60 % HP
// and as the opener from 15, then fighterAttacks (row DF, column 4);
// orcFighterAttacks the orc fighter's Iron Punch with fist weapons and
// Power Strike with a sword or blunt (row OF).
var (
	darkFighterSteps = append([]Step{
		{Skill: skillDrainHealth, From: 15, SelfHPBelow: 60},
		{Skill: skillDrainHealth, From: 15, Opener: true},
	}, fighterAttacks...)
	orcFighterAttacks = []Step{
		{Skill: skillIronPunch, From: 5, Weapons: []string{"DUALFIST"}},
		{Skill: skillPowerStrike, From: 5, Weapons: meleeWeapons},
	}
)

// fighterAuras are the elven and dark elven fighters' out-of-combat self
// buffs (rows EF DF, column 6): Defense Aura from 5, Attack Aura from 10.
var fighterAuras = []Buff{{Skill: skillDefenseAura}, {Skill: skillAttackAura}}

// The fighter base rows (docs/strategy.md, "Behavior per class", columns
// 3-8): every one melees whenever no step passes. Skills lists what each
// learns at a trainer, its attacks for the weapon it may wield first, then
// its sustain, buffs and Relax.
var (
	// humanFighterKit (HF): Power Strike, Mortal Blow or Power Shot by
	// weapon; rests on HP with Relax.
	humanFighterKit = ClassKit{
		Skills:   []int32{skillPowerStrike, skillMortalBlow, skillPowerShot, skillRelax},
		Rotation: fighterAttacks,
		PvP:      fighterAttacks,
		Relax:    skillRelax,
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30,
			SitHP: 35, SitMP: 15, StandHP: 90, StandMP: 70,
		},
		SpotMargin: marginNormal,
	}
	// elvenFighterKit (EF): HF's attacks, Elemental Heal on itself under
	// 50 % HP from 15, the auras out of combat.
	elvenFighterKit = ClassKit{
		Skills: []int32{skillPowerStrike, skillMortalBlow, skillPowerShot, skillElementalHeal, skillDefenseAura, skillAttackAura},
		Rotation: append([]Step{
			{Skill: skillElementalHeal, From: 15, Target: TargetSelf, SelfHPBelow: 50},
		}, fighterAttacks...),
		PvP:   fighterAttacks,
		Buffs: fighterAuras,
		Thresholds: Thresholds{
			CriticalHP: 25, CrowdedHP: 80, OutmatchedHP: 50, MPReserve: 20,
			SitHP: 45, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// darkFighterKit (DF): Drain Health under 60 % HP and as the opener from
	// 15, then HF's attacks; the auras out of combat.
	darkFighterKit = ClassKit{
		Skills:   []int32{skillDrainHealth, skillPowerStrike, skillMortalBlow, skillPowerShot, skillDefenseAura, skillAttackAura},
		Rotation: darkFighterSteps,
		PvP:      darkFighterSteps,
		Buffs:    fighterAuras,
		Thresholds: Thresholds{
			CriticalHP: 30, CrowdedHP: 85, OutmatchedHP: 60, MPReserve: 20,
			SitHP: 50, SitMP: 20, StandHP: 95, StandMP: 80,
		},
		SpotMargin: marginGlassCannon,
	}
	// orcFighterKit (OF): Iron Punch with fist weapons, Power Strike with a
	// sword or blunt; rests on HP with Relax and fights on longest.
	orcFighterKit = ClassKit{
		Skills:   []int32{skillIronPunch, skillPowerStrike, skillRelax},
		Rotation: orcFighterAttacks,
		PvP:      orcFighterAttacks,
		Relax:    skillRelax,
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 50, OutmatchedHP: 25,
			SitHP: 30, SitMP: 15, StandHP: 85, StandMP: 70,
		},
		SpotMargin: marginTank,
	}
	// dwarvenFighterKit (WF): Spoil opens on a fresh target from 10, then
	// melee; Sweeper on the spoiled corpse (ruleSweep).
	dwarvenFighterKit = ClassKit{
		Skills:   []int32{skillSpoil, skillSweeper},
		Rotation: []Step{{Skill: skillSpoil, From: 10, Spoil: true}},
		Sweep:    skillSweeper,
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
)
