package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
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

// SendTradeRequest asks the bot to trade with the player SenderID.
type SendTradeRequest struct{ SenderID int32 }

// TradeItem is an item row of the trade window (writeTradeItem in
// serverpackets/shop_trade.go). Count is the quantity the row is about: an
// add's added quantity, an availability row's quantity left.
type TradeItem struct {
	ObjectID, TemplateID     int32
	Count                    int32
	Category, SubCategory    uint16
	CustomType1, CustomType2 uint16
	BodySlot                 int32
	Enchant                  uint16
}

// TradeStart opens the trade window with the player PartnerID. Items are
// the items the bot may offer: in its inventory (not equipped), tradable
// and no quest item (availableTradeStartItems).
type TradeStart struct {
	PartnerID int32
	Items     []TradeItem
}

// TradeOwnAdd confirms an item the bot added to its offer.
type TradeOwnAdd struct{ Item TradeItem }

// TradeOtherAdd reports an item the partner added to its offer.
type TradeOtherAdd struct{ Item TradeItem }

// TradeAvail is a row of TradeItemUpdate: Mode 3 for a stackable item with
// some left, 2 otherwise.
type TradeAvail struct {
	Mode uint16
	Item TradeItem
}

// TradeItemUpdate refreshes what the bot may still offer after one of its
// adds. TradeUpdate (the one item added) and TradeItemUpdate (every item)
// share the opcode and the layout, so one event carries both.
type TradeItemUpdate struct{ Items []TradeAvail }

// TradePressOwnOk confirms the bot's own confirm of the trade.
type TradePressOwnOk struct{}

// TradePressOtherOk reports the partner's confirm of the trade.
type TradePressOtherOk struct{}

// SendTradeDone ends the trade: settled when Success, else cancelled.
type SendTradeDone struct{ Success bool }

func (SendTradeRequest) event()  {}
func (TradeStart) event()        {}
func (TradeOwnAdd) event()       {}
func (TradeOtherAdd) event()     {}
func (TradeItemUpdate) event()   {}
func (TradePressOwnOk) event()   {}
func (TradePressOtherOk) event() {}
func (SendTradeDone) event()     {}

// readTradeItem reads a trade item row: category, object id, template id,
// count, subcategory, custom type 1, body slot, enchant, custom type 2 and
// a zero (writeTradeItem).
func readTradeItem(r *wire.Reader) TradeItem {
	it := TradeItem{
		Category: r.ReadUint16(), ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(),
		Count: r.ReadInt32(), SubCategory: r.ReadUint16(), CustomType1: r.ReadUint16(),
		BodySlot: r.ReadInt32(), Enchant: r.ReadUint16(), CustomType2: r.ReadUint16(),
	}
	r.Skip(2)
	return it
}

// decodeTradeStart: partner id, row count as uint16, then the rows. The
// count comes off the wire, so the loop ends at the first short read.
func decodeTradeStart(r *wire.Reader) Event {
	e := TradeStart{PartnerID: r.ReadInt32()}
	for n := r.ReadUint16(); n > 0 && r.Err() == nil; n-- {
		e.Items = append(e.Items, readTradeItem(r))
	}
	return e
}

// decodeTradeOwnAdd and decodeTradeOtherAdd: a row count, always 1
// (frameTradeAdd), then the row.
func decodeTradeOwnAdd(r *wire.Reader) Event {
	r.Skip(2)
	return TradeOwnAdd{Item: readTradeItem(r)}
}

func decodeTradeOtherAdd(r *wire.Reader) Event {
	r.Skip(2)
	return TradeOtherAdd{Item: readTradeItem(r)}
}

// decodeTradeItemUpdate: row count as uint16, then per row its mode as
// uint16 and the row (FrameTradeUpdate, writeTradeItemUpdate). The loop
// ends at the first short read.
func decodeTradeItemUpdate(r *wire.Reader) Event {
	var e TradeItemUpdate
	for n := r.ReadUint16(); n > 0 && r.Err() == nil; n-- {
		e.Items = append(e.Items, TradeAvail{Mode: r.ReadUint16(), Item: readTradeItem(r)})
	}
	return e
}

