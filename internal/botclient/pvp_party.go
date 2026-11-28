package botclient

import (
	"maps"
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
)

// Party PvP (docs/pvp/party.md, X9): in any party, whoever leads it, the
// bot fights the party's enemy set, the enemy healer first, peels for its
// healer and leader, and its healer buffs and stands behind the leader.
// Everything here is the brain goroutine's, as the PvP world model is; every
// cast and walk goes through the one cast path (castOn, castStep, the side
// cast) and the walker, under the frame bucket, so X9 adds no sender.

// Party PvP figures (docs/pvp/party.md).
const (
	// enemyHealerFor is how long a player seen healing an enemy stays the
	// enemy healer.
	enemyHealerFor = 60 * time.Second
	// pullHP is the HP percent under which a member pulls the party onto
	// its attacker.
	pullHP = 30
	// healerReach is how far from the bot an enemy healer is still
	// targeted: as far as the party's buffs reach, the party's fight.
	healerReach = partyBuffReach
	// behindDist is how far behind the leader, away from the enemies, the
	// bot's healer stands; behindSlack how far from that point it lets
	// itself be before it walks there again.
	behindDist  = 200
	behindSlack = 100
)

// enemyHeals are the skill types of a cast that marks the enemy healer.
var enemyHeals = map[string]bool{"HEAL": true, "HOT": true, "HEAL_PERCENT": true, "BALANCE_LIFE": true}

// partyHit is a creature's last hit or hostile cast on the party: on whom
// (a member, or the bot itself) and when.
type partyHit struct {
	victim int32
	at     time.Time
}

// inParty reports whether id is the bot or one of its party members.
func (b *Brain) inParty(id int32) bool { return id == b.world.Self.ObjectID || b.partyMate(id) }

// partyFight reports whether the bot fights as a party at now: in a party
// and out of a duel (a duel fights only its other side, docs/pvp/guards.md;
// a party duel's members each fight it by ruleDuel, pvp_party_duel.go).
func (b *Brain) partyFight(now time.Time) bool { return b.world.Party.In() && !b.inDuel(now) }

// partyAttack keeps the party's hits from e at now, and reports when the
// PvP reaction to it is due: a player's or summon's hit on a party member
// (one on the bot wakes it as before, salient).
func (b *Brain) partyAttack(e Attack, now time.Time) time.Time {
	due := false
	for _, h := range e.Hits {
		due = b.partyStrike(e.AttackerID, h.TargetID, now) || due
	}
	if due {
		return b.pvpReact(now)
	}
	return time.Time{}
}

// partyStrike records attacker's hit or offensive cast on target at now: by
// the bot or a member, target was struck by the party (the sleep rule's "no
// member hits"); by a player or summon outside the party on the bot or a
// member, it is a party hit. It reports whether it was a party hit on a
// member other than the bot.
func (b *Brain) partyStrike(attacker, target int32, now time.Time) bool {
	p := &b.pvp
	if b.inParty(attacker) {
		if p.struck == nil {
			p.struck = make(map[int32]time.Time)
		}
		maps.DeleteFunc(p.struck, func(_ int32, at time.Time) bool { return now.Sub(at) > attackerWindow })
		p.struck[target] = now
		return false
	}
	o := b.world.object(attacker)
	if o == nil || !b.inParty(target) || o.Kind != KindPlayer && !o.Summon {
		return false
	}
	if p.hits == nil {
		p.hits = make(map[int32]partyHit)
	}
	maps.DeleteFunc(p.hits, func(_ int32, h partyHit) bool { return now.Sub(h.at) > attackerWindow })
	p.hits[attacker] = partyHit{victim: target, at: now}
	return target != b.world.Self.ObjectID
}

// struck reports whether the bot or a member hit id within attackerWindow
// of now.
func (b *Brain) struck(id int32, now time.Time) bool {
	at, ok := b.pvp.struck[id]
	return ok && now.Sub(at) <= attackerWindow
}

