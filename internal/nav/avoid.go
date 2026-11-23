package nav

import (
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
// The zero Avoid avoids nothing.
type Avoid struct {
	cols map[column]struct{}
}

// column is one geodata column.
type column struct{ x, y int }

// AddLine adds the columns the line from a to b crosses, except a's own: the
// rest of a refused segment, a the stop point and b the segment's end.
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

// Len is the number of avoided columns.
func (v *Avoid) Len() int { return len(v.cols) }

// Clear empties the set, for a new route.
func (v *Avoid) Clear() { clear(v.cols) }

// has reports whether the column of c is avoided.
func (v *Avoid) has(c Cell) bool {
	_, ok := v.cols[column{c.X, c.Y}]
	return ok
}

// skips reports whether a search toward goal must not step into c: its
// column is avoided and it is not goal's.
func (v *Avoid) skips(c, goal Cell) bool {
	return len(v.cols) > 0 && (c.X != goal.X || c.Y != goal.Y) && v.has(c)
}

// avoided reports whether the line from a to b crosses an avoided column
// other than those of the route's endpoints from and to. The line cells go
// to s.line.
func (p *Planner) avoided(s *scratch, a, b, from, to location.Location) bool {
	if len(p.Avoid.cols) == 0 {
		return false
	}
	fx, fy := engine.GeoX(from.X), engine.GeoY(from.Y)
	tx, ty := engine.GeoX(to.X), engine.GeoY(to.Y)
	s.line = appendLineCells(s.line[:0], a, b)
	for _, lc := range s.line {
		if lc.x == fx && lc.y == fy || lc.x == tx && lc.y == ty {
			continue
		}
		if _, ok := p.Avoid.cols[column{lc.x, lc.y}]; ok {
			return true
		}
	}
	return false
}
