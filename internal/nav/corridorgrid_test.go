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
// test: the oracle the grid version must match. It fills the corridor in
// the grid's order (each line cell, then its ring), gives each cell its
// walking layer as it joins, and applies the pair rules over the whole
// corridor at the end, with the engine's own Height. It returns the
// stretches corridorStretches must find and whether corridorClear must pass.
func refCorridor(p *Planner, a, b location.Location) ([]stretch, bool) {
	type cell struct {
		layers    []block.Cell
		h         int16
		nswe      block.NSWE
		ok, bad   bool
		layered   bool
		hasLayers bool
	}
	line := appendLineCells(nil, a, b)
	req := quadrant(a, b)
	corridor := map[[2]int]*cell{}
	var order [][2]int
	ea := [2]int{engine.GeoX(a.X), engine.GeoY(a.Y)}
	eb := [2]int{engine.GeoX(b.X), engine.GeoY(b.Y)}
	below := func(layers []block.Cell, q int) (block.Cell, bool) {
		var best block.Cell
		found := false
		for _, l := range layers {
			if int(l.Height) < q && (!found || l.Height > best.Height) {
				best, found = l, true
			}
		}
		return best, found
	}
	nearest := func(k [2]int, z int) int16 { return p.Engine.HeightNearest(k[0], k[1], z) }
	// anchor is the floor of the first passing neighbour the quadrant
	// crosses from, else of another 4-neighbour, else of a diagonal one.
	anchor := func(k [2]int) (int16, bool) {
		var other, diag []int16
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				n, in := corridor[[2]int{k[0] + dx, k[1] + dy}]
				if (dx == 0 && dy == 0) || !in || !n.ok {
					continue
				}
				switch {
				case dx != 0 && dy != 0:
					diag = append(diag, n.h)
				case req.Allows(stepDir(-dx, -dy)):
					return n.h, true
				default:
					other = append(other, n.h)
				}
			}
		}
		if all := append(other, diag...); len(all) > 0 {
			return all[0], true
		}
		return 0, false
	}
	ring := [9][2]int{{0, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}, {-1, -1}, {0, -1}, {1, -1}, {-1, 0}}
	for _, lc := range line {
		for i, d := range ring {
			k := [2]int{lc.x + d[0], lc.y + d[1]}
			if _, in := corridor[k]; in || i > 0 && refDistance(a, b, k) > corridorReach {
				continue
			}
			c := &cell{}
			corridor[k] = c
			order = append(order, k)
			if !p.hasGeo(k[0], k[1]) {
				continue
			}
			c.layers = p.Engine.AppendLayers(nil, k[0], k[1])
			c.layered = len(c.layers) > 1
			switch {
			case !c.layered:
				c.h, c.nswe, c.hasLayers = c.layers[0].Height, c.layers[0].NSWE, true
			case p.Lmax <= 0:
				continue
			case k == ea:
				c.h, c.hasLayers = nearest(k, a.Z), true
			default:
				from, ok := anchor(k)
				if !ok {
					continue
				}
				l, ok := below(c.layers, int(from)+block.CellIgnoreHeight)
				if !ok {
					continue
				}
				c.h, c.hasLayers = l.Height, true
			}
			if c.layered {
				if k == eb && c.h != nearest(k, b.Z) {
					continue
				}
				for _, l := range c.layers {
					if l.Height == c.h {
						c.nswe = l.NSWE
						break
					}
				}
			}
			c.ok = c.nswe.Allows(req) && !p.Water.wet(k[0], k[1], c.h)
		}
	}
	// Condition 3 and CanMove's crossings between neighbours.
	for _, k := range order {
		c := corridor[k]
		for _, d := range [2][2]int{{1, 0}, {0, 1}} {
			nk := [2]int{k[0] + d[0], k[1] + d[1]}
			n, in := corridor[nk]
			if !c.ok || !in || !n.ok {
				continue
			}
			bad := abs(int(c.h)-int(n.h)) >= block.CellIgnoreHeight
			if l, ok := below(n.layers, int(c.h)+block.CellIgnoreHeight); req.Allows(stepDir(d[0], d[1])) && (!ok || l.Height != n.h) {
				bad = true
			}
			if l, ok := below(c.layers, int(n.h)+block.CellIgnoreHeight); req.Allows(stepDir(-d[0], -d[1])) && (!ok || l.Height != c.h) {
				bad = true
			}
			if bad {
				c.bad, n.bad = true, true
			}
		}
	}
	// The layer rule: a step from O within reach into a layered C lands on
	// C's walking layer.
	reach := int((p.Lmax+2*stepSlack)/block.CellSize) + 1
	for _, k := range order {
		c := corridor[k]
		if !c.ok || !c.layered {
			continue
		}
		x, y := engine.WorldX(k[0]), engine.WorldY(k[1])
		for _, ok := range order {
			o := corridor[ok]
			if o.ok && abs(ok[0]-k[0]) <= reach && abs(ok[1]-k[1]) <= reach && p.Engine.Height(x, y, int(o.h)+2*block.CellHeight) != c.h {
				c.bad = true
			}
		}
	}

	var out []stretch
	for _, lc := range line {
		open := true
		for dx := -1; dx <= 1 && open; dx++ {
			for dy := -1; dy <= 1 && open; dy++ {
				if c, in := corridor[[2]int{lc.x + dx, lc.y + dy}]; in {
					open = c.ok && !c.bad
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
	return out, len(out) == 0
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
