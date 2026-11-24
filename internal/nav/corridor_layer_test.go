package nav

import (
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestCorridorWalkingLayer checks the walking layer the corridor carries
// from the start floor over cells of several layers (#307): the ground
// under a deck 60 or 100 above passes, the ground under a deck 40 above
// fails (CanMove crossing from 0 lands on belowCell(0 + 48), the deck), and
// a segment whose end stands on a layer the walking layer does not reach
// fails.
func TestCorridorWalkingLayer(t *testing.T) {
	p := testPlanner(t)
	near := func(f geotest.Point, dx, dy, z int) location.Location {
		return loc(geotest.Point{CellX: f.CellX + dx, CellY: f.CellY + dy, Z: z})
	}
	tests := []struct {
		name string
		a, b location.Location
		want bool
	}{
		{"ground under the 100 deck", near(Floors100, -4, -2, 0), near(Floors100, 4, 2, 0), true},
		{"on the 100 deck", near(Floors100, -4, -2, 100), near(Floors100, 4, 2, 100), true},
		{"ground under the 60 deck", near(Floors60, -4, -2, 0), near(Floors60, 4, 2, 0), true},
		{"on the 60 deck", near(Floors60, -4, 2, 60), near(Floors60, 4, -2, 60), true},
		{"ground under the 40 deck: a crossing lands on the deck", near(Floors40, -4, -2, 0), near(Floors40, 4, 2, 0), false},
		{"on the 40 deck", near(Floors40, -4, -2, 40), near(Floors40, 4, 2, 40), true},
		{"in from open ground under the 100 deck", near(Floors100, -12, 0, 0), near(Floors100, 4, 0, 0), true},
		{"in from open ground, to the 100 deck above the walking layer", near(Floors100, -12, 0, 0), near(Floors100, 4, 0, 100), false},
		{"under the bridge deck", loc(geotest.BridgeWestGround), loc(geotest.BridgeEastGround), true},
		{"along the bridge deck", loc(geotest.BridgeWestDeck), loc(geotest.BridgeEastDeck), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !p.Engine.CanMove(tt.a.X, tt.a.Y, tt.a.Z, tt.b.X, tt.b.Y, tt.b.Z) && tt.want {
				t.Fatal("CanMove fails on a segment the corridor should pass")
			}
			if got := p.CorridorOpen(tt.a, tt.b); got != tt.want {
				t.Fatalf("CorridorOpen(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// layeredScenes are the scenes with cells of several layers: the bridge
// deck over ground, stacked floors 30, 40, 60 and 100 apart, and the stair.
var layeredScenes = []geotest.Point{
	geotest.BridgeWestDeck, geotest.BridgeEastGround, StairLow, StairDeck, StackedGround,
	Floors40, Floors60, Floors100,
	{CellX: Floors40.CellX, CellY: Floors40.CellY, Z: 40},
	{CellX: Floors60.CellX, CellY: Floors60.CellY, Z: 60},
	{CellX: Floors100.CellX, CellY: Floors100.CellY, Z: 100},
}

// TestFirstStepAgreesWithWalkSteps checks the first step's bound (#307):
// the server times the first position update from the move request, so it
// is at most Lmax like every other. Every segment the step-chain check
// accepts around the layered scenes must survive the copied step chain with
// a first step of each length from 1 to Lmax.
func TestFirstStepAgreesWithWalkSteps(t *testing.T) {
	p := testPlanner(t)
	e := p.Engine
	rng := rand.New(rand.NewPCG(17, 19))
	var corridor, exact int
	for i := range 400 {
		sc := layeredScenes[i%len(layeredScenes)]
		a := floor(e, at(sc, rng.IntN(161)-80, rng.IntN(161)-80, 0))
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
			continue
		}
		for first := 1; first <= int(p.Lmax); first++ {
			if w, ok := walkFirst(e, rng, a, b, p.Lmax, float64(first), 2); !ok {
				t.Fatalf("%v -> %v accepted, but a first step of %d ends %+v", a, b, first, w)
			}
		}
	}
	t.Logf("corridor %d, exact %d", corridor, exact)
	if corridor == 0 || exact == 0 {
		t.Fatalf("coverage: corridor %d, exact %d; want each > 0", corridor, exact)
	}
}
