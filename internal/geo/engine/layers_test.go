package engine_test

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
)

func TestAppendLayers(t *testing.T) {
	e := engine.New()
	region, err := block.ParseRegion(storedScenes(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetRegion(geotest.RegionX, geotest.RegionY, region); err != nil {
		t.Fatal(err)
	}
	geo := func(p geotest.Point) (int, int) {
		x, y, _ := p.World()
		return engine.GeoX(x), engine.GeoY(y)
	}
	tests := []struct {
		name string
		at   geotest.Point
		want []int16
	}{
		{"bridge: lowest floor first", geotest.BridgeWestDeck, []int16{geotest.Ground, geotest.Deck}},
		{"plateau", geotest.LedgeHigh, []int16{geotest.Plateau}},
		{"unloaded region: one open layer at 0", geotest.Point{CellX: -4, CellY: 100}, []int16{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf [block.MaxLayers]block.Cell
			x, y := geo(tt.at)
			got := e.AppendLayers(buf[:0], x, y)
			if len(got) != len(tt.want) {
				t.Fatalf("layers = %v, want heights %v", got, tt.want)
			}
			for i := range got {
				if got[i].Height != tt.want[i] {
					t.Fatalf("layers = %v, want heights %v", got, tt.want)
				}
			}
			if allocs := testing.AllocsPerRun(100, func() { _ = e.AppendLayers(buf[:0], x, y) }); allocs != 0 {
				t.Fatalf("AppendLayers allocations = %.0f, want 0", allocs)
			}
		})
	}
}

// storedScenes returns the fixture scenes in the stored region form.
func storedScenes(t *testing.T) []byte {
	t.Helper()
	path, err := geotest.Scenes().WriteL2OFF(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := reader.ReadL2OFF(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored bytes.Buffer
	if _, err := decoded.WriteTo(&stored); err != nil {
		t.Fatal(err)
	}
	return stored.Bytes()
}
