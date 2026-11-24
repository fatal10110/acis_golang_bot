package nav

import (
	"cmp"
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
	// layers holds the layers of the layered cells read this generation;
	// a layered slot points into it.
	layers []block.Cell
	// ends are the current test's endpoints, line the index of the line
	// cell being filled, and layered the layered cells the corridor took,
	// with the line cell each joined at (landsAround).
	ends    [2]gridEnd
	line    int
	layered []layeredRef
}

// gridEnd is a segment endpoint: its cell and the Z it stands at.
type gridEnd struct {
	gx, gy, z int
}

// layeredRef is a layered cell of the current corridor and the index of the
// line cell it joined at.
type layeredRef struct {
	gx, gy, line int
}

// gridSlot is one cell: what readGridCell read of it when gen is the grid's,
// and its place in the corridor when mark is the grid's.
type gridSlot struct {
	key  uint32
	gen  uint32
	mark uint32
	// li and ln locate a layered cell's layers in corridorGrid.layers.
	li uint32
	ln uint8
	// h is the cell's floor: the one layer, or for a layered cell the
	// walking layer the current test gave it (gridFloor). up and down are
	// the distances to the next layer above and below it, noLayer for none;
	// nswe is the floor's.
	h, up, down int16
	nswe        block.NSWE
	flags       uint8
}

// noLayer is a gridSlot's up or down when no layer lies that way.
const noLayer = math.MaxInt16

// gridSlot flags. gridOneLayer, gridLayered and gridWet hold for the whole
// generation; gridFloor, gridOK and gridClimb are the current test's.
const (
	gridOneLayer uint8 = 1 << iota // geodata and exactly one layer
	gridLayered                    // geodata and several layers
	gridWet                        // the one layer is in water
	gridFloor                      // h holds the cell's walking layer
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
	g.layers = g.layers[:0]
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
	g.layered = g.layered[:0]
	g.line = 0
}

// wrap clears the grid when a counter wraps, so an old slot never passes as
// current.
func (g *corridorGrid) wrap() {
	clear(g.slots)
	g.gen, g.mark = 1, 1
	g.layers = g.layers[:0]
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

// floorAt returns the walking layer the current test gave cell (gx, gy).
func (g *corridorGrid) floorAt(gx, gy int) (int16, bool) {
	c := g.member(gx, gy)
	if c == nil || c.flags&gridFloor == 0 {
		return 0, false
	}
	return c.h, true
}

// corridorAdd puts cell (gx, gy) into the current test's corridor and
// reports whether the corridor still passes after it (docs/navigation.md,
// "Step-chain check"). The cell gets its walking layer: its one layer, or
// for a layered cell the layer the chain stands on there (layeredFloor).
// It passes conditions 1 and 2 when that floor is dry and its NSWE allows
// req; condition 3 holds toward every neighbour already in the corridor
// that passes them (floorsLinked), and the layer rule toward every such cell
// within one step (landsAround). Each pair is checked once, when its second
// cell joins. A cell already in the corridor changes nothing.
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
	s.work++
	c.mark = g.mark
	c.flags &^= gridFloor | gridOK | gridClimb
	switch {
	case c.flags&gridOneLayer != 0:
		if c.flags&gridWet != 0 {
			c.flags |= gridFloor
			return false
		}
	case c.flags&gridLayered != 0 && p.Lmax > 0:
		if !p.layeredFloor(s, c, gx, gy, req) {
			return false
		}
	default:
		return false
	}
	c.flags |= gridFloor
	if !c.nswe.Allows(req) {
		return false
	}
	c.flags |= gridOK
	open := true
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		n := g.member(gx+d[0], gy+d[1])
		if n == nil || n.flags&gridOK == 0 {
			continue
		}
		if !floorsLinked(n, c, req.Allows(stepDir(-d[0], -d[1])), req.Allows(stepDir(d[0], d[1]))) {
			c.flags |= gridClimb
			n.flags |= gridClimb
			open = false
		}
	}
	if c.flags&gridLayered != 0 {
		g.layered = append(g.layered, layeredRef{gx, gy, g.line})
	}
	if !p.landsAround(g, gx, gy, c) {
		open = false
	}
	return open
}

