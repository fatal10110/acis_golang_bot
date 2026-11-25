package nav

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// fanPillar is a 2x2 cell pillar Plateau high on open ground: the middle
// line of the fan of TestFanOpen crosses it, the lines from the stretch's
// ends pass more than a cell clear of it.
var fanPillar = [4]int{104, 100, 105, 101}

var (
	fanOnce   sync.Once
	fanEngine *engine.Engine
	fanErr    error
)

func fanPlanner(t *testing.T) *Planner {
	t.Helper()
	fanOnce.Do(func() {
		fanEngine, fanErr = buildEngine(terrain().Raise(fanPillar[0], fanPillar[1], fanPillar[2], fanPillar[3], geotest.Plateau))
	})
	if fanErr != nil {
		t.Fatal(fanErr)
	}
	return &Planner{Engine: fanEngine, Lmax: testLmax, CollisionHeight: 23, Budget: 40000}
}

func fanCell(x, y int) location.Location {
	return loc(geotest.Point{CellX: x, CellY: y, Z: geotest.Ground})
}

// TestFanOpen checks the fan of a click-ahead: every line from the stretch
// to the next waypoint must hold, not only the lines from its ends.
func TestFanOpen(t *testing.T) {
	p := fanPlanner(t)
	a0, a1 := fanCell(100, 96), fanCell(100, 104)
	for _, end := range []location.Location{a0, a1} {
		if !p.Engine.CanMove(end.X, end.Y, end.Z, fanCell(125, 100).X, fanCell(125, 100).Y, 23) || !p.CorridorOpen(end, fanCell(125, 100)) {
			t.Fatalf("the line from the stretch end %v must hold on its own", end)
		}
	}
	avoidMid := *p
	avoidMid.Avoid.AddLine(fanCell(91, 100), fanCell(90, 100))
	tests := []struct {
		name   string
		p      *Planner
		a0, a1 location.Location
		b      location.Location
		want   bool
	}{
		{"open ground", p, fanCell(100, 96), fanCell(100, 104), fanCell(100, 140), true},
		{"open ground, one point", p, fanCell(100, 96), fanCell(100, 96), fanCell(130, 96), true},
		{"pillar inside the fan", p, a0, a1, fanCell(125, 100), false},
		{"pillar on the end line", p, fanCell(100, 100), fanCell(100, 100), fanCell(125, 100), false},
		{"open ground westward", p, a0, a1, fanCell(75, 100), true},
		{"avoided column inside the fan", &avoidMid, a0, a1, fanCell(75, 100), false},
		{"wall to the target", testPlanner(t), loc(geotest.UInside), at(geotest.UInside, 0, 32, 0), loc(geotest.UBehind), false},
		{"off the geodata", p, location.Location{X: engine.WorldXMax + 100, Y: 0}, location.Location{X: engine.WorldXMax + 100, Y: 0}, fanCell(100, 100), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.FanOpen(tt.a0, tt.a1, tt.b); got != tt.want {
				t.Errorf("FanOpen = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFanQuadrant checks that the fan's NSWE covers every line of it, the
// band around the stretch included.
func TestFanQuadrant(t *testing.T) {
	o := location.Location{X: 1000, Y: 1000}
	off := func(dx, dy int) location.Location { return location.Location{X: o.X + dx, Y: o.Y + dy} }
	tests := []struct {
		name   string
		a0, a1 location.Location
		want   block.NSWE
	}{
		{"one quadrant", off(-100, -10), off(-100, 10), block.East | block.South | block.North},
		{"due east, band both ways", off(-100, 0), off(-100, 0), block.East | block.North | block.South},
		{"across the target's column", off(-50, -100), off(50, -100), block.East | block.West | block.South},
		{"far south-east", off(-300, -300), off(-200, -300), block.East | block.South},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fanQuadrant(tt.a0, tt.a1, o); got != tt.want {
				t.Errorf("fanQuadrant = %v, want %v", got, tt.want)
			}
		})
	}
}
