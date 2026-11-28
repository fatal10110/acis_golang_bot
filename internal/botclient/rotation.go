package botclient

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The rotation runner (docs/strategy.md, Design sections 4, 7 and 9): fight
// casts the build's steps in four passes, combat toggles follow the bot's
// MP and its fights, ruleCritical tries the heal and drain steps before its
// potions, ruleRest sits with Relax, and every buff obeys the stack rule.
// Each cast goes through the one cast path (castOn, castStep): one at a
// time, waiting for its MagicSkillUse or castTimeout, then the skill's
// reuse, under the frame bucket.

// Pass ranks of a rotation step: fight casts every self-target step that
// passes first, then the enemy sustain steps (a SelfHPBelow step on the
// target), then the attacks, then the fillers (Step.Filler).
const (
	rankSelf = iota
	rankSustain
	rankAttack
	rankFiller
)

// rank is the step's pass in fight: a filler's whatever its target, and a
// step on another attacker or on the leader's target is an attack.
func (s Step) rank() int {
	switch {
	case s.Filler:
		return rankFiller
	case s.Target == TargetSelf:
		return rankSelf
	case s.Target == TargetEnemy && s.SelfHPBelow > 0:
		return rankSustain
	}
	return rankAttack
}

// critSustain are the heals and drains ruleCritical tries before its
// potions (docs/strategy.md, Design section 7): Self Heal, Elemental Heal,
// Divine Heal, Battle Roar, Drain Health, Vampiric Touch and Life Drain,
// and the second professions' Life Leech (SK), Steal Essence (WC),
// Vampiric Claw (SH) and Revival (WL).
var critSustain = []int32{
	skillSelfHeal, skillElementalHeal, skillDivineHeal, skillBattleRoar, skillDrainHealth, skillVampiricTouch, skillLifeDrain,
	skillLifeLeech, skillStealEssence, skillVampiricClaw, skillRevival,
}

// rotationCast is the cast of the first step of rot, fighting id at now,
// whose gates pass and whose skill is ready, in rank order: a self-target
// step only under the stack rule (buffDue), an attack or a filler only with
// MP at the reserve or more (underReserve false), a step on the leader's target only
// on that target in a party (leaderTarget), a step on another attacker on
// one the bot is not fighting (otherCast). Idle steps are ruleRebuff's. ok
// is false when none is; a hold (Action{}, true) when id ran out of
// attempts.
func (b *Brain) rotationCast(id int32, rot []Step, underReserve bool, now time.Time) (Action, bool) {
	if len(rot) == 0 {
		return Action{}, false
	}
	st := b.stepState(id, rot, now)
	for pass := rankSelf; pass <= rankFiller; pass++ {
		for _, step := range rot {
			if step.rank() != pass {
				continue
			}
			if a, ok := b.tryStep(id, step, st, underReserve, now); ok {
				return a, true
			}
		}
	}
	return Action{}, false
}

// tryStep is the cast of step, fighting id at now with gate state st, when
// it goes: not Idle, an attack or a filler only with MP at the reserve or
// more, its gates pass, its skill is ready with its reagent carried
// (castReady) and the guards do not hold it, a
// self-target step only under the stack rule (on a skill not
// casterCentered, once the bot selected itself), a step on the leader's target
// only on that target, a step on another attacker on one the bot is not
// fighting, and, while Ultimate Defense holds the bot (immobile), a step on
// another only in cast range. Only an attack uses up the opener. ok is false
// when it does not.
func (b *Brain) tryStep(id int32, step Step, st stepState, underReserve bool, now time.Time) (Action, bool) {
	r := step.rank()
	if step.Idle || (r == rankAttack || r == rankFiller) && underReserve ||
		!step.passes(st) || !b.castReady(step.Skill, now) ||
		step.Target != TargetSelf && b.stepHeld(id, step.Skill, now) ||
		step.Target != TargetSelf && b.immobile(now) && !b.castInRange(id, step.Skill, now) {
		return Action{}, false
	}
	switch step.Target {
	case TargetSelf:
		self := b.world.Self.ObjectID
		if !b.buffDue(self, step.Skill, now) {
			return Action{}, false
		}
		// A skill of datapack target ONE (Mental Shield) lands on the
		// selected creature, the opponent mid-fight (server
		// handler/target/cast_rejection.go oneCastRejection): the bot
		// selects itself first and casts it once selected (sideCasting).
		// A skill without datapack data goes as it is.
		sk, ok := b.skillData(step.Skill)
		a := b.castOn(self, step.Skill, ok && !casterCentered(sk.Target), now)
		if a.kind == actSelect {
			b.side = sideCast{target: self, skill: step.Skill}
		}
		return a, true
	case TargetLeaderTarget:
		if !b.leaderTarget(id) {
			return Action{}, false
		}
	case TargetOtherAttacker:
		return b.otherCast(id, step.Skill, now)
	}
	return b.castStep(id, step.Skill, r == rankAttack, now), true
}