// stepDir is the NSWE direction of a move by (dx, dy) cells.
func stepDir(dx, dy int) block.NSWE {
	var n block.NSWE
	switch {
	case dx > 0:
		n |= block.East
	case dx < 0:
		n |= block.West
	}
	switch {
	case dy > 0:
		n |= block.South
	case dy < 0:
		n |= block.North
	}
	return n
}

// floorsLinked reports whether neighbouring cells n and c keep the chain on their
// walking layers: they differ by less than block.CellIgnoreHeight
// (condition 3), and CanMove's crossing from one into the other, where the
// segment's quadrant crosses that way (intoC from n into c, intoN back),
// lands on the walking layer: belowCell(c, L_n + CellIgnoreHeight) is the
// highest layer strictly below that height (server geo/engine
// engine_query.go CanMove).
func floorsLinked(n, c *gridSlot, intoC, intoN bool) bool {
	if abs(int(c.h)-int(n.h)) >= block.CellIgnoreHeight {
		return false
	}
	return (!intoC || c.crossesFrom(n.h)) && (!intoN || n.crossesFrom(c.h))
}

// crossesFrom reports whether CanMove crossing into c from a floor at z lands
// on c's walking layer: it is the highest layer of c below
// z + CellIgnoreHeight.
func (c *gridSlot) crossesFrom(z int16) bool {
	q := int(z) + block.CellIgnoreHeight
	return int(c.h) < q && (c.up == noLayer || int(c.h)+int(c.up) >= q)
}

// lands reports whether a step from a floor at z that ends in c lands on c's
// walking layer: the chain re-floors at Height(c, z + 2*CellHeight), the
// nearest layer, of two equally near the lower (block.Multilayer.Nearest).
func (c *gridSlot) lands(z int16) bool {
	d := int(z) + 2*block.CellHeight - int(c.h)
	if d >= 0 {
		return c.up == noLayer || 2*d <= int(c.up)
	}
	return c.down == noLayer || -2*d < int(c.down)
}

// stepCells is how many cells a step of at most Lmax + 2*stepSlack between
// rounded points can move on each axis.
func (p *Planner) stepCells() int {
	return int((p.Lmax+2*stepSlack)/block.CellSize) + 1
}

// lineCellLag is how many line cells after lc the corridor fill must reach
// before lc's verdict is final: a flag of a cell around lc changes only when
// a cell within stepCells of it joins, around a line cell within
// stepCells+2 of lc in Chebyshev distance, and the line, monotone on both
// axes, gets farther than that within twice as many cells.
func (p *Planner) lineCellLag() int {
	return 2 * (max(p.stepCells(), 1) + 2)
}

// landsAround applies the layer rule to cell c, just joined at (gx, gy) with
// a walking layer: a step from any corridor cell O within one step that ends
// in a layered cell C must land on L_C, Height(C, L_O + 2*CellHeight) =
// L_C. c is checked as C against every such O, and as O against every
// layered C. A layered cell that fails is flagged gridClimb, and the
// corridor fails.
func (p *Planner) landsAround(g *corridorGrid, gx, gy int, c *gridSlot) bool {
	k := p.stepCells()
	ok := true
	if c.flags&gridLayered != 0 {
		for dx := -k; dx <= k; dx++ {
			for dy := -k; dy <= k; dy++ {
				if o := g.member(gx+dx, gy+dy); o != nil && o.flags&gridOK != 0 && !c.lands(o.h) {
					c.flags |= gridClimb
					ok = false
				}
			}
		}
	}
	reach := 2 * (k + 2)
	for i := len(g.layered) - 1; i >= 0 && g.line-g.layered[i].line <= reach; i-- {
		l := g.layered[i]
		if (l.gx == gx && l.gy == gy) || abs(l.gx-gx) > k || abs(l.gy-gy) > k {
			continue
		}
		if n := g.member(l.gx, l.gy); n != nil && n.flags&gridOK != 0 && !n.lands(c.h) {
			n.flags |= gridClimb
			ok = false
		}
	}
	return ok
}

