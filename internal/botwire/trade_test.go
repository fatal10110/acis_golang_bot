package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

const (
	goodsID = int32(0x40000001) // the bot's goods for sale
	potID   = int32(0x40000002) // an unpriced item
	coinID  = int32(0x40000003) // the partner's adena
	bladeID = int32(0x40000004) // a blade the partner offers
)

var (
	goodsRow = TradeItem{ObjectID: goodsID, TemplateID: 1864, Count: 10, Category: 4, SubCategory: 5}
	bladeRow = TradeItem{ObjectID: bladeID, TemplateID: 1, Count: 1, CustomType1: 1, BodySlot: 128, Enchant: 3, CustomType2: 2}
)

func adenaRow(n int32) TradeItem {
	return TradeItem{ObjectID: coinID, TemplateID: adenaID, Count: n, Category: 4, SubCategory: 4}
}

// tradeRow is a trade item row's fields in the server's order
// (writeTradeItem).
func tradeRow(it TradeItem) []any {
	return []any{
		it.Category, it.ObjectID, it.TemplateID, it.Count, it.SubCategory, it.CustomType1,
		it.BodySlot, it.Enchant, it.CustomType2, uint16(0),
	}
}

func TestDecodeTradeEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"SendTradeRequest bytes", []byte{0x5e, 2, 0, 0, 0x20}, SendTradeRequest{SenderID: pcID}},
		{"SendTradeDone success", packet(OpGameSendTradeDone, 1), SendTradeDone{Success: true}},
		{"SendTradeDone cancelled", packet(OpGameSendTradeDone, 0), SendTradeDone{}},
		{"TradePressOwnOk", packet(OpGameTradePressOwnOk), TradePressOwnOk{}},
		{"TradePressOtherOk", packet(OpGameTradePressOtherOk), TradePressOtherOk{}},
		{
			"TradeStart", packet(OpGameTradeStart, append(append([]any{pcID, uint16(2)}, tradeRow(goodsRow)...), tradeRow(bladeRow)...)...),
			TradeStart{PartnerID: pcID, Items: []TradeItem{goodsRow, bladeRow}},
		},
		{"TradeStart empty", packet(OpGameTradeStart, pcID, uint16(0)), TradeStart{PartnerID: pcID}},
		{"TradeOwnAdd", packet(OpGameTradeOwnAdd, append([]any{uint16(1)}, tradeRow(goodsRow)...)...), TradeOwnAdd{Item: goodsRow}},
		{"TradeOtherAdd", packet(OpGameTradeOtherAdd, append([]any{uint16(1)}, tradeRow(adenaRow(500))...)...), TradeOtherAdd{Item: adenaRow(500)}},
		{
			"TradeUpdate", packet(OpGameTradeItemUpdate, append([]any{uint16(1), uint16(3)}, tradeRow(goodsRow)...)...),
			TradeItemUpdate{Items: []TradeAvail{{Mode: 3, Item: goodsRow}}},
		},
		{
			"TradeItemUpdate", packet(OpGameTradeItemUpdate, append(append([]any{uint16(2), uint16(3)}, tradeRow(goodsRow)...), append([]any{uint16(2)}, tradeRow(bladeRow)...)...)...),
			TradeItemUpdate{Items: []TradeAvail{{Mode: 3, Item: goodsRow}, {Mode: 2, Item: bladeRow}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v", ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeTradeEventsRejects(t *testing.T) {
	start := packet(OpGameTradeStart, append([]any{pcID, uint16(1)}, tradeRow(goodsRow)...)...)
	add := packet(OpGameTradeOtherAdd, append([]any{uint16(1)}, tradeRow(goodsRow)...)...)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"SendTradeRequest short", []byte{OpGameSendTradeRequest, 1, 0}, wire.ErrShortPacket},
		{"TradeStart short of its count", packet(OpGameTradeStart, append([]any{pcID, uint16(2)}, tradeRow(goodsRow)...)...), wire.ErrShortPacket},
		{"TradeStart trailing", append(start, 0), ErrTrailingBytes},
		{"TradeOtherAdd without its zero", add[:len(add)-2], wire.ErrShortPacket},
		{"TradeOtherAdd trailing", append(add, 0), ErrTrailingBytes},
		{"TradeItemUpdate short row", packet(OpGameTradeItemUpdate, uint16(1), uint16(3), uint16(4)), wire.ErrShortPacket},
		{"TradePressOtherOk trailing", packet(OpGameTradePressOtherOk, 0), ErrTrailingBytes},
		{"SendTradeDone short", packet(OpGameSendTradeDone, uint16(1)), wire.ErrShortPacket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
