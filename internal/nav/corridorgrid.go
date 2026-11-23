package nav

import (
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
)

// corridorGrid is the corridor test's working memory: what the test reads of
// each cell, cached across the tests one pull runs (from one waypoint they
// share most of their cells), and which cells are in the current test's
// corridor. It replaces a map rebuilt per test (#209).
//
// The grid is toroidal and of a fixed size, whatever the segment's length:
// cell (gx, gy) lives in slot (gx mod gridSide, gy mod gridSide), and the
// slot keeps the cell's coordinates, so it answers only for its own cell and
// a cell evicted by another is read again. The corridor is filled in line
// order and the line is monotone on both axes, so two cells sharing a slot
// lie at least gridSide-2 line cells apart, while a cell is needed only
// while line cells at most lineCellLag away are added: no cell is evicted
// while it is still needed, and the memory stays gridSide² slots.
type corridorGrid struct {
	gen   uint32
	mark  uint32
	slots []gridSlot
	// ends are the current test's endpoints, and tops the stacked cells
	// it took on their tops (corridorAdd).
	ends [2]gridEnd
	tops [][2]int
}

// gridEnd is a segment endpoint: its cell and the Z it stands at.
type gridEnd struct {
	gx, gy, z int
}

// gridSlot is one cell: what corridorCell read of it when gen is the grid's,
// and its place in the corridor when mark is the grid's.
type gridSlot struct {
	key   uint32
	gen   uint32
	mark  uint32
	h     int16
	gap   int16 // a stacked cell's top minus its next layer down
	nswe  block.NSWE
	flags uint8
}

// gridSlot flags. gridOneLayer, gridStacked and gridWet hold for the whole
// generation; gridOK and gridClimb are the current test's.
const (
	gridOneLayer uint8 = 1 << iota // geodata and exactly one layer
	gridStacked                    // geodata and several layers: h is the top's
	gridWet                        // the floor (the one layer, or the top) is in water
	gridOK                         // conditions 1 and 2 under the test's quadrant
	gridClimb                      // condition 3 fails toward a neighbour
)

// gridSide is the grid's side, a power of two: a navSegment corridor and its
// neighbours (157 cells plus a margin of two on each side) fit, so one
// pull's candidates from a waypoint share their cached cells.
const gridSide = 256

// invalidate drops every cached cell read: the engine may have changed (a
// door toggled) since the last pull.
func (g *corridorGrid) invalidate() {
	g.gen++
	if g.gen == 0 {
		g.wrap()
	}
}

// begin starts a test.
func (g *corridorGrid) begin() {
	if g.slots == nil {
		g.slots = make([]gridSlot, gridSide*gridSide)
		g.gen, g.mark = 1, 0
	}
	g.mark++
	if g.mark == 0 {
		g.wrap()
		g.mark = 1
	}
	g.tops = g.tops[:0]
}

// wrap clears the grid when a counter wraps, so an old slot never passes as
// current.
func (g *corridorGrid) wrap() {
	clear(g.slots)
	g.gen, g.mark = 1, 1
}

func gridKey(gx, gy int) uint32 { return uint32(uint16(gx))<<16 | uint32(uint16(gy)) }

func (g *corridorGrid) at(gx, gy int) *gridSlot {
	const m = gridSide - 1
	return &g.slots[(gy&m)*gridSide+gx&m]
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
// conditions 1 and 2, and differs by less than block.CellIgnoreHeight from
// every neighbour already in the corridor that passes them (condition 3).
// With stacked, a stacked cell passes condition 1 on its top, unless an
// endpoint of the segment stands in it below the top, and joins g.tops for
// topsHold. Each neighbouring pair is checked once, when its second cell
// joins. A cell already in the corridor changes nothing.
func (p *Planner) corridorAdd(s *scratch, gx, gy int, req block.NSWE, stacked bool) bool {
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
	floor := gridOneLayer
	if stacked {
		floor |= gridStacked
	}
	if c.flags&floor == 0 || c.flags&gridWet != 0 || !c.nswe.Allows(req) || c.flags&gridStacked != 0 && g.below(gx, gy, c.h) {
		return false
	}
	c.flags |= gridOK
	if c.flags&gridStacked != 0 {
		g.tops = append(g.tops, [2]int{gx, gy})
	}
	open := true
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		n := g.member(gx+d[0], gy+d[1])
		if n == nil || n.flags&gridOK == 0 {
			continue
		}
		if abs(int(c.h)-int(n.h)) >= block.CellIgnoreHeight {
			c.flags |= gridClimb
			n.flags |= gridClimb
			open = false
		}
	}
	return open
}

// below reports whether an endpoint of the current test stands in cell
// (gx, gy) below its top h.
func (g *corridorGrid) below(gx, gy int, h int16) bool {
	for _, e := range g.ends {
		if e.gx == gx && e.gy == gy && e.z != int(h) {
			return true
		}
	}
	return false
}

// readGridCell reads what conditions 1 and 2 need of one cell: geodata,
// exactly one layer (so belowCell, HeightNearest and Height can only answer
// that floor) or else its top and the gap below it (gridStacked), the
// floor's NSWE, and whether that floor is in water (such a cell fails, so
// the exact check walks that stretch point by point).
func (p *Planner) readGridCell(s *scratch, gx, gy int) gridSlot {
	if !p.hasGeo(gx, gy) {
		return gridSlot{}
	}
	layers := p.Engine.AppendLayers(s.layers[:0], gx, gy)
	top := 0
	for i, l := range layers {
		if l.Height > layers[top].Height {
			top = i
		}
	}
	c := gridSlot{h: layers[top].Height, nswe: layers[top].NSWE, flags: gridOneLayer}
	if len(layers) > 1 {
		c.flags, c.gap = gridStacked, math.MaxInt16
		for i, l := range layers {
			if i != top {
				c.gap = min(c.gap, c.h-l.Height)
			}
		}
		if c.gap == 0 {
			return gridSlot{} // two layers at the top: no one floor
		}
	}
	if p.Water.wet(gx, gy, c.h) {
		c.flags |= gridWet
	}
	return c
}

// topsHold reports whether every stacked cell the corridor took on its top
// keeps it for a chain on the corridor's floors. A step of at most
// Lmax + stepSlack from cell C lands within k cells of it and asks
// Height(N, T_C + 2*CellHeight), T_C the floor of C, for the cell N it lands
// in; the nearest layer to that is N's top T_N when the top is nearer than
// the layer below it, at T_N - gap: always when T_N is at or below the
// query, else when 2*(T_N - T_C - 2*CellHeight) < gap (a tie goes to the
// lower layer). Every other lookup of the chain stays on the tops whatever
// the gap (docs/navigation.md, "Step-chain check").
func (p *Planner) topsHold(s *scratch) bool {
	k := int((p.Lmax+stepSlack)/block.CellSize) + 1
	g := &s.grid
	for _, t := range g.tops {
		n := g.member(t[0], t[1])
		for dx := -k; dx <= k; dx++ {
			for dy := -k; dy <= k; dy++ {
				if c := g.member(t[0]+dx, t[1]+dy); c != nil && 2*(int(n.h)-int(c.h)-2*block.CellHeight) >= int(n.gap) {
					return false
				}
			}
		}
	}
	return true
}
