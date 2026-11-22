package botclient

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"time"
)

// Combat timing. Melee clicks a target once (engaged) and the server
// repeats the swing; a cast waits for its MagicSkillUse, then the skill
// waits out hit time plus reuse (World.SkillReady). Both go through
// Brain.send and the frame bucket; neither has a server reuse gate of its
// own beyond the skill's reuse, which the bot waits out.
const (
	// castTimeout is how long a RequestMagicSkillUse waits for the bot's
	// own MagicSkillUse before the bot acts again. The skill stays held as
	// long, so a refused cast (no target, MP short, muted: magic_skill.go
	// attemptMagicSkill) is not asked again sooner; a refusal with a
	// reason message ends the wait at once (castRefused). A cast asked during a
	// swing is deferred and still answered ActionFailed (magic_skill.go
	// handleRequestMagicSkillUse), so ActionFailed does not end the wait.
	castTimeout = 3 * time.Second
	// stuckAfter is how long the bot may stand engaged with no swing, cast
	// or move of its own before it side steps (plan, shared behavior).
	stuckAfter = 2500 * time.Millisecond
	// sideStep is the length of that step.
	sideStep = 150
	// switchFactor is how many times its target's damage a harder hitter
	// must have dealt before rule 4 switches to it, so a close race does
	// not flip the target every decision.
	switchFactor = 2
)

// PvPPolicy is the persona's player-versus-player defaults (D6): whom the
// bot fights and supports. It is not a safety boundary.
type PvPPolicy struct {
	// FightFlagged lets the bot fight back on, and assist against, players
	// who are PvP flagged or have karma.
	FightFlagged bool
	// ForceAttack lets the bot force-attack (AttackRequest) players who are
	// neither, which makes it a player killer.
	ForceAttack bool
	// SupportFlagged lets the bot heal or buff a flagged player, which flags
	// the bot.
	SupportFlagged bool
}

// MayFight reports whether the policy lets the bot attack o: an attackable
// NPC, or a player the policy allows.
func (p PvPPolicy) MayFight(o *Object) bool {
	switch o.Kind {
	case KindNPC:
		return o.Attackable
	case KindPlayer:
		return p.ForceAttack || p.FightFlagged && flagged(o)
	}
	return false
}

// MaySupport reports whether the policy lets the bot heal or buff player o.
func (p PvPPolicy) MaySupport(o *Object) bool {
	return p.SupportFlagged || o.PvPFlag == 0
}

// selectAction clicks id to select it.
func selectAction(id int32, from Point) Action {
	return Action{kind: actSelect, payload: encodeAction(id, from), object: id}
}

// mayFight reports whether the bot may attack id at now: in sight, alive,
// allowed by the PvP policy and not blacklisted.
func (b *Brain) mayFight(id int32, now time.Time) bool {
	o := b.world.object(id)
	return o != nil && !o.Dead && b.persona.PvP.MayFight(o) && !now.Before(b.blacklist[id])
}

// attackers are the bot's attackers it may fight at now, the hardest hitter
// first.
func (b *Brain) attackers(now time.Time) []int32 {
	var ids []int32
	for id := range b.world.Attackers {
		if b.world.attacking(id, now) && b.mayFight(id, now) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(x, y int32) int {
		return cmp.Or(cmp.Compare(b.world.Attackers[y].Damage, b.world.Attackers[x].Damage), cmp.Compare(x, y))
	})
	return ids
}

// unfought is the hardest hitter among the live creatures attacking the
// bot at now that it blacklisted, or 0 when there is none. A player the PvP
// policy leaves alone is not one: holding is that policy's answer.
func (b *Brain) unfought(now time.Time) int32 {
	var (
		id     int32
		damage int64
	)
	for k, a := range b.world.Attackers {
		o := b.world.object(k)
		if o == nil || o.Dead || !b.world.attacking(k, now) || !now.Before(b.blacklist[k]) {
			continue
		}
		if id == 0 || a.Damage > damage || a.Damage == damage && k < id {
			id, damage = k, a.Damage
		}
	}
	return id
}

