package nav

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// checkRoute asserts the route invariants: it runs from from to to on their
// floors, every segment is at most navSegment long and passes CanMove and
// StepChainOK, every intermediate waypoint is a clickable cell centre, and a
// few random step sequences of the copied step chain walk each segment.
func checkRoute(t *testing.T, p *Planner, rng *rand.Rand, route []location.Location, from, to location.Location) {
	t.Helper()
	e := p.Engine
	if len(route) < 2 || route[0] != floor(e, from) || route[len(route)-1] != floor(e, to) {
		t.Fatalf("route %v does not run from %v to %v", route, floor(e, from), floor(e, to))
	}
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		if d := hypot(b.X-a.X, b.Y-a.Y); d > navSegment {
			t.Fatalf("segment %d %v -> %v is %.0f long", i, a, b, d)
		}
		if !e.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
			t.Fatalf("segment %d %v -> %v fails CanMove", i, a, b)
		}
		if !p.StepChainOK(a, b) {
			t.Fatalf("segment %d %v -> %v fails the step-chain check", i, a, b)
		}
		if w, ok := walkSteps(e, rng, a, b, p.Lmax, 3); !ok {
			t.Fatalf("segment %d %v -> %v: a step sequence ends %+v", i, a, b, w)
		}
		if i < len(route)-1 {
			c, ok := p.Locate(b)
			if !ok || c.World() != b || !p.clickable(c, &[block.MaxLayers]block.Cell{}) {
				t.Fatalf("waypoint %v is not a clickable cell centre", b)
			}
		}
	}
}

func TestRoute(t *testing.T) {
	tests := []struct {
		name     string
		from, to location.Location
		budget   int
		err      error
	}{
		{"along the corridor", loc(geotest.CorridorWest), loc(geotest.CorridorEast), 40000, nil},
		{"out of the U and around it", loc(geotest.UInside), loc(geotest.UBehind), 40000, nil},
		{"drop off the ledge", loc(geotest.LedgeHigh), loc(geotest.LedgeLow), 40000, nil},
		{"jump into the pit", loc(geotest.PitOutside), loc(geotest.PitInside), 40000, nil},
		{"across the bridge deck", loc(geotest.BridgeWestDeck), loc(geotest.BridgeEastDeck), 40000, nil},
		{"under the bridge", loc(geotest.BridgeWestGround), loc(geotest.BridgeEastGround), 40000, nil},
		{"up the stair", loc(StairLow), loc(StairDeck), 40000, nil},
		{"long open run, split into segments", loc(Open), at(Open, 5200, 2100, 0), 40000, nil},
		{"same cell", loc(Open), at(Open, 3, 4, 0), 10, nil},
		{"out of the pit", loc(geotest.PitInside), loc(geotest.PitOutside), 40000, ErrUnreachable},
		{"out of the pocket", loc(geotest.PocketInside), loc(geotest.PocketOutside), 40000, ErrUnreachable},
		{"onto the ledge: budget", loc(geotest.LedgeLow), loc(geotest.LedgeHigh), 3000, ErrBudget},
		{"no budget", loc(geotest.CorridorWest), loc(geotest.CorridorEast), 0, ErrBudget},
		{"into an unloaded region", loc(Open), at(geotest.Point{CellX: -3, CellY: 100}, 0, 0, 0), 40000, ErrNotWalkable},
		{"to ground under a low layer", loc(Open), loc(StackedGround), 40000, ErrUnclickable},
	}
	rng := rand.New(rand.NewPCG(3, 5))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testPlanner(t)
			p.Budget = tt.budget
			route, err := p.Route(tt.from, tt.to)
			if !errors.Is(err, tt.err) {
				t.Fatalf("Route = %v, %v; want error %v", route, err, tt.err)
			}
			if err == nil {
				checkRoute(t, p, rng, route, tt.from, tt.to)
			}
		})
	}
}

// TestRouteProperty plans between random pairs around every scene. Each
// answer is a route that keeps the invariants or one of the distinct errors.
func TestRouteProperty(t *testing.T) {
	p := testPlanner(t)
	p.Budget = 8000
	rng := rand.New(rand.NewPCG(13, 17))
	scenes := []geotest.Point{
		geotest.CorridorWest, geotest.AboveCorridor, geotest.UInside, geotest.LedgeHigh, geotest.LedgeLow,
		geotest.PitOutside, geotest.PitInside, geotest.BridgeWestDeck, geotest.BridgeEastGround,
		geotest.PocketInside, geotest.PocketOutside, StairLow, StairDeck, Open,
	}
	counts := map[error]int{}
	for i := range 80 {
		sc := scenes[i%len(scenes)]
		from := at(sc, rng.IntN(601)-300, rng.IntN(601)-300, rng.IntN(2)*160)
		to := at(sc, rng.IntN(601)-300, rng.IntN(601)-300, rng.IntN(2)*160)
		route, err := p.Route(from, to)
		counts[err]++
		switch {
		case err == nil:
			checkRoute(t, p, rng, route, from, to)
		case errors.Is(err, ErrBudget), errors.Is(err, ErrUnreachable), errors.Is(err, ErrNotWalkable), errors.Is(err, ErrUnclickable):
		default:
			t.Fatalf("Route(%v, %v): unexpected error %v", from, to, err)
		}
	}
	t.Logf("results: %v", counts)
	if counts[nil] == 0 {
		t.Fatal("no route found")
	}
}

// TestRouteReusesScratch: once its buffers have grown, a route allocates
// nothing on the scratch Route takes from the pool (Route adds only its
// result). The scratch is held here, not taken from the free list, so no
// other query shares it.
func TestRouteReusesScratch(t *testing.T) {
	p := testPlanner(t)
	s := getScratch()
	defer putScratch(s)
	from, to := loc(geotest.UInside), loc(geotest.UBehind)
	if _, err := p.route(s, from, to); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(20, func() {
		if _, err := p.route(s, from, to); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("route allocations = %.1f, want 0", allocs)
	}
}
