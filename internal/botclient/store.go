package botclient

import (
	"math"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Private sell store timing and limits.
const (
	// storeReplyTimeout is how long an opening waits for the manage list
	// after RequestPrivateStoreManageSell, and for the UserInfo showing
	// the store open after SetPrivateStoreListSell. The server refuses a
	// seated or standing-up player, a busy one and a dead one without a
	// word (network/privatestore.go tryOpenStore), and closes the store on
	// a list it refuses, mostly without one (setSellStoreList).
	storeReplyTimeout = 5 * time.Second
	// storeStandSettle is how long after its stand-up the bot waits before
	// it opens: the server refuses a player still standing up, for its
	// sitStandDelay of 2.5 s (player/character_state.go).
	storeStandSettle = 3 * time.Second
	// storeTitleMax is the longest store title in UTF-16 units: the server
	// drops a longer one silently (privatestore.MaxMessageLength).
	storeTitleMax = 29
)

// Store types of UserInfo and CharInfo (privatestore.OperateType) the
// bot's store shows once open.
const (
	storeTypeSell        = 1
	storeTypePackageSell = 8
)

// StorePolicy is how a merchant bot sells its loot in a private sell store
// (rule "store"). Every value is a configurable default (D6); the store
// registry (BrainConfig.Stores) holds the process-wide limits.
type StorePolicy struct {
	// Budget is the adena the bot's store sales of loot may bring in per
	// session; a store lists no more loot than what is left of it. 0 lists
	// no loot: a store opens then only for released valuables
	// (docs/valuables/release.md), under the valuables budget.
	Budget int64
	// Title is the store title; empty, or longer than storeTitleMax UTF-16
	// units, sends none.
	Title string
	// MinMarkup and MaxMarkup bound the store price in percent of the NPC
	// buy-back price (half the item's reference price); each session
	// draws one markup between them, so its store and its trades sell at
	// the same prices. MaxMarkup is under 200, which keeps the price under
	// the reference price an NPC shop sells at.
	MinMarkup, MaxMarkup int
	// MaxRows is the most rows a store lists: the server closes a store
	// listing more than its owner's PrivateSellStoreLimit (4 for every race
	// but dwarves by default, plus a stat).
	MaxRows int
	// Retry is how long after a store closed, or an opening failed, the
	// bot opens no other.
	Retry time.Duration
}

// defaultStorePolicy prices loot at 1.2 to 1.5 times the NPC buy-back
// price (docs/plan.md, merchant persona) and opens no store: a merchant
// sets a Budget.
func defaultStorePolicy() StorePolicy {
	return StorePolicy{Title: "Loot for sale", MinMarkup: 120, MaxMarkup: 150, MaxRows: 4, Retry: 5 * time.Minute}
}

// storeRow is one row of a sell list or a purchase from a store: an item
// object, how many and the unit price.
type storeRow struct{ object, count, price int32 }

// listedValuable is a released valuable's row of a store list, its ledger
// entry and the units its object held when listed (a stack may list part
// of them).
type listedValuable struct {
	row    storeRow
	key    valuableKey
	atList int64
}

// encodeRequestPrivateStoreManageSell opens the sell store manage window;
// no body (clientpackets/privatestore.go).
func encodeRequestPrivateStoreManageSell() []byte {
	return wire.NewPacketWriter(opGameRequestPrivateStoreManageSell).Bytes()
}

// encodeSetPrivateStoreMsgSell sets the store title (clientpackets
// DecodeStoreMessage).
func encodeSetPrivateStoreMsgSell(title string) []byte {
	w := wire.NewPacketWriter(opGameSetPrivateStoreMsgSell)
	w.WriteString(title)
	return w.Bytes()
}

// encodeSetPrivateStoreListSell lists rows for sale: packaged (1 sells the
// whole list as one), row count, then per row object id, count and unit
// price (DecodeSetPrivateStoreListSell).
func encodeSetPrivateStoreListSell(packaged bool, rows []storeRow) []byte {
	w := wire.NewPacketWriter(opGameSetPrivateStoreListSell)
	w.WriteInt32(boolInt32(packaged))
	writeStoreRows(w, rows)
	return w.Bytes()
}

// encodeRequestPrivateStoreQuitSell closes the sell store, or leaves its
// manage window; no body.
func encodeRequestPrivateStoreQuitSell() []byte {
	return wire.NewPacketWriter(opGameRequestPrivateStoreQuitSell).Bytes()
}

// encodeRequestPrivateStoreBuy buys rows from the sell store of player
// owner: owner id, row count, then per row object id, count and the
// store's unit price (DecodeRequestPrivateStoreBuy).
func encodeRequestPrivateStoreBuy(owner int32, rows []storeRow) []byte {
	w := wire.NewPacketWriter(opGameRequestPrivateStoreBuy)
	w.WriteInt32(owner)
	writeStoreRows(w, rows)
	return w.Bytes()
}

func writeStoreRows(w *wire.Writer, rows []storeRow) {
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.object)
		w.WriteInt32(r.count)
		w.WriteInt32(r.price)
	}
}

