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
	// castStopsFrom is the shortest hit time in the bot's own MagicSkillUse
	// that proves the server stopped it before the cast. The server stops a
	// caster when the skill's template hit time is over 50 ms
	// (cast/request.go stopForCast); the packet carries it scaled by cast
	// speed, which may raise a 50 ms one above 50 but floors any template of
	// 500 ms or more at 500 (cast/controller.go buildPlan). Shorter casts
	// (potions, toggles, instant buffs) leave the walk alone.
	castStopsFrom = 500 * time.Millisecond
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
// bot fights and supports, and the PvP modes it plays (docs/pvp/modes.md,
// D35). It is not a safety boundary.
type PvPPolicy struct {
	// FightFlagged lets the bot fight back on, and assist against, players
	// who are PvP flagged or have karma.
	FightFlagged bool
	// Duels takes the duel challenges the bot can take and challenges
	// players (docs/pvp/finding.md, X5); without it the bot declines every
	// challenge at once (Brain.declineDuel).
	Duels bool
	// Hunt attacks the flagged and karma players in sight of its tier
	// (docs/pvp/targets.md "Hunting", X5).
	Hunt bool
	// Arenas visits an arena on a timer and fights there
	// (docs/pvp/finding.md, X5).
	Arenas bool
	// ForceAttack is PK mode (docs/pvp/karma.md, rule 7 of MayFight): it
	// lets no player pass until its limits ship (X11).
	ForceAttack bool
	// SupportFlagged lets the bot heal or buff a flagged or karma player,
	// which flags the bot (character_pvpflag.go skillTargetFlagsPvP).
	SupportFlagged bool
}

// MaySupport reports whether the policy lets the bot heal or buff player o:
// one with neither a PvP flag nor karma, or any with SupportFlagged. The
// brain narrows it in a duel and widens it in a PvP zone
// (Brain.supportAllowed).
func (p PvPPolicy) MaySupport(o *Object) bool {
	return p.SupportFlagged || clean(o)
}

// selectAction clicks id to select it.
func selectAction(id int32, from Point) Action {
	return Action{kind: actSelect, payload: encodeAction(id, from), object: id}
}

// mayFight reports whether the bot may attack id at now: in sight, alive,
// allowed by MayFight's rules (fightKind) and not blacklisted.
func (b *Brain) mayFight(id int32, now time.Time) bool {
	o := b.world.object(id)
	return o != nil && !o.Dead && b.fightKind(id, o, now) != fightNone && !now.Before(b.blacklist[id])
}

// attackers are the bot's attackers it may fight at now, the hardest hitter
// first (damageOf). A summon's owner in sight that the bot may fight stands
// in for the summon (docs/pvp/targets.md "Summons and pets").
func (b *Brain) attackers(now time.Time) []int32 {
	var ids []int32
	for id := range b.world.Attackers {
		if !b.world.attacking(id, now) {
			continue
		}
		if owner := b.summonOwner(id, now); owner != 0 {
			id = owner
		} else if !b.mayFight(id, now) {
			continue
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(x, y int32) int {
		return cmp.Or(cmp.Compare(b.damageOf(y, now), b.damageOf(x, now)), cmp.Compare(x, y))
	})
	return ids
}

// damageOf is the damage attacker id's streak dealt the bot at now, with
// that of the summons it stands in for (attackers).
func (b *Brain) damageOf(id int32, now time.Time) int64 {
	var d int64
	for k, a := range b.world.Attackers {
		if k == id || b.world.attacking(k, now) && b.summonOwner(k, now) == id {
			d += a.Damage
		}
	}
	return d
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
		// A summon MayFight refuses is held off, not fled, while the bot
		// has no karma (docs/pvp/targets.md "Summons and pets").
		if o.Summon && b.world.Self.Karma == 0 && b.fightRule(k, o, now) == fightNone {
			continue
		}
		if id == 0 || a.Damage > damage || a.Damage == damage && k < id {
			id, damage = k, a.Damage
		}
	}
	return id
}

// mayFlee reports whether a run away at now could end the fight: always,
// but with FleeOutrun while a live creature attacking the bot runs as fast
// as it or faster (effective run speeds, NPCInfo and CharInfo against
// UserInfo).
func (b *Brain) mayFlee(now time.Time) bool {
	if !b.persona.FleeOutrun {
		return true
	}
	run := b.world.Self.RunSpeed
	for id := range b.world.Attackers {
		if o := b.world.object(id); o != nil && !o.Dead && b.world.attacking(id, now) && o.RunSpeed >= run {
			return false
		}
	}
	return true
}

// ruleOutnumbered is rule 4: hit by two or more attackers, flee from the
// hardest hitter with HP under OutmatchedHP while it may (mayFlee), else
// fight it. A target that
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
	if below(s.HP, s.MaxHP, b.build().Thresholds.OutmatchedHP) && b.mayFlee(now) {
		return b.flee(hits[0], now), true
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	id, cur := hits[0], b.world.Target
	if cur != id && slices.Contains(hits, cur) && b.damageOf(id, now) <= switchFactor*b.damageOf(cur, now) {
		id = cur
	}
	return b.fight(id, now), true
}

