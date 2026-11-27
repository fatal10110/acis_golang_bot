package botclient

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// tradeRequestTimeout is how long the server keeps a trade request
// answerable (RequestTimeout, internal/gameserver/trade/book.go): an answer
// sent later reaches no request.
const tradeRequestTimeout = 15 * time.Second

// tradeRange is the farthest, in 3D world units, a trader's partner may
// stand at the trader's first confirm: farther, the server cancels the
// whole trade (tradeInteractionDistance, network/trade.go handleTradeDone).
// The bot confirms only within tradeRange-tradeRangeMargin, the margin
// covering the error of dead-reckoned positions.
const (
	tradeRange       = 150
	tradeRangeMargin = 30
)

// tradeForget is how long after the bot cancelled a trade it waits for the
// server's SendTradeDone before it forgets the trade itself, so a lost
// answer cannot hold the bot in rule 2.
const tradeForget = 10 * time.Second

// subCategoryQuest is the item subcategory of quest items
// (item.SubCategoryQuest on the server).
const subCategoryQuest = 3

// answerTradeAction answers the open trade request; an accepted one names
// partner, for the per-partner rate limit.
func answerTradeAction(accept bool, partner int32) Action {
	a := Action{kind: actAnswerTrade, payload: botwire.EncodeAnswerTradeRequest(accept)}
	if accept {
		a.object = partner
	}
	return a
}

func addTradeItemAction(it TradeItem) Action {
	return Action{kind: actTradeAdd, payload: botwire.EncodeAddTradeItem(it.ObjectID, it.Count)}
}

func tradeConfirmAction() Action {
	return Action{kind: actTradeConfirm, payload: botwire.EncodeTradeDone(true)}
}

func tradeCancelAction() Action {
	return Action{kind: actTradeCancel, payload: botwire.EncodeTradeDone(false)}
}

// TradeInvite is a trade request waiting for the bot's answer.
type TradeInvite struct {
	Open      bool
	PartnerID int32
	// At is when the request arrived.
	At time.Time
}

// Pending reports whether the request is open and still answerable at now.
func (t TradeInvite) Pending(now time.Time) bool {
	return t.Open && now.Sub(t.At) < tradeRequestTimeout
}

// Trade is the bot's open trade window; the zero Trade is none.
type Trade struct {
	PartnerID int32
	// Offerable are the items the bot may offer (TradeStart).
	Offerable []TradeItem
	// Own and Other are the bot's and the partner's offers, a row per add
	// with the quantity added.
	Own, Other []TradeItem
	// OwnOk and OtherOk are set once the server reported the bot's and the
	// partner's confirm; either locks both offers.
	OwnOk, OtherOk bool
	// At is when the window opened.
	At time.Time
	// offer is what the bot offers, picked before its first add; added
	// counts the adds it sent, one per offer row, each once the one before
	// it is in Own. confirmed is set once the bot sent its confirm,
	// cancelAt once it sent its cancel: each goes out once a trade.
	offer     []tradeOffer
	added     int
	confirmed bool
	cancelAt  time.Time
	// valuable is set while the bot's offer is valuables
	// (Brain.valuableGoods); sale is then the trade's price, fixed when the
	// bot confirms, which a SendTradeDone with success counts in the
	// session's valuables budget (Brain.valuableSaleEvent).
	valuable bool
	sale     int64
}

// tradeOffer is a row the bot offers in a trade and, of a valuable, its
// ledger entry.
type tradeOffer struct {
	row TradeItem
	key valuableKey
}

// Open reports whether a trade window is open.
func (t *Trade) Open() bool { return t.PartnerID != 0 }