// storeStep is where an opening or an open store stands.
type storeStep uint8

const (
	// storeIdle: no store, no opening under way.
	storeIdle storeStep = iota
	// storeAskManage waits for the manage list.
	storeAskManage
	// storeManage has the manage list; the title goes out next.
	storeManage
	// storeTitled sent the title; the list goes out next.
	storeTitled
	// storeListed waits for the UserInfo showing the store open.
	storeListed
	// storeOpen is open for business.
	storeOpen
	// storeQuit takes a failed opening's manage state down: the server
	// refuses every later opening from it.
	storeQuit
)

// storeState is the bot's private sell store (rule "store"). It is owned
// by the brain goroutine.
type storeState struct {
	step storeStep
	// waitUntil is when an answer the step waits for is given up on.
	waitUntil time.Time
	// candidates are the manage list's candidates of the opening.
	candidates []StoreItem
	// claim is the store registry's claim, held while claimed.
	claim   int
	claimed bool
	// adenaAtOpen is the adena carried when the list went out; sold is the
	// adena the session's stores brought in (StorePolicy.Budget).
	adenaAtOpen, sold int64
	// markup is the session's markup in percent, drawn at its first
	// opening.
	markup int
	// prices are the unit prices of the loot templates the store sells,
	// from the manage lists; trades sell at them too.
	prices map[int32]int32
	// valuables are the released valuable rows the last list carried, by
	// entry: their sales count in the valuables budget (countSold).
	valuables []listedValuable
	// nextAt is when the bot may start the next opening; stoodAt is its
	// last stand-up.
	nextAt, stoodAt time.Time
}

// active reports whether an opening is under way or the store is open:
// store mode, whatever the server shows yet.
func (st *storeState) active() bool { return st.step != storeIdle }

// ruleStore is the merchant's store: it opens a private sell store of the
// bot's loot where it stands, then holds the bot seated in it until it
// closes. It also holds a bot the server shows in a store it did not open.
// A bot told to follow or assist another player leaves its store.
func (b *Brain) ruleStore(now time.Time) (Action, bool) {
	st, s := &b.store, &b.world.Self
	if b.called() {
		b.leaveStore(now)
	}
	switch st.step {
	case storeIdle:
		if s.StoreType != 0 {
			return Action{}, true
		}
		return b.startStore(now)
	case storeAskManage:
		if now.After(st.waitUntil) {
			st.step = storeQuit
		}
	case storeManage:
		if t := b.persona.Store.Title; t != "" && storeTitleFits(t) {
			return Action{kind: actStoreTitle, payload: encodeSetPrivateStoreMsgSell(t)}, true
		}
		st.step = storeTitled
		return b.ruleStore(now)
	case storeTitled:
		rows := b.storeRows()
		if len(rows) == 0 {
			st.step = storeQuit
			return b.ruleStore(now)
		}
		return Action{kind: actStoreList, payload: encodeSetPrivateStoreListSell(false, rows)}, true
	case storeListed:
		switch {
		case s.StoreType == storeTypeSell || s.StoreType == storeTypePackageSell:
			st.step = storeOpen
		case now.After(st.waitUntil):
			st.step = storeQuit
		}
	case storeOpen:
		if s.StoreType == 0 {
			// Sold out, or closed by a teleport: the bot stays seated.
			b.closeStore(now)
		}
	}
	if st.step == storeQuit {
		return Action{kind: actStoreQuit, payload: encodeRequestPrivateStoreQuitSell()}, true
	}
	return Action{}, true
}

