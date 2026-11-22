package nav

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/fatal10110/acis_golang_bot/internal/geo"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// ErrWater: an endpoint's cell lies in a water zone. The planner keeps
// routes out of water (docs/navigation.md, "Water"), so it plans neither
// into it nor out of it.
var ErrWater = errors.New("nav: point is in a water zone")

// WaterBox is one water zone's volume in world units, bounds inclusive: a
// position is in it when X lies in X1..X2, Y in Y1..Y2 and Z in MinZ..MaxZ,
// as the server's cuboid zone form contains it (acis_golang
// model/geometry Territory.Contains, Rectangle.Contains).
type WaterBox struct {
	X1, Y1, X2, Y2 int
	MinZ, MaxZ     int
}

// Water is the water zones a planner keeps its routes out of. While a
// player's position is inside a water zone the server moves it as a swimmer
// (acis_golang model/actor/move/creature.go waterLocked, resolvePathLocked),
// with checks the ground planner does not model, so no cell-graph edge
// enters or leaves a cell whose floor lies in water and no segment passes a
// point in water. It is immutable after NewWater, so any number of planners
// may share one. A nil *Water is no water.
type Water struct {
	boxes []WaterBox
	// tiles lists, per geodata region tile, the boxes overlapping it.
	tiles [tilesX * tilesY][]int32
	hash  [8]byte
}

const (
	tilesX = geo.TileXMax - geo.TileXMin + 1
	tilesY = geo.TileYMax - geo.TileYMin + 1
)

// NewWater indexes boxes by region tile. A box with its corners or Z band
// swapped is normalized; a box outside the world is kept for Contains and
// never matches a cell.
func NewWater(boxes []WaterBox) *Water {
	w := &Water{boxes: make([]WaterBox, len(boxes))}
	sum := sha256.New()
	var buf [6 * 8]byte
	for i, b := range boxes {
		b.X1, b.X2 = min(b.X1, b.X2), max(b.X1, b.X2)
		b.Y1, b.Y2 = min(b.Y1, b.Y2), max(b.Y1, b.Y2)
		b.MinZ, b.MaxZ = min(b.MinZ, b.MaxZ), max(b.MinZ, b.MaxZ)
		w.boxes[i] = b
		for j, v := range [6]int{b.X1, b.Y1, b.X2, b.Y2, b.MinZ, b.MaxZ} {
			binary.LittleEndian.PutUint64(buf[8*j:], uint64(int64(v)))
		}
		sum.Write(buf[:])
		tx0, tx1 := tileIndex(b.X1, geo.WorldXMin, tilesX), tileIndex(b.X2, geo.WorldXMin, tilesX)
		ty0, ty1 := tileIndex(b.Y1, geo.WorldYMin, tilesY), tileIndex(b.Y2, geo.WorldYMin, tilesY)
		if b.X2 < geo.WorldXMin || b.X1 > geo.WorldXMax || b.Y2 < geo.WorldYMin || b.Y1 > geo.WorldYMax {
			continue
		}
		for tx := tx0; tx <= tx1; tx++ {
			for ty := ty0; ty <= ty1; ty++ {
				w.tiles[tx*tilesY+ty] = append(w.tiles[tx*tilesY+ty], int32(i))
			}
		}
	}
	if len(boxes) > 0 {
		w.hash = [8]byte(sum.Sum(nil)[:8])
	}
	return w
}

// tileIndex is the region tile holding world coordinate v, clamped to the
// world.
func tileIndex(v, worldMin, tiles int) int {
	return min(max((v-worldMin)/geo.TileSize, 0), tiles-1)
}

// Hash identifies the boxes, in order: the water half of a nav cache key
// (NavKey.WaterHash). No water hashes to zero.
func (w *Water) Hash() [8]byte {
	if w == nil {
		return [8]byte{}
	}
	return w.hash
}

// Contains reports whether the world point lies in a water zone, as the
// server tests a position.
func (w *Water) Contains(x, y, z int) bool {
	if w == nil {
		return false
	}
	for _, b := range w.boxes {
		if x >= b.X1 && x <= b.X2 && y >= b.Y1 && y <= b.Y2 && z >= b.MinZ && z <= b.MaxZ {
			return true
		}
	}
	return false
}

// wet reports whether any point of the geodata cell (gx, gy) at floor height
// z lies in a water zone: a walker on that floor can stand anywhere in the
// cell's square.
func (w *Water) wet(gx, gy int, z int16) bool {
	if w == nil || gx < 0 || gy < 0 || gx >= tilesX*geo.TileSize/block.CellSize || gy >= tilesY*geo.TileSize/block.CellSize {
		return false
	}
	x0 := engine.WorldX(gx) - block.CellSize/2
	y0 := engine.WorldY(gy) - block.CellSize/2
	x1, y1 := x0+block.CellSize-1, y0+block.CellSize-1
	t := (gx*block.CellSize/geo.TileSize)*tilesY + gy*block.CellSize/geo.TileSize
	for _, i := range w.tiles[t] {
		b := &w.boxes[i]
		if x0 <= b.X2 && x1 >= b.X1 && y0 <= b.Y2 && y1 >= b.Y1 && int(z) >= b.MinZ && int(z) <= b.MaxZ {
			return true
		}
	}
	return false
}
