package botclient

import "time"

// The anti-KS model (plan, "Shared behavior"): the client sees a monster
// fight only through MoveToPawn (the monster chasing someone, or someone
// closing in on it), Attack and MagicSkillUse, so who engaged a monster is
// inferred from those. Aggro held before any of them is invisible, which
// makes this a heuristic.

// monster reports whether id is an attackable NPC in sight that is alive.
func (w *World) monster(id int32) bool {
	o := w.object(id)
	return o != nil && o.Kind == KindNPC && o.Attackable && !o.Dead
}

// engage records at now that a and b were seen fighting, when one of them
// is a monster and the other is not: the other engaged the monster. The
// bot's own fights are not recorded.
func (w *World) engage(a, b int32, now time.Time) {
	m, by := a, b
	if !w.monster(m) {
		m, by = b, a
	}
	if by == 0 || by == w.Self.ObjectID || !w.monster(m) || w.monster(by) {
		return
	}
	e := w.Engaged[m]
	if e == nil {
		e = make(map[int32]time.Time)
		w.Engaged[m] = e
	}
	e[by] = now
}

// engagedByOther reports whether someone outside the bot's party engaged
// monster id within window of now.
func (w *World) engagedByOther(id int32, window time.Duration, now time.Time) bool {
	p := &w.Party
	for by, at := range w.Engaged[id] {
		if now.Sub(at) > window || by == p.LeaderID || p.Members[by] != nil {
			continue
		}
		return true
	}
	return false
}
