package nav

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// refCorridor is the corridor test written plainly on a map rebuilt per
// test: the oracle the grid version must match. It returns the stretches
// corridorStretches must find and whether corridorClear must pass.
func refCorridor(p *Planner, a, b location.Location) ([]stretch, bool) {
	type cell struct {
		h, gap    int16
		stacked   bool
		ok, climb bool
	}
	line := appendLineCells(nil, a, b)
	req := quadrant(a, b)
	var buf [block.MaxLayers]block.Cell
	// read returns the corridor cells and whether each passes conditions 1
	// and 2, stacked tops allowed or not.
	read := func(stacked bool) map[[2]int]cell {
		corridor := map[[2]int]cell{}
		for _, lc := range line {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					k := [2]int{lc.x + dx, lc.y + dy}
					if _, ok := corridor[k]; ok || (dx != 0 || dy != 0) && refDistance(a, b, k) > corridorReach {
						continue
					}
					var c cell
					if p.hasGeo(k[0], k[1]) {
						layers := p.Engine.AppendLayers(buf[:0], k[0], k[1])
						heights := make([]int16, 0, len(layers))
						for _, l := range layers {
							heights = append(heights, l.Height)
						}
						slices.Sort(heights)
						top := layers[slices.IndexFunc(layers, func(l block.Cell) bool { return l.Height == heights[len(heights)-1] })]
						c = cell{h: top.Height, gap: math.MaxInt16, stacked: len(layers) > 1}
						if c.stacked {
							c.gap = top.Height - heights[len(heights)-2]
						}
						c.ok = (!c.stacked || stacked && c.gap > 0) && top.NSWE.Allows(req) && !p.Water.wet(k[0], k[1], top.Height)
						for _, e := range []location.Location{a, b} {
							if c.stacked && engine.GeoX(e.X) == k[0] && engine.GeoY(e.Y) == k[1] && e.Z != int(top.Height) {
								c.ok = false
							}
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
				if abs(int(c.h)-int(nc.h)) >= block.CellIgnoreHeight {
					c.climb, nc.climb = true, true
					corridor[k], corridor[nk] = c, nc
				}
			}
		}
		return corridor
	}

	strict := read(false)
	var out []stretch
	for _, lc := range line {
		open := true
		for dx := -1; dx <= 1 && open; dx++ {
			for dy := -1; dy <= 1 && open; dy++ {
				if c, in := strict[[2]int{lc.x + dx, lc.y + dy}]; in {
					open = c.ok && !c.climb
				}
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

	relaxed := read(p.Lmax > 0)
	clear := true
	reach := int((p.Lmax+stepSlack)/block.CellSize) + 1
	for k, n := range relaxed {
		clear = clear && n.ok && !n.climb
		if !n.stacked {
			continue
		}
		for c, in := range relaxed {
			if in.ok && abs(c[0]-k[0]) <= reach && abs(c[1]-k[1]) <= reach && 2*(int(n.h)-int(in.h)-2*block.CellHeight) >= int(n.gap) {
				clear = false
			}
		}
	}
	return out, clear
}

// refDistance is the distance from the segment a-b to the closed square of
// cell k, by ternary search over the segment (the distance to a convex set
// is convex along it).
func refDistance(a, b location.Location, k [2]int) float64 {
	x0, y0 := float64(k[0]*block.CellSize+engine.WorldXMin), float64(k[1]*block.CellSize+engine.WorldYMin)
	at := func(t float64) float64 {
		x := float64(a.X) + t*float64(b.X-a.X)
		y := float64(a.Y) + t*float64(b.Y-a.Y)
		dx := max(x0-x, 0, x-x0-block.CellSize)
		dy := max(y0-y, 0, y-y0-block.CellSize)
		return math.Hypot(dx, dy)
	}
	lo, hi := 0.0, 1.0
	for range 100 {
		m1, m2 := lo+(hi-lo)/3, hi-(hi-lo)/3
		if at(m1) <= at(m2) {
			hi = m2
		} else {
			lo = m1
		}
	}
	return at((lo+hi)/2) - 1e-9
}

// TestCorridorGridMatchesMap checks the grid corridor test against the map
// one on random segments around every scene, short and long (a segment
// longer than the grid wraps around it), dry and with a water box, on one scratch
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
		{"long wraps the grid", dry, 6000},
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
				want, wantClear := refCorridor(tc.p, a, b)
				got := slices.Clone(tc.p.corridorStretches(s, a, b))
				if len(want) == 0 && len(got) == 0 {
					got, want = nil, nil
				}
				if !slices.Equal(got, want) {
					t.Fatalf("corridorStretches(%v, %v) = %v, want %v", a, b, got, want)
				}
				if clear := tc.p.corridorClear(s, a, b); clear != wantClear {
					t.Fatalf("corridorClear(%v, %v) = %v, want %v", a, b, clear, wantClear)
				}
				if !wantClear {
					failing++
				}
			}
			if got, want := len(s.grid.slots), gridSide*gridSide; got != want {
				t.Fatalf("grid holds %d slots, want %d", got, want)
			}
			if failing == 0 || failing == 600 {
				t.Fatalf("%d of 600 segments fail the corridor test: the sample does not exercise both sides", failing)
			}
		})
	}
}

// TestCorridorGridBoundedOnLongSegment checks that a segment far longer than
// the grid (40000 units, 2500 cells, across the scenes and beyond the
// geodata) keeps the grid at gridSide² slots and still matches the map test.
func TestCorridorGridBoundedOnLongSegment(t *testing.T) {
	p := testPlanner(t)
	s := getScratch()
	defer putScratch(s)
	for _, d := range [][2]int{{40000, 0}, {0, 40000}, {28284, 28284}, {-30000, 20000}} {
		a := floor(p.Engine, at(Open, 0, 0, 0))
		b := location.Location{X: a.X + d[0], Y: a.Y + d[1], Z: a.Z}
		s.grid.invalidate()
		want, wantClear := refCorridor(p, a, b)
		got := slices.Clone(p.corridorStretches(s, a, b))
		if !slices.Equal(got, want) {
			t.Fatalf("corridorStretches(%v, %v) = %v, want %v", a, b, got, want)
		}
		if clear := p.corridorClear(s, a, b); clear != wantClear {
			t.Fatalf("corridorClear(%v, %v) = %v, want %v", a, b, clear, wantClear)
		}
		if got, want := len(s.grid.slots), gridSide*gridSide; got != want || cap(s.grid.slots) != want {
			t.Fatalf("grid holds %d slots (cap %d), want %d", got, cap(s.grid.slots), want)
		}
	}
}
