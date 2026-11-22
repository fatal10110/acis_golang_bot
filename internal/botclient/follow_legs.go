package botclient

import "math"

// The straight legs and the stall count follow still walks with. The spot
// rule walks through the walker (goTo) since #158; follow moves onto routes
// in #159, which deletes this file.
const (
	// spotLeg is the longest move of a follow leg, well under the 9900 the
	// server refuses (movement.go moveLivePlayer).
	spotLeg = 3000
	// spotProgress is how much closer each leg must bring the bot;
	// spotStalls legs in a row short of it count as a stall.
	spotProgress = 100
	spotStalls   = 3
)

// legToward is the point at most spotLeg along the line from from to to, d
// away, z in proportion.
func legToward(from, to Point, d float64) Point {
	if d <= spotLeg {
		return to
	}
	f := spotLeg / d
	return Point{
		X: from.X + int32(math.Round(float64(to.X-from.X)*f)),
		Y: from.Y + int32(math.Round(float64(to.Y-from.Y)*f)),
		Z: from.Z + int32(math.Round(float64(to.Z-from.Z)*f)),
	}
}
