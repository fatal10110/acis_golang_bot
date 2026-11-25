package charmodel

import (
	"math"
	"time"
)

// RoleKind is what a role values a build by.
type RoleKind int

const (
	// Damage roles value the kills a minute they sustain, rests included.
	Damage RoleKind = iota
	// Support roles value the HP a minute they heal, rests included, while
	// they survive.
	Support
)

// The persona defaults of a role (D6: configurable, not a safety
// boundary).
const (
	// DefaultRestTime is how long resting from no HP or MP to full takes.
	DefaultRestTime = 2 * time.Minute
	// DefaultSurvivalFloor is how long a support must last against the
	// target for its heals to count.
	DefaultSurvivalFloor = 30 * time.Second
)

// Role is what a build is valued for.
type Role struct {
	Kind RoleKind
	// Attack is what a damage role fights with.
	Attack Attack
	// RestTime is how long the role rests from no HP or MP to full; the
	// share of HP or MP a kill costs, or a support's full MP bar, is
	// rested at that rate.
	RestTime time.Duration
	// HealPower and HealMP are a support's heal: the HP one cast restores
	// before M. Atk counts and the MP it takes. HealHitTime and HealReuse
	// time it as a magic cast (castPeriod: cast speed shortens both).
	// HealMAtk is set for a heal the server adds sqrt(M. Atk) to (skill
	// type HEAL, unshot; a HEAL_STATIC adds nothing: formulas/heal.go
	// HealAmount).
	HealPower, HealMP      float64
	HealHitTime, HealReuse time.Duration
	HealMAtk               bool
	// SurvivalFloor is how long a support must last against the target;
	// below it the build is worth nothing.
	SurvivalFloor time.Duration
}

// Value is the one number errands compare builds by, for role against
// target (a monster's profile). For a damage role it is the kills a minute
// a fight sustains: each kill takes its time to kill plus the rest that
// wins back the larger share of HP or MP the kill cost; 0 when the role
// dies before the kill or cannot pay a cast. For a support role it is the
// HP a minute it heals: a full MP bar's casts (MaxMP/HealMP) of the heal
// (HealPower, plus sqrt(MAtk) with HealMAtk), over the time they take
// (castPeriod each) plus RestTime to win the bar back; 0 when it dies
// within SurvivalFloor or the casts and the rest take no time.
func Value(p Profile, role Role, target Profile) float64 {
	if role.Kind == Support {
		if role.HealMP <= 0 || TimeToDie(p, target) < role.SurvivalFloor {
			return 0
		}
		casts, heal := p.MaxMP/role.HealMP, role.HealPower
		if role.HealMAtk {
			heal += math.Sqrt(max(p.MAtk, 0))
		}
		each := castPeriod(p, Attack{Magic: true, HitTime: role.HealHitTime, Reuse: role.HealReuse})
		cycle := casts*float64(each) + float64(role.RestTime)
		if cycle <= 0 {
			return 0
		}
		return float64(time.Minute) * casts * heal / cycle
	}
	ttk, ttd := TimeToKill(p, role.Attack, target), TimeToDie(p, target)
	if ttk == never || ttk >= ttd {
		return 0
	}
	spent := float64(ttk) / float64(ttd)
	if mp := actionsToKill(p, role.Attack, target) * role.Attack.MPCost; mp > 0 {
		if p.MaxMP < mp {
			return 0
		}
		spent = max(spent, mp/p.MaxMP)
	}
	cycle := float64(ttk) + spent*float64(role.RestTime)
	return float64(time.Minute) / cycle
}
