package nav

import (
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Cell is one node of the cell graph: a geodata cell and the layer whose
// floor is at Z. The floor height names the layer, so a Cell stays the same
// node whatever the layer's index in the block.
type Cell struct {
	X, Y int   // geodata coordinates
	Z    int16 // floor height of the layer
}

// World returns the cell's centre on its floor.
func (c Cell) World() location.Location {
	return location.Location{X: engine.WorldX(c.X), Y: engine.WorldY(c.Y), Z: int(c.Z)}
}

// Planner plans for one walker. It is a plain value: a walker builds its own
// with its speed and collision height, and the engine it reads is shared and
// safe for concurrent reads. A Planner is not safe for concurrent use only in
// the sense that its fields must not change while a query runs.
type Planner struct {
	// Engine is the geodata the planner reads.
	Engine *engine.Engine
	// Lmax is the longest step after the first, in world units: run speed ×
	// Emax, the longest gap between the server's position updates.
	// docs/navigation.md ("Settled by N0", Emax) measured 141 ms at most on
	// the live server; the walker (N5) supplies it with a margin.
	Lmax float64
	// CollisionHeight is the walker's collision height: a layer with another
	// layer less than twice that above it cannot be clicked to, because the
	// server lands the click on the upper one (docs/navigation.md, "Head-level
	// Z on multilayer").
	CollisionHeight float64
	// Budget caps the cells a route search expands, across its replans.
	Budget int
	// Water is the water zones routes keep out of; nil is none. A planner
	// for a hierarchy reads the Water its nav cache was built with.
	Water *Water
}

// Locate snaps a world point to its cell and the layer nearest its Z, as the
// server's Height does. It reports false outside the world and on cells
// without geodata, which the planner treats as no-go.
func (p *Planner) Locate(at location.Location) (Cell, bool) {
	if engine.OutOfWorld(at.X, at.Y) || !p.Engine.HasGeo(at.X, at.Y) {
		return Cell{}, false
	}
	gx, gy := engine.GeoX(at.X), engine.GeoY(at.Y)
	return Cell{X: gx, Y: gy, Z: p.Engine.HeightNearest(gx, gy, at.Z)}, true
}

// hasGeo reports whether the geodata cell belongs to a loaded non-null block.
// The null block answers open ground at height 0 to every query, so without
// this check a route would wander into unlisted regions.
func (p *Planner) hasGeo(gx, gy int) bool {
	return p.Engine.HasGeo(engine.WorldX(gx), engine.WorldY(gy))
}

// clickable reports whether a click with c's floor Z lands on c's layer: no
// other layer lies above it closer than twice the collision height.
func (p *Planner) clickable(c Cell, buf *[block.MaxLayers]block.Cell) bool {
	for _, l := range p.Engine.AppendLayers(buf[:0], c.X, c.Y) {
		if l.Height > c.Z && float64(l.Height-c.Z) < 2*p.CollisionHeight {
			return false
		}
	}
	return true
}

// direction is one cardinal step on the geodata grid.
type direction struct {
	dx, dy int
	flag   block.NSWE
}

// cardinals are the four crossings CanMove checks, with the NSWE bit the cell
// being left must allow (engine moveDirectionFor: +X is East, +Y is South).
var cardinals = [4]direction{
	{1, 0, block.East},
	{-1, 0, block.West},
	{0, 1, block.South},
	{0, -1, block.North},
}

// cross is the directed border crossing from c (whose layer has mask n) into
// the neighbour d, exactly as CanMove takes it: n must allow d, and the walker
// lands on the neighbour's layer nearest at or below c's floor plus
// block.CellIgnoreHeight. So a step goes down any height and up at most 48.
// A crossing neither leaves nor enters a cell whose floor is in water.
func (p *Planner) cross(c Cell, n block.NSWE, d direction) (Cell, block.NSWE, bool) {
	if !n.Allows(d.flag) || p.Water.wet(c.X, c.Y, c.Z) {
		return Cell{}, 0, false
	}
	nx, ny := c.X+d.dx, c.Y+d.dy
	if !p.hasGeo(nx, ny) {
		return Cell{}, 0, false
	}
	h, nn, ok := p.Engine.NodeBelow(nx, ny, int(c.Z)+block.CellIgnoreHeight)
	if !ok || p.Water.wet(nx, ny, h) {
		return Cell{}, 0, false
	}
	return Cell{X: nx, Y: ny, Z: h}, nn, true
}

// nswe returns the mask of c's own layer.
func (p *Planner) nswe(c Cell) block.NSWE {
	return p.Engine.NSWENearest(c.X, c.Y, int(c.Z))
}

// Step costs, in tenths of a cell: a cardinal step, a diagonal step, and the
// extra for leaving a cell next to a blocked edge or a drop. The clearance
// extra keeps routes off walls and ledges, which leaves a margin for the step
// chain and for dead reckoning (docs/navigation.md, "Wall clearance cost").
const (
	costCardinal  = 10
	costDiagonal  = 14
	costClearance = 10
)

// neighbour is one directed edge out of a cell.
type neighbour struct {
	cell Cell
	cost int32
}

// neighbours appends every directed edge out of c to dst. Cardinal edges are
// the crossings CanMove allows. A diagonal edge needs both L-shaped detours
// open and landing on the same layer, and CanMove along the diagonal itself,
// as the server's pathfinder does for its corner steps
// (geo/pathfind/pathfind.go, cornerSteps and addCandidate).
func (p *Planner) neighbours(dst []neighbour, c Cell) []neighbour {
	n := p.nswe(c)
	var next [4]Cell
	var nextNSWE [4]block.NSWE
	var open [4]bool
	edge := n != block.AllDirections
	for i, d := range cardinals {
		next[i], nextNSWE[i], open[i] = p.cross(c, n, d)
		if !open[i] {
			edge = true // a wall, a climb over 48 or no-go ground
			continue
		}
		if next[i].Z < c.Z-block.CellIgnoreHeight {
			edge = true // a drop
		}
	}
	extra := int32(0)
	if edge {
		extra = costClearance
	}
	for i := range cardinals {
		if open[i] {
			dst = append(dst, neighbour{next[i], costCardinal + extra})
		}
	}
	// Diagonals: X index (East 0, West 1) then Y index (South 2, North 3).
	for _, xi := range [2]int{0, 1} {
		for _, yi := range [2]int{2, 3} {
			if !open[xi] || !open[yi] {
				continue
			}
			viaX, okX := p.crossFrom(next[xi], nextNSWE[xi], cardinals[yi])
			viaY, okY := p.crossFrom(next[yi], nextNSWE[yi], cardinals[xi])
			if !okX || !okY || viaX != viaY {
				continue
			}
			from, to := c.World(), viaX.World()
			if !p.Engine.CanMove(from.X, from.Y, from.Z, to.X, to.Y, to.Z) {
				continue
			}
			dst = append(dst, neighbour{viaX, costDiagonal + extra})
		}
	}
	return dst
}

// crossFrom is cross for the second leg of an L-shaped detour.
func (p *Planner) crossFrom(c Cell, n block.NSWE, d direction) (Cell, bool) {
	next, _, ok := p.cross(c, n, d)
	return next, ok
}
