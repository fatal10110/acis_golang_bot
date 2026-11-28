package botwire

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The rows of multisell 500 the priest trip trades (multisell/500.xml):
// a Greater CP Potion for 600 Ancient Adena and 50 adena (500 at a 10 %
// castle tax), a CP Potion for 240 and 20, as the server prepares them.
var (
	greaterCPRow = MultiSellEntry{
		ID: 34, Stackable: true,
		Products:    []MultiSellItem{{ItemID: 5592, Type2: 5, Count: 1}},
		Ingredients: []MultiSellItem{{ItemID: 5575, Type2: 4, Count: 600}, {ItemID: 57, Type2: 4, Count: 50}},
	}
	cpRow = MultiSellEntry{
		ID: 33, Stackable: true,
		Products:    []MultiSellItem{{ItemID: 5591, Type2: 5, Count: 1}},
		Ingredients: []MultiSellItem{{ItemID: 5575, Type2: 4, Count: 240}, {ItemID: 57, Type2: 4, Count: 20}},
	}
	// msBladeRow is a non-stackable product with a body part and an enchant.
	msBladeRow = MultiSellEntry{
		ID:          2,
		Products:    []MultiSellItem{{ItemID: 6579, BodySlot: 0x4000, Count: 1, Enchant: 3}},
		Ingredients: []MultiSellItem{{ItemID: 6362, Type2: 5, Count: 9, Enchant: 0}},
	}
)

// multiSellPage builds a MultiSellList page as FrameMultiSellList writes it.
func multiSellPage(list, page, finished, count int, rows ...MultiSellEntry) []byte {
	f := []any{list, page, finished, 40, count}
	for _, e := range rows {
		st := byte(0)
		if e.Stackable {
			st = 1
		}
		f = append(f, e.ID, 0, 0, st, uint16(len(e.Products)), uint16(len(e.Ingredients)))
		for _, p := range e.Products {
			f = append(f, uint16(p.ItemID), p.BodySlot, p.Type2, p.Count, p.Enchant, 0, 0)
		}
		for _, in := range e.Ingredients {
			f = append(f, uint16(in.ItemID), in.Type2, in.Count, in.Enchant, 0, 0)
		}
	}
	return packet(OpGameMultiSellList, f...)
}

func TestDecodeMultiSellList(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want MultiSellList
	}{
		// List 500, page 1 of 1, one row: the Greater CP Potion.
		{
			"bytes",
			[]byte{
				0xd0, 0xf4, 0x01, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 40, 0, 0, 0, 1, 0, 0, 0,
				34, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 2, 0,
				0xd8, 0x15, 0, 0, 0, 0, 5, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0xc7, 0x15, 4, 0, 0x58, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0x39, 0, 4, 0, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			MultiSellList{ListID: 500, Page: 1, Finished: true, PageSize: 40, Entries: []MultiSellEntry{greaterCPRow}},
		},
		{
			"two rows",
			multiSellPage(500, 1, 1, 2, cpRow, greaterCPRow),
			MultiSellList{ListID: 500, Page: 1, Finished: true, PageSize: 40, Entries: []MultiSellEntry{cpRow, greaterCPRow}},
		},
		{
			"not the last page",
			multiSellPage(9, 1, 0, 1, msBladeRow),
			MultiSellList{ListID: 9, Page: 1, PageSize: 40, Entries: []MultiSellEntry{msBladeRow}},
		},
		{
			"empty page",
			multiSellPage(500, 1, 1, 0),
			MultiSellList{ListID: 500, Page: 1, Finished: true, PageSize: 40, Entries: []MultiSellEntry{}},
		},
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

func TestDecodeMultiSellListRejects(t *testing.T) {
	page := multiSellPage(500, 1, 1, 1, greaterCPRow)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short header", packet(OpGameMultiSellList, 500, 1, 1, 40), wire.ErrShortPacket},
		{"short ingredient", page[:len(page)-1], wire.ErrShortPacket},
		{"count past the frame", multiSellPage(500, 1, 1, 2, greaterCPRow), wire.ErrShortPacket},
		{"rows past the count", multiSellPage(500, 1, 1, 1, cpRow, greaterCPRow), ErrTrailingBytes},
		{"trailing", append(page, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestEncodeMultiSellChoose matches the server's decoder golden bytes
// (clientpackets_warehouse_store_test.go: list 1002, entry 2, amount 9999)
// and the priest trip's Greater CP row.
func TestEncodeMultiSellChoose(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"server vector", EncodeMultiSellChoose(1002, 2, 9999), []byte{0xa7, 0xea, 0x03, 0, 0, 2, 0, 0, 0, 0x0f, 0x27, 0, 0}},
		{"Greater CP", EncodeMultiSellChoose(500, 34, 12), []byte{0xa7, 0xf4, 0x01, 0, 0, 34, 0, 0, 0, 12, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