// startStore starts an opening when the bot may open a store: a merchant
// (StorePolicy.Budget) with budget left and loot with a store price
// (storeSells), in a town of the store registry with room for it, alive,
// out of combat and PvP flag, not casting, not trading, still, and on no
// errand. A seated bot stands up
// first and waits for the stand to end.
func (b *Brain) startStore(now time.Time) (Action, bool) {
	if !b.storeReady(now) {
		b.releaseStore()
		return Action{}, false
	}
	s := &b.world.Self
	at := s.Position(now)
	if !b.store.claimed && !b.cfg.Stores.hasRoom(at) {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	if now.Sub(b.store.stoodAt) < storeStandSettle {
		return Action{}, true
	}
	if !b.store.claimed {
		id, ok := b.cfg.Stores.claim(at)
		if !ok {
			return Action{}, false
		}
		b.store.claim, b.store.claimed = id, true
	}
	return Action{kind: actStoreManage, payload: encodeRequestPrivateStoreManageSell()}, true
}

// storeReady reports whether the bot may start an opening at now: no
// reopen wait, tradeReady, loot it sells with the store budget left or a
// released valuable with the valuables budget left
// (docs/valuables/release.md "Release to the store"), and neither its own
// trade request awaiting an answer (ownTradeRequest) nor a pending
// valuables round or release withdrawal holding the store (storeHeld).
func (b *Brain) storeReady(now time.Time) bool {
	if now.Before(b.store.nextAt) || !b.tradeReady(now) {
		return false
	}
	loot, valuables := b.persona.Store.Budget > b.store.sold, b.valuablesLeft() > 0
	for _, it := range b.world.Inventory.Items {
		if b.releasedValuable(it) && valuables || loot && b.storeLoot(it) && b.storeSells(it) {
			return !b.ownTradeRequest(now) && !b.storeHeld(now)
		}
	}
	return false
}

// tradeReady holds the state checks a store opening and the valuables
// errands share (docs/valuables/round.md "Start"): a registry and travel
// map, alive, out of combat and PvP flag, not casting, still, no trade
// prompt or window, no shop, trip, newbie, class, dyes or train visit, and
// not called away.
func (b *Brain) tradeReady(now time.Time) bool {
	s, w := &b.world.Self, b.world
	if b.cfg.Stores == nil || b.cfg.Travel == nil {
		return false
	}
	if s.Dead || s.InCombat || s.PvPFlag != 0 || s.Casting(now) || !b.castAt.IsZero() || s.Moving(now) {
		return false
	}
	if w.Prompts.Trade.Pending(now) || w.Trade.Open() {
		return false
	}
	if b.shop.v.active() || b.trip.v.active() || b.newbie.v.active() || b.class.v.active() || b.dyes.v.active() || b.train.npc != 0 {
		return false
	}
	return !b.called()
}

// storeSells reports whether the bot's store sells it: a released valuable
// (releasedValuable), or loot (storeLoot) with a store price, the one its
// store's manage lists showed, else the one of the travel map's reference
// price, the test storeRows applies. An opening with no such item lists
// nothing: its manage list and title go out for no store.
func (b *Brain) storeSells(it Item) bool {
	if b.releasedValuable(it) {
		return true
	}
	if !b.storeLoot(it) {
		return false
	}
	if _, ok := b.store.prices[it.TemplateID]; ok {
		return true
	}
	ref, ok := b.cfg.Travel.prices[it.TemplateID]
	return ok && b.storePrice(ref) > 0
}

// called reports whether the bot follows or assists another player (a GM
// order or its party leader): it stays with that player, and a seated store
// would leave it behind.
func (b *Brain) called() bool {
	f := b.followed()
	return (f != 0 && f != b.world.Self.ObjectID) || b.assisted() != 0
}

// leaveStore takes the store down at now for a bot called away (called):
// an opening under way or an open store quits, counting what it sold, and
// so does a store the server shows that the bot did not open, once Retry
// after the last quit. The server leaves the bot seated
// (privatestore.go quitPrivateStore); once it shows the store closed, the
// follow and assist rules stand the bot up and take over. A store that
// closed already ends in ruleStore without a quit.
func (b *Brain) leaveStore(now time.Time) {
	st, s := &b.store, &b.world.Self
	switch {
	case st.step == storeQuit, st.step == storeOpen && s.StoreType == 0:
	case st.step != storeIdle:
		b.countSold()
		st.step = storeQuit
	case s.StoreType != 0 && !now.Before(st.nextAt):
		st.step = storeQuit
	}
}

// storeLoot reports whether the bot sells it in its store: loot, never an
// item an NPC shop sells (the travel map's buylists), equipped, a quest
// item, adena, augmented or an item the bot keeps (keepsItem: its
// supplies, Persona.Potions, Shots, Shop.Keep and Restock, the wielded
// bow's arrows, wanted spellbooks, what the keep hook keeps) or wears next
// (the gear errand), or a piece of the armor set it buys towards
// (keepsForSet), or a valuable (Brain.valuable). A bot without a travel
// map sells nothing.
func (b *Brain) storeLoot(it Item) bool {
	if b.cfg.Travel == nil || it.Equipped || it.SubCategory == subCategoryQuest || it.Augmentation != 0 || b.keepsItem(it) || b.wearsNext(it) || b.valuable(it) {
		return false
	}
	if w := b.worn(); b.keepsForSet(it, &w) {
		return false
	}
	_, shopSells := b.cfg.Travel.cheapest[it.TemplateID]
	return !shopSells
}

// storePrice is the store's unit price of loot template whose reference
// price is ref: the session's markup on the NPC buy-back price (half the
// reference price, serverpackets/shop_trade.go), under the reference
// price; 0 sells it not.
func (b *Brain) storePrice(ref int32) int32 {
	st, p := &b.store, &b.persona.Store
	if st.markup == 0 {
		lo, hi := max(p.MinMarkup, 1), max(p.MaxMarkup, p.MinMarkup, 1)
		st.markup = lo + b.rng.IntN(hi-lo+1)
	}
	price := int64(ref/2) * int64(st.markup) / 100
	if price <= 0 || price >= int64(ref) {
		return 0
	}
	return int32(price)
}

// storeRows is the list of the opening, at most MaxRows and, with the
// adena carried, within the largest int32 the server takes (privatestore
// FillSell): first the released valuables (valuableStoreRows), kept in
// the store's valuables for countSold; then the candidates that are loot,
// priced (storePrice of the reference price the manage list shows, kept in
// the store's prices), within the store budget left.
func (b *Brain) storeRows() []storeRow {
	st, p := &b.store, &b.persona.Store
	left := p.Budget - st.sold
	var adena int64
	if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
		adena = int64(it.Count)
	}
	room := math.MaxInt32 - adena
	rows := b.valuableStoreRows(&room)
	for _, c := range st.candidates {
		if len(rows) >= p.MaxRows {
			break
		}
		it, ok := b.world.Inventory.Items[c.ObjectID]
		if !ok || !b.storeLoot(it) {
			continue
		}
		if _, ok := st.prices[c.TemplateID]; !ok {
			if st.prices == nil {
				st.prices = make(map[int32]int32)
			}
			if price := b.storePrice(c.ReferencePrice); price > 0 {
				st.prices[c.TemplateID] = price
			}
		}
		price := int64(st.prices[c.TemplateID])
		if price <= 0 {
			continue
		}
		n := min(int64(c.Count), left/price, room/price)
		if n < 1 {
			continue
		}
		rows = append(rows, storeRow{object: c.ObjectID, count: int32(n), price: int32(price)})
		left -= n * price
		room -= n * price
	}
	return rows
}

