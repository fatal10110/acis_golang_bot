package geoquery

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/geo"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// Kind names a kind of site Curate looks for.
type Kind string

const (
	// Wall: a cell layer with a closed NSWE direction. Short canmove both
	// ways across the closed side, along the wall, and los across it.
	Wall Kind = "wall"
	// Ledge: a cell layer whose neighbour's nearest layer is at least
	// ledgeRise higher or lower. Canmove both ways across the edge, the
	// height on both sides, and validlocation both ways.
	Ledge Kind = "ledge"
	// Multilayer: a cell with two or more layers. The height at each layer
	// and above it (head level), at and beside the midpoint of each pair of
	// layers (where the nearest layer changes), and above the top and below
	// the bottom; canmove and validlocation from each layer.
	Multilayer Kind = "multilayer"
	// Random: a random short pair. Canmove, los and validlocation.
	Random Kind = "random"
)

// Kinds lists every Kind in the order Curate fills them.
var Kinds = [...]Kind{Wall, Ledge, Multilayer, Random}

const (
	// regionCells is the side of a geodata region in cells.
	regionCells = geo.TileSize >> 4
	// cellSize is the side of a geodata cell in world units.
	cellSize = 16
	// ledgeRise is the smallest height step between neighbouring cells a
	// ledge site has: below the 48 a walk may climb (CellIgnoreHeight), so
	// the climb limit falls inside the sampled band.
	ledgeRise = 32
	// maxLayers bounds the layers read from one cell.
	maxLayers = 32
	// triesPerKind bounds the cells sampled for one kind, so a region set
	// without a kind (no multilayer cell, say) still ends.
	triesPerKind = 2_000_000
)

// headOffsets are the heights above a layer the multilayer heights ask at:
// the layer itself and head levels around the player collision heights the
// server adds to a click's Z before it reads the target's height.
var headOffsets = [...]int{0, 9, 16, 24, 32, 48}

type dir struct {
	bit    block.NSWE
	dx, dy int
}

// dirs are the cardinal directions with their cell steps; south is +Y.
var dirs = [...]dir{{block.East, 1, 0}, {block.West, -1, 0}, {block.South, 0, 1}, {block.North, 0, -1}}

// Set is a curated query set: the queries, sorted by id and each once, and
// how many sites of each kind they came from.
type Set struct {
	Queries []Query
	Sites   map[Kind]int
}

// Curate picks up to n sites of each kind in the listed regions of e, from
// a random source seeded by seed, and returns their queries. The same
// engine, regions, seed and n give the same set.
func Curate(e *engine.Engine, regions []geodata.Region, seed uint64, n int) Set {
	c := &curator{
		e:       e,
		regions: regions,
		r:       rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		seen:    make(map[string]bool),
		listed:  make(map[geodata.Region]bool, len(regions)),
	}
	for _, r := range regions {
		c.listed[r] = true
	}
	sites := make(map[Kind]int, len(Kinds))
	if len(regions) == 0 || n <= 0 {
		return Set{Sites: sites}
	}
	site := map[Kind]func() bool{Wall: c.wall, Ledge: c.ledge, Multilayer: c.multilayer, Random: c.random}
	for _, k := range Kinds {
		for try := 0; try < triesPerKind && sites[k] < n; try++ {
			if before := len(c.out); site[k]() && len(c.out) > before {
				sites[k]++
			}
		}
	}
	slices.SortFunc(c.out, func(a, b Query) int { return strings.Compare(a.ID(), b.ID()) })
	return Set{Queries: c.out, Sites: sites}
}

type curator struct {
	e       *engine.Engine
	regions []geodata.Region
	r       *rand.Rand
	seen    map[string]bool
	listed  map[geodata.Region]bool
	out     []Query
}

// inListed reports whether the box spanned by a and b lies in listed
// regions. geoprobe loads every region file in the directory, listed or
// not, where the bot's engine answers an unlisted region as open ground, so
// a query reaching one would disagree for a reason that is not the engine.
func (c *curator) inListed(a, b location.Location) bool {
	for _, p := range [...][2]int{{a.X, a.Y}, {b.X, b.Y}, {a.X, b.Y}, {b.X, a.Y}} {
		rx, ry := engine.RegionOf(p[0], p[1])
		if !c.listed[geodata.Region{X: rx, Y: ry}] {
			return false
		}
	}
	return true
}

func (c *curator) add(cat Category, from, to location.Location) {
	q := Query{Category: cat, From: from, To: to}
	if cat == Height {
		q.To = location.Location{}
		to = from
	}
	if !c.inListed(from, to) {
		return
	}
	if id := q.ID(); !c.seen[id] {
		c.seen[id] = true
		c.out = append(c.out, q)
	}
}

// cell picks a random geodata cell of a random listed region.
func (c *curator) cell() (gx, gy int) {
	reg := c.regions[c.r.IntN(len(c.regions))]
	return (reg.X-geo.TileXMin)*regionCells + c.r.IntN(regionCells),
		(reg.Y-geo.TileYMin)*regionCells + c.r.IntN(regionCells)
}

