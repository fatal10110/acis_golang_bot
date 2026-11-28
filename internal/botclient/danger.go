package botclient

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The spot margins of the rows (docs/strategy.md, "Numbers and summary",
// column 10): how many times as long as a kill a row must last against a
// spot's likely pack (ClassKit.SpotMargin).
const (
	marginTank        = 1.5
	marginNormal      = 2.0
	marginGlassCannon = 3.0
)

// damageSkillTypes are the skill types whose power is damage to the target:
// the rotation steps the spot fit may kill with (spotAttack).
var damageSkillTypes = []string{"PDAM", "BLOW", "MDAM", "DRAIN", "DEATHLINK"}

// sustainStep reports whether st is gated on the bot's own HP
// (SelfHPBelow): a drain it casts only when hurt, not what it kills with,
// so the attack every model times (killAttack) passes over it: the
// sorcerer kills with Prominence, not Vampiric Touch (12 s reuse, under
// 60 % HP), PR with Stunning Shot, not Drain Health (#676, #712).
func sustainStep(st Step) bool {
	return st.SelfHPBelow > 0
}

// killAttack is skill sk of rotation step st as the attack the bot kills
// with (charmodel.Attack), each cast costing its mpInitialConsume and
// mpConsume as the server charges them (cast/controller.go CanCast): the
// one rule the solo spot fit (spotAttack), the party fit (memberModel) and
// the gear model (gearRole) take the first such step of a rotation by. Not
// ok for a sustain step (sustainStep) or a skill that deals its target no
// damage (damageSkillTypes, power above 0).
func killAttack(st Step, sk datapack.Skill) (charmodel.Attack, bool) {
	if sustainStep(st) || sk.Power <= 0 || !slices.Contains(damageSkillTypes, sk.Type) {
		return charmodel.Attack{}, false
	}
	return charmodel.Attack{Magic: slices.Contains(magicSkillTypes, sk.Type), Power: sk.Power, HitTime: sk.HitTime, Reuse: sk.ReuseDelay, MPCost: float64(sk.MPInitialConsume + sk.MPConsume)}, true
}

// packFits reports whether s fits the bot's stats by the model
// (docs/strategy.md, Design section 11): it lasts against the spot's likely
// pack at least its margin times as long as it takes to kill one of the
// spot's monsters (packFight). With no margin (a row without one, or
// SpotPolicy.Margin below 0) or nothing to judge
// by (packFight not ok) only the level and a death's teaching bound a spot.
// A leader fits it with the bot members present at now (partyCrew,
// docs/parties/model.md), by its own row's margin; a summoner by its
// servitor's while it has one out or can summon one (spotMargin).
func (b *Brain) packFits(s *datapack.Spot, now time.Time) bool {
	bd := b.build()
	margin := b.spotMargin(&bd)
	if margin <= 0 {
		return true
	}
	c := b.partyCrew(now)
	kill, die, ok := b.packFight(s, &bd, margin, &c)
	return !ok || die >= margin*kill
}

// packFight is how long, in seconds, the bot takes to kill one of s's monsters and how
// long it lasts against its likely pack, both averaged over the monsters by
// their spawn totals: charmodel.TimeToKill with its melee or the first
// rotation step it knows that it kills with, whichever is faster (spotAttack),
// and charmodel.TimeToDie against each monster's final stats
// (charmodel.Monster), its own stats as its UserInfo reports them, with the
// HP its healing potions restore over margin kills added (potionsHeal). The
// pack is fought one monster after another, so the potions spread over its
// spotPack fights. Not ok when the bot's stats are not in yet, s has no
// monster with HP and P.Def, or the bot casts on its target what it has no
// travel map to price (spotAttack). Regeneration, shots and its own buffs beyond
// what UserInfo carries are left out. A leader's crew c kills each monster
// with it (crew.kill) and heals it over margin kills as its potions do
// (crew.healed); the whole pack still hits the leader. The bot's servitor,
// out or one it can summon (servitorModel), kills each monster with it with
// its melee (charmodel.TimeWithServitor); the pack is still all on the bot
// (docs/servitors.md section 3, "Pack threat"): it gives itself no credit
// for the hate the servitor holds.
func (b *Brain) packFight(s *datapack.Spot, bd *Build, margin float64, c *crew) (kill, die float64, ok bool) {
	self := b.reported()
	if self.PAtk <= 0 || self.PDef <= 0 || self.PAtkSpd <= 0 || self.MaxHP <= 0 {
		return 0, 0, false
	}
	attack, skill, priced := b.spotAttack(bd)
	if !priced {
		return 0, 0, false
	}
	sv, withServitor := b.servitorModel()
	var n, k, dps float64
	for i := range s.Mobs {
		m := &s.Mobs[i]
		if m.Count <= 0 || m.Combat.HP <= 0 || m.Combat.PDef <= 0 {
			continue
		}
		mob := charmodel.Monster(datapack.NPC{Level: m.Level, Combat: m.Combat})
		t := charmodel.TimeToKill(self, charmodel.Attack{}, mob)
		if skill {
			t = min(t, charmodel.TimeToKill(self, attack, mob))
		}
		if withServitor {
			t = charmodel.TimeWithServitor(t, sv, mob)
		}
		t = c.kill(t, mob)
		cnt := float64(m.Count)
		n, k = n+cnt, k+cnt*t.Seconds()
		dps += cnt * self.MaxHP / max(charmodel.TimeToDie(self, mob).Seconds(), 1e-3)
	}
	if n == 0 {
		return 0, 0, false
	}
	k, dps = k/n, dps/n
	fights := max(1, spotPack(s))
	hp := self.MaxHP + b.potionsHeal(margin*k, fights) + c.healed(margin*k, fights)
	return k, hp / dps, true
}

// spotAttack is the first step of bd's rotation the bot knows and casts
// on its target that it kills with (killAttack: not a sustain step); not
// ok without one. Its weapon gate is left out: the fit takes the faster of
// it and the melee. priced is false when the bot knows a step of its
// rotation it casts on its target, a sustain step aside, but has no travel
// map to read the skill from: its melee is no measure of a caster's kills,
// so the fit falls back to the level (a brain given spots without a travel
// map, as TestLiveFarmMage's). A bot that knows only sustain steps melees
// until hurt, so its melee is priced.
func (b *Brain) spotAttack(bd *Build) (a charmodel.Attack, ok, priced bool) {
	m := b.cfg.Travel
	for _, st := range bd.Kit.Rotation {
		k, known := b.world.Skills[st.Skill]
		if st.Target != TargetEnemy || sustainStep(st) || !known || k.Disabled {
			continue
		}
		if m == nil {
			return charmodel.Attack{}, false, false
		}
		if sk, found := m.skills[datapack.SkillRef{ID: st.Skill, Level: k.Level}]; found {
			if a, ok := killAttack(st, sk); ok {
				return a, true, true
			}
		}
	}
	return charmodel.Attack{}, false, true
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

// potionsHeal is what the healing potions the bot carries (Persona.Potions)
// restore in d seconds of one of fights fights: their total shared over
// the fights, at the rate of the best.
func (b *Brain) potionsHeal(d, fights float64) float64 {
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
	return min(total/fights, rate*d)
}
