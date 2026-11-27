package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

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

func (SendTradeRequest) IsEvent() {}

func (TradeStart) IsEvent() {}

func (TradeOwnAdd) IsEvent() {}

func (TradeOtherAdd) IsEvent() {}

func (TradeItemUpdate) IsEvent() {}

func (TradePressOwnOk) IsEvent() {}

func (TradePressOtherOk) IsEvent() {}

func (SendTradeDone) IsEvent() {}

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

// EncodeTradeRequest asks the player objectID to trade with the bot.
func EncodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(OpGameTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

// EncodeAnswerTradeRequest answers the pending trade request: 1 accepts, 0
// declines.
func EncodeAnswerTradeRequest(accept bool) []byte {
	w := wire.NewPacketWriter(OpGameAnswerTradeRequest)
	w.WriteInt32(boolInt32(accept))
	return w.Bytes()
}

// EncodeAddTradeItem adds count of the item objectID to the bot's offer.
// The leading trade id is read and ignored by the server
// (handleAddTradeItem); the bot sends 0.
func EncodeAddTradeItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(OpGameAddTradeItem)
	w.WriteInt32(0)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

// EncodeTradeDone confirms the trade (1) or cancels it (0).
func EncodeTradeDone(confirm bool) []byte {
	w := wire.NewPacketWriter(OpGameTradeDone)
	w.WriteInt32(boolInt32(confirm))
	return w.Bytes()
}

func boolInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
