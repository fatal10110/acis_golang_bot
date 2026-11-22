package botclient

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// Store buying timing and reach (docs/items.md section 8).
const (
	// marketArrive is how close to a seller the bot walks before it clicks:
	// inside the server's interact approach offset (100, plus both
	// collision radii), so the server walks it nowhere, and well inside the
	// trade interaction distance (150 in 3D) a purchase needs
	// (network/targeting.go interactApproachOffset, network/trade.go
	// tradeInteractionDistance).
	marketArrive = 100
	// marketReplyTimeout is how long the click that opens a store waits for
	// its PrivateStoreListSell, and a purchase for the InventoryUpdate it
	// brings. The server refuses a purchase in silence but for adena,
	// weight and slots (network/privatestore.go buyFromStore), and its
	// InventoryUpdate comes with the next 333 ms inventory tick.
	marketReplyTimeout = 5 * time.Second
)

// MarketPolicy is how the bot buys from other players' private sell
// stores (rule 9 errand "market", docs/items.md section 8). Every value is
// a configurable default (D6); Cooldown and the one request per store
// window are the errand's pacing.
type MarketPolicy struct {
	// MaxOverReference is the most the bot pays for an item, in percent of
	// its reference price; a spawned merchant's base price for it caps it
	// too. 0 buys nothing from stores.
	MaxOverReference int
	// Cooldown is how long after a visit to a store the bot leaves its
	// owner alone, whatever the visit bought.
	Cooldown time.Duration
}

// defaultMarketPolicy pays up to 1.5 times the reference price and visits a
// store at most every 10 minutes (docs/items.md section 8).
func defaultMarketPolicy() MarketPolicy {
	return MarketPolicy{MaxOverReference: 150, Cooldown: 10 * time.Minute}
}

// marketStep is the answer a store visit waits for.
type marketStep uint8

const (
	marketIdle marketStep = iota
	// marketAsked waits for the list the second click opens.
	marketAsked
	// marketBuying waits for the purchase's InventoryUpdate.
	marketBuying
)

// marketVisit is the bot's errand at another player's sell store (rule 9
// errand "market"). It is owned by the brain goroutine. v.obj is the
// owner's object id, 0 for no visit; v keeps the legs toward it (closeIn).
type marketVisit struct {
	v    npcVisit
	step marketStep
	// waitUntil is when the answer step waits for is given up on.
	waitUntil time.Time
	// list is the window the server opened and the bot has not answered.
	list *PrivateStoreListSell
	// clicks counts the visit's selections and window clicks.
	clicks int
	// cooled are the owners visited, by object id, until when they are
	// left alone (MarketPolicy.Cooldown).
	cooled map[int32]time.Time
}

// active reports whether a visit runs.
func (m *marketVisit) active() bool { return m.v.obj != 0 }

// end ends the visit and leaves its owner alone for cooldown; owners whose
// cool-down is over are forgotten.
func (m *marketVisit) end(now time.Time, cooldown time.Duration) {
	if m.cooled == nil {
		m.cooled = make(map[int32]time.Time)
	}
	for id, until := range m.cooled {
		if !now.Before(until) {
			delete(m.cooled, id)
		}
	}
	m.cooled[m.v.obj] = now.Add(cooldown)
	*m = marketVisit{cooled: m.cooled}
}

// sent records a packet of the visit that went out at now.
func (m *marketVisit) sent(a Action, now time.Time) {
	if !m.active() || a.object != m.v.obj {
		return
	}
	switch a.kind {
	case actSelect:
		m.clicks++
	case actTalk:
		m.clicks++
		m.step, m.waitUntil = marketAsked, now.Add(marketReplyTimeout)
	case actStoreBuy:
		m.step, m.list, m.waitUntil = marketBuying, nil, now.Add(marketReplyTimeout)
	}
}

