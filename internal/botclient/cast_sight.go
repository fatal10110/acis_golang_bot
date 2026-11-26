package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// unseenState is a cast the server refused for sight (system message 181,
// msgCantSeeTarget): id its target, from where the bot stood and at where id
// stood then; once chosen, stand is the stand point the next cast at id
// walks to first, with purpose. It is owned by the brain goroutine.
type unseenState struct {
	id       int32
	from, at Point
	chosen   bool
	stand    Point
	purpose  walkPurpose
}

// sightRefused records the cast in flight refused 181 at now: its target is
// out of the server's sight from where the bot stands (network/magic_skill.go
// CanCastSighted, ErrCantSeeTarget: the caster's sight of a target past a
// castRange > 0 skill), so the next cast at it is asked from elsewhere
// (sightWalk). A cast at the bot itself is never refused for sight.
func (b *Brain) sightRefused(now time.Time) {
	o := b.world.object(b.castObject)
	if o == nil || b.castObject == b.world.Self.ObjectID {
		return
	}
	b.unseen = unseenState{id: b.castObject, from: b.world.Self.Position(now), at: o.Position(now)}
}

// sightWalk is the walk the next cast of skill at id takes after the
// server refused one at it for sight from where the bot still stands
// (sightRefused): to a stand point in the bot's own sight of id within the
// cast's range, or, where the bot's geodata sees id from where it stands
// though the server did not, to a melee stand point with the straight line
// to id open, so the bot approaches instead of asking the same cast from the
// same spot every castRefusedHold. It reports false when the bot may cast:
// no such refusal, the bot or id moved off since, the walk arrived or found
// no way, the bot is immobile, or without geodata (the refused skill's
// castRefusedHold alone paces the retry then). It walks as engageWalk does:
// one segment at a time through goTo.
func (b *Brain) sightWalk(id, skill int32, now time.Time) (Action, bool) {
	u := &b.unseen
	if u.id == 0 || u.id != id {
		return Action{}, false
	}
	w := b.walker()
	o := b.world.object(id)
	if w.nav == nil || o == nil || b.immobile(now) {
		*u = unseenState{}
		return Action{}, false
	}
	from, to := b.world.Self.Position(now), o.Position(now)
	if horizontal2(to, u.at) > walkSameDest*walkSameDest ||
		!u.chosen && horizontal2(from, u.from) > walkSameDest*walkSameDest {
		// The sight the server refused is not the one the cast would be
		// asked with now.
		*u = unseenState{}
		return Action{}, false
	}
	if !u.chosen {
		r, ok := b.castReach(skill)
		if !ok {
			*u = unseenState{}
			return Action{}, false
		}
		mode, rng := r.stand()
		if w.sightOpen(from, to, defaultTargetHeight) {
			mode, rng = nav.StandLine, meleeStand
		}
		st, ok := w.standPoint(from, to, rng, mode, defaultTargetHeight)
		if !ok {
			*u = unseenState{}
			return Action{}, false
		}
		u.stand, u.chosen, u.purpose = st, true, walkRanged
		if mode == nav.StandLine {
			u.purpose = walkMelee
		}
	}
	a, st := b.goTo(u.stand, walkSlop, u.purpose, now)
	if st != walkWalking {
		*u = unseenState{}
		return Action{}, false
	}
	return a, true
}
