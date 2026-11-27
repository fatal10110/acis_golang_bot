package botclient

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

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
