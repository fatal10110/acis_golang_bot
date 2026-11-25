package botclient

import (
	"math"
	"os"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestValuablesShipped pins the figures of docs/valuables/score.md on the
// shipped datapack at the default supply config and policy.
func TestValuablesShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatalf("Load(%s): %v", dir, err)
	}
	m := NewTravelMap(dp, DefaultSupplyConfig())
	p := defaultValuablePolicy()
	tests := []struct {
		name     string
		id       int32
		ref      int32
		s, r, v  float64
		valuable bool
		// unmarked is the figure's R where a mark sets R to 0: R from S
		// alone.
		unmarked float64
	}{
		{name: "Sword of Watershadow", id: 121, ref: 136000, s: 0.67, r: 2.17, v: 612000, valuable: true},
		{name: "Tears of Eva", id: 309, ref: 136000, s: 0.62, r: 2.21, v: 628000, valuable: true},
		{name: "Elven Tunic", id: 433, ref: 99900, s: 0.44, r: 2.35, v: 510000, valuable: true},
		{name: "Conjuror's Staff, craftable", id: 184, ref: 644000, s: 0.056, r: 1.63, v: 1988000, valuable: true},
		// The figures give the D scroll R 0.27 and V 60100, from S alone;
		// but spawned NPCs' multisell rows make it of no weapon or armor
		// (multisell/102.xml:83-85, NPC 31688 at spawnlist/24_16.xml:717),
		// which the rarity rule scores R 0: V is its price. Not valuable
		// either way.
		{name: "Scroll: Enchant Weapon (Grade D)", id: 955, ref: 50000, s: 54.3, r: 0, v: 50000, unmarked: 0.27},
		{name: "Fist of Butcher", id: 2371, ref: 25500, s: 3.77, r: 1.42, v: 68400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := m.valueScore(p, tt.id, 0)
			t.Logf("ref %d S %.4f R %.4f V %.0f", sc.ref, sc.s, sc.r, sc.v)
			// The figures are rounded: S to 2 significant digits, R to
			// 0.01, V to 3 significant digits.
			if sc.ref != tt.ref || math.Abs(sc.s-tt.s) > tt.s*0.01 || math.Abs(sc.r-tt.r) > 0.006 ||
				math.Abs(sc.v-tt.v) > tt.v*0.005 {
				t.Errorf("ref %d S %v R %v V %v, want %d, %v, %v, %v", sc.ref, sc.s, sc.r, sc.v, tt.ref, tt.s, tt.r, tt.v)
			}
			if tt.unmarked != 0 {
				if r := math.Log10(p.CommonSupply / sc.s); math.Abs(r-tt.unmarked) > 0.006 {
					t.Errorf("R from S alone %v, want %v", r, tt.unmarked)
				}
			}
			if got := p.valuable(sc); got != tt.valuable {
				t.Errorf("valuable = %v, want %v", got, tt.valuable)
			}
		})
	}
}
