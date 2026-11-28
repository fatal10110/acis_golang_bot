package botclient

// tyrantPvP (TY) is the tyrant's second-profession PvP row
// (docs/pvp/rows-second.md, row TY; X26): a whole list set as the kit's
// PvP in place of the monk's (monkPvP stays for MO). Force Blaster opens
// with a charge only vs C, A; Cripple vs C, A; Soul Breaker (Stun 9 s,
// effectPower 50, hit 1360) held for a caster's or healer's cast and at
// reuse otherwise, then Stunning Fist (fixed) the same, so the interrupt
// is Soul Breaker, else Stunning Fist; Cripple to break off. The list names
// Force Blaster, so it repeats the M10 attacks through Force Blaster with
// one charge in M10 order (Force Storm from 49 and Force Burst from 43
// with two attackers under the area rule, Hurricane Assault with two
// charges, Burning Fist); the M10 steps it does not name play after it
// (pvpSteps): Iron Punch (fixed), Zealot from 58 under 30 %, Focused Force
// out of combat.
var tyrantPvP = []Step{
	{Skill: skillForceBlaster, Weapons: fistWeapons, Charges: 1, Opener: true, Vs: vsCA},
	{Skill: skillCripple, Weapons: []string{"FIST", "DUALFIST"}, Vs: vsCA},
	{Skill: skillSoulBreaker, Weapons: fistWeapons, Hold: true},
	{Skill: skillStunningFist, Weapons: fistWeapons, Hold: true},
	{Skill: skillCripple, Weapons: []string{"FIST", "DUALFIST"}, Break: true},
	{Skill: skillForceStorm, From: 49, Weapons: fistWeapons, Attackers: 2, Charges: 1},
	{Skill: skillForceBurst, From: 43, Weapons: fistWeapons, Attackers: 2, Charges: 1},
	{Skill: skillHurricaneAssault, Weapons: fistWeapons, Charges: 2},
	{Skill: skillBurningFist, Weapons: fistWeapons},
	{Skill: skillForceBlaster, Weapons: fistWeapons, Charges: 1},
}