// healSeen marks caster, a player outside the party, as the enemy healer
// at now when its cast of skill at level is a heal (enemyHeals) landing on
// one of targets, another player of the party's enemy set, and reports
// whether it did. A target ONE heal names its target in MagicSkillUse; a
// target PARTY heal (1027 Group Heal, 1335 Balance Life) names the caster
// there and its party only in MagicSkillLaunched's target list.
func (b *Brain) healSeen(caster, skill, level int32, targets []int32, now time.Time) bool {
	r := b.pvp.opponents[caster]
	if r == nil || b.inParty(caster) {
		return false
	}
	s, ok := b.skillAt(skill, level)
	if !ok || !enemyHeals[s.Type] {
		return false
	}
	enemies := b.partyEnemies(now)
	for _, id := range targets {
		if id != caster && slices.Contains(enemies, id) {
			r.HealedAt = now
			return true
		}
	}
	return false
}

// launchSeen marks the enemy healer from a player's cast landing on e's
// targets at now (healSeen), and reports when the PvP reaction to it is
// due.
func (b *Brain) launchSeen(e MagicSkillLaunched, now time.Time) time.Time {
	o := b.world.object(e.CasterID)
	if o == nil || o.Kind != KindPlayer || !b.healSeen(e.CasterID, e.SkillID, e.Level, e.Targets, now) {
		return time.Time{}
	}
	return b.pvpReact(now)
}

// hitters are the party's hits within attackerWindow of now by attacker, a
// summon's credited to its owner when in sight and fightable (summonOwner),
// the latest per attacker.
func (b *Brain) hitters(now time.Time) map[int32]partyHit {
	out := make(map[int32]partyHit)
	for id, h := range b.pvp.hits {
		if now.Sub(h.at) > attackerWindow {
			continue
		}
		if o := b.world.object(id); o != nil && o.Summon {
			if id = b.summonOwner(id, now); id == 0 {
				continue
			}
		}
		if old, ok := out[id]; !ok || h.at.After(old.at) {
			out[id] = h
		}
	}
	return out
}

// partyEnemies are the party's enemy set at now by object id
// (docs/pvp/party.md "Enemies"): the players the bot may fight that hit it
// or a member within attackerWindow (a summon's owner in sight for the
// summon), that healed one of those within enemyHealerFor, and the leader's
// target. None outside a party fight.
func (b *Brain) partyEnemies(now time.Time) []int32 {
	if !b.partyFight(now) {
		return nil
	}
	var ids []int32
	add := func(id int32) {
		if o := b.world.object(id); o != nil && o.Kind == KindPlayer && !slices.Contains(ids, id) && b.mayFight(id, now) {
			ids = append(ids, id)
		}
	}
	for id := range b.hitters(now) {
		add(id)
	}
	for id, r := range b.pvp.opponents {
		if !r.HealedAt.IsZero() && now.Sub(r.HealedAt) <= enemyHealerFor {
			add(id)
		}
	}
	if t := b.assistTarget(now); t != 0 {
		add(t)
	}
	slices.Sort(ids)
	return ids
}

// enemyHealer is the enemy healer among enemies at now: the one seen
// healing an enemy last within enemyHealerFor, else the first of the
// Healer archetype (the prior), else 0.
func (b *Brain) enemyHealer(enemies []int32, now time.Time) int32 {
	var id, prior int32
	var last time.Time
	for _, e := range enemies {
		r := b.pvp.opponents[e]
		switch {
		case r == nil:
		case !r.HealedAt.IsZero() && now.Sub(r.HealedAt) <= enemyHealerFor && r.HealedAt.After(last):
			id, last = e, r.HealedAt
		case prior == 0 && r.Archetype == charmodel.Healer:
			prior = e
		}
	}
	if id != 0 {
		return id
	}
	return prior
}

// partyHealer reports whether member id plays a Healer row (CL, EO, SO,
// BI, PP, EE, SE) by its party window class.
func (b *Brain) partyHealer(id int32) bool {
	m := b.world.Party.Members[id]
	if m == nil {
		return false
	}
	row, ok := charmodel.ClassRow(m.ClassID, false)
	return ok && row.Archetype() == charmodel.Healer
}

// selfHealer reports whether the bot is its party's healer: it plays a
// Healer row and its kit heals.
func (b *Brain) selfHealer() bool {
	row, ok := charmodel.ClassRow(b.world.Self.ClassID, false)
	return ok && row.Archetype() == charmodel.Healer && len(b.build().Kit.Heals) > 0
}

