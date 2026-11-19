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
	if m := w.motionOf(e.AttackerID); m != nil {
		*m = m.resample(now, e.Pos)
	}
	if e.AttackerID == w.Self.ObjectID {
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
// the bot by a monster, or by a player who is flagged or has karma, counts
// as a hit: MagicSkillUse does not tell an attack from a buff, and only
// those casters can attack the bot without a forced attack.
func (w *World) applyMagicSkillUse(e MagicSkillUse, now time.Time) {
	if e.CasterID == w.Self.ObjectID {
		w.Self.castEnd = now.Add(e.HitTime)
		w.holdSkill(e.SkillID, now.Add(e.HitTime+e.Reuse))
		return
	}
	if e.TargetID != w.Self.ObjectID {
		return
	}
	if o := w.object(e.CasterID); o != nil && hostileCaster(o) {
		w.hitBy(e.CasterID, 0, now)
	}
}

func hostileCaster(o *Object) bool {
	if o.Kind == KindNPC {
		return o.Attackable
	}
	return flagged(o)
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
