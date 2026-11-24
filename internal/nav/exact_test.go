package nav

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestExactStair is the stretch only some step sequences pass: short steps
// climb the stair 0, 40, 80, while a step long enough to cross the 40 cell
// lands on the 20 layer and is blocked. With the measured Lmax no step is
// that long, and the exact check passes; with a longer Lmax (a first step
// of any length up to it) it rejects, and the copied step chain confirms a
// blocking sequence.
func TestExactStair(t *testing.T) {
	p := testPlanner(t)
	e := p.Engine
	a, b := loc(StairLow), loc(StairDeck)
	if !e.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
		t.Fatal("CanMove up the stair fails")
	}
	if p.CorridorOpen(a, b) {
		t.Fatal("corridor test passes over multilayer cells")
	}
	if !p.StepChainOK(a, b) {
		t.Fatalf("StepChainOK with Lmax %v = false, want true", p.Lmax)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	if w, ok := walkSteps(e, rng, a, b, p.Lmax, 200); !ok {
		t.Fatalf("a step sequence within Lmax fails: %+v", w)
	}

	long := *p
	long.Lmax = 40
	if long.StepChainOK(a, b) {
		t.Fatal("StepChainOK with Lmax 40 = true, want false")
	}
	// From the last cell before the step, one 30-unit first update crosses
	// the 40 cell into the layered one.
	start := at(geotest.Point{CellX: 199, CellY: StairLow.CellY}, 0, 0, 0)
	w := e.WalkSteps(start, b, 300, 300, []time.Duration{100 * time.Millisecond})
	if !w.Blocked {
		t.Fatalf("a 30-unit first step = %+v, want blocked", w)
	}
}

// TestStepChainAgreesWithWalkSteps runs random short segments around every
// scene with an obstacle. A segment the check accepts must survive random
// step sequences of the copied step chain; the run must also include
// segments the corridor test rejects and the exact check accepts, and
// segments the exact check rejects.
func TestStepChainAgreesWithWalkSteps(t *testing.T) {
	p := testPlanner(t)
	e := p.Engine
	rng := rand.New(rand.NewPCG(7, 11))
	scenes := []geotest.Point{
		geotest.BridgeWestDeck, geotest.BridgeEastGround, geotest.LedgeHigh, geotest.LedgeLow,
		geotest.CorridorWest, geotest.UInside, geotest.PitOutside, StairLow, StairDeck,
		Floors40, Floors60, Floors100, {CellX: Floors40.CellX, CellY: Floors40.CellY, Z: 40},
		{CellX: Floors100.CellX, CellY: Floors100.CellY, Z: 100},
	}
	long := *p
	long.Lmax = 40
	var corridor, exact, rejected int
	for i := range 600 {
		sc := scenes[i%len(scenes)]
		p := p
		if i%2 == 1 {
			p = &long // a longer step bound, as a loaded server stretches Emax
		}
		a := floor(e, at(sc, rng.IntN(161)-80, rng.IntN(161)-80, rng.IntN(2)*160))
		b := floor(e, location.Location{X: a.X + rng.IntN(161) - 80, Y: a.Y + rng.IntN(161) - 80, Z: a.Z + rng.IntN(241) - 80})
		if !e.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) {
			continue
		}
		switch {
		case p.CorridorOpen(a, b):
			corridor++
		case p.StepChainOK(a, b):
			exact++
		default:
			rejected++
			continue
		}
		if w, ok := walkSteps(e, rng, a, b, p.Lmax, 20); !ok {
			t.Fatalf("%v -> %v accepted, but a step sequence ends %+v", a, b, w)
		}
	}
	t.Logf("corridor %d, exact %d, rejected %d", corridor, exact, rejected)
	if corridor == 0 || exact == 0 || rejected == 0 {
		t.Fatalf("coverage: corridor %d, exact %d, rejected %d; want each > 0", corridor, exact, rejected)
	}
}
