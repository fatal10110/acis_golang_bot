package botclient

import (
	"reflect"
	"slices"
	"testing"
)

// TestMarketGearRows checks the gear tier of a store window (marketRows):
// a goal piece is bought at most at a spawned merchant's base price, one
// piece, above GearPolicy.KeepAdena, and a piece of the planned set only
// with the rest of the set still paid at its cheapest base price.
func TestMarketGearRows(t *testing.T) {
	const keep = 10000 // GearPolicy.KeepAdena, the gear tier's floor
	old := []Item{wornAt(1, oldShirt, partChest), wornAt(2, oldPants, partLegs)}
	row := func(obj, id, count, price int32) StoreItem {
		return StoreItem{ObjectID: obj, TemplateID: id, Count: count, Price: price, ReferencePrice: price}
	}
	// brain is a set brain carrying adena whose merchant is spawned, so its
	// base prices cap the store's.
	brain := func(t *testing.T, adena int32) *Brain {
		b := newSetBrain(t, setShop, append(slices.Clone(old), adenaStack(adena))...)
		m := b.cfg.Travel
		m.spawnedSold = spawnedSold(m.shops, m.spawns)
		return b
	}

	t.Run("single piece", func(t *testing.T) {
		const adena = keep + 5000
		probe := brain(t, adena)
		probe.persona.Gear.SetBonus = 0
		goal, set := probe.gearGoal()
		if len(goal) != 1 || len(set) != 0 {
			t.Fatalf("gearGoal = %v, set %v, want one piece outside a set", goal, set)
		}
		piece := goal[0]
		price := probe.cfg.Travel.cheapest[piece]
		tests := []struct {
			name  string
			buyer int32 // the window's buyer adena
			items []StoreItem
			want  []storeRow
		}{
			{
				name: "one piece at the base price", buyer: adena,
				items: []StoreItem{row(1, piece, 3, price)},
				want:  []storeRow{{1, 1, price}},
			},
			{
				name: "the cheaper row of the piece", buyer: adena,
				items: []StoreItem{row(1, piece, 1, price), row(2, piece, 1, price-1)},
				want:  []storeRow{{2, 1, price - 1}},
			},
			{name: "never above the base price", buyer: adena, items: []StoreItem{row(1, piece, 1, price+1)}},
			{name: "under KeepAdena", buyer: keep + price - 1, items: []StoreItem{row(1, piece, 1, price)}},
			{
				name: "just above KeepAdena", buyer: keep + price,
				items: []StoreItem{row(1, piece, 1, price)},
				want:  []storeRow{{1, 1, price}},
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				b := brain(t, adena)
				b.persona.Gear.SetBonus = 0
				l := PrivateStoreListSell{OwnerID: sellerObj, BuyerAdena: tc.buyer, Items: tc.items}
				if got := b.marketRows(l); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("marketRows = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("planned set", func(t *testing.T) {
		// Set S at the merchant: shield 600, chest 1000, legs 800, head 500.
		const adena = keep + 2900
		set := []int32{setShield, setChest, setLegs, setHead}
		if goal, s := brain(t, adena).gearGoal(); !slices.Equal(goal, set) || !slices.Equal(s, set) {
			t.Fatalf("gearGoal = %v, set %v, want the set %v", goal, s, set)
		}
		legs := []StoreItem{row(1, setLegs, 1, 800)}
		tests := []struct {
			name  string
			buyer int32
			items []StoreItem
			want  []storeRow
		}{
			{
				// 2900 past KeepAdena: the legs, and 2100 left for the rest.
				name: "the piece with the rest paid", buyer: adena, items: legs,
				want: []storeRow{{1, 1, 800}},
			},
			{name: "the rest not paid", buyer: adena - 1, items: legs},
			{
				// Each piece bought lowers the rest still to pay.
				name: "the whole set", buyer: adena,
				items: []StoreItem{row(1, setLegs, 1, 800), row(2, setHead, 1, 500), row(3, setChest, 1, 1000), row(4, setShield, 1, 600)},
				want:  []storeRow{{4, 1, 600}, {3, 1, 1000}, {1, 1, 800}, {2, 1, 500}},
			},
			{name: "a set piece above the base price", buyer: adena + 1000, items: []StoreItem{row(1, setLegs, 1, 801)}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				b := brain(t, adena)
				l := PrivateStoreListSell{OwnerID: sellerObj, BuyerAdena: tc.buyer, Items: tc.items}
				if got := b.marketRows(l); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("marketRows = %v, want %v", got, tc.want)
				}
			})
		}
	})
}
