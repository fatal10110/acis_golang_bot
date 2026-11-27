package nav

import (
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// The ledge, a scene of its own in region-local cells: a plateau
// ledgeHigh high over columns 100-119 and rows 100-139, and a shelf
// ledgeShelf high along its east edge in rows 100-109 only. The plateau
// meets the ground below it by drops alone: 136 onto the shelf then 160 off
// it in the north, 296 straight down further south. The cell search weighs
// the two small drops over the high one; a straight line from the plateau to
// the ground east of it falls the whole 296 off the plateau's east edge.
const (
	ledgeHigh  = 296
	ledgeShelf = 160
)

var (
	ledgeTop    = geotest.Point{CellX: 110, CellY: 130, Z: ledgeHigh}
	ledgeGround = geotest.Point{CellX: 150, CellY: 130, Z: geotest.Ground}
)

// TestRouteLineDropNoHigher pins the drop rule of the string pull to the
// cell path's heights (#575): a segment may take a drop only as high as the
// largest drop of the cell path it stands for. On the ledge the cell path
// drops 136 and 160 by the shelf; the line from the plateau to the ground,
// which CanMove passes, drops 296, and the route keeps to the shelf.
func TestRouteLineDropNoHigher(t *testing.T) {
	e, err := buildEngine((&geotest.Terrain{}).Raise(100, 100, 119, 139, ledgeHigh).Raise(120, 100, 129, 109, ledgeShelf))
	if err != nil {
		t.Fatal(err)
	}
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000}
	from, to := floor(e, loc(ledgeTop)), floor(e, loc(ledgeGround))
	if !e.CanMove(from.X, from.Y, from.Z, to.X, to.Y, to.Z) {
		t.Fatal("CanMove refuses the straight line off the plateau: the case tests no line drop")
	}
	if d, _ := routeDescent(e, []location.Location{from, to}); d != ledgeHigh {
		t.Fatalf("straight line descends at most %d, want %d", d, ledgeHigh)
	}
	route, err := p.Route(from, to)
	if err != nil {
		t.Fatal(err)
	}
	checkRoute(t, p, rand.New(rand.NewPCG(5, 75)), route, from, to)
	if d, at := routeDescent(e, route); d > ledgeShelf {
		t.Errorf("route %v drops %d at %v, want at most the shelf's %d", route, d, at, ledgeShelf)
	}
}

// borderLedge is the border ledge at 64-cell clusters, in region-local
// cells: in the west cluster (columns 128-191, rows 128-191) a plateau
// ledgeStep high over columns 140-191 and rows 128-159, along the border
// with the east cluster, whose ground is flat. A ramp down its west end,
// columns 128-139, joins it to the ground south of it inside the cluster,
// so the west cluster's border cells are one entrance node, its
// representative on the plateau's south edge; the east cluster's are one
// node on the ground. The crossings between the two nodes include flat ones
// south of the plateau, so the straight line between the representatives
// is one cell, while the cell path between them drops off the plateau: the
// way down the ramp is longer than a drop costs.
const ledgeStep = 104

func borderLedge(tb testing.TB) *testHier {
	tb.Helper()
	t := (&geotest.Terrain{}).Raise(140, 128, 191, 159, ledgeStep)
	for x := 128; x < 140; x++ {
		t.Raise(x, 128, x, 159, int16(8*(x-127)))
	}
	e, err := buildEngine(t)
	if err != nil {
		tb.Fatal(err)
	}
	return buildHier(tb, e, 64, nil, window(64, 2, 2, 3, 2))
}

// checkCrossingCosts checks that every crossing edge of th costs the cell
// path between its two nodes' representatives inside the two clusters' box
// (#575), and returns how many cost more than their straight line.
func checkCrossingCosts(t *testing.T, th *testHier) int {
	t.Helper()
	if th.stats.BoundedCrossings != 0 {
		t.Errorf("%d crossings costed by a bound, want none", th.stats.BoundedCrossings)
	}
	g := th.g
	p := Planner{Engine: th.cfg.Engine.Static(), Water: th.cfg.Water}.withCovered(int32(g.CoveredCost()))
	s := new(scratch)
	size := g.Key().ClusterCells
	box := func(n int) cellBox {
		x0, y0 := g.clusterOrigin(int(g.nodeCluster(n)))
		return cellBox{x0, y0, x0 + size, y0 + size}
	}
	more := 0
	for n := range g.Nodes() {
		first, end := g.nodeEdges(n)
		for i := first; i < end; i++ {
			e := g.edge(i)
			if g.nodeCluster(e.to) == g.nodeCluster(n) {
				continue
			}
			a, c := g.nodeRep(n), g.nodeRep(e.to)
			ba, bc := box(n), box(e.to)
			both := cellBox{min(ba.x0, bc.x0), min(ba.y0, bc.y0), max(ba.x1, bc.x1), max(ba.y1, bc.y1)}
			if _, _, err := p.search(s, a, c, crossBudget, nil, both); err != nil {
				t.Errorf("crossing %d -> %d: no cell path between %v and %v: %v", n, e.to, a, c, err)
				continue
			}
			want := uint32(s.goalCost(c))
			if e.open != want {
				t.Errorf("crossing %d -> %d (%v -> %v) costs %d, want its cell path's %d", n, e.to, a, c, e.open, want)
			}
			if e.free != noCost && e.free < want {
				t.Errorf("crossing %d -> %d costs %d crossing no door, under its cell path's %d", n, e.to, e.free, want)
			}
			if want > uint32(max(costCardinal, heuristic(a, c))) {
				more++
			}
		}
	}
	return more
}

// TestCrossingCostsCellPath pins the crossing edge costs of the build to
// the cell path between the two nodes' representatives (#575): over the
// fixture scenes, and on the border ledge, where the straight line misses
// the drop off the plateau the cell path takes.
func TestCrossingCostsCellPath(t *testing.T) {
	t.Run("scenes", func(t *testing.T) {
		if more := checkCrossingCosts(t, hierScenes32(t)); more == 0 {
			t.Error("no crossing costs more than its straight line: the scenes test nothing")
		}
	})
	t.Run("border ledge", func(t *testing.T) {
		th := borderLedge(t)
		checkCrossingCosts(t, th)
		g := th.g
		found := false
		for n := range g.Nodes() {
			first, end := g.nodeEdges(n)
			for i := first; i < end; i++ {
				e := g.edge(i)
				a, c := g.nodeRep(n), g.nodeRep(e.to)
				if g.nodeCluster(e.to) == g.nodeCluster(n) || a.Z != ledgeStep || c.Z != geotest.Ground {
					continue
				}
				found = true
				if heuristic(a, c) > costDiagonal {
					t.Errorf("representatives %v and %v are not neighbours: the case tests no hidden drop", a, c)
				}
				if e.open < costDrop {
					t.Errorf("crossing %v -> %v costs %d, under the drop's %d", a, c, e.open, costDrop)
				}
			}
		}
		if !found {
			t.Fatal("no crossing from the plateau's representative to the ground's")
		}
	})
}