// sellsToBot reports whether player o runs a sell store the bot may click:
// store type sell (a package store sells only its whole list; a buy or a
// manufacture store opens another window), alive, and neither flagged nor
// attackable without force, whom the server attacks on the second click
// instead of opening its store (network/player_follow.go actOnPlayer).
func sellsToBot(o *Object) bool {
	return o != nil && o.Kind == KindPlayer && o.StoreType == storeTypeSell && !o.Dead && !flagged(o) && !o.Attackable
}

// ruleMarket is a rule 9 errand: with supplies to refill or a gear goal,
// the bot walks to within marketArrive of a sell store in sight, selects
// its owner, clicks it again to open the store's list and buys the wanted
// rows it lists at or under their price cap (marketCap), within the budget
// (marketRows). Legs and clicks pass the frame pacer, a visit spending at
// most visitClicks clicks; RequestPrivateStoreBuy has no flood protector
// on the server and goes out once per list the server opened; an owner is
// visited once per MarketPolicy.Cooldown.
func (b *Brain) ruleMarket(now time.Time) (Action, bool) {
	m := &b.market
	if b.world.Self.InCombat || (!m.active() && !b.startMarket(now)) {
		return Action{}, false
	}
	cooldown := b.persona.Market.Cooldown
	if !sellsToBot(b.world.object(m.v.obj)) {
		// Out of sight, closed, sold out, dead or flagged meanwhile.
		m.end(now, cooldown)
		return Action{}, false
	}
	if m.step != marketIdle {
		if now.Before(m.waitUntil) {
			return Action{}, true
		}
		// The list never came (the store refused in silence), or the
		// purchase is over, answered or not: the visit is.
		m.end(now, cooldown)
		return Action{}, false
	}
	if l := m.list; l != nil {
		if rows := b.marketRows(*l); len(rows) > 0 {
			return Action{kind: actStoreBuy, payload: encodeRequestPrivateStoreBuy(l.OwnerID, rows), object: m.v.obj}, true
		}
		m.end(now, cooldown)
		return Action{}, false
	}
	if a, ok := b.closeIn(&m.v, marketArrive, now); ok {
		return a, true
	}
	if m.clicks >= visitClicks {
		m.end(now, cooldown)
		return Action{}, false
	}
	s := &b.world.Self
	if s.Sitting {
		return b.toggle(now), true
	}
	from := s.Position(now)
	if b.world.Target != m.v.obj {
		return selectAction(m.v.obj, from), true
	}
	return talkAction(m.v.obj, from), true
}

// startMarket starts a visit at the nearest sell store in sight whose
// owner is not cooling down, when the bot wants something (marketWants)
// and carries adena past the reserve, no shop visit runs and the brain has
// a travel map: the merchants' prices cap the stores'. It reports whether
// it started one.
func (b *Brain) startMarket(now time.Time) bool {
	m := &b.market
	if b.persona.Market.MaxOverReference <= 0 || b.cfg.Travel == nil || b.shop.v.active() {
		return false
	}
	if int64(b.carried(adenaID)) <= b.reserve() || len(b.marketWants()) == 0 {
		return false
	}
	from := b.world.Self.Position(now)
	var best int32
	bestDist := int64(math.MaxInt64)
	for id, o := range b.world.Objects {
		if !sellsToBot(o) || now.Before(m.cooled[id]) {
			continue
		}
		if d := dist2(from, o.Position(now)); d < bestDist {
			best, bestDist = id, d
		}
	}
	if best == 0 {
		return false
	}
	m.v = npcVisit{obj: best}
	return true
}

// marketWants are what a store visit looks for, in budget order: the
// supplies tier, each trip target of the use meter (supplies) carried
// under SupplyPolicy.Refill percent, up to the target, wherever it is
// sold; then the gear tier, the gear goal's pieces (gearGoal).
func (b *Brain) marketWants() []want {
	var out []want
	for _, s := range b.supplies() {
		if have := b.carried(s.item); b.refills(have, s.count) {
			out = append(out, want{buyRow: buyRow{item: s.item, count: s.count - have}, tier: tierSupplies})
		}
	}
	goal, _ := b.gearGoal()
	for _, id := range goal {
		out = append(out, want{buyRow: buyRow{item: id, count: 1}, tier: tierGear})
	}
	return out
}

