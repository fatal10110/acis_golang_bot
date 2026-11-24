package charmodel

import (
	"math"
	"testing"
)

// TestStatBonus checks the curves against rows of the server's
// skill/statbonus/testdata/statbonus.golden, and the clamp of a stat
// outside the tables to their edges.
func TestStatBonus(t *testing.T) {
	tests := []struct {
		v                            int
		str, con, dex, int, wit, men float64
		evasionAccuracy              float64
	}{
		{0, 0.29, 0.44, 0.84, 0.54, 0.38, 1.0, 0},
		{20, 0.59, 0.8, 1.01, 0.8, 1.0, 1.22, 26.832815729997478},
		{30, 0.84, 1.07, 1.1, 0.97, 1.63, 1.35, 32.863353450309965},
		{43, 1.33, 1.58, 1.24, 1.26, 3.07, 1.53, 39.344631145812},
		{99, 9.67, 8.24, 2.04, 3.82, 47.2, 2.68, 59.69924622639719},
		// Below and above the tables: the edge rows.
		{-5, 0.29, 0.44, 0.84, 0.54, 0.38, 1.0, 0},
		{150, 9.67, 8.24, 2.04, 3.82, 47.2, 2.68, 59.69924622639719},
	}
	for _, tt := range tests {
		got := []float64{STRBonus(tt.v), CONBonus(tt.v), DEXBonus(tt.v), INTBonus(tt.v), WITBonus(tt.v), MENBonus(tt.v), EvasionAccuracy(tt.v)}
		want := []float64{tt.str, tt.con, tt.dex, tt.int, tt.wit, tt.men, tt.evasionAccuracy}
		for i, name := range []string{"STR", "CON", "DEX", "INT", "WIT", "MEN", "EvasionAccuracy"} {
			if math.Abs(got[i]-want[i]) > 1e-9 {
				t.Errorf("%s(%d) = %v, want %v", name, tt.v, got[i], want[i])
			}
		}
	}
}

// TestLevelMod checks the level factor (89+level)/100, level at least 1.
func TestLevelMod(t *testing.T) {
	tests := []struct {
		level int
		want  float64
	}{
		{0, 0.9},
		{1, 0.9},
		{20, 1.09},
		{80, 1.69},
	}
	for _, tt := range tests {
		if got := LevelMod(tt.level); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("LevelMod(%d) = %v, want %v", tt.level, got, tt.want)
		}
	}
}
