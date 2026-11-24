package nav

import (
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// wideCorridorOpen is the corridor test before #256 narrowed it: every cell
// of the 3×3 around each line cell has exactly one layer, its NSWE allows
// the quadrant, it is dry, and 4-neighbours differ by less than 48.
func wideCorridorOpen(p *Planner, a, b location.Location) bool {
	req := quadrant(a, b)
	var buf [block.MaxLayers]block.Cell
	h := map[[2]int]int16{}
	for _, lc := range appendLineCells(nil, a, b) {
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				k := [2]int{lc.x + dx, lc.y + dy}
				if _, ok := h[k]; ok {
					continue
				}
				if !p.hasGeo(k[0], k[1]) {
					return false
				}
				layers := p.Engine.AppendLayers(buf[:0], k[0], k[1])
				if len(layers) != 1 || !layers[0].NSWE.Allows(req) || p.Water.wet(k[0], k[1], layers[0].Height) {
					return false
				}
				h[k] = layers[0].Height
			}
		}
	}
	for k, z := range h {
		for _, d := range [2][2]int{{1, 0}, {0, 1}} {
			if nz, ok := h[[2]int{k[0] + d[0], k[1] + d[1]}]; ok && abs(int(z)-int(nz)) >= block.CellIgnoreHeight {
				return false
			}
		}
	}
	return true
}

// TestNarrowCorridorAgreesWithExact checks the corridor test's two
// widenings (#256) against the exact check and the copied step chain: a
// segment the corridor test passes where the wide one fails, because a cell
// out of reach of the step chain fails or because it stands on stacked
// tops, must pass the exact check over its whole length and survive random
// step sequences. The run must include both kinds.
func TestNarrowCorridorAgreesWithExact(t *testing.T) {
	short := testPlanner(t)
	long := *short
	long.Lmax = 40 // a longer step bound reaches farther stacked cells
	e := short.Engine
	rng := rand.New(rand.NewPCG(5, 13))
	scenes := []geotest.Point{
		geotest.BridgeWestDeck, geotest.BridgeEastGround, geotest.LedgeHigh, geotest.LedgeLow,
		geotest.CorridorWest, geotest.UInside, geotest.PitOutside, StairLow, StairDeck, StackedGround,
		{CellX: 300, CellY: 1002, Z: 30},
	}
	s := getScratch()
	defer putScratch(s)
	var narrow, stacked int
	for i := range 3000 {
		p := short
		if i%2 == 1 {
			p = &long
		}
		sc := scenes[i%len(scenes)]
		a := floor(e, at(sc, rng.IntN(161)-80, rng.IntN(161)-80, rng.IntN(2)*160))
		b := floor(e, location.Location{X: a.X + rng.IntN(161) - 80, Y: a.Y + rng.IntN(161) - 80, Z: a.Z + rng.IntN(241) - 80})
		if !e.CanMove(a.X, a.Y, a.Z, b.X, b.Y, b.Z) || wideCorridorOpen(p, a, b) {
			continue
		}
		s.grid.invalidate()
		if !p.corridorClear(s, a, b) {
			continue
		}
		if len(s.grid.layered) > 0 {
			stacked++
		} else {
			narrow++
		}
		if !p.exactStretch(s, a, b, stretch{0, hypot(b.X-a.X, b.Y-a.Y)}) {
			t.Fatalf("Lmax %v: %v -> %v passes the corridor test, but not the exact check", p.Lmax, a, b)
		}
		if w, ok := walkSteps(e, rng, a, b, p.Lmax, 20); !ok {
			t.Fatalf("Lmax %v: %v -> %v passes the corridor test, but a step sequence ends %+v", p.Lmax, a, b, w)
		}
	}
	t.Logf("narrowed %d, on stacked tops %d", narrow, stacked)
	if narrow == 0 || stacked == 0 {
		t.Fatalf("coverage: narrowed %d, on stacked tops %d; want each > 0", narrow, stacked)
	}
}

// TestNearCell checks which cells beside a segment join its corridor: those
// within corridorReach of it.
func TestNearCell(t *testing.T) {
	o := at(Open, 0, 0, 0) // a cell centre
	gx, gy := cellOf(o)
	tests := []struct {
		name   string
		a, b   location.Location
		dx, dy int
		want   bool
	}{
		{"row above, line through the centres", o, at(Open, 160, 0, 0), 1, -1, false},
		{"row above, line 4 units below its border", at(Open, 0, -4, 0), at(Open, 160, -4, 0), 1, -1, true},
		{"row above, line 5 units below its border", at(Open, 0, -3, 0), at(Open, 160, -3, 0), 1, -1, false},
		{"behind the start, beyond reach", o, at(Open, 160, 0, 0), -1, 0, false},
		{"behind a start 4 units from the border", at(Open, -4, 0, 0), at(Open, 160, 0, 0), -1, 0, true},
		{"diagonal: the corner the line passes", o, at(Open, 160, 160, 0), 1, 0, true},
		{"diagonal: 1 unit off a corner", at(Open, 0, 1, 0), at(Open, 160, 161, 0), 1, 0, true},
		{"diagonal: across the cell", at(Open, -7, 7, 0), at(Open, 153, 167, 0), 1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newSegment(tt.a, tt.b).nearCell(gx+tt.dx, gy+tt.dy); got != tt.want {
				t.Fatalf("nearCell(%+d, %+d) = %v, want %v", tt.dx, tt.dy, got, tt.want)
			}
		})
	}
}
