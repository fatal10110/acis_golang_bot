package botclient

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Necromancer corpses (docs/servitors.md section 6): a monster's corpse
// takes a CORPSE_MOB skill only while it is younger than half its
// template's corpse time, checked when the cast commits
// (handler/target/target_predicates.go corpseTooOld; a spoiled or seeded
// corpse is exempt, target_handlers_corpse.go corpseMobCastRejection), and
// the skill consumes it on landing (cast/effects.go). The corpse rule casts
// the row's corpse summon, drain or Corpse Burst on a corpse that falls in
// reach; it never closes in to make one (owner answer 4, D51).

const (
	// corpseMargin is the corpse rule's safety margin on a corpse's window
	// (docs/servitors.md section 6, CorpseMargin): the cast must commit at
	// least this long before the window ends.
	corpseMargin = 500 * time.Millisecond
	// corpseFrameDelay is a cast request's way to the server: a wait in
	// the frame bucket plus the trip.
	corpseFrameDelay = 250 * time.Millisecond
	// corpseBurstRadius is Corpse Burst's skillRadius (skills/1100-1199.xml
	// 1155) when the travel map lacks it.
	corpseBurstRadius = 200
	// msgCorpseTooOld is the server's refusal of a CORPSE_MOB cast at a
	// corpse past half its corpse time (serverpackets/systemmessage.go
	// SystemMessageCorpseTooOldSkillNotUsed, network/magic_skill.go).
	msgCorpseTooOld = 1247
)

// CorpseKit are a row's CORPSE_MOB casts (docs/servitors.md section 6):
// Summons its corpse summons, its own first (1334 Summon Cursed Man, then
// 1129 Summon Reanimated Man; never 1154); Drain the corpse drain (1151
// Corpse Life Drain, the Dark Avenger's 46 Life Scavenge) and Burst the
// corpse area nuke (1155 Corpse Burst), 0 for none. The zero value for a
// row without corpse skills: the corpse rule tracks nothing for it.
type CorpseKit struct {
	Summons      []int32
	Drain, Burst int32
}

// any reports whether the row casts on corpses at all.
func (k CorpseKit) any() bool { return len(k.Summons) > 0 || k.Drain != 0 || k.Burst != 0 }

// has reports whether skill is one of the row's corpse casts.
func (k CorpseKit) has(skill int32) bool {
	return skill != 0 && (skill == k.Drain || skill == k.Burst || slices.Contains(k.Summons, skill))
}

// corpse is a monster's corpse in sight the corpse rule may cast on: where
// it lies and when its CORPSE_MOB window (half its template's corpse time
// from its death) closes.
type corpse struct {
	pos    Point
	cutoff time.Time
}

// corpseEvent keeps Brain.mobCorpses for a row with corpse casts: a monster's
// Die opens its corpse's window, unless the corpse is spoiled (Die.Sweepable:
// the spoiler's or its party's sweep comes first) or the bot spoiled it
// (spoilMine, spoilOther: run before spoilEvent forgets the mark); the
// server's refusal of a corpse cast in flight for the corpse's age (1247)
// or as no corpse (109), and the bot's own corpse cast (its MagicSkillUse:
// the skill consumes the corpse on landing), drop it; a DeleteObject
// forgets it.
func (b *Brain) corpseEvent(ev Event, now time.Time) {
	switch e := ev.(type) {
	case Die:
		kit := b.build().Kit.Corpse
		if !kit.any() || e.Sweepable {
			return
		}
		if m, ok := b.spoils[e.ObjectID]; ok && m.state != spoilResisted {
			return
		}
		o := b.world.object(e.ObjectID)
		if o == nil || o.Kind != KindNPC || !o.Attackable || o.Summon {
			return
		}
		// A template with no corpse time leaves its corpse (hostile_rewards.go
		// scheduleDecay): too rare to plan on, so it is not tracked.
		ct := b.cfg.Travel.corpseTime(o.NPCID)
		if ct <= 0 {
			return
		}
		b.pruneCorpses(now)
		if b.mobCorpses == nil {
			b.mobCorpses = make(map[int32]corpse)
		}
		b.mobCorpses[e.ObjectID] = corpse{pos: o.Position(now), cutoff: now.Add(ct / 2)}
	case SystemMessage:
		if (e.ID == msgCorpseTooOld || e.ID == msgInvalidTarget) && b.castPending(now) &&
			b.build().Kit.Corpse.has(b.castSkill) {
			delete(b.mobCorpses, b.castObject)
			if e.ID == msgCorpseTooOld {
				// castRefused does not end the cast on 1247: nothing is in
				// flight and the skill started no reuse, so the next corpse
				// may be tried at once. Only the send's own hold is lifted.
				if until := b.castAt.Add(castTimeout); b.world.skillReady[b.castSkill].Equal(until) {
					delete(b.world.skillReady, b.castSkill)
				}
				b.castAt = time.Time{}
			}
		}
	case MagicSkillUse:
		if e.CasterID == b.world.Self.ObjectID && b.build().Kit.Corpse.has(e.SkillID) {
			delete(b.mobCorpses, e.TargetID)
		}
	case DeleteObject:
		delete(b.mobCorpses, e.ObjectID)
	}
}