// casterCentered reports whether the server casts a skill of datapack
// target t on or around the caster whatever it has selected: the SELF,
// PARTY, ALLY and CLAN handlers ignore the selection (server
// handler/target/target_handlers_single.go selfHandler, group.go). Any
// other target (ONE, PARTY_MEMBER) takes the selected creature.
func casterCentered(t string) bool {
	switch t {
	case "SELF", "PARTY", "ALLY", "CLAN":
		return true
	}
	return false
}

// leaderTarget reports whether id is the target of a TargetLeaderTarget
// step: the creature the leader last attacked while the bot assists one
// (assistPlan), any target while it assists no one.
func (b *Brain) leaderTarget(id int32) bool {
	return b.assisted() == 0 || id == b.assist.target
}

// sideCast is a step's skill due on another attacker than the one the bot
// fights, once that attacker is selected.
type sideCast struct{ target, skill int32 }

// otherCast is the first step of a TargetOtherAttacker step with skill,
// fighting id at now: select the first attacker by object id that is
// neither id nor the last one a side cast went to (Brain.sided), and cast
// skill on it once it is the target (sideCasting). ok is false with no
// such attacker.
func (b *Brain) otherCast(id, skill int32, now time.Time) (Action, bool) {
	others := slices.Sorted(slices.Values(b.attackers(now)))
	i := slices.IndexFunc(others, func(o int32) bool { return o != id && o != b.sided })
	if i < 0 {
		return Action{}, false
	}
	o := others[i]
	b.side, b.sided = sideCast{target: o, skill: skill}, o
	if b.world.Target == o {
		return b.sideCasting(o, now)
	}
	return selectAction(o, b.world.Self.Position(now)), true
}

// forgetSide drops the side cast due on id and forgets that the last one
// went to id, which died or left: the server gives a dead NPC's object id
// to its respawn (data/manager/npcs_respawn.go), a fresh attacker.
func (b *Brain) forgetSide(id int32) {
	if b.side.target == id {
		b.side = sideCast{}
	}
	if b.sided == id {
		b.sided = 0
	}
}

// sideCasting is the side cast due on target t, the bot's target, at now,
// which fight sends before it selects its own target again: a hold while a
// cast is in flight. ok is false when none is due on t, or its skill is
// not ready or t may no longer be fought. A side cast on the bot itself is
// a self step of target ONE (tryStep), cast while its buff is still due.
func (b *Brain) sideCasting(t int32, now time.Time) (Action, bool) {
	sc := b.side
	if sc.target == 0 || sc.target != t {
		return Action{}, false
	}
	s := &b.world.Self
	if s.Casting(now) || b.castPending(now) {
		return Action{}, true
	}
	b.side = sideCast{}
	if t == s.ObjectID {
		if !b.castReady(sc.skill, now) || !b.buffDue(t, sc.skill, now) {
			return Action{}, false
		}
		return b.castOn(t, sc.skill, true, now), true
	}
	if !b.castReady(sc.skill, now) || !b.mayFight(t, now) || b.stepHeld(t, sc.skill, now) {
		return Action{}, false
	}
	return b.castStep(t, sc.skill, false, now), true
}

// castStep casts skill on enemy id at now, walking to a stand point first
// when the straight line to it is closed (engageWalk); nothing once id ran
// out of attempts (exhausted). An attack (opens) uses up the target's
// opener when it goes out (Brain.send).
func (b *Brain) castStep(id, skill int32, opens bool, now time.Time) Action {
	if b.exhausted(id, now) {
		return Action{}
	}
	if r, approach := b.castReach(skill); approach {
		if a, walk := b.engageWalk(id, r, now); walk {
			return a
		}
	}
	return Action{kind: actCast, payload: botwire.EncodeRequestMagicSkillUse(skill, b.forced(id, now)), object: id, skill: skill, opens: opens}
}

