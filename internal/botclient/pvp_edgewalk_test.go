package botclient

import (
	"testing"
	"time"
)

// TestPvPArenaEdgeWalk checks the arena-edge wake of a walking opponent
// (docs/pvp/guards.md "Arena edge"): a MoveToLocation that starts outside
// the swing margin wakes the guards at the frame the walk crosses it, a
// decision before then keeps that wake, and the guard's move goes at it. In
// the test arena (a box 2000 to each side) the swing margin of a run speed
// of 124 and an attack speed of 333 is 124 × (1.5015 s + 0.5 s) = 248.2: a
// walk from 490 off the edge reaches it after 1.95 s.
func TestPvPArenaEdgeWalk(t *testing.T) {
	const (
		start    = time.Second
		crossing = start + 1950*time.Millisecond
		frame    = time.Second / frameRate
	)
	tests := []struct {
		name string
		dest Point
		// decideAt, when set, is a regular decision before the crossing.
		decideAt time.Duration
		want     bool
	}{
		{name: "a walk out of the arena: woken at the crossing", dest: Point{Y: 2100}, want: true},
		{name: "a decision before the crossing keeps the wake", dest: Point{Y: 2100}, decideAt: 2 * time.Second, want: true},
		{name: "a walk ending 300 off the edge: no wake", dest: Point{Y: 1700}},
		{name: "a walk deeper in: no wake", dest: Point{Y: 1000}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := guardBrain(t, true, 0, 0)
			b.handleOne(t, foeAt(Point{Y: 1510}, 0, 0, 0), t0)
			b.reactAt = time.Time{}
			if rule, a := b.decide(t0); a.kind != actAttack {
				t.Fatalf("first decision: rule %q kind %v, want an attack", rule, a.kind)
			} else if _, err := b.send(a, t0); err != nil {
				t.Fatal(err)
			}
			b.handleOne(t, MoveToLocation{ObjectID: pcID, Origin: Point{Y: 1510}, Dest: tc.dest}, at(start))
			got := b.edgeCross(at(start))
			if !tc.want {
				if !got.IsZero() {
					t.Fatalf("edge wake at %v, want none", got.Sub(t0))
				}
				return
			}
			if got.Before(at(crossing)) || got.After(at(crossing+frame)) {
				t.Fatalf("edge wake at %v, want in [%v, %v]", got.Sub(t0), crossing, crossing+frame)
			}
			if b.reactAt.IsZero() || b.reactAt.After(got) {
				t.Fatalf("woke at %v, want by %v", b.reactAt.Sub(t0), got.Sub(t0))
			}
			if tc.decideAt > 0 {
				b.nextTick = time.Time{}
				if err := b.wake(at(tc.decideAt)); err != nil {
					t.Fatal(err)
				}
				if !b.nextTick.After(at(tc.decideAt)) {
					t.Fatalf("no decision at %v", tc.decideAt)
				}
				if b.reactAt.IsZero() || b.reactAt.After(got) {
					t.Fatalf("after the decision at %v: woke at %v, want by %v", tc.decideAt, b.reactAt.Sub(t0), got.Sub(t0))
				}
			}
			if rule, a := b.decide(got.Add(-frame)); rule == "guard" {
				t.Fatalf("a frame before the crossing: rule %q kind %v, want no guard", rule, a.kind)
			}
			if rule, a := b.decide(got); a.kind != actMove || rule != "guard" {
				t.Fatalf("at the crossing: rule %q kind %v, want the guard's move", rule, a.kind)
			}
		})
	}
}