// applyTrade updates the trade prompt and window with a trade event.
func (w *World) applyTrade(ev Event, now time.Time) {
	t := &w.Trade
	switch e := ev.(type) {
	case SendTradeRequest:
		w.Prompts.Trade = TradeInvite{Open: true, PartnerID: e.SenderID, At: now}
	case TradeStart:
		w.Prompts.Trade = TradeInvite{}
		*t = Trade{PartnerID: e.PartnerID, Offerable: e.Items, At: now}
	case TradeOwnAdd:
		if t.Open() {
			t.Own = append(t.Own, e.Item)
		}
	case TradeOtherAdd:
		if t.Open() {
			t.Other = append(t.Other, e.Item)
		}
	case TradePressOwnOk:
		t.OwnOk = t.Open()
	case TradePressOtherOk:
		t.OtherOk = t.Open()
	case SendTradeDone:
		*t = Trade{}
	}
}

// TradePolicy is how the bot trades (rule 2): it sells its own goods at
// fixed prices for adena, a trade at a time, to the player who asks.
type TradePolicy struct {
	// Prices are the goods the bot sells, the unit price in adena by
	// template id. A trade offers the whole stack of the first priced item
	// TradeStart lists that is neither equipped nor a quest item. An
	// item without a price here sells at its store price when it is loot
	// the bot's store would sell (Brain.tradePrice); with neither, every
	// request but an exempt partner's is declined.
	Prices map[int32]int32
	// Exempt reports whether the player partner is exempt (a GM): the bot
	// accepts the partner's requests whatever its goods and the rate limit,
	// offers nothing, and confirms once the partner confirmed. Nil exempts
	// none.
	Exempt func(w *World, partner int32) bool
	// Every is how long a partner whose request the bot accepted waits
	// before the bot accepts another of its requests; 0 never waits.
	Every time.Duration
	// Timeout is how long a trade may stay open before the bot cancels
	// it; 0 never cancels.
	Timeout time.Duration
	// MaxLoad is the load percent above which the bot refuses an item
	// other than adena an exempt partner offers: the load it carries, and
	// with the item weights known (TravelMap.weights) the load the offer
	// would bring it to, past which it refuses the offer, as it does an
	// item the loaded weight table lacks (#488). The server refuses an
	// overweight trade itself at settlement (ReceiverWeightFits). Other
	// partners may offer only adena.
	MaxLoad int
}

func defaultTradePolicy() TradePolicy {
	return TradePolicy{Every: time.Minute, Timeout: time.Minute, MaxLoad: 50}
}

func (p *TradePolicy) exempt(w *World, partner int32) bool {
	return p.Exempt != nil && p.Exempt(w, partner)
}

// tradeGoods is the item the bot sells in a trade: the first of rows,
// in their order, the inventory holds unequipped and not as a quest item,
// priced by the policy, or else loot its store would sell (storeLoot) at
// the store's price. A valuable (Brain.valuable) is never offered so: a
// round sells it at its own price.
func (b *Brain) tradeGoods(rows []TradeItem) (TradeItem, bool) {
	for _, row := range rows {
		if row.Count <= 0 || row.SubCategory == subCategoryQuest {
			continue
		}
		it, ok := b.world.Inventory.Items[row.ObjectID]
		if !ok || it.Equipped || it.SubCategory == subCategoryQuest || b.valuable(it) {
			continue
		}
		if _, priced := b.tradePrice(it); !priced {
			continue
		}
		if _, fixed := b.persona.Trade.Prices[row.TemplateID]; fixed || b.storeLoot(it) {
			return row, true
		}
	}
	return TradeItem{}, false
}

// inventoryRows is the bot's inventory as trade rows, for the goods check
// before a trade opens.
func (b *Brain) inventoryRows() []TradeItem {
	rows := make([]TradeItem, 0, len(b.world.Inventory.Items))
	for _, it := range b.world.Inventory.Items {
		rows = append(rows, TradeItem{ObjectID: it.ObjectID, TemplateID: it.TemplateID, Count: it.Count, SubCategory: it.SubCategory})
	}
	return rows
}

// ruleTrade is rule 2's trade: it answers a pending trade request, then
// runs the open trade, holding the bot still until the trade ends.
func (b *Brain) ruleTrade(now time.Time) (Action, bool) {
	if inv := b.world.Prompts.Trade; inv.Pending(now) {
		return answerTradeAction(b.acceptTrade(inv.PartnerID, now), inv.PartnerID), true
	}
	if !b.world.Trade.Open() {
		return Action{}, false
	}
	return b.tradeStep(now), true
}

