package botclient

import (
	"math"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// statBonus is the server's attribute bonus curve (skill/statbonus,
// buildCurve): floor(base^(v-offset)*100+0.5)/100.
func statBonus(base, offset float64, v int) float64 {
	return math.Floor(math.Pow(base, float64(v)-offset)*100+0.5) / 100
}

// The server's STR, CON and DEX curves (statbonus.go strCurve, conCurve,
// dexCurve).
func strBonus(v int) float64 { return statBonus(1.036, 34.845, v) }
func conBonus(v int) float64 { return statBonus(1.030, 27.632, v) }
func dexBonus(v int) float64 { return statBonus(1.009, 19.360, v) }

// hitRate is the chance a swing of accuracy acc lands on evasion eva, from
// the front by day on level ground (formulas/rate.go HitRate): (90 + 2 *
// (acc - eva)) percent, within 30 and 98.
func hitRate(acc, eva float64) float64 {
	return min(980, max(300, (90+2*(acc-eva))*10)) / 1000
}

// mobFight is what fighting one m alone costs the bot of self: how long it
// takes to kill (seconds) and the damage per second m deals it meanwhile.
// It finalizes m's stats as the server's hostile stat pipeline does
// (npc/hostile_stats.go defaultBuiltin, skill/funcs): max HP by CON, P.Atk
// by STR and level, P.Def by level, attack speed and critical rate by DEX,
// accuracy and evasion from DEX and level. A melee hit deals
// P.Atk*77/P.Def, twice that on a critical hit (formulas/damage.go
// PhysicalAttackDamage, random spread averaging out), one swing every
// 500000/attack speed ms (formulas/speed.go TimeBetweenAttacks). The bot's
// own critical hits and shots are left out: the cost errs high. The bot's
// P.Atk, P.Def and attack speed and m's HP and P.Def are above 0.
func mobFight(self *SelfState, m *datapack.Mob) (kill, dps float64) {
	c := &m.Combat
	level := float64(max(m.Level, 1))
	mod := (89 + level) / 100
	hp := c.HP * conBonus(c.CON)
	pAtk, pDef := c.PAtk*strBonus(c.STR)*mod, c.PDef*mod
	atkSpd := math.Trunc(c.AtkSpd * dexBonus(c.DEX))
	crit := min(500, c.Crit*dexBonus(c.DEX)*10) / 1000
	acc := math.Trunc(math.Sqrt(float64(c.DEX))*6 + level) // HitRate takes ints
	dps = pAtk * 77 / float64(self.PDef) * (1 + crit) * hitRate(acc, float64(self.Evasion)) * atkSpd / 500
	hit := float64(self.PAtk) * 77 / pDef * hitRate(float64(self.Accuracy), acc)
	return hp / (hit * float64(self.PAtkSpd) / 500), dps
}

// potionHeal is what one healing potion restores: its HealOverTime's val
// a tick, Count ticks Time seconds apart.
type potionHeal struct {
	// total is the HP the potion restores in all, rate the HP a second it
	// restores while it lasts: a potion drunk while the last one ticks
	// replaces it (one stack type), so the bot heals no faster.
	total, rate float64
}

// indexPotionHeals indexes what each healing potion of a datapack restores
// (the potions indexPotions lists).
func indexPotionHeals(etc map[int32]datapack.EtcItem, skills map[datapack.SkillRef]datapack.Skill) map[int32]potionHeal {
	out := make(map[int32]potionHeal)
	for id, it := range etc {
		if it.Kind != datapack.EtcPotion || len(it.Skills) == 0 {
			continue
		}
		for _, e := range skills[it.Skills[0]].Effects {
			if e.Name == "HealOverTime" && e.StackType == hpRecover && e.Time > 0 {
				out[id] = potionHeal{total: e.Value * float64(e.Count), rate: e.Value / float64(e.Time)}
				break
			}
		}
	}
	return out
}

// packRisk is how much of what the bot can take s's pack deals it while it
// kills one of them (SpotPolicy.PackRisk): the spot's expected pack
// (spotPack, at least one) times the damage one of its aggressive monsters
// deals it while the bot kills it (mobFight, by their spawn totals), over
// its max HP plus what the healing potions it carries restore in that
// time. 0 when s has no aggressive monster with HP and P.Def, the bot's
// stats are not in yet, or it is a mystic, whose spells kill rather than
// its melee.
func (b *Brain) packRisk(s *datapack.Spot) float64 {
	self := &b.world.Self
	if self.PAtk <= 0 || self.PDef <= 0 || self.PAtkSpd <= 0 || self.MaxHP <= 0 || slices.Contains(mysticClasses, self.ClassID) {
		return 0
	}
	var n, kill, damage float64
	for i := range s.Mobs {
		m := &s.Mobs[i]
		if m.Combat.HP <= 0 || m.Combat.PDef <= 0 {
			continue
		}
		k, dps := mobFight(self, m)
		c := float64(m.Count)
		n, kill, damage = n+c, kill+c*k, damage+c*k*dps
	}
	if n == 0 {
		return 0
	}
	kill, damage = kill/n, damage/n
	return max(1, spotPack(s)) * damage / (float64(self.MaxHP) + b.potionsHeal(kill))
}

// potionsHeal is what the healing potions the bot carries (Persona.Potions)
// restore in d seconds: all of them, at the rate of the best.
func (b *Brain) potionsHeal(d float64) float64 {
	m := b.cfg.Travel
	if m == nil {
		return 0
	}
	var total, rate float64
	for _, id := range b.persona.Potions {
		h, ok := m.heals[id]
		if n := b.carried(id); ok && n > 0 {
			total += float64(n) * h.total
			rate = max(rate, h.rate)
		}
	}
	return min(total, rate*d)
}
