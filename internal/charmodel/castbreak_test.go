package charmodel

import (
	"math"
	"testing"
	"time"
)

// TestCastBreakOdds checks the break-cast gate's odds: one roll per plain
// hit inside the window, at the hit's damage against the caster's MEN.
func TestCastBreakOdds(t *testing.T) {
	attacker := Fighter{Profile: Profile{PAtk: 200, PAtkSpd: 333}}
	caster := Fighter{Profile: Profile{PDef: 100}, MEN: 39}
	hit := CastBreakChance(hitDamage(attacker.Profile, Attack{}, caster.Profile), caster.MEN) / 100
	interval := AttackInterval(attacker.Profile.PAtkSpd)
	tests := []struct {
		name   string
		window time.Duration
		want   float64
	}{
		{"no hit inside the window", interval - time.Millisecond, 0},
		{"one hit", interval, 100 * hit},
		{"three hits", 3*interval + interval/2, 100 * (1 - math.Pow(1-hit, 3))},
	}
	for _, tt := range tests {
		if got := CastBreakOdds(attacker, caster, tt.window); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s: CastBreakOdds = %v, want %v", tt.name, got, tt.want)
		}
	}
	strong := attacker
	strong.Mul = map[string]float64{"pvpPhysDmg": 4}
	if CastBreakOdds(strong, caster, interval) <= CastBreakOdds(attacker, caster, interval) {
		t.Error("a PvP damage bonus does not raise the odds")
	}
}
