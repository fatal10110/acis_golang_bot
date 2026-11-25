package nav

import (
	"errors"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// routeDescent walks each segment of route over the floor the way CanMove
// follows it (each line cell landing on the layer nearest at or below the
// last floor plus block.CellIgnoreHeight) and returns the largest
// single-cell descent and the cell it lands on.
func routeDescent(e *engine.Engine, route []location.Location) (int, Cell) {
	worst, at := 0, Cell{}
	var cells []lineCell
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		cells = appendLineCells(cells[:0], a, b)
		z := a.Z
		for _, c := range cells[1:] {
			h, _, ok := e.NodeBelow(c.x, c.y, z+block.CellIgnoreHeight)
			if !ok {
				continue
			}
			if d := z - int(h); d > worst {
				worst, at = d, Cell{X: c.x, Y: c.y, Z: h}
			}
			z = int(h)
		}
	}
	return worst, at
}

var (
	wallBandOnce sync.Once
	wallBandEng  *engine.Engine
	wallBandH    *testHier
	wallBandErr  error
)

// wallBand is the wall band's engine and its hierarchy at 32-cell clusters,
// built once per test binary.
func wallBand(tb testing.TB) (*engine.Engine, *testHier) {
	tb.Helper()
	wallBandOnce.Do(func() {
		if wallBandEng, wallBandErr = buildEngine(geotest.WallBand()); wallBandErr == nil {
			wallBandH = buildHier(tb, wallBandEng, 32, nil, window(32, 8, 8, 11, 11))
		}
	})
	if wallBandErr != nil || wallBandH == nil {
		tb.Fatalf("wall band failed to build: %v", wallBandErr)
	}
	return wallBandEng, wallBandH
}

// TestRouteWallBandTakesRamp routes off the wall band (#504): a place route
// from the walkway to the ground below it, by the cell search and by the
// hierarchy both ways, goes down the ramp and never drops past dropBound,
// while the yard only a drop reaches is refused to a place route and taken,
// over the drop, by the one-way route follow uses.
func TestRouteWallBandTakesRamp(t *testing.T) {
	e, th := wallBand(t)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000}
	top, outside, yard := loc(geotest.WallBandTop), loc(geotest.WallBandOutside), loc(geotest.WallBandYard)
	rng := rand.New(rand.NewPCG(5, 6))
	routes := []struct {
		name     string
		from, to location.Location
		route    func(from, to location.Location) ([]location.Location, error)
	}{
		{"cell search down", top, outside, p.Route},
		{"cell search up", outside, top, p.Route},
		{"hierarchy down", top, outside, func(a, b location.Location) ([]location.Location, error) { return th.h.Route(p, a, b, nil) }},
		{"hierarchy up", outside, top, func(a, b location.Location) ([]location.Location, error) { return th.h.Route(p, a, b, nil) }},
		{"one-way down", top, outside, func(a, b location.Location) ([]location.Location, error) { return th.h.RouteOneWay(p, a, b, nil) }},
	}
	for _, tc := range routes {
		t.Run(tc.name, func(t *testing.T) {
			route, err := tc.route(tc.from, tc.to)
			if err != nil {
				t.Fatal(err)
			}
			checkRoute(t, p, rng, route, tc.from, tc.to)
			if d, at := routeDescent(e, route); d > dropBound {
				t.Errorf("route %v drops %d at %v, want the ramp", route, d, at)
			}
		})
	}

	ma, err := th.h.Locate(pointCell(geotest.WallBandTop), nil)
	if err != nil {
		t.Fatal(err)
	}
	mb, err := th.h.Locate(pointCell(geotest.WallBandYard), nil)
	if err != nil {
		t.Fatal(err)
	}
	if th.h.SameSCC(ma, mb) || !th.h.Reachable(ma, mb) {
		t.Fatalf("yard SameSCC %v, Reachable %v: want reachable one way", th.h.SameSCC(ma, mb), th.h.Reachable(ma, mb))
	}
	if _, err := th.h.Route(p, top, yard, nil); !errors.Is(err, ErrUnreachable) {
		t.Errorf("place route into the yard: %v, want ErrUnreachable", err)
	}
	route, err := th.h.RouteOneWay(p, top, yard, nil)
	if err != nil {
		t.Fatalf("one-way route into the yard: %v", err)
	}
	checkRoute(t, p, rng, route, top, yard)
	if d, _ := routeDescent(e, route); d != geotest.BandTop {
		t.Errorf("one-way route %v descends at most %d, want the drop of %d", route, d, geotest.BandTop)
	}
}
