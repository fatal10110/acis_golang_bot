package botclient

import (
	"encoding/binary"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// moveOrigin is the origin of a MoveBackwardToLocation payload.
func moveOrigin(p []byte) Point {
	i := func(off int) int32 { return int32(binary.LittleEndian.Uint32(p[off:])) }
	return Point{X: i(13), Y: i(17), Z: i(21)}
}

// walkStep is one decision of a walk to dest: the walker's packet, sent
// through the brain's pacer when there is one.
func walkStep(t *testing.T, b *Brain, dest Point, now time.Time) (Action, walkStatus) {
	t.Helper()
	a, st := b.goTo(dest, 0, walkPlace, now)
	if a.kind != actHold {
		if ok, err := b.send(a, now); !ok || err != nil {
			t.Fatalf("send %v at %v: sent %v, %v", a.kind, now.Sub(t0), ok, err)
		}
	}
	return a, st
}

// TestWalkerSegmentsFromStandstill walks out of the U to behind its base:
// the straight line is blocked, so the route takes several segments, each
// sent from where the previous one ended and only after its simulated
// arrival plus a tick, each passing CanMove.
func TestWalkerSegmentsFromStandstill(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	b, _, reg := newNavBrain(t, n, Persona{}, from)
	if b.walk.lineOpen(from, dest) {
		t.Fatal("the U's base should block the straight line")
	}
	now, prev := t0, Point{}
	var segs int
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
		origin, to := moveOrigin(a.payload), moveTarget(a.payload)
		if segs > 0 && origin != prev {
			t.Fatalf("segment %d from %+v, want the previous end %+v", segs, origin, prev)
		}
		if !n.Engine.CanMove(int(origin.X), int(origin.Y), int(origin.Z), int(to.X), int(to.Y), int(to.Z)) {
			t.Fatalf("segment %d %+v -> %+v fails CanMove", segs, origin, to)
		}
		segs++
		b.handle([]Event{segAnswer(origin, to)}, now)
		run := runFor(origin, to)
		if a, _ := b.goTo(dest, 0, walkPlace, now.Add(run-50*time.Millisecond)); a.kind != actHold {
			t.Fatalf("segment %d: %v sent before the arrival plus a tick (click-ahead)", segs, a.kind)
		}
		now, prev = now.Add(run+time.Millisecond), to
	}
	if segs < 2 {
		t.Errorf("%d segments, want the route around the U", segs)
	}
	if horizontal2(prev, dest) > walkSlop*walkSlop {
		t.Errorf("ended at %+v, want %+v", prev, dest)
	}
	if s := reg.Collect(now); s.NavBlocked+s.NavDesync+s.NavDoorReplan != 0 || s.NavSegments != uint64(segs) {
		t.Errorf("counters blocked %d desync %d door %d segments %d, want none and %d segments", s.NavBlocked, s.NavDesync, s.NavDoorReplan, s.NavSegments, segs)
	}
}

