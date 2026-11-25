package botclient

import "time"

// storeHold is the count of a pending round's hold on the store
// (docs/valuables/round.md "The store waits for a pending round"), owned
// by the brain goroutine: the town it runs in and the first decision that
// held the store there, zero for none. A running look pauses it from
// paused on.
type storeHold struct {
	town   int
	since  time.Time
	paused time.Time
}

// pause stops a running count at now while the look runs.
func (h *storeHold) pause(now time.Time) {
	if !h.since.IsZero() && h.paused.IsZero() {
		h.paused = now
	}
}

// resume restarts a paused count at now, the pause not counted.
func (h *storeHold) resume(now time.Time) {
	if !h.paused.IsZero() {
		h.since = h.since.Add(now.Sub(h.paused))
		h.paused = time.Time{}
	}
}

// trackHold resets the count once town finds no town or another one where
// the bot stands; during the rule's own keeper visits and walks back the
// anchor's town counts.
func (b *Brain) trackHold(now time.Time) {
	h := &b.held
	if h.since.IsZero() {
		return
	}
	at := b.world.Self.Position(now)
	switch v := b.cfg.valuables; {
	case b.look.phase != lookIdle:
		at = b.look.anchor
	case v.round.open:
		at = v.round.anchor
	case b.round.phase != roundIdle:
		at = b.round.anchor
	}
	if t, ok := b.cfg.Stores.town(at); !ok || t != h.town {
		*h = storeHold{}
	}
}

// roundPending reports whether a round is pending at now: roundDue's
// conditions all hold but its short waits (the ad board, the open, the
// errand checks, the look not done), and whether it waits on timers only,
// no errand running. Within RoundGap of the latest start, with the budget
// spent or no valuable carried none is.
func (b *Brain) roundPending(now time.Time) (pending, timers bool) {
	v, p := b.cfg.valuables, b.persona.Valuables
	if p == (ValuablePolicy{}) || b.cfg.Travel == nil || v.round.open || b.round.phase != roundIdle {
		return false, false
	}
	if !v.lastStart.IsZero() && now.Sub(v.lastStart) < p.RoundGap || b.valuablesLeft() <= 0 || !v.carried() || !b.tradeReady(now) {
		return false, false
	}
	at := b.world.Self.Position(now)
	if _, ok := b.cfg.Stores.town(at); !ok || !b.privateKeeper(at) || b.cfg.Travel.restartRegion(at.X, at.Y) < 0 {
		return false, false
	}
	return true, !b.errandRunning()
}

// holding reports whether the hold of a pending round still runs at now:
// under StoreHold from the first decision it held the store in this town,
// which it records.
func (b *Brain) holding(now time.Time) bool {
	h := &b.held
	if h.since.IsZero() {
		t, _ := b.cfg.Stores.town(b.world.Self.Position(now))
		*h = storeHold{town: t, since: now}
	}
	return now.Before(h.since.Add(b.persona.Valuables.StoreHold))
}

// storeHeld reports whether a pending round keeps the store from opening
// at now: for StoreHold at most, then the store opens and the valuables
// wait for it to close or the round to be ready.
func (b *Brain) storeHeld(now time.Time) bool {
	pending, _ := b.roundPending(now)
	return pending && b.holding(now)
}

// holdTurn reports whether rule "valuables" holds the turn at now for a
// round pending on timers only (the board, the open), so no later rule
// walks the bot away, StoreHold at most. Pending on an errand it lets the
// errand's rule step it.
func (b *Brain) holdTurn(now time.Time) bool {
	pending, timers := b.roundPending(now)
	return pending && b.holding(now) && timers
}

// ownTradeRequest reports whether the bot's own trade request awaits its
// answer at now: sent under tradeRequestTimeout ago, no TradeStart or
// refusal since (inviteEvent). The server refuses a store's set-up
// meanwhile (network/privatestore.go storeOpenState, ProcessingRequest).
func (b *Brain) ownTradeRequest(now time.Time) bool {
	r := b.round.request
	return !r.IsZero() && now.Sub(r) < tradeRequestTimeout
}

// offRoundDue reports whether the budget-spent deposit runs at now
// (docs/valuables/round-end.md "Off-round errands"): the policy enabled,
// the session's look done, the valuables budget spent, valuables carried
// with one picked up since the last deposit tried (newFind), and the
// look's conditions (lookReady). The ad board never holds it.
func (b *Brain) offRoundDue(now time.Time) bool {
	if b.persona.Valuables == (ValuablePolicy{}) || b.cfg.Travel == nil || !b.wh.private.Known || b.look.phase != lookIdle {
		return false
	}
	if b.valuablesLeft() > 0 {
		return false
	}
	return b.cfg.valuables.newFind(b.carriedRows()) && b.lookReady(now)
}

// startOffRound starts the off-round deposit of every carried valuable,
// anchored where the bot stands, at the keeper a round's deposit takes.
// It reports whether the request runs; tried or not, it waits for a new
// find (queueRound marks the rows).
func (b *Brain) startOffRound(now time.Time) bool {
	b.round = sessionRound{anchor: b.world.Self.Position(now), request: b.round.request}
	if !b.queueRound(roundDeposit, now) {
		return false
	}
	b.round.phase = roundDeposit
	return true
}
