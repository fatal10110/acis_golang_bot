package charmodel

import (
	"math"
	"time"
)

// Shot is the shot an attack is charged with.
type Shot int

const (
	NoShot Shot = iota
	// Soulshot is a soulshot on a physical attack, a spiritshot on a magic
	// one.
	Soulshot
	// BlessedSpiritshot is a blessed spiritshot on a magic attack; on a
	// physical one it counts as a soulshot.
	BlessedSpiritshot
)

// Attack is what a fight is fought with: a plain melee hit when Power is 0
// and Magic is false, else one skill cast over and over.
type Attack struct {
	// Magic marks a magic skill: M.Atk against M.Def, timed by cast speed.
	Magic bool
	// Power is the skill's power.
	Power float64
	// HitTime and Reuse are the skill's base cast time and reuse delay,
	// before the attack or cast speed scales them.
	HitTime, Reuse time.Duration
	// MPCost is the MP one cast takes.
	MPCost float64
	Shot   Shot
}

// never is the time a fight that deals no damage takes.
const never = time.Duration(math.MaxInt64)

// PhysicalDamage is a melee hit's damage (formulas/damage.go
// PhysicalAttackDamage) without the random, position, race and elemental
// factors: pAtk*77/pDef, doubled by a soulshot.
func PhysicalDamage(pAtk, pDef float64, soulshot bool) float64 {
	d := pAtk * 77 / max(pDef, 1)
	if soulshot {
		d *= 2
	}
	return d
}

// PhysicalSkillDamage is a physical skill's damage (PhysicalSkillDamage):
// (pAtk*ss+power)*77/pDef, ss 2.04 with a soulshot.
func PhysicalSkillDamage(pAtk, power, pDef float64, soulshot bool) float64 {
	ss := 1.0
	if soulshot {
		ss = 2.04
	}
	return (pAtk*ss + power) * 77 / max(pDef, 1)
}

// MagicDamage is a magic skill's damage (MagicDamage) when it neither
// fails nor crits: 91*sqrt(mAtk*shot)/mDef*power, shot 2 for a spiritshot
// and 4 for a blessed one.
func MagicDamage(mAtk, mDef, power float64, shot Shot) float64 {
	switch shot {
	case Soulshot:
		mAtk *= 2
	case BlessedSpiritshot:
		mAtk *= 4
	}
	return 91 * math.Sqrt(mAtk) / max(mDef, 1) * power
}

// HitRate is the per mille chance a melee hit lands (formulas/rate.go
// HitRate) on even ground, by day, from the front:
// (90+2*(accuracy-evasion))*10, held to 300..980.
func HitRate(accuracy, evasion float64) float64 {
	return min(max((90+2*(accuracy-evasion))*10, 300), 980)
}

// AttackInterval is the time between two melee swings at attack speed
// pAtkSpd (formulas/speed.go TimeBetweenAttacks): 500000/pAtkSpd ms in
// whole milliseconds, at least 100.
func AttackInterval(pAtkSpd float64) time.Duration {
	ms := max(500000/max(int(pAtkSpd), 1), 100)
	return time.Duration(ms) * time.Millisecond
}

// castPeriod is the time between two casts of a: its cast time (castTime)
// and its reuse scaled by the speed that times it, whichever is longer.
func castPeriod(p Profile, a Attack) time.Duration {
	reuse := int64(float64(a.Reuse.Milliseconds()) * (333 / castSpeed(p, a)))
	return max(castTime(p, a), time.Duration(reuse)*time.Millisecond)
}

// castTime is how long one cast of a takes (cast/controller.go buildPlan:
// t*333/speed, a cast time of 500 ms or more held to 500, a spiritshot
// cutting a magic cast to 70 %).
func castTime(p Profile, a Attack) time.Duration {
	hitMs := a.HitTime.Milliseconds()
	hit := int64(float64(hitMs) * 333 / castSpeed(p, a))
	if a.Magic && a.Shot != NoShot {
		hit = int64(float64(hit) * 0.7)
	}
	if hitMs >= 500 && hit < 500 {
		hit = 500
	}
	return time.Duration(hit) * time.Millisecond
}

// castSpeed is the speed that times a: cast speed for magic, attack speed
// else, at least 1.
func castSpeed(p Profile, a Attack) float64 {
	if a.Magic {
		return max(p.MAtkSpd, 1)
	}
	return max(p.PAtkSpd, 1)
}

// hitDamage is the expected damage of one action of a by p on target: a
// melee hit weighed by its hit rate and critical rate (a critical doubles
// it), a skill's plain damage.
func hitDamage(p Profile, a Attack, target Profile) float64 {
	switch {
	case a.Magic:
		return MagicDamage(p.MAtk, target.MDef, a.Power, a.Shot)
	case a.Power > 0:
		return PhysicalSkillDamage(p.PAtk, a.Power, target.PDef, a.Shot != NoShot)
	}
	d := PhysicalDamage(p.PAtk, target.PDef, a.Shot != NoShot)
	return d * HitRate(p.Accuracy, target.Evasion) / 1000 * (1 + p.Critical/1000)
}

// period is the time between two actions of a by p.
func period(p Profile, a Attack) time.Duration {
	if !a.Magic && a.Power == 0 {
		return AttackInterval(p.PAtkSpd)
	}
	return castPeriod(p, a)
}

// actionsToKill is how many actions of a p needs on average to kill
// target, +Inf when they deal no damage.
func actionsToKill(p Profile, a Attack, target Profile) float64 {
	d := hitDamage(p, a, target)
	if d <= 0 {
		return math.Inf(1)
	}
	return target.MaxHP / d
}

// TimeToKill is how long p takes on average to kill target with a from
// full HP, counting fractional actions; a very long time when a deals no
// damage.
func TimeToKill(p Profile, a Attack, target Profile) time.Duration {
	return scale(period(p, a), actionsToKill(p, a, target))
}

// TimeToDie is how long monster takes on average to kill p from full HP
// with its melee hits.
func TimeToDie(p, monster Profile) time.Duration {
	return TimeToKill(monster, Attack{}, p)
}

// scale is d times n, never when that overflows or n is not finite.
func scale(d time.Duration, n float64) time.Duration {
	v := float64(d) * n
	if math.IsNaN(v) || v >= float64(never) {
		return never
	}
	return time.Duration(v)
}
