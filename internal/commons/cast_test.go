package commons

import (
	"math"
	"testing"
)

func TestTruncInt32Sat(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want int32
	}{
		{1.9, 1},
		{-1.9, -1},
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt32},
		{math.Inf(-1), math.MinInt32},
		{3e9, math.MaxInt32},
		{-3e9, math.MinInt32},
		{math.MaxInt32, math.MaxInt32},
	} {
		if got := TruncInt32Sat(tc.in); got != tc.want {
			t.Errorf("TruncInt32Sat(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
