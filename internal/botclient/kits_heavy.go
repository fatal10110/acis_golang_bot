package botclient

import "slices"

// Skill ids of the heavy damage second-profession rows GL WL DS BH
// (docs/strategy.md "Second professions (levels 40+)"; data/xml/skills).
const (
	skillTripleSlash      int32 = 1
	skillDoubleSonicSlash int32 = 5
	skillSonicBlaster     int32 = 6
	skillSonicStorm       int32 = 7
	skillSonicFocus       int32 = 8
	skillSonicBuster      int32 = 9
	skillWhirlwind        int32 = 36
	skillThunderStorm     int32 = 48
	skillFatalStrike      int32 = 190
	skillHammerCrush      int32 = 260
	skillTripleSonicSlash int32 = 261
	skillDuelistSpirit    int32 = 297
	skillCrushOfDoom      int32 = 315
	skillZealot           int32 = 420
	skillFellSwoop        int32 = 421
	skillPolearmAccuracy  int32 = 422
	skillWarFrenzy        int32 = 424
)

// Weapon gates of the heavy rows: each skill's weaponsAllowed.
var (
	sonicWeapons = []string{"SWORD", "BLUNT", "DUAL", "BIGBLUNT", "BIGSWORD"}
	poleOnly     = []string{"POLE"}
	doomWeapons  = []string{"BIGBLUNT", "BIGSWORD"}
)

// warFrenzy is War Frenzy from 58, a combat toggle on while MP is above 30 %
// (rows GL WL DS BH, column 6); heavyStances are Accuracy and Vicious
// Stance, known at 40, on the same bound.
var (
	warFrenzy    = Toggle{Skill: skillWarFrenzy, From: 58, MPAbove: 30}
	heavyStances = []Toggle{{Skill: skillAccuracy, MPAbove: 30}, {Skill: skillViciousStance, MPAbove: 30}}
)

// gladiatorAttacks are GL's attacks in M10 order (gladiatorKit's rotation),
// which its PvP row (gladiatorPvP) repeats.
var gladiatorAttacks = []Step{
	{Skill: skillSonicBlaster, From: 43, Weapons: sonicWeapons, Charges: 2, Opener: true},
	{Skill: skillTripleSonicSlash, From: 55, Weapons: dualOnly, Charges: 4},
	{Skill: skillDoubleSonicSlash, From: 49, Weapons: dualOnly, Charges: 3},
	{Skill: skillSonicBuster, From: 43, Weapons: sonicWeapons, Attackers: 2, Charges: 2},
	{Skill: skillSonicStorm, From: 49, Weapons: sonicWeapons, Attackers: 2, Charges: 2},
	{Skill: skillTripleSlash, Weapons: dualOnly},
}

