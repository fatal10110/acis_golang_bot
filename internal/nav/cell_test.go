package nav

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

func TestLocate(t *testing.T) {
	p := testPlanner(t)
	tests := []struct {
		name  string
		at    location.Location
		wantZ int16
		ok    bool
	}{
		{"open ground", at(Open, 3, -2, 40), 0, true},
		{"bridge deck by Z", at(geotest.BridgeWestDeck, 0, 0, 10), geotest.Deck, true},
		{"under the bridge by Z", at(geotest.BridgeWestGround, 0, 0, 10), geotest.Ground, true},
		{"plateau", at(geotest.LedgeHigh, 0, 0, 0), geotest.Plateau, true},
		{"unloaded region", at(geotest.Point{CellX: -5, CellY: 100}, 0, 0, 0), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := p.Locate(tt.at)
			if ok != tt.ok || (ok && c.Z != tt.wantZ) {
				t.Fatalf("Locate(%v) = %v, %v; want Z %d, %v", tt.at, c, ok, tt.wantZ, tt.ok)
			}
		})
	}
}

// TestEdgesFollowCanMove checks every cardinal edge out of every layer of
// the cells around each scene against CanMove between cell centres: an edge
// exists exactly when CanMove allows that crossing, and lands on the layer
// CanMove reaches. Diagonal edges must pass CanMove too.
func TestEdgesFollowCanMove(t *testing.T) {
	p := testPlanner(t)
	e := p.Engine
	scenes := []geotest.Point{
		geotest.CorridorWest, geotest.AboveCorridor, geotest.UInside, geotest.LedgeLow,
		geotest.PitOutside, geotest.BridgeWestGround, geotest.BridgeEastGround, geotest.PocketOutside,
		StairLow, StackedGround,
	}
	var buf, nbuf [block.MaxLayers]block.Cell
	var edges []neighbour
	checked := 0
	for _, sc := range scenes {
		cx, cy := cellOf(at(sc, 0, 0, 0))
		for x := cx - 14; x <= cx+14; x++ {
			for y := cy - 14; y <= cy+14; y++ {
				for _, l := range e.AppendLayers(buf[:0], x, y) {
					c := Cell{X: x, Y: y, Z: l.Height}
					edges = p.neighbours(edges[:0], c)
					from := c.World()
					for _, d := range cardinals {
						var got *Cell
						for i := range edges {
							if edges[i].cell.X == x+d.dx && edges[i].cell.Y == y+d.dy {
								got = &edges[i].cell
							}
						}
						allowed := false
						for _, nl := range e.AppendLayers(nbuf[:0], x+d.dx, y+d.dy) {
							to := Cell{X: x + d.dx, Y: y + d.dy, Z: nl.Height}.World()
							if e.CanMove(from.X, from.Y, from.Z, to.X, to.Y, to.Z) {
								allowed = true
								if got == nil || got.Z != nl.Height {
									t.Fatalf("%v -> %v: CanMove allows, edge %v", c, to, got)
								}
							}
						}
						if !allowed && got != nil {
							t.Fatalf("%v -> %v: edge without CanMove", c, *got)
						}
						checked++
					}
					for _, n := range edges {
						to := n.cell.World()
						if !e.CanMove(from.X, from.Y, from.Z, to.X, to.Y, to.Z) {
							t.Fatalf("%v -> %v: edge fails CanMove", c, n.cell)
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no edge checked")
	}
}

func TestEdges(t *testing.T) {
	p := testPlanner(t)
	tests := []struct {
		name     string
		from, to geotest.Point
		want     bool
	}{
		{"open ground", Open, geotest.Point{CellX: Open.CellX + 1, CellY: Open.CellY}, true},
		{"open diagonal", Open, geotest.Point{CellX: Open.CellX + 1, CellY: Open.CellY + 1}, true},
		{"into the corridor's north wall", geotest.Point{CellX: 270, CellY: 260}, geotest.Point{CellX: 270, CellY: 259, Z: geotest.Wall}, false},
		{"diagonal into the corridor wall", geotest.Point{CellX: 270, CellY: 260}, geotest.Point{CellX: 271, CellY: 259, Z: geotest.Wall}, false},
		{"climb onto the ledge", geotest.Point{CellX: 799, CellY: 285}, geotest.Point{CellX: 800, CellY: 285, Z: geotest.Plateau}, false},
		{"drop off the ledge", geotest.Point{CellX: 800, CellY: 285, Z: geotest.Plateau}, geotest.Point{CellX: 799, CellY: 285}, true},
		{"jump into the pit", geotest.Point{CellX: 1039, CellY: 287}, geotest.Point{CellX: 1040, CellY: 287, Z: geotest.PitDown}, true},
		{"climb out of the pit", geotest.Point{CellX: 1040, CellY: 287, Z: geotest.PitDown}, geotest.Point{CellX: 1039, CellY: 287}, false},
		{"stair step up 40", geotest.Point{CellX: 199, CellY: 1005}, geotest.Point{CellX: 200, CellY: 1005, Z: 40}, true},
		{"stair onto the upper layer", geotest.Point{CellX: 200, CellY: 1005, Z: 40}, geotest.Point{CellX: 201, CellY: 1005, Z: 80}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx, fy := cellOf(at(tt.from, 0, 0, 0))
			tx, ty := cellOf(at(tt.to, 0, 0, 0))
			from := Cell{X: fx, Y: fy, Z: int16(tt.from.Z)}
			want := Cell{X: tx, Y: ty, Z: int16(tt.to.Z)}
			got := false
			for _, n := range p.neighbours(nil, from) {
				got = got || n.cell == want
			}
			if got != tt.want {
				t.Fatalf("edge %v -> %v = %v, want %v", from, want, got, tt.want)
			}
		})
	}
}

func TestClearanceCost(t *testing.T) {
	p := testPlanner(t)
	cost := func(pt geotest.Point) int32 {
		x, y := cellOf(at(pt, 0, 0, 0))
		edges := p.neighbours(nil, Cell{X: x, Y: y, Z: int16(pt.Z)})
		for _, n := range edges {
			if n.cell.X != x && n.cell.Y == y {
				return n.cost
			}
		}
		t.Fatalf("no east-west edge out of %v", pt)
		return 0
	}
	if got := cost(Open); got != costCardinal {
		t.Errorf("open ground step = %d, want %d", got, costCardinal)
	}
	if got := cost(geotest.Point{CellX: 270, CellY: 260}); got != costCardinal+costClearance {
		t.Errorf("step along the corridor wall = %d, want %d", got, costCardinal+costClearance)
	}
	if got := cost(geotest.Point{CellX: 800, CellY: 285, Z: geotest.Plateau}); got != costCardinal+costClearance {
		t.Errorf("step at the ledge's drop = %d, want %d", got, costCardinal+costClearance)
	}
}

func TestClickable(t *testing.T) {
	p := testPlanner(t)
	var buf [block.MaxLayers]block.Cell
	tests := []struct {
		name string
		pt   geotest.Point
		want bool
	}{
		{"single layer", Open, true},
		{"ground under a deck 160 up", geotest.BridgeWestGround, true},
		{"deck", geotest.BridgeWestDeck, true},
		{"ground under a layer 30 up", StackedGround, false},
		{"layer 30 up", geotest.Point{CellX: StackedGround.CellX, CellY: StackedGround.CellY, Z: 30}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y := cellOf(at(tt.pt, 0, 0, 0))
			if got := p.clickable(Cell{X: x, Y: y, Z: int16(tt.pt.Z)}, &buf); got != tt.want {
				t.Fatalf("clickable = %v, want %v", got, tt.want)
			}
		})
	}
}
