package nav

import (
	"errors"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// The same-cluster scene, in region-local cells, at 64-cell clusters over
// cluster columns 2-4 and rows 2-5 (columns 128-319, rows 128-383). A wall
// at columns 224-225 runs from row 0 to row 370, so it cuts cluster (3, 3), cells
// 192-255, in two: its west half joins its east half only through the
// clusters below, around the wall's end. sameStart and sameGoal stand in
// that one cluster, on either side of the wall: the free cell search
// between them floods far more than the cluster's 4096 columns.
var (
	sameStart = geotest.Point{CellX: 210, CellY: 230, Z: geotest.Ground}
	sameGoal  = geotest.Point{CellX: 240, CellY: 230, Z: geotest.Ground}
)

var (
	sameOnce sync.Once
	sameEng  *engine.Engine
	sameH    *testHier
	sameErr  error
)

func sameScene(tb testing.TB) (*engine.Engine, *testHier) {
	tb.Helper()
	sameOnce.Do(func() {
		t := (&geotest.Terrain{}).Raise(224, 0, 225, 370, geotest.Wall).Raise(200, 205, 214, 206, geotest.Wall)
		if sameEng, sameErr = buildEngine(t); sameErr == nil {
			sameH = buildHier(tb, sameEng, 64, nil, window(64, 2, 2, 4, 5))
		}
	})
	if sameErr != nil || sameH == nil {
		tb.Fatalf("same-cluster scene failed to build: %v", sameErr)
	}
	return sameEng, sameH
}

// testSameCap is the cap of a route between two ends of one cluster that
// BOT_NAV_TEST_SAME_CAP sets on the test hierarchies, so that the suite
// runs under another cap than ClusterCells squared (#648); unset keeps it.
func testSameCap(tb testing.TB) int {
	v := os.Getenv("BOT_NAV_TEST_SAME_CAP")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		tb.Fatalf("BOT_NAV_TEST_SAME_CAP=%q: want a positive cell count", v)
	}
	return n
}

// endsQuery is the routeQuery Route builds from a to b.
func endsQuery(t *testing.T, p *Planner, h *Hierarchy, a, b geotest.Point) routeQuery {
	t.Helper()
	from, to := loc(a), loc(b)
	start, ok := p.Locate(from)
	if !ok {
		t.Fatal("start not walkable")
	}
	goal, ok := p.Locate(to)
	if !ok {
		t.Fatal("goal not walkable")
	}
	ma, err := h.Locate(start, nil)
	if err != nil {
		t.Fatal(err)
	}
	mb, err := h.Locate(goal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !h.SameSCC(ma, mb) {
		t.Fatal("want SameSCC ends")
	}
	from.Z, to.Z = int(start.Z), int(goal.Z)
	return routeQuery{a: ma, b: mb, start: start, goal: goal, from: from, to: to, span: sameSpan(ma)}
}

// TestRouteSameClusterPastCap routes between the two halves of the cut
// cluster with a budget one cell short of what the free search needs: the
// cell planner, which is the route of two ends of one cluster before #648,
// answers ErrBudget. Past the cap the route is seated on the west half's
// entrance nodes, runs around the wall's end through the cluster below,
// and fits the same budget. Distance keeps the straight line.
func TestRouteSameClusterPastCap(t *testing.T) {
	e, th := sameScene(t)
	h := th.h
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23}
	s := getScratch()
	defer putScratch(s)
	q := endsQuery(t, p, h, sameStart, sameGoal)
	if q.a.res.cluster != q.b.res.cluster {
		t.Fatal("want both ends in one cluster")
	}
	limit := h.sameClusterCap()
	_, free, err := p.search(s, q.start, q.goal, 1<<24, nil, anywhere)
	if err != nil {
		t.Fatal(err)
	}
	if free <= limit {
		t.Skipf("the free search needs %d cells, within the cap %d: the fixture needs a cap under it", free, limit)
	}
	p.Budget = free - 1
	from, to := loc(sameStart), loc(sameGoal)
	if _, err := p.Route(from, to); !errors.Is(err, ErrBudget) {
		t.Fatalf("free route: %v, want ErrBudget", err)
	}
	kept := h.sameCap
	h.sameCap = math.MaxInt // the free search alone: Route before #648
	_, err = h.Route(p, from, to, nil)
	h.sameCap = kept
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("Route with no cap: %v, want ErrBudget", err)
	}

	b := routeBudget{cells: p.Budget, nodes: abstractBudget}
	_, err = h.search(s, p, &q, nil, &b)
	t.Logf("the free search needs %d cells; the cap is %d; the seated route spends %d", free, limit, p.Budget-b.cells)
	if err != nil {
		t.Fatalf("seated search: %v", err)
	}
	if len(q.seat.starts) == 0 || len(s.apath) == 0 {
		t.Fatalf("seats %v, abstract path %v: want the start seated and a node path", q.seat.starts, s.apath)
	}
	wall := geoCell(224, 0, 0).X
	for _, n := range q.seat.starts {
		if c := th.g.nodeRep(int(n)); c.X >= wall {
			t.Errorf("start seated on node %d at %v, past the wall", n, c)
		}
	}

	route, err := h.Route(p, from, to, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	checkRoute(t, p, rand.New(rand.NewPCG(5, 6)), route, from, to)

	d, err := h.Distance(q.a, q.b)
	if err != nil || d != int(heuristic(q.a.cell, q.b.cell)) {
		t.Errorf("Distance = %d, %v; want the straight line %d", d, err, heuristic(q.a.cell, q.b.cell))
	}
}

