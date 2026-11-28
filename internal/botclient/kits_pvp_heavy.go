package botclient

import "slices"

// skillWrath is 320 Wrath (data/xml/skills/0300-0399.xml): CPDAMPERCENT,
// AREA radius 80 around its target, POLE only, reuse 120 s. The server
// takes its power (7 % at level 1 to 30 % at 10, levels 1 and 2 at 66)
// of each player's current CP (handler/skill/resources.go
// cpDamagePercentHandler), so it is worth most on full CP: the opener.
const skillWrath int32 = 320

// The heavy damage second-profession PvP rows (docs/pvp/rows-second.md,
// rows GL, WL, DS and BH; X23): each a whole list. GL and WL drop WA's
// Stun Attack step, which neither a DUAL nor a POLE passes; DS and BH hold
// 260 Hammer Crush (a stun of Stun Attack's land table) for a cast ahead of
// the (fixed) Stun Attack. Area steps go only where the area rule allows
// (areaHeld). The M10 steps the lists do not name play after them
// (pvpSteps): GL's Sonic Blaster and sonic slashes, WL's Whirlwind and Wild
// Sweep, DS's Crush of Doom, Fatal Strike and Power Smash, the sustain.
var (
	// gladiatorPvP (GL): Lionheart at engage vs C, H, B; the M10 attacks
	// in M10 order (gladiatorAttacks: Triple Slash keeps its damage step
	// after the sonic slashes); Triple Slash also as the interrupt of a
	// magic cast when its cast-break chance passes MinBreak (Int).
	gladiatorPvP = slices.Concat(
		[]Step{{Skill: skillLionheart, Target: TargetSelf, Vs: vsCHB}},
		gladiatorAttacks,
		[]Step{{Skill: skillTripleSlash, Weapons: dualOnly, Interrupt: true}},
	)
	// warlordSecondPvP (WL): Lionheart at engage vs C, H, B; Wrath from 66
	// as the opener (owner decision O2, D50); Thunder Storm (an aura stun)
	// held for a caster's or healer's cast, at reuse otherwise.
	warlordSecondPvP = []Step{
		{Skill: skillLionheart, Target: TargetSelf, Vs: vsCHB},
		{Skill: skillWrath, From: 66, Weapons: poleOnly, Opener: true},
		{Skill: skillThunderStorm, Weapons: poleOnly, Hold: true},
	}
	// destroyerPvP (DS): Rage and Lionheart at engage vs C, H, B (as OR);
	// Hammer Crush, then Stun Attack (fixed), with a blunt, held for a
	// caster's or healer's cast, at reuse otherwise.
	destroyerPvP = []Step{
		{Skill: skillRage, Target: TargetSelf, Vs: vsCHB},
		{Skill: skillLionheart, Target: TargetSelf, Vs: vsCHB},
		{Skill: skillHammerCrush, Weapons: stunWeapons, Hold: true},
		{Skill: skillStunAttack, Weapons: stunWeapons, Hold: true},
	}
	// bountyHunterPvP (BH): as DS without the engage buffs; no Spoil on a
	// player (stepState).
	bountyHunterPvP = []Step{
		{Skill: skillHammerCrush, Weapons: stunWeapons, Hold: true},
		{Skill: skillStunAttack, Weapons: stunWeapons, Hold: true},
	}
)
