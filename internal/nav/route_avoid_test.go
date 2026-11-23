package nav

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// cellAt is the world point of the corridor scene's ground cell x, y.
func cellAt(x, y int) location.Location {
	return loc(geotest.Point{CellX: x, CellY: y, Z: geotest.Ground})
}

// ring adds the square of columns at Chebyshev distance r around c.
func ring(v *Avoid, c geotest.Point, r int) {
	corners := [...][2]int{{-r, -r}, {r, -r}, {r, r}, {-r, r}, {-r, -r}}
	for i := 1; i < len(corners); i++ {
		a, b := corners[i-1], corners[i]
		v.AddLine(cellAt(c.CellX+a[0], c.CellY+a[1]), cellAt(c.CellX+b[0], c.CellY+b[1]))
	}
}

// crossesAvoid reports whether a segment of route crosses a column of
// p.Avoid other than the route's endpoints'.
func crossesAvoid(p *Planner, route []location.Location) bool {
	s := getScratch()
	defer putScratch(s)
	from, to := route[0], route[len(route)-1]
	for i := 1; i < len(route); i++ {
		if p.avoided(s, route[i-1], route[i], from, to) {
			return true
		}
	}
	return false
}

// TestRouteAvoid routes through the corridor scene with a per-route avoid
// set, on the cell search and on the hierarchy: with another way the route
// takes it and crosses no avoided column, without one it is ErrUnreachable.
// The shared hierarchy is the same for every case.
func TestRouteAvoid(t *testing.T) {
	th := hierScenes32(t)
	from, to := loc(geotest.CorridorWest), loc(geotest.CorridorEast)
	tests := []struct {
		name  string
		avoid func(*Avoid)
		err   error
	}{
		{"the corridor cut across: around it", func(v *Avoid) { v.AddLine(cellAt(288, 259), cellAt(288, 264)) }, nil},
		{"the straight line, its goal column exempt", func(v *Avoid) { v.AddLine(from, to) }, nil},
		{"the start ringed: unreachable", func(v *Avoid) { ring(v, geotest.CorridorWest, 2) }, ErrUnreachable},
	}
	routers := []struct {
		name  string
		route func(p *Planner) ([]location.Location, error)
	}{
		{"cells", func(p *Planner) ([]location.Location, error) { return p.Route(from, to) }},
		{"hierarchy", func(p *Planner) ([]location.Location, error) { return th.h.Route(p, from, to, nil) }},
	}
	rng := rand.New(rand.NewPCG(7, 9))
	for _, r := range routers {
		for _, tt := range tests {
			t.Run(r.name+"/"+tt.name, func(t *testing.T) {
				p := testPlanner(t)
				p.Budget = 400000
				plain, err := r.route(p)
				if err != nil {
					t.Fatalf("without avoid: %v", err)
				}
				tt.avoid(&p.Avoid)
				if tt.err == nil && !crossesAvoid(p, plain) {
					t.Fatalf("the plain route %v crosses no avoided column: the case tests nothing", plain)
				}
				route, err := r.route(p)
				if !errors.Is(err, tt.err) {
					t.Fatalf("err %v, want %v", err, tt.err)
				}
				if err != nil {
					return
				}
				checkRoute(t, p, rng, route, from, to)
				if crossesAvoid(p, route) {
					t.Fatalf("route %v crosses an avoided column", route)
				}
			})
		}
	}
}

// TestAvoidAddLine checks the columns a refused segment adds: every column
// of its line but the stop point's, and none after Clear.
func TestAvoidAddLine(t *testing.T) {
	var v Avoid
	if v.Len() != 0 || v.skips(Cell{X: 1, Y: 1}, Cell{}) {
		t.Fatal("the zero Avoid avoids something")
	}
	v.AddLine(cellAt(300, 261), cellAt(304, 261))
	if v.Len() != 4 {
		t.Fatalf("%d columns, want 4", v.Len())
	}
	stop, next := pointCell(geotest.Point{CellX: 300, CellY: 261, Z: geotest.Ground}), pointCell(geotest.Point{CellX: 301, CellY: 261, Z: geotest.Ground})
	if v.has(stop) || !v.has(next) {
		t.Fatalf("stop column avoided %v, next %v: want only the next", v.has(stop), v.has(next))
	}
	if v.skips(next, next) {
		t.Fatal("the goal's own column is skipped")
	}
	v.Clear()
	if v.Len() != 0 || v.has(next) {
		t.Fatal("Clear left columns")
	}
}
