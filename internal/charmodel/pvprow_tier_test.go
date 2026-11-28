package charmodel

import (
	"slices"
	"testing"
)

// TestRowClasses checks the rows the mix shares equally when nobody has
// been seen (docs/pvp/model.md "Row table"): the base classes' rows for a
// base class, the first professions' for a first profession, the 24
// second rows for a second or third profession, SC once.
func TestRowClasses(t *testing.T) {
	base := []int32{0, 10, 18, 25, 31, 38, 44, 49, 53}
	first := []int32{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54}
	second := []int32{2, 3, 5, 8, 9, 12, 16, 17, 20, 21, 23, 24, 27, 30, 33, 34, 36, 37, 40, 43, 46, 48, 52, 55}
	for _, tc := range []struct {
		tier int
		want []int32
	}{{0, base}, {1, first}, {2, second}, {3, second}} {
		if got := RowClasses(tc.tier); !slices.Equal(got, tc.want) {
			t.Errorf("RowClasses(%d) = %v, want %v", tc.tier, got, tc.want)
		}
	}
}
