package nav

import (
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Avoid is the geodata columns one route keeps out of: the rest of each
// segment the server refused with a blocked stop (docs/navigation.md,
// "Server answers to its own moves"). It belongs to one planner and one
// route; the shared hierarchy and nav cache never see it. It holds columns,
// not cells: a route keeps off every layer of an avoided column, which costs
// at most a layer above or below a refused stretch and needs no layer guess
// for the points of a refused line. A route's own start and goal columns are
// never avoided, so a set holding them cannot fail a route by itself.
//
// Area, when set, adds ground the caller keeps a margin from: a karma
// bot's towns and guards (docs/pvp/karma.md "With karma"). It reports
// whether the column whose centre is at world (x, y) is avoided. Clear
// leaves it: the caller sets it for each plan.
//
// The zero Avoid avoids nothing.
type Avoid struct {
	cols map[column]struct{}
	Area func(x, y int) bool
}

// column is one geodata column.
type column struct{ x, y int }

// AddLine adds the columns the line from a to b crosses, except a's own: the
// refused part of a segment, a the stop point and b where BlockedEnd puts
// its end.
func (v *Avoid) AddLine(a, b location.Location) {
	line := appendLineCells(nil, a, b)
	if len(line) < 2 {
		return
	}
	if v.cols == nil {
		v.cols = make(map[column]struct{}, len(line))
	}
	for _, lc := range line[1:] {
		v.cols[column{lc.x, lc.y}] = struct{}{}
	}
}

// BlockedEnd returns where the refused part of the segment from a to b ends
// for a blocked stop at stop (docs/navigation.md, "Server answers to its own
// moves"). The server stops before the step that failed (acis_golang
// model/actor/move/creature.go), so that step starts at the stop and spans
// at most Lmax: the refused part runs one step and a cell past the stop,
// then on to the end of the first stretch beyond that the corridor test
// fails, as far as the exact check walked it. A stop inside such a stretch
// refuses the whole rest of the segment: there the chain stood where only
// the exact check vouched for it.
func (p *Planner) BlockedEnd(a, b, stop location.Location) location.Location {
	length := hypot(b.X-a.X, b.Y-a.Y)
	if length == 0 {
		return b
	}
	ux, uy := float64(b.X-a.X)/length, float64(b.Y-a.Y)/length
	ts := float64(stop.X-a.X)*ux + float64(stop.Y-a.Y)*uy
	end := ts + p.Lmax + block.CellSize
	s := getScratch()
	defer putScratch(s)
	s.grid.invalidate()
	for _, st := range p.corridorStretches(s, a, b) {
		t0, t1 := st.t0-stretchPad, st.t1+stretchPad
		if t0 <= ts && ts <= t1 {
			return b
		}
		if t1 > end {
			end = t1
			break
		}
	}
	if end >= length {
		return b
	}
	return location.Location{X: a.X + int(math.Round(ux*end)), Y: a.Y + int(math.Round(uy*end)), Z: b.Z}
}

// Len is the number of avoided columns.
func (v *Avoid) Len() int { return len(v.cols) }

// Clear empties the set of refused columns, for a new route.
func (v *Avoid) Clear() { clear(v.cols) }

// empty reports whether v avoids nothing.
func (v *Avoid) empty() bool { return len(v.cols) == 0 && v.Area == nil }

// has reports whether the column of c is avoided.
func (v *Avoid) has(c Cell) bool { return v.hasColumn(c.X, c.Y) }

// hasColumn reports whether the geodata column (x, y) is avoided: a
// refused column, or one in Area.
func (v *Avoid) hasColumn(x, y int) bool {
	if _, ok := v.cols[column{x, y}]; ok {
		return true
	}
	return v.Area != nil && v.Area(engine.WorldX(x), engine.WorldY(y))
}

// skips reports whether a search toward goal must not step into c: its
// column is avoided and it is not goal's.
func (v *Avoid) skips(c, goal Cell) bool {
	return !v.empty() && (c.X != goal.X || c.Y != goal.Y) && v.has(c)
}

// avoided reports whether the line from a to b crosses an avoided column
// other than those of the route's endpoints from and to. The line cells go
// to s.line.
func (p *Planner) avoided(s *scratch, a, b, from, to location.Location) bool {
	if p.Avoid.empty() {
		return false
	}
	fx, fy := engine.GeoX(from.X), engine.GeoY(from.Y)
	tx, ty := engine.GeoX(to.X), engine.GeoY(to.Y)
	s.line = appendLineCells(s.line[:0], a, b)
	for _, lc := range s.line {
		if lc.x == fx && lc.y == fy || lc.x == tx && lc.y == ty {
			continue
		}
		if p.Avoid.hasColumn(lc.x, lc.y) {
			return true
		}
	}
	return false
}
