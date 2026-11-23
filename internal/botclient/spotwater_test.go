package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// TestDrySpots drops the spots whose centre snaps to a wet cell (#275) and
// keeps the rest in order: a spot on the shore, one past the lake, one whose
// centre has no geodata. Without navigation or water none is dropped.
func TestDrySpots(t *testing.T) {
	n, err := buildTestNavWater(geotest.Scenes(), navWindow(0, 0, 1, 0), nav.NewWater([]nav.WaterBox{swimLake}))
	if err != nil {
		t.Fatal(err)
	}
	shore := testSpot("shore", 3500, 500, 1, 5, 3)
	lake := testSpot("lake", 5500, 500, 1, 5, 3)
	above := testSpot("above the lake", 5200, 800, 1, 5, 3)
	above.Z = 400 // snaps down to the lake's floor
	far := testSpot("far", 6500, 500, 1, 5, 3)
	nowhere := testSpot("no geodata", -200000, 300000, 1, 5, 3)
	spots := []datapack.Spot{shore, lake, far, above, nowhere}
	dryNav := *n
	dryNav.Water = nil
	for _, tc := range []struct {
		name    string
		n       *Nav
		want    []string
		dropped int
	}{
		{"with water", n, []string{"shore", "far", "no geodata"}, 2},
		{"without water", &dryNav, []string{"shore", "lake", "far", "above the lake", "no geodata"}, 0},
		{"without navigation", nil, []string{"shore", "lake", "far", "above the lake", "no geodata"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, dropped := tc.n.DrySpots(slices.Clone(spots))
			var names []string
			for _, sp := range got {
				names = append(names, sp.Maker)
			}
			if !slices.Equal(names, tc.want) || dropped != tc.dropped {
				t.Errorf("DrySpots = %v, dropped %d; want %v, dropped %d", names, dropped, tc.want, tc.dropped)
			}
		})
	}
}