// onceRecord is what Step.Once reads (stepState.onced): the target the
// bot's last cast the server took went to, and the skills the server took
// a cast of on it. A new target of the bot's, the target's death (its
// object id goes to its respawn) or its leaving forgets them (Brain.handle).
type onceRecord struct {
	target int32
	skills []int32
}

// onceCast records the bot's own MagicSkillUse e answering its cast in
// flight at now (castPending: sent within castTimeout and not refused) on
// another creature than the bot. A cast the server refused, or answered
// only after castTimeout, is not recorded, so a Once step may go again.
func (b *Brain) onceCast(e MagicSkillUse, now time.Time) {
	if !b.castPending(now) || e.SkillID != b.castSkill || e.TargetID != b.castObject || e.TargetID == b.world.Self.ObjectID {
		return
	}
	if b.once.target != e.TargetID {
		b.once = onceRecord{target: e.TargetID}
	}
	if !slices.Contains(b.once.skills, e.SkillID) {
		b.once.skills = append(b.once.skills, e.SkillID)
	}
}

// forgetOnce drops the Once record of id: the bot took another target, or
// id died or left.
func (b *Brain) forgetOnce(id int32) {
	if b.once.target == id {
		b.once = onceRecord{}
	}
}

// sustain is the cast of the build's first heal or drain step
// (critSustain, a SelfHPBelow step) whose gates pass and whose skill is
// ready at now: on the bot, or on its target while it may fight it. ok is
// false when none is, or while a cast is in flight.
func (b *Brain) sustain(now time.Time) (Action, bool) {
	s := &b.world.Self
	if s.Casting(now) || b.castPending(now) {
		return Action{}, false
	}
	id, rot := b.world.Target, b.build().Kit.Rotation
	st := b.stepState(id, rot, now)
	for _, step := range rot {
		if step.SelfHPBelow == 0 || !slices.Contains(critSustain, step.Skill) || !step.passes(st) || !b.castReady(step.Skill, now) || b.longCastHeld(step.Skill, now) {
			continue
		}
		switch step.Target {
		case TargetSelf:
			return b.castOn(s.ObjectID, step.Skill, false, now), true
		case TargetEnemy:
			if id == 0 || id == s.ObjectID || !b.mayFight(id, now) || b.stepHeld(id, step.Skill, now) {
				continue
			}
			// The server refuses a seated cast (magic_skill.go, ErrSitting):
			// stand up first, as castOn does.
			if s.Sitting {
				return b.toggle(now), true
			}
			return b.castStep(id, step.Skill, false, now), true
		}
	}
	return Action{}, false
}

// sustaining reports whether a heal or drain of critSustain the bot cast
// is still in flight at now: ruleCritical drinks no potion over it.
func (b *Brain) sustaining(now time.Time) bool {
	return (b.world.Self.Casting(now) || b.castPending(now)) && slices.Contains(critSustain, b.castSkill)
}

// toggleOn reports whether the bot's toggle skill is on: an effect of it
// with no end (AbnormalStatusUpdate's duration -1).
func (b *Brain) toggleOn(skill int32) bool {
	end, ok := b.world.Buffs[skill]
	return ok && end.IsZero()
}

// toggleSettling reports whether a cast of toggle skill went out within
// toggleSettle of now: its new state may not have reached the bot yet, and
// a second cast would undo the first.
func (b *Brain) toggleSettling(skill int32, now time.Time) bool {
	at, ok := b.skillToggledAt[skill]
	return ok && now.Sub(at) < toggleSettle
}

// isToggle reports whether skill is one of the build's toggles (its combat
// toggles or Relax).
func (b *Brain) isToggle(skill int32) bool {
	k := b.build().Kit
	return skill == k.Relax && skill != 0 || slices.ContainsFunc(k.Toggles, func(t Toggle) bool { return t.Skill == skill })
}

