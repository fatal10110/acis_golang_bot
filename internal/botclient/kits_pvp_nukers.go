package botclient

import "slices"

// Skill ids the nuker PvP rows cast and their M10 rows do not
// (data/xml/skills): 1236 Frost Bolt (SG from 40), an MDAM with a 30 s
// run speed slow (×0.7, effectPower 60), hit 3100, castRange 900, reuse
// 8 s; 1064 Silence (SH from 40, owner decision O1, D50), a MUTE of power
// 80, hit 4000, reuse 60 s, a 30 s mute in which the player casts no
// magic skill (server model/actor/cast/player_actor.go MagicMuted) and
// shows the Mute bit (0x100) in its CharInfo.
const (
	skillSilence   int32 = 1064
	skillFrostBolt int32 = 1236
)

// The nukers' second-profession PvP rows (docs/pvp/rows-second.md, rows
// SN, SG, SH and WC; X27): whole lists set as their kits' PvP in place of
// the first profession's (humanWizardPvP, elvenWizardPvP, darkWizardPvP
// and orcShamanPvP stay for HW, EW, DW and OS). Their steps replace the
// M10 steps of every skill they name (pvpSteps), so each repeats its
// kit's second-tree attacks in M10 order (sorcererAttacks,
// spellsingerAttacks, spellhowlerAttacks); the first tree's attacks it
// does not name (Blaze, Solar Spark, Aqua Swirl, Frost Flame) and the
// sustain play their M10 steps behind it. A first-tree attack the list
// names (Ice Bolt) is no plain damage step: it plays only as the list's
// gated steps. The damage Int steps go when the break chance passes
// MinBreak, the sleep and fear Break steps under the sleep rule and the
// break-cast gate (docs/pvp/layer.md); the bot kites after a slow
// (Slow, Ice Bolt, Frost Bolt) as the X8 mystic rows do.
var (
	// sorcererPvP (SN): Ice Bolt opens on melee and archers, Prominence
	// on casters and healers; Slow and Ice Bolt (kite) on melee, Curse
	// Chaos on melee and archers; then the second tree's attacks
	// (Prominence, Surrender To Fire once, Rain of Fire from 58 with two
	// attackers, Aura Bolt within 400, Aura Flare within 150); Aura Bolt
	// and Aura Flare also as Int; Break: Ice Bolt unless the opponent is
	// still slowed, then the flee.
	sorcererPvP = slices.Concat([]Step{
		{Skill: skillIceBolt, Opener: true, Vs: mysticVsMA},
		{Skill: skillProminence, Opener: true, Vs: mysticVsCH},
		{Skill: skillSlow, Vs: mysticVsM},
		{Skill: skillCurseChaos, Vs: mysticVsMA},
		{Skill: skillIceBolt, Vs: mysticVsM},
	}, sorcererAttacks, []Step{
		{Skill: skillAuraBolt, Within: 400, Interrupt: true},
		{Skill: skillAuraFlare, Within: 150, Interrupt: true},
		{Skill: skillIceBolt, Break: true},
	})
	// spellsingerPvP (SG): Wind Shackle (fixed) on melee (opener on
	// archers), else Hydro Blast opens; Frost Bolt (kite) and Curse:
	// Weakness on melee; then the second tree's attacks (Hydro Blast, Ice
	// Dagger from 44, Solar Flare, Aqua Splash from 58 with two attackers,
	// Aura Bolt, Aura Flare); Int as SN; Break: Frost Bolt unless the
	// opponent is still slowed, then the flee.
	spellsingerPvP = slices.Concat([]Step{
		{Skill: skillWindShackle, Vs: mysticVsM},
		{Skill: skillWindShackle, Opener: true, Vs: mysticVsA},
		{Skill: skillHydroBlast, Opener: true},
		{Skill: skillFrostBolt, Vs: mysticVsM},
		{Skill: skillCurseWeakness, Vs: mysticVsM},
	}, spellsingerAttacks, []Step{
		{Skill: skillAuraBolt, Within: 400, Interrupt: true},
		{Skill: skillAuraFlare, Within: 150, Interrupt: true},
		{Skill: skillFrostBolt, Break: true},
	})
	// spellhowlerPvP (SH): Wind Shackle (fixed) on melee (opener on
	// archers); Silence (O1) on casters, healers and battle casters, again
	// only once its mute wore off (controlDue); Curse: Poison on casters
	// and healers, ahead of every attack, so their opener, not again while
	// it ticks; Slow and Ice Bolt (fixed, kite) on melee; then the second
	// tree's attacks (Hurricane, Death Spike from 44, Shadow Flare, Tempest
	// from 48 with two attackers, Shadow Spark, Twister, Aura Burn within
	// 150, the last three fixed); Aura Burn also as Int, as DW; Break: Ice
	// Bolt unless the opponent is still slowed, then the flee.
	spellhowlerPvP = slices.Concat([]Step{
		{Skill: skillWindShackle, Vs: mysticVsM},
		{Skill: skillWindShackle, Opener: true, Vs: mysticVsA},
		{Skill: skillSilence, Vs: mysticVsCHB},
		{Skill: skillCursePoison, Vs: mysticVsCH},
		{Skill: skillSlow, Vs: mysticVsM},
		{Skill: skillIceBolt, Vs: mysticVsM},
	}, spellhowlerAttacks, []Step{
		{Skill: skillAuraBurn, Within: 150, Interrupt: true},
		{Skill: skillIceBolt, Break: true},
	})
	// warcryerPvP (WC): Chant of Fire on itself against casters and
	// battle casters (as OS); Aura Sink on casters and healers; Hammer
	// Crush, then Stun Attack (fixed), with a blunt within stunAttackReach,
	// held for a caster's or healer's cast, at reuse against the others
	// (Hammer Crush no plain damage step); Dreaming Spirit Int as OM's.
	// Then M10: Steal Essence opens, Frost Flame (fixed); Steal Essence and
	// Life Drain (fixed) under 70 % HP. Break: Seal of Binding (fixed) on
	// a melee opponent within its 200 radius, Dreaming Spirit on one free
	// of DOTs, Fear, all under the break-cast gate (WC keeps OS's Seal of
	// Binding, D50).
	warcryerPvP = []Step{
		{Skill: skillChantFire, Target: TargetSelf, Vs: mysticVsCB},
		{Skill: skillAuraSink, Vs: mysticVsCH},
		{Skill: skillHammerCrush, Weapons: stunWeapons, Within: stunAttackReach, Hold: true},
		{Skill: skillStunAttack, Weapons: stunWeapons, Within: stunAttackReach, Hold: true},
		{Skill: skillDreamingSpirit, Interrupt: true},
		{Skill: skillSealBinding, Within: 200, Vs: mysticVsM, Break: true},
		{Skill: skillDreamingSpirit, Break: true},
		{Skill: skillFear, Break: true},
	}
)
