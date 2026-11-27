package nav

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// fastScene is a hierarchy for TestRouteStraight: its graph, engine and
// water, from which the full-search twin is built.
type fastScene struct {
	g *Graph
	e *engine.Engine
	w *Water
}

func scenes32Fast(t *testing.T) fastScene {
	th := hierScenes32(t)
	return fastScene{th.g, testEngine(t), nil}
}

// doorFast is the door scene at 64-cell clusters, its door shut in the
// engine as the door table toggles it when shut is set.
func doorFast(shut bool) func(*testing.T) fastScene {
	return func(t *testing.T) fastScene {
		e, doors := doorScene(t)
		th := buildHier(t, e, 64, doors, window(64, 27, 28, 29, 28))
		if shut {
			closeDoor(t, e, doors[0])
		}
		return fastScene{th.g, e, nil}
	}
}

func wallBandFast(t *testing.T) fastScene {
	e, th := wallBand(t)
	return fastScene{th.g, e, nil}
}

func waterFast(t *testing.T) fastScene {
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
	return fastScene{g, e, w}
}

// TestRouteStraight checks Route's straight segment (#565). A short pair
// whose segment passes pull's first-candidate checks is that one segment
// with no search run: a cell budget of 1, which any search spends at its
// first expansion, still routes. Every other pair runs the search, so the
// budget of 1 is ErrBudget, and with a real budget the route is the one the
// full search alone gives.
func TestRouteStraight(t *testing.T) {
	closedDoor := func(d int) bool { return d == doorID }
	doorFrom := loc(geotest.Point{CellX: 1760, CellY: 1823, Z: geotest.Ground})
	doorTo := loc(geotest.Point{CellX: 1888, CellY: 1823, Z: geotest.Ground})
	corridorCut := func(v *Avoid) { v.AddLine(cellAt(288, 259), cellAt(288, 264)) }
	tests := []struct {
		name       string
		scene      func(*testing.T) fastScene
		from, to   location.Location
		closed     func(door int) bool
		avoid      func(*Avoid)
		maxSegment float64
		oneWay     bool
		straight   bool
		err        error // of both routes, when not nil
	}{
		{name: "open ground", scene: scenes32Fast, from: loc(geotest.CorridorWest), to: loc(geotest.CorridorEast), straight: true},
		{name: "open ground, one way", scene: scenes32Fast, from: loc(geotest.CorridorEast), to: loc(geotest.CorridorWest), oneWay: true, straight: true},
		{name: "open ground within MaxSegment", scene: scenes32Fast, from: loc(geotest.CorridorWest), to: cellAt(280, 261), maxSegment: 500, straight: true},
		{
			name: "avoided end columns are exempt", scene: scenes32Fast, from: loc(geotest.CorridorWest), to: loc(geotest.CorridorEast),
			avoid: func(v *Avoid) {
				v.AddLine(loc(geotest.CorridorWest), cellAt(258, 262))
				v.AddLine(loc(geotest.CorridorEast), cellAt(317, 262))
			}, straight: true,
		},
		{name: "an open door", scene: doorFast(false), from: doorFrom, to: doorTo, straight: true},
		{name: "a door the table holds closed", scene: doorFast(true), from: doorFrom, to: doorTo, closed: closedDoor},
		{name: "across a wall", scene: scenes32Fast, from: loc(geotest.UInside), to: loc(geotest.UBehind)},
		{name: "a karma Avoid column", scene: scenes32Fast, from: loc(geotest.CorridorWest), to: loc(geotest.CorridorEast), avoid: corridorCut},
		{name: "a line drop the cell path avoids", scene: wallBandFast, from: loc(geotest.WallBandTop), to: loc(geotest.WallBandOutside)},
		{name: "a wet end", scene: waterFast, from: loc(WaterWest), to: loc(WaterInLake), err: ErrWater},
		{name: "a wet line", scene: waterFast, from: loc(WaterWest), to: loc(WaterEast)},
		{name: "beyond the segment cap", scene: scenes32Fast, from: loc(geotest.CorridorWest), to: loc(geotest.CorridorEast), maxSegment: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := tt.scene(t)
			h := NewHierarchy(sc.g, sc.e, sc.w, DefaultMemberCacheBytes)
			full := NewHierarchy(sc.g, sc.e, sc.w, DefaultMemberCacheBytes)
			full.searchOnly = true
			p := &Planner{Engine: sc.e, Lmax: testLmax, CollisionHeight: 23, Water: sc.w, MaxSegment: tt.maxSegment}
			if tt.avoid != nil {
				tt.avoid(&p.Avoid)
			}
			route := func(h *Hierarchy, budget int) ([]location.Location, error) {
				q := *p
				q.Budget = budget
				if tt.oneWay {
					return h.RouteOneWay(&q, tt.from, tt.to, tt.closed)
				}
				return h.Route(&q, tt.from, tt.to, tt.closed)
			}

			r, err := route(h, 1)
			switch {
			case tt.err != nil:
				if !errors.Is(err, tt.err) {
					t.Fatalf("budget 1: %v, want %v", err, tt.err)
				}
			case tt.straight:
				if err != nil || len(r) != 2 {
					t.Fatalf("budget 1: %v %v, want the straight segment with no search", r, err)
				}
			default:
				if !errors.Is(err, ErrBudget) {
					t.Fatalf("budget 1: %v %v, want ErrBudget from the search", r, err)
				}
			}

			got, err := route(h, 400000)
			want, wantErr := route(full, 400000)
			if !errors.Is(err, wantErr) || (err == nil) != (wantErr == nil) {
				t.Fatalf("Route = %v, the full search %v", err, wantErr)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("Route = %v, the full search %v", got, want)
			}
			if tt.err == nil && err != nil {
				t.Fatalf("Route: %v", err)
			}
			if tt.straight != (len(got) == 2) && err == nil {
				t.Fatalf("route %v, want straight %v", got, tt.straight)
			}
		})
	}
}

// TestRouteStraightDropOnly pins the line-drop case of TestRouteStraight to
// the drop rule (#504): the wall band's straight segment off its south edge
// passes CanMove, so only lineDrops keeps it to the search and the ramp.
func TestRouteStraightDropOnly(t *testing.T) {
	e, _ := wallBand(t)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23}
	a, b := floor(e, loc(geotest.WallBandTop)), floor(e, loc(geotest.WallBandOutside))
	s := getScratch()
	defer putScratch(s)
	if !e.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
		t.Fatal("CanMove refuses the band's straight segment: the case tests no drop")
	}
	if !p.lineDrops(s, a, b, dropBound) {
		t.Fatal("lineDrops finds no drop off the band")
	}
	if p.straight(s, a, b) {
		t.Fatal("straight takes the drop off the band")
	}
}
