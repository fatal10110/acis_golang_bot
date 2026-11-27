package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestShopBowArrowsFirst checks the visit of an archer carrying a bow it
// keeps off for want of arrows (bowWaits): the arrows of the bow it wears
// next are a supplies want (arrowWants), so the visit goes to the grocer
// selling them, in sight or with its spawn within Reach, past a nearer
// armorer selling only the gear it wants (#370); with its arrows carried,
// or the grocer out of reach, the armorer's gear is not held back.
func TestShopBowArrowsFirst(t *testing.T) {
	const (
		armorer, armorerObj = int32(30002), int32(901)
		grocer, grocerObj   = int32(30004), int32(902)
	)
	tests := []struct {
		name   string
		items  []Item
		sight  bool // both merchants in sight, else only the armorer
		reach  int32
		want   int32
		lists  []int32
		arrows bool // the arrows among the visit's wants
	}{
		{name: "grocer within reach", items: []Item{carriedBow, adenaStack(30000)}, want: grocer, lists: []int32{14}, arrows: true},
		{name: "grocer in sight", items: []Item{carriedBow, adenaStack(30000)}, sight: true, want: grocer, lists: []int32{14}, arrows: true},
		{name: "arrows carried", items: []Item{carriedBow, carriedArrows, adenaStack(30000)}, sight: true, want: armorer, lists: []int32{2}},
		{name: "grocer beyond reach", items: []Item{carriedBow, adenaStack(30000)}, reach: 6000, want: armorer, lists: []int32{2}, arrows: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newArcherBrain(t, 0, tt.items...)
			if !b.bowWaits(carriedBow) && !slices.Contains(tt.items, carriedArrows) {
				t.Fatal("the carried bow does not wait for its arrows")
			}
			shops, cheapest := indexShops([]datapack.BuyList{
				{ID: 2, NPCID: armorer, Products: []datapack.Product{{ItemID: leatherShirt22, Price: 2430}, {ItemID: woodenGaiters, Price: 4970}}},
				{ID: 14, NPCID: grocer, Products: []datapack.Product{{ItemID: woodenArrow, Price: 2}}},
			})
			gear, shopGear := indexGear(arrowTemplates, cheapest)
			b.cfg.Travel = &TravelMap{
				spawns: map[int32][]Point{armorer: {{X: 300}}, grocer: {{X: 7000}}},
				shops:  shops, cheapest: cheapest, gear: gear, shopGear: shopGear,
				arrows: indexArrows(arrowTemplates),
			}
			if tt.reach > 0 {
				b.persona.Shop.Reach = tt.reach
			}
			b.world.Apply(NPCInfo{ObjectID: armorerObj, NPCID: armorer, Pos: Point{X: 300}}, t0)
			if tt.sight {
				b.world.Apply(NPCInfo{ObjectID: grocerObj, NPCID: grocer, Pos: Point{X: 2000}}, t0)
			}
			wants := b.shopWants(false)
			if got := slices.ContainsFunc(wants, func(r buyRow) bool { return r.Item == woodenArrow }); got != tt.arrows {
				t.Fatalf("shopWants = %v: arrows wanted %v, want %v", wants, got, tt.arrows)
			}
			if !b.startShop(at(time.Second)) || b.shop.v.npcID != tt.want || !slices.Equal(b.shop.lists, tt.lists) {
				t.Fatalf("visit %+v lists %v, want merchant %d lists %v", b.shop.v, b.shop.lists, tt.want, tt.lists)
			}
		})
	}
}
