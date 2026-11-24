package nav

import (
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestMaxSegment routes with and without a segment cap through every
// route entry point: uncapped, some segment is longer than the cap; capped,
// none is, and the route keeps the route invariants (#306).
func TestMaxSegment(t *testing.T) {
	const capped = 500
	tests := []struct {
		name     string
		from, to location.Location
		// planner is the planner the case routes with, budget and all.
		planner func(testing.TB) *Planner
		route   func(t *testing.T, p *Planner, from, to location.Location) ([]location.Location, error)
	}{
		{
			name: "cell search", from: loc(Open), to: at(Open, 5200, 2100, 0),
			planner: testPlanner,
			route: func(_ *testing.T, p *Planner, from, to location.Location) ([]location.Location, error) {
				return p.Route(from, to)
			},
		},
		{
			name: "hierarchy", from: loc(geotest.PitOutside), to: loc(geotest.BridgeWestGround),
			planner: testPlanner,
			route: func(t *testing.T, p *Planner, from, to location.Location) ([]location.Location, error) {
				return hierScenes256(t).h.Route(p, from, to, nil)
			},
		},
		{
			name: "one way off the far drop", from: loc(geotest.FarDropHigh), to: loc(geotest.FarDropLow),
			planner: func(t testing.TB) *Planner {
				e, _ := farDrop(t)
				return &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: farDropBudget}
			},
			route: func(t *testing.T, p *Planner, from, to location.Location) ([]location.Location, error) {
				_, th := farDrop(t)
				return th.h.RouteOneWay(p, from, to, nil)
			},
		},
	}
	rng := rand.New(rand.NewPCG(30, 6))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.planner(t)
			route, err := tt.route(t, p, tt.from, tt.to)
			if err != nil {
				t.Fatalf("uncapped: %v", err)
			}
			if got := longest(route); got <= capped {
				t.Fatalf("uncapped: longest segment %.0f, want one over %d for the cap to matter", got, capped)
			}
			p.MaxSegment = capped
			route, err = tt.route(t, p, tt.from, tt.to)
			if err != nil {
				t.Fatalf("capped at %d: %v", capped, err)
			}
			if got := longest(route); got > capped {
				t.Fatalf("capped at %d: longest segment %.0f", capped, got)
			}
			checkRoute(t, p, rng, route, tt.from, tt.to)
		})
	}
}

// TestSegmentCap: zero, or a cap past navSegment, is navSegment.
func TestSegmentCap(t *testing.T) {
	for _, tt := range []struct{ max, want float64 }{{0, navSegment}, {500, 500}, {navSegment + 1, navSegment}} {
		if got := (&Planner{MaxSegment: tt.max}).segmentCap(); got != tt.want {
			t.Errorf("MaxSegment %.0f: cap %.0f, want %.0f", tt.max, got, tt.want)
		}
	}
}

// longest is the longest horizontal segment of route.
func longest(route []location.Location) float64 {
	var m float64
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		m = max(m, hypot(b.X-a.X, b.Y-a.Y))
	}
	return m
}
