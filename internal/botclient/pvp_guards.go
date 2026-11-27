package botclient

import (
	"math"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The guards against accidental karma (docs/pvp/guards.md, X4b): a
// karma-free opponent whose flag ends, or who leaves the arena, dies to the
// bot's direct hit as a PK, so the bot starts no damage step that could
// land after that, Esc-cancels the one in flight while the server still
// honours it, and drops its attack intention with a move. A player's DOT
// never kills (hooks_dot.go), so DOTs and control always go.

// Guard figures (docs/pvp/guards.md "Arena edge"): the bot keeps arenaEdge
// inside an arena and lets an opponent's run cover edgeSlack more than the
// time to landing; an Esc stops the bot's cast until escLead before its hit
// (network/targeting.go cancelCastForTargetCancel).
const (
	arenaEdge = 150
	edgeSlack = 500 * time.Millisecond
	escLead   = 200 * time.Millisecond
	// rootAbnormal is the CharInfo abnormal bit of a root
	// (AbnormalEffect ROOT).
	rootAbnormal = 0x200
	// unknownRunSpeed is the run speed of an opponent the bot has none for,
	// over any run speed of the classes before buffs (144).
	unknownRunSpeed = 150
)

// isDamageTypes are the server's IsDamage skill types
// (model/skill/definition.go).
var isDamageTypes = map[string]bool{
	"PDAM": true, "MDAM": true, "DRAIN": true, "BLOW": true, "CPDAMPERCENT": true,
	"DEATHLINK": true, "CHARGEDAM": true, "FATAL": true, "SIGNET_CASTTIME": true,
}

// skillData is skill's datapack entry at the level the bot knows it; ok is
// false with no travel map or a skill it does not hold.
func (b *Brain) skillData(skill int32) (datapack.Skill, bool) {
	if b.cfg.Travel == nil {
		return datapack.Skill{}, false
	}
	s, ok := b.cfg.Travel.skills[datapack.SkillRef{ID: skill, Level: b.world.Skills[skill].Level}]
	return s, ok
}

// damageStep reports whether skill is a damage step (docs/pvp/guards.md):
// an IsDamage skill or one the server follows with a swing
// (nextActionAttack). A skill the bot has no datapack entry for counts as
// one: the guards err toward holding.
func (b *Brain) damageStep(skill int32) bool {
	s, ok := b.skillData(skill)
	return !ok || isDamageTypes[s.Type] || s.NextActionAttack
}

// hitTime is the predicted hit time of the bot's cast of skill
// (docs/pvp/layer.md "Interrupts"): the template hit time × 333 / cast
// speed for a magic skill or attack speed else, truncated to the
// millisecond; × 0.70 for a magic skill while a spiritshot is expected
// charged; at least 500 ms for a template time of 500 ms or more
// (skill/formulas cast time). 0 for a skill with no datapack entry.
func (b *Brain) hitTime(skill int32) time.Duration {
	s, ok := b.skillData(skill)
	if !ok {
		return 0
	}
	tmpl := s.HitTime.Milliseconds()
	spd := b.world.Self.PAtkSpd
	if s.Magic {
		spd = b.world.Self.CastSpd
	}
	ms := tmpl
	if spd > 0 {
		ms = tmpl * 333 / int64(spd)
	}
	if s.Magic && b.spiritshotCharged() {
		ms = ms * 70 / 100
	}
	if tmpl >= 500 {
		ms = max(ms, 500)
	}
	return time.Duration(ms) * time.Millisecond
}

// spiritshotCharged reports whether a spiritshot is expected charged: the
// server reported auto-use on for a spiritshot or blessed spiritshot
// (ExAutoSoulShot) and the bot carries a stack of it.
func (b *Brain) spiritshotCharged() bool {
	inv := &b.world.Inventory
	table := b.shotTable()
	for id, on := range inv.AutoShots {
		if k := table[id].kind; !on || k != datapack.EtcSpiritshot && k != datapack.EtcBlessedSpiritshot {
			continue
		}
		if it, ok := inv.ByTemplate(id); ok && it.Count > 0 {
			return true
		}
	}
	return false
}

// rootCast is the bot's own last cast of a root skill: its target, when it
// lands, how long the root lasts, and whether the server told it resisted
// (139).
type rootCast struct {
	target   int32
	lands    time.Time
	lasts    time.Duration
	resisted bool
}

// rootLasts is how long skill's root lasts (its Root effect's time); ok is
// false for a skill that lands no root.
func (b *Brain) rootLasts(skill int32) (time.Duration, bool) {
	s, ok := b.skillData(skill)
	if !ok {
		return 0, false
	}
	for _, e := range s.Effects {
		if e.Name == "Root" {
			return time.Duration(e.Time) * time.Second, true
		}
	}
	return 0, false
}

// ownCast keeps the bot's own cast e at now: a root's landing, which a
// later CharInfo confirms (Brain.rootSeen), and the PvP layer's record of
// it (Brain.layerCast), both at the player it was cast at (castTarget).
func (b *Brain) ownCast(e MagicSkillUse, now time.Time) {
	e.TargetID = b.castTarget(e)
	if lasts, ok := b.rootLasts(e.SkillID); ok {
		b.pvp.root = rootCast{target: e.TargetID, lands: now.Add(e.HitTime), lasts: lasts}
	}
	b.layerCast(e, now)
}

// castTarget is the object the bot's own cast e is at: its MagicSkillUse
// target, but for an offensive aura skill (Seal of Binding), whose
// MagicSkillUse names the caster (server
// handler/target/target_handlers_area.go: every aura handler's
// FinalTarget), the bot's selected target, the one it cast the step at.
func (b *Brain) castTarget(e MagicSkillUse) int32 {
	if e.TargetID != e.CasterID {
		return e.TargetID
	}
	s, ok := b.skillAt(e.SkillID, e.Level)
	if !ok || !s.Offensive || !strings.Contains(s.Target, "AURA") {
		return e.TargetID
	}
	return b.world.Target
}

// rootLandSlack is how long after its landing a root the bot cast may
// still show first in the target's CharInfo.
const rootLandSlack = 2 * time.Second

// rootSeen keeps r's root from player id's CharInfo abnormal mask at now: a
// root bit turning on just after the bot's own root landed on it with no
// 139 holds it for the root's duration from the landing; the bit gone ends
// it. Stun and sleep never count (a hit may end them).
func (b *Brain) rootSeen(id int32, r *Opponent, abnormal int32, now time.Time) {
	if abnormal&rootAbnormal == 0 {
		r.RootUntil = time.Time{}
		return
	}
	rc := b.pvp.root
	if r.Abnormal&rootAbnormal != 0 || rc.target != id || rc.resisted || now.Before(rc.lands.Add(-escLead)) || now.After(rc.lands.Add(rootLandSlack)) {
		return
	}
	r.RootUntil = rc.lands.Add(rc.lasts)
}

// rootHold is how long the root the bot landed on id still holds it at now.
func (b *Brain) rootHold(id int32, now time.Time) time.Duration {
	r := b.pvp.opponents[id]
	if r == nil {
		return 0
	}
	return max(r.RootUntil.Sub(now), 0)
}

// guarded reports whether id is a player or summon the guards watch.
func guarded(o *Object) bool { return o != nil && (o.Kind == KindPlayer || o.Summon) }

// blinking reports whether the flag guard holds damage on o (object id) at
// now (docs/pvp/guards.md "Flag guard"): a karma-free player or summon
// whose flag blinks (state 2), outside a duel and unless both stand in an
// arena (a PvP zone, where a kill is free).
func (b *Brain) blinking(o *Object, now time.Time) bool {
	if o.Karma > 0 || o.PvPFlag != 2 || b.inDuel(now) {
		return false
	}
	if v := b.karma.victim; b.pkVictim(v, now) && b.world.object(v) == o {
		// The guards are off for PK mode's victim (docs/pvp/karma.md).
		return false
	}
	return !b.inArena(now) || b.arenaAt(o.Position(now)).Arena == 0
}

// edgeWatched is the arena opponent whose edge distance the arena-edge rule
// reads for o (object id) at now: o itself for a karma-free player, a
// summon's owner (the server credits its death by its owner's zone), nil
// outside the rule (no arena, a duel, karma). ok is false for one the bot
// may not hit at all: a summon whose owner is not in sight.
func (b *Brain) edgeWatched(id int32, o *Object, now time.Time) (w *Object, ok bool) {
	if o.Karma > 0 || b.inDuel(now) || !b.inArena(now) {
		return nil, true
	}
	if o.Summon {
		owner := b.world.object(b.playerNamed(b.pvp.owners[id]))
		return owner, owner != nil
	}
	return o, true
}

// runOf is o's run speed, unknownRunSpeed when none is known.
func runOf(o *Object) float64 {
	if o.RunSpeed <= 0 {
		return unknownRunSpeed
	}
	return float64(o.RunSpeed)
}

// edgeHeld reports whether a damage step landing in landing would risk an
// arena kill outside at now (docs/pvp/guards.md "Arena edge"): the watched
// opponent w is outside the bot's arena, or its edge distance does not
// exceed its run speed × (free time + edgeSlack), free time being landing
// less the time the bot's root still holds it; or the bot itself stands
// within arenaEdge of the edge.
func (b *Brain) edgeHeld(id int32, w *Object, landing time.Duration, now time.Time) bool {
	own := b.arenaAt(b.world.Self.Position(now))
	at := b.arenaAt(w.Position(now))
	if at.Arena == 0 || at.Arena != own.Arena || own.Edge <= arenaEdge {
		return true
	}
	free := max(landing-b.rootHold(id, now), 0)
	return at.Edge <= runOf(w)*(free+edgeSlack).Seconds()
}

// damageHeld reports whether the guards hold a damage step on id landing
// in landing at now: the flag guard, or the arena-edge rule.
func (b *Brain) damageHeld(id int32, landing time.Duration, now time.Time) bool {
	o := b.world.object(id)
	if !guarded(o) {
		return false
	}
	if b.blinking(o, now) {
		return true
	}
	w, ok := b.edgeWatched(id, o, now)
	if !ok {
		return true
	}
	return w != nil && b.edgeHeld(id, w, landing, now)
}

// stepHeld reports whether the guards hold the bot's step of skill on id at
// now: a damage step the flag guard or the arena edge holds, an area step
// with a bystander in reach, a step the pole's bystander rule holds, or one
// out of range while the duel follow holds the bot where it stands.
func (b *Brain) stepHeld(id, skill int32, now time.Time) bool {
	if b.duelHeld(id, now) && !b.castInRange(id, skill, now) {
		return true
	}
	if b.areaHeld(id, skill, now) {
		return true
	}
	if !b.damageStep(skill) {
		return false
	}
	if s, _ := b.skillData(skill); s.NextActionAttack && b.poleHeld(id, now) {
		return true
	}
	return b.damageHeld(id, b.hitTime(skill), now)
}

// swingHeld reports whether the guards hold the bot's swing at id at now.
func (b *Brain) swingHeld(id int32, now time.Time) bool {
	return b.damageHeld(id, swingTime(b.world.Self.PAtkSpd), now) ||
		b.duelHeld(id, now) && !b.meleeInRange(id, now)
}

// guardState is the guards' memory: the opponent of the bot's attack
// intention (its last swing or damage cast, until a move drops it), the
// target of its last cast and whether that cast was a damage step, and the
// hit time of the cast an Esc already went for (one Esc per cast).
type guardState struct {
	intent     int32
	castOn     int32
	castDamage bool
	escFor     time.Time
	// selecting is the creature of the last selecting Action, sent at
	// selectAt.
	selecting int32
	selectAt  time.Time
	// clicked is the creature of the last Action packet, a select or an
	// attack: the one a 692 (frozen duellist) answers.
	clicked int32
	// stopping is the opponent of a stop under way: its disengage move
	// still goes once the Esc ended the cast, even with the cause gone.
	stopping int32
}

// selectPending reports whether a selecting Action on id is still
// unanswered at now while the guards hold a swing at it: a second one
// would reach the server with id selected and attack it (late Esc,
// docs/pvp/guards.md "Flag guard").
func (b *Brain) selectPending(id int32, now time.Time) bool {
	g := &b.pvp.guard
	return g.selecting == id && now.Sub(g.selectAt) < castTimeout && b.world.Target != id && b.swingHeld(id, now)
}

// guardSent keeps the guards' memory of action a sent at now. A cast
// counts as a damage step only on a creature the bot may fight (a heal on
// a party member is none, whatever the datapack lacks).
func (b *Brain) guardSent(a Action, now time.Time) {
	g := &b.pvp.guard
	switch a.kind {
	case actAttack:
		g.intent, g.clicked = a.object, a.object
	case actCast:
		g.castOn = a.object
		g.castDamage = a.object != b.world.Self.ObjectID && b.damageStep(a.skill) && b.mayFight(a.object, now)
		if g.castDamage {
			g.intent = a.object
		}
	case actMove, actSitStand:
		g.intent, g.stopping = 0, 0
	case actSelect:
		g.selecting, g.selectAt, g.clicked = a.object, now, a.object
	case actTargetCancel:
		g.escFor = b.world.Self.castEnd
	}
}

// casting reports whether the bot's own cast is in flight at now: asked
// for, or started and not hit yet.
func (b *Brain) casting(now time.Time) bool {
	return b.world.Self.Casting(now) || b.castPending(now)
}

// guardDue is the player or summon the guards must stop on at now, 0 for
// none: the target of the bot's damage cast in flight, or of its attack
// intention, that it may no longer fight, whose flag blinks, that the
// arena edge or a duel's ground forbids, or that a pole swing would reach a
// bystander next to.
func (b *Brain) guardDue(now time.Time) int32 {
	g := &b.pvp.guard
	id := g.intent
	inFlight := b.casting(now) && g.castDamage && g.castOn != b.world.Self.ObjectID
	if inFlight {
		id = g.castOn
	}
	o := b.world.object(id)
	if !guarded(o) || o.Dead {
		return 0
	}
	if !b.mayFight(id, now) || b.blinking(o, now) || b.duelGroundStop(id, now) {
		return id
	}
	if id == g.intent && id == b.engaged && b.poleHeld(id, now) {
		return id
	}
	w, ok := b.edgeWatched(id, o, now)
	switch {
	case !ok:
		return id
	case w == nil:
		return 0
	}
	if id == g.intent && b.edgeHeld(0, w, swingTime(b.world.Self.PAtkSpd), now) {
		return id
	}
	if inFlight && b.world.Self.Casting(now) && b.edgeHeld(id, w, b.world.Self.castEnd.Sub(now), now) {
		return id
	}
	return 0
}

// ruleGuard stops a fight the guards forbid (docs/pvp/guards.md "Stop"):
// an Esc for the bot's own damage cast while the server still honours it
// (until escLead before its hit), a hold while a cast past that window
// lands, then a disengage move that drops the attack intention and the
// follow-up swing. It takes no reaction delay: the events that call for it
// wake the brain at the next frame (Brain.guardWake).
func (b *Brain) ruleGuard(now time.Time) (Action, bool) {
	id := b.guardTarget(now)
	if id == 0 {
		return Action{}, false
	}
	g, s := &b.pvp.guard, &b.world.Self
	g.stopping = id
	if b.casting(now) {
		if g.castOn == id && g.castDamage && s.Casting(now) && now.Before(s.castEnd.Add(-escLead)) && !g.escFor.Equal(s.castEnd) {
			return Action{kind: actTargetCancel, payload: botwire.EncodeRequestTargetCancel()}, true
		}
		// Past the window (or answered with no MagicSkillUse yet): it
		// lands; the move goes first once it did.
		if s.Casting(now) && (b.reactAt.IsZero() || s.castEnd.Before(b.reactAt)) {
			b.reactAt = s.castEnd
		}
		return Action{}, true
	}
	if g.intent != id {
		return Action{}, false
	}
	return b.disengage(now), true
}

// guardTarget is the opponent a guard stop is for at now: one due
// (guardDue), or the stop under way while its intention stays.
func (b *Brain) guardTarget(now time.Time) int32 {
	if id := b.guardDue(now); id != 0 {
		return id
	}
	g := &b.pvp.guard
	if g.stopping != 0 && (g.stopping == g.intent || g.stopping == g.castOn && b.casting(now)) {
		return g.stopping
	}
	g.stopping = 0
	return 0
}

// guardWake is when ev, received at now, wakes the brain for a guard stop:
// at the next frame when it is an event the stop reads and one is due, at
// the frame a walk carries the arena opponent into the swing margin
// (Brain.edgeCross) when none is due yet, zero otherwise.
func (b *Brain) guardWake(ev Event, now time.Time) time.Time {
	switch ev.(type) {
	case RelationChanged, CharInfo, MoveToLocation, MagicSkillUse, MagicSkillCanceled, TargetUnselected, StopMove, ValidateLocation:
		if b.guardTarget(now) != 0 {
			return now.Add(time.Second / frameRate)
		}
		return b.edgeCross(now)
	}
	return time.Time{}
}

// edgeCross is the first frame after now at which the walk of the arena
// opponent the bot's attack intention watches (edgeWatched) brings it
// within the swing margin of the arena edge, or out of the arena
// (edgeHeld), zero when it walks nowhere near. The margin is fixed while
// the edge distance shrinks, so a walk that starts outside it crosses it
// on the way: without this wake the stop would wait for the regular tick
// while the server keeps swinging on the intention. The walk ends by its
// motion's arrival, so the frames sampled are bounded by its length.
func (b *Brain) edgeCross(now time.Time) time.Time {
	id := b.pvp.guard.intent
	o := b.world.object(id)
	if !guarded(o) || o.Dead {
		return time.Time{}
	}
	w, ok := b.edgeWatched(id, o, now)
	if !ok || w == nil || !w.Moving(now) {
		return time.Time{}
	}
	swing, frame := swingTime(b.world.Self.PAtkSpd), time.Second/frameRate
	end := w.move.end().Add(frame)
	for t := now.Add(frame); !t.After(end); t = t.Add(frame) {
		if b.edgeHeld(0, w, swing, t) {
			return t
		}
	}
	return time.Time{}
}

// moveOK reports whether the bot may step from from to to at now: inside an
// arena the step ends at least arenaEdge inside (or, from closer, further
// in), and in a duel it keeps off forbidden ground (duelGroundOK) and clear
// of aggressive monsters (duelClearOK).
func (b *Brain) moveOK(from, to Point, now time.Time) bool {
	if own := b.arenaAt(from); own.Arena != 0 {
		at := b.arenaAt(to)
		if at.Arena != own.Arena || at.Edge < arenaEdge && at.Edge <= own.Edge {
			return false
		}
	}
	return b.duelGroundOK(from, to, now) && b.duelClearOK(from, to, now)
}

// dist3 is the 3D distance between a and b.
func dist3(a, b Point) float64 {
	return math.Sqrt(float64(dist2(a, b)) + float64(a.Z-b.Z)*float64(a.Z-b.Z))
}
