package nav

import (
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// lineCell is one cell a segment's line crosses, with the stretch of the line
// inside it, in world units from the segment's start.
type lineCell struct {
	x, y   int
	t0, t1 float64
}

// corridorCell is what the corridor test reads of one cell.
type corridorCell struct {
	h     int16
	ok    bool // conditions 1 and 2
	climb bool // condition 3 fails toward a neighbour
}

// stretch is a run of line cells the corridor test fails, as distances along
// the segment from its start.
type stretch struct{ t0, t1 float64 }

// appendLineCells appends, in order, every cell the line from a to b
// crosses (a grid traversal; at an exact corner it steps diagonally, which
// the corridor's one-cell margin covers).
func appendLineCells(dst []lineCell, a, b location.Location) []lineCell {
	ax := float64(a.X-engine.WorldXMin) / block.CellSize
	ay := float64(a.Y-engine.WorldYMin) / block.CellSize
	bx := float64(b.X-engine.WorldXMin) / block.CellSize
	by := float64(b.Y-engine.WorldYMin) / block.CellSize
	cx, cy := engine.GeoX(a.X), engine.GeoY(a.Y)
	ex, ey := engine.GeoX(b.X), engine.GeoY(b.Y)
	length := math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y))

	stepX, tMaxX, tDeltaX := gridStep(ax, bx-ax, cx)
	stepY, tMaxY, tDeltaY := gridStep(ay, by-ay, cy)
	s := 0.0
	for range abs(ex-cx) + abs(ey-cy) + 1 {
		next := min(tMaxX, tMaxY, 1)
		if cx == ex && cy == ey {
			next = 1
		}
		dst = append(dst, lineCell{x: cx, y: cy, t0: s * length, t1: next * length})
		if (cx == ex && cy == ey) || next >= 1 {
			break
		}
		switch {
		case tMaxX < tMaxY:
			cx += stepX
			s, tMaxX = tMaxX, tMaxX+tDeltaX
		case tMaxY < tMaxX:
			cy += stepY
			s, tMaxY = tMaxY, tMaxY+tDeltaY
		default:
			cx += stepX
			cy += stepY
			s, tMaxX, tMaxY = tMaxX, tMaxX+tDeltaX, tMaxY+tDeltaY
		}
	}
	return dst
}

// gridStep is one axis of the grid traversal: the cell step, the line
// parameter of the first border crossing and the parameter between two.
func gridStep(from, delta float64, cell int) (step int, tMax, tDelta float64) {
	switch {
	case delta > 0:
		return 1, (float64(cell+1) - from) / delta, 1 / delta
	case delta < 0:
		return -1, (float64(cell) - from) / delta, -1 / delta
	default:
		return 0, math.Inf(1), math.Inf(1)
	}
}

// quadrant returns the NSWE bits a step from a toward b can cross. The copied
// step chain moves monotonically toward the destination on each axis: a step
// adds (dest - rounded origin) × fraction with fraction < 1 to a float
// position that rounds half up to the origin, so it never passes the
// destination's integer coordinate and never turns back
// (engine.WalkSteps; server move/creature.go stepLocked, and the accurate
// position reseeded at the move start and on arrival). So only the
// quadrant's own directions are ever crossed.
func quadrant(a, b location.Location) block.NSWE {
	var n block.NSWE
	switch {
	case b.X > a.X:
		n |= block.East
	case b.X < a.X:
		n |= block.West
	}
	switch {
	case b.Y > a.Y:
		n |= block.South
	case b.Y < a.Y:
		n |= block.North
	}
	return n
}

// CorridorOpen reports whether the segment from a to b passes the open-corridor
// test (docs/navigation.md, "Step-chain check"): over every cell its line
// crosses plus a one-cell margin, each cell has geodata and exactly one layer,
// its NSWE allows the segment's quadrant directions, and no two neighbouring
// cells differ in height by more than block.CellIgnoreHeight. Then every step
// the server can take between two points of the corridor passes CanMove,
// whatever the step lengths. It costs O(cells along the segment) and makes no
// CanMove call.
func (p *Planner) CorridorOpen(a, b location.Location) bool {
	s := getScratch()
	defer putScratch(s)
	return len(p.corridorStretches(s, a, b)) == 0
}

// corridorStretches runs the open-corridor test on the segment from a to b
// and returns the runs of line cells that fail it, in s.stretches.
func (p *Planner) corridorStretches(s *scratch, a, b location.Location) []stretch {
	s.line = appendLineCells(s.line[:0], a, b)
	clear(s.corridor)
	req := quadrant(a, b)
	for _, lc := range s.line {
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				k := [2]int{lc.x + dx, lc.y + dy}
				if _, ok := s.corridor[k]; !ok {
					s.corridor[k] = p.corridorCell(s, k[0], k[1], req)
				}
			}
		}
	}
	// Condition 3: no climb over 48 between neighbouring corridor cells.
	for k, c := range s.corridor {
		if !c.ok {
			continue
		}
		for _, d := range [2][2]int{{1, 0}, {0, 1}} {
			nk := [2]int{k[0] + d[0], k[1] + d[1]}
			nc, in := s.corridor[nk]
			if !in || !nc.ok {
				continue
			}
			if abs(int(c.h)-int(nc.h)) > block.CellIgnoreHeight {
				c.climb, nc.climb = true, true
				s.corridor[k], s.corridor[nk] = c, nc
			}
		}
	}
	s.stretches = s.stretches[:0]
	for _, lc := range s.line {
		if p.lineCellOpen(s, lc) {
			continue
		}
		if n := len(s.stretches); n > 0 && s.stretches[n-1].t1 >= lc.t0 {
			s.stretches[n-1].t1 = max(s.stretches[n-1].t1, lc.t1)
			continue
		}
		s.stretches = append(s.stretches, stretch{lc.t0, lc.t1})
	}
	return s.stretches
}

// lineCellOpen reports whether every cell around lc passed.
func (p *Planner) lineCellOpen(s *scratch, lc lineCell) bool {
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if c := s.corridor[[2]int{lc.x + dx, lc.y + dy}]; !c.ok || c.climb {
				return false
			}
		}
	}
	return true
}

// corridorCell reads conditions 1 and 2 for one cell: geodata, exactly one
// layer (so belowCell, HeightNearest and Height can only answer that floor),
// and NSWE allowing the quadrant's crossings. A cell whose floor is in water
// fails too, so the exact check walks that stretch point by point.
func (p *Planner) corridorCell(s *scratch, gx, gy int, req block.NSWE) corridorCell {
	if !p.hasGeo(gx, gy) {
		return corridorCell{}
	}
	layers := p.Engine.AppendLayers(s.layers[:0], gx, gy)
	if len(layers) != 1 {
		return corridorCell{}
	}
	h := layers[0].Height
	return corridorCell{h: h, ok: layers[0].NSWE.Allows(req) && !p.Water.wet(gx, gy, h)}
}