// knowsAttackSkill reports whether the character knows one of its
// rotation's skills as an active skill.
func (b *Brain) knowsAttackSkill() bool {
	for _, st := range b.build().Kit.Rotation {
		if s, ok := b.world.Skills[st.Skill]; ok && !s.Passive {
			return true
		}
	}
	return false
}

// knowsNuke reports whether the character knows the skill of one of rot's
// attack steps as an active skill: a caster that knows none melees
// whatever its Melee policy, as before its row (a mystic seeded or trained
// without its nukes would otherwise stand by while its leader fights).
func (b *Brain) knowsNuke(rot []Step) bool {
	return slices.ContainsFunc(rot, func(st Step) bool {
		s, ok := b.world.Skills[st.Skill]
		return ok && !s.Passive && st.rank() == rankAttack
	})
}

// fight attacks id: cast a side cast due on the selected attacker
// (sideCasting) or select id, switch a combat toggle that is due
// (toggleCast), then cast the first ready step of the build's rotation
// whose gates pass (rotationCast: self-target steps, sustain, then attacks
// with MP at MPReserve or more; pvpCast's PvP order against a player), or
// click it once for melee as the kit's
// Melee policy allows, side stepping when stuck. Either goes
// out only while the straight line to id is open, else the bot walks to a
// stand point first (engageWalk). A target out of attempts is blacklisted
// instead. While Ultimate Defense holds the bot (immobile) no walk starts:
// no stand point, chase or side step.
func (b *Brain) fight(id int32, now time.Time) Action {
	s := &b.world.Self
	from := s.Position(now)
	if t := b.world.Target; id != t {
		if a, ok := b.sideCasting(t, now); ok {
			return a
		}
		if b.engaged != 0 && b.engaged == t && !b.mayFight(t, now) {
			return b.disengage(now)
		}
		if b.selectPending(id, now) {
			return Action{}
		}
		return selectAction(id, from)
	}
	if s.Casting(now) || b.castPending(now) {
		return Action{}
	}
	forced := b.forced(id, now)
	bd := b.build()
	if a, ok := b.toggleCast(bd.Kit.Toggles, true, now); ok {
		return a
	}
	underReserve := below(s.MP, s.MaxMP, bd.Thresholds.MPReserve)
	cast := b.rotationCast
	if o := b.world.object(id); o != nil && o.Kind == KindPlayer {
		// Against a player: the PvP layer over the row (docs/pvp/layer.md,
		// D35), for every preset.
		cast = b.pvpCast
	}
	if a, ok := cast(id, bd.Kit.Rotation, underReserve, now); ok {
		return a
	}
	if !bd.Kit.Melee.allows(underReserve, !b.world.Party.In()) && b.knowsNuke(bd.Kit.Rotation) {
		return Action{}
	}
	r := b.attackReach()
	// The guards (docs/pvp/guards.md): a pole wielder whose swing would
	// reach a bystander repositions or holds; no swing the flag guard, the
	// arena edge or the duel follow holds.
	if b.poleHeld(id, now) {
		return b.poleReposition(id, now)
	}
	if b.swingHeld(id, now) {
		return Action{}
	}
	if b.immobile(now) && !b.meleeInRange(id, now) {
		// Ultimate Defense: no chase, a move would only idle the bot.
		return Action{}
	}
	if b.engaged != id {
		if b.exhausted(id, now) {
			return Action{}
		}
		if a, walk := b.engageWalk(id, r, now); walk {
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
	if a, walk := b.engageWalk(id, r, now); walk {
		return a
	}
	if now.Sub(b.progressAt) >= stuckAfter && !b.immobile(now) {
		return b.sideStep(id, now)
	}
	return Action{}
}

// forced reports whether attacking id at now takes a forced attack: a
// player not attackable without force (character_social.go
// SocialWithoutForce), so not its duel opponent while fighting, not one in
// its arena, and neither flagged nor with karma. A summon never is: the bot
// fights one with plain Action and casts only.
func (b *Brain) forced(id int32, now time.Time) bool {
	o := b.world.object(id)
	return o != nil && o.Kind == KindPlayer && id != b.duelFoe(now) && !flagged(o) && !b.sameArena(id, now)
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
	case msgCannotBeUsed:
		b.chargesRefused(b.castSkill)
		fallthrough
	case msgNotEnoughHP, msgNotEnoughMP, msgInvalidTarget, msgCantSeeTarget:
		b.castAt = time.Time{}
		b.world.holdSkill(b.castSkill, now.Add(castRefusedHold))
	}
}

// chargesRefused corrects the bot's force charges when the server refused
// its cast of skill with msgCannotBeUsed, the message of a failed Charges
// condition (data/xml/skills, cond msgId 113), and a rotation step gates
// skill on charges. EtcStatusUpdate carries no object id, so another
// player's broadcast (its Charm of Courage) may have overwritten the bot's
// count; the refusal tells the right side: a ChargeToMax skill is refused at
// its maxCharges, a Charges skill under its Charges. So a wrong count costs
// one refused cast, not one per castRefusedHold until the charges change.
func (b *Brain) chargesRefused(skill int32) {
	s := &b.world.Self
	for _, st := range b.build().Kit.Rotation {
		switch {
		case st.Skill != skill:
		case st.ChargeToMax:
			if mx := b.cfg.Travel.maxChargesOf(skill, b.world.Skills[skill].Level); mx > 0 {
				s.Charges = int32(mx)
			}
			return
		case st.Charges > 0:
			s.Charges = min(s.Charges, int32(st.Charges-1))
			return
		}
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
// the attack intention on the server. The step goes in a random direction
// that holds on the geodata and keeps inside an arena and off a duel's
// forbidden ground (turnAround, moveOK), else nowhere (orStay).
func (b *Brain) disengage(now time.Time) Action {
	return b.orStay(b.turnAround(b.rng.Float64()*2*math.Pi, now), now)
}

// sideStep steps sideStep across the line to id, to either side, when an
// attack on it is stuck. The side it picks comes first; when that step does
// not hold on the geodata the other side is tried, else it steps nowhere
// (orStay): the move still drops the stuck attack, so the next decision
// attacks again and counts the attempt toward the blacklist.
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
	return b.orStay(b.checkedStep(sideStep, now, angle, angle+math.Pi), now)
}

// orStay is step, or a move to where the bot stands when step sends
// nothing. That move goes nowhere, so it cannot leave the floor, yet it
// replaces the attack intention like any walk (the server's movement.go
// startLiveMove replaces the intention before it simulates the walk), so an
// attack in flight ends either way.
func (b *Brain) orStay(step Action, now time.Time) Action {
	if step.kind == actMove {
		return step
	}
	return moveAction(b.standing(now), b.world.Self.Position(now))
}

// turnHeadings is how many evenly spaced headings a turnStep step tries.
const turnHeadings = 8

// turnAround is a turnStep step heading angle, else the first of the other
// turnHeadings headings around it whose step holds and moveOK passes;
// nothing when none does.
func (b *Brain) turnAround(angle float64, now time.Time) Action {
	var angles [turnHeadings]float64
	for i := range angles {
		angles[i] = angle + float64(i)*2*math.Pi/turnHeadings
	}
	return b.groundStep(turnStep, now, b.moveOK, angles[:]...)
}

// checkedStep is a move of dist from where the bot stands along the first
// of angles whose step holds on the geodata both ways (D13): the walker's
// segment check there and back, so the step neither crosses a wall nor
// drops off a ledge or into a pit the bot cannot climb back out of, and
// whose end is not in water. It is the zero Action when no angle holds. Without geodata nothing is checked
// and the first angle goes out, as before.
func (b *Brain) checkedStep(dist int32, now time.Time, angles ...float64) Action {
	return b.groundStep(dist, now, nil, angles...)
}

// groundStep is checkedStep along the first of angles whose step also
// passes accept (from the bot's spot to the step's end at now); nil accepts
// every step.
func (b *Brain) groundStep(dist int32, now time.Time, accept func(from, to Point, now time.Time) bool, angles ...float64) Action {
	pos := b.world.Self.Position(now)
	w := b.walker()
	if w.nav == nil {
		for _, angle := range angles {
			if to := offset(pos, angle, dist); accept == nil || accept(pos, to, now) {
				return moveAction(to, pos)
			}
		}
		return Action{}
	}
	from := b.standing(now)
	for _, angle := range angles {
		to := offset(from, angle, dist)
		if accept != nil && !accept(from, to, now) {
			continue
		}
		c, ok := w.plan.Locate(toLoc(to))
		if !ok {
			continue
		}
		to.Z = int32(c.Z)
		// A swimmer's segments may cross water, but a step never ends in
		// it: a bot left standing in water drowns.
		if w.nav.Water.Contains(int(to.X), int(to.Y), int(c.Z)) {
			continue
		}
		if w.segmentOK(from, to) && w.segmentOK(to, from) {
			return moveAction(to, pos)
		}
	}
	return Action{}
}

// standCaster stands the bot exactly where its own MagicSkillUse casts
// from, for the cast it asked for when the server stopped it first: the
// player cast path stops the caster (cast/request.go stopForCast) before
// it broadcasts the cast with the caster's position then
// (network/magic_skill.go broadcastCastStart). A dead-reckoned walk's end
// may miss that spot by a step, since a cast approach's pawn walk ends on
// its first step in range with no packet, and the walker's next segment
// goes out from it (#270, as applyAttack for #246). A cast the bot did not
// ask for (a cubic's, a class change's) may come while the bot walks on,
// and so does one too short to stop it.
func (b *Brain) standCaster(e MagicSkillUse) {
	if b.castAt.IsZero() || e.SkillID != b.castSkill || e.HitTime < castStopsFrom {
		return
	}
	if m := b.world.motionOf(e.CasterID); m != nil {
		*m = stopAt(e.CasterPos)
	}
}