// encodeTradeRequest asks the player objectID to trade with the bot.
func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(opGameTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

// encodeAnswerTradeRequest answers the pending trade request: 1 accepts, 0
// declines.
func encodeAnswerTradeRequest(accept bool) []byte {
	w := wire.NewPacketWriter(opGameAnswerTradeRequest)
	w.WriteInt32(boolInt32(accept))
	return w.Bytes()
}

// encodeAddTradeItem adds count of the item objectID to the bot's offer.
// The leading trade id is read and ignored by the server
// (handleAddTradeItem); the bot sends 0.
func encodeAddTradeItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(opGameAddTradeItem)
	w.WriteInt32(0)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

// encodeTradeDone confirms the trade (1) or cancels it (0).
func encodeTradeDone(confirm bool) []byte {
	w := wire.NewPacketWriter(opGameTradeDone)
	w.WriteInt32(boolInt32(confirm))
	return w.Bytes()
}

func boolInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// answerTradeAction answers the open trade request; an accepted one names
// partner, for the per-partner rate limit.
func answerTradeAction(accept bool, partner int32) Action {
	a := Action{kind: actAnswerTrade, payload: encodeAnswerTradeRequest(accept)}
	if accept {
		a.object = partner
	}
	return a
}

func addTradeItemAction(it TradeItem) Action {
	return Action{kind: actTradeAdd, payload: encodeAddTradeItem(it.ObjectID, it.Count)}
}

func tradeConfirmAction() Action {
	return Action{kind: actTradeConfirm, payload: encodeTradeDone(true)}
}

func tradeCancelAction() Action {
	return Action{kind: actTradeCancel, payload: encodeTradeDone(false)}
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
	// added and confirmed are set once the bot sent its add and its
	// confirm, cancelAt once it sent its cancel: each goes out once a
	// trade.
	added, confirmed bool
	cancelAt         time.Time
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
	// other than adena an exempt partner offers. The wire carries no item
	// weight, so the bot can only check the load it already carries; the
	// server refuses an overweight trade itself at settlement
	// (ReceiverWeightFits). Other partners may offer only adena.
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
// the store's price.
func (b *Brain) tradeGoods(rows []TradeItem) (TradeItem, bool) {
	for _, row := range rows {
		if row.Count <= 0 || row.SubCategory == subCategoryQuest {
			continue
		}
		it, ok := b.world.Inventory.Items[row.ObjectID]
		if !ok || it.Equipped || it.SubCategory == subCategoryQuest {
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
// or with no goods for sale.
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
	if last, ok := b.tradedAt[partner]; ok && now.Sub(last) < p.Every {
		return false
	}
	_, ok := b.tradeGoods(b.inventoryRows())
	return ok
}

// tradeStep is the bot's next move in its open trade. It offers its goods,
// confirms once the partner's adena covers their price and the partner
// stands within range, and cancels a trade that cannot settle: timed out,
// the bot in combat, an item refused, or the offers locked before the bot
// offered or short of the price.
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
		if len(t.Own) == 0 {
			if t.OtherOk {
				// The partner's confirm locked the offers before the bot's.
				return tradeCancelAction()
			}
			if t.added {
				return Action{}
			}
			goods, ok := b.tradeGoods(t.Offerable)
			if !ok {
				return tradeCancelAction()
			}
			return addTradeItemAction(goods)
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
// item while the bot's load is at most MaxLoad percent.
func (b *Brain) tradeReceivable(exempt bool) bool {
	s := &b.world.Self
	for _, it := range b.world.Trade.Other {
		if it.TemplateID == adenaID {
			continue
		}
		if !exempt || s.MaxLoad <= 0 || int64(s.Load)*100 > int64(b.persona.Trade.MaxLoad)*int64(s.MaxLoad) {
			return false
		}
	}
	return true
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
// bot's offer.
func (b *Brain) tradePaid() bool {
	t := &b.world.Trade
	var price, paid int64
	for _, own := range t.Own {
		it, ok := b.world.Inventory.Items[own.ObjectID]
		if !ok {
			it = Item{ObjectID: own.ObjectID, TemplateID: own.TemplateID, Count: own.Count}
		}
		unit, _ := b.tradePrice(it)
		price += int64(unit) * int64(own.Count)
	}
	for _, it := range t.Other {
		if it.TemplateID == adenaID {
			paid += int64(it.Count)
		}
	}
	return price > 0 && paid >= price
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
		t.added = true
	case actTradeConfirm:
		t.confirmed = true
	case actTradeCancel:
		t.cancelAt = now
	}
}