// TestRouteSameClusterAsBefore checks the routes #648 keeps: two ends of
// one cluster whose free search fits the cap, or whose budget is at or
// under the cap, route exactly as the cell planner does, cell for cell, or
// fail as it does.
func TestRouteSameClusterAsBefore(t *testing.T) {
	e, th := sameScene(t)
	h := th.h
	h.searchOnly = true
	t.Cleanup(func() { h.searchOnly = false })
	limit := h.sameClusterCap()
	tests := []struct {
		name   string
		a, b   geotest.Point
		budget int
		want   error
	}{
		{"open ground", geotest.Point{CellX: 196, CellY: 196, Z: geotest.Ground}, geotest.Point{CellX: 214, CellY: 240, Z: geotest.Ground}, 1 << 20, nil},
		{"round the short wall", geotest.Point{CellX: 207, CellY: 200, Z: geotest.Ground}, geotest.Point{CellX: 207, CellY: 212, Z: geotest.Ground}, 1 << 20, nil},
		{"across the wall, budget at the cap", sameStart, sameGoal, limit, ErrBudget},
		{"across the wall, budget under the cap", sameStart, sameGoal, limit / 2, ErrBudget},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: tt.budget}
			s := getScratch()
			defer putScratch(s)
			q := endsQuery(t, p, h, tt.a, tt.b)
			if q.a.res.cluster != q.b.res.cluster {
				t.Fatal("want both ends in one cluster")
			}
			_, free, err := p.search(s, q.start, q.goal, 1<<24, nil, anywhere)
			switch {
			case err != nil:
				t.Fatal(err)
			case tt.want == nil && free > limit:
				t.Fatalf("free search: %d cells, want a route within the cap %d", free, limit)
			case tt.want != nil && free <= tt.budget:
				t.Skipf("the free search needs %d cells, within the budget %d: the row needs a cap under it", free, tt.budget)
			}
			from, to := loc(tt.a), loc(tt.b)
			want, wantErr := p.Route(from, to)
			got, err := h.Route(p, from, to, nil)
			if !errors.Is(wantErr, tt.want) || (tt.want == nil) != (wantErr == nil) {
				t.Fatalf("cell planner: %v, want %v", wantErr, tt.want)
			}
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) || !slices.Equal(got, want) {
				t.Errorf("Route = %v, %v; the cell planner's %v, %v", got, err, want, wantErr)
			}
		})
	}
}
