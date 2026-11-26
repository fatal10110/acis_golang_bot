package nav

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// The room scene, in region-local cells, at 64-cell clusters over cluster
// columns 2-4 and rows 2-4 (cells 128-319): a walled room, columns and rows
// 210-240, inside cluster (3, 3), cells 192-255, so no entrance node is in
// it. Its only way in is door 7's gap in its west wall, rows 223-226.
var (
	roomInside  = geotest.Point{CellX: 225, CellY: 225, Z: geotest.Ground}
	roomOutside = geotest.Point{CellX: 200, CellY: 196, Z: geotest.Ground} // the room's cluster
	roomAway    = geotest.Point{CellX: 150, CellY: 225, Z: geotest.Ground} // the cluster west of it
)

// roomScene builds the room scene with door 7 closed in the engine, as the
// door table closes it, or open.
func roomScene(tb testing.TB, shut bool) (*engine.Engine, *testHier) {
	tb.Helper()
	t := &geotest.Terrain{}
	t.Raise(210, 210, 240, 210, geotest.Wall).Raise(210, 240, 240, 240, geotest.Wall).Raise(240, 210, 240, 240, geotest.Wall).
		Raise(210, 210, 210, 222, geotest.Wall).Raise(210, 227, 210, 240, geotest.Wall)
	e, err := buildEngine(t)
	if err != nil {
		tb.Fatal(err)
	}
	doors := []*door.Template{doorAt(doorID, 210, 223, 226)}
	th := buildHier(tb, e, 64, doors, window(64, 2, 2, 4, 4))
	if shut {
		closeDoor(tb, e, doors[0])
	}
	return e, th
}

// TestRouteDoorGoal checks a goal behind a closed door (#649): the room's
// goal answers ErrDoor, from another cluster and from its own, where it
// answered ErrBudget after the free search flooded the budget away; the
// same room with the door open routes; and a planner that avoids a column
// keeps the free search, which floods.
func TestRouteDoorGoal(t *testing.T) {
	closedDoor := func(d int) bool { return d == doorID }
	tests := []struct {
		name    string
		from    geotest.Point
		shut    bool
		avoided bool // the planner avoids a column far off the room
		want    error
	}{
		{"closed, from another cluster", roomAway, true, false, ErrDoor},
		{"closed, from the room's cluster", roomOutside, true, false, ErrDoor},
		{"open, from another cluster", roomAway, false, false, nil},
		{"open, from the room's cluster", roomOutside, false, false, nil},
		{"closed, a column avoided", roomAway, true, true, ErrBudget},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, th := roomScene(t, tt.shut)
			// The cap of two ends of one cluster comes out of the budget.
			p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: th.h.sameClusterCap() + 40000, MaxSegment: 300}
			if tt.avoided {
				far := loc(geotest.Point{CellX: 1500, CellY: 1500, Z: geotest.Ground})
				p.Avoid.AddLine(far, loc(geotest.Point{CellX: 1510, CellY: 1500, Z: geotest.Ground}))
				if p.Avoid.Len() == 0 {
					t.Fatal("no column avoided")
				}
			}
			var closed func(int) bool
			if tt.shut {
				closed = closedDoor
			}
			route, err := th.h.Route(p, loc(tt.from), loc(roomInside), closed)
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) {
				t.Fatalf("Route = %v, want %v", err, tt.want)
			}
			if err == nil && len(route) < 2 {
				t.Fatalf("route %v", route)
			}
		})
	}
}

// TestLegDoorCheck runs the legs' static check on their own: lastLeg checks
// only with no edge avoided; firstLeg checks for two ends of one cluster
// only when the start reaches no node (the start in the room), and never
// for a start it seats.
func TestLegDoorCheck(t *testing.T) {
	e, th := roomScene(t, true)
	h, g := th.h, th.g
	s := getScratch()
	defer putScratch(s)
	newPlanner := func() *Planner { return &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23} }
	const budget = 40000

	t.Run("lastLeg", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			avoid map[edgeKey]struct{}
			want  error
		}{
			{"no edge avoided", nil, ErrDoor},
			{"an edge avoided", map[edgeKey]struct{}{{}: {}}, ErrBudget},
		} {
			p := newPlanner()
			q := endsQuery(t, p, h, roomAway, roomInside)
			first, end := g.clusterNodes(int(q.b.res.cluster))
			node := -1
			for n := first; n < end; n++ {
				if g.nodeSCC(n) == q.b.SCC() {
					node = n
					break
				}
			}
			if node < 0 {
				t.Fatal("no node of the goal's SCC in its cluster")
			}
			_, _, err := h.lastLeg(s, p, &q, g.nodeRep(node), q.goal, budget, tt.avoid)
			if !errors.Is(err, tt.want) {
				t.Errorf("%s: lastLeg = %v, want %v", tt.name, err, tt.want)
			}
			if q.seat.goals != nil {
				t.Errorf("%s: goal seated on %v, want no seat", tt.name, q.seat.goals)
			}
		}
	})
	t.Run("firstLeg", func(t *testing.T) {
		for _, tt := range []struct {
			name       string
			from, to   geotest.Point
			want       error
			wantSeated bool
		}{
			{"start in the room", roomInside, roomOutside, ErrDoor, false},
			{"start seated", roomOutside, roomInside, errReseat, true},
		} {
			p := newPlanner()
			q := endsQuery(t, p, h, tt.from, tt.to)
			if q.a.res.cluster != q.b.res.cluster {
				t.Fatal("want both ends in one cluster")
			}
			_, _, err := h.firstLeg(s, p, &q, q.start, q.goal, budget, nil)
			if !errors.Is(err, tt.want) {
				t.Errorf("%s: firstLeg = %v, want %v", tt.name, err, tt.want)
			}
			if seated := q.seat.starts != nil; seated != tt.wantSeated {
				t.Errorf("%s: start seats %v, want seated %v", tt.name, q.seat.starts, tt.wantSeated)
			}
		}
	})
}