// TestWalkerServerAnswers feeds each server answer to a segment and checks
// what the walker counts and where it plans from next.
func TestWalkerServerAnswers(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	tests := []struct {
		name string
		// reply answers the segment from origin to to.
		reply                  func(origin, to Point) []Event
		blocked, desync, doors uint64
		// from is where the next segment must go out from, given the
		// first one.
		from func(origin, to Point) Point
	}{
		{
			name:    "same-point MoveToLocation is a blocked stop",
			reply:   func(o, _ Point) []Event { return []Event{MoveToLocation{ObjectID: selfID, Origin: o, Dest: o}} },
			blocked: 1,
			from:    func(o, _ Point) Point { return o },
		},
		{
			// A step failing mid-way: the server answered the segment,
			// then sends its blocked correction (move/controller.go
			// BroadcastBlockedCorrection).
			name: "same-point MoveToLocation short of the end after the answer is a blocked stop",
			reply: func(o, to Point) []Event {
				mid := Point{X: (o.X + to.X) / 2, Y: (o.Y + to.Y) / 2, Z: o.Z}
				return []Event{segAnswer(o, to), MoveToLocation{ObjectID: selfID, Origin: mid, Dest: mid}}
			},
			blocked: 1,
			from:    func(o, to Point) Point { return Point{X: (o.X + to.X) / 2, Y: (o.Y + to.Y) / 2, Z: o.Z} },
		},
		{
			name: "same-point MoveToLocation on the end after the answer is its arrival",
			reply: func(o, to Point) []Event {
				return []Event{segAnswer(o, to), MoveToLocation{ObjectID: selfID, Origin: to, Dest: to}}
			},
			from: func(_, to Point) Point { return to },
		},
		{
			// A stun or a stop: no blocked step, the route replans.
			name: "StopMove short of the end replans without a blocked stop",
			reply: func(o, to Point) []Event {
				mid := Point{X: (o.X + to.X) / 2, Y: (o.Y + to.Y) / 2, Z: o.Z}
				return []Event{segAnswer(o, to), StopMove{ObjectID: selfID, Pos: mid}}
			},
			from: func(o, to Point) Point { return Point{X: (o.X + to.X) / 2, Y: (o.Y + to.Y) / 2, Z: o.Z} },
		},
		{
			name: "another destination is a desync",
			reply: func(o, to Point) []Event {
				other := Point{X: o.X + (to.X-o.X)/3, Y: o.Y + (to.Y-o.Y)/3, Z: o.Z}
				return []Event{MoveToLocation{ObjectID: selfID, Origin: o, Dest: other}}
			},
			desync: 1,
			from:   func(o, to Point) Point { return Point{X: o.X + (to.X-o.X)/3, Y: o.Y + (to.Y-o.Y)/3, Z: o.Z} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, reg := newNavBrain(t, n, Persona{}, from)
			a, _ := walkStep(t, b, dest, t0)
			o, to := moveOrigin(a.payload), moveTarget(a.payload)
			b.handle(tt.reply(o, to), t0)
			s := reg.Collect(t0)
			if s.NavBlocked != tt.blocked || s.NavDesync != tt.desync || s.NavDoorReplan != tt.doors {
				t.Fatalf("blocked %d desync %d door %d, want %d %d %d", s.NavBlocked, s.NavDesync, s.NavDoorReplan, tt.blocked, tt.desync, tt.doors)
			}
			now := t0.Add(time.Minute)
			a, st := walkStep(t, b, dest, now)
			if st != walkWalking || a.kind != actMove {
				t.Fatalf("after the answer: status %v kind %v (%s), want a new move", st, a.kind, b.walk.reason)
			}
			want := tt.from(o, to)
			if got := moveOrigin(a.payload); got.X != want.X || got.Y != want.Y {
				t.Errorf("replanned from %+v, want %+v", got, want)
			}
		})
	}
}

