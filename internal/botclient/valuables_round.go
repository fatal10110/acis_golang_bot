package botclient

import (
	"cmp"
	"slices"
	"time"
)

// roundPhase is where this session's part of a valuables round stands
// (docs/valuables/round.md, round-end.md).
type roundPhase uint8

const (
	// roundIdle: none of the round's keeper visits, walks back or closing
	// runs; with the round open it advertises at its anchor.
	roundIdle roundPhase = iota
	// roundWithdraw: the withdrawal's private request runs (step 1).
	roundWithdraw
	// roundReturn: the walk back to the anchor after the withdrawal.
	roundReturn
	// roundSettle: the round ended; the deposit waits for the pending
	// trade requests (step 5).
	roundSettle
	// roundDeposit: the deposit's private request runs (step 5).
	roundDeposit
	// roundLeave: the walk back to the anchor after the end (step 6).
	roundLeave
	// roundHold: cut off with the request sent; it is stepped to its answer
	// or warehouseReplyTimeout, the bot not moving.
	roundHold
)

// sessionRound is this session's part of the valuables round, owned by the
// brain goroutine: what dies with the session (its keeper visits are the
// session's warehouse errand) while the round's own state outlives it
// (valuables.round).
type sessionRound struct {
	phase roundPhase
	// anchor is the round's anchor, kept past endRound for the closing's
	// walk back.
	anchor Point
	// request is when the round's own TradeRequest went out, carried past
	// the round's end until answered (inviteEvent) or tradeRequestTimeout
	// old; zero for none.
	request time.Time
	// stale is set when the session started with an earlier session's
	// round open, for its first decision to end (endStaleRound).
	stale bool
	// offRound is set for an off-round errand (docs/valuables/round-end.md
	// "Off-round errands"): the release withdrawal or the budget-spent
	// deposit, no round open.
	offRound bool
}

// roundReady reports whether a round may start at now as far as the bot's
// state goes (docs/valuables/round.md "Start"): lookReady's checks, a
// restart region and the ad board free there.
func (b *Brain) roundReady(now time.Time) bool {
	if !b.lookReady(now) {
		return false
	}
	pos := b.world.Self.Position(now)
	region := b.cfg.Travel.restartRegion(pos.X, pos.Y)
	return region >= 0 && b.cfg.Ads.Free(region, now)
}

// roundDue reports whether a round starts at now (docs/valuables/round.md
// "Start"): the policy enabled, the session's look done and none running,
// a valuable not released carried, RoundGap past the latest start, the
// valuables budget not spent, and roundReady. Stored valuables alone start
// none, nor released ones (docs/valuables/ledger.md "Released entries").
func (b *Brain) roundDue(now time.Time) bool {
	v, p := b.cfg.valuables, b.persona.Valuables
	if p == (ValuablePolicy{}) || b.cfg.Travel == nil || !b.wh.private.Known || b.look.phase != lookIdle {
		return false
	}
	if !v.lastStart.IsZero() && now.Sub(v.lastStart) < p.RoundGap || b.valuablesLeft() <= 0 {
		return false
	}
	return v.carried() && b.roundReady(now)
}

// startRound opens a round at now anchored where the bot stands and queues
// its withdrawal of the stored valuables (step 1, storedRows). A
// withdrawal that does not start leaves them stored: the round advertises
// what it carries.
func (b *Brain) startRound(now time.Time) {
	anchor := b.world.Self.Position(now)
	b.cfg.valuables.openRound(anchor, now)
	b.round = sessionRound{anchor: anchor}
	if b.queueRound(roundWithdraw, now) {
		b.round.phase = roundWithdraw
	}
}

// storedRows are the withdrawal's rows, each private warehouse row that
// passes the warehouse row test, named by its object
// (docs/valuables/round-end.md step 1, "Off-round errands"): a round's
// takes the entries not released, and the released ones while the
// valuables budget is not spent; the off-round release withdrawal's takes
// the released ones only.
func (b *Brain) storedRows() []WarehouseRow {
	v, spent := b.cfg.valuables, b.valuablesLeft() <= 0
	var rows []WarehouseRow
	for _, r := range b.wh.private.Contents {
		if r.Count <= 0 || !b.valuableRow(r) {
			continue
		}
		if rel := v.released(valuableKey{template: r.TemplateID, enchant: r.Enchant}); rel && spent || !rel && b.round.offRound {
			continue
		}
		rows = append(rows, WarehouseRow{TemplateID: r.TemplateID, Count: r.Count, Object: r.ObjectID})
	}
	return rows
}

