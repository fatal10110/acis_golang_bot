package botclient

import "time"

// walkWake is when the walk's next decision is due, the walk wake
// (docs/walking.md, "Walk wake"): one position tick past the modeled end of
// the bot's own move, offered while the walk is walking with a segment in
// flight or the last decision called goTo, and only while that time is later
// than the last walk wake. A segment a click-ahead may follow wakes first
// when the click is due (Brain.aheadAt), then at the end as before. It is
// derived from the bot's own motion and changes nothing: an end that moves
// later (a resample, a server re-time, the answer to a new segment) arms it
// again, a fired one is never offered again. A walk held after a desync
// wakes at the hold's end instead (Brain.holdEnd), which the next leg of the
// server's route moves later.
func (b *Brain) walkWake() (time.Time, bool) {
	w := &b.walk
	if w.status != walkWalking || w.seg.sentAt.IsZero() && !w.asked {
		return time.Time{}, false
	}
	if at, ok := b.aheadAt(); ok && at.After(w.wokeAt) {
		return at, true
	}
	due := b.walkDue()
	return due, due.After(w.wokeAt)
}

// walkDue is the walk wake's time at the end of the bot's own move: one
// position tick past it, or the hold's end while the walk is held.
func (b *Brain) walkDue() time.Time {
	if b.walk.held {
		return b.holdEnd()
	}
	return b.world.Self.arrival().Add(positionTick)
}

// spendWalkWake fires the walk wake at now, the decision gate passed: once
// now is at or past its due time (walkDue), or past a click-ahead's time,
// that time is spent whether or not the decision reaches goTo, so
// decisionAt never stays in the past.
func (b *Brain) spendWalkWake(now time.Time) {
	at, ahead := b.aheadAt()
	if ahead && !now.Before(at) || !now.Before(b.walkDue()) {
		b.walk.wokeAt = now
	}
}
