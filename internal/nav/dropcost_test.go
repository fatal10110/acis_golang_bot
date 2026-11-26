package nav

import "testing"

// TestDropCost pins the drop weight of #514: nothing up to dropBound, then
// costDrop for each whole dropBound of the fall, so a cliff weighs more than
// a ledge as its fall damage does.
func TestDropCost(t *testing.T) {
	for _, tc := range []struct {
		name  string
		z, to int16
		want  int32
	}{
		{"climb", 0, 40, 0},
		{"level", 0, 0, 0},
		{"slope step", 0, -72, 0},
		{"at the bound", 0, -dropBound, 0},
		{"just past the bound", 0, -dropBound - 1, costDrop},
		{"orc wall", -240, -240 - 312, 3 * costDrop},
		{"rune ledge", -1784, -2080, 3 * costDrop},
		{"rune cliff", 912, -1912, 29 * costDrop},
	} {
		if got := dropCost(tc.z, tc.to); got != tc.want {
			t.Errorf("%s: dropCost(%d, %d) = %d, want %d", tc.name, tc.z, tc.to, got, tc.want)
		}
	}
}

// TestPackCross pins a crossing's packed buildCross.side: the border side
// comes back unchanged, the drop multiple is capped at maxCrossDrop, and the
// crossing edge gains that multiple of costDrop.
func TestPackCross(t *testing.T) {
	for _, side := range []int{sideEast, sideWest, sideSouth, sideNorth} {
		for _, tc := range []struct {
			name     string
			drop     int32
			multiple uint32
		}{
			{"no drop", 0, 0},
			{"ledge", costDrop, 1},
			{"rune cliff", 29 * costDrop, 29},
			{"at the cap", maxCrossDrop * costDrop, maxCrossDrop},
			{"past the cap", 200 * costDrop, maxCrossDrop},
		} {
			packed := packCross(side, tc.drop)
			if got := crossSide(packed); got != side {
				t.Errorf("side %d, %s: crossSide = %d", side, tc.name, got)
			}
			if got, want := crossCost(packed), tc.multiple*costDrop; got != want {
				t.Errorf("side %d, %s: crossCost = %d, want %d", side, tc.name, got, want)
			}
		}
	}
}