// acceptTrade decides a request of the player partner: declined in store
// mode, an opening included (the server forbids it), in combat, from a player out of sight, and
// unless the partner is exempt, within Every of its last accepted request
// (but for a partner with an agreed price in a valuables round) or with no
// goods for sale, a valuable counting as goods in a round (valuableGoods).
func (b *Brain) acceptTrade(partner int32, now time.Time) bool {
	w, p := b.world, &b.persona.Trade
	if b.store.active() || w.Self.StoreType != 0 || w.Self.InCombat {
		return false
	}
	if o := w.Objects[partner]; o == nil || o.Kind != KindPlayer {
		return false
	}
	if p.exempt(w, partner) {
		return true
	}
	if last, ok := b.tradedAt[partner]; ok && now.Sub(last) < p.Every && !b.agreedDeal(partner) {
		return false
	}
	rows := b.inventoryRows()
	if _, ok := b.tradeGoods(rows); ok {
		return true
	}
	return len(b.valuableGoods(partner, rows)) > 0
}

// tradeStep is the bot's next move in its open trade. It offers its goods,
// in a valuables round valuables first (valuableGoods: every entry the
// partner agreed a price for, an add a turn), confirms once the partner's
// adena covers their price and the partner stands within range, and cancels
// a trade that cannot settle: timed out, the bot in combat, an item
// refused, or the offers locked before the bot offered them all or short of
// the price.
func (b *Brain) tradeStep(now time.Time) Action {
	t, p := &b.world.Trade, &b.persona.Trade
	if !t.cancelAt.IsZero() {
		if now.Sub(t.cancelAt) >= tradeForget {
			*t = Trade{}
		}
		return Action{}
	}
	if t.OwnOk && t.OtherOk {
		// Settling: SendTradeDone follows.
		return Action{}
	}
	exempt := p.exempt(b.world, t.PartnerID)
	if (p.Timeout > 0 && now.Sub(t.At) >= p.Timeout) || b.world.Self.InCombat || !b.tradeReceivable(exempt) {
		return tradeCancelAction()
	}
	if t.OwnOk || t.confirmed {
		return Action{}
	}
	if exempt {
		if !t.OtherOk {
			return Action{}
		}
	} else {
		if t.offer == nil {
			if t.OtherOk {
				// The partner's confirm locked the offers before the bot's.
				return tradeCancelAction()
			}
			t.offer = b.valuableGoods(t.PartnerID, t.Offerable)
			t.valuable = len(t.offer) > 0
			if goods, ok := b.tradeGoods(t.Offerable); !t.valuable && ok {
				t.offer = []tradeOffer{{row: goods}}
			}
			if t.offer == nil {
				return tradeCancelAction()
			}
		}
		if len(t.Own) < len(t.offer) {
			if t.OtherOk {
				// The partner's confirm locked the offers before the bot
				// offered them all.
				return tradeCancelAction()
			}
			if t.added > len(t.Own) {
				// An add waits for its TradeOwnAdd.
				return Action{}
			}
			return addTradeItemAction(t.offer[t.added].row)
		}
		if !b.tradePaid() {
			if t.OtherOk {
				return tradeCancelAction()
			}
			return Action{}
		}
	}
	if !b.inTradeRange(t.PartnerID, now) {
		return Action{}
	}
	return tradeConfirmAction()
}

// tradeReceivable reports whether the bot takes every item the partner
// offered: adena only from a partner not exempt, and from an exempt one any
// item while the bot's load, with the offer's weight when the item weights
// are known, is at most MaxLoad percent. An offered item the loaded weight
// table lacks is refused.
func (b *Brain) tradeReceivable(exempt bool) bool {
	s, t := &b.world.Self, b.cfg.Travel
	weighs := t != nil && len(t.weights) > 0
	limit := int64(b.persona.Trade.MaxLoad) * int64(s.MaxLoad)
	load := int64(s.Load)
	for _, it := range b.world.Trade.Other {
		if it.TemplateID == adenaID {
			continue
		}
		if !exempt || s.MaxLoad <= 0 || int64(s.Load)*100 > limit {
			return false
		}
		if weighs {
			w, ok := t.weights[it.TemplateID]
			if !ok {
				return false
			}
			load += int64(w) * int64(max(it.Count, 1))
		}
	}
	return load*100 <= limit || load == int64(s.Load)
}

