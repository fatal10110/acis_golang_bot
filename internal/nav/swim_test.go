package nav

import (
	"context"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestWaterSpanExit checks the swim line's water tests: Span (where a
// straight line first enters and last leaves water) and exit (how far a
// swimmer starting in water stays in it), touching zones chained.
func TestWaterSpanExit(t *testing.T) {
	box := WaterBox{X1: 100, Y1: -100, X2: 200, Y2: 100, MinZ: -50, MaxZ: 50}
	next := WaterBox{X1: 201, Y1: -100, X2: 300, Y2: 100, MinZ: -50, MaxZ: 50}
	l := func(x, z int) location.Location { return location.Location{X: x, Z: z} }
	span := []struct {
		name   string
		boxes  []WaterBox
		a, b   location.Location
		t0, t1 float64
		ok     bool
	}{
		{"through", []WaterBox{box}, l(0, 0), l(400, 0), 0.25, 0.5, true},
		{"misses above", []WaterBox{box}, l(0, 60), l(400, 60), 0, 0, false},
		{"inside", []WaterBox{box}, l(120, 0), l(180, 0), 0, 1, true},
		{"two zones", []WaterBox{box, next}, l(0, 0), l(400, 0), 0.25, 0.75, true},
		{"rising out", []WaterBox{box}, l(150, 0), l(150, 100), 0, 0.5, true},
	}
	for _, tt := range span {
		t0, t1, ok := NewWater(tt.boxes).Span(tt.a, tt.b)
		if ok != tt.ok || (ok && (t0 != tt.t0 || t1 != tt.t1)) {
			t.Errorf("Span %s = %v, %v, %v; want %v, %v, %v", tt.name, t0, t1, ok, tt.t0, tt.t1, tt.ok)
		}
	}
	exit := []struct {
		name  string
		boxes []WaterBox
		a, b  location.Location
		want  float64
	}{
		{"stays in", []WaterBox{box}, l(120, 0), l(180, -40), 1},
		{"leaves east", []WaterBox{box}, l(100, 0), l(300, 0), 0.5},
		{"leaves at the surface", []WaterBox{box}, l(150, 0), l(150, 100), 0.5},
		{"chains into a touching zone", []WaterBox{box, next}, l(150, 0), l(290, 0), 1},
		{"starts dry", []WaterBox{box}, l(0, 0), l(150, 0), 0},
	}
	for _, tt := range exit {
		if got := NewWater(tt.boxes).exit(tt.a, tt.b); got != tt.want {
			t.Errorf("exit %s = %v, want %v", tt.name, got, tt.want)
		}
	}
	var none *Water
	if _, _, ok := none.Span(l(0, 0), l(400, 0)); ok || none.exit(l(0, 0), l(400, 0)) != 0 || none.Near(l(0, 0), l(1, 1), 100) {
		t.Error("nil water has water")
	}
	w := NewWater([]WaterBox{box})
	if !w.Near(l(0, 0), l(50, 0), 60) || w.Near(l(0, 0), l(50, 0), 40) || w.Near(l(0, 200), l(50, 200), 60) {
		t.Error("Near: wrong answer around the box")
	}
}

// swimPlanner is a test planner that swims the lake with breath for swim.
func swimPlanner(t *testing.T, swim float64) *Planner {
	p := testPlanner(t)
	p.Water = NewWater([]WaterBox{waterLake})
	p.Swim = swim
	return p
}

// assertSwimRoute checks every segment of route as the walker does before
// it sends one: CanMove, then the corridor test or the step-chain check
// (which swims), and that the route swims somewhere.
func assertSwimRoute(t *testing.T, p *Planner, route []location.Location) {
	t.Helper()
	swam := false
	for i := 1; i < len(route); i++ {
		a, b := route[i-1], route[i]
		if !p.Engine.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) || !(p.CorridorOpen(a, b) || p.StepChainOK(a, b)) {
			t.Errorf("segment %d %v -> %v fails the walker's checks", i, a, b)
		}
		if _, _, ok := p.Water.Span(a, b); ok {
			swam = true
		}
	}
	if !swam {
		t.Errorf("route %v never swims", route)
	}
}

// TestRouteSwims checks the swimming cell planner: it crosses the lake
// rather than walk round it, routes into and out of it, keeps to its breath
// (a dry way round when the swim is too long, ErrBreath when there is
// none), and never swims without a breath.
func TestRouteSwims(t *testing.T) {
	p := swimPlanner(t, 5000)
	for _, tc := range []struct {
		name     string
		from, to location.Location
	}{
		{"across the lake", loc(WaterWest), loc(WaterEast)},
		{"into the lake", loc(WaterWest), loc(WaterInLake)},
		{"out of the lake", loc(WaterInLake), loc(WaterEast)},
	} {
		route, err := p.Route(tc.from, tc.to)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		assertSwimRoute(t, p, route)
	}

	short := swimPlanner(t, 40)
	route, err := short.Route(loc(WaterWest), loc(WaterEast))
	if err != nil {
		t.Fatalf("breath 40 across: %v", err)
	}
	assertDry(t, short, short.Water, route)
	if _, err := short.Route(loc(WaterWest), loc(WaterInLake)); !errors.Is(err, ErrBreath) {
		t.Errorf("breath 40 into the lake: %v, want ErrBreath", err)
	}

	none := swimPlanner(t, 0)
	if _, err := none.Route(loc(WaterWest), loc(WaterInLake)); !errors.Is(err, ErrWater) {
		t.Errorf("no breath into the lake: %v, want ErrWater", err)
	}
}

// TestStepChainSwims checks the exact check's swimmer: a segment that
// wades into the lake, swims it and walks out passes; without a breath it
// fails as before; the corridor test fails it either way.
func TestStepChainSwims(t *testing.T) {
	p := swimPlanner(t, 5000)
	a, b := loc(WaterWest), loc(WaterEast)
	if !p.StepChainOK(a, b) {
		t.Error("swimming: the segment across the lake fails the step chain")
	}
	if !p.StepChainOK(loc(WaterInLake), b) || !p.StepChainOK(a, loc(WaterInLake)) {
		t.Error("swimming: a segment out of or into the lake fails the step chain")
	}
	if p.CorridorOpen(a, b) {
		t.Error("the corridor test passes water")
	}
	p.Swim = 0
	if p.StepChainOK(a, b) {
		t.Error("without a breath the segment across the lake passes the step chain")
	}
}

// TestHierarchyNeverSwims checks that the hierarchy keeps out of water
// whatever the planner's breath: a swim is the cell planner's.
func TestHierarchyNeverSwims(t *testing.T) {
	e := testEngine(t)
	w := NewWater([]WaterBox{waterLake})
	image, _, err := Build(context.Background(), BuildConfig{Engine: e, Water: w, Clusters: window(32, 2, 2, 6, 6), ClusterCells: 32, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	g, err := parseGraph(image)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHierarchy(g, e, w, DefaultMemberCacheBytes)
	p := swimPlanner(t, 5000)
	route, err := h.Route(p, loc(WaterWest), loc(WaterEast), nil)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	assertDry(t, p, w, route)
	if _, err := h.Route(p, loc(WaterWest), loc(WaterInLake), nil); !errors.Is(err, ErrWater) {
		t.Errorf("into the lake: %v, want ErrWater", err)
	}
	if p.Swim != 5000 {
		t.Errorf("Route changed the planner's breath to %v", p.Swim)
	}
}