// pruneCorpses forgets the corpses whose window ended at now or that left
// sight.
func (b *Brain) pruneCorpses(now time.Time) {
	for id, c := range b.mobCorpses {
		if o := b.world.object(id); o == nil || !o.Dead || !now.Before(c.cutoff) {
			delete(b.mobCorpses, id)
		}
	}
}

// ruleCorpse is the corpse rule, right after "sweep" (docs/servitors.md
// section 6): the row's corpse summon on the nearest fresh corpse in reach
// while it wants a servitor (none stands: a dead one stays until its
// PetDelete, and the server refuses a second with 580) and carries the
// crystals; else its corpse drain under its sustain bound; else Corpse
// Burst on a corpse with two or more of its attackers around it. Its casts
// take 1.5 s, so it casts in a fight too. It stands up, waits out a cast in
// flight and selects the corpse; the server walks the bot into cast range
// (magic_skill.go walkToCastTarget), which the reach test (corpseReach)
// times inside the window.
//
// It sends Action and RequestMagicSkillUse through the frame bucket, one
// cast at a time: each waits for its MagicSkillUse or castTimeout, then the
// skill's 20 s reuse; a corpse refused for its age or as gone (1247, 109)
// is dropped, so no corpse is asked twice for those.
func (b *Brain) ruleCorpse(now time.Time) (Action, bool) {
	if len(b.mobCorpses) == 0 {
		return Action{}, false
	}
	b.pruneCorpses(now)
	skill, id := b.corpseCast(now)
	if id == 0 {
		return Action{}, false
	}
	s := &b.world.Self
	switch {
	case s.Sitting:
		return b.toggle(now), true
	case s.Casting(now) || b.castPending(now):
		return Action{}, true
	case b.world.Target != id:
		return selectAction(id, s.Position(now)), true
	}
	return Action{kind: actCast, payload: botwire.EncodeRequestMagicSkillUse(skill, false), object: id, skill: skill}, true
}

// corpseCast is the corpse cast the rule makes at now and its corpse, 0 and
// 0 for none: the summon, else the drain, else the burst.
func (b *Brain) corpseCast(now time.Time) (skill, id int32) {
	kit := b.build().Kit.Corpse
	if len(kit.Summons) > 0 && b.world.Servitor == nil {
		if ref := b.summonFrom(kit.Summons); ref.ID != 0 && b.corpseReady(ref.ID, now) {
			if id := b.corpseInReach(ref.ID, nil, now); id != 0 {
				return ref.ID, id
			}
		}
	}
	s := &b.world.Self
	if kit.Drain != 0 && below(s.HP, s.MaxHP, b.drainBound()) && b.corpseReady(kit.Drain, now) {
		if id := b.corpseInReach(kit.Drain, nil, now); id != 0 {
			return kit.Drain, id
		}
	}
	if kit.Burst != 0 && b.corpseReady(kit.Burst, now) {
		if id := b.corpseInReach(kit.Burst, b.burstFits(kit.Burst), now); id != 0 {
			return kit.Burst, id
		}
	}
	return 0, 0
}