// ruleOutnumbered is rule 4: hit by two or more attackers, flee from the
// hardest hitter with HP under OutmatchedHP, else fight it. A target that
// is one of the attackers is kept until the hardest hitter has dealt more
// than switchFactor times its damage.
func (b *Brain) ruleOutnumbered(now time.Time) (Action, bool) {
	s := &b.world.Self
	if !s.InCombat {
		return Action{}, false
	}
	hits := b.attackers(now)
	if len(hits) < 2 {
		return Action{}, false
	}
	if below(s.HP, s.MaxHP, b.persona.OutmatchedHP) {
		return b.flee(hits[0], now), true
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	id, cur := hits[0], b.world.Target
	if cur != id && slices.Contains(hits, cur) && b.world.Attackers[id].Damage <= switchFactor*b.world.Attackers[cur].Damage {
		id = cur
	}
	return b.fight(id, now), true
}

// fight attacks id: select it, then cast the persona's first ready attack
// skill or click it once for melee, side stepping when stuck. Either goes
// out only while the straight line to id is open, else the bot walks to a
// stand point first (engageWalk). A target out of attempts is blacklisted
// instead.
func (b *Brain) fight(id int32, now time.Time) Action {
	s := &b.world.Self
	from := s.Position(now)
	if t := b.world.Target; id != t {
		if b.engaged != 0 && b.engaged == t && !b.mayFight(t, now) {
			return b.disengage(now)
		}
		return selectAction(id, from)
	}
	if s.Casting(now) || b.castPending(now) {
		return Action{}
	}
	forced := b.forced(id)
	var skills []int32
	if !below(s.MP, s.MaxMP, b.persona.MPReserve) {
		skills = b.persona.AttackSkills
	}
	for _, skill := range skills {
		if b.world.SkillReady(skill, now) {
			if b.exhausted(id, now) {
				return Action{}
			}
			if a, walk := b.engageWalk(id, true, now); walk {
				return a
			}
			return Action{kind: actCast, payload: encodeRequestMagicSkillUse(skill, forced), object: id, skill: skill}
		}
	}
	ranged := b.bowEquipped()
	if b.engaged != id {
		if b.exhausted(id, now) {
			return Action{}
		}
		if a, walk := b.engageWalk(id, ranged, now); walk {
			return a
		}
		if forced {
			return Action{kind: actAttack, payload: encodeAttackRequest(id, from), object: id}
		}
		return attackAction(id, from)
	}
	if s.Moving(now) {
		b.progressAt = now
		return Action{}
	}
	// The target moved where the straight line to it is closed: walk to a
	// new stand point (the move drops the attack; it goes out again from
	// there).
	if a, walk := b.engageWalk(id, ranged, now); walk {
		return a
	}
	if now.Sub(b.progressAt) >= stuckAfter {
		return b.sideStep(id, now)
	}
	return Action{}
}

// forced reports whether attacking id takes a forced attack: a player who
// is neither flagged nor has karma.
func (b *Brain) forced(id int32) bool {
	o := b.world.object(id)
	return o != nil && o.Kind == KindPlayer && !flagged(o)
}

// Cast refusals: the system messages the server answers a player's cast
// with when it fails its cost or target checks, with no MagicSkillUse and
// no ActionFailed (magic_skill.go sendMagicCastFailureReason,
// sendTargetCastRejection; ids from serverpackets/systemmessage.go). A
// datapack condition's own message is not listed: castTimeout bounds that
// wait.
const (
	msgNotEnoughHP   = 23
	msgNotEnoughMP   = 24
	msgInvalidTarget = 109
	msgCannotBeUsed  = 113
	msgCantSeeTarget = 181
)

// castRefusedHold is how long a refused skill is not asked again: MP or HP
// short, or the target out of sight, does not change within castTimeout,
// so the bot fights or rests meanwhile instead of re-asking every
// castTimeout.
const castRefusedHold = 10 * time.Second

// castRefused ends the cast in flight on the server's refusal message id
// at now and holds its skill for castRefusedHold.
func (b *Brain) castRefused(id int32, now time.Time) {
	if !b.castPending(now) {
		return
	}
	switch id {
	case msgNotEnoughHP, msgNotEnoughMP, msgInvalidTarget, msgCannotBeUsed, msgCantSeeTarget:
		b.castAt = time.Time{}
		b.world.holdSkill(b.castSkill, now.Add(castRefusedHold))
	}
}

// castPending reports whether a cast asked for is still waiting for its
// MagicSkillUse at now.
func (b *Brain) castPending(now time.Time) bool {
	return !b.castAt.IsZero() && now.Sub(b.castAt) < castTimeout
}

// exhausted reports whether id used up its BlacklistAfter attempts with no
// swing or cast landing, and blacklists it for BlacklistFor if so.
func (b *Brain) exhausted(id int32, now time.Time) bool {
	n := b.persona.BlacklistAfter
	if n <= 0 || b.tries[id] < n {
		return false
	}
	maps.DeleteFunc(b.blacklist, func(_ int32, until time.Time) bool { return !now.Before(until) })
	b.blacklist[id] = now.Add(b.persona.BlacklistFor)
	delete(b.tries, id)
	return true
}

// attempt records an attack or a cast on id sent at now.
func (b *Brain) attempt(id int32, now time.Time) {
	b.tries[id]++
	b.progressAt = now
}

// landed records a swing or a cast of the bot's own on id at now: the
// target is reachable, its attempts start over.
func (b *Brain) landed(id int32, now time.Time) {
	delete(b.tries, id)
	b.progressAt = now
}

// disengage drops the attack in flight with a short step: a move replaces
// the attack intention on the server.
func (b *Brain) disengage(now time.Time) Action {
	from := b.world.Self.Position(now)
	return moveAction(offset(from, b.rng.Float64()*2*math.Pi, turnStep), from)
}

// sideStep steps sideStep across the line to id, to either side, when an
// attack on it is stuck.
func (b *Brain) sideStep(id int32, now time.Time) Action {
	from := b.world.Self.Position(now)
	angle := b.rng.Float64() * 2 * math.Pi
	if o := b.world.object(id); o != nil {
		if t := o.Position(now); t.X != from.X || t.Y != from.Y {
			angle = math.Atan2(float64(t.Y-from.Y), float64(t.X-from.X)) + math.Pi/2
			if b.rng.IntN(2) == 0 {
				angle -= math.Pi
			}
		}
	}
	return moveAction(offset(from, angle, sideStep), from)
}
