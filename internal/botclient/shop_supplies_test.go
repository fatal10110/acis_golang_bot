package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestStartShopSuppliesFirst checks the merchant a visit goes to: one
// selling a supplies-tier want (the arrows of the bow the bot waits to
// wear) before a nearer one selling only gear, in sight or at a spawn
// within Reach; with no supplies wanted, a merchant in sight before a
// nearer spawn, the nearest first (TestLiveArrows: the bow bought, a
// jewelry merchant in sight kept the arrows' grocer from being visited).
func TestStartShopSuppliesFirst(t *testing.T) {
	const (
		grocer    int32 = 30004
		gearObj   int32 = 0x20000050
		grocerObj int32 = 0x20000051
	)
	gearAt, grocerAt := Point{X: 500}, Point{X: 3000}
	tests := []struct {
		name   string
		items  []Item
		sight  []int32 // the merchants in sight, by object
		spawns map[int32][]Point
		want   int32 // merchant visited, 0 for none
	}{
		{"both in sight: the arrows' grocer", []Item{wornSword, carriedBow}, []int32{gearObj, grocerObj}, nil, grocer},
		{"grocer at a spawn: the arrows' grocer", []Item{wornSword, carriedBow}, []int32{gearObj}, map[int32][]Point{grocer: {grocerAt}}, grocer},
		{"grocer past Reach: the gear merchant", []Item{wornSword, carriedBow}, []int32{gearObj}, map[int32][]Point{grocer: {{X: 9000}}}, gearMerchant},
		{"arrows carried: the gear merchant in sight", []Item{wornSword, carriedBow, carriedArrows}, []int32{gearObj, grocerObj}, nil, gearMerchant},
		{"arrows carried: in sight before a nearer spawn", []Item{wornSword, carriedBow, carriedArrows}, []int32{gearObj}, map[int32][]Point{gearMerchant: {{X: 100}}}, gearMerchant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newArcherBrain(t, 1, append(tt.items, adenaStack(50000))...)
			shops, cheapest := indexShops([]datapack.BuyList{
				{ID: 1, NPCID: gearMerchant, Products: gearShop},
				{ID: 2, NPCID: grocer, Products: []datapack.Product{{ItemID: woodenArrow, Price: 2}}},
			})
			gear, shopGear := indexGear(arrowTemplates, cheapest)
			b.cfg.Travel.shops, b.cfg.Travel.cheapest, b.cfg.Travel.gear, b.cfg.Travel.shopGear = shops, cheapest, gear, shopGear
			b.cfg.Travel.spawns = tt.spawns
			for _, obj := range tt.sight {
				npc, at := gearMerchant, gearAt
				if obj == grocerObj {
					npc, at = grocer, grocerAt
				}
				b.world.Apply(NPCInfo{ObjectID: obj, NPCID: npc, Pos: at}, t0)
			}
			if len(b.gearWants()) == 0 {
				t.Fatal("no gear wanted: the gear merchant has no use")
			}
			started := b.startShop(t0)
			if got := b.shop.v.npcID; started != (tt.want != 0) || got != tt.want {
				t.Errorf("startShop = %v at %d, want %d", started, got, tt.want)
			}
		})
	}
}
