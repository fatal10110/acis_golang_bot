package charmodel

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestEffectLand checks an effect lands only after its skill's own roll:
// an effect with its own effect power multiplies the two rolls, one
// without lands with the skill, a typeless one rolls its bare power and a
// CANCEL always lands once the skill has.
func TestEffectLand(t *testing.T) {
	caster, target := testFighter("WA"), testFighter("WA")
	r := LandRate(testStunAttack, caster, target)
	cancel := datapack.Skill{
		Type: "CANCEL", Power: 50, MagicLevel: 20, LevelDepend: 1,
		Effects: []datapack.Effect{{Name: "Cancel", EffectPower: 40, EffectType: "CANCEL"}},
	}
	typeless := datapack.Skill{
		Type: "PDAM", Power: 100, MagicLevel: 20, LevelDepend: 1,
		Effects: []datapack.Effect{{Name: "Debuff", EffectPower: 30}},
	}
	for _, tt := range []struct {
		name string
		s    datapack.Skill
		want float64
	}{
		{"own effect power", testStunAttack, r * r / 100},
		{"no own power", testShieldStun, LandRate(testShieldStun, caster, target)},
		{"typeless", typeless, LandRate(typeless, caster, target) * 30 / 100},
		{"cancel", cancel, LandRate(cancel, caster, target)},
	} {
		if got := effectLand(tt.s, tt.s.Effects[0], caster, target); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s: effect land = %v, want %v", tt.name, got, tt.want)
		}
	}
	if r <= 0 || r >= 100 {
		t.Fatalf("Stun Attack land rate = %v, want inside (0, 100)", r)
	}
}

// TestSleepHoldsUntilNextHit checks a Sleep holds its target only until the
// holder's next plain hit, not for the effect's full time.
func TestSleepHoldsUntilNextHit(t *testing.T) {
	sleep := datapack.Skill{
		Type: "SLEEP", Magic: true, Power: 70, MagicLevel: 20, LevelDepend: 2,
		HitTime: 1500 * time.Millisecond, ReuseDelay: 30 * time.Second,
		Effects: []datapack.Effect{{Name: "Sleep", Time: 30, EffectPower: -1}},
	}
	root := sleep
	root.Type = "ROOT"
	root.Effects = []datapack.Effect{{Name: "Root", Time: 30, EffectPower: -1}}
	caster, target := testCaster("HM"), testFighter("WA")
	period := castPeriod(caster.Profile, Attack{Magic: true, Power: 70, HitTime: sleep.HitTime, Reuse: sleep.ReuseDelay}).Seconds()
	interval := AttackInterval(caster.Profile.PAtkSpd).Seconds()
	for _, tt := range []struct {
		name string
		s    datapack.Skill
		held float64
	}{
		{"sleep", sleep, interval},
		{"root", root, min(30, period)},
	} {
		rows := RowTable{Row{ID: "HM"}: {{Skill: tt.s, Level: 20}}}
		want := LandRate(tt.s, caster, target) / 100 * tt.held / period
		if got := play(caster, target, rows).hold; math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: hold = %v, want %v", tt.name, got, want)
		}
	}
}
