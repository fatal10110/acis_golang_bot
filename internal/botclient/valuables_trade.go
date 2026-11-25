package botclient

import "time"

// inviteArrive is how near the whisperer it asks for a trade the bot walks
// first (docs/valuables/trades.md "Asking for a trade"): inside
// tradeRange less its margin, so the trade's confirm needs no second walk.
const inviteArrive = 100

// inviteAreaMargin keeps an asked whisperer inside the round's area: within
// RoundReach less it of the round's anchor.
const inviteAreaMargin = 100

// Trade request refusals the requester gets (server network/trade.go
// handleTradeRequest, handleAnswerTradeRequest; docs/valuables/server.md).
// The busy ones (msgAlreadyTrading, msgS1IsBusyTryLater), the block ones
// (msgBlockedEverything, msgIgnoredBy) and the olympiad or chaotic text
// (msgS1) are named where other requests meet them.
const (
	msgTradeDeclined    = 119  // the partner declined
	msgTargetIncorrect  = 144  // the partner is not in the bot's known list
	msgStoreUnderWay    = 340  // a side is setting up a store
	msgNotAuthorized    = 794  // the bot's access level forbids trades
	msgTradeInStoreMode = 1065 // a side runs a store
)

// tradeRequestRefusals are the system messages that end the bot's own trade
// request at once.
var tradeRequestRefusals = map[int32]bool{
	msgTradeDeclined: true, msgTargetIncorrect: true, msgStoreUnderWay: true, msgTradeInStoreMode: true,
	msgNotAuthorized: true, msgBlockedEverything: true, msgIgnoredBy: true, msgS1: true,
	msgAlreadyTrading: true, msgS1IsBusyTryLater: true,
}

// valuableInvite is a round's trade request to a whisperer: the bot walks
// to where partner stood when the invite started (to), then sends
// TradeRequest, sent from then until TradeStart, a refusal or
// tradeRequestTimeout. The zero valuableInvite is none.
type valuableInvite struct {
	partner int32
	to      Point
	sent    time.Time
}

func tradeRequestAction(partner int32) Action {
	return Action{kind: actTradeRequest, payload: encodeTradeRequest(partner), object: partner}
}

// valuablesLeft is what the session's valuables budget has left
// (docs/valuables/release.md "Valuables budget (D34)").
func (b *Brain) valuablesLeft() int64 {
	return b.persona.Valuables.Budget - b.valuablesSold
}

// valuableUnit is the price of one unit of entry k to partner: the agreed
// price, else the partner's current price, else the entry's ask; 0 with no
// entry.
func (b *Brain) valuableUnit(partner int32, k valuableKey) int64 {
	v := b.cfg.valuables
	if d, ok := v.talk.deals[whisperDeal{partner, k}]; ok {
		return d.current
	}
	if e := v.entries[k]; e != nil {
		return e.ask
	}
	return 0
}

// agreedDeal reports whether partner has an agreed price in the open
// valuables round.
func (b *Brain) agreedDeal(partner int32) bool {
	v := b.cfg.valuables
	if !v.round.open {
		return false
	}
	for dl, d := range v.talk.deals {
		if dl.partner == partner && d.agreed {
			return true
		}
	}
	return false
}

// valuableGoods is the valuable the bot offers partner in a trade
// (docs/valuables/trades.md "Trades"): only while a round is open and the
// budget has some left, one row of rows of the entry the partner agreed a
// price for, else named, else the top-scored carried one (by score V, as
// the ads order them), at most ceil(left / unit price) of its units. The
// release mark that keeps a released entry out lands with V7c.
func (b *Brain) valuableGoods(partner int32, rows []TradeItem) (TradeItem, valuableKey, bool) {
	v := b.cfg.valuables
	if !v.round.open || b.persona.Valuables == (ValuablePolicy{}) || b.cfg.Travel == nil {
		return TradeItem{}, valuableKey{}, false
	}
	left := b.valuablesLeft()
	if left <= 0 {
		return TradeItem{}, valuableKey{}, false
	}
	byKey := make(map[valuableKey]TradeItem)
	for _, row := range rows {
		if row.Count <= 0 || row.SubCategory == subCategoryQuest {
			continue
		}
		it, ok := b.world.Inventory.Items[row.ObjectID]
		if !ok || !b.valuable(it) {
			continue
		}
		k := valuableKey{template: it.TemplateID, enchant: it.Enchant}
		if _, seen := byKey[k]; !seen {
			byKey[k] = row
		}
	}
	keys := b.adEntries()
	pick := func(match func(valuableKey) bool) (valuableKey, bool) {
		for _, k := range keys {
			if _, ok := byKey[k]; ok && match(k) {
				return k, true
			}
		}
		return valuableKey{}, false
	}
	deal := func(k valuableKey) (whisperPrice, bool) {
		d, ok := v.talk.deals[whisperDeal{partner, k}]
		return d, ok
	}
	k, ok := pick(func(k valuableKey) bool { d, ok := deal(k); return ok && d.agreed })
	if !ok {
		k, ok = pick(func(k valuableKey) bool { _, ok := deal(k); return ok })
	}
	if !ok {
		k, ok = pick(func(valuableKey) bool { return true })
	}
	unit := b.valuableUnit(partner, k)
	if !ok || unit <= 0 {
		return TradeItem{}, valuableKey{}, false
	}
	row := byKey[k]
	row.Count = int32(min(int64(row.Count), ceilDiv(left, unit)))
	return row, k, true
}

