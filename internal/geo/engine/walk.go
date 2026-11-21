package engine

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// walkStartUpdates is how many position updates of a player's move advance
// at its start speed before the run speed takes over.
const walkStartUpdates = 5

// walkMaxZ caps a ground mover's height outside water.
const walkMaxZ = 16410

// Walk is where one sequence of a player's ground position updates along a
// straight leg leaves the player.
type Walk struct {
	// Stop is the cell the player stands on after the last update applied.
	Stop location.Location
	// Steps counts the updates applied; a blocked update is not applied.
	Steps int
	// Reached reports that the last update applied landed on the leg's
	// destination (its ground height snapped as the server snaps it).
	Reached bool
	// Blocked reports that an update failed CanMove from Stop, which ends
	// the walk where it stands.
	Blocked bool
}

// WalkSteps runs the server's player step chain for a ground walk from
// from to to: each update advances by its elapsed whole milliseconds (at
// least one) of the current speed per second, startSpeed for the first five
// updates and speed after, measured from the current cell toward the
// destination; the landing cell rounds half up, its height is the ground
// at most two cell heights above the current one, and each step must pass
// CanMove from the current cell or the walk stops there blocked. It returns
// where stepDurations, the time between consecutive position updates, leave
// the player. It reads only the engine's geodata and changes nothing.
func (e *Engine) WalkSteps(from, to location.Location, startSpeed, speed float64, stepDurations []time.Duration) Walk {
	w := Walk{Stop: from}
	dest := to
	accurate := [3]float64{float64(from.X), float64(from.Y), float64(from.Z)}
	for i, elapsed := range stepDurations {
		updateSpeed := speed
		if i < walkStartUpdates {
			updateSpeed = startSpeed
		}
		passed := playerPassed(updateSpeed, elapsed)

		dest.Z = min(int(e.Height(dest.X, dest.Y, dest.Z)), walkMaxZ)
		origin := w.Stop
		dx := float64(dest.X) - float64(origin.X)
		dy := float64(dest.Y) - float64(origin.Y)
		dz := float64(dest.Z) - float64(origin.Z)
		left := math.Sqrt(dx*dx + dy*dy)

		var next location.Location
		reached := left == 0 || passed >= left
		if reached {
			next = dest
			next.Z = min(next.Z, walkMaxZ)
		} else {
			fraction := passed / left
			accurate = [3]float64{accurate[0] + dx*fraction, accurate[1] + dy*fraction, accurate[2] + dz*fraction}
			next.X, next.Y = roundHalfUp(accurate[0]), roundHalfUp(accurate[1])
			next.Z = min(int(e.Height(next.X, next.Y, origin.Z+2*block.CellHeight)), walkMaxZ)
		}
		if !e.CanMove(origin.X, origin.Y, origin.Z, next.X, next.Y, next.Z) {
			w.Blocked = true
			return w
		}
		w.Stop = next
		w.Steps++
		if reached {
			w.Reached = true
			return w
		}
	}
	return w
}

// playerPassed is how far a player's position update covering elapsed
// advances at speed: the elapsed whole milliseconds, at least one, of the
// speed per second.
func playerPassed(speed float64, elapsed time.Duration) float64 {
	ms := elapsed.Milliseconds()
	if ms <= 0 {
		ms = 1
	}
	return speed / (1000 / float64(ms))
}

// roundHalfUp rounds v to the nearest integer, halves toward +Inf.
func roundHalfUp(v float64) int {
	return int(math.Floor(v + 0.5))
}
