package craftplan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestBuildWorkBound checks Config.MaxWork stops a plan on a deep, wide
// recipe graph: every item of a level made by two 100 % recipes from three
// items of the level below, five levels over a stocked base.
func TestBuildWorkBound(t *testing.T) {
	recs := map[int32]datapack.Recipe{}
	prices := map[int32]int32{}
	wh := map[int32]int64{}
	item := func(level, i int32) int32 { return 10000 + level*100 + i }
	id := int32(1)
	var book []int32
	for i := range int32(4) {
		wh[item(0, i)] = 1 << 20
		prices[item(0, i)] = 1
	}
	for level := int32(1); level <= 5; level++ {
		for i := range int32(4) {
			prices[item(level, i)] = 10 << (3 * level)
			for v := range int32(2) {
				r := rec(id, item(level, i), 1)
				for k := range int32(3) {
					r.Materials = append(r.Materials, datapack.Ingredient{ItemID: item(level-1, (i+v+k)%4), Count: 2})
				}
				recs[id] = r
				book = append(book, id)
				id++
			}
		}
	}
	in := Input{Warehouse: wh, Recipes: recs, Prices: prices, Book: book, DwarvenLevel: 1, DwarvenLimit: 100}
	for _, maxWork := range []int{1, 50, 5000} {
		got := Build(in, Config{MaxChains: 20, MaxWork: maxWork})
		checkPlan(t, in, got)
		if got.Work > maxWork || !got.Exhausted {
			t.Errorf("MaxWork %d: work %d, exhausted %v", maxWork, got.Work, got.Exhausted)
		}
	}
	if got := Build(in, Config{MaxChains: 20, MaxWork: 1 << 30}); got.Exhausted || len(got.Chains) != 20 {
		t.Errorf("unbounded: %d chains, exhausted %v", len(got.Chains), got.Exhausted)
	}
}

// shippedInput is a plan on the shipped recipes.xml and reference prices
// when BOT_DATAPACK_XML names the server checkout's data/xml (skipped
// otherwise): every recipe in the book at Create Item 9, ten thousand of
// every material and recipe scroll in the warehouse, and a member's order
// of Soulshot D.
func shippedInput(tb testing.TB) Input {
	tb.Helper()
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		tb.Skip("BOT_DATAPACK_XML not set")
	}
	recs, err := datapack.LoadRecipes(filepath.Join(dir, "recipes.xml"))
	if err != nil {
		tb.Fatal(err)
	}
	_, _, prices, err := datapack.LoadItems(filepath.Join(dir, "items"))
	if err != nil {
		tb.Fatal(err)
	}
	in := Input{
		Warehouse: map[int32]int64{}, Recipes: recs, Prices: prices,
		DwarvenLevel: 9, CommonLevel: 9, DwarvenLimit: 1000, CommonLimit: 1000,
		Orders: []Order{{Product: 1463, Count: 1000}},
	}
	for _, r := range recs {
		in.Book = append(in.Book, r.ID)
		for _, m := range r.Materials {
			in.Warehouse[m.ItemID] = 10000
		}
	}
	return in
}

// TestBuildShipped checks the planner's bound on the full recipes.xml: the
// plan stays within DefaultConfig's work, serves the order and plans
// chains. It also checks the planner's assumption that every intermediate
// (a product another recipe uses) is a 100 % recipe.
func TestBuildShipped(t *testing.T) {
	in := shippedInput(t)
	materials := map[int32]bool{}
	for _, r := range in.Recipes {
		for _, m := range r.Materials {
			materials[m.ItemID] = true
		}
	}
	for _, r := range in.Recipes {
		if materials[r.Product.ItemID] && r.SuccessRate < 100 {
			t.Errorf("recipe %d makes intermediate %d at %d %%", r.ID, r.Product.ItemID, r.SuccessRate)
		}
	}
	cfg := DefaultConfig()
	got := Build(in, cfg)
	checkPlan(t, in, got)
	t.Logf("work %d, exhausted %v, chains %d", got.Work, got.Exhausted, len(got.Chains))
	if got.Work > cfg.MaxWork {
		t.Fatalf("work %d over MaxWork %d", got.Work, cfg.MaxWork)
	}
	if len(got.Chains) < 2 || got.Chains[0].Order != 0 || got.Chains[0].Count < 1000 {
		t.Fatalf("chains %+v, want the order then the clan's", got.Chains)
	}
}

// BenchmarkBuildShipped measures one plan on the full recipes.xml.
func BenchmarkBuildShipped(b *testing.B) {
	in := shippedInput(b)
	cfg := DefaultConfig()
	b.ReportAllocs()
	for b.Loop() {
		Build(in, cfg)
	}
}
