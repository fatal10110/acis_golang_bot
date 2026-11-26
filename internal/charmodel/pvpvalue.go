package charmodel

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// PvPWeight is the pvp preset's weight of PvP value in a gear comparison
// (docs/pvp/model.md "Blend", D37); a persona default (D6), not a safety
// boundary.
const PvPWeight = 0.5

// Share is one row of the mix: the reference opponent of the row and the
// share of the players seen that play it.
type Share struct {
	Opponent Fighter
	Share    float64
}

// PvPValue is the expected share of fights bot wins over mix
// (docs/pvp/model.md "PvP value"): the sum of each row's share times
// win_r, the bot's time to die over its time to die plus its time to kill
// the row's reference opponent (0.5 against an equal).
func PvPValue(bot Fighter, mix []Share, rows RowTable) float64 {
	v := 0.0
	for _, s := range mix {
		v += s.Share * winShare(bot, s.Opponent, rows)
	}
	return v
}

// winShare is win_r of bot against opp: 0.5 when neither can kill the
// other.
func winShare(bot, opp Fighter, rows RowTable) float64 {
	kill, die := fight(bot, opp, rows)
	switch {
	case math.IsInf(kill, 1) && math.IsInf(die, 1):
		return 0.5
	case math.IsInf(die, 1):
		return 1
	case math.IsInf(kill, 1):
		return 0
	}
	return die / (die + kill)
}

// Engage is the engage estimate (docs/pvp/model.md "Engage estimate"): bot
// takes on opp while its time to die is at least margin (the bot row's
// spot margin: tank 1.5, normal 2.0, glass cannon 3.0) times its time to
// kill. Never against an opponent it cannot hurt.
func Engage(bot, opp Fighter, rows RowTable, margin float64) bool {
	kill, die := fight(bot, opp, rows)
	return !math.IsInf(kill, 1) && die >= margin*kill
}

// FightTimes are how long bot takes on average to kill opp from full CP
// and HP, and opp to kill bot, each side playing its row's steps under
// their Vs gates and plain attacks; a very long time for a side that deals
// no damage.
func FightTimes(bot, opp Fighter, rows RowTable) (kill, die time.Duration) {
	k, d := fight(bot, opp, rows)
	return seconds(k), seconds(d)
}

// fight is FightTimes in seconds, +Inf for a side that deals no damage.
// Each side's damage is cut by the time share the other side's control
// holds it (u_r, docs/pvp/model.md "PvP value"); the rule is applied to
// both sides so an equal opponent is an even fight.
func fight(bot, opp Fighter, rows RowTable) (kill, die float64) {
	b, o := play(bot, opp, rows), play(opp, bot, rows)
	return lasts(opp, b.dps*(1-o.hold)), lasts(bot, o.dps*(1-b.hold))
}

// lasts is how long f's CP and HP last against dps, +Inf when it is none.
func lasts(f Fighter, dps float64) float64 {
	if dps <= 0 {
		return math.Inf(1)
	}
	return (f.MaxCP + f.Profile.MaxHP) / dps
}

func seconds(s float64) time.Duration {
	return scale(time.Second, s)
}

// pressure is what one side's play does to the other.
type pressure struct {
	// dps is its damage a second while it is free to act.
	dps float64
	// hold is the time share its control holds the other side, 0..1.
	hold float64
}

// play is f's pressure on target: its row's steps it casts against
// target's archetype, each at its period (cast time or scaled reuse,
// whichever is longer), and plain attacks for the time the casts leave.
// A damage step deals its skill damage, a DamOverTime effect its ticks
// while it lasts, a control effect holds target for its duration (a sleep
// only until f's next hit wakes it); each effect weighed by its land rate. MP is not modeled: a fight is short.
func play(f, target Fighter, rows RowTable) pressure {
	var busy, damage, hold float64
	for _, step := range rows.steps(f, target.Row.Archetype()) {
		s := step.Skill
		a := Attack{Magic: s.Magic, Power: s.Power, HitTime: s.HitTime, Reuse: s.ReuseDelay, Shot: f.Shot}
		period := castPeriod(f.Profile, a).Seconds()
		if period <= 0 {
			period = AttackInterval(f.Profile.PAtkSpd).Seconds()
		}
		busy += castTime(f.Profile, a).Seconds() / period
		if damageTypes[typeKey(s.Type)] && s.Power > 0 {
			damage += skillDamage(f, s.Magic, s.Power, target) / period
		}
		for _, e := range s.Effects {
			land := effectLand(s, e, f, target) / 100
			ticks := max(e.Count, 1)
			lasting := float64(e.Time * ticks)
			switch {
			case e.Name == "DamOverTime":
				damage += land * e.Value * float64(ticks) / max(period, lasting)
			case controlEffects[e.Name]:
				if e.Name == "Sleep" {
					// Any hit wakes a sleeping player
					// (character_stats.go
					// applyNonConsumptionDamageEffects), and the
					// holder keeps attacking: it holds until the
					// holder's next plain hit.
					lasting = min(lasting, AttackInterval(f.Profile.PAtkSpd).Seconds())
				}
				hold += land * min(lasting, period) / period
			}
		}
	}
	if busy > 1 {
		// Casts back to back: the rotation stretches to fit.
		damage, hold, busy = damage/busy, hold/busy, 1
	}
	plain := hitDamage(f.Profile, Attack{Shot: f.Shot}, target.Profile) * f.mul("pvpPhysDmg") /
		AttackInterval(f.Profile.PAtkSpd).Seconds()
	return pressure{dps: damage + plain*(1-busy), hold: min(hold, 1)}
}

// damageTypes are the skill types whose power is damage.
var damageTypes = map[string]bool{
	"PDAM": true, "MDAM": true, "BLOW": true, "DRAIN": true, "CHARGEDAM": true, "FATAL": true,
}

// controlEffects are the effects that stop their target acting.
var controlEffects = map[string]bool{
	"Stun": true, "Sleep": true, "Root": true, "Paralyze": true, "Fear": true, "Petrification": true,
}

// SkillDamage is one cast's damage of skill s by f on target, its PvP
// bonus included (skillDamage): its power for a damage skill type, 0 for
// any other skill.
func SkillDamage(f Fighter, s datapack.Skill, target Fighter) float64 {
	if !damageTypes[typeKey(s.Type)] || s.Power <= 0 {
		return 0
	}
	return skillDamage(f, s.Magic, s.Power, target)
}

// skillDamage is one cast's damage of a skill of power by f on target with
// its PvP bonus (formula.go: pvpMagicalDmg, pvpPhysSkillsDmg).
func skillDamage(f Fighter, magic bool, power float64, target Fighter) float64 {
	if magic {
		return MagicDamage(f.Profile.MAtk, target.Profile.MDef, power, f.Shot) * f.mul("pvpMagicalDmg")
	}
	return PhysicalSkillDamage(f.Profile.PAtk, power, target.Profile.PDef, f.Shot != NoShot) * f.mul("pvpPhysSkillsDmg")
}

// Blend is a gear candidate's score for a persona that weighs PvP by w
// (docs/pvp/model.md "Blend"): value x ((1 - w) x deltaValue / value + w x
// deltaPvP / pvpValue), value and pvpValue the current loadout's, the
// deltas the candidate's change; in Value's unit. w 0 is exactly
// deltaValue.
func Blend(value, deltaValue, pvpValue, deltaPvP, w float64) float64 {
	d := (1 - w) * deltaValue
	if w != 0 && pvpValue > 0 {
		d += w * value * deltaPvP / pvpValue
	}
	return d
}
