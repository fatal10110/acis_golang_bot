package nav

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"slices"

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
	// The boxes overlapping each bucket of waterBucket × waterBucket world
	// units, as CSR: bucket k's boxes are list[start[k]:start[k+1]]. A
	// bucket is a sixteenth of a region tile on each axis, so a cell test
	// scans a handful of boxes, not every box of a watery region (#209).
	start []int32
	list  []int32
	hash  [8]byte
}

const (
	tilesX = geo.TileXMax - geo.TileXMin + 1
	tilesY = geo.TileYMax - geo.TileYMin + 1
	// waterBucket is a multiple of block.CellSize, so a cell lies in one
	// bucket.
	waterBucket = geo.TileSize / 16
	bucketsX    = tilesX * geo.TileSize / waterBucket
	bucketsY    = tilesY * geo.TileSize / waterBucket
)

// NewWater indexes boxes by world bucket. A box with its corners or Z band
// swapped is normalized; a box outside the world matches no point.
func NewWater(boxes []WaterBox) *Water {
	w := &Water{boxes: make([]WaterBox, len(boxes)), start: make([]int32, bucketsX*bucketsY+1)}
	sum := sha256.New()
	var buf [6 * 8]byte
	// buckets calls fn with every bucket box i overlaps; a box outside the
	// world overlaps none.
	buckets := func(i int, fn func(k int)) {
		b := w.boxes[i]
		if b.X2 < geo.WorldXMin || b.X1 > geo.WorldXMax || b.Y2 < geo.WorldYMin || b.Y1 > geo.WorldYMax {
			return
		}
		bx0, bx1 := bucketIndex(b.X1, geo.WorldXMin, bucketsX), bucketIndex(b.X2, geo.WorldXMin, bucketsX)
		by0, by1 := bucketIndex(b.Y1, geo.WorldYMin, bucketsY), bucketIndex(b.Y2, geo.WorldYMin, bucketsY)
		for bx := bx0; bx <= bx1; bx++ {
			for by := by0; by <= by1; by++ {
				fn(bx*bucketsY + by)
			}
		}
	}
	for i, b := range boxes {
		b.X1, b.X2 = min(b.X1, b.X2), max(b.X1, b.X2)
		b.Y1, b.Y2 = min(b.Y1, b.Y2), max(b.Y1, b.Y2)
		b.MinZ, b.MaxZ = min(b.MinZ, b.MaxZ), max(b.MinZ, b.MaxZ)
		w.boxes[i] = b
		for j, v := range [6]int{b.X1, b.Y1, b.X2, b.Y2, b.MinZ, b.MaxZ} {
			binary.LittleEndian.PutUint64(buf[8*j:], uint64(int64(v)))
		}
		sum.Write(buf[:])
		buckets(i, func(k int) { w.start[k+1]++ })
	}
	for k := range bucketsX * bucketsY {
		w.start[k+1] += w.start[k]
	}
	w.list = make([]int32, w.start[bucketsX*bucketsY])
	pos := slices.Clone(w.start[:bucketsX*bucketsY])
	for i := range w.boxes {
		buckets(i, func(k int) {
			w.list[pos[k]] = int32(i)
			pos[k]++
		})
	}
	if len(boxes) > 0 {
		w.hash = [8]byte(sum.Sum(nil)[:8])
	}
	return w
}

// bucketIndex is the bucket holding world coordinate v on one axis, clamped
// to the world.
func bucketIndex(v, worldMin, buckets int) int {
	return min(max((v-worldMin)/waterBucket, 0), buckets-1)
}

// bucket returns the boxes overlapping bucket (bx, by).
func (w *Water) bucket(bx, by int) []int32 {
	k := bx*bucketsY + by
	return w.list[w.start[k]:w.start[k+1]]
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
// server tests a position. A point outside the geodata world is never in
// water: no walker stands there.
func (w *Water) Contains(x, y, z int) bool {
	if w == nil || x < geo.WorldXMin || x > geo.WorldXMax || y < geo.WorldYMin || y > geo.WorldYMax {
		return false
	}
	for _, i := range w.bucket(bucketIndex(x, geo.WorldXMin, bucketsX), bucketIndex(y, geo.WorldYMin, bucketsY)) {
		b := &w.boxes[i]
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
	for _, i := range w.bucket(gx*block.CellSize/waterBucket, gy*block.CellSize/waterBucket) {
		b := &w.boxes[i]
		if x0 <= b.X2 && x1 >= b.X1 && y0 <= b.Y2 && y1 >= b.Y1 && int(z) >= b.MinZ && int(z) <= b.MaxZ {
			return true
		}
	}
	return false
}
