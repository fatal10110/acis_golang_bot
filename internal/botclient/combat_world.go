package botclient

import "time"

// attackerWindow is how long a creature that hit the bot counts as its
// attacker after its last hit, and the longest gap between two hits of one
// streak whose damage adds up.
const attackerWindow = 10 * time.Second

// Attacker is a creature that hit the bot recently.
type Attacker struct {
	// Damage is what its current streak of hits dealt: hits no more than
	// attackerWindow apart. A hostile cast counts as a hit of 0.
	Damage int64
	// Last is its last hit.
	Last time.Time
}

// Casting reports whether the character's own cast is still in progress at
// now.
func (s *SelfState) Casting(now time.Time) bool { return now.Before(s.castEnd) }

// SkillReady reports whether the character knows the active skill id, has
// it enabled, and its reuse is over at now.
func (w *World) SkillReady(id int32, now time.Time) bool {
	s, ok := w.Skills[id]
	return ok && !s.Passive && !s.Disabled && !now.Before(w.skillReady[id])
}

// holdSkill keeps skill id from being ready before until.
func (w *World) holdSkill(id int32, until time.Time) {
	if until.After(w.skillReady[id]) {
		w.skillReady[id] = until
	}
}

// attacking reports whether id hit the bot within attackerWindow of now.
func (w *World) attacking(id int32, now time.Time) bool {
	a := w.Attackers[id]
	return a != nil && now.Sub(a.Last) <= attackerWindow
}

// hitBy records a hit of damage on the bot by id at now.
func (w *World) hitBy(id int32, damage int32, now time.Time) {
	for k, a := range w.Attackers {
		if now.Sub(a.Last) > attackerWindow {
			delete(w.Attackers, k)
		}
	}
	a := w.Attackers[id]
	if a == nil {
		a = &Attacker{}
		w.Attackers[id] = a
	}
	a.Damage += int64(max(damage, 0))
	a.Last = now
}

// applyAttack records every hit on the bot, misses included, against the
// attacker, and places the attacker where it swings from.
func (w *World) applyAttack(e Attack, now time.Time) {
	// The server stops an attacker before it swings (ai/player.go,
	// ai/attackable.go: move.Stop before DoAttack), and Pos is where it
	// stands then (attack/controller.go snapshot): exact, unlike a walk's
	// dead-reckoned end, which a pawn walk ending on its first step in range
	// misses by up to a step. The walker's next segment goes out from it.
	if m := w.motionOf(e.AttackerID); m != nil {
		*m = stopAt(e.Pos)
	}
	if e.AttackerID == w.Self.ObjectID {
		w.Self.swingEnd = now.Add(swingTime(w.Self.PAtkSpd))
		return
	}
	for _, h := range e.Hits {
		if h.TargetID == w.Self.ObjectID {
			w.hitBy(e.AttackerID, h.Damage, now)
		}
	}
}

// applyMagicSkillUse starts the bot's own cast and its skill's reuse: the
// skill is ready again hit time plus reuse after the cast starts. A cast at
// the bot by a monster counts as a hit; a player's or a summon's counts only
// when its skill is offensive, which the brain tells from the datapack
// (Brain.castSeen).
//
// Another caster stands where it casts from when the hit time proves the
// server stopped it first; the bot's own cast is stood by the brain, which
// knows whether it asked for it (Brain.standCaster).
func (w *World) applyMagicSkillUse(e MagicSkillUse, now time.Time) {
	if e.CasterID == w.Self.ObjectID {
		w.Self.castEnd = now.Add(e.HitTime)
		w.holdSkill(e.SkillID, now.Add(e.HitTime+e.Reuse))
		return
	}
	// A monster, folk NPC, summon, pet or player stops before a cast whose
	// template hit time is over 50 ms (ai/attackable.go thinkCast,
	// npc/folk_cast.go castOn, ai/summon.go thinkCastLocked,
	// cast/request.go stopForCast), and CasterPos is where it stands when the
	// broadcast goes out (BroadcastSkillUse reads Position then). castStopsFrom
	// is the hit time in the packet that proves the template one. Script
	// casts (cast/ai_call_skill.go CallSkill, CastUnchecked) do not stop and
	// the packet cannot tell them apart: the caster's next move corrects it.
	if o := w.object(e.CasterID); o != nil && e.HitTime >= castStopsFrom {
		o.move = stopAt(e.CasterPos)
	}
	if e.TargetID != w.Self.ObjectID {
		return
	}
	if o := w.object(e.CasterID); o != nil && o.Kind == KindNPC && !o.Summon && o.Attackable {
		w.hitBy(e.CasterID, 0, now)
	}
}

// flagged reports whether a player is PvP flagged or has karma: the players
// anyone may attack without a forced attack.
func flagged(o *Object) bool {
	return o.Kind == KindPlayer && (o.PvPFlag != 0 || o.Karma > 0)
}

// applySkillList replaces the skill set.
func (w *World) applySkillList(e SkillList) {
	clear(w.Skills)
	for _, s := range e.Skills {
		w.Skills[s.ID] = s
	}
}

// swingTime is how long a melee swing at attack speed pAtkSpd lasts: the
// server's formulas.TimeBetweenAttacks, max(100, 500000/pAtkSpd) ms
// (attack/controller.go). An attack speed not known yet (no UserInfo) takes
// slowSwing, longer than any usual swing.
func swingTime(pAtkSpd int32) time.Duration {
	if pAtkSpd <= 0 {
		return slowSwing
	}
	return max(100*time.Millisecond, 500*time.Second/time.Duration(pAtkSpd))
}

// slowSwing is the swing of an unknown attack speed: 500000/200 ms.
const slowSwing = 2500 * time.Millisecond