// peels reports whether a Peel step goes on player id at now
// (docs/pvp/party.md "Peel"): in a party fight, id's last hit within
// attackerWindow was on the party's leader (or the one the bot assists) or
// on a member playing a Healer row.
func (b *Brain) peels(id int32, now time.Time) bool {
	if !b.partyFight(now) {
		return false
	}
	h, ok := b.hitters(now)[id]
	if !ok || h.victim == b.world.Self.ObjectID {
		return false
	}
	return h.victim == b.world.Party.LeaderID || h.victim == b.assisted() || b.partyHealer(h.victim)
}

// peelReady reports whether a Peel step of the bot's PvP steps is ready at
// now.
func (b *Brain) peelReady(now time.Time) bool {
	kit := b.build().Kit
	return slices.ContainsFunc(pvpSteps(kit.PvP, kit.Rotation), func(s Step) bool {
		return s.Peel && b.castReady(s.Skill, now)
	})
}

// partyFoe is the enemy the bot fights in a party fight at now, 0 for none
// (docs/pvp/party.md "The healer first"): with a Peel step ready the enemy
// hitting its healer or leader; else the enemy healer within healerReach;
// else the latest attacker of a member (or the bot) under pullHP; else the
// leader's target; else the latest attacker of the party.
func (b *Brain) partyFoe(now time.Time) int32 {
	enemies := b.partyEnemies(now)
	if len(enemies) == 0 {
		return 0
	}
	if b.peelReady(now) {
		if i := slices.IndexFunc(enemies, func(id int32) bool { return b.peels(id, now) }); i >= 0 {
			return enemies[i]
		}
	}
	if h := b.enemyHealer(enemies, now); h != 0 && b.inReach(h, healerReach, now) {
		return h
	}
	hits := b.hitters(now)
	var pull, last int32
	var pullAt, lastAt time.Time
	for _, id := range enemies {
		h, ok := hits[id]
		if !ok {
			continue
		}
		if hp, maxHP := b.vitals(h.victim); hp > 0 && below(hp, maxHP, pullHP) && h.at.After(pullAt) {
			pull, pullAt = id, h.at
		}
		if h.at.After(lastAt) {
			last, lastAt = id, h.at
		}
	}
	if pull != 0 {
		return pull
	}
	if t := b.assistTarget(now); t != 0 && slices.Contains(enemies, t) {
		return t
	}
	return last
}

// rulePartyFight fights the party's enemy (partyFoe) in a party fight,
// standing up first and waiting out a cast in flight before it turns to
// another target. The bot's healer does not: it stands behind the leader
// (healerStand).
func (b *Brain) rulePartyFight(now time.Time) (Action, bool) {
	if b.selfHealer() {
		return b.healerStand(now)
	}
	id := b.partyFoe(now)
	if id == 0 {
		return Action{}, false
	}
	s := &b.world.Self
	switch {
	case s.Sitting:
		return b.toggle(now), true
	case (s.Casting(now) || b.castPending(now)) && b.world.Target != id:
		return Action{}, true
	}
	return b.fight(id, now), true
}

// healerStand is the bot's healer's place in a party fight at now
// (docs/pvp/party.md "The bot's healer"): behindDist behind the leader, away
// from the middle of the enemies in sight, walked to while farther than
// behindSlack from it, and held there instead of fighting. ok is false with
// no enemy, no leader in sight, or an attacker on the bot itself, which it
// fights back (ruleAttacked).
func (b *Brain) healerStand(now time.Time) (Action, bool) {
	enemies := b.partyEnemies(now)
	lead := b.world.object(b.assisted())
	if len(enemies) == 0 || lead == nil || len(b.attackers(now)) > 0 {
		return Action{}, false
	}
	var cx, cy float64
	n := 0
	for _, id := range enemies {
		if o := b.world.object(id); o != nil {
			p := o.Position(now)
			cx, cy, n = cx+float64(p.X), cy+float64(p.Y), n+1
		}
	}
	l := lead.Position(now)
	dx, dy := float64(l.X)-cx/float64(n), float64(l.Y)-cy/float64(n)
	d := math.Hypot(dx, dy)
	s := &b.world.Self
	switch {
	case s.Sitting:
		return b.toggle(now), true
	case s.Casting(now) || b.castPending(now) || d < 1:
		return Action{}, true
	}
	p := Point{X: l.X + int32(math.Round(dx/d*behindDist)), Y: l.Y + int32(math.Round(dy/d*behindDist)), Z: l.Z}
	from := s.Position(now)
	if horizontal2(from, p) <= behindSlack*behindSlack {
		return Action{}, true
	}
	if b.walker().nav == nil {
		if s.Moving(now) {
			return Action{}, true
		}
		return moveAction(p, from), true
	}
	a, st := b.goTo(p, walkSlop, walkFollow, now)
	if st == walkUnreachable {
		return Action{}, false
	}
	return a, true
}