// TestWalkerGivesUp ends a route after repeated failures of one kind, shown
// in the status snapshot, while door replans never end it.
func TestWalkerGivesUp(t *testing.T) {
	n := testNav(t)
	tests := []struct {
		name   string
		nav    *Nav
		from   geotest.Point
		dest   geotest.Point
		reply  func(origin, to Point) []Event
		reason string
	}{
		{
			name: "blocked", nav: n, from: geotest.UInside, dest: geotest.UBehind,
			reply:  func(o, _ Point) []Event { return []Event{MoveToLocation{ObjectID: selfID, Origin: o, Dest: o}} },
			reason: "unreachable: blocked",
		},
		{
			name: "desync", nav: n, from: geotest.UInside, dest: geotest.UBehind,
			reply: func(o, to Point) []Event {
				return []Event{MoveToLocation{ObjectID: selfID, Origin: o, Dest: Point{X: (o.X + to.X) / 2, Y: (o.Y + to.Y) / 2, Z: o.Z}}}
			},
			reason: "unreachable: desync",
		},
		{
			name: "watchdog", nav: n, from: geotest.UInside, dest: geotest.UBehind,
			reply:  func(Point, Point) []Event { return nil },
			reason: "unreachable: watchdog",
		},
		{
			// Each answer stops the bot short of the door, so every next
			// segment crosses it again.
			name: "door replans do not count", nav: corridorNav(t, n), from: geotest.CorridorWest, dest: geotest.CorridorEast,
			reply: func(o, to Point) []Event {
				return []Event{MoveToLocation{ObjectID: selfID, Origin: o, Dest: Point{X: o.X + (to.X-o.X)/8, Y: o.Y + (to.Y-o.Y)/8, Z: o.Z}}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := scenePoint(tt.dest, 0, 0)
			b, _, reg := newNavBrain(t, tt.nav, Persona{}, scenePoint(tt.from, 0, 0))
			now := t0
			for range walkMaxBlocked {
				a, st := walkStep(t, b, dest, now)
				if st != walkWalking || a.kind != actMove {
					t.Fatalf("status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
				}
				o, to := moveOrigin(a.payload), moveTarget(a.payload)
				evs := tt.reply(o, to)
				b.handle(evs, now)
				// The server's move of a desync runs out first.
				now = now.Add(time.Minute)
				if len(evs) > 0 {
					if m, ok := evs[0].(MoveToLocation); ok {
						b.world.Self.move = stopAt(m.Dest)
					}
				}
			}
			_, st := b.goTo(dest, 0, walkPlace, now)
			if got := b.walk.failed(); got != tt.reason {
				t.Errorf("status %v, failed %q, want %q", st, got, tt.reason)
			}
			if tt.reason == "" {
				if s := reg.Collect(now); s.NavDoorReplan != walkMaxBlocked || s.NavDesync != 0 {
					t.Errorf("door replans %d desync %d, want %d and 0", s.NavDoorReplan, s.NavDesync, walkMaxBlocked)
				}
			}
		})
	}
}

// corridorNav is n with a door table holding the corridor's door, open so
// the shared engine stays as built.
func corridorNav(t *testing.T, n *Nav) *Nav {
	t.Helper()
	tmpl := geotest.CorridorDoor()
	tmpl.Opened = true
	doors, err := door.NewTable([]*door.Template{tmpl})
	if err != nil {
		t.Fatal(err)
	}
	c := *n
	c.Doors = NewDoorTable(n.Engine, doors, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &c
}

// TestWalkerHoldsDuringSwingAndCast sends no segment during the bot's own
// swing or cast: the server would defer it and answer ActionFailed.
func TestWalkerHoldsDuringSwingAndCast(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	for _, name := range []string{"swing", "cast"} {
		t.Run(name, func(t *testing.T) {
			b, _, _ := newNavBrain(t, n, Persona{}, from)
			if name == "swing" {
				b.handle([]Event{Attack{AttackerID: selfID, Pos: from, Hits: []AttackHit{{TargetID: npcID}}}}, t0)
			} else {
				b.world.Self.castEnd = t0.Add(time.Second)
			}
			if a, st := b.goTo(dest, 0, walkPlace, t0); a.kind != actHold || st != walkWalking {
				t.Fatalf("during the %s: kind %v status %v, want a hold", name, a.kind, st)
			}
			if a, _ := b.goTo(dest, 0, walkPlace, t0.Add(slowSwing)); a.kind != actMove {
				t.Errorf("after the %s: kind %v, want a move", name, a.kind)
			}
		})
	}
}

// TestWalkerRecovery drops off one-way-out ground into an abstract SCC with
// one checked route, and asks /unstuck only on isolated ground.
func TestWalkerRecovery(t *testing.T) {
	n := testNav(t)
	t.Run("one-way-out drops", func(t *testing.T) {
		from := scenePoint(geotest.BridgeWestDeck, 0, 0)
		b, _, _ := newNavBrain(t, n, Persona{}, from)
		a, st := walkStep(t, b, scenePoint(geotest.PitOutside, 0, 0), t0)
		if st != walkWalking || a.kind != actMove {
			t.Fatalf("status %v kind %v (%s), want the drop", st, a.kind, b.walk.reason)
		}
		// The deck's stacked tops pass the corridor test up to its edge,
		// so the route may walk to the edge before the drop (#256): its
		// end, not its first segment, lands on InSCC ground.
		route := b.walk.route
		if len(route) == 0 || moveTarget(a.payload) != route[0] {
			t.Fatalf("move to %+v, route %+v: want the route's first point", moveTarget(a.payload), route)
		}
		to := route[len(route)-1]
		c, ok := b.walk.plan.Locate(toLoc(to))
		if !ok {
			t.Fatalf("drop to %+v off the geodata", to)
		}
		if m, err := n.Hier.Locate(c, nil); err != nil || m.Kind() != nav.InSCC {
			t.Errorf("drop to %+v: kind %v err %v, want InSCC ground", to, m.Kind(), err)
		}
	})
	t.Run("isolated asks /unstuck once", func(t *testing.T) {
		b, _, _ := newNavBrain(t, n, Persona{}, scenePoint(geotest.PitInside, 0, 0))
		dest := scenePoint(geotest.PitOutside, 0, 0)
		a, _ := walkStep(t, b, dest, t0)
		if a.kind != actUserCommand {
			t.Fatalf("kind %v (%s), want /unstuck", a.kind, b.walk.reason)
		}
		if a, st := b.goTo(dest, 0, walkPlace, t0.Add(time.Second)); a.kind != actHold || st != walkWalking {
			t.Fatalf("waiting for the recall: kind %v status %v", a.kind, st)
		}
		// The recall left the bot where it stood.
		if _, st := b.goTo(dest, 0, walkPlace, t0.Add(escapeRetry)); st != walkUnreachable || b.walk.reason != reasonIsolated {
			t.Errorf("status %v reason %q, want unreachable isolated", st, b.walk.reason)
		}
	})
}
