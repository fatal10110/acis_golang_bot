package botclient

import (
	"cmp"
	"time"
)

// ruleFarm is the rule 9 errand "farm": a bot that arrived at its farming
// spot (rule "spot") and is out of combat attacks a creature that hit it
// lately, else its target while that one is still a farm target, else the
// nearest farm target. The fight itself goes on under rule 5 once the
// server reports the bot in combat. With no farm target the turn goes to
// the persona's goal.
//
// It sends what fight sends, a selection or one attack per target paced by
// the brain's decision tick and the frame pacer, plus the stand-up toggle
// of a bot that sat down idling.
func (b *Brain) ruleFarm(now time.Time) (Action, bool) {
	s, v := &b.world.Self, &b.spot
	if b.cfg.Spots == nil || !v.farming() || !v.arrived || s.InCombat {
		return Action{}, false
	}
	id := b.farmTarget(now)
	if id == 0 {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	return b.fight(id, now), true
}

// farmTarget is the creature the farming bot attacks at now, 0 for none:
// its hardest hitter first, so attackers come before fresh targets, then
// its target and then the nearest monster that is a farm target.
func (b *Brain) farmTarget(now time.Time) int32 {
	if hits := b.attackers(now); len(hits) > 0 {
		return hits[0]
	}
	sp := &b.cfg.Spots.spots[b.spot.idx]
	centre := Point{X: sp.Center.X, Y: sp.Center.Y, Z: sp.Z}
	if t := b.world.Target; b.mayFarm(t, centre, now) {
		return t
	}
	from := b.world.Self.Position(now)
	var (
		id int32
		d  int64
	)
	for k, o := range b.world.Objects {
		if !b.mayFarm(k, centre, now) {
			continue
		}
		if dk := dist2(from, o.Position(now)); id == 0 || cmp.Or(cmp.Compare(dk, d), cmp.Compare(k, id)) < 0 {
			id, d = k, dk
		}
	}
	return id
}

// mayFarm reports whether id is a farm target at now: a monster the bot may
// fight within the spot's leash of centre. A monster another player is
// fighting is a target all the same: kill stealing is allowed (owner
// decision on #138, plan "Shared behavior").
func (b *Brain) mayFarm(id int32, centre Point, now time.Time) bool {
	if !b.world.monster(id) || !b.mayFight(id, now) {
		return false
	}
	leash := int64(b.persona.Spots.Leash)
	return dist2(b.world.Objects[id].Position(now), centre) <= leash*leash
}

// monster reports whether id is an attackable NPC in sight that is alive.
// A player's summon is not one: the server marks it attackable when its
// owner is flagged, has karma or stands in a PvP zone (visibility.go
// AttackableWithoutForceBy), and hitting it is PvP.
func (w *World) monster(id int32) bool {
	o := w.object(id)
	return o != nil && o.Kind == KindNPC && o.Attackable && !o.Summon && !o.Dead
}