var (
	// gladiatorKit (GL, from WA): Sonic Focus out of combat below its
	// maxCharges; Sonic Blaster opens from 43 with 2 charges, Triple Sonic
	// Slash from 55 with 4 and Double Sonic Slash from 49 with 3 (duals),
	// Sonic Buster from 43 and Sonic Storm from 49 with two attackers and 2
	// charges, Triple Slash (duals); with no attack ready, Sonic Focus on
	// the bot (the filler). Sonic Focus costs 25 HP a cast (hpConsume); the
	// plan writes no HP gate. Sonic Buster and Storm eat 3 Energy Stones
	// each (castReady's reagent gate). Battle Roar under 30 % HP; War Cry
	// and, with duals from 64, Duelist Spirit in combat; the stance toggles
	// and War Frenzy; rests with Relax.
	gladiatorKit = ClassKit{
		Skills: []int32{
			skillSonicFocus, skillSonicBlaster, skillTripleSonicSlash, skillDoubleSonicSlash, skillSonicBuster,
			skillSonicStorm, skillTripleSlash, skillBattleRoar, skillWarCry, skillDuelistSpirit,
			skillAccuracy, skillViciousStance, skillWarFrenzy, skillRelax, skillLionheart,
		},
		Rotation: slices.Concat([]Step{
			{Skill: skillBattleRoar, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillWarCry, Target: TargetSelf},
			// Duelist Spirit has no weaponsAllowed: its stats apply only
			// <using kind="DUAL"/>, so the DUAL gate is a branch gate.
			{Skill: skillDuelistSpirit, From: 64, Weapons: dualOnly, Branch: true, Target: TargetSelf},
			{Skill: skillSonicFocus, Weapons: sonicWeapons, Target: TargetSelf, Idle: true, ChargeToMax: true},
		}, gladiatorAttacks, []Step{
			{Skill: skillSonicFocus, Weapons: sonicWeapons, Target: TargetSelf, Filler: true, ChargeToMax: true},
		}),
		PvP:     gladiatorPvP,
		Toggles: append(append([]Toggle{}, heavyStances...), warFrenzy),
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30, MPReserve: 10,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// warlordKit (WL, from WA): with a pole Thunder Storm, Whirlwind, Wild
	// Sweep with two attackers; Battle Roar under 30 % HP, Revival under
	// 15 % from 55; War Cry and, with a pole from 58, Fell Swoop in combat;
	// the stance toggles, Polearm Accuracy with a pole and War Frenzy from
	// 58; rests with Relax. Shock Stomp and Focus Attack are dropped (owner
	// decision 4 on #624).
	warlordKit = ClassKit{
		Skills: []int32{
			skillThunderStorm, skillWhirlwind, skillWildSweep, skillBattleRoar, skillRevival, skillWarCry,
			skillFellSwoop, skillAccuracy, skillViciousStance, skillPolearmAccuracy, skillWarFrenzy,
			skillRelax, skillLionheart, skillWrath,
		},
		Rotation: []Step{
			{Skill: skillBattleRoar, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillRevival, From: 55, Target: TargetSelf, SelfHPBelow: 15},
			{Skill: skillWarCry, Target: TargetSelf},
			{Skill: skillFellSwoop, From: 58, Weapons: poleOnly, Target: TargetSelf},
			{Skill: skillThunderStorm, Weapons: poleOnly},
			{Skill: skillWhirlwind, Weapons: poleOnly},
			{Skill: skillWildSweep, Weapons: poleOnly, Attackers: 2},
		},
		PvP: warlordSecondPvP,
		Toggles: append(append([]Toggle{}, heavyStances...),
			// Polearm Accuracy has no weaponsAllowed: its bonus applies
			// only <using kind="POLE"/>.
			Toggle{Skill: skillPolearmAccuracy, From: 58, MPAbove: 30, Weapons: poleOnly},
			warFrenzy,
		),
		Relax: skillRelax,
		Base:  []int32{skillRelax},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 50, OutmatchedHP: 25, MPReserve: 10,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginNormal,
	}
	// destroyerKit (DS, from OR): Battle Roar under 30 % HP, Frenzy under
	// 30 %, else Guts (the pinch stack, as OR), Zealot from 58 under 30 %,
	// Rage in combat; Crush of Doom from 60 with a two-hander, Hammer Crush
	// with a blunt, Fatal Strike, Stun Attack with a blunt and Power Smash
	// (both fixed); the stance toggles and War Frenzy; rests with Relax.
	destroyerKit = ClassKit{
		Skills: []int32{
			skillCrushOfDoom, skillHammerCrush, skillFatalStrike, skillStunAttack, skillPowerSmash,
			skillBattleRoar, skillRage, skillFrenzy, skillGuts, skillZealot,
			skillAccuracy, skillViciousStance, skillWarFrenzy, skillRelax, skillLionheart,
		},
		Rotation: []Step{
			{Skill: skillBattleRoar, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillFrenzy, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillGuts, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillZealot, From: 58, Target: TargetSelf, SelfHPBelow: 30},
			{Skill: skillRage, Target: TargetSelf},
			{Skill: skillCrushOfDoom, From: 60, Weapons: doomWeapons},
			{Skill: skillHammerCrush, Weapons: stunWeapons},
			{Skill: skillFatalStrike, Weapons: smashWeapons},
			{Skill: skillStunAttack, Weapons: stunWeapons},
			{Skill: skillPowerSmash, Weapons: smashWeapons},
		},
		PvP:     destroyerPvP,
		Toggles: append(append([]Toggle{}, heavyStances...), warFrenzy),
		Relax:   skillRelax,
		Base:    []int32{skillRelax},
		Fixed:   []int32{skillStunAttack, skillPowerSmash},
		Thresholds: Thresholds{
			CriticalHP: 10, CrowdedHP: 50, OutmatchedHP: 25,
			SitHP: 30, SitMP: 15, StandHP: 85, StandMP: 70,
		},
		SpotMargin: marginTank,
	}
	// bountyHunterKit (BH, from SC): Spoil opens on a target nobody spoiled
	// (as SC: no Opener, so a resisted Spoil goes again at its reuse), then
	// Hammer Crush and Stun Attack (fixed) with a blunt; Sweeper on the
	// spoiled corpse (ruleSweep); War Frenzy from 58; a plain sit.
	bountyHunterKit = ClassKit{
		Skills: []int32{skillSpoil, skillHammerCrush, skillStunAttack, skillSweeper, skillWarFrenzy},
		Rotation: []Step{
			{Skill: skillSpoil, Spoil: true},
			{Skill: skillHammerCrush, Weapons: stunWeapons},
			{Skill: skillStunAttack, Weapons: stunWeapons},
		},
		PvP:     bountyHunterPvP,
		Sweep:   skillSweeper,
		Toggles: []Toggle{warFrenzy},
		Fixed:   []int32{skillStunAttack},
		Thresholds: Thresholds{
			CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30, MPReserve: 10,
			SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
		},
		SpotMargin: marginTank,
	}
)
