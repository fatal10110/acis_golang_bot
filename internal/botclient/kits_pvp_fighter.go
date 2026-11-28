package botclient

import "github.com/fatal10110/acis_golang_bot/internal/charmodel"

// Skill ids the fighter PvP rows add to their M10 rows (docs/pvp/rows.md;
// data/xml/skills).
const (
	skillDeflectArrow int32 = 112
	skillPoison       int32 = 129
	skillLionheart    int32 = 287
)

// The archetype sets of the rows' `vs` gates (docs/pvp/rows.md).
var (
	vsM   = []charmodel.Archetype{charmodel.Melee}
	vsA   = []charmodel.Archetype{charmodel.Archer}
	vsMA  = []charmodel.Archetype{charmodel.Melee, charmodel.Archer}
	vsCA  = []charmodel.Archetype{charmodel.Caster, charmodel.Archer}
	vsCH  = []charmodel.Archetype{charmodel.Caster, charmodel.Healer}
	vsCHA = []charmodel.Archetype{charmodel.Caster, charmodel.Healer, charmodel.Archer}
	vsCHB = []charmodel.Archetype{charmodel.Caster, charmodel.Healer, charmodel.BattleCaster}
)

// The fighter PvP rows (docs/pvp/rows.md, rows HF to SC; X7): the steps
// fight casts against a player in the PvP order (docs/pvp/layer.md), in
// place of the M10 rotation's steps of the same skills (pvpSteps); the
// M10 steps of the skills a row does not name play as they do against a
// monster. Each poison step goes only vs C, H. A shared sustain or
// opener step (Drain Health, Ultimate Defense) is repeated where the row
// names its skill for another step. The HF, EF, DF and OF rows differ from
// M10 in nothing but list their attacks so they ship; WF plays plain
// attacks (no Spoil on a player, stepState).
var (
	// knightPvP (KN): Ultimate Defense under 20 % HP only vs M (and only
	// where it will not break off: the break-off runs first), Deflect
	// Arrow vs A (put up out of combat too once an archer the bot may
	// fight is in sight: Sighted), Drain Health under 60 % and as the
	// opener vs C, A, Shield Stun held for a caster's or healer's cast
	// (Hold: also its interrupt), at reuse otherwise; Aggression only as a
	// Peel.
	knightPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillDrainHealth, SelfHPBelow: 60},
		{Skill: skillDrainHealth, Opener: true, Vs: vsCA},
		{Skill: skillShieldStun, Weapons: []string{"SHIELD"}, Hold: true},
		{Skill: skillAggression, From: 24, Peel: true},
	}
	// warlordPvP (WA): Lionheart at engage vs C, H, B from 36; Stun
	// Attack held for a cast.
	warlordPvP = []Step{
		{Skill: skillLionheart, From: 36, Target: TargetSelf, Vs: vsCHB},
		{Skill: skillStunAttack, Weapons: stunWeapons, Hold: true},
	}
	// roguePvP (RO): Ultimate Evasion under 30 % only vs M, A; with a
	// dagger Dash at engage vs C, A from beyond 400; Dash to break off;
	// with a bow Stunning Shot held for a cast.
	roguePvP = []Step{
		{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30, Vs: vsMA},
		{Skill: skillDash, Weapons: daggerBranch, Branch: true, Target: TargetSelf, Beyond: 400, Vs: vsCA},
		{Skill: skillDash, Target: TargetSelf, Break: true},
		{Skill: skillStunningShot, From: 36, Weapons: bowBranch, Branch: true, Hold: true},
	}
	// elvenKnightPvP (EK): Ultimate Defense as KN, Deflect Arrow vs A as KN,
	// Entangle vs C, A from 36 and to break off; Aggression only as a
	// Peel.
	elvenKnightPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillEntangle, From: 36, Vs: vsCA},
		{Skill: skillEntangle, From: 36, Break: true},
		{Skill: skillAggression, From: 24, Peel: true},
	}
	// elvenScoutPvP (ES): Ultimate Evasion as RO; Entangle from 36 vs M
	// with a bow (it kites), vs C, A with a dagger, and to break off,
	// Sprint from 32 to break off; with a bow Stunning Shot held for a
	// cast.
	elvenScoutPvP = []Step{
		{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30, Vs: vsMA},
		{Skill: skillEntangle, From: 36, Weapons: bowBranch, Branch: true, Vs: vsM},
		{Skill: skillEntangle, From: 36, Weapons: daggerBranch, Branch: true, Vs: vsCA},
		{Skill: skillEntangle, From: 36, Break: true},
		{Skill: skillSprint, From: 32, Target: TargetSelf, Break: true},
		{Skill: skillStunningShot, From: 36, Weapons: bowBranch, Branch: true, Hold: true},
	}
	// palusKnightPvP (PK): Ultimate Defense as KN, Deflect Arrow vs A as KN,
	// Poison vs C, H, Power Break vs M, A from 32, Freezing Strike only
	// vs M from 36; Aggression only as a Peel.
	palusKnightPvP = []Step{
		{Skill: skillUltimateDefense, Target: TargetSelf, SelfHPBelow: 20, Vs: vsM},
		{Skill: skillDeflectArrow, From: 24, Target: TargetSelf, Vs: vsA, Sighted: true},
		{Skill: skillPoison, Vs: vsCH},
		{Skill: skillPowerBreak, From: 32, Vs: vsMA},
		{Skill: skillFreezingStrike, From: 36, Vs: vsM},
		{Skill: skillAggression, From: 24, Peel: true},
	}
	// assassinPvP (AS): Ultimate Evasion as RO; with a dagger Poison vs
	// C, H and Power Break vs M from 32; with a bow Stunning Shot held
	// for a cast (as RO).
	assassinPvP = []Step{
		{Skill: skillUltimateEvasion, From: 28, Target: TargetSelf, SelfHPBelow: 30, Vs: vsMA},
		{Skill: skillPoison, Weapons: daggerBranch, Branch: true, Vs: vsCH},
		{Skill: skillPowerBreak, From: 32, Weapons: daggerBranch, Branch: true, Vs: vsM},
		{Skill: skillStunningShot, From: 36, Weapons: bowBranch, Branch: true, Hold: true},
	}
	// overlordPvP (OR): Rage from 24 and Lionheart from 36 at engage only
	// vs C, H, B (Rage costs P. Def); Stun Attack held for a cast.
	overlordPvP = []Step{
		{Skill: skillRage, From: 24, Target: TargetSelf, Vs: vsCHB},
		{Skill: skillLionheart, From: 36, Target: TargetSelf, Vs: vsCHB},
		{Skill: skillStunAttack, Weapons: stunWeapons, Hold: true},
	}
	// monkPvP (MO): Force Blaster opens with a charge from 24 only vs C,
	// A; Cripple vs C, A; Stunning Fist held for a cast; Hurricane Assault
	// with two charges from 36 before Force Blaster with one.
	monkPvP = []Step{
		{Skill: skillForceBlaster, From: 24, Weapons: fistWeapons, Charges: 1, Opener: true, Vs: vsCA},
		{Skill: skillCripple, Weapons: []string{"FIST", "DUALFIST"}, Vs: vsCA},
		{Skill: skillStunningFist, Weapons: fistWeapons, Hold: true},
		{Skill: skillHurricaneAssault, From: 36, Weapons: fistWeapons, Charges: 2},
		{Skill: skillForceBlaster, From: 24, Weapons: fistWeapons, Charges: 1},
	}
	// scavengerPvP (SC): Stun Attack held for a cast.
	scavengerPvP = []Step{
		{Skill: skillStunAttack, Weapons: stunWeapons, Hold: true},
	}
)
