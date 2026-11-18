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
)

// sidestepTurns are the directions of a visit's sidesteps, in turn, in
// degrees counterclockwise from the line to the visited NPC: square to it
// left and right, straight back, then the diagonals back and ahead. Straight
// ahead is the way the legs stall. The server ends a straight move at the
// first wall (geo.ValidLocation), so a sidestep moves the bot unless the wall
// is right where it stands; all of them are tried before the visit's one
// /unstuck.
var sidestepTurns = [...]float64{90, -90, 180, 135, -135, 45, -45}

// budged reports whether the bot at from stands away from at.
func budged(from, at Point) bool { return dist2(from, at) > stuckSlack*stuckSlack }

// unstick is the next try of visit v at leaving the spot at from, d away
// from the NPC's spawn, where its legs stall in place: a sidestep in each of
// sidestepTurns, then one /unstuck, whose recall lands the bot in the
// nearest town; the bot stands still while that cast runs. A sidestep or
// recall that moves the bot lets the legs go again (walkTo). It reports
// false once the tries ran out, or escapeRetry after an /unstuck, or after
// the end of its recall cast, left the bot where it stood: the visit is over.
//
// Sidesteps are moves and go through the frame pacer like the legs, one per
// decision; a visit sends at most one /unstuck (RequestUserCommand, no
// server gate).
func (b *Brain) unstick(v *npcVisit, from Point, d float64, now time.Time) (Action, bool) {
	switch {
	case v.unsticks < len(sidestepTurns):
		turn := sidestepTurns[v.unsticks]
		v.unsticks++
		v.stuckAt = from
		return moveAction(sidestep(from, v.at, d, turn), from), true
	case v.escapeAt.IsZero():
		v.stuckAt, v.escapeAt = from, now
		return escapeAction(), true
	case b.world.Self.Casting(now) || now.Sub(escapeWait(v.escapeAt, b.world.Self.castEnd)) < escapeRetry:
		return Action{}, true
	}
	return Action{}, false
}

// escapeWait is when the wait for an /unstuck asked at asked counts from:
// the end of the recall cast when one ran after it (the server teleports
// when the cast completes, so the TeleportToLocation lands just after
// castEnd), else when it was asked.
func escapeWait(asked, castEnd time.Time) time.Time {
	if castEnd.After(asked) {
		return castEnd
	}
	return asked
}

// sidestep is the end of a sidestep of a bot at from facing to, d away:
// unstickStep from from, turn degrees counterclockwise from the line to to.
func sidestep(from, to Point, d, turn float64) Point {
	ux, uy := float64(to.X-from.X)/d, float64(to.Y-from.Y)/d
	sin, cos := math.Sincos(turn * math.Pi / 180)
	return Point{
		X: from.X + int32(math.Round((ux*cos-uy*sin)*unstickStep)),
		Y: from.Y + int32(math.Round((ux*sin+uy*cos)*unstickStep)),
		Z: from.Z,
	}
}