// valuableSaleEvent counts a trade that settled with a valuable in the
// session's valuables budget, at the price the bot confirmed (Trade.sale).
// It runs before the world applies ev, which clears the trade.
func (b *Brain) valuableSaleEvent(ev Event) {
	e, ok := ev.(SendTradeDone)
	if t := &b.world.Trade; ok && e.Success && t.Open() && t.valuable {
		b.valuablesSold += t.sale
	}
}

// inviteEvent ends the round's sent trade request on its answer: a trade
// window opening, or a refusal (tradeRequestRefusals).
func (b *Brain) inviteEvent(ev Event) {
	iv := &b.cfg.valuables.round.invite
	if iv.sent.IsZero() {
		return
	}
	switch e := ev.(type) {
	case TradeStart:
		*iv = valuableInvite{}
	case SystemMessage:
		if tradeRequestRefusals[e.ID] {
			*iv = valuableInvite{}
		}
	}
}

// inviteSent records the round's trade request to partner sent at now.
func (b *Brain) inviteSent(partner int32, now time.Time) {
	if iv := &b.cfg.valuables.round.invite; iv.partner == partner {
		iv.sent = now
	}
}

// roundEnd is the open round's end as it stands: RoundWait after its first
// ad line; none before that line. The Grace extension lands with V7a; until
// then no request goes out in it.
func (b *Brain) roundEnd() (time.Time, bool) {
	r := &b.cfg.valuables.round
	if r.ads == 0 {
		return time.Time{}, false
	}
	return r.first.Add(b.persona.Valuables.RoundWait), true
}

// inviteGate reports whether the round may set out to ask a whisperer, or
// send the request, at now (docs/valuables/trades.md "Asking for a
// trade"): the round said its first line and ends more than
// tradeRequestTimeout later, the store is neither open nor opening, no
// warehouse errand runs (the round's private request), the bot is out of
// combat with no trade open or trade request awaiting its answer, and it
// carries a valuable the budget lets it offer.
func (b *Brain) inviteGate(now time.Time) bool {
	w := b.world
	end, ok := b.roundEnd()
	return ok && now.Before(end.Add(-tradeRequestTimeout)) &&
		!b.store.active() && w.Self.StoreType == 0 && !b.wh.running &&
		!w.Self.InCombat && !w.Trade.Open() && !w.Prompts.Trade.Pending(now) &&
		b.valuablesLeft() > 0 && len(b.adEntries()) > 0
}

// startInvite picks the whisperer the round asks for a trade at now, the
// bot standing at the anchor: the nearest (then lowest id) partner the
// responder replied to this round, not asked yet, in sight as a player,
// within InviteReach of the bot and within RoundReach less
// inviteAreaMargin of the anchor. A partner outside the area gets only the
// whisper ("trade me"). It marks the partner asked whatever the request's
// fate and reports whether it picked one.
func (b *Brain) startInvite(now time.Time) bool {
	v, p := b.cfg.valuables, b.persona.Valuables
	r := &v.round
	if r.invite.partner != 0 || !b.inviteGate(now) {
		return false
	}
	self := b.world.Self.Position(now)
	reach, area := int64(p.InviteReach), int64(p.RoundReach-inviteAreaMargin)
	var best int32
	var bestAt Point
	var bestD int64
	for partner := range v.talk.to {
		o := b.world.Objects[partner]
		if r.asked[partner] || o == nil || o.Kind != KindPlayer {
			continue
		}
		at := o.Position(now)
		d := horizontal2(self, at)
		if d > reach*reach || horizontal2(r.anchor, at) > area*area {
			continue
		}
		if best == 0 || d < bestD || d == bestD && partner < best {
			best, bestAt, bestD = partner, at, d
		}
	}
	if best == 0 {
		return false
	}
	if r.asked == nil {
		r.asked = make(map[int32]bool)
	}
	r.asked[best] = true
	r.invite = valuableInvite{partner: best, to: bestAt}
	return true
}

// inviteTurn runs the round's trade request: it walks to within
// inviteArrive of where the partner stood, sends TradeRequest there and
// holds the turn until TradeStart, a refusal or tradeRequestTimeout. A
// partner gone from sight, an unreachable walk or a closed gate
// (inviteGate) before the request ends it unsent. It reports false with
// none under way, so the walk back to the anchor comes next.
//
// Pacing: one TradeRequest a partner a round, at most one open, each
// awaited up to tradeRequestTimeout (15 s, the server's RequestTimeout,
// trade/book.go); the server has no gate on TradeRequest, only the busy
// refusals (142, 153) and that expiry. The walk's segments are paced as
// every goTo.
func (b *Brain) inviteTurn(now time.Time) (Action, bool) {
	iv := &b.cfg.valuables.round.invite
	switch {
	case iv.partner == 0:
		return Action{}, false
	case !iv.sent.IsZero():
		if now.Sub(iv.sent) < tradeRequestTimeout {
			return Action{}, true
		}
		*iv = valuableInvite{}
		return Action{}, false
	}
	if o := b.world.Objects[iv.partner]; o == nil || o.Kind != KindPlayer || !b.inviteGate(now) {
		*iv = valuableInvite{}
		return Action{}, false
	}
	a, st := b.goTo(iv.to, inviteArrive, walkPlace, now)
	switch st {
	case walkWalking:
		return a, true
	case walkArrived:
		return tradeRequestAction(iv.partner), true
	}
	*iv = valuableInvite{}
	return Action{}, false
}
