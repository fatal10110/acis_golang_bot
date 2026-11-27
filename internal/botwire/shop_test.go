package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Items of the shop tests (aCis_datapack/data/xml/items).
const (
	animalSkin   int32 = 1867
	leatherShirt int32 = 1146
	healBook     int32 = 1152
)

// shopRows is a window's rows as packet fields.
func shopRows(items ...ShopItem) []any {
	var f []any
	for _, it := range items {
		f = append(f, it.Category, it.ObjectID, it.TemplateID, it.Count, it.SubCategory, uint16(0), it.BodySlot, it.Enchant, uint16(0), uint16(0), it.Price)
	}
	return f
}

func shopWindow(op byte, money, listID int32, count uint16, items ...ShopItem) []byte {
	return packet(op, append([]any{money, listID, count}, shopRows(items...)...)...)
}

var (
	potionOffer = ShopItem{Category: 5, ObjectID: lesserHealingPotion, TemplateID: lesserHealingPotion, SubCategory: 5, Price: 103}
	shotOffer   = ShopItem{Category: 5, ObjectID: spiritshotNoGrade, TemplateID: spiritshotNoGrade, SubCategory: 5, Price: 17}
	bookOffer   = ShopItem{Category: 5, ObjectID: healBook, TemplateID: healBook, Count: 3, SubCategory: 5, Price: 110}
	skinRow     = ShopItem{Category: 5, ObjectID: 102, TemplateID: animalSkin, Count: 10, SubCategory: 5, Price: 5}
)

func TestDecodeShopWindows(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// 100 adena, list 14, one row: Lesser Healing Potion, unlimited, at 103.
		{
			"BuyList bytes",
			[]byte{
				0x11, 100, 0, 0, 0, 14, 0, 0, 0, 1, 0,
				5, 0, 0x24, 4, 0, 0, 0x24, 4, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 103, 0, 0, 0,
			},
			BuyList{Money: 100, ListID: 14, Items: []ShopItem{potionOffer}},
		},
		{"BuyList", shopWindow(OpGameBuyList, 5000, 29, 2, bookOffer, potionOffer), BuyList{Money: 5000, ListID: 29, Items: []ShopItem{bookOffer, potionOffer}}},
		// A sold-out product of limited stock is counted, not written.
		{"BuyList sold out", shopWindow(OpGameBuyList, 5000, 29, 3, potionOffer), BuyList{Money: 5000, ListID: 29, Items: []ShopItem{potionOffer}}},
		{"BuyList empty", shopWindow(OpGameBuyList, 0, 14, 0), BuyList{ListID: 14, Items: []ShopItem{}}},
		{"SellList", shopWindow(OpGameSellList, 300, 0, 1, skinRow), SellList{Money: 300, Items: []ShopItem{skinRow}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeShopWindowsRejects(t *testing.T) {
	buy := shopWindow(OpGameBuyList, 1, 14, 1, potionOffer)
	sell := shopWindow(OpGameSellList, 1, 0, 1, skinRow)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"BuyList short header", packet(OpGameBuyList, 1, 14), wire.ErrShortPacket},
		{"BuyList short row", buy[:len(buy)-1], wire.ErrShortPacket},
		{"BuyList trailing", append(shopWindow(OpGameBuyList, 1, 14, 0), 0), ErrTrailingBytes},
		{"BuyList rows past the count", shopWindow(OpGameBuyList, 1, 14, 1, potionOffer, shotOffer), ErrTrailingBytes},
		{"SellList short row", sell[:len(sell)-4], wire.ErrShortPacket},
		{"SellList count past the frame", shopWindow(OpGameSellList, 1, 0, 2, skinRow), wire.ErrShortPacket},
		{"SellList trailing", append(sell, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
