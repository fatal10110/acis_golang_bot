package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// storeRowSize and storeListedRowSize are the wire sizes of a candidate row
// and of a listed row (with its reference price).
const (
	storeRowSize       = 4 + 4 + 4 + 4 + 2 + 2 + 2 + 4 + 4
	storeListedRowSize = storeRowSize + 4
)

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

func (PrivateStoreManageListSell) IsEvent() {}

func (PrivateStoreListSell) IsEvent() {}

func (PrivateStoreMsgSell) IsEvent() {}

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
