package nav

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestPlannerWetAt checks the wet test the spot registry drops spots by
// (#275): a point snapped to its cell and layer (Locate) is wet when that
// cell's floor is in a water zone; a point off the geodata or a planner
// without water is never wet.
func TestPlannerWetAt(t *testing.T) {
	p := testPlanner(t)
	p.Water = NewWater([]WaterBox{waterLake})
	high := loc(WaterInLake)
	high.Z += 500 // snaps down to the lake's floor: the only layer
	tests := []struct {
		name string
		at   location.Location
		want bool
	}{
		{"in the lake", loc(WaterInLake), true},
		{"above the lake, snapped to its floor", high, true},
		{"west shore", loc(WaterWest), false},
		{"east shore", loc(WaterEast), false},
		{"off the geodata", location.Location{X: -1 << 20, Y: -1 << 20}, false},
	}
	for _, tt := range tests {
		if got := p.WetAt(tt.at); got != tt.want {
			t.Errorf("%s: WetAt(%v) = %v, want %v", tt.name, tt.at, got, tt.want)
		}
	}
	dry := testPlanner(t)
	if dry.WetAt(loc(WaterInLake)) {
		t.Error("a planner without water: the lake's middle is wet")
	}
}