// rulePartyBuff puts up the PvP row's self buffs gated by archetype (a
// TargetSelf step with Vs, no HP gate) against the party's enemy set at
// now (docs/pvp/party.md "The bot's healer"): due while an enemy of one of
// those archetypes is in the set and the step's gates pass against it. The
// bot's healer casts 1035 Mental Shield on every patient (supportAllowed:
// its SupportFlagged), another bot on itself (a tank's 112 Deflect Arrow
// against archers). Each goes under the stack rule (buffDue) and buffRetry.
func (b *Brain) rulePartyBuff(now time.Time) (Action, bool) {
	enemies := b.partyEnemies(now)
	if len(enemies) == 0 {
		return Action{}, false
	}
	kit := b.build().Kit
	self := b.world.Self.ObjectID
	for _, step := range kit.PvP {
		if step.Target != TargetSelf || len(step.Vs) == 0 || step.SelfHPBelow > 0 || step.SelfHPAbove > 0 ||
			step.Idle || step.Peel || step.Break || step.Interrupt || !b.castReady(step.Skill, now) {
			continue
		}
		i := slices.IndexFunc(enemies, func(id int32) bool {
			r := b.pvp.opponents[id]
			return r != nil && slices.Contains(step.Vs, r.Archetype)
		})
		if i < 0 || !step.passes(b.stepState(enemies[i], kit.PvP, now)) {
			continue
		}
		on := []int32{self}
		if step.Skill == skillMentalShield && b.selfHealer() {
			on = b.patients(now)
		}
		// A buff of target ONE lands on the selected creature: the bot
		// selects the patient, itself included, first.
		sk, _ := b.skillData(step.Skill)
		for _, id := range on {
			if !b.buffDue(id, step.Skill, now) || now.Sub(b.buffedAt[buffKey{id, step.Skill}]) < buffRetry {
				continue
			}
			return b.castOn(id, step.Skill, sk.Target != "SELF", now), true
		}
	}
	return Action{}, false
}

// partySleep is the side sleep of a party fight (docs/pvp/layer.md "Sleep
// and damage", docs/pvp/party.md): fighting id at now, a ready sleep control
// step of rot goes on another enemy of the set that neither the bot nor a
// member struck within attackerWindow (never the healer being hit), under
// the control rules against it (controlDue: no DOT on it, not resisted
// twice, MinLand) and the step's gates. It selects that enemy first and
// casts once it is selected (sideCasting).
func (b *Brain) partySleep(id int32, rot []Step, underReserve bool, now time.Time) (Action, bool) {
	if underReserve || !b.partyFight(now) {
		return Action{}, false
	}
	enemies := b.partyEnemies(now)
	for _, step := range rot {
		s, ok := b.skillData(step.Skill)
		if !ok || step.Target != TargetEnemy || step.rank() != rankAttack || step.Idle || step.Interrupt || step.Break || step.Peel ||
			!lands(s, "SLEEP", "Sleep") || !b.castReady(step.Skill, now) {
			continue
		}
		for _, e := range enemies {
			if e == id || b.struck(e, now) || b.stepHeld(e, step.Skill, now) || !step.passes(b.stepState(e, rot, now)) {
				continue
			}
			if bot, opp, fighters := b.pvpFighters(e); !b.controlDue(e, step.Skill, bot, opp, fighters, now) {
				continue
			}
			b.side, b.sided = sideCast{target: e, skill: step.Skill}, e
			if b.world.Target == e {
				return b.sideCasting(e, now)
			}
			return selectAction(e, b.world.Self.Position(now)), true
		}
	}
	return Action{}, false
}

// sleeps reports whether skill puts its target to sleep.
func (b *Brain) sleeps(skill int32) bool {
	s, ok := b.skillData(skill)
	return ok && lands(s, "SLEEP", "Sleep")
}