// carriedRows are the rows of every valuable the bot carries, released or
// not, named by its object, in object order: the budget-spent deposit's
// (docs/valuables/round-end.md "Off-round errands").
func (b *Brain) carriedRows() []WarehouseRow {
	return b.valuableRows(true)
}

// keptRows are a round's deposit rows: carriedRows but the released
// entries, which stay carried for the store (docs/valuables/round-end.md
// step 5).
func (b *Brain) keptRows() []WarehouseRow {
	return b.valuableRows(false)
}

// valuableRows are the rows of the valuables the bot carries, the released
// entries' with released set, in object order.
func (b *Brain) valuableRows(released bool) []WarehouseRow {
	v := b.cfg.valuables
	var rows []WarehouseRow
	for _, it := range b.world.Inventory.Items {
		if !b.valuable(it) || !released && v.released(valuableKey{template: it.TemplateID, enchant: it.Enchant}) {
			continue
		}
		rows = append(rows, WarehouseRow{TemplateID: it.TemplateID, Count: it.Count, Object: it.ObjectID})
	}
	slices.SortFunc(rows, func(a, c WarehouseRow) int { return cmp.Compare(a.Object, c.Object) })
	return rows
}

// valuableFees is the deposit fees a merchant's shop visits never spend
// (docs/valuables/warehouse.md "Fees and slots"): warehouseFee a row for
// the larger of HoldRows and the rows its carried valuables take; none
// with the policy disabled.
func (b *Brain) valuableFees() int64 {
	if b.persona.Valuables == (ValuablePolicy{}) || b.cfg.Travel == nil {
		return 0
	}
	return int64(warehouseFee) * int64(max(b.persona.Valuables.HoldRows, len(b.carriedRows())))
}

// queueRound queues and starts the private request of phase: the
// withdrawal of the stored valuables (storedRows) or the deposit of the
// carried ones (keptRows in a round, carriedRows off-round), at
// the nearest unmarked keeper within RoundReach of the anchor. It reports
// whether one runs; one that does not leaves nothing queued.
func (b *Brain) queueRound(phase roundPhase, now time.Time) bool {
	req := WarehouseRequest{Private: true, Anchor: b.round.anchor, Reach: b.persona.Valuables.RoundReach}
	if phase == roundWithdraw {
		req.Withdraw = b.storedRows()
	} else {
		req.Deposit = b.keptRows()
		if b.round.offRound {
			req.Deposit = b.carriedRows()
		}
		b.cfg.valuables.markDeposited(req.Deposit)
	}
	if req.empty() {
		return false
	}
	b.wh.privateWant = req
	if !b.startWarehouse(now) {
		b.wh.privateWant = WarehouseRequest{}
		return false
	}
	return true
}

// stepPrivate steps the round's running private request and reports
// whether it holds the turn. A request ended on a new noPrivate mark is
// queued again at once from where the bot stands while an unmarked keeper
// remains in reach of the anchor (docs/valuables/round.md "Back to the
// anchor").
func (b *Brain) stepPrivate(now time.Time) (Action, bool) {
	e := &b.wh
	if !e.running {
		return Action{}, false
	}
	marks := len(e.noPrivate)
	if a, ok := b.stepWarehouse(now); ok || e.running {
		return a, true
	}
	if len(e.noPrivate) > marks && b.privateKeeper(b.round.anchor) && b.queueRound(b.round.phase, now) {
		if a, ok := b.stepWarehouse(now); ok || e.running {
			return a, true
		}
	}
	return Action{}, false
}

// roundStep runs the round at now (docs/valuables/round-end.md "Steps"):
// the withdrawal and the walk back, the ad lines, answers and trade
// requests at the anchor until roundOver, then the count, the deposit and
// the walk back, after which the rule lets go. Called away it ends the
// round as a cut-off (cutOff).
func (b *Brain) roundStep(now time.Time) (Action, bool) {
	s := &b.round
	if s.phase == roundHold {
		return b.holdStep(now)
	}
	if b.called() {
		return b.cutOff(now)
	}
	switch s.phase {
	case roundWithdraw:
		if b.world.Self.InCombat {
			return Action{}, false
		}
		if a, ok := b.stepPrivate(now); ok {
			return a, true
		}
		s.phase = roundReturn
		fallthrough
	case roundReturn:
		a, st := b.backTo(s.anchor, now)
		switch st {
		case walkWalking:
			return a, true
		case walkUnreachable:
			return b.endCut(now)
		}
		if s.offRound {
			*s = sessionRound{request: s.request}
			return Action{}, false
		}
		s.phase = roundIdle
		b.cfg.valuables.round.back = now
		return b.advertise(now)
	case roundSettle, roundDeposit, roundLeave:
		return b.closeStep(now)
	}
	return b.advertise(now)
}