// layers returns the heights of the layers of cell gx, gy, highest first.
func (c *curator) layers(gx, gy int) []int16 {
	var hs []int16
	z := math.MaxInt16
	for len(hs) < maxLayers {
		h, _, ok := c.e.NodeBelow(gx, gy, z)
		if !ok || (len(hs) > 0 && h >= hs[len(hs)-1]) {
			break
		}
		hs = append(hs, h)
		z = int(h) - 1
	}
	return hs
}

// at is a world point inside cell gx, gy, off its centre, at the height of
// the cell's layer nearest z.
func (c *curator) at(gx, gy, z int) location.Location {
	return location.Location{
		X: engine.WorldX(gx) - cellSize/2 + c.r.IntN(cellSize),
		Y: engine.WorldY(gy) - cellSize/2 + c.r.IntN(cellSize),
		Z: int(c.e.HeightNearest(gx, gy, z)),
	}
}

// layered picks a random cell outside a flat block and one of its layers.
func (c *curator) layered() (gx, gy int, hs []int16, h int, ok bool) {
	gx, gy = c.cell()
	if name := c.e.BlockName(gx, gy); name == "BlockFlat" || name == "BlockNull" {
		return 0, 0, nil, 0, false
	}
	hs = c.layers(gx, gy)
	if len(hs) == 0 {
		return 0, 0, nil, 0, false
	}
	return gx, gy, hs, int(hs[c.r.IntN(len(hs))]), true
}

func (c *curator) wall() bool {
	gx, gy, _, h, ok := c.layered()
	if !ok {
		return false
	}
	nswe := c.e.NSWENearest(gx, gy, h)
	var closed []dir
	for _, d := range dirs {
		if !nswe.Allows(d.bit) {
			closed = append(closed, d)
		}
	}
	if len(closed) == 0 {
		return false
	}
	d := closed[c.r.IntN(len(closed))]
	back := c.r.IntN(3)
	ahead := 1 + c.r.IntN(4)
	from := c.at(gx-d.dx*back, gy-d.dy*back, h)
	to := c.at(gx+d.dx*ahead, gy+d.dy*ahead, h)
	c.add(CanMove, from, to)
	c.add(CanMove, to, from)
	c.add(LineOfSight, from, to)
	// Along the wall: the perpendicular, either way.
	along := 2 + c.r.IntN(5)
	if c.r.IntN(2) == 0 {
		along = -along
	}
	side := c.at(gx+d.dy*along, gy+d.dx*along, h)
	c.add(CanMove, c.at(gx, gy, h), side)
	return true
}

func (c *curator) ledge() bool {
	gx, gy, _, h, ok := c.layered()
	if !ok {
		return false
	}
	d := dirs[c.r.IntN(len(dirs))]
	nh := int(c.e.HeightNearest(gx+d.dx, gy+d.dy, h))
	if rise := nh - h; rise < ledgeRise && rise > -ledgeRise {
		return false
	}
	back := c.r.IntN(3)
	ahead := 1 + c.r.IntN(3)
	here := c.at(gx-d.dx*back, gy-d.dy*back, h)
	there := c.at(gx+d.dx*ahead, gy+d.dy*ahead, nh)
	c.add(CanMove, here, there)
	c.add(CanMove, there, here)
	c.add(ValidLocation, here, there)
	c.add(ValidLocation, there, here)
	for _, p := range [...]location.Location{here, there} {
		c.add(Height, p, location.Location{})
		c.add(Height, location.Location{X: p.X, Y: p.Y, Z: p.Z + 32}, location.Location{})
	}
	return true
}

func (c *curator) multilayer() bool {
	gx, gy := c.cell()
	if c.e.BlockName(gx, gy) != "BlockMultilayer" {
		return false
	}
	hs := c.layers(gx, gy)
	if len(hs) < 2 {
		return false
	}
	p := c.at(gx, gy, int(hs[0]))
	height := func(z int) { c.add(Height, location.Location{X: p.X, Y: p.Y, Z: z}, location.Location{}) }
	for i, h := range hs {
		for _, off := range headOffsets {
			height(int(h) + off)
		}
		if i+1 < len(hs) {
			mid := (int(h) + int(hs[i+1])) / 2
			height(mid - 1)
			height(mid)
			height(mid + 1)
		}
		d := dirs[c.r.IntN(len(dirs))]
		k := 1 + c.r.IntN(4)
		from := location.Location{X: p.X, Y: p.Y, Z: int(h)}
		to := c.at(gx+d.dx*k, gy+d.dy*k, int(h))
		c.add(CanMove, from, to)
		c.add(ValidLocation, from, to)
	}
	height(int(hs[0]) + 256)
	height(int(hs[len(hs)-1]) - 256)
	return true
}

func (c *curator) random() bool {
	gx, gy := c.cell()
	hs := c.layers(gx, gy)
	if len(hs) == 0 {
		return false
	}
	h := int(hs[c.r.IntN(len(hs))])
	from := c.at(gx, gy, h)
	to := c.at(gx+c.r.IntN(17)-8, gy+c.r.IntN(17)-8, h)
	c.add(CanMove, from, to)
	c.add(LineOfSight, from, to)
	c.add(ValidLocation, from, to)
	return true
}
