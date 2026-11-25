package botclient

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
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
		{"WarehouseDepositList", whList(opGameWarehouseDepositList, 2, 5000, 2, whSkins, whSword), WarehouseDepositList{Type: 2, Adena: 5000, Items: []WarehouseItem{whSkins, whSword}}},
		{"WarehouseWithdrawList", whList(opGameWarehouseWithdrawList, 2, 70, 1, whSword), WarehouseWithdrawList{Type: 2, Adena: 70, Items: []WarehouseItem{whSword}}},
		{"WarehouseWithdrawList private", whList(opGameWarehouseWithdrawList, 1, 0, 0), WarehouseWithdrawList{Type: 1, Items: []WarehouseItem{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeWarehouseListsRejects(t *testing.T) {
	dep := whList(opGameWarehouseDepositList, 2, 1, 1, whSkins)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short header", packet(opGameWarehouseDepositList, uint16(2), int32(1)), wire.ErrShortPacket},
		{"short row", dep[:len(dep)-1], wire.ErrShortPacket},
		{"count past the frame", whList(opGameWarehouseWithdrawList, 2, 1, 2, whSkins), wire.ErrShortPacket},
		{"rows past the count", whList(opGameWarehouseWithdrawList, 2, 1, 1, whSkins, whSword), errTrailingBytes},
		{"trailing", append(dep, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestWarehouseEncoders pins the request bytes: exactly 4+8*rows after the
// opcode, or the server discards the request in silence.
func TestWarehouseEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{
			"SendWareHouseDepositList", encodeWarehouseRequest(opGameSendWareHouseDepositList, []warehouseRow{{102, animalSkin, 10}, {100, adenaID, 1000}}),
			[]byte{0x31, 2, 0, 0, 0, 102, 0, 0, 0, 10, 0, 0, 0, 100, 0, 0, 0, 0xe8, 3, 0, 0},
		},
		{
			"SendWareHouseWithDrawList", encodeWarehouseRequest(opGameSendWareHouseWithdrawList, []warehouseRow{{0x10000203, 2369, 1}}),
			[]byte{0x32, 1, 0, 0, 0, 3, 2, 0, 0x10, 1, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
		if rows := int(tt.got[1]); len(tt.got) != 1+4+8*rows {
			t.Errorf("%s: %d bytes for %d rows", tt.name, len(tt.got), rows)
		}
	}
}

// TestTravelMapKeepers keeps the warehouse keeper templates with a fixed
// spawn, and no other type.
func TestTravelMapKeepers(t *testing.T) {
	pos := []datapack.Position{{X: 1}}
	dp := &datapack.Datapack{
		NPCs: map[int32]datapack.NPC{
			30005: {ID: 30005, Type: npcTypeWarehouseKeeper},
			30054: {ID: 30054, Type: npcTypeWarehouseKeeper},
			35000: {ID: 35000, Type: "CastleWarehouseKeeper"},
			30004: {ID: 30004, Type: "Merchant"},
		},
		Spawnlist: &datapack.Spawnlist{Makers: []*datapack.Maker{{Name: "town", Spawns: []datapack.Spawn{
			{NPCID: 30005, Positions: pos},
			{NPCID: 35000, Positions: pos},
			{NPCID: 30004, Positions: pos},
			{NPCID: 30054}, // placed at random: no fixed spawn
		}}}},
	}
	if got, want := NewTravelMap(dp, DefaultSupplyConfig()).keepers, map[int32]bool{30005: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("keepers = %v, want %v", got, want)
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
		{opGameWarehouseDepositList, func(ev Event) bool {
			e, ok := ev.(WarehouseDepositList)
			return ok && e.Type == clanWarehouseType && e.Adena > 0 && skins(e.Items)
		}},
		{opGameWarehouseWithdrawList, func(ev Event) bool {
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
			ev, ok, err := decodeEvent(raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", ev, ok, err)
			}
			if !tt.sane(ev) {
				t.Errorf("decoded %+v, want the clan's list with the skins", ev)
			}
		})
	}
}