// marketCap is the most the bot pays for one of item, listed at reference
// price ref (else the travel map's): MaxOverReference percent of the
// reference price, and never more than a spawned merchant's base price for
// it. ok is false when neither price is known.
func (b *Brain) marketCap(item, ref int32) (int64, bool) {
	m := b.cfg.Travel
	if ref <= 0 {
		ref = m.prices[item]
	}
	limit, ok := int64(-1), false
	if ref > 0 {
		limit, ok = int64(ref)*int64(b.persona.Market.MaxOverReference)/100, true
	}
	if p, sold := m.cheapest[item]; sold && m.spawnedSold[item] && (!ok || int64(p) < limit) {
		limit, ok = int64(p), true
	}
	return limit, ok
}

// marketRows is the purchase from store window l, the budget spent tier
// by tier above each tier's floor (docs/items.md section 5): per wanted
// item, its rows at or under the price cap, cheapest first, each at its
// listed price exactly (the server refuses the whole purchase otherwise),
// as many as the row lists, the want needs and the adena allows; of the
// gear, one piece per slot, and a piece of the planned set (gearGoal) only
// with the rest of the set still paid at its cheapest base price. A package
// store sells only its whole list: nothing is bought from one.
func (b *Brain) marketRows(l PrivateStoreListSell) []storeRow {
	if l.Packaged {
		return nil
	}
	_, set := b.gearGoal()
	var spent int64
	var rows []storeRow
	var got []int32 // the templates rows buy
	var bought [slotCount]bool
	for _, w := range b.marketWants() {
		t, slot := w.tier, slotNone
		if g, ok := b.gearItem(w.item); ok {
			if slot = slotOf(g.part); bought[slot] {
				continue
			}
			t = max(t, tierGear)
		}
		var rest int64 // the planned set's pieces still to buy after this one
		if slices.Contains(set, w.item) {
			for _, id := range set {
				if id != w.item && !slices.Contains(got, id) {
					rest += int64(b.cfg.Travel.cheapest[id])
				}
			}
		}
		need := int64(w.count)
		for _, it := range b.offers(l, w.item) {
			n := min(need, int64(it.Count))
			if it.Price > 0 {
				n = min(n, (int64(l.BuyerAdena)-spent-b.floor(t)-rest)/int64(it.Price))
			}
			if n < 1 {
				// The cheaper rows came first: no other row fits either.
				break
			}
			bought[slot] = true
			spent += n * int64(it.Price)
			need -= n
			rows = append(rows, storeRow{object: it.ObjectID, count: int32(n), price: it.Price})
			got = append(got, w.item)
			if len(rows) == shopMaxRows {
				return rows
			}
			if need == 0 {
				break
			}
		}
	}
	return rows
}

// offers are the rows of window l listing item at or under its price cap
// (marketCap), cheapest first.
func (b *Brain) offers(l PrivateStoreListSell, item int32) []StoreItem {
	var out []StoreItem
	for _, it := range l.Items {
		if it.TemplateID != item || it.Count < 1 || it.Price < 0 {
			continue
		}
		if limit, ok := b.marketCap(item, it.ReferencePrice); ok && int64(it.Price) <= limit {
			out = append(out, it)
		}
	}
	slices.SortStableFunc(out, func(x, y StoreItem) int { return cmp.Compare(x.Price, y.Price) })
	return out
}

// marketEvent moves the visit on with the server's answer ev: the store's
// list after the second click, and the purchase's items.
func (b *Brain) marketEvent(ev Event) {
	m := &b.market
	if !m.active() {
		return
	}
	switch e := ev.(type) {
	case PrivateStoreListSell:
		if m.step == marketAsked && e.OwnerID == m.v.obj {
			m.list, m.step, m.waitUntil = &e, marketIdle, time.Time{}
		}
	case InventoryUpdate, ItemList:
		if m.step == marketBuying {
			// The purchase came: the visit ends at the next decision.
			m.waitUntil = time.Time{}
		}
	}
}
