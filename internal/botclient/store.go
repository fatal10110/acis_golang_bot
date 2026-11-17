package botclient

import (
	"math"
	"time"
	"unicode/utf16"

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
	// storeRowSize and storeListedRowSize are the wire sizes of a
	// candidate row and of a listed row (with its reference price).
	storeRowSize       = 4 + 4 + 4 + 4 + 2 + 2 + 2 + 4 + 4
	storeListedRowSize = storeRowSize + 4
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
	// Budget is the adena the bot's store sales may bring in per session;
	// a store lists no more than what is left of it. 0 opens no store.
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

// StoreItem is one row of a private sell store window.
type StoreItem struct {
	// Type2 is the item's client class (item.Template.Category's second
	// value).
	Type2                       int32
	ObjectID, TemplateID, Count int32
	Enchant                     uint16
	BodySlot                    int32
	// Price is the unit price of a listed row. ReferencePrice is the
	// template's reference price: a candidate row carries it in its price
	// field, so both hold it there.
	Price, ReferencePrice int32
}

// PrivateStoreManageListSell is the bot's sell store manage window: what
// it may list (Candidates) and what it listed.
type PrivateStoreManageListSell struct {
	OwnerID    int32
	Packaged   bool
	Adena      int32
	Candidates []StoreItem
	Listed     []StoreItem
}

// PrivateStoreListSell is a sell store's window as its buyer sees it.
type PrivateStoreListSell struct {
	OwnerID    int32
	Packaged   bool
	BuyerAdena int32
	Items      []StoreItem
}

// PrivateStoreMsgSell is a sell store's title.
type PrivateStoreMsgSell struct {
	OwnerID int32
	Title   string
}

func (PrivateStoreManageListSell) event() {}
func (PrivateStoreListSell) event()       {}
func (PrivateStoreMsgSell) event()        {}

// readStoreItem reads a sell store row: type2, object id, item id, count,
// a zero, enchant, a zero, body slot and price, then for a listed row the
// reference price (serverpackets/privatestore.go writeStoreSellRow,
// writeListedSellRows).
func readStoreItem(r *wire.Reader, listed bool) StoreItem {
	it := StoreItem{Type2: r.ReadInt32(), ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(), Count: r.ReadInt32()}
	r.Skip(2)
	it.Enchant = r.ReadUint16()
	r.Skip(2)
	it.BodySlot, it.Price = r.ReadInt32(), r.ReadInt32()
	it.ReferencePrice = it.Price
	if listed {
		it.ReferencePrice = r.ReadInt32()
	}
	return it
}

// readStoreItems reads an int32 row count and the rows. The loop ends at
// the first short read.
func readStoreItems(r *wire.Reader, listed bool) []StoreItem {
	size := storeRowSize
	if listed {
		size = storeListedRowSize
	}
	n := int(r.ReadInt32())
	items := make([]StoreItem, 0, capFor(r, n, size))
	for ; n > 0 && r.Err() == nil; n-- {
		items = append(items, readStoreItem(r, listed))
	}
	return items
}

// decodePrivateStoreManageListSell: owner id, packaged, adena, the
// candidates, then the listed rows (serverpackets/privatestore.go
// FramePrivateStoreManageListSell).
func decodePrivateStoreManageListSell(r *wire.Reader) Event {
	l := PrivateStoreManageListSell{OwnerID: r.ReadInt32(), Packaged: r.ReadInt32() == 1, Adena: r.ReadInt32()}
	l.Candidates = readStoreItems(r, false)
	l.Listed = readStoreItems(r, true)
	return l
}

// decodePrivateStoreListSell: owner id, packaged, the buyer's adena, then
// the listed rows (FramePrivateStoreListSell).
func decodePrivateStoreListSell(r *wire.Reader) Event {
	l := PrivateStoreListSell{OwnerID: r.ReadInt32(), Packaged: r.ReadInt32() == 1, BuyerAdena: r.ReadInt32()}
	l.Items = readStoreItems(r, true)
	return l
}

// decodePrivateStoreMsgSell: owner id, title (frameStoreMessage).
func decodePrivateStoreMsgSell(r *wire.Reader) Event {
	return PrivateStoreMsgSell{OwnerID: r.ReadInt32(), Title: r.ReadString()}
}

// storeRow is one row of a sell list or a purchase from a store: an item
// object, how many and the unit price.
type storeRow struct{ object, count, price int32 }

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
func (b *Brain) ruleStore(now time.Time) (Action, bool) {
	st, s := &b.store, &b.world.Self
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
// (StorePolicy.Budget) with budget left and loot to sell, in a town of the
// store registry with room for it, alive, out of combat and PvP flag, not
// casting, not trading, still, and on no errand. A seated bot stands up
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

// storeReady reports whether the bot may start an opening at now.
func (b *Brain) storeReady(now time.Time) bool {
	p, s, w := &b.persona.Store, &b.world.Self, b.world
	if p.Budget <= b.store.sold || b.cfg.Stores == nil || b.cfg.Travel == nil || now.Before(b.store.nextAt) {
		return false
	}
	if s.Dead || s.InCombat || s.PvPFlag != 0 || s.Casting(now) || !b.castAt.IsZero() || s.Moving(now) {
		return false
	}
	if w.Prompts.Trade.Pending(now) || w.Trade.Open() {
		return false
	}
	if b.shop.v.active() || b.trip.v.active() || b.newbie.v.active() || b.train.npc != 0 {
		return false
	}
	for _, it := range w.Inventory.Items {
		if b.storeLoot(it) {
			return true
		}
	}
	return false
}

// storeLoot reports whether the bot sells it in its store: loot, never an
// item an NPC shop sells (the travel map's buylists), equipped, a quest
// item, adena, augmented or an item the bot keeps (Persona.Potions, Shots,
// Shop.Keep and Restock, wanted spellbooks).
func (b *Brain) storeLoot(it Item) bool {
	if it.Equipped || it.SubCategory == subCategoryQuest || it.Augmentation != 0 || b.keeps(it.TemplateID) {
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

// storeRows is the list of the opening: the candidates that are loot,
// priced, at most MaxRows, within the budget left and, with the adena
// carried, within the largest int32 the server takes (privatestore
// FillSell).
func (b *Brain) storeRows() []storeRow {
	st, p := &b.store, &b.persona.Store
	left := p.Budget - st.sold
	var adena int64
	if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
		adena = int64(it.Count)
	}
	room := math.MaxInt32 - adena
	var rows []storeRow
	for _, c := range st.candidates {
		if len(rows) >= p.MaxRows {
			break
		}
		it, ok := b.world.Inventory.Items[c.ObjectID]
		if !ok || !b.storeLoot(it) {
			continue
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

// storeManageList takes the manage list the opening asked for, pricing its
// loot. One arriving after the list went out is the server refusing a row
// (FillSell): the opening is taken down.
func (b *Brain) storeManageList(e PrivateStoreManageListSell) {
	st := &b.store
	if e.OwnerID != b.world.Self.ObjectID {
		return
	}
	switch st.step {
	case storeAskManage:
		st.candidates, st.step = e.Candidates, storeManage
		if st.prices == nil {
			st.prices = make(map[int32]int32)
		}
		for _, c := range e.Candidates {
			it, ok := b.world.Inventory.Items[c.ObjectID]
			if !ok || !b.storeLoot(it) {
				continue
			}
			if price := b.storePrice(c.ReferencePrice); price > 0 {
				st.prices[c.TemplateID] = price
			}
		}
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
	st := &b.store
	if st.step == storeOpen || st.step == storeListed {
		if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
			st.sold += max(int64(it.Count)-st.adenaAtOpen, 0)
		}
	}
	b.endStore(now)
}

// endStore ends the opening or the store: the registry claim goes back and
// no other opening starts before Retry.
func (b *Brain) endStore(now time.Time) {
	st := &b.store
	b.releaseStore()
	st.step, st.candidates, st.waitUntil = storeIdle, nil, time.Time{}
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