// tradePrice is the unit price the bot sells carried item it at in a
// trade: the policy's price, or else its store's price of that loot
// (storePrice): the one its store's manage lists showed, else the one of
// the travel map's reference price, so its store and its trades sell at
// the same prices. An item an NPC shop sells, or the bot keeps, has no
// store price; one the keep hook keeps (Persona.Holds) has none at all.
func (b *Brain) tradePrice(it Item) (int32, bool) {
	if b.hold(it).Keep {
		return 0, false
	}
	id := it.TemplateID
	if p, ok := b.persona.Trade.Prices[id]; ok {
		return p, true
	}
	if p, ok := b.store.prices[id]; ok {
		return p, true
	}
	m := b.cfg.Travel
	if m == nil || b.keeps(id) {
		return 0, false
	}
	if _, shopSells := m.cheapest[id]; shopSells {
		return 0, false
	}
	ref, ok := m.prices[id]
	if !ok {
		return 0, false
	}
	p := b.storePrice(ref)
	return p, p > 0
}

// tradePaid reports whether the partner's adena covers the price of the
// bot's offer (tradeDue).
func (b *Brain) tradePaid() bool {
	var paid int64
	for _, it := range b.world.Trade.Other {
		if it.TemplateID == adenaID {
			paid += int64(it.Count)
		}
	}
	price := b.tradeDue()
	return price > 0 && paid >= price
}

// tradeDue is the price of the bot's offer: of valuables the sum of each
// row's unit price to the partner (valuableUnit) times its units offered,
// 0 with a row the bot did not pick (Trade.offer); of other goods their
// trade prices (tradePrice).
func (b *Brain) tradeDue() int64 {
	t := &b.world.Trade
	var price int64
	if t.valuable {
		for _, own := range t.Own {
			i := slices.IndexFunc(t.offer, func(o tradeOffer) bool { return o.row.ObjectID == own.ObjectID })
			if i < 0 {
				return 0
			}
			price += b.valuableUnit(t.PartnerID, t.offer[i].key) * int64(own.Count)
		}
		return price
	}
	for _, own := range t.Own {
		it, ok := b.world.Inventory.Items[own.ObjectID]
		if !ok {
			it = Item{ObjectID: own.ObjectID, TemplateID: own.TemplateID, Count: own.Count}
		}
		unit, _ := b.tradePrice(it)
		price += int64(unit) * int64(own.Count)
	}
	return price
}

// inTradeRange reports whether the player partner is in sight and within
// tradeRange-tradeRangeMargin of the bot at now.
func (b *Brain) inTradeRange(partner int32, now time.Time) bool {
	o := b.world.Objects[partner]
	if o == nil {
		return false
	}
	a, c := b.world.Self.Position(now), o.Position(now)
	dz := int64(a.Z - c.Z)
	const r = tradeRange - tradeRangeMargin
	return dist2(a, c)+dz*dz <= r*r
}

// tradeSent records a trade packet the bot sent.
func (b *Brain) tradeSent(a Action, now time.Time) {
	t := &b.world.Trade
	switch a.kind {
	case actAnswerTrade:
		// The server takes the request with the first answer.
		b.world.Prompts.Trade = TradeInvite{}
		if a.object != 0 {
			for id, at := range b.tradedAt {
				if now.Sub(at) >= b.persona.Trade.Every {
					delete(b.tradedAt, id)
				}
			}
			b.tradedAt[a.object] = now
		}
	case actTradeAdd:
		t.added++
	case actTradeConfirm:
		t.confirmed = true
		if t.valuable {
			t.sale = b.tradeDue()
		}
	case actTradeCancel:
		t.cancelAt = now
	case actTradeRequest:
		b.inviteSent(a.object, now)
	}
}
