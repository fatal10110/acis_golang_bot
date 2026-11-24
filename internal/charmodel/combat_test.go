package charmodel

import (
	"math"
	"testing"
	"time"
)

func TestDamage(t *testing.T) {
	tests := []struct {
		name      string
		got, want float64
	}{
		{"melee", PhysicalDamage(100, 77, false), 100},
		{"melee soulshot", PhysicalDamage(100, 77, true), 200},
		{"skill", PhysicalSkillDamage(100, 50, 77, false), 150},
		{"skill soulshot", PhysicalSkillDamage(100, 50, 77, true), 254},
		{"magic", MagicDamage(100, 91, 10, NoShot), 100},
		{"magic spiritshot", MagicDamage(100, 91, 10, Soulshot), 100 * math.Sqrt2},
		{"magic blessed", MagicDamage(100, 91, 10, BlessedSpiritshot), 200},
	}
	for _, tt := range tests {
		if math.Abs(tt.got-tt.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestHitRateAndInterval(t *testing.T) {
	for _, tt := range []struct{ acc, eva, want float64 }{
		{0, 0, 900}, {0, 10, 700}, {4, 0, 980}, {10, 0, 980}, {0, 40, 300},
	} {
		if got := HitRate(tt.acc, tt.eva); got != tt.want {
			t.Errorf("HitRate(%v, %v) = %v, want %v", tt.acc, tt.eva, got, tt.want)
		}
	}
	for _, tt := range []struct {
		speed float64
		want  time.Duration
	}{
		{500, time.Second}, {333, 1501 * time.Millisecond}, {10000, 100 * time.Millisecond}, {0, 500 * time.Second},
	} {
		if got := AttackInterval(tt.speed); got != tt.want {
			t.Errorf("AttackInterval(%v) = %v, want %v", tt.speed, got, tt.want)
		}
	}
}

func TestTimeToKill(t *testing.T) {
	fighter := Profile{PAtk: 100, PAtkSpd: 500}
	mystic := Profile{MAtk: 100, MAtkSpd: 333}
	target := Profile{MaxHP: 900, PDef: 77, MDef: 91}
	bolt := Attack{Magic: true, Power: 10, HitTime: 4 * time.Second, Reuse: 2 * time.Second}
	tests := []struct {
		name   string
		p      Profile
		a      Attack
		target Profile
		want   time.Duration
	}{
		// 100 a hit landing 90 %: 10 hits a second apart.
		{"melee", fighter, Attack{}, target, 10 * time.Second},
		// a 10 % critical rate: 99 a swing.
		{"melee critical", Profile{PAtk: 100, PAtkSpd: 500, Critical: 100}, Attack{}, target, dur(float64(time.Second) * 900 / 99)},
		// 100 a cast: 9 casts of 4 s.
		{"magic", mystic, bolt, target, 36 * time.Second},
		// twice the cast speed: 2 s casts.
		{"magic fast", Profile{MAtk: 100, MAtkSpd: 666}, bolt, target, 18 * time.Second},
		// reuse longer than the cast time.
		{"magic reuse", Profile{MAtk: 100, MAtkSpd: 666}, Attack{Magic: true, Power: 10, HitTime: time.Second, Reuse: 6 * time.Second}, target, 27 * time.Second},
		// a spiritshot: 141.42 a cast of 2.8 s.
		{"magic spiritshot", mystic, Attack{Magic: true, Power: 10, HitTime: 4 * time.Second, Shot: Soulshot}, target, dur(float64(2800*time.Millisecond) * 900 / (100 * math.Sqrt2))},
		// a 1 s cast at 1332 speed is held to 500 ms.
		{"cast floor", Profile{MAtk: 100, MAtkSpd: 1332}, Attack{Magic: true, Power: 10, HitTime: time.Second}, target, 4500 * time.Millisecond},
		{"no damage", Profile{PAtkSpd: 500}, Attack{}, target, never},
	}
	for _, tt := range tests {
		got := TimeToKill(tt.p, tt.a, tt.target)
		if d := got - tt.want; d < -time.Microsecond || d > time.Microsecond {
			t.Errorf("%s: TimeToKill = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTimeToDie(t *testing.T) {
	// 77 a hit landing 90 %, 2 s apart, on 450 HP: 6.49 hits.
	monster := Profile{PAtk: 77, PAtkSpd: 250}
	got := TimeToDie(Profile{MaxHP: 450, PDef: 77}, monster)
	want := dur(float64(2*time.Second) * 450 / 69.3)
	if d := got - want; d < -time.Microsecond || d > time.Microsecond {
		t.Errorf("TimeToDie = %v, want %v", got, want)
	}
}

// dur converts a computed number of nanoseconds.
func dur(ns float64) time.Duration { return time.Duration(ns) }
