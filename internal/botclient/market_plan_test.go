package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// marketPlanBrain is benchModelBrain carrying 1,000,000 adena, with
// merchant gearMerchant spawned at (5000, 0) selling every armor of the
// model tests at its base price, so a gear plan has a shop pool to search.
func marketPlanBrain(tb testing.TB) *Brain {
	tb.Helper()
	b := benchModelBrain(tb)
	var products []datapack.Product
	for _, id := range slices.Sorted(maps.Keys(modelItems)) {
		products = append(products, datapack.Product{ItemID: id, Price: modelItems[id].Price})
	}
	m := b.cfg.Travel
	m.shops, m.cheapest = indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: products}})
	m.gear, m.shopGear = indexGear(modelItems, m.cheapest)
	m.sets, m.setPieces = indexSets(modelSets, m.gear)
	m.spawns = map[int32][]Point{gearMerchant: {{X: 5000}}}
	b.world.Apply(ItemList{Items: []Item{wornItem(301, robeTop), wornItem(302, robeBottom), adenaStack(1000000)}}, t0)
	return b
}

// TestMarketStartPlansLast checks that startMarket computes the gear plan
// (marketWants) only once it found a sell store to visit (#493): with none
// in sight, only a buy store, or the only seller cooling down, it plans
// nothing; with a seller it plans once and starts the visit.
func TestMarketStartPlansLast(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(b *Brain)
		want      bool
		wantPlans int
	}{
		{name: "no store in sight"},
		{name: "buy store only", setup: func(b *Brain) {
			info := sellStore(sellerObj, 600)
			info.StoreType = 3
			b.world.Apply(info, t0)
		}},
		{name: "seller cooling down", setup: func(b *Brain) {
			b.world.Apply(sellStore(sellerObj, 600), t0)
			b.market.cooled = map[int32]time.Time{sellerObj: at(time.Minute)}
		}},
		{name: "seller in sight", setup: func(b *Brain) { b.world.Apply(sellStore(sellerObj, 600), t0) }, want: true, wantPlans: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := marketPlanBrain(t)
			if tc.setup != nil {
				tc.setup(b)
			}
			if got := b.startMarket(at(time.Second)); got != tc.want {
				t.Fatalf("startMarket = %v, want %v", got, tc.want)
			}
			if b.gear.plans != tc.wantPlans {
				t.Fatalf("gear plans = %d, want %d", b.gear.plans, tc.wantPlans)
			}
		})
	}
}

// BenchmarkStartMarket runs startMarket as ruleMarket does on every
// out-of-combat decision with no visit under way, with no sell store in
// sight (the usual case, #493) and with one in sight whose owner cools
// down (#493 still plans nothing).
func BenchmarkStartMarket(bm *testing.B) {
	for _, bc := range []struct {
		name  string
		setup func(b *Brain)
	}{
		{"no-store", nil},
		{"seller-cooling", func(b *Brain) {
			b.world.Apply(sellStore(sellerObj, 600), t0)
			b.market.cooled = map[int32]time.Time{sellerObj: at(time.Hour)}
		}},
	} {
		bm.Run(bc.name, func(bm *testing.B) {
			b := marketPlanBrain(bm)
			if bc.setup != nil {
				bc.setup(b)
			}
			bm.ReportAllocs()
			for bm.Loop() {
				if b.startMarket(at(time.Second)) {
					bm.Fatal("visit started")
				}
			}
		})
	}
}
