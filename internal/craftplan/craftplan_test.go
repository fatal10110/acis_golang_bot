package craftplan

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Test items: base materials, intermediates, end products and scrolls.
const (
	ore, coal, steel, part, weapon, weaponScroll = 1001, 1002, 1010, 1020, 1030, 2030
	wood, shaft, tip, arrow                      = 1041, 1040, 1051, 1050
	gem, ring                                    = 1061, 1060
	leaf, potion, potionScroll, lampScroll       = 1071, 1070, 2070, 2080
	relic, idol                                  = 1091, 1090
	shotD, shotPack                              = 1463, 1100
	blade, sabre                                 = 1110, 1111
	gilded, hilt                                 = 1120, 1025
)

// rec is a recipe at level 1, 100 %, 10 MP, dwarven, unless edited.
func rec(id int32, product, yield int32, mats ...int32) datapack.Recipe {
	r := datapack.Recipe{ID: id, ScrollID: 5000 + id, Level: 1, MPCost: 10, SuccessRate: 100, Product: datapack.Ingredient{ItemID: product, Count: yield}, Dwarven: true}
	for i := 0; i+1 < len(mats); i += 2 {
		r.Materials = append(r.Materials, datapack.Ingredient{ItemID: mats[i], Count: mats[i+1]})
	}
	return r
}

func withRate(r datapack.Recipe, rate int32) datapack.Recipe { r.SuccessRate = rate; return r }

// recipes are the test recipe set: steel to a weapon (its top step 70 %
// and consuming its own scroll) or to a hilt, arrows from shafts made ten at a time, a
// ring with a 100 % and a 60 % variant, a potion consuming its own scroll
// and a lamp learned from its scroll, an idol from an unpriced relic, the
// compressed shot pack, two products competing for steel, and one made
// from adena.
var recipes = func() map[int32]datapack.Recipe {
	w := withRate(rec(30, weapon, 1, part, 1, steel, 1, weaponScroll, 1), 70)
	w.ScrollID, w.Level = weaponScroll, 2
	potionR := rec(70, potion, 1, leaf, 1, potionScroll, 1)
	potionR.ScrollID = potionScroll
	lamp := rec(80, 1080, 1, leaf, 2)
	lamp.ScrollID = lampScroll
	out := map[int32]datapack.Recipe{}
	for _, r := range []datapack.Recipe{
		rec(10, steel, 1, ore, 5, coal, 5), rec(20, part, 1, steel, 2), rec(25, hilt, 1, part, 1), w,
		rec(40, shaft, 10, wood, 1), rec(50, arrow, 500, shaft, 4, tip, 1),
		rec(60, ring, 1, gem, 10), withRate(rec(61, ring, 1, gem, 2), 60),
		potionR, lamp,
		rec(90, idol, 1, relic, 1),
		rec(100, shotPack, 1, shotD, 1000),
		rec(110, blade, 1, steel, 2), rec(111, sabre, 1, steel, 4),
		rec(120, gilded, 1, adenaID, 100),
	} {
		out[r.ID] = r
	}
	return out
}()

var prices = map[int32]int32{
	ore: 10, coal: 10, steel: 100, part: 200, hilt: 500, weaponScroll: 100, weapon: 5000,
	wood: 10, shaft: 1, tip: 10, arrow: 1,
	gem: 50, ring: 1000,
	leaf: 100, potionScroll: 100, potion: 1000, lampScroll: 100, 1080: 50,
	idol: 1000, shotD: 7, shotPack: 20000,
	blade: 1000, sabre: 1400,
	gilded: 1000000,
}

// input is a dwarf at Create Item 2 with room for 50 recipes a page,
// holding book, with the test recipes and prices.
func input(wh map[int32]int64, book ...int32) Input {
	return Input{
		Warehouse: wh, Recipes: recipes, Prices: prices, Book: book,
		DwarvenLevel: 2, CommonLevel: 2, DwarvenLimit: 50, CommonLimit: 50, MaxMP: 300,
	}
}

