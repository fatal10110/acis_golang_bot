package botclient

import (
	"testing"
	"time"
)

// TestWalkerHoldsThroughForeignRoute plays the frames of #502: a move of
// the bot's the walker did not click (another rule's click, routed by the
// server as a segment's would be, move/creature.go resolvePathLocked) and
// the next legs of that route, each sent at the end of the one before with
// no click (controller.go segmentAdvanced). The walker sends nothing at a
// leg's modeled end plus a tick, where the next leg is still on its way and
// would answer the walker's click with the server's own route; one position
// tick plus A after the last leg's modeled end it plans from where the
// route ended. A blocked correction stands the bot confirmed: no hold.
func TestWalkerHoldsThroughForeignRoute(t *testing.T) {
	dest := Point{X: 1000}
	z := int32(defaultCollisionHeight)
	// legGap is how long after a leg's modeled end plus a tick the next leg
	// arrives: inside the hold.
	const legGap = 50 * time.Millisecond
	tests := []struct {
		name string
		legs []Point
		// held is set when the walker waits for the server's route.
		held bool
	}{
		{name: "a routed move of another rule's", legs: []Point{{X: 300, Z: z}, {X: 600, Z: z}, {X: 800, Z: z}}, held: true},
		{name: "a direct move of another rule's", legs: []Point{{X: 200, Z: z}}, held: true},
		{name: "a blocked correction", legs: []Point{{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, out, reg := newNavBrain(t, testNav(t), Persona{}, Point{})
			b.nextTick, b.reactAt = t0.Add(time.Minute), time.Time{}
			now := t0
			origin := Point{}
			for i, leg := range tt.legs {
				if i > 0 {
					now = b.world.Self.arrival().Add(positionTick)
					if a, _ := walkStep(t, b, dest, now); a.kind != actHold {
						t.Fatalf("leg %d at its end plus a tick: %v, want a hold", i, a.kind)
					}
					now = now.Add(legGap)
				}
				b.handleOne(t, MoveToLocation{ObjectID: selfID, Origin: origin, Dest: leg}, now)
				origin = leg
			}
			if b.walk.held != tt.held {
				t.Fatalf("held %v after the server's move, want %v", b.walk.held, tt.held)
			}
			due := now
			if tt.held {
				due = b.world.Self.arrival().Add(positionTick + b.world.Self.lag())
				if a, _ := walkStep(t, b, dest, due.Add(-time.Millisecond)); a.kind != actHold {
					t.Fatalf("before the hold's end: %v, want a hold", a.kind)
				}
			}
			if len(out.frames) != 0 {
				t.Fatalf("sent %d packets before the hold's end, want none", len(out.frames))
			}
			a, st := walkStep(t, b, dest, due)
			if a.kind != actMove {
				t.Fatalf("at the hold's end: status %v kind %v (%s), want a segment", st, a.kind, b.walk.reason)
			}
			last := tt.legs[len(tt.legs)-1]
			if o := moveOrigin(a.payload); o.X != last.X || o.Y != 0 {
				t.Fatalf("segment from %+v, want the route's end X=%d", o, last.X)
			}
			if s := reg.Collect(due); s.NavDesync != 0 {
				t.Errorf("nav_desync_total %d, want 0: no leg answers a click of the walker's", s.NavDesync)
			}
		})
	}
}
