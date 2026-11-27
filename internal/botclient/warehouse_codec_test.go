package botclient

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

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
