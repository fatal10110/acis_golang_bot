package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// blockMidway walks b from the corridor's west end toward its east end and
// stops the first segment halfway with the server's blocked correction. It
// returns that segment and the stop point.
func blockMidway(t *testing.T, b *Brain, dest Point) (origin, to, stop Point) {
	t.Helper()
	a, st := walkStep(t, b, dest, t0)
	if st != walkWalking || a.kind != actMove {
		t.Fatalf("first decision: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
	}
	origin, to = moveOrigin(a.payload), moveTarget(a.payload)
	stop = Point{X: (origin.X + to.X) / 2, Y: (origin.Y + to.Y) / 2, Z: origin.Z}
	b.handle([]Event{segAnswer(origin, to), MoveToLocation{ObjectID: selfID, Origin: stop, Dest: stop}}, t0)
	return origin, to, stop
}

// TestWalkerBlockedStopAvoidsSegment blocks the straight segment along the
// corridor halfway: the replan from the stop point keeps off the rest of
// that segment's line, so the next segment leaves its row; the avoid set
// holds on later replans of the walk and a new destination clears it.
func TestWalkerBlockedStopAvoidsSegment(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.CorridorWest, 0, 0), scenePoint(geotest.CorridorEast, 0, 0)
	b, _, reg := newNavBrain(t, n, Persona{}, from)
	_, to, stop := blockMidway(t, b, dest)
	if to != dest {
		t.Fatalf("first segment ends %+v, want the straight run to %+v", to, dest)
	}
	if s := reg.Collect(t0); s.NavBlocked != 1 {
		t.Fatalf("blocked %d, want 1", s.NavBlocked)
	}
	avoided := b.walk.plan.Avoid.Len()
	if avoided == 0 {
		t.Fatal("the blocked stop avoided no column")
	}

	now := t0.Add(time.Second)
	a, st := walkStep(t, b, dest, now)
	if st != walkWalking || a.kind != actMove {
		t.Fatalf("replan: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
	}
	o2, to2 := moveOrigin(a.payload), moveTarget(a.payload)
	if o2 != stop {
		t.Fatalf("replan from %+v, want the stop point %+v", o2, stop)
	}
	if to2 == to || engine.GeoY(int(to2.Y)) == engine.GeoY(int(stop.Y)) {
		t.Fatalf("replan sends %+v -> %+v: want off the refused segment's row", o2, to2)
	}

	// A stop that is no blocked stop replans the walk: the set holds.
	mid := Point{X: (o2.X + to2.X) / 2, Y: (o2.Y + to2.Y) / 2, Z: o2.Z}
	b.handle([]Event{segAnswer(o2, to2), StopMove{ObjectID: selfID, Pos: mid}}, now)
	now = now.Add(time.Second)
	if a, st := walkStep(t, b, dest, now); st != walkWalking || a.kind != actMove {
		t.Fatalf("second replan: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
	}
	if got := b.walk.plan.Avoid.Len(); got != avoided {
		t.Fatalf("second replan avoids %d columns, want the %d of the refused segment", got, avoided)
	}

	if _, st := b.goTo(from, 0, walkPlace, now); st != walkWalking {
		t.Fatalf("new destination: status %v (%s)", st, b.walk.reason)
	}
	if got := b.walk.plan.Avoid.Len(); got != 0 {
		t.Fatalf("a new destination keeps %d avoided columns", got)
	}
}

// TestWalkerAvoidCutsEveryWay ends the walk as blocked when the refused
// columns leave no way from the stop point.
func TestWalkerAvoidCutsEveryWay(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.CorridorWest, 0, 0), scenePoint(geotest.CorridorEast, 0, 0)
	b, _, _ := newNavBrain(t, n, Persona{}, from)
	_, _, stop := blockMidway(t, b, dest)
	corner := func(dx, dy int32) Point { return Point{X: stop.X + dx*32, Y: stop.Y + dy*32, Z: stop.Z} }
	ring := []Point{corner(-1, -1), corner(1, -1), corner(1, 1), corner(-1, 1), corner(-1, -1)}
	for i := 1; i < len(ring); i++ {
		b.walk.plan.Avoid.AddLine(toLoc(ring[i-1]), toLoc(ring[i]))
	}
	if a, st := walkStep(t, b, dest, t0.Add(time.Second)); st != walkUnreachable || a.kind != actHold || b.walk.reason != reasonBlocked {
		t.Fatalf("status %v kind %v reason %q, want unreachable: %q", st, a.kind, b.walk.reason, reasonBlocked)
	}
}
