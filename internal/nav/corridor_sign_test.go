package nav

import (
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// signTiles hold the synthetic region at each sign of the world
// coordinates: CanMove picks each border crossing's row with int(), which
// truncates toward zero, down for positive coordinates and up for negative
// ones (server geo/engine/engine_query.go CanMove).
var signTiles = [][2]int{
	{geotest.RegionX, geotest.RegionY},         // x ≥ 0, y ≥ 0
	{geotest.RegionX - 1, geotest.RegionY - 1}, // x < 0, y < 0
	{geotest.RegionX - 1, geotest.RegionY},     // x < 0, y ≥ 0
	{geotest.RegionX, geotest.RegionY - 1},     // x ≥ 0, y < 0
}

var (
	signOnce   sync.Once
	signEngine *engine.Engine
	signErr    error
)

// TestCorridorReachBothSigns checks corridorReach against CanMove's own
// border walk at every sign of the coordinates: for random segments on open
// ground, every walk CanMove takes between two points the step chain can
// round to (band points, a step ahead of each other toward b, a among them)
// visits only cells of the segment's corridor: line cells, and cells next
// to them within corridorReach.
func TestCorridorReachBothSigns(t *testing.T) {
	signOnce.Do(func() { signEngine, signErr = buildEngineAt(terrain(), signTiles...) })
	if signErr != nil {
		t.Fatal(signErr)
	}
	e := signEngine
	rng := rand.New(rand.NewPCG(23, 29))
	const lmax = 40.0 // longer than any measured step
	for _, tile := range signTiles {
		ox := engine.WorldX((tile[0]-engine.TileXMin)*block.RegionBlocksX*block.CellsX + Open.CellX)
		oy := engine.WorldY((tile[1]-engine.TileYMin)*block.RegionBlocksY*block.CellsY + Open.CellY)
		var walks, aside int
		for range 300 {
			a := location.Location{X: ox + rng.IntN(161) - 80, Y: oy + rng.IntN(161) - 80}
			b := location.Location{X: a.X + rng.IntN(601) - 300, Y: a.Y + rng.IntN(601) - 300}
			if a.X == b.X && a.Y == b.Y {
				continue
			}
			corridor := map[[2]int]bool{}
			seg := newSegment(a, b)
			for _, lc := range appendLineCells(nil, a, b) {
				for dx := -1; dx <= 1; dx++ {
					for dy := -1; dy <= 1; dy++ {
						if dx == 0 && dy == 0 || seg.nearCell(lc.x+dx, lc.y+dy) {
							corridor[[2]int{lc.x + dx, lc.y + dy}] = true
						}
					}
				}
			}
			line := map[[2]int]bool{}
			for _, lc := range appendLineCells(nil, a, b) {
				line[[2]int{lc.x, lc.y}] = true
			}
			band := appendBand(nil, a, b, 0, hypot(b.X-a.X, b.Y-a.Y))
			points := append([]location.Location{a}, make([]location.Location, 0, len(band))...)
			for _, bp := range band {
				points = append(points, location.Location{X: bp.x, Y: bp.y})
			}
			for range 60 {
				p, q := points[rng.IntN(len(points))], points[rng.IntN(len(points))]
				if (p.X == q.X && p.Y == q.Y) || !between(p.X, q.X, b.X) || !between(p.Y, q.Y, b.Y) || hypot(q.X-p.X, q.Y-p.Y) > lmax+stepSlack {
					continue
				}
				walks++
				for _, c := range walkCells(e, p, q) {
					if !corridor[c] {
						t.Fatalf("tile %v: segment %v -> %v, CanMove %v -> %v visits cell %v outside the corridor", tile, a, b, p, q, c)
					}
					if !line[c] {
						aside++
					}
				}
			}
		}
		t.Logf("tile %v: %d walks, %d cells off the line", tile, walks, aside)
		if walks == 0 || aside == 0 {
			t.Fatalf("tile %v: coverage: %d walks, %d cells off the line; want each > 0", tile, walks, aside)
		}
	}
}

// walkCells returns the cells CanMove's border walk from p to q visits, from
// its trace: the cell it leaves at each crossing, then q's.
func walkCells(e *engine.Engine, p, q location.Location) [][2]int {
	var tr engine.Trace
	if !e.CanMoveTraced(p.X, p.Y, p.Z, q.X, q.Y, q.Z, &tr) {
		panic("CanMove fails on open ground")
	}
	cells := [][2]int{{engine.GeoX(p.X), engine.GeoY(p.Y)}}
	for _, st := range tr.Strokes {
		if st.Color == engine.TraceGreen && st.Line && st.X2 == st.X && st.Y2 > st.Y {
			cells = append(cells, [2]int{engine.GeoX(st.X), engine.GeoY(st.Y)})
		}
	}
	return append(cells, [2]int{engine.GeoX(q.X), engine.GeoY(q.Y)})
}
