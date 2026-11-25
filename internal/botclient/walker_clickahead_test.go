package botclient

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// aheadStart and aheadDest are the ends of a follow walk on the open flat
// ground east of the origin, clear of the region's edge at Y 0: segments
// of at most movingSegment, every corner between them open to its fan.
var aheadStart, aheadDest = Point{X: 16, Y: 200}, Point{X: 1000, Y: 300}

// followStep is one decision of a follow walk to dest: the walker's packet,
// sent through the brain's pacer when there is one.
func followStep(t *testing.T, b *Brain, dest Point, now time.Time) (Action, walkStatus) {
	t.Helper()
	a, st := b.goTo(dest, 0, walkFollow, now)
	if a.kind != actHold {
		if ok, err := b.send(a, now); !ok || err != nil {
			t.Fatalf("send %v at %v: sent %v, %v", a.kind, now.Sub(t0), ok, err)
		}
	}
	return a, st
}

// aheadWalk is a brain on the open flat ground whose follow walk to
// aheadDest sent its first segment at t0, answered at t0+wakeAnswerLag, with
// the regular tick a minute away; first is that segment, at when its
// click-ahead is due.
func aheadWalk(t *testing.T) (b *Brain, reg *botmetrics.Registry, first segment, at time.Time) {
	t.Helper()
	b, _, reg = newNavBrain(t, testNav(t), Persona{}, aheadStart)
	a, st := followStep(t, b, aheadDest, t0)
	if st != walkWalking || a.kind != actMove {
		t.Fatalf("first decision: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
	}
	if len(b.walk.route) < 2 {
		t.Fatalf("route %v, want a corner", b.walk.route)
	}
	b.handleOne(t, segAnswer(moveOrigin(a.payload), moveTarget(a.payload)), t0.Add(wakeAnswerLag))
	b.nextTick, b.reactAt = t0.Add(time.Minute), time.Time{}
	s := &b.world.Self
	return b, reg, b.walk.seg, s.arrival().Add(-s.aheadLead())
}

// aheadSent is aheadWalk with the click-ahead sent at at.
func aheadSent(t *testing.T) (b *Brain, reg *botmetrics.Registry, prev segment, at time.Time) {
	t.Helper()
	b, reg, prev, at = aheadWalk(t)
	if a, _ := followStep(t, b, aheadDest, at); a.kind != actMove {
		t.Fatalf("click-ahead at %v: %v, want a move", at.Sub(t0), a.kind)
	}
	return b, reg, prev, at
}

// TestClickAheadAtCheckedCorner checks a click-ahead (#308): the walk wakes
// lead before the segment's modeled end, holds before then, and clicks the
// next waypoint while the bot still moves, from its dead-reckoned position,
// with no standstill gate or layer snap; it idles zero and the route goes on
// from the next waypoint.
func TestClickAheadAtCheckedCorner(t *testing.T) {
	b, reg, first, at := aheadWalk(t)
	w, s := &b.walk, &b.world.Self
	w1, w2 := w.route[0], w.route[1]
	if got := b.decisionAt(); !got.Equal(at) {
		t.Fatalf("decision at %v, want the click-ahead at %v", got.Sub(t0), at.Sub(t0))
	}
	if a, _ := followStep(t, b, aheadDest, at.Add(-time.Millisecond)); a.kind != actHold {
		t.Fatalf("%v sent before the click-ahead time", a.kind)
	}
	if !s.Moving(at) {
		t.Fatal("the bot must still move at the click-ahead time")
	}
	pos := s.Position(at)
	a, st := followStep(t, b, aheadDest, at)
	if st != walkWalking || a.kind != actMove {
		t.Fatalf("click-ahead: status %v kind %v, want a move", st, a.kind)
	}
	if got := moveTarget(a.payload); got != w2 {
		t.Errorf("click-ahead to %+v, want the next waypoint %+v", got, w2)
	}
	if got := moveOrigin(a.payload); got != pos {
		t.Errorf("click-ahead origin %+v, want the dead-reckoned %+v", got, pos)
	}
	if !w.seg.ahead || w.prev.to != w1 || w.prev.sentAt != first.sentAt || len(w.route) == 0 || w.route[0] != w2 {
		t.Errorf("after the click: seg %+v prev %+v route %v, want a click-ahead after the first segment, route from %+v", w.seg, w.prev, w.route, w2)
	}
	line := segment{fan0: first.origin, fan1: w1}
	if horizontal2(w.seg.fan1, w1) != 0 || horizontal2(w.seg.fan0, w1) == 0 || !line.inFan(w.seg.fan0) {
		t.Errorf("fan %+v..%+v, want the last stretch before %+v", w.seg.fan0, w.seg.fan1, w1)
	}
	snap := reg.Collect(at)
	if snap.NavWalkIdleMillis.Count != 1 || snap.NavWalkIdleMillis.Max != 0 || snap.NavSegments != 2 {
		t.Errorf("idle %+v segments %d, want one zero idle and two segments", snap.NavWalkIdleMillis, snap.NavSegments)
	}
	// The server takes each click where the bot's model has it and walks
	// on: the walk arrives, every later corner clicked ahead too.
	now, ahead := at, 1
	for i := 0; ; i++ {
		if i > 20 {
			t.Fatal("no arrival after 20 decisions")
		}
		if a.kind == actMove {
			answerAt := now.Add(wakeAnswerLag)
			b.handleOne(t, segAnswer(s.Position(answerAt), moveTarget(a.payload)), answerAt)
		}
		now = b.decisionAt()
		if a, st = followStep(t, b, aheadDest, now); st == walkArrived {
			break
		}
		if a.kind == actMove && w.seg.ahead {
			ahead++
		}
	}
	snap = reg.Collect(now)
	if snap.NavBlocked+snap.NavDesync != 0 {
		t.Errorf("blocked %d desync %d, want none", snap.NavBlocked, snap.NavDesync)
	}
	if want := int(snap.NavSegments) - 1; ahead != want {
		t.Errorf("%d click-aheads over %d segments, want one at every corner", ahead, snap.NavSegments)
	}
}

// TestClickAheadFanFails checks that no click goes out early when a line of
// the fan fails: the segment is marked, the walk wakes at its end and the
// next segment goes out from standstill.
func TestClickAheadFanFails(t *testing.T) {
	b, _, _, at := aheadWalk(t)
	w, s := &b.walk, &b.world.Self
	w1 := w.route[0]
	c := along(w.seg, 1-40/segLen(w.seg))
	// A column the lines from the fan's start cross, not the planned
	// segment's: as a blocked stop on another route would leave.
	w.plan.Avoid.AddLine(toLoc(Point{X: c.X, Y: c.Y + 40}), toLoc(Point{X: c.X, Y: c.Y - 40}))
	if a, _ := followStep(t, b, aheadDest, at); a.kind != actHold {
		t.Fatalf("%v sent through a failing fan", a.kind)
	}
	if !w.seg.noAhead {
		t.Fatal("the segment is not marked: its fan would be checked again")
	}
	due := s.arrival().Add(positionTick)
	if got := b.decisionAt(); !got.Equal(due) {
		t.Fatalf("decision at %v, want the end plus a tick %v", got.Sub(t0), due.Sub(t0))
	}
	a, _ := followStep(t, b, aheadDest, due)
	if a.kind != actMove || moveOrigin(a.payload) != w1 || w.seg.ahead {
		t.Errorf("at the end plus a tick: %v from %+v ahead %v, want a standstill segment from %+v", a.kind, moveOrigin(a.payload), w.seg.ahead, w1)
	}
}

// TestClickAheadUCorners walks around the U, whose corners hug its walls:
// every fan there fails, so every segment goes out from standstill.
func TestClickAheadUCorners(t *testing.T) {
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	b, _, _ := newNavBrain(t, testNav(t), Persona{}, from)
	w, s := &b.walk, &b.world.Self
	now := t0
	var refused int
	for i := 0; ; i++ {
		if i > 60 {
			t.Fatal("no arrival after 60 decisions")
		}
		a, st := walkStep(t, b, dest, now)
		if st == walkArrived {
			break
		}
		if a.kind != actMove || w.seg.ahead {
			t.Fatalf("decision %d: %v ahead %v, want a standstill move", i, a.kind, w.seg.ahead)
		}
		b.handleOne(t, segAnswer(moveOrigin(a.payload), moveTarget(a.payload)), now)
		if len(w.route) >= 2 {
			at := s.arrival().Add(-s.aheadLead())
			if a, _ := walkStep(t, b, dest, at); a.kind != actHold {
				t.Fatalf("segment %d: %v sent ahead at a wall-hugging corner", i, a.kind)
			}
			if w.seg.noAhead {
				refused++
			}
		}
		now = s.arrival().Add(positionTick)
	}
	if refused == 0 {
		t.Error("no fan was checked: the route has no corner")
	}
}

// TestClickAheadAnswers feeds the server's answers to a click-ahead, and the
// previous segment's last packets arriving after it went out, and checks
// what the walker charges and where it goes on from.
func TestClickAheadAnswers(t *testing.T) {
	tests := []struct {
		name string
		// events are the server's packets after the click, given the
		// previous segment and the click's dead-reckoned origin pos.
		events func(prev, seg segment, pos Point) []Event
		// blocked is the blocked stops counted; route whether the walk
		// still follows its route; inFlight whether the click is still
		// the segment in flight; from where the server took it.
		blocked  uint64
		route    bool
		inFlight bool
		from     func(prev segment, pos Point) Point
	}{
		{
			name:     "answer from the fan",
			events:   func(_, seg segment, pos Point) []Event { return []Event{segAnswer(pos, seg.to)} },
			route:    true,
			inFlight: true,
			from:     func(_ segment, pos Point) Point { return pos },
		},
		{
			name:     "answer from the previous end: the click came after the arrival",
			events:   func(prev, seg segment, _ Point) []Event { return []Event{segAnswer(prev.to, seg.to)} },
			route:    true,
			inFlight: true,
			from:     func(prev segment, _ Point) Point { return prev.to },
		},
		{
			name: "answer off the fan: stop following the route",
			events: func(prev, seg segment, _ Point) []Event {
				return []Event{segAnswer(along(prev, 1.0/3), seg.to)}
			},
			inFlight: true,
			from:     func(prev segment, _ Point) Point { return along(prev, 1.0/3) },
		},
		{
			name: "stale blocked stop of the previous segment, then the click from the stop",
			events: func(prev, seg segment, _ Point) []Event {
				stop := along(prev, 0.5)
				return []Event{MoveToLocation{ObjectID: selfID, Origin: stop, Dest: stop}, segAnswer(stop, seg.to)}
			},
			blocked:  1,
			inFlight: true,
			from:     func(prev segment, _ Point) Point { return along(prev, 0.5) },
		},
		{
			name: "stale sticky-block arrival of the previous segment",
			events: func(prev, seg segment, _ Point) []Event {
				end := prev.to
				end.Z += defaultCollisionHeight
				return []Event{MoveToLocation{ObjectID: selfID, Origin: end, Dest: end}, segAnswer(prev.to, seg.to)}
			},
			route:    true,
			inFlight: true,
			from:     func(prev segment, _ Point) Point { return prev.to },
		},
		{
			name: "stale StopMove at the previous end",
			events: func(prev, _ segment, _ Point) []Event {
				return []Event{StopMove{ObjectID: selfID, Pos: prev.to}}
			},
			route:    true,
			inFlight: true,
			from:     func(prev segment, _ Point) Point { return prev.to },
		},
		{
			name: "stale StopMove short of the previous end drops the route",
			events: func(prev, _ segment, _ Point) []Event {
				return []Event{StopMove{ObjectID: selfID, Pos: along(prev, 0.5)}}
			},
		},
		{
			name: "sticky-block arrival of the click is its arrival",
			events: func(_, seg segment, pos Point) []Event {
				end := seg.to
				end.Z += defaultCollisionHeight
				return []Event{segAnswer(pos, seg.to), MoveToLocation{ObjectID: selfID, Origin: end, Dest: end}}
			},
			route:    true,
			inFlight: true,
			from:     func(_ segment, pos Point) Point { return pos },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, reg, prev, at := aheadSent(t)
			w := &b.walk
			seg := w.seg
			pos := b.world.Self.Position(at.Add(wakeAnswerLag))
			var want nav.Avoid
			if tt.blocked > 0 {
				stop := toLoc(along(prev, 0.5))
				want.AddLine(stop, w.plan.BlockedEnd(toLoc(prev.from), toLoc(prev.to), stop))
			}
			for _, ev := range tt.events(prev, seg, pos) {
				b.handleOne(t, ev, at.Add(wakeAnswerLag))
			}
			if s := reg.Collect(at); s.NavBlocked != tt.blocked || s.NavDesync != 0 {
				t.Errorf("blocked %d desync %d, want %d and none", s.NavBlocked, s.NavDesync, tt.blocked)
			}
			if got := w.plan.Avoid.Len(); got != want.Len() {
				t.Errorf("%d avoided columns, want %d: the previous segment's refused part", got, want.Len())
			}
			if got := len(w.route) > 0; got != tt.route {
				t.Errorf("route %v, want followed %v", w.route, tt.route)
			}
			if got := !w.seg.sentAt.IsZero(); got != tt.inFlight {
				t.Fatalf("segment in flight %v, want %v", got, tt.inFlight)
			}
			if !tt.inFlight {
				return
			}
			if w.seg.answered {
				if from := tt.from(prev, pos); horizontal2(w.seg.from, from) != 0 {
					t.Errorf("segment from %+v, want the server's origin %+v", w.seg.from, from)
				}
			}
		})
	}
}

// along is the point a fraction f of the way along seg's line from its
// server origin to its end.
func along(seg segment, f float64) Point {
	from := seg.origin
	return Point{
		X: from.X + int32(math.Round(f*float64(seg.to.X-from.X))),
		Y: from.Y + int32(math.Round(f*float64(seg.to.Y-from.Y))),
		Z: seg.to.Z,
	}
}

// segLen is seg's horizontal length from its server origin.
func segLen(seg segment) float64 { return math.Sqrt(float64(horizontal2(seg.origin, seg.to))) }