// corpseReady reports whether the bot may cast corpse skill at now: it
// knows it, the skill is enabled and ready with its reagent, and the bot
// has the MP of it.
func (b *Brain) corpseReady(skill int32, now time.Time) bool {
	k, known := b.world.Skills[skill]
	if !known || k.Disabled || !b.castReady(skill, now) {
		return false
	}
	sk, ok := b.skillData(skill)
	return ok && int(b.world.Self.MP) >= sk.MPInitialConsume+sk.MPConsume
}

// drainBound is the row's sustain bound, the HP percent under which it
// drains a corpse: the highest SelfHPBelow of its heal and drain steps
// (critSustain), 0 for none.
func (b *Brain) drainBound() int {
	var pct int
	for _, st := range b.build().Kit.Rotation {
		if slices.Contains(critSustain, st.Skill) {
			pct = max(pct, st.SelfHPBelow)
		}
	}
	return pct
}

// corpseInReach is the nearest corpse (ties to the lower object id) a cast
// of skill reaches in time at now (corpseReach) that fits passes (nil for
// any), 0 for none.
func (b *Brain) corpseInReach(skill int32, fits func(id int32, c corpse, now time.Time) bool, now time.Time) int32 {
	from := b.world.Self.Position(now)
	var best int32
	var bestD float64
	for id, c := range b.mobCorpses {
		if !b.corpseReach(id, c, skill, now) || fits != nil && !fits(id, c, now) {
			continue
		}
		d := dist3(from, c.pos)
		if best == 0 || d < bestD || d == bestD && id < best {
			best, bestD = id, d
		}
	}
	return best
}

// corpseReach reports whether the bot's cast of skill on corpse c (object
// id) commits inside its window at now (docs/servitors.md section 6,
// "Reach"): the walk into cast range, distance − cast range − both
// collision radii, fits in what is left of the window less corpseMargin
// and corpseFrameDelay at the bot's speed.
func (b *Brain) corpseReach(id int32, c corpse, skill int32, now time.Time) bool {
	left := c.cutoff.Sub(now) - corpseMargin - corpseFrameDelay
	if left <= 0 {
		return false
	}
	r, approach := b.castReach(skill)
	if !approach {
		return true
	}
	s := &b.world.Self
	gap := dist3(s.Position(now), c.pos) - r.rng - s.CollisionRadius - b.radiusOf(id)
	if gap <= 0 {
		return true
	}
	return gap <= float64(b.world.speedOf(s.ObjectID))*left.Seconds()
}

// burstFits is the Corpse Burst test of a corpse: two or more of the bot's
// attackers stand within skill's radius of it, and nobody else it could
// harm does (a monster not attacking the bot, a player bystander), so the
// burst pulls nothing new.
func (b *Brain) burstFits(skill int32) func(id int32, c corpse, now time.Time) bool {
	radius := float64(corpseBurstRadius)
	if sk, ok := b.skillData(skill); ok && sk.Radius > 0 {
		radius = float64(sk.Radius)
	}
	return func(id int32, c corpse, now time.Time) bool {
		attackers := b.attackers(now)
		n := 0
		for oid, o := range b.world.Objects {
			if oid == id || o.Dead || dist3(c.pos, o.Position(now)) > radius+b.radiusOf(oid) {
				continue
			}
			switch {
			case slices.Contains(attackers, oid):
				n++
			case o.Kind == KindNPC && o.Attackable:
				return false
			}
		}
		return n >= 2 && !b.bystander(id, c.pos, radius, nil, now)
	}
}

// summonFrom is the first summon of list the bot can cast now: one it
// knows, of a SUMMON skill of an XP penalty under expPenaltyMax, whose
// crystals it carries for the cast plus one upkeep step (summonAffords).
// The zero value for none or without a travel map.
func (b *Brain) summonFrom(list []int32) datapack.SkillRef {
	m := b.cfg.Travel
	if m == nil {
		return datapack.SkillRef{}
	}
	for _, id := range list {
		k, ok := b.world.Skills[id]
		if !ok {
			continue
		}
		ref := datapack.SkillRef{ID: id, Level: k.Level}
		sk, ok := m.skills[ref]
		if !ok || sk.Type != "SUMMON" || sk.ExpPenalty >= expPenaltyMax || !b.summonAffords(sk) {
			continue
		}
		return ref
	}
	return datapack.SkillRef{}
}
