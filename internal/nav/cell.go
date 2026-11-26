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
	// Swim is the longest stretch of water a route may swim, in world
	// units measured in 3D: the walker's swim speed times the breath it
	// allows one stretch. Zero keeps routes out of water. With a Swim the
	// cell search, the string pull and the step-chain check take swim moves
	// by the server's swim rules (docs/navigation.md, "Swimming");
	// Hierarchy.Route never swims.
	Swim float64
	// MaxSegment caps a segment's length in world units, below navSegment;
	// zero is navSegment. A walker toward a destination that moves keeps its
	// segments short: a segment in flight cannot be cut short (docs/walking.md,
	// "Follower and moving destinations"). Every route honors it: Route, the
	// swimming cell search, Hierarchy.Route and RouteOneWay.
	MaxSegment float64
	// Avoid is the columns the current route keeps out of; the zero value
	// is none. The cell search, the string pull and the refinement of
	// Hierarchy.Route honor it; its owner sets it per route.
	Avoid Avoid

	// noDrops refuses every drop (dropCost), for the hierarchy's endpoint
	// legs; set on a copy only.
	noDrops bool
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

// WetAt reports whether the world point snaps to a wet cell: the cell and
// layer Locate gives it has its floor in one of the planner's water zones, as
// a walker standing there would be. A point Locate rejects is not wet.
func (p *Planner) WetAt(at location.Location) bool {
	c, ok := p.Locate(at)
	return ok && p.Water.wet(c.X, c.Y, c.Z)
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
// Unless the planner swims, a crossing neither leaves nor enters a cell whose
// floor is in water. A swimmer's leg must pass the same CanMove (the swim
// branch of resolvePathLocked), so the floor graph is its graph too.
func (p *Planner) cross(c Cell, n block.NSWE, d direction) (Cell, block.NSWE, bool) {
	if !p.swims() && p.Water.wet(c.X, c.Y, c.Z) {
		return Cell{}, 0, false
	}
	return p.crossDry(c, n, d)
}

// crossDry is cross from a cell whose floor is known to be out of water, or
// a swimming planner's, so a caller crossing from one cell several times
// tests it once.
func (p *Planner) crossDry(c Cell, n block.NSWE, d direction) (Cell, block.NSWE, bool) {
	if !n.Allows(d.flag) {
		return Cell{}, 0, false
	}
	nx, ny := c.X+d.dx, c.Y+d.dy
	if !p.hasGeo(nx, ny) {
		return Cell{}, 0, false
	}
	h, nn, ok := p.Engine.NodeBelow(nx, ny, int(c.Z)+block.CellIgnoreHeight)
	if !ok || (!p.swims() && p.Water.wet(nx, ny, h)) || (p.noDrops && dropCost(c.Z, h) > 0) {
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

// A drop is a step that lands more than dropBound below the cell it leaves:
// a fall off a wall or a cliff, past twice the climb limit, where a steep
// slope's steps stay within it. CanMove takes it (the server limits a
// descent by fall damage only), but a bot walking it looks like one jumping
// off a wall (#504). Each drop costs costDrop more per whole dropBound it
// falls (dropCost), in the cell search and in the nav build's edge costs, so
// a route goes down a ramp or through a gate whenever that way is less than
// costDrop/costCardinal cells longer per dropBound of the fall, and takes a
// drop only where no such way exists, as a one-way drop toward
// a leader below does (docs/navigation.md, "Drops"). A larger cost makes a
// leg that must drop flood more cells before it does.
const (
	dropBound = 2 * block.CellIgnoreHeight
	costDrop  = 1000
)

// dropCost returns costDrop for each whole dropBound a step from a floor at
// z down to one at to falls, when it is a drop, else 0: a fall's cost grows
// with its height, as its fall damage does (the server's FallDamageBase,
// height times max HP over 1000, past the safe fall height), so a high
// cliff weighs a longer way round than a ledge (#514).
func dropCost(z, to int16) int32 {
	if fall := int(z) - int(to); fall > dropBound {
		return costDrop * int32(fall/dropBound)
	}
	return 0
}

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
//
// A swimming planner's edge into a wet cell costs swimCost times as much.
func (p *Planner) neighbours(dst []neighbour, c Cell) []neighbour {
	swims := p.swims()
	if !swims && p.Water.wet(c.X, c.Y, c.Z) {
		return dst // no crossing leaves a cell in water
	}
	n := p.nswe(c)
	var next [4]Cell
	var nextNSWE [4]block.NSWE
	var open [4]bool
	edge := n != block.AllDirections
	for i, d := range cardinals {
		next[i], nextNSWE[i], open[i] = p.crossDry(c, n, d)
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
	cost := func(c Cell, base int32) int32 {
		if swims && p.Water.wet(c.X, c.Y, c.Z) {
			return base * swimCost
		}
		return base
	}
	for i := range cardinals {
		if open[i] {
			dst = append(dst, neighbour{next[i], cost(next[i], costCardinal+extra) + dropCost(c.Z, next[i].Z)})
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
			if !okX || !okY || viaX != viaY || (p.noDrops && dropCost(c.Z, viaX.Z) > 0) {
				continue
			}
			from, to := c.World(), viaX.World()
			if !p.Engine.CanMove(from.X, from.Y, from.Z, to.X, to.Y, to.Z) {
				continue
			}
			dst = append(dst, neighbour{viaX, cost(viaX, costDiagonal+extra) + dropCost(c.Z, viaX.Z)})
		}
	}
	return dst
}

// crossFrom is cross for the second leg of an L-shaped detour, from a cell
// the first leg landed on (so out of water, or a swimming planner's).
func (p *Planner) crossFrom(c Cell, n block.NSWE, d direction) (Cell, bool) {
	next, _, ok := p.crossDry(c, n, d)
	return next, ok
}
