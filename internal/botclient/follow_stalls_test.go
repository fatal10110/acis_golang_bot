package botclient

import (
	"math"
	"testing"
	"time"
)

// TestFollowUnstickPerStall has a follower whose long trip to its slot
// stalls in place again and again, as the village restart's way back over
// rough ground does (#131): the sidesteps are counted per stall, so a stall
// the legs got past leaves the next one every sidestep again, while one
// they did not get past keeps counting toward giving the slot up.
func TestFollowUnstickPerStall(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, b *Brain, slot Point, now time.Time)
	}{
		{
			name: "every stall got past: more stalls than sidestepTurns, never given up",
			run: func(t *testing.T, b *Brain, slot Point, now time.Time) {
				pos := Point{}
				for i := range len(sidestepTurns) + 2 {
					a := followMove(t, b, now)
					checkSidestep(t, a, pos, slot, sidestepTurns[0], i)
					to := moveTarget(a.payload)
					now = now.Add(time.Second)
					b.world.Apply(StopMove{ObjectID: selfID, Pos: to}, now)
					if i < followDetours {
						now = refuseDetour(t, b, to, now)
					}
					followMove(t, b, now)
					// The leg gets the bot 500 on, past the stall, where the
					// legs stall again.
					pos = toward(to, slot, 500)
					now = now.Add(time.Second)
					b.world.Apply(StopMove{ObjectID: selfID, Pos: pos}, now)
					now = stallLegs(t, b, pos, now)
				}
				if b.follow.stuck || !b.follow.traveling {
					t.Fatal("a follower that got past every stall gave its slot up")
				}
			},
		},
		{
			name: "a stall not got past: the next sidestep in turn",
			run: func(t *testing.T, b *Brain, slot Point, now time.Time) {
				checkSidestep(t, followMove(t, b, now), Point{}, slot, sidestepTurns[0], 0)
				// The server slides the bot 300 straight back; the next leg
				// gets it 250 on, still farther from the slot than the stall.
				back := toward(Point{}, slot, -300)
				now = now.Add(time.Second)
				b.world.Apply(StopMove{ObjectID: selfID, Pos: back}, now)
				now = refuseDetour(t, b, back, now)
				followMove(t, b, now)
				pos := toward(back, slot, 250)
				now = now.Add(time.Second)
				b.world.Apply(StopMove{ObjectID: selfID, Pos: pos}, now)
				now = stallLegs(t, b, pos, now)
				checkSidestep(t, followMove(t, b, now), pos, slot, sidestepTurns[1], 1)
				if b.follow.unsticks != 2 {
					t.Errorf("unsticks = %d, want 2", b.follow.unsticks)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, slot, now := stalledFollower(t)
			tt.run(t, b, slot, now)
		})
	}
}

// stallLegs has the follower at pos send its legs, each echoed in place,
// until the next decision, at the time it returns, sidesteps.
func stallLegs(t *testing.T, b *Brain, pos Point, now time.Time) time.Time {
	t.Helper()
	followMove(t, b, now)
	for range spotStalls - 1 {
		echoInPlace(b, pos, now)
		now = now.Add(time.Second)
		followMove(t, b, now)
	}
	echoInPlace(b, pos, now)
	return now.Add(time.Second)
}

// toward is from moved n along the line to to (back for a negative n).
func toward(from, to Point, n float64) Point {
	d := math.Sqrt(float64(dist2(from, to)))
	return Point{
		X: from.X + int32(math.Round(float64(to.X-from.X)*n/d)),
		Y: from.Y + int32(math.Round(float64(to.Y-from.Y)*n/d)),
	}
}
