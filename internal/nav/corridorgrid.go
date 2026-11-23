package nav

import "github.com/fatal10110/acis_golang_bot/internal/geo/block"

// corridorGrid is the corridor test's working memory: what the test reads of
// each cell, cached across the tests one pull runs (from one waypoint they
// share most of their cells), and which cells are in the current test's
// corridor. It replaces a map rebuilt per test (#209).
//
// The grid is toroidal: cell (gx, gy) lives in slot (gx mod side, gy mod
// side), and the slot keeps the cell's coordinates, so it answers only for
// its own cell and a cell evicted by another is read again. One test's
// corridor and its neighbours span fewer than side cells on each axis (begin
// grows the grid for a longer segment), so they never share a slot.
type corridorGrid struct {
	side  int // a power of two
	gen   uint32
	mark  uint32
	slots []gridSlot
}

// gridSlot is one cell: what corridorCell read of it when gen is the grid's,
// and its place in the corridor when mark is the grid's.
type gridSlot struct {
	key   uint32
	gen   uint32
	mark  uint32
	h     int16
	nswe  block.NSWE
	flags uint8
}

// gridSlot flags. gridOneLayer and gridWet hold for the whole generation;
// gridOK and gridClimb are the current test's.
const (
	gridOneLayer uint8 = 1 << iota // geodata and exactly one layer
	gridWet                        // that layer's floor is in water
	gridOK                         // conditions 1 and 2 under the test's quadrant
	gridClimb                      // condition 3 fails toward a neighbour
)

// minGridSide covers a navSegment corridor and its neighbours (157 cells
// plus a margin of two on each side) with room for the waypoint's other
// candidates.
const minGridSide = 256

// invalidate drops every cached cell read: the engine may have changed (a
// door toggled) since the last pull.
func (g *corridorGrid) invalidate() {
	g.gen++
	if g.gen == 0 {
		g.wrap()
	}
}

// begin starts a test whose line cells span span cells on their longer axis.
func (g *corridorGrid) begin(span int) {
	if need := span + 5; g.side < need {
		side := max(g.side, minGridSide)
		for side < need {
			side *= 2
		}
		g.side = side
		g.slots = make([]gridSlot, side*side)
		g.gen, g.mark = 1, 0
	}
	g.mark++
	if g.mark == 0 {
		g.wrap()
		g.mark = 1
	}
}

// wrap clears the grid when a counter wraps, so an old slot never passes as
// current.
func (g *corridorGrid) wrap() {
	clear(g.slots)
	g.gen, g.mark = 1, 1
}

func gridKey(gx, gy int) uint32 { return uint32(uint16(gx))<<16 | uint32(uint16(gy)) }

func (g *corridorGrid) at(gx, gy int) *gridSlot {
	m := g.side - 1
	return &g.slots[(gy&m)*g.side+gx&m]
}

// member returns the slot of (gx, gy) when that cell is in the current
// test's corridor, else nil.
func (g *corridorGrid) member(gx, gy int) *gridSlot {
	if c := g.at(gx, gy); c.key == gridKey(gx, gy) && c.gen == g.gen && c.mark == g.mark {
		return c
	}
	return nil
}

// corridorAdd puts cell (gx, gy) into the current test's corridor and
// reports whether the corridor still passes after it: the cell passes
// conditions 1 and 2, and differs by at most block.CellIgnoreHeight from
// every neighbour already in the corridor that passes them (condition 3).
// Each neighbouring pair is checked once, when its second cell joins. A
// cell already in the corridor changes nothing.
func (p *Planner) corridorAdd(s *scratch, gx, gy int, req block.NSWE) bool {
	g := &s.grid
	c := g.at(gx, gy)
	k := gridKey(gx, gy)
	if c.key != k || c.gen != g.gen {
		*c = p.readGridCell(s, gx, gy)
		c.key, c.gen = k, g.gen
	} else if c.mark == g.mark {
		return true
	}
	c.mark = g.mark
	c.flags &^= gridOK | gridClimb
	if c.flags&(gridOneLayer|gridWet) != gridOneLayer || !c.nswe.Allows(req) {
		return false
	}
	c.flags |= gridOK
	open := true
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		n := g.member(gx+d[0], gy+d[1])
		if n == nil || n.flags&gridOK == 0 {
			continue
		}
		if abs(int(c.h)-int(n.h)) > block.CellIgnoreHeight {
			c.flags |= gridClimb
			n.flags |= gridClimb
			open = false
		}
	}
	return open
}

// readGridCell reads what conditions 1 and 2 need of one cell: geodata,
// exactly one layer (so belowCell, HeightNearest and Height can only answer
// that floor), its NSWE, and whether its floor is in water (such a cell
// fails, so the exact check walks that stretch point by point).
func (p *Planner) readGridCell(s *scratch, gx, gy int) gridSlot {
	if !p.hasGeo(gx, gy) {
		return gridSlot{}
	}
	layers := p.Engine.AppendLayers(s.layers[:0], gx, gy)
	if len(layers) != 1 {
		return gridSlot{}
	}
	c := gridSlot{h: layers[0].Height, nswe: layers[0].NSWE, flags: gridOneLayer}
	if p.Water.wet(gx, gy, c.h) {
		c.flags |= gridWet
	}
	return c
}
