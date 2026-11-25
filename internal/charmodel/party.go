package charmodel

import (
	"math"
	"time"
)

// The party terms of a fight (docs/parties/model.md): a pack monster falls
// to the summed damage rate of every attacker, and the one who pulls lasts
// longer by what the healers restore.

// Heal is a heal a healer casts over and over on whoever pulls.
type Heal struct {
	// Power is the HP one cast restores before M. Atk counts; for a heal
	// over time, its ticks' total.
	Power float64
	// MAtk adds sqrt(M. Atk) to every cast: a HEAL, unshot
	// (formulas/heal.go HealAmount); a HEAL_STATIC or a heal over time adds
	// nothing.
	MAtk bool
	// HitTime and Reuse are the skill's base cast time and reuse delay,
	// timed as a magic cast (castPeriod).
	HitTime, Reuse time.Duration
	// MPCost is the MP one cast takes.
	MPCost float64
}

// HealAmount is the HP one cast of h by p restores.
func HealAmount(p Profile, h Heal) float64 {
	if h.MAtk {
		return h.Power + math.Sqrt(max(p.MAtk, 0))
	}
	return h.Power
}

// HealPeriod is the time between two casts of h by p: its cast time and
// its reuse, scaled by p's cast speed, whichever is longer.
func HealPeriod(p Profile, h Heal) time.Duration {
	return castPeriod(p, Attack{Magic: true, HitTime: h.HitTime, Reuse: h.Reuse})
}

// TimeTogether is how long attackers who each kill a target alone in one
// of ts take to kill it together: their damage rates summed, 1/Σ(1/t). An
// attacker who never kills it adds nothing; never when none does, 0 when
// one kills it at once.
func TimeTogether(ts ...time.Duration) time.Duration {
	var rate float64
	for _, t := range ts {
		switch {
		case t <= 0:
			return 0
		case t != never:
			rate += 1 / float64(t)
		}
	}
	if rate == 0 {
		return never
	}
	return scale(1, 1/rate)
}
