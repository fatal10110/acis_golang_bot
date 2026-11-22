package botclient

import (
	"testing"
)

// TestSidestep checks the sidesteps' ends against a target off the axes.
func TestSidestep(t *testing.T) {
	from, to := Point{X: 100, Y: 100, Z: -3717}, Point{X: 400, Y: 500, Z: -3000}
	tests := []struct {
		turn float64
		want Point
	}{
		{90, Point{X: -140, Y: 280, Z: -3717}},
		{-90, Point{X: 340, Y: -80, Z: -3717}},
		{180, Point{X: -80, Y: -140, Z: -3717}},
		{45, Point{X: 58, Y: 397, Z: -3717}},
	}
	for _, tt := range tests {
		if got := sidestep(from, to, 500, tt.turn); got != tt.want {
			t.Errorf("sidestep %v = %+v, want %+v", tt.turn, got, tt.want)
		}
	}
}