// layeredFloor gives layered cell c its walking layer for the current test:
// at the segment's start the layer nearest the start's Z (CanMove's origin
// floor), else the layer CanMove crosses onto from a neighbour already on
// its floor (belowCell(c, L_n + CellIgnoreHeight)), a neighbour the
// quadrant crosses from first. A cell no neighbour reaches, the segment's
// end off its walking layer, or a floor in water fails.
func (p *Planner) layeredFloor(s *scratch, c *gridSlot, gx, gy int, req block.NSWE) bool {
	g := &s.grid
	layers := g.layers[c.li : c.li+uint32(c.ln)]
	var at int
	switch e := g.ends[0]; {
	case e.gx == gx && e.gy == gy:
		at = nearestLayer(layers, e.z)
	default:
		from, ok := g.anchor(gx, gy, req)
		if !ok {
			return false
		}
		if at = belowLayer(layers, int(from)+block.CellIgnoreHeight); at < 0 {
			return false
		}
	}
	if e := g.ends[1]; e.gx == gx && e.gy == gy && layers[at].Height != layers[nearestLayer(layers, e.z)].Height {
		return false
	}
	c.h, c.nswe = layers[at].Height, layers[at].NSWE
	c.up, c.down = noLayer, noLayer
	for i, l := range layers {
		switch {
		case i == at:
		case l.Height >= c.h:
			c.up = min(c.up, l.Height-c.h)
		default:
			c.down = min(c.down, c.h-l.Height)
		}
	}
	return !p.Water.wet(gx, gy, c.h)
}

// anchor returns the walking layer of a neighbour of (gx, gy) already in the
// corridor on its floor: a 4-neighbour the quadrant crosses from into the
// cell first, then any 4-neighbour, then a diagonal one.
func (g *corridorGrid) anchor(gx, gy int, req block.NSWE) (int16, bool) {
	var other, diag *gridSlot
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			n := g.member(gx+dx, gy+dy)
			if (dx == 0 && dy == 0) || n == nil || n.flags&gridOK == 0 {
				continue
			}
			switch {
			case dx != 0 && dy != 0:
				diag = cmp.Or(diag, n)
			case req.Allows(stepDir(-dx, -dy)):
				return n.h, true
			default:
				other = cmp.Or(other, n)
			}
		}
	}
	if n := cmp.Or(other, diag); n != nil {
		return n.h, true
	}
	return 0, false
}

// nearestLayer is the index of the layer nearest z, of two equally near the
// lower (block.Multilayer.Nearest).
func nearestLayer(layers []block.Cell, z int) int {
	best := 0
	for i, l := range layers {
		d, bd := abs(int(l.Height)-z), abs(int(layers[best].Height)-z)
		if d < bd || d == bd && l.Height < layers[best].Height {
			best = i
		}
	}
	return best
}

// belowLayer is the index of the highest layer strictly below z, or -1
// (block.Multilayer.Below).
func belowLayer(layers []block.Cell, z int) int {
	best := -1
	for i, l := range layers {
		if int(l.Height) < z && (best < 0 || l.Height > layers[best].Height) {
			best = i
		}
	}
	return best
}

// readGridCell reads what the corridor test needs of one cell: geodata, and
// either its one layer (so belowCell, HeightNearest and Height can only
// answer that floor), its NSWE and whether it is in water (such a cell
// fails, so the exact check walks that stretch point by point), or for a
// cell of several layers the layers themselves, kept in g.layers until the
// next invalidate.
func (p *Planner) readGridCell(s *scratch, gx, gy int) gridSlot {
	if !p.hasGeo(gx, gy) {
		return gridSlot{}
	}
	layers := p.Engine.AppendLayers(s.layers[:0], gx, gy)
	if len(layers) > 1 {
		g := &s.grid
		c := gridSlot{li: uint32(len(g.layers)), ln: uint8(len(layers)), flags: gridLayered}
		g.layers = append(g.layers, layers...)
		return c
	}
	c := gridSlot{h: layers[0].Height, up: noLayer, down: noLayer, nswe: layers[0].NSWE, flags: gridOneLayer}
	if p.Water.wet(gx, gy, c.h) {
		c.flags |= gridWet
	}
	return c
}