// toggleCast is the cast that switches a combat toggle of toggles at now:
// on while fighting with MP above its bound and its weapon worn
// (Toggle.Weapons) and, for a servitor toggle, its servitor out and alive
// (Toggle.Servitor), off with MP under it, with another weapon, without the
// servitor or out of a fight (docs/strategy.md, Design section 9). Each switch waits for the
// last one to settle; ok is false when none is due.
func (b *Brain) toggleCast(toggles []Toggle, fighting bool, now time.Time) (Action, bool) {
	s := &b.world.Self
	var worn string
	if slices.ContainsFunc(toggles, func(t Toggle) bool { return len(t.Weapons) > 0 }) {
		worn, _ = b.wornKind()
	}
	for _, t := range toggles {
		on := b.toggleOn(t.Skill)
		armed := fighting && (len(t.Weapons) == 0 || slices.Contains(t.Weapons, worn)) && (!t.Servitor || b.liveServitor() != nil)
		switch {
		case on && armed && !below(s.MP, s.MaxMP, t.MPAbove),
			!on && (!armed || !above(s.MP, s.MaxMP, t.MPAbove)),
			b.toggleSettling(t.Skill, now) || !b.castReady(t.Skill, now):
			continue
		}
		return b.castOn(s.ObjectID, t.Skill, false, now), true
	}
	return Action{}, false
}

// relax is the cast of the kit's Relax at now in place of a plain sit
// (docs/strategy.md, Design section 9): known and ready, with MP not under
// SitMP (a bot resting on MP sits plainly). ok is false when the bot sits
// plainly; a hold while a Relax cast settles.
func (b *Brain) relax(now time.Time) (Action, bool) {
	s, bd := &b.world.Self, b.build()
	r := bd.Kit.Relax
	switch {
	case r == 0:
		return Action{}, false
	case b.toggleSettling(r, now):
		return Action{}, true
	case b.toggleOn(r) || !b.castReady(r, now) || below(s.MP, s.MaxMP, bd.Thresholds.SitMP):
		return Action{}, false
	}
	return b.castOn(s.ObjectID, r, false, now), true
}

// sitDown sits the bot to rest at now: its combat toggles off first (the
// server refuses a toggle while seated, SystemMessage 31), then Relax, else
// a plain sit; nothing while a cast is in flight.
func (b *Brain) sitDown(now time.Time) Action {
	if b.world.Self.Casting(now) || b.castPending(now) {
		return Action{}
	}
	if a, ok := b.toggleCast(b.build().Kit.Toggles, false, now); ok {
		return a
	}
	if a, ok := b.relax(now); ok {
		return a
	}
	return b.toggle(now)
}

// idleStep is the cast of the first Idle step of rot on the bot whose gates
// pass and whose skill is ready at now (Focused Force between fights).
func (b *Brain) idleStep(rot []Step, now time.Time) (Action, bool) {
	if !slices.ContainsFunc(rot, func(s Step) bool { return s.Idle }) {
		return Action{}, false
	}
	self := b.world.Self.ObjectID
	st := b.stepState(0, rot, now)
	for _, step := range rot {
		if step.Idle && step.Target == TargetSelf && step.passes(st) && b.castReady(step.Skill, now) {
			return b.castOn(self, step.Skill, false, now), true
		}
	}
	return Action{}, false
}

// effectsOf are patient id's effects and their levels: the bot's own, or a
// party member's; nil for neither.
func (b *Brain) effectsOf(id int32) (Buffs, map[int32]int32) {
	if id == b.world.Self.ObjectID {
		return b.world.Buffs, b.world.BuffLevels
	}
	if m := b.world.Party.Members[id]; m != nil {
		return m.Buffs, m.BuffLevels
	}
	return nil, nil
}

// buffDue reports whether the buff skill may go out on patient id at now
// under the stack rule (docs/strategy.md, Design section 4): only while
// the patient holds no effect of the skill's stack type at an equal or
// higher stack order, except the skill's own effect at its order with
// under RebuffBefore left, which it renews. A weaker effect of the type is
// upgraded, as the server replaces it. A skill without a known stack type
// (no travel map, no stacking effect) is due while its own effect is
// missing or ends within RebuffBefore.
func (b *Brain) buffDue(id, skill int32, now time.Time) bool {
	buffs, levels := b.effectsOf(id)
	before := b.persona.RebuffBefore
	m := b.cfg.Travel
	mine, ok := m.skillStackOf(skill, b.world.Skills[skill].Level)
	if !ok {
		return buffs.Due(skill, before, now)
	}
	for e := range buffs {
		st, ok := m.skillStackOf(e, levels[e])
		switch {
		case !ok || st.kind != mine.kind || st.order < mine.order:
			continue
		case st.order > mine.order, e != skill, !buffs.Due(skill, before, now):
			return false
		}
	}
	return true
}

// above reports whether cur is over pct percent of maxV; never for an
// unknown maximum.
func above(cur, maxV int32, pct int) bool {
	return maxV > 0 && int64(cur)*100 > int64(maxV)*int64(pct)
}
