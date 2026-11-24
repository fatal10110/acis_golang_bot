// Package charmodel predicts what a character fights with: its final stats
// from its class, level, gear, passive and set skills and dyes, a monster's
// final stats from its template, and from both how long a fight takes and
// what a build is worth to a role.
//
// It copies the server's stat formulas (skill/statbonus, skill/funcs,
// skill/formulas) and knows nothing of the wire or the brain: the brain maps
// what UserInfo reports into a Profile and calibrates the model with it.
package charmodel

import "math"

// maxStat is the size of the server's bonus tables (statbonus.MaxStatValue):
// a base stat indexes them clamped to 0..maxStat-1.
const maxStat = 100

// Attrs are the six base stats.
type Attrs struct {
	STR, CON, DEX, INT, WIT, MEN int
}

// bonus is the server's bonus curve (statbonus.go buildCurve):
// floor(base^(v-offset)*100+0.5)/100, with v clamped to the table
// (statbonus.go ClampIndex).
func bonus(base, offset float64, v int) float64 {
	v = clampStat(v)
	return math.Floor(math.Pow(base, float64(v)-offset)*100+0.5) / 100
}

func clampStat(v int) int {
	return min(max(v, 0), maxStat-1)
}

// STRBonus is the P.Atk multiplier of STR v.
func STRBonus(v int) float64 { return bonus(1.036, 34.845, v) }

// CONBonus is the max HP, HP regen and weight limit multiplier of CON v.
func CONBonus(v int) float64 { return bonus(1.030, 27.632, v) }

// DEXBonus is the attack speed, critical rate and run speed multiplier of
// DEX v.
func DEXBonus(v int) float64 { return bonus(1.009, 19.360, v) }

// INTBonus is the M.Atk multiplier of INT v, which M.Atk takes squared.
func INTBonus(v int) float64 { return bonus(1.020, 31.375, v) }

// WITBonus is the cast speed and magic critical rate multiplier of WIT v.
func WITBonus(v int) float64 { return bonus(1.050, 20.000, v) }

// MENBonus is the M.Def, max MP and MP regen multiplier of MEN v.
func MENBonus(v int) float64 { return bonus(1.010, -0.060, v) }

// EvasionAccuracy is what DEX v adds to both accuracy and evasion
// (statbonus.go BaseEvasionAccuracy): sqrt(v)*6.
func EvasionAccuracy(v int) float64 {
	return math.Sqrt(float64(clampStat(v))) * 6
}

// LevelMod is the level factor P.Atk, P.Def, M.Def and M.Atk (squared)
// scale by: (89+level)/100, level at least 1.
func LevelMod(level int) float64 {
	return (89 + float64(max(level, 1))) / 100
}
