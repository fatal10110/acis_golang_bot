package botclient

import (
	"reflect"
	"testing"
	"time"
)

// cpStockBrain is newMarketBrain as a pvp bot stocking 10 CP Potions
// (PvPPolicy.CPStock) and wanting nothing else, carrying items, with the
// reference prices 240 and 600 of the CP potions on its travel map.
func cpStockBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newMarketBrain(t, items...)
	b.persona.Supply.Extra = nil
	b.persona.PvP.CPStock, b.persona.PvP.KeepCPPotions = 10, true
	b.cfg.Travel.prices[itemCPPotion], b.cfg.Travel.prices[itemGreaterCPPotion] = 240, 600
	return b
}

// cpOf is a stack of count of CP potion template id.
func cpOf(id, count int32) Item {
	return Item{ObjectID: 960 + id%10, TemplateID: id, Count: count}
}

// TestCPShort checks the CP stock the supply list asks for: a Greater CP
// Potion counts as 4 CP Potions, nothing while Supply.Refill (25) percent
// of PvPPolicy.CPStock is carried, else the difference, clamped by the
// weight left under BuyLoad.
func TestCPShort(t *testing.T) {
	tests := []struct {
		name  string
		stock int32
		items []Item
		load  int32 // carried weight, of a 1000 limit; 0 leaves the weight unknown
		want  int32
	}{
		{name: "CPStock 0 asks for none", stock: 0},
		{name: "none carried: all ten", stock: 10, want: 10},
		{name: "2 carried, under 25 percent: the 8 missing", stock: 10, items: []Item{cpOf(itemCPPotion, 2)}, want: 8},
		{name: "3 carried, at 30 percent: none", stock: 10, items: []Item{cpOf(itemCPPotion, 3)}},
		{name: "a Greater one counts as 4: none", stock: 10, items: []Item{cpOf(itemGreaterCPPotion, 1)}},
		{name: "full in Greater and plain: none", stock: 10, items: []Item{cpOf(itemGreaterCPPotion, 2), cpOf(itemCPPotion, 2)}},
		{name: "over the stock: none", stock: 10, items: []Item{cpOf(itemGreaterCPPotion, 5)}},
		// 45 % of 1000 less 300 carried leaves 150: six at 25.
		{name: "clamped by the weight left", stock: 10, load: 300, want: 6},
		{name: "no weight left: none", stock: 10, load: 450},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := cpStockBrain(t, append([]Item{adenaStack(10000)}, tc.items...)...)
			b.persona.PvP.CPStock = tc.stock
			if tc.load > 0 {
				b.world.Self.MaxLoad, b.world.Self.Load = 1000, tc.load
				b.cfg.Travel.weights = map[int32]int32{itemCPPotion: 25, itemGreaterCPPotion: 100}
			}
			if got := b.cpShort(); got != tc.want {
				t.Fatalf("cpShort = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestMarketCPRows checks the CP potions a store window buys for the CP
// stock: rows at no more than MaxOverReference (150) percent of their
// reference price, cheapest per CP first and Greater first at an equal
// price per CP, up to the CP Potions missing and never past them, within
// the budget.
func TestMarketCPRows(t *testing.T) {
	cp := func(obj, count, price int32) StoreItem {
		return StoreItem{ObjectID: obj, TemplateID: itemCPPotion, Count: count, Price: price, ReferencePrice: 240}
	}
	gcp := func(obj, count, price int32) StoreItem {
		return StoreItem{ObjectID: obj, TemplateID: itemGreaterCPPotion, Count: count, Price: price, ReferencePrice: 600}
	}
	tests := []struct {
		name  string
		held  []Item
		adena int32
		stock int32 // CPStock; 0 is the pvp default 10
		list  []StoreItem
		want  []storeRow
	}{
		{
			name: "CP Potions below the cap", adena: 10000,
			list: []StoreItem{cp(1, 20, 200)},
			want: []storeRow{{Object: 1, Count: 10, Price: 200}},
		},
		{
			name: "CP Potions at the cap, 1.5 x 240", adena: 10000,
			list: []StoreItem{cp(1, 20, 361), cp(2, 20, 360)},
			want: []storeRow{{Object: 2, Count: 10, Price: 360}},
		},
		{
			name: "CP Potions above the cap", adena: 10000,
			list: []StoreItem{cp(1, 20, 361)},
		},
		{
			name: "Greater CP Potions at and above the cap, 1.5 x 600", adena: 10000,
			list: []StoreItem{gcp(1, 5, 901), gcp(2, 5, 900)},
			want: []storeRow{{Object: 2, Count: 2, Price: 900}},
		},
		{
			name: "the travel map's reference price for a row without one", adena: 10000,
			list: []StoreItem{{ObjectID: 1, TemplateID: itemCPPotion, Count: 20, Price: 361}, {ObjectID: 2, TemplateID: itemCPPotion, Count: 20, Price: 360}},
			want: []storeRow{{Object: 2, Count: 10, Price: 360}},
		},
		{
			// 800 for 4 is 200 a CP Potion: two Greater ones, then 2 CP
			// Potions for the rest.
			name: "Greater cheaper per CP first", adena: 10000,
			list: []StoreItem{cp(1, 20, 240), gcp(2, 5, 800)},
			want: []storeRow{{Object: 2, Count: 2, Price: 800}, {Object: 1, Count: 2, Price: 240}},
		},
		{
			name: "CP Potions cheaper per CP first", adena: 10000,
			list: []StoreItem{gcp(1, 5, 800), cp(2, 20, 150)},
			want: []storeRow{{Object: 2, Count: 10, Price: 150}},
		},
		{
			name: "Greater first at an equal price per CP", adena: 10000,
			list: []StoreItem{cp(1, 20, 200), gcp(2, 5, 800)},
			want: []storeRow{{Object: 2, Count: 2, Price: 800}, {Object: 1, Count: 2, Price: 200}},
		},
		{
			name: "never past the stock: no third Greater one for 2 CP Potions", adena: 10000,
			list: []StoreItem{gcp(1, 5, 800)},
			want: []storeRow{{Object: 1, Count: 2, Price: 800}},
		},
		{
			name: "the difference in Greater ones", adena: 10000, held: []Item{cpOf(itemCPPotion, 2)},
			list: []StoreItem{gcp(1, 5, 800), cp(2, 20, 240)},
			want: []storeRow{{Object: 1, Count: 2, Price: 800}},
		},
		{
			// 700 past the 1000 reserve: no Greater one at 800, two CP
			// Potions at 240.
			name: "a Greater one past the budget, CP Potions within it", adena: 1700,
			list: []StoreItem{gcp(1, 5, 800), cp(2, 20, 240)},
			want: []storeRow{{Object: 2, Count: 2, Price: 240}},
		},
		{
			name: "stock full", adena: 10000, held: []Item{cpOf(itemGreaterCPPotion, 2), cpOf(itemCPPotion, 2)},
			list: []StoreItem{cp(1, 20, 200)},
		},
		{
			name: "no CP stock: none bought", adena: 10000, stock: -1,
			list: []StoreItem{cp(1, 20, 200), gcp(2, 5, 600)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := cpStockBrain(t, append([]Item{adenaStack(tc.adena)}, tc.held...)...)
			if tc.stock != 0 {
				b.persona.PvP.CPStock = max(tc.stock, 0)
			}
			l := PrivateStoreListSell{OwnerID: sellerObj, BuyerAdena: tc.adena, Items: tc.list}
			if got := b.marketRows(l); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("marketRows = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMarketCPStart checks that a pvp bot short of CP potions starts a
// store visit at a clean owner, and never at a karma or flagged one, whose
// second click the server answers with an attack.
func TestMarketCPStart(t *testing.T) {
	tests := []struct {
		name  string
		held  []Item
		setup func(b *Brain)
		want  bool
	}{
		{name: "short of CP potions, a clean owner", want: true},
		{name: "karma owner", setup: func(b *Brain) { b.world.Apply(RelationChanged{ObjectID: sellerObj, Karma: 50}, t0) }},
		{name: "flagged owner", setup: func(b *Brain) { b.world.Apply(RelationChanged{ObjectID: sellerObj, PvPFlag: 1}, t0) }},
		{name: "CP stock carried", held: []Item{cpOf(itemGreaterCPPotion, 3)}},
		{name: "no CP stock", setup: func(b *Brain) { b.persona.PvP.CPStock = 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := cpStockBrain(t, append([]Item{adenaStack(10000)}, tc.held...)...)
			b.world.Apply(sellStore(sellerObj, 600), t0)
			if tc.setup != nil {
				tc.setup(b)
			}
			if got := b.startMarket(at(time.Second)); got != tc.want {
				t.Fatalf("startMarket = %v, want %v", got, tc.want)
			}
		})
	}
}
