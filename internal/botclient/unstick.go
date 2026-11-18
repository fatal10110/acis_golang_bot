package botclient

import (
	"math"
	"time"
)

// Unsticking a visit. The server walks the approach of a click straight
// (MoveToPawn), and it can end in a spot its routed ground moves cannot
// leave: the pathfinder finds no way out and the straight fallback
// (acis_golang move.resolvePathLocked, geo.ValidLocation) ends where the bot
// stands, so every leg comes back as ActionFailed and a MoveToLocation from
// the bot's own position to the same position (#122).
const (
	// stuckSlack is how far from where its last leg went out the bot may
	// stand and still stand where it was: one geodata cell.
	stuckSlack = 16
	// unstickStep is the length of a sidestep, a short leg in another
	// direction than the visited NPC's.
	unstickStep = 300
	// unstickSteps is how many sidesteps a visit tries (left, right, back)
	// before its one /unstuck.
	unstickSteps = 3
)

// budged reports whether the bot at from stands away from at.
func budged(from, at Point) bool { return dist2(from, at) > stuckSlack*stuckSlack }

// unstick is the next try of visit v at leaving the spot at from, d away
// from the NPC's spawn, where its legs stall in place: a sidestep to the
// left, to the right, then back from the NPC, then one /unstuck, whose
// recall lands the bot in the nearest town; the bot stands still while that
// cast runs. Each sidestep that moves the bot lets the legs go again. It
// reports false once the tries ran out, or escapeRetry after an /unstuck no
// cast came of: the visit is over.
//
// Sidesteps are moves and go through the frame pacer like the legs, one per
// decision; a visit sends at most one /unstuck (RequestUserCommand, no
// server gate).
func (b *Brain) unstick(v *npcVisit, from Point, d float64, now time.Time) (Action, bool) {
	switch {
	case v.unsticks < unstickSteps:
		v.unsticks++
		v.stuckAt = from
		return moveAction(sidestep(from, v.at, d, v.unsticks), from), true
	case v.escapeAt.IsZero():
		v.stuckAt, v.escapeAt = from, now
		return escapeAction(), true
	case b.world.Self.Casting(now) || now.Sub(v.escapeAt) < escapeRetry:
		return Action{}, true
	}
	return Action{}, false
}

// sidestep is the end of sidestep n (1 left, 2 right, 3 back) of a bot at
// from facing to, d away: unstickStep from from, square to the line to to
// or straight away from it.
func sidestep(from, to Point, d float64, n int) Point {
	ux, uy := float64(to.X-from.X)/d, float64(to.Y-from.Y)/d
	switch n {
	case 1:
		ux, uy = -uy, ux
	case 2:
		ux, uy = uy, -ux
	default:
		ux, uy = -ux, -uy
	}
	return Point{
		X: from.X + int32(math.Round(ux*unstickStep)),
		Y: from.Y + int32(math.Round(uy*unstickStep)),
		Z: from.Z,
	}
}
