package nav

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

func TestCorridorOpen(t *testing.T) {
	p := testPlanner(t)
	cell := func(x, y, z int) location.Location { return at(geotest.Point{CellX: x, CellY: y, Z: z}, 0, 0, 0) }
	tests := []struct {
		name string
		a, b location.Location
		want bool
	}{
		{"open ground", at(Open, 0, 0, 0), at(Open, 640, 330, 0), true},
		{"open ground, axis aligned", at(Open, 0, 0, 0), at(Open, 0, -700, 0), true},
		{"along the corridor beside both walls", loc(geotest.CorridorWest), loc(geotest.CorridorEast), true},
		{"stacked tops along the bridge deck", loc(geotest.BridgeWestDeck), loc(geotest.BridgeEastDeck), true},
		{"ends under the stacked tops of the bridge", loc(geotest.BridgeWestGround), loc(geotest.BridgeEastGround), false},
		{"stacked tops along the stair deck", cell(203, 1005, 80), cell(214, 1005, 80), true},
		{"onto the stair deck: a step over its 40 cell lands on the 20 layer", loc(StairLow), loc(StairDeck), false},
		{"partial NSWE: north-east toward the corridor wall", cell(260, 262, 0), cell(300, 260, 0), false},
		{"a drop one cell aside, out of the step chain's reach", cell(800, 270, geotest.Plateau), cell(800, 300, geotest.Plateau), true},
		{"a drop within corridorReach of the line", at(geotest.Point{CellX: 800, CellY: 270, Z: geotest.Plateau}, -5, 0, 0), at(geotest.Point{CellX: 800, CellY: 300, Z: geotest.Plateau}, -5, 0, 0), false},
		{"no geodata", cell(5, 100, 0), cell(-5, 100, 0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.CorridorOpen(tt.a, tt.b); got != tt.want {
				t.Fatalf("CorridorOpen(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestLineCells checks the traversal against dense sampling of the line:
// every sampled point's cell is listed, in order, and the listed t ranges
// tile the segment.
func TestLineCells(t *testing.T) {
	base := at(Open, 0, 0, 0)
	ends := []location.Location{
		{X: base.X + 400, Y: base.Y + 37},
		{X: base.X - 120, Y: base.Y + 333},
		{X: base.X + 64, Y: base.Y + 64},
		{X: base.X, Y: base.Y - 90},
		{X: base.X + 5, Y: base.Y + 3},
	}
	for _, b := range ends {
		cells := appendLineCells(nil, base, b)
		if cells[0].t0 != 0 || cells[len(cells)-1].t1 != hypot(b.X-base.X, b.Y-base.Y) {
			t.Fatalf("%v: t range %v..%v", b, cells[0].t0, cells[len(cells)-1].t1)
		}
		listed := map[[2]int]bool{}
		for i, c := range cells {
			listed[[2]int{c.x, c.y}] = true
			if i > 0 && c.t0 != cells[i-1].t1 {
				t.Fatalf("%v: gap between cells %d and %d", b, i-1, i)
			}
		}
		for i := 0; i <= 1000; i++ {
			f := float64(i) / 1000
			x := float64(base.X) + f*float64(b.X-base.X)
			y := float64(base.Y) + f*float64(b.Y-base.Y)
			cx, cy := cellOf(location.Location{X: int(x), Y: int(y)})
			near := false
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					near = near || listed[[2]int{cx + dx, cy + dy}]
				}
			}
			if !near {
				t.Fatalf("%v: point %.1f,%.1f in cell %d,%d not covered", b, x, y, cx, cy)
			}
		}
	}
}
