package botclient

import (
	"maps"
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The PvP layer (docs/pvp/layer.md, X6): against a player the bot runs its
// row (the same Step list, gates and Melee policy) with its PvP row on top,
// in the PvP order: sustain under the long-cast rule, interrupts, control
// at a land rate of MinLand or more, the opener, then damage (finishers
// first once the pool estimate is under their expected damage), then the
// Melee policy. A caster whose root or slow landed on a melee opponent
// kites first. Every cast still goes through the one cast path (tryStep,
// castStep): one at a time under the frame bucket, so the layer adds no
// sender of its own; the kite walk goes through the walker's pacing.

// Layer figures (docs/pvp/layer.md).
const (
	// MinLand is the least land rate, in percent (charmodel.LandRate), a
	// control step is cast at against a player.
	MinLand = 35
	// MinBreak is the least cast-break chance, in percent
	// (charmodel.CastBreakChance), a damage step's interrupt is used at.
	MinBreak = 30
	// longCast is the template hit time from which a heal or HOT falls
	// under the long-cast rule; longCastReach is how far past a melee
	// opponent's attack range it holds the heal.
	longCast      = 4000 * time.Millisecond
	longCastReach = 150
	// worthyHit is the template hit time from which an opponent's sleep,
	// root, fear, mute or MDAM at the bot or its party is worth an
	// interrupt.
	worthyHit = 3000 * time.Millisecond
	// kiteSlack is how much closer than its cast range a kiting caster
	// lets a melee opponent come before it walks out again; kitePoints is
	// how many stand points around the opponent the kite tries.
	kiteSlack  = 200
	kitePoints = 16
	// disabledMask are the CharInfo abnormal bits of a stun, a sleep and
	// a root (model/skill/abnormal.go); dotMask those of the HP DOTs:
	// bleeding, poison and flame.
	disabledMask = 0x40 | 0x80 | rootAbnormal
	dotMask      = 0x1 | 0x2 | 0x4000
)

// healTypes are the skill types of a heal or HOT; worthyHeals those an
// opponent's cast is worth an interrupt by at anyone, worthyHostile those
// it is worth one by at the bot or its party from worthyHit on.
var (
	healTypes     = map[string]bool{"HEAL": true, "HOT": true, "HEAL_PERCENT": true}
	worthyHeals   = map[string]bool{"HEAL": true, "HOT": true, "HEAL_PERCENT": true, "BALANCE_LIFE": true, "RESURRECT": true}
	worthyHostile = map[string]bool{"SLEEP": true, "ROOT": true, "FEAR": true, "MUTE": true, "MDAM": true}
)

// controlTypes are the skill types of a control step; controlEffects the
// effects that make one of any type (Stun Attack is a PDAM landing a
// Stun); effectBits the CharInfo abnormal bit each effect shows, which
// tells the bot it wore off (model/skill/abnormal.go).
var (
	controlTypes   = map[string]bool{"STUN": true, "SLEEP": true, "ROOT": true, "FEAR": true, "MUTE": true, "PARALYZE": true, "DEBUFF": true}
	controlEffects = map[string]bool{"Stun": true, "Sleep": true, "Root": true, "Fear": true, "Mute": true, "Paralyze": true}
	effectBits     = map[string]int32{"Fear": 0x20, "Stun": 0x40, "Sleep": 0x80, "Mute": 0x100, "Root": rootAbnormal}
)

// landing is one of the bot's casts on an opponent, as long as no 139 told
// it resisted: when it lands, until when its longest effect lasts (ticks ×
// time), the CharInfo bits its effects show (0 for none), whether it ticks
// HP (a DamOverTime effect) and whether it kites (a root or a run speed
// slow).
type landing struct {
	at, until  time.Time
	bits       int32
	dot, kites bool
}

// holds reports whether l still holds its target, whose CharInfo mask is
// abnormal, at now: before its end, and, for an effect with a bit, while
// the bit shows (or before it may show, rootLandSlack after landing).
func (l landing) holds(abnormal int32, now time.Time) bool {
	return now.Before(l.until) && (l.bits == 0 || now.Before(l.at.Add(rootLandSlack)) || abnormal&l.bits != 0)
}

// layerFight is the layer's record of the bot's casts on one opponent: its
// skills' landings, the resists (139) per skill, and the landing of the
// opponent's cast it last answered with an interrupt. It is owned by the
// brain goroutine, kept in the opponent record while the player is in
// sight and started anew when a duel with the player starts.
type layerFight struct {
	landed   map[int32]landing
	resists  map[int32]int
	answered time.Time
}

// ownLast is the bot's own last cast at a player: a 139 is about it, and
// a cancel of it puts back prev, the skill's landing before it (had: there
// was one).
type ownLast struct {
	target, skill int32
	prev          landing
	had           bool
}

// skillAt is skill's datapack entry at level; ok is false with no travel
// map or no such entry.
func (b *Brain) skillAt(skill, level int32) (datapack.Skill, bool) {
	if b.cfg.Travel == nil {
		return datapack.Skill{}, false
	}
	s, ok := b.cfg.Travel.skills[datapack.SkillRef{ID: skill, Level: level}]
	return s, ok
}

// spanOf is how long s's effects last (the longest ticks × time), the
// CharInfo bits they show, whether one ticks HP and whether one kites.
func spanOf(s datapack.Skill) (lasts time.Duration, bits int32, dot, kites bool) {
	for _, e := range s.Effects {
		lasts = max(lasts, time.Duration(e.Time*max(e.Count, 1))*time.Second)
		bits |= effectBits[e.Name]
		dot = dot || e.Name == "DamOverTime"
		kites = kites || e.Name == "Root" || slows(e)
	}
	return lasts, bits, dot, kites
}

// slows reports whether e lowers its target's run speed.
func slows(e datapack.Effect) bool {
	return slices.ContainsFunc(e.Funcs, func(f datapack.StatFunc) bool {
		return f.Stat == "runSpd" && f.Op == "mul" && f.Value < 1
	})
}

// isControl reports whether s is a control skill: a control type, or one
// landing a control effect.
func isControl(s datapack.Skill) bool {
	return controlTypes[s.Type] || slices.ContainsFunc(s.Effects, func(e datapack.Effect) bool { return controlEffects[e.Name] })
}

// lands reports whether s is of type typ or lands an effect named name.
func lands(s datapack.Skill, typ, name string) bool {
	return s.Type == typ || slices.ContainsFunc(s.Effects, func(e datapack.Effect) bool { return e.Name == name })
}

// aborts reports whether s, landed, aborts its target's cast: a stun or a
// sleep (docs/pvp/server-casts.md "Cast break").
func aborts(s datapack.Skill) bool {
	return lands(s, "STUN", "Stun") || lands(s, "SLEEP", "Sleep")
}

// layerCast keeps the bot's own cast e at now in its target's opponent
// record: the skill's landing while its effects last, and the cast a 139
// will be about.
func (b *Brain) layerCast(e MagicSkillUse, now time.Time) {
	r := b.pvp.opponents[e.TargetID]
	if r == nil {
		b.pvp.last = ownLast{}
		return
	}
	f := &r.fight
	prev, had := f.landed[e.SkillID]
	b.pvp.last = ownLast{target: e.TargetID, skill: e.SkillID, prev: prev, had: had}
	s, ok := b.skillAt(e.SkillID, e.Level)
	if !ok {
		return
	}
	lasts, bits, dot, kites := spanOf(s)
	if lasts == 0 {
		return
	}
	if f.landed == nil {
		f.landed = make(map[int32]landing)
	}
	at := now.Add(e.HitTime)
	f.landed[e.SkillID] = landing{at: at, until: at.Add(lasts), bits: bits, dot: dot, kites: kites}
}

// castCanceled takes back the bot's own last cast at a player when the
// server cancels it before its hit (MagicSkillCanceled goes to the caster
// too: server network/magic_skill.go broadcastCastAborted): its landing
// never comes, the skill's earlier one is put back. A cancel at or after
// the hit is about another cast: the landing stays.
func (b *Brain) castCanceled(now time.Time) {
	l := b.pvp.last
	b.pvp.last = ownLast{}
	r := b.pvp.opponents[l.target]
	if r == nil {
		return
	}
	g, ok := r.fight.landed[l.skill]
	if !ok || !now.Before(g.at) {
		return
	}
	if l.had {
		r.fight.landed[l.skill] = l.prev
		return
	}
	delete(r.fight.landed, l.skill)
}

// resisted keeps a 139 (the target resisted the bot's effect): the bot's
// last cast at a player did not land, one more resist of its skill this
// fight.
func (b *Brain) resisted() {
	l := b.pvp.last
	r := b.pvp.opponents[l.target]
	if r == nil {
		return
	}
	b.pvp.last = ownLast{}
	f := &r.fight
	delete(f.landed, l.skill)
	if f.resists == nil {
		f.resists = make(map[int32]int)
	}
	f.resists[l.skill]++
}

// pvpCast is the PvP order over rot, the row's rotation, fighting player
// id at now (docs/pvp/layer.md "PvP order"): a cure or CP potion (pvpItem)
// and the break-off (breaksOff, breakAway) first, the kite walk, then the PvP
// row's steps and rot's (pvpSteps) in groups, each under the M10 gates
// (tryStep): self-target steps and sustain under the long-cast rule,
// interrupts, control steps that are due, the opener, finishers once the
// pool estimate is under their expected damage, the other attacks; a DOT
// step not while its own DOT still ticks on id (dotHolds). ok is false
// when none goes: fight follows the Melee policy.
func (b *Brain) pvpCast(id int32, rot []Step, underReserve bool, now time.Time) (Action, bool) {
	if a, ok := b.pvpItem(id, now); ok {
		return a, true
	}
	rot = pvpSteps(b.build().Kit.PvP, rot)
	st := b.stepState(id, rot, now)
	if b.breaksOff(id, rot, st, now) {
		return b.breakAway(id, rot, st, now), true
	}
	if a, ok := b.kiteWalk(id, now); ok {
		return a, true
	}
	if len(rot) == 0 {
		return Action{}, false
	}
	first := func(pick func(Step) bool) (Action, bool) {
		for _, step := range rot {
			if !pick(step) {
				continue
			}
			if a, ok := b.tryStep(id, step, st, underReserve, now); ok {
				return a, true
			}
		}
		return Action{}, false
	}
	for _, rank := range []int{rankSelf, rankSustain} {
		if a, ok := first(func(s Step) bool { return s.rank() == rank && !b.longCastHeld(s.Skill, now) }); ok {
			return a, true
		}
	}
	if a, ok := b.interruptCast(id, rot, st, underReserve, now); ok {
		return a, true
	}
	if st.peel {
		if a, ok := first(func(s Step) bool { return s.Peel }); ok {
			return a, true
		}
	}
	bot, opp, fighters := b.pvpFighters(id)
	control := func(s Step) bool {
		sk, ok := b.skillData(s.Skill)
		return ok && s.Target == TargetEnemy && isControl(sk)
	}
	// In a party fight a sleep never goes on the bot's own target, which
	// it hits next: it goes on another enemy nobody hits (partySleep).
	party := b.partyFight(now)
	if a, ok := first(func(s Step) bool {
		return s.rank() == rankAttack && control(s) && !(party && b.sleeps(s.Skill)) && b.controlDue(id, s.Skill, bot, opp, fighters, now)
	}); ok {
		return a, true
	}
	if a, ok := b.partySleep(id, rot, underReserve, now); ok {
		return a, true
	}
	attack := func(s Step) bool { return s.rank() == rankAttack && !control(s) && !b.dotHolds(id, s.Skill, now) }
	if a, ok := first(func(s Step) bool { return attack(s) && s.Opener }); ok {
		return a, true
	}
	if pool, ok := b.pool(id); ok && fighters {
		if a, ok := first(func(s Step) bool {
			sk, _ := b.skillData(s.Skill)
			return attack(s) && charmodel.SkillDamage(bot, sk, opp) >= pool
		}); ok {
			return a, true
		}
	}
	return first(attack)
}

// pvpSteps are the steps a row plays against a player (docs/pvp/rows.md:
// a row lists only what differs from M10): its PvP row's steps, then its
// rotation's steps of the skills the PvP row does not name, and its side
// casts on another attacker (TargetOtherAttacker, EO's Dryad Root). A PvP
// row naming a skill plays it only as its own steps say (KN's Aggression
// only as a Peel, Ultimate Defense only vs M; a mystic opener's Vs gate).
func pvpSteps(pvp, rot []Step) []Step {
	out := slices.Clip(pvp)
	for _, st := range rot {
		if st.Target == TargetOtherAttacker || !slices.ContainsFunc(pvp, func(p Step) bool { return p.Skill == st.Skill }) {
			out = append(out, st)
		}
	}
	return out
}

// dotHolds reports whether the bot's DOT of skill (an HP or MP damage
// over time) it landed on player id still ticks at now: the step waits
// for it to wear off (docs/pvp/layer.md "PvP order"), as a control step
// does.
func (b *Brain) dotHolds(id, skill int32, now time.Time) bool {
	s, ok := b.skillData(skill)
	r := b.pvp.opponents[id]
	if !ok || r == nil || !slices.ContainsFunc(s.Effects, func(e datapack.Effect) bool {
		return e.Name == "DamOverTime" || e.Name == "ManaDamOverTime"
	}) {
		return false
	}
	l, ok := r.fight.landed[skill]
	return ok && l.holds(r.Abnormal, now)
}

// pool is the pool estimate of player id (docs/pvp/layer.md "CP, cures,
// pool, break off"): its record's CP and HP, as last reported less the
// damage the bot dealt since (Brain.dealt). ok is false before any report
// of its HP.
func (b *Brain) pool(id int32) (float64, bool) {
	r := b.pvp.opponents[id]
	if r == nil || r.MaxHP <= 0 {
		return 0, false
	}
	return float64(r.HP + r.CP), true
}

// longCastHeld reports whether the long-cast rule holds the bot's heal of
// skill at now (docs/pvp/layer.md "PvP order"): a heal or HOT of template
// hit time longCast or more while a melee player the bot fights, not
// stunned, asleep or rooted (its CharInfo mask), stands within its attack
// range plus longCastReach. The opponent's weapon is not kept, so a pole's
// range, the longest melee reach, stands for it.
func (b *Brain) longCastHeld(skill int32, now time.Time) bool {
	s, ok := b.skillData(skill)
	if !ok || !healTypes[s.Type] || s.HitTime < longCast {
		return false
	}
	for id, r := range b.pvp.opponents {
		if r.Archetype == charmodel.Melee && r.Abnormal&disabledMask == 0 && b.fights(id, now) && b.inReach(id, poleRange+longCastReach, now) {
			return true
		}
	}
	return false
}

// controlDue reports whether the bot's control step of skill goes on player
// id at now (docs/pvp/layer.md "PvP order"): its effect does not hold id
// (a fresh opponent, or the effect wore off), a sleep or root id did not
// resist twice this fight, a sleep only under the sleep rule (sleepHolds),
// and its land rate of bot on opp, the reference opponent, is MinLand or
// more (unknown without fighters: the row decides as against a monster).
func (b *Brain) controlDue(id, skill int32, bot, opp charmodel.Fighter, fighters bool, now time.Time) bool {
	s, ok := b.skillData(skill)
	r := b.pvp.opponents[id]
	if !ok || r == nil {
		return true
	}
	if l, ok := r.fight.landed[skill]; ok && l.holds(r.Abnormal, now) {
		return false
	}
	sleep := lands(s, "SLEEP", "Sleep")
	if (sleep || lands(s, "ROOT", "Root")) && r.fight.resists[skill] >= 2 {
		return false
	}
	if sleep && !b.sleepHolds(id, now) {
		return false
	}
	return !fighters || charmodel.LandRate(s, bot, opp) >= MinLand
}

// sleepHolds reports whether a holding sleep may go on player id at now
// (docs/pvp/layer.md "Sleep and damage"): every hit and DOT tick ends a
// sleep, so none while its CharInfo mask shows an HP DOT (bleeding, poison,
// flame) or a DOT the bot landed on it still ticks. That nobody hits it
// while it sleeps is the caller's to know: a Break sleep is followed by the
// flee (X6b), a party sleeps a target no member hits (X9).
func (b *Brain) sleepHolds(id int32, now time.Time) bool {
	r := b.pvp.opponents[id]
	if r == nil || r.Abnormal&dotMask != 0 {
		return false
	}
	for _, l := range r.fight.landed {
		if l.dot && now.Before(l.until) {
			return false
		}
	}
	return true
}

// interruptCast is the interrupt the bot answers player id's last cast
// with at now (docs/pvp/layer.md "Interrupts"): on an interrupt-worthy cast
// still to land and not answered yet, the first Interrupt or Hold step on
// the enemy that lands in time (answersInTime) and goes (tryStep). At most
// one per opponent cast.
func (b *Brain) interruptCast(id int32, rot []Step, st stepState, underReserve bool, now time.Time) (Action, bool) {
	r := b.pvp.opponents[id]
	if r == nil {
		return Action{}, false
	}
	c := r.LastCast
	if !now.Before(c.Lands) || r.fight.answered.Equal(c.Lands) {
		return Action{}, false
	}
	cast, ok := b.interruptWorthy(c)
	if !ok {
		return Action{}, false
	}
	bot, opp, fighters := b.pvpFighters(id)
	st.interrupt = true
	for _, step := range rot {
		if !step.Interrupt && !step.Hold || step.Target != TargetEnemy || !b.answersInTime(id, step.Skill, c, cast, bot, opp, fighters, now) {
			continue
		}
		if a, ok := b.tryStep(id, step, st, underReserve, now); ok {
			r.fight.answered = c.Lands
			return a, true
		}
	}
	return Action{}, false
}

// interruptWorthy is the datapack entry of c, a player's cast, when it is
// worth an interrupt: a heal, HOT, percent heal or resurrection at anyone,
// or a sleep, root, fear, mute or MDAM of template hit time worthyHit or
// more at the bot or a party member.
func (b *Brain) interruptWorthy(c CastSeen) (datapack.Skill, bool) {
	s, ok := b.skillAt(c.Skill, c.Level)
	switch {
	case !ok:
		return s, false
	case worthyHeals[s.Type]:
		return s, true
	case worthyHostile[s.Type] && s.HitTime >= worthyHit:
		_, member := b.world.Party.Members[c.Target]
		return s, c.Target == b.world.Self.ObjectID || member
	}
	return s, false
}

// answersInTime reports whether the bot's step of skill answers cast c (its
// datapack entry cast) on player id in time at now: sent once the reaction
// delay from c's arrival passed (at most PvPReactMax), plus the walk into
// cast range and the step's own hit time (Brain.hitTime), a stun or sleep
// lands before c does; a damage step lands escLead before a magic c, with
// a cast-break chance of bot's expected damage against opp's MEN of
// MinBreak or more. No other step answers.
func (b *Brain) answersInTime(id, skill int32, c CastSeen, cast datapack.Skill, bot, opp charmodel.Fighter, fighters bool, now time.Time) bool {
	s, ok := b.skillData(skill)
	if !ok {
		return false
	}
	walk, ok := b.walkTime(id, skill, now)
	if !ok {
		return false
	}
	start := now
	if react := c.At.Add(b.cfg.PvPReactMax); react.After(start) {
		start = react
	}
	done := start.Add(walk + b.hitTime(skill))
	switch {
	case aborts(s):
		return done.Before(c.Lands)
	case isDamageTypes[s.Type] && s.Power > 0:
		return cast.Magic && !done.After(c.Lands.Add(-escLead)) && fighters &&
			charmodel.CastBreakChance(charmodel.SkillDamage(bot, s, opp), opp.MEN) >= MinBreak
	}
	return false
}

// walkTime is how long the bot runs at now before its cast of skill reaches
// player id: none in range or for a skill cast with no approach, else the
// gap over its run speed. ok is false with no run speed known.
func (b *Brain) walkTime(id, skill int32, now time.Time) (time.Duration, bool) {
	r, approach := b.castReach(skill)
	if !approach || b.inReach(id, r.rng, now) {
		return 0, true
	}
	s, o := &b.world.Self, b.world.object(id)
	if o == nil || s.RunSpeed <= 0 {
		return 0, false
	}
	gap := dist3(s.Position(now), o.Position(now)) - r.rng - s.CollisionRadius - b.radiusOf(id)
	return time.Duration(gap / float64(s.RunSpeed) * float64(time.Second)), true
}

// rangedRow reports whether the bot plays a caster's row or a bow branch:
// the rows that kite (docs/pvp/layer.md "Kite").
func (b *Brain) rangedRow() bool {
	row, ok := charmodel.ClassRow(b.world.Self.ClassID, b.bowEquipped())
	return ok && row.Archetype() != charmodel.Melee
}

// kiting is the skill whose landing the bot kites player id on at now: a
// root or slow of its own holding a melee opponent while the bot plays a
// ranged row, the longest lasting first. ok is false for none.
func (b *Brain) kiting(id int32, now time.Time) (skill int32, ok bool) {
	r := b.pvp.opponents[id]
	if r == nil || r.Archetype != charmodel.Melee || !b.rangedRow() {
		return 0, false
	}
	var until time.Time
	for sk, l := range r.fight.landed {
		if l.kites && !now.Before(l.at) && l.holds(r.Abnormal, now) && (l.until.After(until) || l.until.Equal(until) && sk < skill) {
			skill, until, ok = sk, l.until, true
		}
	}
	return skill, ok
}

// kiteWalk is the kite (docs/pvp/layer.md "Kite"): while the bot's root or
// slow holds melee player id at now (kiting) and id stands kiteSlack closer
// than that skill's cast range, a walk to a stand point at the cast range
// (kitePoint) before the next cast; a hold while that walk is on. ok is
// false when no walk is due, no stand point passes or Ultimate Defense
// holds the bot (immobile): the bot casts where it stands.
func (b *Brain) kiteWalk(id int32, now time.Time) (Action, bool) {
	skill, ok := b.kiting(id, now)
	o := b.world.object(id)
	r, approach := b.castReach(skill)
	if !ok || o == nil || !approach || b.immobile(now) {
		b.pvp.kite = Point{}
		return Action{}, false
	}
	s, w := &b.world.Self, b.walker()
	from, to := s.Position(now), o.Position(now)
	if w.nav == nil && b.pvp.kite != (Point{}) && s.Moving(now) {
		return Action{}, true
	}
	if dist3(from, to) > r.rng-kiteSlack {
		b.pvp.kite = Point{}
		return Action{}, false
	}
	rng := r.rng - walkSlop
	p := b.pvp.kite
	if p == (Point{}) || math.Abs(math.Sqrt(float64(horizontal2(p, to)))-rng) > walkSameDest {
		if p, ok = b.kitePoint(from, to, rng, now); !ok {
			b.pvp.kite = Point{}
			return Action{}, false
		}
	}
	b.pvp.kite = p
	if w.nav == nil {
		return moveAction(p, from), true
	}
	a, st := b.goTo(p, walkSlop, walkRanged, now)
	if st == walkArrived || st == walkUnreachable {
		b.pvp.kite = Point{}
		return Action{}, false
	}
	return a, true
}

// kitePoint is the kite's stand point rng from the opponent at to, the bot
// at from, at now: of kitePoints points around to, straight away from it
// first, then alternately to either side, the first that stays arenaEdge
// inside the bot's arena when it stands in one (the arena-edge guard holds
// damage nearer, docs/pvp/guards.md "Arena edge"), whose segment from the
// bot passes the duel ground in a duel (duelGroundOK) and the walker's
// straight-line check, and which sees to. ok is false when none does.
func (b *Brain) kitePoint(from, to Point, rng float64, now time.Time) (Point, bool) {
	away := 0.0
	if from.X != to.X || from.Y != to.Y {
		away = math.Atan2(float64(from.Y-to.Y), float64(from.X-to.X))
	}
	w, own := b.walker(), b.arenaAt(from)
	for i := range kitePoints {
		off := float64((i+1)/2) * 2 * math.Pi / kitePoints
		if i%2 == 0 {
			off = -off
		}
		a := away + off
		p := Point{X: to.X + int32(math.Round(rng*math.Cos(a))), Y: to.Y + int32(math.Round(rng*math.Sin(a))), Z: from.Z}
		if own.Arena != 0 {
			if at := b.arenaAt(p); at.Arena != own.Arena || at.Edge <= arenaEdge {
				continue
			}
		}
		if b.duelGroundOK(from, p, now) && w.lineOpen(from, p) && w.sightOpen(p, to, defaultTargetHeight) {
			return p, true
		}
	}
	return Point{}, false
}

// pvpRows is the engage estimate's PvP row table at the bot's level
// (pvpRowTable), built again when its level changes.
func (b *Brain) pvpRows() charmodel.RowTable {
	level := max(int(b.world.Self.Level), 1)
	if p := &b.pvp; p.rows == nil || p.rowsLevel != level {
		p.rows, p.rowsLevel = pvpRowTable(b.cfg.Travel, shippedKits, level), level
	}
	return b.pvp.rows
}

// shippedKits are the kit tables whose PvP rows the row table reads.
var shippedKits = []map[int32]ClassKit{lineKits, professionKits}

// pvpRowTable is the PvP row table of kits at character level
// (docs/pvp/model.md "Row table"): per row whose PvP row ships
// (ClassKit.PvP: the fighter rows from X7, the mystic rows from X8), the
// steps it plays against a player (pvpSteps) of RoleNone or RoleDamage
// from level on, filtered to damage, DOT and control skills, each at the
// highest skill level its class tree grants by level, Vs copied; a scout
// row's bow steps go to its bow branch, its other weapon steps to its
// dagger branch, the rest to both. Empty without a travel map.
func pvpRowTable(m *TravelMap, kits []map[int32]ClassKit, level int) charmodel.RowTable {
	t := charmodel.RowTable{}
	if m == nil {
		return t
	}
	for _, table := range kits {
		for _, class := range slices.Sorted(maps.Keys(table)) {
			kit := table[class]
			if len(kit.PvP) == 0 || !rowClass(class) {
				continue
			}
			row, ok := charmodel.ClassRow(class, false)
			bow, _ := charmodel.ClassRow(class, true)
			if !ok {
				continue
			}
			for _, st := range pvpSteps(kit.PvP, kit.Rotation) {
				ref, ok := m.refStep(class, st, level)
				if !ok {
					continue
				}
				rows := []charmodel.Row{row}
				switch {
				case !bow.Bow:
				case slices.Contains(st.Weapons, "BOW"):
					rows = []charmodel.Row{bow}
				case len(st.Weapons) == 0:
					rows = append(rows, bow)
				}
				for _, r := range rows {
					t[r] = append(t[r], ref)
				}
			}
		}
	}
	return t
}

// rowClass reports whether class plays a row of its own: a base class or a
// first profession.
func rowClass(class int32) bool {
	parent, ok := datapack.ParentClass(class)
	if !ok || parent < 0 {
		return ok
	}
	grand, ok := datapack.ParentClass(parent)
	return ok && grand < 0
}

// refStep is step st of class's PvP row as the row table holds it at
// character level: its skill at the highest level class's tree grants by
// level, from the later of st.From and the first grant's level; ok is
// false for a step not played at level, of another role, not granted by
// then, neither damage, DOT nor control, or not cast in a plain fight (a
// Break, Peel or Interrupt step, a side cast on another attacker).
func (m *TravelMap) refStep(class int32, st Step, level int) (charmodel.RefStep, bool) {
	if int(st.From) > level || st.Role != RoleNone && st.Role != RoleDamage || st.Break || st.Peel || st.Interrupt || st.Target == TargetOtherAttacker {
		return charmodel.RefStep{}, false
	}
	skill, first := 0, 0
	for _, g := range m.classes[class].Skills {
		if g.ID != st.Skill || g.MinLevel > level {
			continue
		}
		skill = max(skill, g.Level)
		if first == 0 || g.MinLevel < first {
			first = g.MinLevel
		}
	}
	s, ok := m.skills[datapack.SkillRef{ID: st.Skill, Level: int32(skill)}]
	if !ok || skill == 0 {
		return charmodel.RefStep{}, false
	}
	_, _, dot, _ := spanOf(s)
	if !dot && !isControl(s) && (!isDamageTypes[s.Type] || s.Power <= 0) {
		return charmodel.RefStep{}, false
	}
	return charmodel.RefStep{Skill: s, Level: max(int(st.From), first), Vs: st.Vs}, true
}
