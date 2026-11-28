package botclient

import (
	"cmp"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// ruleFarm is the rule 9 errand "farm": a bot that arrived at its farming
// spot (rule "spot") and is out of combat attacks a creature that hit it
// lately, else its target while that one is still a farm target, else the
// nearest farm target. The fight itself goes on under rule 5 once the
// server reports the bot in combat. With no farm target the turn goes to
// the persona's goal, until the bot has gone Persona.Spots.Idle with none:
// then it moves on and avoids the spot for AvoidFor (idleOut, #360). A
// leader waiting for a bot member behind (gathering) picks no new target
// and its wait is no idle stretch.
//
// While its servitor's next upkeep checkpoint is due and the bot cannot
// pay it (upkeepBlocks), it starts no fight on a fresh target, only on an
// attacker.
//
// It sends what fight sends, a selection or one attack per target paced by
// the brain's decision tick and the frame pacer, plus the stand-up toggle
// of a bot that sat down idling.
func (b *Brain) ruleFarm(now time.Time) (Action, bool) {
	s, v := &b.world.Self, &b.spot
	if b.cfg.Spots == nil || !v.farming() || !v.arrived || s.InCombat {
		return Action{}, false
	}
	if b.duel(now).Phase != DuelIdle {
		// From a challenge until idle no monster fight starts: a hit
		// on an NPC cancels the duel (docs/pvp/finding.md "Clear of
		// monsters").
		return Action{}, true
	}
	id := b.farmTarget(now)
	if id != 0 && b.upkeepBlocks() && !slices.Contains(b.attackers(now), id) {
		// No new pull: the servitor's next checkpoint is due and unpaid,
		// so it would leave mid-fight; it runs out first.
		return Action{}, false
	}
	if id == 0 {
		if b.gathering(now) {
			v.idleSince = time.Time{}
			return Action{}, false
		}
		if b.idleOut(now) {
			b.moveOn(now, b.persona.Spots.AvoidFor)
			return Action{}, true
		}
		return Action{}, false
	}
	v.idleSince = time.Time{}
	if s.Sitting {
		return b.toggle(now), true
	}
	return b.fight(id, now), true
}

// farmTarget is the creature the farming bot attacks at now, 0 for none:
// its hardest hitter first, so attackers come before fresh targets, then
// its target and then the nearest monster that is a farm target, unless it
// leads a party whose bot member it waits for (gathering).
func (b *Brain) farmTarget(now time.Time) int32 {
	if hits := b.attackers(now); len(hits) > 0 {
		return hits[0]
	}
	sp := &b.cfg.Spots.spots[b.spot.idx]
	centre := Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z}
	if t := b.world.Target; b.mayFarm(t, sp, centre, now) {
		return t
	}
	if b.gathering(now) {
		return 0
	}
	from := b.world.Self.Position(now)
	var (
		id int32
		d  int64
	)
	for k, o := range b.world.Objects {
		if !b.mayFarm(k, sp, centre, now) {
			continue
		}
		if dk := dist2(from, o.Position(now)); id == 0 || cmp.Or(cmp.Compare(dk, d), cmp.Compare(k, id)) < 0 {
			id, d = k, dk
		}
	}
	return id
}

// mayFarm reports whether id is a farm target at now: a monster the bot may
// fight within the leash of spot sp's centre or inside sp's territory
// (inTerritory). A monster another player is fighting is a target all the
// same: kill stealing is allowed (owner decision on #138, plan "Shared
// behavior").
func (b *Brain) mayFarm(id int32, sp *datapack.Spot, centre Point, now time.Time) bool {
	if !b.world.monster(id) || !b.mayFight(id, now) {
		return false
	}
	leash := int64(b.persona.Spots.Leash)
	at := b.world.Objects[id].Position(now)
	return dist2(at, centre) <= leash*leash || inTerritory(sp, at)
}

// inTerritory reports whether p lies inside spot sp's territory, within its
// height band: the maker spreads its monsters over the whole territory,
// which can be far wider than the leash (oren23_2018_04 spans about 4000 by
// 6000), so a bot held at the centre would see them fought and never reach
// one (#447). The leash still bounds a spot with no territory.
func inTerritory(sp *datapack.Spot, p Point) bool {
	t := sp.Territory
	return t != nil && p.Z >= t.MinZ && p.Z <= t.MaxZ && t.Contains(datapack.Point{X: p.X, Y: p.Y})
}

// idleGap is the longest pause between two decisions that find the bot at
// its spot with no farm target within one idle stretch: a longer one means
// a rule ahead held the turn (a fight, a rest, a supply trip) and the
// stretch starts over.
const idleGap = 30 * time.Second

// idleOut records a decision at now that finds the bot at its spot, out of
// combat, with no farm target, and reports whether such decisions have run
// on for Persona.Spots.Idle: the spot gives it nothing to fight, every
// monster in reach killed by others or none coming near (#360). A farm
// target ends the stretch (ruleFarm), as does leaving the spot (leaveSpot).
func (b *Brain) idleOut(now time.Time) bool {
	v, d := &b.spot, b.persona.Spots.Idle
	if d <= 0 {
		return false
	}
	if v.idleSince.IsZero() || now.Sub(v.idleSeen) > idleGap {
		v.idleSince = now
	}
	v.idleSeen = now
	return now.Sub(v.idleSince) >= d
}

// monster reports whether id is an attackable NPC in sight that is alive.
// A player's summon is not one: the server marks it attackable when its
// owner is flagged, has karma or stands in a PvP zone (visibility.go
// AttackableWithoutForceBy), and hitting it is PvP.
func (w *World) monster(id int32) bool {
	o := w.object(id)
	return o != nil && o.Kind == KindNPC && o.Attackable && !o.Summon && !o.Dead
}
