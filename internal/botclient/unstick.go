package botclient

import "math"

// The follow rule's sidesteps (follow.go), kept until the follow rule moves
// onto the walker (N7, #159), which deletes them with its own heuristics. The
// visits walk through the walker (walker.go) and sidestep no more.
const (
	// stuckSlack is how far from where its last leg went out the bot may
	// stand and still stand where it was: one geodata cell.
	stuckSlack = 16
	// unstickStep is the length of a sidestep, a short leg in another
	// direction than the destination's.
	unstickStep = 300
)

// sidestepTurns are the directions of the follow rule's sidesteps, in turn,
// in degrees counterclockwise from the line to its destination: square to it
// left and right, straight back, then the diagonals back and ahead.
var sidestepTurns = [...]float64{90, -90, 180, 135, -135, 45, -45}

// budged reports whether the bot at from stands away from at.
func budged(from, at Point) bool { return dist2(from, at) > stuckSlack*stuckSlack }

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
