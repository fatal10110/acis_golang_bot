package nav

import (
	"errors"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// farDropBudget is a cell budget the far drop's hierarchy route keeps well
// under and its cell search alone runs out of, as a far leader past a drop
// runs a follower's walkBudget out on real ground (#231).
const farDropBudget = 20000

var (
	farDropOnce sync.Once
	farDropEng  *engine.Engine
	farDropH    *testHier
	farDropErr  error
)

// farDrop is the far drop's engine and its hierarchy at the default
// cluster side, built once per test binary.
func farDrop(tb testing.TB) (*engine.Engine, *testHier) {
	tb.Helper()
	farDropOnce.Do(func() {
		if farDropEng, farDropErr = buildEngine(geotest.FarDrop()); farDropErr == nil {
			farDropH = buildHier(tb, farDropEng, DefaultClusterCells, nil, window(DefaultClusterCells, 1, 1, 3, 2))
		}
	})
	if farDropErr != nil || farDropH == nil {
		tb.Fatalf("far drop failed to build: %v", farDropErr)
	}
	return farDropEng, farDropH
}

// TestRouteOneWayFarDrop routes off the far drop's plateau to the ground
// past the wall: the cell search alone and the default Route fail, the
// one-way route walks within the budget, and nothing routes back up.
func TestRouteOneWayFarDrop(t *testing.T) {
	e, th := farDrop(t)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: farDropBudget}
	high, low := loc(geotest.FarDropHigh), loc(geotest.FarDropLow)
	ma, err := th.h.Locate(pointCell(geotest.FarDropHigh), nil)
	if err != nil {
		t.Fatal(err)
	}
	mb, err := th.h.Locate(pointCell(geotest.FarDropLow), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ma.Kind() != InSCC || mb.Kind() != InSCC || th.h.SameSCC(ma, mb) || !th.h.Reachable(ma, mb) {
		t.Fatalf("kinds %v %v, SameSCC %v: want two SCCs, the ground reachable one way", ma.Kind(), mb.Kind(), th.h.SameSCC(ma, mb))
	}
	if _, err := p.Route(high, low); !errors.Is(err, ErrBudget) {
		t.Fatalf("cell search alone: %v, want ErrBudget", err)
	}
	if _, err := th.h.Route(p, high, low, nil); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("default Route: %v, want ErrUnreachable", err)
	}
	route, err := th.h.RouteOneWay(p, high, low, nil)
	if err != nil {
		t.Fatalf("one-way route: %v", err)
	}
	checkRoute(t, p, rand.New(rand.NewPCG(7, 8)), route, high, low)
	if _, err := th.h.RouteOneWay(p, low, high, nil); !errors.Is(err, ErrUnreachable) {
		t.Errorf("back onto the plateau: %v, want ErrUnreachable", err)
	}
}

// TestRouteOneWaySucceedsExactlyWhenReachable routes one way between random
// cells of the door-free scene windows and the drops of the fixture scenes:
// a route is found exactly when the goal is Reachable from the start, and
// every route keeps the route invariants.
func TestRouteOneWaySucceedsExactlyWhenReachable(t *testing.T) {
	th := hierScenes32(t)
	p := testPlanner(t)
	p.Budget = 400000
	keys := append(window(32, 24, 8, 49, 9), window(32, 8, 8, 17, 9)...)
	cells := clusterCells(testEngine(t), 32, keys)
	rng := rand.New(rand.NewPCG(9, 10))
	pairs := [][2]Cell{
		{pointCell(geotest.LedgeHigh), pointCell(geotest.LedgeLow)},
		{pointCell(geotest.LedgeLow), pointCell(geotest.LedgeHigh)},
		{pointCell(geotest.PitOutside), pointCell(geotest.PitInside)},
		{pointCell(geotest.PitInside), pointCell(geotest.PitOutside)},
		{pointCell(geotest.BridgeWestDeck), pointCell(geotest.PitOutside)},
		{pointCell(geotest.UInside), pointCell(geotest.UBehind)},
	}
	for len(pairs) < 60 {
		pairs = append(pairs, [2]Cell{cells[rng.IntN(len(cells))], cells[rng.IntN(len(cells))]})
	}
	oneWay := 0
	for _, pr := range pairs {
		var buf [block.MaxLayers]block.Cell
		if !p.clickable(pr[1], &buf) {
			continue
		}
		ma, err := th.h.Locate(pr[0], nil)
		if err != nil {
			t.Fatal(err)
		}
		mb, err := th.h.Locate(pr[1], nil)
		if err != nil {
			t.Fatal(err)
		}
		from, to := pr[0].World(), pr[1].World()
		route, err := th.h.RouteOneWay(p, from, to, nil)
		if want := th.h.Reachable(ma, mb); (err == nil) != want {
			t.Errorf("%v -> %v: route error %v, Reachable %v", pr[0], pr[1], err, want)
			continue
		}
		if err != nil {
			if !errors.Is(err, ErrUnreachable) {
				t.Errorf("%v -> %v: %v, want ErrUnreachable", pr[0], pr[1], err)
			}
			continue
		}
		if !th.h.SameSCC(ma, mb) {
			oneWay++
		}
		checkRoute(t, p, rng, route, from, to)
	}
	if oneWay < 3 {
		t.Errorf("only %d one-way pairs routed: the sample says little", oneWay)
	}
}
