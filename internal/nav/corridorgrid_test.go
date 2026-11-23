package nav

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// refCorridorStretches is the corridor test as N2 wrote it, on a map rebuilt
// per test: the oracle the grid version must match.
func refCorridorStretches(p *Planner, a, b location.Location) []stretch {
	type cell struct {
		h         int16
		ok, climb bool
	}
	line := appendLineCells(nil, a, b)
	corridor := map[[2]int]cell{}
	req := quadrant(a, b)
	var buf [block.MaxLayers]block.Cell
	for _, lc := range line {
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				k := [2]int{lc.x + dx, lc.y + dy}
				if _, ok := corridor[k]; ok {
					continue
				}
				var c cell
				if p.hasGeo(k[0], k[1]) {
					if layers := p.Engine.AppendLayers(buf[:0], k[0], k[1]); len(layers) == 1 {
						h := layers[0].Height
						c = cell{h: h, ok: layers[0].NSWE.Allows(req) && !p.Water.wet(k[0], k[1], h)}
					}
				}
				corridor[k] = c
			}
		}
	}
	for k, c := range corridor {
		if !c.ok {
			continue
		}
		for _, d := range [2][2]int{{1, 0}, {0, 1}} {
			nk := [2]int{k[0] + d[0], k[1] + d[1]}
			nc, in := corridor[nk]
			if !in || !nc.ok {
				continue
			}
			if abs(int(c.h)-int(nc.h)) > block.CellIgnoreHeight {
				c.climb, nc.climb = true, true
				corridor[k], corridor[nk] = c, nc
			}
		}
	}
	var out []stretch
	for _, lc := range line {
		open := true
		for dx := -1; dx <= 1 && open; dx++ {
			for dy := -1; dy <= 1 && open; dy++ {
				c := corridor[[2]int{lc.x + dx, lc.y + dy}]
				open = c.ok && !c.climb
			}
		}
		if open {
			continue
		}
		if n := len(out); n > 0 && out[n-1].t1 >= lc.t0 {
			out[n-1].t1 = max(out[n-1].t1, lc.t1)
			continue
		}
		out = append(out, stretch{lc.t0, lc.t1})
	}
	return out
}

// TestCorridorGridMatchesMap checks the grid corridor test against the map
// one on random segments around every scene, short and long (a segment
// longer than the grid grows it), dry and with a water box, on one scratch
// reused across them all as a pull reuses it: corridorStretches returns the
// same stretches and corridorClear answers whether there are none.
func TestCorridorGridMatchesMap(t *testing.T) {
	dry := testPlanner(t)
	wet := *dry
	x, y, z := geotest.LedgeHigh.World()
	wet.Water = NewWater([]WaterBox{{X1: x - 200, Y1: y - 200, X2: x + 200, Y2: y + 200, MinZ: z - 300, MaxZ: z + 300}})
	scenes := []geotest.Point{
		geotest.CorridorWest, geotest.AboveCorridor, geotest.UInside, geotest.LedgeHigh, geotest.LedgeLow,
		geotest.PitOutside, geotest.PitInside, geotest.BridgeWestDeck, geotest.BridgeEastGround,
		geotest.PocketInside, geotest.PocketOutside, StairLow, StairDeck, StackedGround, Open,
	}
	tests := []struct {
		name   string
		p      *Planner
		spread int // world units around the scene
	}{
		{"short", dry, 300},
		{"navSegment", dry, 1800},
		{"long grows the grid", dry, 6000},
		{"water", &wet, 600},
	}
	rng := rand.New(rand.NewPCG(3, 9))
	s := getScratch()
	defer putScratch(s)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			failing := 0
			for i := range 600 {
				sc := scenes[i%len(scenes)]
				a := floor(tc.p.Engine, at(sc, rng.IntN(2*tc.spread+1)-tc.spread, rng.IntN(2*tc.spread+1)-tc.spread, rng.IntN(2)*160))
				b := floor(tc.p.Engine, at(sc, rng.IntN(2*tc.spread+1)-tc.spread, rng.IntN(2*tc.spread+1)-tc.spread, rng.IntN(2)*160))
				if i%5 == 0 {
					s.grid.invalidate()
				}
				want := refCorridorStretches(tc.p, a, b)
				got := slices.Clone(tc.p.corridorStretches(s, a, b))
				if len(want) == 0 && len(got) == 0 {
					got, want = nil, nil
				}
				if !slices.Equal(got, want) {
					t.Fatalf("corridorStretches(%v, %v) = %v, want %v", a, b, got, want)
				}
				if clear := tc.p.corridorClear(s, a, b); clear != (len(want) == 0) {
					t.Fatalf("corridorClear(%v, %v) = %v, want %v", a, b, clear, len(want) == 0)
				}
				if len(want) > 0 {
					failing++
				}
			}
			if failing == 0 || failing == 600 {
				t.Fatalf("%d of 600 segments fail the corridor test: the sample does not exercise both sides", failing)
			}
		})
	}
}