func ing(pairs ...int32) []datapack.Ingredient {
	var out []datapack.Ingredient
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, datapack.Ingredient{ItemID: pairs[i], Count: pairs[i+1]})
	}
	return out
}

func steps(pairs ...int64) []Step {
	var out []Step
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Step{Recipe: int32(pairs[i]), Times: pairs[i+1]})
	}
	return out
}

// weaponStock covers one weapon: three steel and a scroll copy.
var weaponStock = map[int32]int64{ore: 15, coal: 15, weaponScroll: 1}

// TestBuild checks plans against fixed warehouses, inventories, books and
// orders (docs/crafting.md, section 4).
func TestBuild(t *testing.T) {
	weaponChain := Chain{Product: weapon, Count: 1, Order: -1, Margin: 5000 - 572, Steps: steps(10, 2, 20, 1, 10, 1, 30, 1)}
	arrowChain := Chain{Product: arrow, Count: 500, Order: -1, Margin: 486}
	tests := []struct {
		name string
		in   Input
		edit func(*Input)
		cfg  *Config
		want Plan
	}{
		{
			// Steel 100 and a part 200 are worth exactly their inputs, so
			// only the chain to the weapon gains: 300 of ore and coal and a
			// 100 scroll copy, over the 70 % top step, is 572 expected.
			name: "multi-step chain to a weapon, worth it",
			in:   input(weaponStock, 10, 20, 30),
			want: Plan{Withdraw: ing(ore, 15, coal, 15, weaponScroll, 1), Chains: []Chain{weaponChain}},
		},
		{
			name: "multi-step chain to a weapon, not worth it",
			in:   input(weaponStock, 10, 20, 30),
			edit: func(in *Input) { in.Prices = withPrice(weapon, 571) },
		},
		{
			name: "MinGain over the margin skips the chain",
			in:   input(weaponStock, 10, 20, 30),
			cfg:  &Config{MinGain: 4428, MaxChains: 20, MaxWork: 1000},
		},
		{
			name: "intermediates in stock are used, not crafted",
			in:   input(map[int32]int64{steel: 1, part: 1, weaponScroll: 1}, 10, 20, 30),
			want: Plan{
				Withdraw: ing(steel, 1, part, 1, weaponScroll, 1),
				Chains:   []Chain{{Product: weapon, Count: 1, Order: -1, Margin: 5000 - 572, Steps: steps(30, 1)}},
			},
		},
		{
			name: "top step above Create Item level is not crafted",
			in:   input(weaponStock, 10, 20, 30),
			edit: func(in *Input) { in.DwarvenLevel = 1 },
		},
		{
			name: "recipe outside the book is not crafted",
			in:   input(weaponStock, 10, 30),
		},
		{
			name: "recipe costing more than max MP is not crafted",
			in:   input(weaponStock, 10, 20, 30),
			edit: func(in *Input) { in.MaxMP = 9 },
		},
		{
			// One wood makes ten shafts; the first chain uses four, the
			// second four more of the leftover, the last two are kept.
			name: "yields above one and their leftovers",
			in:   input(map[int32]int64{wood: 1, tip: 5}, 40, 50),
			want: Plan{
				Withdraw: ing(wood, 1, tip, 2), Keep: ing(shaft, 2),
				Chains: []Chain{withSteps(arrowChain, 40, 1, 50, 1), withSteps(arrowChain, 50, 1)},
			},
		},
		{
			name: "60 % variant when its margin per value wins",
			in:   input(map[int32]int64{gem: 10}, 60, 61),
			cfg:  &Config{MaxChains: 1, MaxWork: 1000},
			want: Plan{Withdraw: ing(gem, 2), Chains: []Chain{{Product: ring, Count: 1, Order: -1, Margin: 1000 - 167, Steps: steps(61, 1)}}},
		},
		{
			name: "100 % variant first on a tie",
			in:   input(map[int32]int64{gem: 10}, 60, 61),
			edit: func(in *Input) { in.Recipes = withRecipe(withRate(rec(61, ring, 1, gem, 6), 60)) },
			cfg:  &Config{MaxChains: 1, MaxWork: 1000},
			want: Plan{Withdraw: ing(gem, 10), Chains: []Chain{{Product: ring, Count: 1, Order: -1, Margin: 500, Steps: steps(60, 1)}}},
		},
		{
			// One copy of the lamp scroll is learned; the potion consumes a
			// copy of its own scroll per craft.
			name: "scroll copies: one learned, the rest materials",
			in:   input(map[int32]int64{leaf: 5, potionScroll: 2, lampScroll: 3}, 70),
			want: Plan{
				Learn: []int32{lampScroll}, Withdraw: ing(leaf, 2, potionScroll, 2, lampScroll, 1),
				Chains: []Chain{potionChain, potionChain},
			},
		},
		{
			name: "scroll not learned with the page full",
			in:   input(map[int32]int64{lampScroll: 3}, 70),
			edit: func(in *Input) { in.DwarvenLimit = 1 },
		},
		{
			name: "scroll not learned above Create Item level",
			in:   input(map[int32]int64{lampScroll: 3}, 70),
			edit: func(in *Input) { in.Recipes = withRecipe(withLevel(recipes[80], 3)) },
		},
		{
			name: "scroll not learned without the craft",
			in:   input(map[int32]int64{lampScroll: 3}, 70),
			edit: func(in *Input) { in.DwarvenLevel = 0 },
		},
		{
			name: "carried scroll learned without a withdraw",
			in:   input(nil, 70),
			edit: func(in *Input) { in.Inventory = map[int32]int64{lampScroll: 1} },
			want: Plan{Learn: []int32{lampScroll}, Keep: ing(lampScroll, 1)},
		},
		{
			name: "unpriced items untouched",
			in:   input(map[int32]int64{relic: 3}, 90),
		},
		{
			name: "unpriced items used when an order names them",
			in:   input(map[int32]int64{relic: 3}, 90),
			edit: func(in *Input) { in.Orders = []Order{{Product: idol, Count: 2}} },
			want: Plan{Withdraw: ing(relic, 2), Chains: []Chain{{Product: idol, Count: 2, Order: 0, Margin: 2000, Steps: steps(90, 2)}}},
		},
		{
			name: "members' shots excluded from the pack recipe",
			in:   input(map[int32]int64{shotD: 1500}, 100),
			edit: func(in *Input) { in.Reserved = map[int32]int64{shotD: 1000} },
		},
		{
			name: "clan's shots packed",
			in:   input(map[int32]int64{shotD: 2500}, 100),
			edit: func(in *Input) { in.Reserved = map[int32]int64{shotD: 1000} },
			want: Plan{Withdraw: ing(shotD, 1000), Chains: []Chain{{Product: shotPack, Count: 1, Order: -1, Margin: 13000, Steps: steps(100, 1)}}},
		},
		{
			// A blade gains 800 on 200 used, a sabre 1000 on 400: the
			// blades take the steel though a sabre's margin is larger.
			name: "competing chains allocated by margin per value used",
			in:   input(map[int32]int64{steel: 4}, 110, 111),
			want: Plan{Withdraw: ing(steel, 4), Chains: []Chain{bladeChain, bladeChain}},
		},
		{
			name: "MaxChains bounds the chains",
			in:   input(map[int32]int64{steel: 4}, 110, 111),
			cfg:  &Config{MaxChains: 1, MaxWork: 1000},
			want: Plan{Withdraw: ing(steel, 2), Chains: []Chain{bladeChain}},
		},
		{
			// The order takes the steel first; what it cannot cover is cut.
			name: "orders first, cut to what the pool covers",
			in:   input(map[int32]int64{steel: 5}, 110, 111),
			edit: func(in *Input) { in.Orders = []Order{{Product: sabre, Count: 3}, {Product: 9999, Count: 1}} },
			want: Plan{Withdraw: ing(steel, 4), Chains: []Chain{{Product: sabre, Count: 1, Order: 0, Margin: 1000, Steps: steps(111, 1)}}},
		},
		{
			name: "adena never taken",
			in:   input(map[int32]int64{adenaID: 1000000}, 120),
			edit: func(in *Input) { in.Orders = []Order{{Product: gilded, Count: 1}} },
		},
		{
			// The last cycle made the part: it is in the inventory, the
			// warehouse lost most of the ore, and the plan finishes the
			// weapon from what is left.
			name: "re-planned after the warehouse changed",
			in:   input(map[int32]int64{ore: 5, coal: 5, weaponScroll: 1}, 10, 20, 30),
			edit: func(in *Input) { in.Inventory = map[int32]int64{part: 1} },
			want: Plan{
				Withdraw: ing(ore, 5, coal, 5, weaponScroll, 1), Keep: ing(part, 1),
				Chains: []Chain{{Product: weapon, Count: 1, Order: -1, Margin: 5000 - 572, Steps: steps(10, 1, 30, 1)}},
			},
		},
		{
			// The scroll copy is gone: the weapon is not covered, and the
			// part made for it is back in the pool, for a hilt.
			name: "re-planned chain no longer covered frees its intermediate",
			in:   input(map[int32]int64{ore: 5, coal: 5}, 10, 20, 25, 30),
			edit: func(in *Input) { in.Inventory = map[int32]int64{part: 1} },
			want: Plan{Keep: ing(part, 1), Chains: []Chain{{Product: hilt, Count: 1, Order: -1, Margin: 300, Steps: steps(25, 1)}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			if tt.edit != nil {
				tt.edit(&in)
			}
			cfg := Config{MaxChains: 20, MaxWork: 1000}
			if tt.cfg != nil {
				cfg = *tt.cfg
			}
			got := Build(in, cfg)
			checkPlan(t, in, got)
			got.Work = 0
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Build =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

var (
	potionChain = Chain{Product: potion, Count: 1, Order: -1, Margin: 800, Steps: steps(70, 1)}
	bladeChain  = Chain{Product: blade, Count: 1, Order: -1, Margin: 800, Steps: steps(110, 1)}
)

func withSteps(c Chain, pairs ...int64) Chain {
	c.Steps = steps(pairs...)
	return c
}

func withPrice(item int32, price int32) map[int32]int32 {
	out := map[int32]int32{}
	for k, v := range prices {
		out[k] = v
	}
	out[item] = price
	return out
}

func withRecipe(r datapack.Recipe) map[int32]datapack.Recipe {
	out := map[int32]datapack.Recipe{}
	for k, v := range recipes {
		out[k] = v
	}
	out[r.ID] = r
	return out
}

func withLevel(r datapack.Recipe, lvl int32) datapack.Recipe { r.Level = lvl; return r }

// checkPlan checks what holds for every plan: nothing withdrawn past the
// warehouse's unreserved counts, never adena, and every learned scroll's
// recipe out of the book.
func checkPlan(t *testing.T, in Input, p Plan) {
	t.Helper()
	for _, w := range p.Withdraw {
		if w.ItemID == adenaID {
			t.Errorf("withdraws adena")
		}
		if left := in.Warehouse[w.ItemID] - in.Reserved[w.ItemID]; int64(w.Count) > left {
			t.Errorf("withdraws %d of %d, warehouse has %d unreserved", w.Count, w.ItemID, left)
		}
	}
	for _, k := range p.Keep {
		if k.ItemID == adenaID {
			t.Errorf("keeps adena")
		}
	}
}
