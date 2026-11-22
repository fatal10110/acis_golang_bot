package nav

import (
	"cmp"
	"math"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// walkable reports whether c can be stood on and clicked to: it has geodata,
// its floor is out of water, its layer allows at least one direction, and it
// is a click target.
func (p *Planner) walkable(s *scratch, c Cell) bool {
	return p.hasGeo(c.X, c.Y) && !p.Water.wet(c.X, c.Y, c.Z) && p.nswe(c) != block.NoDirections && p.clickable(c, &s.layers)
}

// NearestWalkable snaps a target point (a spot centre, a follow slot, an NPC
// spawn) to the walkable cell nearest it within r world units: the layer
// nearest the point's Z in each cell, ranked by horizontal distance and then
// by height difference.
func (p *Planner) NearestWalkable(at location.Location, r float64) (Cell, bool) {
	return p.NearestWalkableFunc(at, r, nil)
}

// NearestWalkableFunc is NearestWalkable restricted to the cells accept
// takes (nil takes every one), such as the cells inside a spot's territory
// in the walker's strongly connected component. accept sees only walkable
// cells, nearest first, until it takes one.
func (p *Planner) NearestWalkableFunc(at location.Location, r float64, accept func(Cell) bool) (Cell, bool) {
	s := getScratch()
	defer putScratch(s)
	type ranked struct {
		c  Cell
		d  float64
		dz int
	}
	var cs []ranked
	p.cellsWithin(at, r, func(c Cell, d float64) {
		cs = append(cs, ranked{c: c, d: d, dz: abs(int(c.Z) - at.Z)})
	})
	slices.SortStableFunc(cs, func(a, b ranked) int {
		if c := cmp.Compare(a.d, b.d); c != 0 {
			return c
		}
		return cmp.Compare(a.dz, b.dz)
	})
	for _, c := range cs {
		if p.walkable(s, c.c) && (accept == nil || accept(c.c)) {
			return c.c, true
		}
	}
	return Cell{}, false
}

// cellsWithin calls fn with every cell whose centre lies within r of at
// horizontally, on the layer nearest at's Z, and that centre's distance.
func (p *Planner) cellsWithin(at location.Location, r float64, fn func(Cell, float64)) {
	if engine.OutOfWorld(at.X, at.Y) {
		return
	}
	gx, gy := engine.GeoX(at.X), engine.GeoY(at.Y)
	rc := int(math.Ceil(r/block.CellSize)) + 1
	for x := gx - rc; x <= gx+rc; x++ {
		for y := gy - rc; y <= gy+rc; y++ {
			d := hypot(engine.WorldX(x)-at.X, engine.WorldY(y)-at.Y)
			if d > r || !p.hasGeo(x, y) {
				continue
			}
			fn(Cell{X: x, Y: y, Z: p.Engine.HeightNearest(x, y, at.Z)}, d)
		}
	}
}

// StandMode is what a stand point must offer toward its target.
type StandMode int

const (
	// StandLine: a straight walkable line to the target, the line the
	// tracking walk will take (melee, cast, talk).
	StandLine StandMode = iota
	// StandPickUp: CanMove onto the item's raw Z (pick-up).
	StandPickUp
	// StandSight: line of sight with both collision heights (bows, spells).
	StandSight
)

// Target is what a stand point serves.
type Target struct {
	At    location.Location
	Range float64 // world units, measured in 3D from the stand point's floor
	Mode  StandMode
	// CollisionHeight is the target's own, for StandSight.
	CollisionHeight float64
}

// standCandidate is a cell considered for a stand point, with its horizontal
// distance from the walker.
type standCandidate struct {
	cell Cell
	d    float64
}

// StandPoint returns the walkable cell nearest from (horizontally) within
// t.Range of t.At that offers t.Mode toward it, as a world point on its
// floor. Restricting it to the walker's strongly connected component needs
// the hierarchy (N3, #155).
func (p *Planner) StandPoint(from location.Location, t Target) (location.Location, bool) {
	s := getScratch()
	defer putScratch(s)
	s.stands = s.stands[:0]
	p.cellsWithin(t.At, t.Range, func(c Cell, _ float64) {
		w := c.World()
		dz := float64(w.Z - t.At.Z)
		if math.Hypot(hypot(w.X-t.At.X, w.Y-t.At.Y), dz) > t.Range {
			return
		}
		s.stands = append(s.stands, standCandidate{c, hypot(w.X-from.X, w.Y-from.Y)})
	})
	slices.SortFunc(s.stands, func(a, b standCandidate) int {
		switch {
		case a.d < b.d:
			return -1
		case a.d > b.d:
			return 1
		}
		return 0
	})
	for _, c := range s.stands {
		if !p.walkable(s, c.cell) {
			continue
		}
		w := c.cell.World()
		if p.offers(w, t) {
			return w, true
		}
	}
	return location.Location{}, false
}

// offers reports whether a stand point at w offers t.Mode toward t.At.
func (p *Planner) offers(w location.Location, t Target) bool {
	switch t.Mode {
	case StandLine:
		z := int(p.Engine.Height(t.At.X, t.At.Y, t.At.Z))
		return p.Engine.CanMove(w.X, w.Y, w.Z, t.At.X, t.At.Y, z)
	case StandPickUp:
		return p.Engine.CanMove(w.X, w.Y, w.Z, t.At.X, t.At.Y, t.At.Z)
	case StandSight:
		return p.Engine.CanSeeActor(w.X, w.Y, w.Z, p.CollisionHeight, t.At.X, t.At.Y, t.At.Z, t.CollisionHeight)
	}
	return false
}
