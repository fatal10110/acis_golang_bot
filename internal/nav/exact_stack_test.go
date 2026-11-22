package nav

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestExactSamePointStep is the step that rounds back onto its own point: a
// short update from the ground under the 30 deck re-floors the bot at
// Height(cur, Z + 2*CellHeight), the deck, and CanMove refuses the climb.
// Standing in the cell's last column, every other point a step reaches lies
// in the next cell, so only the same-point step catches it.
func TestExactSamePointStep(t *testing.T) {
	p := testPlanner(t)
	e := p.Engine
	a := at(geotest.Point{CellX: 305, CellY: StackedGround.CellY}, 0, 0, 0)
	a.X = engine.WorldXMin + engine.GeoX(a.X)<<4 + 15 // the cell's last column
	b := at(geotest.Point{CellX: 310, CellY: StackedGround.CellY}, 0, 0, 0)
	if z := e.Height(a.X, a.Y, a.Z); z != 0 {
		t.Fatalf("start floor = %d, want the ground 0", z)
	}
	if !e.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
		t.Fatal("CanMove off the stacked cell fails")
	}
	if w := e.WalkSteps(a, b, 100, 100, []time.Duration{time.Millisecond}); !w.Blocked {
		t.Fatalf("a 1 ms first update = %+v, want blocked", w)
	}
	if p.StepChainOK(a, b) {
		t.Fatal("StepChainOK = true, want false")
	}
}
