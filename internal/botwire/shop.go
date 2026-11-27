package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// ShopItem is one row of a merchant's buy or sell window: what the
// merchant sells at Price, or what the bot may sell it at Price each.
type ShopItem struct {
	// ObjectID is the bot's item in a sell window, the template id in a
	// buy window.
	ObjectID, TemplateID int32
	// Count is the bot's count in a sell window; in a buy window the stock
	// left of a limited product, 0 for an unlimited one.
	Count                 int32
	Category, SubCategory uint16
	BodySlot              int32
	Enchant               uint16
	Price                 int32
}

// BuyList opens a merchant's buy window on buylist ListID; Money is the
// bot's adena.
type BuyList struct {
	Money, ListID int32
	Items         []ShopItem
}

// SellList opens a merchant's sell window on what the bot may sell; Money
// is its adena. ListID is the list a RequestSellItem names (always 0).
type SellList struct {
	Money, ListID int32
	Items         []ShopItem
}

func (BuyList) IsEvent() {}

func (SellList) IsEvent() {}

// shopItemSize is the wire size of a window row.
const shopItemSize = 2 + 4 + 4 + 4 + 2 + 2 + 4 + 2 + 2 + 2 + 4

// readShopItem reads a window row: category, object id, template id,
// count, subcategory, custom type 1, body slot, enchant, custom type 2, a
// zero, price (serverpackets/shop_trade.go writeShopItem).
func readShopItem(r *wire.Reader) ShopItem {
	it := ShopItem{Category: r.ReadUint16(), ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(), Count: r.ReadInt32(), SubCategory: r.ReadUint16()}
	r.Skip(2) // custom type 1
	it.BodySlot, it.Enchant = r.ReadInt32(), r.ReadUint16()
	r.Skip(4) // custom type 2 and a zero
	it.Price = r.ReadInt32()
	return it
}

// decodeBuyList: adena, list id, row count as uint16, then the rows
// (serverpackets/shop_trade.go writeBuyList). A sold-out product of
// limited stock is counted but not written, so the rows end at the end of
// the frame, short of the count.
func decodeBuyList(r *wire.Reader) Event {
	l := BuyList{Money: r.ReadInt32(), ListID: r.ReadInt32()}
	n := int(r.ReadUint16())
	l.Items = make([]ShopItem, 0, capFor(r, n, shopItemSize))
	for ; n > 0 && r.Err() == nil && r.Remaining() > 0; n-- {
		l.Items = append(l.Items, readShopItem(r))
	}
	return l
}

// decodeSellList: adena, a zero list id, row count as uint16, then the
// rows (serverpackets/shop_trade.go writeSellList). The loop ends at the
// first short read.
func decodeSellList(r *wire.Reader) Event {
	l := SellList{Money: r.ReadInt32(), ListID: r.ReadInt32()}
	n := int(r.ReadUint16())
	l.Items = make([]ShopItem, 0, capFor(r, n, shopItemSize))
	for ; n > 0 && r.Err() == nil; n-- {
		l.Items = append(l.Items, readShopItem(r))
	}
	return l
}

// BuyRow is one row of a purchase: an item template and how many.
type BuyRow struct{ Item, Count int32 }

// SellRow is one row of a sale: a carried item, its template and how many.
type SellRow struct{ Object, Item, Count int32 }

// EncodeRequestBuyItem buys rows from buylist listID: list id, row count,
// then per row item id and count (clientpackets/shop_trade.go).
func EncodeRequestBuyItem(listID int32, rows []BuyRow) []byte {
	w := wire.NewPacketWriter(OpGameRequestBuyItem)
	w.WriteInt32(listID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.Item)
		w.WriteInt32(r.Count)
	}
	return w.Bytes()
}

// EncodeRequestSellItem sells rows to the targeted merchant: list id, row
// count, then per row object id, item id and count
// (clientpackets/shop_trade.go).
func EncodeRequestSellItem(listID int32, rows []SellRow) []byte {
	w := wire.NewPacketWriter(OpGameRequestSellItem)
	w.WriteInt32(listID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.Object)
		w.WriteInt32(r.Item)
		w.WriteInt32(r.Count)
	}
	return w.Bytes()
}
