package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestDoorTableChanges counts every door state change the table applies to
// the engine, and nothing else: the doors closed at boot, a state a door
// already holds and an id the table does not hold leave the count (#566).
func TestDoorTableChanges(t *testing.T) {
	e := newRecordingEngine()
	tbl := newDoorTable(e, flatGround{}, []*door.Template{testDoor(1, true), testDoor(2, false)}, discardLog())
	if got := tbl.Changes(); got != 0 {
		t.Fatalf("%d changes at boot, want 0", got)
	}
	for i, s := range []struct {
		id     int
		closed bool
		want   uint64
	}{
		{1, true, 1},
		{1, true, 1},
		{99, true, 1},
		{2, false, 2},
		{2, false, 2},
		{1, false, 3},
	} {
		tbl.Set(s.id, s.closed)
		if got := tbl.Changes(); got != s.want || e.toggles-1 != int(s.want) {
			t.Fatalf("step %d (door %d closed %v): %d changes, %d engine toggles after boot, want %d", i, s.id, s.closed, got, e.toggles-1, s.want)
		}
	}
}

// TestWalkerSkipsVerifiedSegments walks out of the U on a hierarchy route
// with no door change: every segment goes out from the waypoint the plan
// verified, so none is checked again (#566).
func TestWalkerSkipsVerifiedSegments(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	b, _, _ := newNavBrain(t, n, Persona{}, from)
	now, segs := t0, 0
	for i := 0; ; i++ {
		if i > 60 {
			t.Fatal("no arrival after 60 decisions")
		}
		a, st := walkStep(t, b, dest, now)
		if st == walkArrived {
			break
		}
		if st != walkWalking || a.kind != actMove {
			t.Fatalf("decision %d: status %v kind %v (%s), want a move", i, st, a.kind, b.walk.reason)
		}
		segs++
		if b.walk.checks != 0 {
			t.Fatalf("segment %d: %d checks, want none", segs, b.walk.checks)
		}
		b.handle([]Event{segAnswer(moveOrigin(a.payload), moveTarget(a.payload))}, now)
		now = now.Add(segRun(b, now) + time.Millisecond)
	}
	if segs < 2 {
		t.Errorf("%d segments, want the route around the U", segs)
	}
}

// TestWalkerRechecksSegments checks the segment after the U route's first
// one again when the plan no longer vouches for it: a door changed since
// the plan, or the bot stands off the waypoint. A door closed across it
// fails the check, and the walker replans as before (#566).
func TestWalkerRechecksSegments(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	corridorDoor := geotest.CorridorDoor().ID
	tests := []struct {
		name string
		nav  *Nav
		// between runs once the first segment, from origin to waypoint,
		// arrived, before the decision on the segment from waypoint to next.
		between func(t *testing.T, b *Brain, origin, waypoint, next Point)
		checks  int
		// sent is whether that decision sends the segment toward next.
		sent bool
	}{
		{
			name: "no door change", nav: n,
			between: func(*testing.T, *Brain, Point, Point, Point) {},
			checks:  0, sent: true,
		},
		{
			// The corridor's door, far from the U, closes and opens again:
			// the engine is as planned, but the count moved.
			name: "a door change off the route", nav: corridorNav(t, n),
			between: func(_ *testing.T, b *Brain, _, _, _ Point) {
				b.walk.nav.Doors.Set(corridorDoor, true)
				b.walk.nav.Doors.Set(corridorDoor, false)
			},
			checks: 1, sent: true,
		},
		{
			name: "a door closed across the next segment", nav: n,
			between: func(t *testing.T, b *Brain, _, waypoint, next Point) {
				withDoorAt(t, b, midpoint(waypoint, next))
				doors := b.walk.nav.Doors
				id := geotest.CorridorDoor().ID
				doors.Set(id, true)
				t.Cleanup(func() { doors.Set(id, false) })
			},
			checks: 1, sent: false,
		},
		{
			// The server put the bot a little past the waypoint without
			// the walker seeing why: standing re-snaps its position, on the
			// line the plan checked, so the check passes.
			name: "a position off the waypoint", nav: n,
			between: func(_ *testing.T, b *Brain, _, waypoint, next Point) {
				b.world.Self.move = stopAt(Point{X: waypoint.X + (next.X-waypoint.X)/8, Y: waypoint.Y + (next.Y-waypoint.Y)/8, Z: waypoint.Z})
			},
			checks: 1, sent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := newNavBrain(t, tt.nav, Persona{}, from)
			a, _ := walkStep(t, b, dest, t0)
			if a.kind != actMove || b.walk.checks != 0 || len(b.walk.route) < 2 {
				t.Fatalf("first decision: kind %v, %d checks, route %v: want an unchecked move on a route of several segments", a.kind, b.walk.checks, b.walk.route)
			}
			origin, waypoint, next := moveOrigin(a.payload), moveTarget(a.payload), b.walk.route[1]
			b.handle([]Event{segAnswer(origin, waypoint)}, t0)
			now := t0.Add(segRun(b, t0) + time.Millisecond)
			tt.between(t, b, origin, waypoint, next)
			pos := b.standing(now)
			a, st := walkStep(t, b, dest, now)
			if b.walk.checks != tt.checks {
				t.Errorf("%d checks, want %d", b.walk.checks, tt.checks)
			}
			if tt.sent {
				if st != walkWalking || a.kind != actMove || moveTarget(a.payload) != next || moveOrigin(a.payload) != pos {
					t.Fatalf("status %v kind %v, want the move from %+v to %+v", st, a.kind, pos, next)
				}
				return
			}
			if st != walkWalking || a.kind != actHold || b.walk.route != nil || b.walk.watchdog != 1 {
				t.Fatalf("status %v kind %v route %v watchdog %d, want a hold and the route dropped", st, a.kind, b.walk.route, b.walk.watchdog)
			}
			// The next decision plans again from where the bot stands.
			a, st = walkStep(t, b, dest, now)
			switch {
			case st == walkWalking && a.kind == actMove:
				if moveOrigin(a.payload) != pos {
					t.Errorf("replanned from %+v, want %+v", moveOrigin(a.payload), pos)
				}
			case st == walkUnreachable && b.walk.reason == reasonDoor:
			default:
				t.Errorf("after the replan: status %v kind %v (%s), want a move or a door", st, a.kind, b.walk.reason)
			}
		})
	}
}

// BenchmarkWalkerURoute walks out of the U, plan and segments, with no door
// change: the share segmentOK took of it is what #566 skips.
func BenchmarkWalkerURoute(bm *testing.B) {
	n := testNav(bm)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	for bm.Loop() {
		bm.StopTimer()
		b, _, _ := newNavBrain(bm, n, Persona{}, from)
		bm.StartTimer()
		now := t0
		for range 60 {
			a, st := b.goTo(dest, 0, walkPlace, now)
			if st != walkWalking || a.kind != actMove {
				break
			}
			if ok, err := b.send(a, now); !ok || err != nil {
				bm.Fatalf("send: %v %v", ok, err)
			}
			b.handle([]Event{segAnswer(moveOrigin(a.payload), moveTarget(a.payload))}, now)
			now = now.Add(segRun(b, now) + time.Millisecond)
		}
		bm.ReportMetric(float64(b.walk.checks), "checks/op")
	}
}
