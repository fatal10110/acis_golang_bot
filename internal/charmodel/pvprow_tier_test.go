package charmodel

import (
	"slices"
	"testing"
)

// TestRowClasses checks the rows the mix shares equally when nobody has
// been seen (docs/pvp/model.md "Row table"): the base classes' rows for a
// base class, the first professions' for every profession tier, SC once.
func TestRowClasses(t *testing.T) {
	base := []int32{0, 10, 18, 25, 31, 38, 44, 49, 53}
	first := []int32{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54}
	for _, tc := range []struct {
		tier int
		want []int32
	}{{0, base}, {1, first}, {2, first}, {3, first}} {
		if got := RowClasses(tc.tier); !slices.Equal(got, tc.want) {
			t.Errorf("RowClasses(%d) = %v, want %v", tc.tier, got, tc.want)
		}
	}
}