// valuableStoreRows are the released valuables' rows of the opening
// (docs/valuables/release.md "Valuables budget (D34)"): by score, each
// candidate of a released entry the bot carries at the entry's floor, a
// row's units at most ceil(left / floor) of the valuables budget left, the
// listed value then taken off left and off room; at most MaxRows, none
// once left is spent. It keeps them in the store's valuables.
func (b *Brain) valuableStoreRows(room *int64) []storeRow {
	st := &b.store
	st.valuables = nil
	if !b.persona.Valuables.Enabled || b.cfg.Travel == nil {
		return nil
	}
	left := b.valuablesLeft()
	var rows []storeRow
	for _, k := range b.valuablesByScore(func(e *valuableEntry) bool { return e.released && e.carried > 0 }) {
		floor := b.valuableFloor(k)
		for _, c := range st.candidates {
			if left <= 0 || len(rows) >= b.persona.Store.MaxRows {
				return rows
			}
			it, ok := b.world.Inventory.Items[c.ObjectID]
			if !ok || it.TemplateID != k.template || it.Enchant != k.enchant || !b.releasedValuable(it) || floor <= 0 {
				continue
			}
			n := min(int64(c.Count), ceilDiv(left, floor), *room/floor)
			if n < 1 {
				continue
			}
			r := storeRow{object: c.ObjectID, count: int32(n), price: int32(floor)}
			rows = append(rows, r)
			st.valuables = append(st.valuables, listedValuable{row: r, key: k, atList: int64(it.Count)})
			left -= n * floor
			*room -= n * floor
		}
	}
	return rows
}

