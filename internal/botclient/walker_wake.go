package botclient

import "time"

// walkWake is when the walk's next decision is due, the walk wake
// (docs/walking.md, "Walk wake"): one position tick past the modeled end of
// the bot's own move, offered while the walk is walking with a segment in
// flight or the last decision called goTo, and only while that time is later
// than the last walk wake. It is derived from the bot's own motion and
// changes nothing: an end that moves later (a resample, a server re-time, the
// answer to a new segment) arms it again, a fired one is never offered again.
func (b *Brain) walkWake() (time.Time, bool) {
	w := &b.walk
	if w.status != walkWalking || w.seg.sentAt.IsZero() && !w.asked {
		return time.Time{}, false
	}
	due := b.world.Self.arrival().Add(positionTick)
	return due, due.After(w.wokeAt)
}

// spendWalkWake fires the walk wake at now, the decision gate passed: once
// now is at or past one tick after the bot's own move's end, that end is
// spent whether or not the decision reaches goTo, so decisionAt never stays
// in the past.
func (b *Brain) spendWalkWake(now time.Time) {
	if !now.Before(b.world.Self.arrival().Add(positionTick)) {
		b.walk.wokeAt = now
	}
}
