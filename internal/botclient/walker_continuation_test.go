package botclient

import (
	"testing"
	"time"
)

// TestWalkerHoldsThroughServerRoute plays the frames of #501: the server
// answers the walker's click with the first waypoint of a route of its own
// (move/creature.go resolvePathLocked), then sends each next leg at the end
// of the one before, with no click (controller.go segmentAdvanced), each
// leg's origin the last one's destination. The walker holds through the
// whole route: no segment goes out at the end of a leg, where the next leg
// is still on its way, and no leg counts as a desync. One position tick plus
// A after the last leg's modeled end it plans from where the route ended, or
// arrives when the route ended at its destination.
func TestWalkerHoldsThroughServerRoute(t *testing.T) {
	dest := Point{X: 1000}
	// legGap is how long after the walker's old click time (a leg's
	// modeled end plus a tick) the next leg arrives: inside the hold.
	const legGap = 50 * time.Millisecond
	tests := []struct {
		name string
		// last is where the server's route ends, X along the walk.
		last int32
		// arrives is set when the route ends at the walk's destination.
		arrives bool
	}{
		{name: "a route ending short of the destination replans from its end", last: 800},
		{name: "a route ending at the destination arrives", last: 1000, arrives: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, out, _ := newNavBrain(t, testNav(t), Persona{}, Point{})
			a, st := walkStep(t, b, dest, t0)
			if st != walkWalking || a.kind != actMove {
				t.Fatalf("first decision: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
			}
			b.nextTick, b.reactAt = t0.Add(time.Minute), time.Time{}
			z := int32(defaultCollisionHeight)
			legs := []Point{{X: 300, Z: z}, {X: 600, Z: z}, {X: tt.last, Z: z}}
			origin := moveOrigin(a.payload)
			// The click's answer: the route's first waypoint, a desync.
			now := t0.Add(wakeAnswerLag)
			b.handleOne(t, MoveToLocation{ObjectID: selfID, Origin: origin, Dest: legs[0]}, now)
			if b.walk.desync != 1 || !b.walk.held {
				t.Fatalf("after the answer: desync %d held %v, want 1 and held", b.walk.desync, b.walk.held)
			}
			sent := len(out.frames)
			for i, leg := range legs[1:] {
				// The walker's old click time: the leg's modeled end plus a
				// tick, before the next leg arrives.
				now = b.world.Self.arrival().Add(positionTick)
				if a, _ := walkStep(t, b, dest, now); a.kind != actHold {
					t.Fatalf("leg %d at its end plus a tick: %v, want a hold", i+1, a.kind)
				}
				now = now.Add(legGap)
				b.handleOne(t, MoveToLocation{ObjectID: selfID, Origin: legs[i], Dest: leg}, now)
			}
			if b.walk.desync != 1 {
				t.Errorf("desync %d after the route, want 1: a continuation leg is no answer", b.walk.desync)
			}
			due := b.world.Self.arrival().Add(positionTick + b.world.Self.lag())
			if got := b.decisionAt(); got != due {
				t.Fatalf("decision at %v, want the hold's end %v", got.Sub(t0), due.Sub(t0))
			}
			if a, _ := walkStep(t, b, dest, due.Add(-time.Millisecond)); a.kind != actHold {
				t.Fatalf("before the hold's end: %v, want a hold", a.kind)
			}
			if len(out.frames) != sent {
				t.Fatalf("sent %d packets during the hold, want none", len(out.frames)-sent)
			}
			a, st = walkStep(t, b, dest, due)
			if tt.arrives {
				if st != walkArrived || a.kind != actHold {
					t.Fatalf("at the hold's end: status %v kind %v, want arrived with no segment", st, a.kind)
				}
				return
			}
			if a.kind != actMove {
				t.Fatalf("at the hold's end: status %v kind %v (%s), want the replan's segment", st, a.kind, b.walk.reason)
			}
			if o := moveOrigin(a.payload); o.X != tt.last || o.Y != 0 {
				t.Fatalf("replan from %+v, want the route's end X=%d", o, tt.last)
			}
		})
	}
}

// TestWalkerHoldEnds checks what ends the hold before its time: the server's
// StopMove or blocked correction stands the bot confirmed, so the replan
// goes out at the next decision.
func TestWalkerHoldEnds(t *testing.T) {
	dest := Point{X: 1000}
	stand := Point{X: 200, Z: defaultCollisionHeight}
	tests := []struct {
		name string
		ev   Event
	}{
		{name: "StopMove", ev: StopMove{ObjectID: selfID, Pos: stand}},
		{name: "blocked correction", ev: MoveToLocation{ObjectID: selfID, Origin: stand, Dest: stand}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := newNavBrain(t, testNav(t), Persona{}, Point{})
			a, _ := walkStep(t, b, dest, t0)
			now := t0.Add(wakeAnswerLag)
			b.handleOne(t, MoveToLocation{ObjectID: selfID, Origin: moveOrigin(a.payload), Dest: Point{X: 300, Z: defaultCollisionHeight}}, now)
			if !b.walk.held {
				t.Fatal("desync answer: not held")
			}
			now = now.Add(time.Second)
			b.handleOne(t, tt.ev, now)
			if b.walk.held {
				t.Fatal("still held after the server's standstill")
			}
			if a, st := walkStep(t, b, dest, now); a.kind != actMove {
				t.Fatalf("after the standstill: status %v kind %v (%s), want the replan's segment", st, a.kind, b.walk.reason)
			}
		})
	}
}