// releasedValuable reports whether carried item it is a valuable of a
// released entry (docs/valuables/ledger.md "Released entries"), which the
// store sells.
func (b *Brain) releasedValuable(it Item) bool {
	if !b.persona.Valuables.Enabled || b.cfg.Travel == nil || b.cfg.valuables == nil {
		return false
	}
	return b.cfg.valuables.released(valuableKey{template: it.TemplateID, enchant: it.Enchant}) && b.valuable(it)
}

// storeListed reports the price at which the open store lists entry k, a
// released valuable with units left of its row.
func (b *Brain) storeListed(k valuableKey) (int64, bool) {
	st := &b.store
	if st.step != storeOpen {
		return 0, false
	}
	for _, l := range st.valuables {
		if it, ok := b.world.Inventory.Items[l.row.object]; ok && l.key == k && it.Count > 0 {
			return int64(l.row.price), true
		}
	}
	return 0, false
}

// storeManageList takes the manage list the opening asked for; storeRows
// prices its loot once the title went out, as an item the server just gave
// can show in the list before the InventoryUpdate that brings it. One
// arriving after the list went out is the server refusing a row
// (FillSell): the opening is taken down.
func (b *Brain) storeManageList(e PrivateStoreManageListSell) {
	st := &b.store
	if e.OwnerID != b.world.Self.ObjectID {
		return
	}
	switch st.step {
	case storeAskManage:
		st.candidates, st.step = e.Candidates, storeManage
	case storeTitled, storeListed:
		st.step = storeQuit
	}
}

// storeSent moves the store on once action a went out at now.
func (b *Brain) storeSent(a Action, now time.Time) {
	st := &b.store
	switch a.kind {
	case actStoreManage:
		st.step, st.waitUntil = storeAskManage, now.Add(storeReplyTimeout)
	case actStoreTitle:
		st.step = storeTitled
	case actStoreList:
		st.step, st.waitUntil = storeListed, now.Add(storeReplyTimeout)
		st.adenaAtOpen = 0
		if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
			st.adenaAtOpen = int64(it.Count)
		}
	case actStoreQuit:
		b.endStore(now)
	}
}

// closeStore ends a store that closed at now, counting what it sold.
func (b *Brain) closeStore(now time.Time) {
	b.countSold()
	b.endStore(now)
}

// countSold adds what an open or listed store brought in to the session's
// sales, split between the budgets (docs/valuables/release.md "Valuables
// budget (D34)"): the units each listed valuable row sold (its object's
// units at listing less still carried, at most the row's) times its price
// to the valuables budget, the rest to the store's. Each valuable row that
// sold units counts one store sale (botmetrics.ViaStore).
func (b *Brain) countSold() {
	st := &b.store
	if st.step == storeOpen || st.step == storeListed {
		if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
			gained := max(int64(it.Count)-st.adenaAtOpen, 0)
			var valuables int64
			rows := 0
			for _, l := range st.valuables {
				var kept int64
				if it, ok := b.world.Inventory.Items[l.row.object]; ok {
					kept = int64(it.Count)
				}
				if sold := min(int64(l.row.count), max(l.atList-kept, 0)); sold > 0 {
					valuables += sold * int64(l.row.price)
					rows++
				}
			}
			valuables = min(valuables, gained)
			b.valuablesSold += valuables
			for ; valuables > 0 && rows > 0; rows-- {
				b.metrics.ValuablesSold(botmetrics.ViaStore)
			}
			st.sold += gained - valuables
		}
	}
	st.valuables = nil
}

// endStore ends the opening or the store: the registry claim goes back and
// no other opening starts before Retry.
func (b *Brain) endStore(now time.Time) {
	st := &b.store
	b.releaseStore()
	st.step, st.candidates, st.waitUntil, st.valuables = storeIdle, nil, time.Time{}, nil
	st.nextAt = now.Add(b.persona.Store.Retry)
}

// releaseStore gives back the store registry's claim.
func (b *Brain) releaseStore() {
	st := &b.store
	if st.claimed {
		b.cfg.Stores.release(st.claim)
		st.claim, st.claimed = 0, false
	}
}

// storeTitleFits reports whether title is short enough for a store title.
func storeTitleFits(title string) bool {
	return len(utf16.Encode([]rune(title))) <= storeTitleMax
}