// advertise is the open round at its anchor (steps 2-4): it ends the round
// once roundOver holds, runs a trade request under way, walks back to the
// anchor, says the next ad line there (adTurn; none from RoundWait after
// the first, in the Grace) and asks a whisperer for a trade (startInvite),
// holding the turn while it waits. An unreachable
// walk back after a buyer walk ends the round as a cut-off.
func (b *Brain) advertise(now time.Time) (Action, bool) {
	r := &b.cfg.valuables.round
	if over, counted := b.roundOver(now); over {
		b.endOpen(now, counted)
		b.round.phase = roundSettle
		return b.closeStep(now)
	}
	if a, ok := b.inviteTurn(now); ok {
		return a, true
	}
	a, st := b.backTo(r.anchor, now)
	switch st {
	case walkWalking:
		return a, true
	case walkUnreachable:
		return b.endCut(now)
	}
	if r.ads == 0 || now.Before(r.first.Add(b.persona.Valuables.RoundWait)) {
		if a, ok := b.adTurn(now); ok {
			return a, true
		}
	}
	if b.startInvite(now) {
		if a, ok := b.inviteTurn(now); ok {
			return a, true
		}
	}
	return Action{}, true
}

// roundOver reports whether the open round ends at now, and whether it
// counts as run (step 4): at once when the trades brought the session's
// sales to the budget; with no ad line FirstLineWait after the walk back
// from the withdrawal (or the start), not counted; else at roundEnd, put
// off once up to Grace while a trade is open or a whisperer with an agreed
// price is in sight.
func (b *Brain) roundOver(now time.Time) (over, counted bool) {
	r, p := &b.cfg.valuables.round, b.persona.Valuables
	if b.valuablesLeft() <= 0 {
		return true, r.ads > 0
	}
	if r.ads == 0 {
		from := r.back
		if from.IsZero() {
			from = r.start
		}
		return !now.Before(from.Add(p.FirstLineWait)), false
	}
	end := r.first.Add(p.RoundWait)
	if now.Before(end) {
		return false, true
	}
	if now.Before(end.Add(p.Grace)) && (b.world.Trade.Open() || b.agreedInSight()) {
		r.extended = true
		return false, true
	}
	return true, true
}

// agreedInSight reports whether a whisperer the bot agreed a price with
// this round stands in sight as a player.
func (b *Brain) agreedInSight() bool {
	for dl, d := range b.cfg.valuables.talk.deals {
		if o := b.world.Objects[dl.partner]; d.agreed && o != nil && o.Kind == KindPlayer {
			return true
		}
	}
	return false
}

// countRound steps every entry not released the bot carries as an unsold
// round: rounds + 1 and the ask RoundStep points of its reference price
// lower, never below its floor (docs/valuables/whispers.md "Prices").
func (b *Brain) countRound() {
	p, m := b.persona.Valuables, b.cfg.Travel
	if m == nil {
		return
	}
	for k, e := range b.cfg.valuables.entries {
		if e.carried == 0 || e.released {
			continue
		}
		e.rounds++
		e.ask = p.lowered(e.ask, m.valueScore(p, k.template, int(k.enchant)).ref, b.valuableFloor(k))
	}
}

// ranRound counts a round run to its end and runs the cap
// (docs/valuables/round-end.md step 5): before the deposit, so the
// deposit leaves the released entries carried.
func (b *Brain) ranRound() {
	b.countRound()
	if m := b.cfg.Travel; m != nil {
		b.cfg.valuables.capEntries(b.persona.Valuables, m.stackable)
	}
}

// endOpen ends the open round at now, counted as run or not, keeping its
// own trade request under way for the deposit to wait on.
func (b *Brain) endOpen(now time.Time, counted bool) {
	v := b.cfg.valuables
	if counted {
		b.ranRound()
	}
	b.round.anchor = v.round.anchor
	if sent := v.round.invite.sent; !sent.IsZero() && now.Sub(sent) < tradeRequestTimeout {
		b.round.request = sent
	}
	v.endRound(now)
}

