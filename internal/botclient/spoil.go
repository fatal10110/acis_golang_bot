package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Dwarven spoil and sweep (docs/strategy.md, Design section 8): a kit's Spoil
// step opens on a fresh target (Step.Spoil); the server's answer to the cast
// marks the monster (spoilEvent); after the kill of a monster the bot
// spoiled, ruleSweep casts the kit's Sweep on its corpse while the corpse
// lasts, before ruleLoot picks anything up. Swept items go straight to the
// bot's inventory (or its party's, by the loot rule), where the sell and
// keep logic of the shop errand already handles them.

// System messages of Spoil and Sweeper (serverpackets/systemmessage.go).
// Spoil's come once its cast lands (handler/skill/spoil.go: 612 on success,
// 357 on a monster anyone already spoiled, the bot included, 139 on a failed
// roll, the generic message of any resisted skill); Sweeper's 343 and 683
// refuse the cast with no MagicSkillUse (handler/target/cast_rejection.go,
// network/magic_skill.go).
const (
	msgResisted        = 139
	msgSweepNotSpoiled = 343
	msgAlreadySpoiled  = 357
	msgSpoilSuccess    = 612
	msgSweepNotAllowed = 683
)

// defaultCorpseTime is how long a corpse lasts when its template sets no
// corpseTime (datapack npc.go, the server's default).
const defaultCorpseTime = 7 * time.Second

// spoilState is what the bot knows of a monster's spoil.
type spoilState uint8

const (
	// spoilResisted: the bot's last Spoil on it was resisted (139); a Spoil
	// step casts again at the skill's reuse while it lives.
	spoilResisted spoilState = iota + 1
	// spoilMine: spoiled by the bot (612); never spoiled again, swept once
	// dead.
	spoilMine
	// spoilOther: spoiled by another player (357 with no 612 of the bot's);
	// never spoiled again nor swept.
	spoilOther
)

// spoilMark is one monster's entry of Brain.spoils.
type spoilMark struct {
	state spoilState
	// sweepUntil is when the sweep window of a corpse the bot spoiled
	// ends: twice its template's corpseTime after its death (the server
	// doubles a spoiled corpse's decay, npc/hostile_rewards.go). Zero
	// while the monster lives.
	sweepUntil time.Time
}

// spoilable reports whether a Spoil step may cast on id: no spoil of it is
// known and it is fresh (Step.Opener), or the bot's last Spoil on it was
// resisted.
func (b *Brain) spoilable(id int32, fresh bool) bool {
	m, ok := b.spoils[id]
	if !ok {
		return fresh
	}
	return m.state == spoilResisted
}

// markSpoil records state for the monster id.
func (b *Brain) markSpoil(id int32, state spoilState) {
	if b.spoils == nil {
		b.spoils = make(map[int32]spoilMark)
	}
	b.spoils[id] = spoilMark{state: state}
}

// spoilEvent keeps Brain.spoils: a Spoil verdict marks the monster of the
// Spoil cast in flight (spoilTarget, castSkill), Sweeper's own refusals
// (343, 683) or the bot's own Sweeper cast end the corpse's sweep, a cost
// or target refusal of the Sweeper cast in flight holds the sweep rule
// (sweepHeld), and a death opens the
// sweep window of a corpse the bot spoiled with spoil items to sweep
// (Die.Sweepable) and forgets any other. A DeleteObject forgets the
// monster where the brain reads it.
func (b *Brain) spoilEvent(ev Event, now time.Time) {
	switch e := ev.(type) {
	case SystemMessage:
		id := b.spoilTarget
		if id == 0 {
			return
		}
		switch b.castSkill {
		case skillSpoil:
			cur, known := b.spoils[id]
			switch e.ID {
			case msgSpoilSuccess:
				b.markSpoil(id, spoilMine)
			case msgAlreadySpoiled:
				if cur.state != spoilMine {
					b.markSpoil(id, spoilOther)
				}
			case msgResisted:
				if !known {
					b.markSpoil(id, spoilResisted)
				}
			default:
				return
			}
			b.spoilTarget = 0
		case skillSweeper:
			switch e.ID {
			case msgSweepNotSpoiled, msgSweepNotAllowed:
				delete(b.spoils, id)
				b.castAt, b.spoilTarget = time.Time{}, 0
			case msgNotEnoughHP, msgNotEnoughMP, msgInvalidTarget, msgCannotBeUsed, msgCantSeeTarget:
				// castRefused holds Sweeper for castRefusedHold; the
				// corpse stays while its window lasts.
				if b.castPending(now) {
					b.sweepHeld, b.spoilTarget = now.Add(castRefusedHold), 0
				}
			}
		}
	case MagicSkillUse:
		// The server sweeps a corpse once and resets its spoil
		// (handler/skill/spoil.go).
		if e.CasterID == b.world.Self.ObjectID && e.SkillID == skillSweeper {
			delete(b.spoils, e.TargetID)
		}
	case Die:
		m, ok := b.spoils[e.ObjectID]
		switch {
		case !ok:
		case m.state != spoilMine || !e.Sweepable:
			delete(b.spoils, e.ObjectID)
		default:
			var npc int32
			if o := b.world.object(e.ObjectID); o != nil {
				npc = o.NPCID
			}
			m.sweepUntil = now.Add(2 * b.cfg.Travel.corpseTime(npc))
			b.spoils[e.ObjectID] = m
		}
	}
}

