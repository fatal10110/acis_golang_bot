package geotest_test

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/dynamic"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
)

// sceneRegion reads the scenes back through the L2OFF reader and the stored
// form, as bots geopack and the mapped cache do.
func sceneRegion(t *testing.T) (*geotest.Terrain, *block.Region) {
	t.Helper()
	terrain := geotest.Scenes()
	path, err := terrain.WriteL2OFF(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	decoded, trailing, err := reader.ReadL2OFF(path)
	if err != nil {
		t.Fatalf("ReadL2OFF(): %v", err)
	}
	if trailing != 0 {
		t.Fatalf("ReadL2OFF() left %d trailing bytes, want it to consume exactly its input", trailing)
	}
	var stored bytes.Buffer
	if _, err := decoded.WriteTo(&stored); err != nil {
		t.Fatal(err)
	}
	if _, err := block.ParseRegion(append(stored.Bytes(), 0)); err == nil {
		t.Fatal("ParseRegion() accepted a stored region with a trailing byte")
	}
	parsed, err := block.ParseRegion(stored.Bytes())
	if err != nil {
		t.Fatalf("ParseRegion(): %v", err)
	}
	if !bytes.Equal(parsed.Stored(), stored.Bytes()) {
		t.Fatal("ParseRegion() region does not view its input")
	}
	return terrain, parsed
}

func sceneEngine(t *testing.T) (*geotest.Terrain, *engine.Engine) {
	t.Helper()
	terrain, region := sceneRegion(t)
	e := engine.New()
	if err := e.SetRegion(geotest.RegionX, geotest.RegionY, region); err != nil {
		t.Fatal(err)
	}
	return terrain, e
}

// TestScenesRoundTrip pins that every scene cell, every layer of it, reads
// back through the reader and the stored form with its height and NSWE, and
// that the region holds flat, complex and multilayer blocks.
func TestScenesRoundTrip(t *testing.T) {
	terrain, region := sceneRegion(t)
	e := engine.New()
	if err := e.SetRegion(geotest.RegionX, geotest.RegionY, region); err != nil {
		t.Fatal(err)
	}
	geoX0 := engine.GeoX(geotest.CellWorldX(0))
	geoY0 := engine.GeoY(geotest.CellWorldY(0))
	for x := 248; x < 1600; x++ {
		for y := 248; y < 328; y++ {
			for _, c := range terrain.Layers(x, y) {
				if got := e.HeightNearest(geoX0+x, geoY0+y, int(c.Height)); got != c.Height {
					t.Fatalf("cell %d,%d layer %d: height %d", x, y, c.Height, got)
				}
				if got := e.NSWENearest(geoX0+x, geoY0+y, int(c.Height)); got != c.NSWE {
					t.Fatalf("cell %d,%d layer %d: NSWE %v, want %v", x, y, c.Height, got, c.NSWE)
				}
			}
		}
	}
	for _, tt := range []struct {
		name     string
		cx, cy   int
		wantKind block.Kind
	}{
		{"open ground", 8, 8, block.KindFlat},
		{"corridor", geotest.CorridorWest.CellX, geotest.CorridorWest.CellY, block.KindComplex},
		{"bridge", geotest.BridgeWestDeck.CellX, geotest.BridgeWestDeck.CellY, block.KindMultilayer},
	} {
		if got := region.KindAt(tt.cx/block.CellsX, tt.cy/block.CellsY); got != tt.wantKind {
			t.Errorf("%s block kind = %v, want %v", tt.name, got, tt.wantKind)
		}
	}
}

// TestScenesMovement pins what each scene is for: the CanMove and Height
// answers later planners are tested against.
func TestScenesMovement(t *testing.T) {
	_, e := sceneEngine(t)
	tests := []struct {
		name     string
		from, to geotest.Point
		want     bool
	}{
		{"along the corridor", geotest.CorridorWest, geotest.CorridorEast, true},
		{"through the corridor wall", geotest.CorridorWest, geotest.AboveCorridor, false},
		{"through the U base", geotest.UInside, geotest.UBehind, false},
		{"off the ledge", geotest.LedgeHigh, geotest.LedgeLow, true},
		{"onto the ledge", geotest.LedgeLow, geotest.LedgeHigh, false},
		{"into the pit", geotest.PitOutside, geotest.PitInside, true},
		{"out of the pit", geotest.PitInside, geotest.PitOutside, false},
		{"along the deck", geotest.BridgeWestDeck, geotest.BridgeEastDeck, true},
		{"under the bridge", geotest.BridgeWestGround, geotest.BridgeEastGround, true},
		{"into the pocket", geotest.PocketOutside, geotest.PocketInside, false},
		{"out of the pocket", geotest.PocketInside, geotest.PocketOutside, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ox, oy, oz := tt.from.World()
			tx, ty, tz := tt.to.World()
			if got := e.CanMove(ox, oy, oz, tx, ty, tz); got != tt.want {
				t.Errorf("CanMove(%v -> %v) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}

	x, y, _ := geotest.BridgeWestDeck.World()
	if got := e.Height(x, y, geotest.Deck+10); got != geotest.Deck {
		t.Errorf("Height on the bridge from above = %d, want the deck %d", got, geotest.Deck)
	}
	if got := e.Height(x, y, geotest.Ground); got != geotest.Ground {
		t.Errorf("Height under the bridge = %d, want the ground %d", got, geotest.Ground)
	}
}

// TestCorridorDoorTogglesMovement pins the door overlay on the copied
// engine: a closed door built from its template blocks the corridor, and
// removing it opens the corridor again.
func TestCorridorDoorTogglesMovement(t *testing.T) {
	_, e := sceneEngine(t)
	obj, err := dynamic.NewDoorObject(geotest.CorridorDoor(), e)
	if err != nil {
		t.Fatalf("NewDoorObject(): %v", err)
	}
	ox, oy, oz := geotest.CorridorWest.World()
	tx, ty, tz := geotest.CorridorEast.World()

	e.AddObject(obj)
	if e.CanMove(ox, oy, oz, tx, ty, tz) {
		t.Error("CanMove through the closed door = true, want false")
	}
	e.RemoveObject(obj)
	if !e.CanMove(ox, oy, oz, tx, ty, tz) {
		t.Error("CanMove through the open doorway = false, want true")
	}
}