// closeStep is the closing of an ended round (steps 5 and 6): once the
// bot's own trade request is answered or tradeRequestTimeout old and no
// player's request awaits its answer, it deposits the carried valuables
// the cap did not release (keptRows) when an open is due, then walks back to the anchor and lets go. With no
// open due, or a deposit that does not start, they stay carried.
func (b *Brain) closeStep(now time.Time) (Action, bool) {
	s, w := &b.round, b.world
	switch s.phase {
	case roundSettle:
		if len(b.keptRows()) > 0 && b.wh.openDue(now) && !now.Before(b.wh.retryAt) && !b.wh.running {
			if !s.request.IsZero() && now.Sub(s.request) < tradeRequestTimeout || w.Prompts.Trade.Pending(now) || w.Trade.Open() {
				return Action{}, true
			}
			s.request = time.Time{}
			if b.queueRound(roundDeposit, now) {
				s.phase = roundDeposit
				return b.closeStep(now)
			}
		}
		s.phase = roundLeave
		return b.closeStep(now)
	case roundDeposit:
		if w.Self.InCombat {
			return Action{}, false
		}
		if a, ok := b.stepPrivate(now); ok {
			return a, true
		}
		s.phase = roundLeave
	}
	if a, st := b.backTo(s.anchor, now); st == walkWalking {
		return a, true
	}
	*s = sessionRound{request: s.request}
	return Action{}, false
}

// endCut ends the round at now as a cut-off without a request to stop
// (docs/valuables/round-end.md "Cut-off"): counted as run after its first
// ad line (ranRound), nothing deposited, the valuables carried; the rule
// lets go.
func (b *Brain) endCut(now time.Time) (Action, bool) {
	if v := b.cfg.valuables; v.round.open {
		if v.round.ads > 0 {
			b.ranRound()
		}
		v.endRound(now)
	}
	b.round = sessionRound{}
	return Action{}, false
}

// cutOff ends the round at now for a bot called away (endCut), with its
// running private request: cancelled before the request went out, else
// stepped to its answer, the bot not moving (holdStep). No walk back.
func (b *Brain) cutOff(now time.Time) (Action, bool) {
	running := b.round.phase == roundWithdraw || b.round.phase == roundDeposit
	b.endCut(now)
	if e := &b.wh; running && e.running && e.req.Private && !e.cancelPrivate() {
		b.round.phase = roundHold
		return b.holdStep(now)
	}
	return Action{}, false
}

// holdStep steps a cut-off round's sent request to its answer or
// warehouseReplyTimeout, holding the turn without moving, then lets go.
func (b *Brain) holdStep(now time.Time) (Action, bool) {
	if a, ok := b.stepWarehouse(now); ok || b.wh.running {
		return a, true
	}
	b.round = sessionRound{}
	return Action{}, false
}

// endStaleRound ends at the session's first decision a round an earlier
// session left open (docs/valuables/ledger.md "The valuables value"): a
// cut-off, its reply queue, partners and trade request dropped, nothing
// deposited. Cut after its first line its entries step at once, and its
// cap runs once the session's look is done (docs/valuables/release.md
// "Tests"): the cap counts the stored units the look reads.
func (b *Brain) endStaleRound(now time.Time) {
	v := b.cfg.valuables
	if v == nil {
		return
	}
	if b.round.stale {
		b.round.stale = false
		if v.round.open {
			if v.round.ads > 0 {
				b.countRound()
				v.capDue = true
			}
			v.endRound(now)
			b.round = sessionRound{}
		}
	}
	if v.capDue && b.wh.private.Known && b.look.phase == lookIdle && b.cfg.Travel != nil {
		v.capDue = false
		v.capEntries(b.persona.Valuables, b.cfg.Travel.stackable)
	}
}

// backTo is the round's walk back to within anchorArrive of anchor (goTo
// with walkPlace); arrived at once with the bot already there.
func (b *Brain) backTo(anchor Point, now time.Time) (Action, walkStatus) {
	if horizontal2(b.world.Self.Position(now), anchor) <= anchorArrive*anchorArrive {
		return Action{}, walkArrived
	}
	return b.goTo(anchor, anchorArrive, walkPlace, now)
}
