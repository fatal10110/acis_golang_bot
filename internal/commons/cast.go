// Package commons holds small helpers copied from the server's commons package.
package commons

import "math"

// TruncInt32Sat narrows f to an int32 by truncating toward zero, saturating
// at the int32 range, with NaN as 0.
func TruncInt32Sat(f float64) int32 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	}
	return int32(f)
}
