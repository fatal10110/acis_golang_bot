package nav

import (
	"errors"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// The split scene, in region-local cells, at 32-cell clusters over cluster
// columns 2-4 and rows 2-4 (cells 64-159). A wall at columns 80-81 runs
// from row 0 to row 130, so it cuts the start's cluster (columns 64-95,
// rows 64-95) in two: its west half joins its east half only through the
// clusters below, around the wall's end. A second wall at row 76 shuts a
// pocket off the west half; the pocket opens only north, out of the
// hierarchy's window.
var (
	splitStart  = geotest.Point{CellX: 70, CellY: 85, Z: geotest.Ground}
	splitGoal   = geotest.Point{CellX: 150, CellY: 85, Z: geotest.Ground}
	splitPocket = geotest.Point{CellX: 70, CellY: 70, Z: geotest.Ground}
)

var (
	splitOnce sync.Once
	splitEng  *engine.Engine
	splitH    *testHier
	splitErr  error
)

func splitScene(tb testing.TB) (*engine.Engine, *testHier) {
	tb.Helper()
	splitOnce.Do(func() {
		t := (&geotest.Terrain{}).Raise(80, 0, 81, 130, geotest.Wall).Raise(64, 76, 79, 77, geotest.Wall)
		if splitEng, splitErr = buildEngine(t); splitErr == nil {
			splitH = buildHier(tb, splitEng, 32, nil, window(32, 2, 2, 4, 4))
		}
	})
	if splitErr != nil || splitH == nil {
		tb.Fatalf("split scene failed to build: %v", splitErr)
	}
	return splitEng, splitH
}

// splitQuery is the routeQuery Route builds from splitStart to splitGoal.
func splitQuery(t *testing.T, p *Planner, h *Hierarchy) routeQuery {
	t.Helper()
	from, to := loc(splitStart), loc(splitGoal)
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
	if !h.SameSCC(ma, mb) || ma.res.cluster == mb.res.cluster {
		t.Fatal("want SameSCC ends in two clusters")
	}
	from.Z, to.Z = int(start.Z), int(goal.Z)
	return routeQuery{a: ma, b: mb, start: start, goal: goal, from: from, to: to, span: sameSpan(ma)}
}

// TestRouteSeatsStart routes from the west half of the split cluster to its
// east. The abstract search left free starts on a node past the wall, which
// the start joins only around the wall's end: that first leg alone spends
// the whole budget. Seated, the route starts on a node of the west half and
// walks within the same budget.
func TestRouteSeatsStart(t *testing.T) {
	e, th := splitScene(t)
	h, g := th.h, th.g
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 1 << 20}
	wall := geoCell(80, 0, 0).X
	s := getScratch()
	defer putScratch(s)

	q := splitQuery(t, p, h)
	if _, _, err := h.abstractSearch(s, q.a, q.b, &q.span, nil, abstractBudget, &seating{}); err != nil {
		t.Fatal(err)
	}
	free := g.nodeRep(int(s.apath[0]))
	if free.X < wall {
		t.Fatalf("free abstract search starts on %v, west of the wall", free)
	}
	_, budget, err := p.search(s, q.start, free, p.Budget, nil, anywhere)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("the free first leg spends %d cells", budget)

	unseated := splitQuery(t, p, h)
	unseated.seat = seating{startDone: true, goalDone: true}
	if _, err := h.search(s, p, &unseated, nil, &routeBudget{cells: budget, nodes: abstractBudget}); !errors.Is(err, ErrBudget) {
		t.Fatalf("unseated route: %v, want ErrBudget", err)
	}

	b := routeBudget{cells: budget, nodes: abstractBudget}
	if _, err := h.search(s, p, &q, nil, &b); err != nil {
		t.Fatalf("seated route: %v", err)
	}
	t.Logf("the seated route spends %d cells", budget-b.cells)
	if len(q.seat.starts) == 0 || q.seat.goals != nil {
		t.Fatalf("seats: starts %v, goals %v; want starts only", q.seat.starts, q.seat.goals)
	}
	for _, n := range q.seat.starts {
		if c := g.nodeRep(int(n)); c.X >= wall {
			t.Errorf("start seated on node %d at %v, past the wall", n, c)
		}
	}
	if !slices.Contains(q.seat.starts, s.apath[0]) {
		t.Errorf("route starts on node %d, not a seat %v", s.apath[0], q.seat.starts)
	}

	p.Budget = budget
	from, to := loc(splitStart), loc(splitGoal)
	route, err := h.Route(p, from, to, nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	checkRoute(t, p, rand.New(rand.NewPCG(3, 4)), route, from, to)
}

// TestLegSeatFallback checks the ends that reach no entrance node inside
// their cluster: the pocket joins the split cluster's west half only out
// of the hierarchy's window, so the leg keeps the free cell search and
// seats nothing.
func TestLegSeatFallback(t *testing.T) {
	e, th := splitScene(t)
	h, g := th.h, th.g
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 1 << 20}
	pocket := pointCell(splitPocket)
	s := getScratch()
	defer putScratch(s)
	q := splitQuery(t, p, h)
	first, end := g.clusterNodes(int(q.a.res.cluster))
	node := Cell{}
	for n := first; n < end; n++ {
		if g.nodeSCC(n) == q.a.SCC() && g.nodeRep(n).X < geoCell(80, 0, 0).X {
			node = g.nodeRep(n)
			break
		}
	}
	if node == (Cell{}) {
		t.Fatal("no node west of the wall")
	}
	if _, _, err := p.search(s, pocket, node, p.Budget, nil, h.clusterBox(pocket)); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("pocket to %v inside the cluster: %v, want ErrUnreachable", node, err)
	}

	tests := []struct {
		name string
		leg  func(q *routeQuery) ([]Cell, int, error)
		a, c Cell
	}{
		{"first leg", func(q *routeQuery) ([]Cell, int, error) {
			return h.firstLeg(s, p, q, pocket, node, p.Budget, nil)
		}, pocket, node},
		{"last leg", func(q *routeQuery) ([]Cell, int, error) {
			q.b = q.a
			return h.lastLeg(s, p, q, node, pocket, p.Budget, nil)
		}, node, pocket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := splitQuery(t, p, h)
			cells, _, err := tt.leg(&q)
			if err != nil {
				t.Fatalf("leg: %v", err)
			}
			if cells[0] != tt.a || cells[len(cells)-1] != tt.c {
				t.Errorf("leg runs %v to %v, want %v to %v", cells[0], cells[len(cells)-1], tt.a, tt.c)
			}
			if q.seat.starts != nil || q.seat.goals != nil || !q.seat.startDone && !q.seat.goalDone {
				t.Errorf("seat %+v, want checked and none seated", q.seat)
			}
		})
	}
}
