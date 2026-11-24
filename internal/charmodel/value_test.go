package charmodel

import (
	"math"
	"testing"
	"time"
)

func TestValue(t *testing.T) {
	// The monster hits 69.3 a swing every 2 s (TestTimeToDie) and has
	// 900 HP: the fighter kills it in 10 s, the mystic in 36 s.
	monster := Profile{MaxHP: 900, PDef: 77, MDef: 91, PAtk: 77, PAtkSpd: 250}
	melee := Role{Kind: Damage, RestTime: 2 * time.Minute}
	bolt := Role{
		Kind: Damage, RestTime: 2 * time.Minute,
		Attack: Attack{Magic: true, Power: 10, HitTime: 4 * time.Second, MPCost: 10},
	}
	heal := Role{Kind: Support, HealPower: 100, HealMP: 10, SurvivalFloor: 30 * time.Second}
	tests := []struct {
		name string
		p    Profile
		role Role
		want float64
	}{
		// 1386 HP last 40 s: a kill costs a quarter, 30 s of rest.
		{"melee", Profile{MaxHP: 1386, PDef: 77, PAtk: 100, PAtkSpd: 500}, melee, 1.5},
		{"melee dies first", Profile{MaxHP: 300, PDef: 77, PAtk: 100, PAtkSpd: 500}, melee, 0},
		// 9 casts take 90 of 180 MP, more than the 18 % of HP: 60 s rest.
		{"magic", Profile{MaxHP: 6930, MaxMP: 180, PDef: 77, MAtk: 100, MAtkSpd: 333}, bolt, 60.0 / 96},
		{"magic out of MP", Profile{MaxHP: 6930, MaxMP: 50, PDef: 77, MAtk: 100, MAtkSpd: 333}, bolt, 0},
		{"support", Profile{MaxHP: 1386, MaxMP: 200, PDef: 77}, heal, 2000},
		{"support under the floor", Profile{MaxHP: 300, MaxMP: 200, PDef: 77}, heal, 0},
	}
	for _, tt := range tests {
		if got := Value(tt.p, tt.role, monster); math.Abs(got-tt.want) > 1e-6 {
			t.Errorf("%s: Value = %v, want %v", tt.name, got, tt.want)
		}
	}
}
