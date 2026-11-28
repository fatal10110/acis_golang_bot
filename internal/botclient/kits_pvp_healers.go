package botclient

// The healers' second-profession PvP rows (docs/pvp/rows-second.md, rows
// BI and EE; X28): whole lists set as their kits' PvP in place of the
// Cleric's and the Elven Oracle's (clericPvP and elvenOraclePvP stay for
// CL and EO). The Prophet keeps the Cleric's list and the Shillien Elder
// the Shillien Oracle's (O3). Their heals are the kits', under the
// long-cast rule; neither names a rotation skill but Wind Strike, so the
// M10 steps after the list are only EE's Dryad Root on a second attacker.
var (
	// bishopPvP (BI): Mental Shield on itself against casters and battle
	// casters; Dryad Root and Ice Bolt (kite) on melee; Wind Strike; Trance
	// from 56 (O1) as the interrupt of a heal, a sleep or a long cast
	// (docs/pvp/layer.md "Interrupts", exempt from the sleep rule). Break:
	// Dryad Root on a melee opponent, else Trance from 56 only under the
	// sleep rule (no HP DOT on the opponent) and the break-cast gate; then
	// the flee.
	bishopPvP = []Step{
		{Skill: skillMentalShield, Target: TargetSelf, Vs: mysticVsCB},
		{Skill: skillDryadRoot, Vs: mysticVsM},
		{Skill: skillIceBolt, Vs: mysticVsM},
		{Skill: skillWindStrike},
		{Skill: skillTrance, From: 56, Interrupt: true},
		{Skill: skillDryadRoot, Vs: mysticVsM, Break: true},
		{Skill: skillTrance, From: 56, Break: true},
	}
	// elvenElderPvP (EE): as BI, with Wind Shackle on melee ahead of Dryad
	// Root.
	elvenElderPvP = []Step{
		{Skill: skillMentalShield, Target: TargetSelf, Vs: mysticVsCB},
		{Skill: skillWindShackle, Vs: mysticVsM},
		{Skill: skillDryadRoot, Vs: mysticVsM},
		{Skill: skillIceBolt, Vs: mysticVsM},
		{Skill: skillWindStrike},
		{Skill: skillTrance, From: 56, Interrupt: true},
		{Skill: skillDryadRoot, Vs: mysticVsM, Break: true},
		{Skill: skillTrance, From: 56, Break: true},
	}
)