// sweepTarget is the corpse whose sweep window closes first at now (ties to
// the lower object id), 0 for none; corpses whose window ended are
// forgotten.
func (b *Brain) sweepTarget(now time.Time) int32 {
	var id int32
	var until time.Time
	for oid, m := range b.spoils {
		switch {
		case m.sweepUntil.IsZero():
		case !now.Before(m.sweepUntil):
			delete(b.spoils, oid)
		case id == 0 || m.sweepUntil.Before(until) || m.sweepUntil.Equal(until) && oid < id:
			id, until = oid, m.sweepUntil
		}
	}
	return id
}

// ruleSweep is the sweep rule, right after "outnumbered" (docs/strategy.md,
// Design section 8): the kit's Sweep (42 Sweeper) on the corpse of a monster
// the bot spoiled, while its sweep window lasts and before ruleLoot picks
// anything up. It stands up, waits out a cast in flight and the skill's
// reuse, selects the corpse, walks into cast range where the straight line
// is closed (engageWalk), then casts. It yields while the skill is disabled
// or a refusal of its last cast holds it (sweepHeld), so the bot rests or
// fights meanwhile instead of idling.
//
// It sends Action and RequestMagicSkillUse through the frame bucket, one
// cast at a time: each waits for its MagicSkillUse or castTimeout, then the
// skill's reuse.
func (b *Brain) ruleSweep(now time.Time) (Action, bool) {
	if len(b.spoils) == 0 {
		return Action{}, false
	}
	skill := b.build().Kit.Sweep
	if k, known := b.world.Skills[skill]; skill == 0 || !known || k.Disabled || now.Before(b.sweepHeld) {
		return Action{}, false
	}
	id := b.sweepTarget(now)
	if id == 0 {
		return Action{}, false
	}
	s := &b.world.Self
	switch {
	case s.Sitting:
		return b.toggle(now), true
	case s.Casting(now) || b.castPending(now) || !b.world.SkillReady(skill, now):
		return Action{}, true
	case b.world.Target != id:
		return selectAction(id, s.Position(now)), true
	}
	if r, approach := b.castReach(skill); approach {
		if a, walk := b.engageWalk(id, r, now); walk {
			return a, true
		}
	}
	return Action{kind: actCast, payload: encodeRequestMagicSkillUse(skill, false), object: id, skill: skill}, true
}

// indexCorpseTimes are the corpse times of the NPC templates whose
// corpseTime is not defaultCorpseTime.
func indexCorpseTimes(npcs map[int32]datapack.NPC) map[int32]time.Duration {
	out := make(map[int32]time.Duration)
	for id, n := range npcs {
		if n.CorpseTime != defaultCorpseTime {
			out[id] = n.CorpseTime
		}
	}
	return out
}

// corpseTime is how long a corpse of NPC template npcID lasts: its
// template's corpseTime, defaultCorpseTime when the map lacks it.
func (m *TravelMap) corpseTime(npcID int32) time.Duration {
	if m != nil {
		if d, ok := m.corpseTimes[npcID]; ok {
			return d
		}
	}
	return defaultCorpseTime
}
