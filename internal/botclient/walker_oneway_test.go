package botclient

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// farDropBudget stands in for walkBudget on the far drop: the cell search
// alone runs out of it between the plateau and the leader past the wall,
// as a far leader past a drop runs walkBudget out on real ground (#231);
// the hierarchy's route keeps well under it.
const farDropBudget = 20000

var (
	farDropOnce sync.Once
	farDropNav  *Nav
	farDropErr  error
)

// farDropTestNav is the navigation over the far drop, built once per test
// binary.
func farDropTestNav(tb testing.TB) *Nav {
	tb.Helper()
	farDropOnce.Do(func() { farDropNav, farDropErr = buildTestNav(geotest.FarDrop(), navWindow(1, 1, 3, 2)) })
	if farDropErr != nil {
		tb.Fatal(farDropErr)
	}
	return farDropNav
}

// TestWalkerFollowsOneWayFar has a follower on the far drop's plateau walk
// to its leader on the ground past the wall, only one-way reachable and
// farther than the cell search alone covers within the walk's budget: the
// walk drops off the plateau and arrives in checked segments, while a walk
// that may not take drops refuses the leader as unreachable.
func TestWalkerFollowsOneWayFar(t *testing.T) {
	n := farDropTestNav(t)
	from, dest := scenePoint(geotest.FarDropHigh, 0, 0), scenePoint(geotest.FarDropLow, 0, 0)
	t.Run("follow arrives", func(t *testing.T) {
		b, _, reg := newNavBrain(t, n, Persona{}, from)
		b.walk.plan.Budget = farDropBudget
		if _, err := b.walk.plan.Route(toLoc(from), toLoc(dest)); !errors.Is(err, nav.ErrBudget) {
			t.Fatalf("cell search alone: %v, want ErrBudget", err)
		}
		now, prev := t0, from
		var segs int
		for i := 0; ; i++ {
			if i > 60 {
				t.Fatal("no arrival after 60 decisions")
			}
			a, st := b.goTo(dest, 0, walkFollow, now)
			if st == walkArrived {
				break
			}
			if st != walkWalking || a.kind != actMove {
				t.Fatalf("decision %d: status %v kind %v (%s), want a move", i, st, a.kind, b.walk.reason)
			}
			if ok, err := b.send(a, now); !ok || err != nil {
				t.Fatalf("send at %v: sent %v, %v", now.Sub(t0), ok, err)
			}
			origin, to := moveOrigin(a.payload), moveTarget(a.payload)
			if origin != prev {
				t.Fatalf("segment %d from %+v, want the previous end %+v", segs, origin, prev)
			}
			if !n.Engine.CanMove(int(origin.X), int(origin.Y), int(origin.Z), int(to.X), int(to.Y), int(to.Z)) {
				t.Fatalf("segment %d %+v -> %+v fails CanMove", segs, origin, to)
			}
			segs++
			b.handle([]Event{segAnswer(origin, to)}, now)
			now, prev = now.Add(segRun(b, now)+time.Millisecond), to
		}
		if prev.Z != dest.Z || horizontal2(prev, dest) > walkSlop*walkSlop {
			t.Errorf("ended at %+v, want %+v", prev, dest)
		}
		if s := reg.Collect(now); s.NavBlocked+s.NavDesync+s.NavDoorReplan != 0 || s.NavSegments != uint64(segs) {
			t.Errorf("counters blocked %d desync %d door %d segments %d, want none and %d segments", s.NavBlocked, s.NavDesync, s.NavDoorReplan, s.NavSegments, segs)
		}
	})
	t.Run("no drops refuses", func(t *testing.T) {
		b, _, _ := newNavBrain(t, n, Persona{}, from)
		if a, st := b.goTo(dest, 0, walkPlace, t0); st != walkUnreachable || a.kind != actHold || b.walk.reason != reasonNoRoute {
			t.Errorf("status %v kind %v reason %q, want unreachable, no route", st, a.kind, b.walk.reason)
		}
	})
}
