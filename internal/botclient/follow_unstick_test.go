package botclient

import (
	"math"
	"testing"
	"time"
)

// TestFollowUnstick has a bot following a leader out of sight, standing
// still, whose legs toward its slot stall where it stands, the server
// echoing its own position (acis_golang#5412, the village restart of #128):
// it sidesteps in each of sidestepTurns before it gives the slot up until the
// leader moves on. A sidestep that moves it sends the legs again; legs that
// move it no closer give up at once.
func TestFollowUnstick(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, b *Brain, slot Point, now time.Time)
	}{
		{
			name: "refused sidesteps: all turns, then given up until the leader moves on",
			run: func(t *testing.T, b *Brain, slot Point, now time.Time) {
				for i, turn := range sidestepTurns {
					checkSidestep(t, followMove(t, b, now), Point{}, slot, turn, i)
					echoInPlace(b, Point{}, now)
					now = now.Add(time.Second)
				}
				b.follow.wake = false
				followHold(t, b, now)
				followHold(t, b, now.Add(time.Minute))
				if !b.follow.dueAt.IsZero() || b.follow.traveling {
					t.Fatal("a bot out of sidesteps took its slot again while the leader stayed put")
				}
				b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: Point{X: 10000, Y: 1000}}}}, now)
				if a, _ := repath(t, b, now.Add(time.Minute+time.Second)); a.kind != actMove {
					t.Fatalf("after the leader moved on: kind %d, want a leg", a.kind)
				}
				if b.follow.unsticks != 0 {
					t.Errorf("unsticks = %d on the way to a new slot, want 0", b.follow.unsticks)
				}
			},
		},
		{
			name: "a sidestep that moves the bot: legs again from there",
			run: func(t *testing.T, b *Brain, slot Point, now time.Time) {
				to := moveTarget(followMove(t, b, now).payload)
				now = now.Add(time.Second)
				b.world.Apply(StopMove{ObjectID: selfID, Pos: to}, now)
				leg := legToward(to, slot, math.Sqrt(float64(dist2(to, slot))))
				if got := moveTarget(followMove(t, b, now).payload); got != leg {
					t.Fatalf("after the sidestep: move to %v, want the leg to %v", got, leg)
				}
				// The legs stall again there: the next sidestep in turn.
				for range spotStalls - 1 {
					echoInPlace(b, to, now)
					now = now.Add(time.Second)
					if got := moveTarget(followMove(t, b, now).payload); got != leg {
						t.Fatalf("move to %v, want the leg to %v again", got, leg)
					}
				}
				echoInPlace(b, to, now)
				checkSidestep(t, followMove(t, b, now.Add(time.Second)), to, slot, sidestepTurns[1], 1)
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

// TestFollowNoCloser has a bot whose legs move it, but no closer to its
// slot: no sidestep, the slot is given up at once.
func TestFollowNoCloser(t *testing.T) {
	b := newFollowBrain(t)
	b.world.Apply(DeleteObject{ObjectID: gmID}, t0)
	b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: Point{X: 10000}}}}, t0)
	a, now := repath(t, b, at(time.Second))
	if a.kind != actMove {
		t.Fatalf("kind %d, want a leg", a.kind)
	}
	slot := b.follow.dest
	d := math.Sqrt(float64(dist2(Point{}, slot)))
	// Each leg ends on the circle around the slot the bot started on, 500
	// further around it.
	for i := 1; i <= spotStalls; i++ {
		sin, cos := math.Sincos(float64(i) * 500 / d)
		pos := Point{
			X: slot.X + int32(math.Round(float64(-slot.X)*cos-float64(-slot.Y)*sin)),
			Y: slot.Y + int32(math.Round(float64(-slot.X)*sin+float64(-slot.Y)*cos)),
		}
		now = now.Add(time.Second)
		b.world.Apply(StopMove{ObjectID: selfID, Pos: pos}, now)
		if i < spotStalls {
			followMove(t, b, now)
		}
	}
	b.follow.wake = false
	followHold(t, b, now)
	if !b.follow.stuck || b.follow.unsticks != 0 {
		t.Fatalf("stuck %v after %d sidesteps, want given up without one", b.follow.stuck, b.follow.unsticks)
	}
}

// stalledFollower is a follow brain whose leader is out of sight at
// (10000, 0) and whose legs toward its slot stalled spotStalls times at the
// origin; it returns the slot and the time of the next decision, which
// sidesteps.
func stalledFollower(t *testing.T) (*Brain, Point, time.Time) {
	t.Helper()
	b := newFollowBrain(t)
	b.world.Apply(DeleteObject{ObjectID: gmID}, t0)
	b.world.Apply(PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: Point{X: 10000}}}}, t0)
	a, now := repath(t, b, at(time.Second))
	if a.kind != actMove {
		t.Fatalf("kind %d, want a leg", a.kind)
	}
	slot := b.follow.dest
	for range spotStalls - 1 {
		echoInPlace(b, Point{}, now)
		now = now.Add(time.Second)
		if got, want := moveTarget(followMove(t, b, now).payload), moveTarget(a.payload); got != want {
			t.Fatalf("leg to %v, want the same leg to %v again", got, want)
		}
	}
	echoInPlace(b, Point{}, now)
	return b, slot, now.Add(time.Second)
}

// echoInPlace is the server refusing the bot's last move where it stands at
// pos: ActionFailed and a MoveToLocation from pos to pos.
func echoInPlace(b *Brain, pos Point, now time.Time) {
	b.world.Apply(ActionFailed{}, now)
	b.world.Apply(MoveToLocation{ObjectID: selfID, Origin: pos, Dest: pos}, now)
}

// followMove decides at now and wants rule "follow" moving.
func followMove(t *testing.T, b *Brain, now time.Time) Action {
	t.Helper()
	name, a := b.decide(now)
	if name != "follow" || a.kind != actMove {
		t.Fatalf("decide(%v) = %q kind %d, want follow moving", now.Sub(t0), name, a.kind)
	}
	return a
}

// checkSidestep fails unless a is sidestep i of a bot at from: unstickStep
// long, turn degrees counterclockwise from the line to slot.
func checkSidestep(t *testing.T, a Action, from, slot Point, turn float64, i int) {
	t.Helper()
	to := moveTarget(a.payload)
	if got := math.Sqrt(float64(dist2(to, from))); math.Abs(got-unstickStep) > 1 {
		t.Errorf("sidestep %d to %v is %.0f long, want %d", i+1, to, got, unstickStep)
	}
	want := math.Atan2(float64(slot.Y-from.Y), float64(slot.X-from.X)) + turn*math.Pi/180
	if off := math.Remainder(math.Atan2(float64(to.Y-from.Y), float64(to.X-from.X))-want, 2*math.Pi); math.Abs(off) > 0.01 {
		t.Errorf("sidestep %d to %v is %.3f rad off %v degrees from the line to the slot %v", i+1, to, off, turn, slot)
	}
}
