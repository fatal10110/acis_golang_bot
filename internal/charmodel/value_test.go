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
	// A 5 s heal of 100 HP for 10 MP, 100 s to win the bar back.
	heal := Role{Kind: Support, HealPower: 100, HealMP: 10, HealHitTime: 5 * time.Second, RestTime: 100 * time.Second, SurvivalFloor: 30 * time.Second}
	healMAtk := heal
	healMAtk.HealMAtk = true
	untimed := heal
	untimed.HealHitTime, untimed.RestTime = 0, 0
	tests := []struct {
		name string
		p    Profile
		role Role
		want float64
	}{
		// 1386 HP last 40 s: a kill costs a quarter, 30 s of rest.
		{"melee", Profile{MaxHP: 1386, PDef: 77, PAtk: 100, PAtkSpd: 500}, melee, 1.5},
		// Melee spends no MP: max MP rests nothing away.
		{"melee max MP", Profile{MaxHP: 1386, MaxMP: 1000, PDef: 77, PAtk: 100, PAtkSpd: 500}, melee, 1.5},
		{"melee dies first", Profile{MaxHP: 300, PDef: 77, PAtk: 100, PAtkSpd: 500}, melee, 0},
		// 9 casts take 90 of 180 MP, more than the 18 % of HP: 60 s rest.
		{"magic", Profile{MaxHP: 6930, MaxMP: 180, PDef: 77, MAtk: 100, MAtkSpd: 333}, bolt, 60.0 / 96},
		// Twice the MP: the 90 MP are a quarter of it, still more than the
		// HP share: 30 s rest.
		{"magic max MP", Profile{MaxHP: 6930, MaxMP: 360, PDef: 77, MAtk: 100, MAtkSpd: 333}, bolt, 60.0 / 66},
		{"magic out of MP", Profile{MaxHP: 6930, MaxMP: 50, PDef: 77, MAtk: 100, MAtkSpd: 333}, bolt, 0},
		// 20 casts heal 2000 HP in 100 s, rested in 100 s: 600 a minute.
		{"support", Profile{MaxHP: 1386, MaxMP: 200, PDef: 77, MAtkSpd: 333}, heal, 600},
		// Twice the MP: 40 casts, 4000 HP in 200 + 100 s.
		{"support max MP", Profile{MaxHP: 1386, MaxMP: 400, PDef: 77, MAtkSpd: 333}, heal, 800},
		// Twice the cast speed: 2.5 s a cast, 2000 HP in 50 + 100 s.
		{"support cast speed", Profile{MaxHP: 1386, MaxMP: 200, PDef: 77, MAtkSpd: 666}, heal, 800},
		// A HEAL adds sqrt(400) = 20 to each cast: 2400 HP in 200 s.
		{"support M. Atk", Profile{MaxHP: 1386, MaxMP: 200, PDef: 77, MAtk: 400, MAtkSpd: 333}, healMAtk, 720},
		{"support static heal ignores M. Atk", Profile{MaxHP: 1386, MaxMP: 200, PDef: 77, MAtk: 400, MAtkSpd: 333}, heal, 600},
		{"support untimed", Profile{MaxHP: 1386, MaxMP: 200, PDef: 77, MAtkSpd: 333}, untimed, 0},
		{"support under the floor", Profile{MaxHP: 300, MaxMP: 200, PDef: 77, MAtkSpd: 333}, heal, 0},
	}
	for _, tt := range tests {
		if got := Value(tt.p, tt.role, monster); math.Abs(got-tt.want) > 1e-6 {
			t.Errorf("%s: Value = %v, want %v", tt.name, got, tt.want)
		}
	}
}
