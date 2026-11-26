package charmodel

import (
	"math"
	"time"
)

// CastBreakOdds is the percent chance attacker's plain hits break caster's
// magic cast over window, the cast's interrupt window (docs/pvp/layer.md
// "CP, cures, pool, break off", the break-cast gate): one hit every attack
// interval of attacker, each rolling CastBreakChance of its damage, PvP
// bonus included, against caster's MEN (skill/formulas/cast.go
// CastBreakRate, rolled per hit). 0 when no hit falls inside window.
func CastBreakOdds(attacker, caster Fighter, window time.Duration) float64 {
	hits := math.Floor(window.Seconds() / AttackInterval(attacker.Profile.PAtkSpd).Seconds())
	if hits <= 0 {
		return 0
	}
	damage := hitDamage(attacker.Profile, Attack{Shot: attacker.Shot}, caster.Profile) * attacker.mul("pvpPhysDmg")
	miss := 1 - CastBreakChance(damage, caster.MEN)/100
	return 100 * (1 - math.Pow(miss, hits))
}
