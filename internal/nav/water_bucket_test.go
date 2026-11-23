package nav

import (
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// TestWaterBucketsMatchScan checks the bucket index against a scan of every
// box: random boxes (small, bucket-spanning, swapped, past the world's
// edge) and random points and cells, many near box borders and bucket
// borders, answer Contains and wet as the scan does.
func TestWaterBucketsMatchScan(t *testing.T) {
	rng := rand.New(rand.NewPCG(21, 22))
	worldX := func() int { return geo.WorldXMin + rng.IntN(geo.WorldXMax-geo.WorldXMin+1) }
	worldY := func() int { return geo.WorldYMin + rng.IntN(geo.WorldYMax-geo.WorldYMin+1) }
	var boxes []WaterBox
	for i := range 300 {
		x, y := worldX(), worldY()
		w, h := rng.IntN(600), rng.IntN(600)
		switch i % 4 {
		case 1: // spans several buckets
			w, h = rng.IntN(5*waterBucket), rng.IntN(5*waterBucket)
		case 2: // swapped corners
			w, h = -w, -h
		case 3: // past the world's edge
			x = geo.WorldXMax - rng.IntN(300)
			w = rng.IntN(600)
		}
		z := rng.IntN(4000) - 2000
		boxes = append(boxes, WaterBox{X1: x, Y1: y, X2: x + w, Y2: y + h, MinZ: z, MaxZ: z + rng.IntN(400)})
	}
	wt := NewWater(boxes)
	scanContains := func(x, y, z int) bool {
		if x < geo.WorldXMin || x > geo.WorldXMax || y < geo.WorldYMin || y > geo.WorldYMax {
			return false
		}
		for _, b := range wt.boxes {
			if x >= b.X1 && x <= b.X2 && y >= b.Y1 && y <= b.Y2 && z >= b.MinZ && z <= b.MaxZ {
				return true
			}
		}
		return false
	}
	scanWet := func(gx, gy int, z int16) bool {
		x0, y0 := engine.WorldX(gx)-block.CellSize/2, engine.WorldY(gy)-block.CellSize/2
		x1, y1 := x0+block.CellSize-1, y0+block.CellSize-1
		for _, b := range wt.boxes {
			if x0 <= b.X2 && x1 >= b.X1 && y0 <= b.Y2 && y1 >= b.Y1 && int(z) >= b.MinZ && int(z) <= b.MaxZ {
				return true
			}
		}
		return false
	}
	hits := 0
	for i := range 200000 {
		var x, y int
		if i%2 == 0 { // near a box corner
			b := wt.boxes[rng.IntN(len(wt.boxes))]
			x, y = b.X1+rng.IntN(41)-20, b.Y2+rng.IntN(41)-20
		} else {
			x, y = worldX(), worldY()
		}
		z := rng.IntN(4400) - 2200
		if got, want := wt.Contains(x, y, z), scanContains(x, y, z); got != want {
			t.Fatalf("Contains(%d, %d, %d) = %v, want %v", x, y, z, got, want)
		}
		if x < geo.WorldXMin || x > geo.WorldXMax || y < geo.WorldYMin || y > geo.WorldYMax {
			continue
		}
		gx, gy := engine.GeoX(x), engine.GeoY(y)
		got, want := wt.wet(gx, gy, int16(z)), scanWet(gx, gy, int16(z))
		if got != want {
			t.Fatalf("wet(%d, %d, %d) = %v, want %v", gx, gy, z, got, want)
		}
		if want {
			hits++
		}
	}
	if hits == 0 {
		t.Fatal("no sampled cell is wet: the sample misses the boxes")
	}
}
