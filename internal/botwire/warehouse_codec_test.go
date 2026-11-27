package botwire

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// whRows is a warehouse list's rows as packet fields
// (serverpackets/warehouse.go writeWarehouseItem).
func whRows(items ...WarehouseItem) []any {
	var f []any
	for _, it := range items {
		f = append(f, it.Category, it.ObjectID, it.TemplateID, it.Count, it.SubCategory, it.CustomType1,
			it.BodySlot, it.Enchant, it.CustomType2, uint16(0), it.ObjectID, int32(it.Augmentation), int32(it.Augmentation>>32))
	}
	return f
}

func whList(op byte, typ uint16, adena int32, count uint16, items ...WarehouseItem) []byte {
	return packet(op, append([]any{typ, adena, count}, whRows(items...)...)...)
}

var (
	whSkins = WarehouseItem{Category: 5, ObjectID: 102, TemplateID: animalSkin, Count: 10, SubCategory: 5}
	whSword = WarehouseItem{ObjectID: 103, TemplateID: 2369, Count: 1, BodySlot: 0x4000, Enchant: 3, Augmentation: 0x0001_0002_0003_0004}
)

func TestDecodeWarehouseLists(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Clan type, 300 adena, one row: 10 Animal Skins, object 102.
		{
			"WarehouseDepositList bytes",
			[]byte{
				0x41, 2, 0, 0x2c, 1, 0, 0, 1, 0,
				5, 0, 102, 0, 0, 0, 0x4b, 7, 0, 0, 10, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 102, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			WarehouseDepositList{Type: 2, Adena: 300, Items: []WarehouseItem{whSkins}},
		},
		{"WarehouseDepositList", whList(OpGameWarehouseDepositList, 2, 5000, 2, whSkins, whSword), WarehouseDepositList{Type: 2, Adena: 5000, Items: []WarehouseItem{whSkins, whSword}}},
		{"WarehouseWithdrawList", whList(OpGameWarehouseWithdrawList, 2, 70, 1, whSword), WarehouseWithdrawList{Type: 2, Adena: 70, Items: []WarehouseItem{whSword}}},
		{"WarehouseWithdrawList private", whList(OpGameWarehouseWithdrawList, 1, 0, 0), WarehouseWithdrawList{Type: 1, Items: []WarehouseItem{}}},
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

func TestDecodeWarehouseListsRejects(t *testing.T) {
	dep := whList(OpGameWarehouseDepositList, 2, 1, 1, whSkins)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short header", packet(OpGameWarehouseDepositList, uint16(2), int32(1)), wire.ErrShortPacket},
		{"short row", dep[:len(dep)-1], wire.ErrShortPacket},
		{"count past the frame", whList(OpGameWarehouseWithdrawList, 2, 1, 2, whSkins), wire.ErrShortPacket},
		{"rows past the count", whList(OpGameWarehouseWithdrawList, 2, 1, 1, whSkins, whSword), ErrTrailingBytes},
		{"trailing", append(dep, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestRecordedWarehouseFixtures checks the list frames TestLiveClanWarehouse
// recorded against the live server: the clan deposit list holds the 10
// skins the scenario gave the bot, and the withdrawal list the clan's
// warehouse holding exactly them. Object ids vary between runs and are not
// pinned; TestRecordedFixtures checks each decodes exactly.
func TestRecordedWarehouseFixtures(t *testing.T) {
	skins := func(items []WarehouseItem) bool {
		return slices.ContainsFunc(items, func(it WarehouseItem) bool {
			return it.ObjectID != 0 && it.TemplateID == animalSkin && it.Count == 10
		})
	}
	tests := []struct {
		op   byte
		sane func(Event) bool
	}{
		{OpGameWarehouseDepositList, func(ev Event) bool {
			e, ok := ev.(WarehouseDepositList)
			return ok && e.Type == clanWarehouseType && e.Adena > 0 && skins(e.Items)
		}},
		{OpGameWarehouseWithdrawList, func(ev Event) bool {
			e, ok := ev.(WarehouseWithdrawList)
			return ok && e.Type == clanWarehouseType && len(e.Items) == 1 && skins(e.Items)
		}},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%02x.bin", tt.op)
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "recorded", name))
			if err != nil {
				t.Fatal(err)
			}
			ev, ok, err := DecodeEvent(raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", ev, ok, err)
			}
			if !tt.sane(ev) {
				t.Errorf("decoded %+v, want the clan's list with the skins", ev)
			}
		})
	}
}
