// Package geotest builds synthetic geodata for tests: one region holding the
// fixture scenes (a corridor with a door, a U-wall, a one-way ledge, a pit, a
// multilayer bridge and an enclosed pocket) on flat ground, encoded in the
// L2OFF region file format so it goes through the same reader, stored form
// and cache as real geodata. No real geodata is committed or needed.
package geotest

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// The region tile the scenes live in, and the heights they use. Every
// height is a multiple of block.CellHeight, so complex and multilayer blocks
// store it exactly.
const (
	RegionX = 20
	RegionY = 18

	Ground  = 0
	Wall    = 400
	Plateau = 200
	PitDown = -200
	Deck    = 160

	// Climb is the most a step may rise between neighbouring cells and stay
	// passable; anything lower, a drop included, is passable.
	Climb = block.CellIgnoreHeight
)

const regionCells = block.RegionBlocksX * block.CellsX

// rect is an inclusive cell rectangle in region-local cell coordinates.
type rect struct{ x0, y0, x1, y1 int }

func (r rect) has(x, y int) bool { return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1 }

// touches reports whether r, grown by one cell, overlaps the other rect.
func (r rect) touches(o rect) bool {
	return r.x0-1 <= o.x1 && o.x0 <= r.x1+1 && r.y0-1 <= o.y1 && o.y0 <= r.y1+1
}

type patch struct {
	area   rect
	height int16
}

// Terrain is a region described by its raw cell heights: ground patches
// (one layer, later patches winning) and decks (an upper layer over the
// ground). NSWE is derived from the heights by the Climb rule, per layer: a
// direction is open when the neighbour has a layer no higher than Climb above.
type Terrain struct {
	patches []patch
	decks   []patch
}

// Point is one named fixture spot in region-local cell coordinates.
type Point struct {
	CellX, CellY int
	Z            int
}

// World returns the world coordinates of the centre of p's cell.
func (p Point) World() (x, y, z int) {
	return CellWorldX(p.CellX), CellWorldY(p.CellY), p.Z
}

// CellWorldX returns the world X of the centre of region-local cell column x.
func CellWorldX(x int) int {
	return engine.WorldX((RegionX-engine.TileXMin)*regionCells + x)
}

// CellWorldY returns the world Y of the centre of region-local cell row y.
func CellWorldY(y int) int {
	return engine.WorldY((RegionY-engine.TileYMin)*regionCells + y)
}

func (t *Terrain) ground(x, y int) int16 {
	h := int16(Ground)
	for _, p := range t.patches {
		if p.area.has(x, y) {
			h = p.height
		}
	}
	return h
}

// heights returns the raw layer heights of cell (x, y), lowest first.
func (t *Terrain) heights(x, y int) []int16 {
	hs := []int16{t.ground(x, y)}
	for _, d := range t.decks {
		if d.area.has(x, y) {
			hs = append(hs, d.height)
		}
	}
	return hs
}

// Layers returns the cell layers of region-local cell (x, y), lowest first,
// with their derived NSWE.
func (t *Terrain) Layers(x, y int) []block.Cell {
	hs := t.heights(x, y)
	cells := make([]block.Cell, len(hs))
	for i, h := range hs {
		cells[i] = block.Cell{Height: h, NSWE: t.nswe(x, y, h)}
	}
	return cells
}

func (t *Terrain) nswe(x, y int, h int16) block.NSWE {
	var n block.NSWE
	for _, d := range []struct {
		dx, dy int
		dir    block.NSWE
	}{{1, 0, block.East}, {-1, 0, block.West}, {0, 1, block.South}, {0, -1, block.North}} {
		nx, ny := x+d.dx, y+d.dy
		if nx < 0 || ny < 0 || nx >= regionCells || ny >= regionCells {
			n |= d.dir
			continue
		}
		for _, nh := range t.heights(nx, ny) {
			if int(nh)-int(h) <= Climb {
				n |= d.dir
				break
			}
		}
	}
	return n
}

// quiet reports whether the block at (bx, by) is plain flat ground: no
// patch or deck in it or next to it.
func (t *Terrain) quiet(bx, by int) bool {
	b := rect{bx * block.CellsX, by * block.CellsY, bx*block.CellsX + block.CellsX - 1, by*block.CellsY + block.CellsY - 1}
	for _, p := range t.patches {
		if b.touches(p.area) {
			return false
		}
	}
	for _, d := range t.decks {
		if b.touches(d.area) {
			return false
		}
	}
	return true
}

// EncodeCell is the on-disk code of c, the inverse of block.DecodeCell for
// heights that are a multiple of block.CellHeight.
func EncodeCell(c block.Cell) uint16 {
	return uint16(c.Height<<1)&0xFFF0 | uint16(c.NSWE)
}

// L2OFF encodes the terrain as one L2OFF region file (`X_Y_conv.dat`): an
// 18-byte header, then per block a type word and a flat height, 64 complex
// cells, or per-cell layer counts with the layers highest first.
func (t *Terrain) L2OFF() []byte {
	buf := make([]byte, 18, 1<<20)
	u16 := func(v uint16) { buf = binary.LittleEndian.AppendUint16(buf, v) }
	for i := range block.RegionBlockCount {
		bx, by := i/block.RegionBlocksY, i%block.RegionBlocksY
		if t.quiet(bx, by) {
			u16(0x0000)
			u16(uint16(int16(Ground)))
			u16(0)
			continue
		}
		var cells [block.CellCount][]block.Cell
		multi := false
		for c := range block.CellCount {
			cx, cy := c/block.CellsY, c%block.CellsY
			cells[c] = t.Layers(bx*block.CellsX+cx, by*block.CellsY+cy)
			multi = multi || len(cells[c]) > 1
		}
		if !multi {
			u16(0x0040)
			for _, layers := range cells {
				u16(EncodeCell(layers[0]))
			}
			continue
		}
		u16(0x0001)
		for _, layers := range cells {
			u16(uint16(len(layers)))
			for l := len(layers) - 1; l >= 0; l-- {
				u16(EncodeCell(layers[l]))
			}
		}
	}
	return buf
}

// WriteL2OFF writes the terrain as region (RegionX, RegionY) into dir, under
// the L2OFF file name, and returns its path.
func (t *Terrain) WriteL2OFF(dir string) (string, error) {
	path := filepath.Join(dir, fmt.Sprintf("%d_%d_conv.dat", RegionX, RegionY))
	if err := os.WriteFile(path, t.L2OFF(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
