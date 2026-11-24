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
// crosses and every cell next to those within corridorReach of it, each cell
// has geodata and a walking layer carried from the segment's start floor
// (corridorAdd), the floor is dry and its NSWE allows the segment's
// quadrant directions, neighbouring floors differ in height by less than
// block.CellIgnoreHeight and CanMove crosses between them onto them, and a
// step of at most Lmax + 2*stepSlack into a cell of several layers lands on
// its walking layer. Then every step the server can take between two points
// of the corridor passes CanMove: on cells of one layer whatever its length,
// on cells of several for steps within that bound. It costs O(cells along
// the segment) and makes no CanMove call.
func (p *Planner) CorridorOpen(a, b location.Location) bool {
	s := getScratch()
	defer putScratch(s)
	s.grid.invalidate()
	return p.corridorClear(s, a, b)
}

// corridorStretches runs the open-corridor test on the segment from a to b
// and returns the runs of line cells that fail it, in s.stretches. The
// corridor's walking layers stay in s.grid for the exact check.
func (p *Planner) corridorStretches(s *scratch, a, b location.Location) []stretch {
	p.corridorFill(s, a, b, false)
	return s.stretches
}

// addStretch appends lc to s.stretches when it fails, merging it into the
// last stretch when they touch.
func (p *Planner) addStretch(s *scratch, lc lineCell) {
	if p.lineCellOpen(s, lc) {
		return
	}
	if n := len(s.stretches); n > 0 && s.stretches[n-1].t1 >= lc.t0 {
		s.stretches[n-1].t1 = max(s.stretches[n-1].t1, lc.t1)
		return
	}
	s.stretches = append(s.stretches, stretch{lc.t0, lc.t1})
}

// corridorClear reports whether the segment from a to b passes the
// open-corridor test, stopping at the first cell that fails: every corridor
// cell lies around some line cell, so the test fails exactly when a corridor
// cell fails conditions 1 and 2, its link to a neighbour or the layer rule,
// and corridorAdd sees each such failure as it adds the cell.
func (p *Planner) corridorClear(s *scratch, a, b location.Location) bool {
	return p.corridorFill(s, a, b, true)
}

// corridorFill puts every cell the line from a to b crosses, and every cell
// next to one of those within corridorReach of the segment, into a new
// corridor of s.grid, the line cells in s.line. Each line cell joins before
// the cells around it, so the walking layer spreads from the start's cell.
// With stop it returns false at the first cell that makes the test fail;
// else it fills the whole corridor, collects the failing runs of line cells
// in s.stretches as their verdicts become final (lineCellLag), and reports
// whether it passed.
func (p *Planner) corridorFill(s *scratch, a, b location.Location, stop bool) bool {
	s.line = appendLineCells(s.line[:0], a, b)
	s.stretches = s.stretches[:0]
	s.grid.begin()
	s.grid.ends = [2]gridEnd{{engine.GeoX(a.X), engine.GeoY(a.Y), a.Z}, {engine.GeoX(b.X), engine.GeoY(b.Y), b.Z}}
	req := quadrant(a, b)
	lag := p.lineCellLag()
	open := true
	seg := newSegment(a, b)
	for i, lc := range s.line {
		s.grid.line = i
		for k := range 9 {
			// The line cell first, then the ring around it.
			dx, dy := (k+4)%9%3-1, (k+4)%9/3-1
			if k > 0 && (s.grid.member(lc.x+dx, lc.y+dy) != nil || !seg.nearCell(lc.x+dx, lc.y+dy)) {
				continue // in the corridor already, or out of reach
			}
			if !p.corridorAdd(s, lc.x+dx, lc.y+dy, req) {
				if stop {
					return false
				}
				open = false
			}
		}
		if !stop && i >= lag {
			p.addStretch(s, s.line[i-lag])
		}
	}
	if !stop {
		for _, lc := range s.line[max(len(s.line)-lag, 0):] {
			p.addStretch(s, lc)
		}
	}
	return open
}

// lineCellOpen reports whether every corridor cell around lc passed.
func (p *Planner) lineCellOpen(s *scratch, lc lineCell) bool {
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			c := s.grid.member(lc.x+dx, lc.y+dy)
			if c == nil {
				continue // not within corridorReach of the segment
			}
			if c.flags&(gridOK|gridClimb) != gridOK {
				return false
			}
		}
	}
	return true
}

// corridorReach is how far from a segment a cell next to a line cell may
// lie and still join the corridor: past the 1.5 units the rounded step
// points stray from the line (bandRadius), CanMove's border walk between two
// of them strays about 1.5 more, as it picks each crossing from the line's
// coordinate at the cell's last column, truncated to an integer. 4 keeps a
// margin over the 3.
const corridorReach = 4.0

// segment is a segment in world units, for corridor distances.
type segment struct {
	ax, ay, dx, dy, len2 float64
	// nx, ny is the line's unit normal, and half a cell's half extent
	// along it; zero for a point.
	nx, ny, half float64
}

func newSegment(a, b location.Location) segment {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	sg := segment{ax: float64(a.X), ay: float64(a.Y), dx: dx, dy: dy, len2: dx*dx + dy*dy}
	if l := math.Sqrt(sg.len2); l > 0 {
		sg.nx, sg.ny = -dy/l, dx/l
		sg.half = block.CellSize / 2 * (math.Abs(sg.nx) + math.Abs(sg.ny))
	}
	return sg
}

// dist is the distance from point m to the segment.
func (sg segment) dist(m [2]int) float64 {
	px, py := float64(m[0])-sg.ax, float64(m[1])-sg.ay
	t := 0.0
	if sg.len2 > 0 {
		t = min(max((px*sg.dx+py*sg.dy)/sg.len2, 0), 1)
	}
	return math.Hypot(px-t*sg.dx, py-t*sg.dy)
}

// nearCell reports whether geodata cell (gx, gy), a closed square, comes
// within corridorReach of the segment. The cell is one the segment does not
// cross, so the nearest points are a segment end or a corner of the cell.
func (sg segment) nearCell(gx, gy int) bool {
	x0 := float64(gx*block.CellSize + engine.WorldXMin)
	y0 := float64(gy*block.CellSize + engine.WorldYMin)
	x1, y1 := x0+block.CellSize, y0+block.CellSize
	// The line through the segment is no farther from the cell than the
	// segment: a cell well clear of the line is out of reach.
	if sg.half > 0 && math.Abs((x0+block.CellSize/2-sg.ax)*sg.nx+(y0+block.CellSize/2-sg.ay)*sg.ny)-sg.half > corridorReach+1e-6 {
		return false
	}
	const r2 = corridorReach * corridorReach
	for _, e := range [2][2]float64{{sg.ax, sg.ay}, {sg.ax + sg.dx, sg.ay + sg.dy}} {
		ex := max(x0-e[0], 0, e[0]-x1)
		ey := max(y0-e[1], 0, e[1]-y1)
		if ex*ex+ey*ey <= r2 {
			return true
		}
	}
	for _, c := range [4][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		px, py := c[0]-sg.ax, c[1]-sg.ay
		t := 0.0
		if sg.len2 > 0 {
			t = min(max((px*sg.dx+py*sg.dy)/sg.len2, 0), 1)
		}
		qx, qy := px-t*sg.dx, py-t*sg.dy
		if qx*qx+qy*qy <= r2 {
			return true
		}
	}
	return false
}
