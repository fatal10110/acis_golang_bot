package charmodel

import (
	"math"
	"testing"
	"time"
)

// TestHealTerms checks a heal's amount and period: 1011 Heal level 4
// (power 59, hit time 5 s, reuse 0.5 s) adds sqrt(M. Atk) as a HEAL, a heal
// over time (1229 Chant of Life level 1: 15 ticks of 12) adds nothing, and
// the cast speed shortens the cast (castPeriod, held to 500 ms).
func TestHealTerms(t *testing.T) {
	heal := Heal{Power: 59, MAtk: true, HitTime: 5 * time.Second, Reuse: 500 * time.Millisecond, MPCost: 23}
	chant := Heal{Power: 15 * 12, HitTime: 7 * time.Second, Reuse: 20 * time.Second, MPCost: 47}
	tests := []struct {
		name   string
		p      Profile
		h      Heal
		amount float64
		period time.Duration
	}{
		{"heal at cast speed 333", Profile{MAtk: 16, MAtkSpd: 333}, heal, 63, 5 * time.Second},
		{"heal at cast speed 666", Profile{MAtk: 16, MAtkSpd: 666}, heal, 63, 2500 * time.Millisecond},
		{"heal with no M. Atk", Profile{MAtkSpd: 333}, heal, 59, 5 * time.Second},
		{"chant of life by its reuse", Profile{MAtk: 16, MAtkSpd: 333}, chant, 180, 20 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HealAmount(tt.p, tt.h); math.Abs(got-tt.amount) > 1e-9 {
				t.Errorf("HealAmount = %v, want %v", got, tt.amount)
			}
			if got := HealPeriod(tt.p, tt.h); got != tt.period {
				t.Errorf("HealPeriod = %v, want %v", got, tt.period)
			}
		})
	}
}

// TestTimeTogether checks kill times combine by their rates.
func TestTimeTogether(t *testing.T) {
	tests := []struct {
		name string
		ts   []time.Duration
		want time.Duration
	}{
		{"alone", []time.Duration{30 * time.Second}, 30 * time.Second},
		{"two equal", []time.Duration{30 * time.Second, 30 * time.Second}, 15 * time.Second},
		{"10 s and 15 s", []time.Duration{10 * time.Second, 15 * time.Second}, 6 * time.Second},
		{"one who never kills adds nothing", []time.Duration{30 * time.Second, never}, 30 * time.Second},
		{"nobody kills", []time.Duration{never, never}, never},
		{"none", nil, never},
		{"one at once", []time.Duration{0, 30 * time.Second}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TimeTogether(tt.ts...); (got - tt.want).Abs() > time.Microsecond {
				t.Fatalf("TimeTogether = %v, want %v", got, tt.want)
			}
		})
	}
}
