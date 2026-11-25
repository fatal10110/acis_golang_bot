package botclient

import (
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// A leader's spot fit with its bot members (docs/parties/model.md): a pack
// monster falls to the summed damage rate of the leader and every Damage
// member, and the leader, on whom the whole pack hits, lasts longer by what
// its healers restore while their MP pays. The margin stays the leader's
// row's; the spot claim's weight does not change (spotWeight).

// crew is what the bot members present add to the leader's fight: zero
// while it leads none (packFight is then the bot's own).
type crew struct {
	damage []memberModel
	heals  []crewHeal
}

// crewHeal is a present healer: its model and the MP it has left.
type crewHeal struct {
	m  memberModel
	mp float64
}

// memberKey is a bot member's class and level, what its model derives from.
type memberKey struct{ class, level int32 }

// memberModel is what a bot member of a class and level brings to a
// fight, uncalibrated: its class template's profile at its level, wearing
// nothing the model knows of (charmodel.Model), and either the heals of its
// class's Support build or the attack of its Damage build, each skill at
// the highest level its class's tree grants by its level.
type memberModel struct {
	p charmodel.Profile
	// heals are the heals it casts on the leader; a member with one is a
	// healer and adds no damage.
	heals []charmodel.Heal
	// attack is the first damage step of its Damage rotation, skill false
	// with none: it then fights in melee alone.
	attack charmodel.Attack
	skill  bool
}

// partyCrew is the bot members present in the party the bot leads: the
// members its lead slot names, alive, and in sight or last reported within
// LeadPolicy.FarAway of it (one away on a trip, or not reported yet, drops
// out until it is back). A player member adds nothing, nor a member whose
// class the travel map has no template for.
func (b *Brain) partyCrew(now time.Time) crew {
	var c crew
	m := b.cfg.Travel
	if m == nil || !b.leadsBotParty() {
		return c
	}
	slot, from := b.cfg.link.party, b.world.Self.Position(now)
	far := int64(b.persona.Lead.withDefaults().FarAway)
	ids := make([]int32, 0, len(b.world.Party.Members))
	for id := range b.world.Party.Members {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		pm := b.world.Party.Members[id]
		if !slotNames(slot, pm.Name) || pm.HP <= 0 {
			continue
		}
		if o := b.world.Objects[id]; o != nil {
			if o.Dead {
				continue
			}
		} else if pm.PosAt.IsZero() || dist2(from, pm.Pos) > far*far {
			continue
		}
		mm, ok := b.memberModel(memberKey{pm.ClassID, pm.Level})
		switch {
		case !ok:
		case len(mm.heals) > 0:
			c.heals = append(c.heals, crewHeal{m: mm, mp: float64(pm.MP)})
		default:
			c.damage = append(c.damage, mm)
		}
	}
	return c
}

// memberModel is the model of a bot member of k's class and level, kept
// for the brain's life (the travel map is fixed); not ok without its class
// template.
func (b *Brain) memberModel(k memberKey) (memberModel, bool) {
	if mm, ok := b.members[k]; ok {
		return mm, true
	}
	m := b.cfg.Travel
	cl, ok := m.classes[k.class]
	if !ok {
		return memberModel{}, false
	}
	race, _ := datapack.ClassRace(k.class)
	level := int(k.level)
	mm := memberModel{p: charmodel.Model(charmodel.Build{Class: cl, Level: level, Mage: charmodel.MageClass(m.classes, k.class)})}
	learned := func(id int32) (datapack.Skill, bool) {
		lv := grantedLevel(cl, id, level)
		if lv == 0 {
			return datapack.Skill{}, false
		}
		sk, ok := m.skills[datapack.SkillRef{ID: id, Level: int32(lv)}]
		return sk, ok
	}
	for _, h := range CompileBuild(race, k.class, k.level, RoleSupport, BranchChoice{}).Kit.Heals {
		if sk, ok := learned(h.Skill); ok {
			if heal, ok := healOf(sk); ok {
				mm.heals = append(mm.heals, heal)
			}
		}
	}
	if len(mm.heals) == 0 {
		for _, st := range CompileBuild(race, k.class, k.level, RoleDamage, BranchChoice{}).Kit.Rotation {
			if st.Target != TargetEnemy {
				continue
			}
			if sk, ok := learned(st.Skill); ok && sk.Power > 0 && slices.Contains(damageSkillTypes, sk.Type) {
				mm.attack = charmodel.Attack{Magic: slices.Contains(magicSkillTypes, sk.Type), Power: sk.Power, HitTime: sk.HitTime, Reuse: sk.ReuseDelay, MPCost: float64(sk.MPInitialConsume + sk.MPConsume)}
				mm.skill = true
				break
			}
		}
	}
	if b.members == nil {
		b.members = make(map[memberKey]memberModel)
	}
	b.members[k] = mm
	return mm, true
}

// grantedLevel is the highest level of skill id cl's tree grants by
// character level, 0 for none.
func grantedLevel(cl datapack.Class, id int32, level int) int {
	lv := 0
	for _, g := range cl.Skills {
		if g.ID == id && g.MinLevel <= level {
			lv = max(lv, g.Level)
		}
	}
	return lv
}

// healOf is sk as a heal on the leader: a HEAL its power plus
// sqrt(M. Atk), a HEAL_STATIC its power, a HOT (1229 Chant of Life) its
// HealOverTime ticks' total, each cast costing its mpInitialConsume and
// mpConsume as the server charges them (cast/controller.go CanCast); not ok
// for any other skill or one that costs no MP.
func healOf(sk datapack.Skill) (charmodel.Heal, bool) {
	h := charmodel.Heal{HitTime: sk.HitTime, Reuse: sk.ReuseDelay, MPCost: float64(sk.MPInitialConsume + sk.MPConsume)}
	switch sk.Type {
	case "HEAL":
		h.Power, h.MAtk = sk.Power, true
	case "HEAL_STATIC":
		h.Power = sk.Power
	case "HOT":
		for _, e := range sk.Effects {
			if e.Name == "HealOverTime" {
				h.Power += e.Value * float64(e.Count)
			}
		}
	}
	return h, h.Power > 0 && h.MPCost > 0
}

// kill is how long the leader, killing mob alone in t, and the crew's
// Damage members take to kill it together: each member's melee or the
// first damage step of its rotation, whichever is faster.
func (c *crew) kill(t time.Duration, mob charmodel.Profile) time.Duration {
	if len(c.damage) == 0 {
		return t
	}
	ts := []time.Duration{t}
	for _, mm := range c.damage {
		mt := charmodel.TimeToKill(mm.p, charmodel.Attack{}, mob)
		if mm.skill {
			mt = min(mt, charmodel.TimeToKill(mm.p, mm.attack, mob))
		}
		ts = append(ts, mt)
	}
	return charmodel.TimeTogether(ts...)
}

// healed is the HP the crew's healers restore on the leader in d seconds
// of one of fights fights: each its heal that restores most, one cast a
// HealPeriod, while the casts its MP pays for, shared over the fights,
// last. A healer with no MP left adds nothing.
func (c *crew) healed(d, fights float64) float64 {
	var hp float64
	for _, h := range c.heals {
		var best float64
		for _, heal := range h.m.heals {
			period := charmodel.HealPeriod(h.m.p, heal).Seconds()
			casts := math.Floor(h.mp/heal.MPCost) / fights
			if period > 0 {
				casts = min(casts, d/period)
			}
			best = max(best, casts*charmodel.HealAmount(h.m.p, heal))
		}
		hp += best
	}
	return hp
}
