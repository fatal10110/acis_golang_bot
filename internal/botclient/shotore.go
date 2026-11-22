package botclient

import (
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// shotRecipe is the recipe that makes shot: the lowest id of the ones whose
// product it is, false for none.
func shotRecipe(recipes map[int32]datapack.Recipe, shot int32) (datapack.Recipe, bool) {
	var out datapack.Recipe
	found := false
	for _, r := range recipes {
		if r.Product.ItemID == shot && r.Product.Count > 0 && (!found || r.ID < out.ID) {
			out, found = r, true
		}
	}
	return out, found
}

// isCrystal reports whether item is a crystal (crystalItem), which only
// crystallizing makes: a shot recipe's every other material is an ore a
// grocer sells (docs/crafting.md section 6).
func isCrystal(item int32) bool { return item >= crystalItem(1) && item <= crystalItem(5) }

// shotOre is the ore n shots of r need: the crafts that make n at r's yield,
// rounded up, times each material but the crystals, by item.
func shotOre(r datapack.Recipe, n int64) map[int32]int64 {
	if n <= 0 || r.Product.Count <= 0 {
		return nil
	}
	crafts := (n + int64(r.Product.Count) - 1) / int64(r.Product.Count)
	out := make(map[int32]int64)
	for _, g := range r.Materials {
		if !isCrystal(g.ItemID) {
			out[g.ItemID] += crafts * int64(g.Count)
		}
	}
	return out
}

// oreShots is how many shots of r the ore of carried covers: the whole
// crafts every ore material is there for, times r's yield.
func oreShots(r datapack.Recipe, carried map[int32]int64) int64 {
	if r.Product.Count <= 0 {
		return 0
	}
	ore := shotOre(r, int64(r.Product.Count))
	if len(ore) == 0 {
		return 0
	}
	crafts := int64(-1)
	for item, per := range ore {
		if n := carried[item] / per; crafts < 0 || n < crafts {
			crafts = n
		}
	}
	return crafts * int64(r.Product.Count)
}

// shotOreSupply is a clan member's extra supply hook (SupplyPolicy.Extra,
// docs/crafting.md section 6): orders are the shots it orders for the trip
// (template to count), and the hook answers the ore their recipes need,
// summed by ore. A shot no recipe makes asks for none. The supplies tier
// buys it after the bot's own shots, potions and Scrolls of Escape, and it
// counts toward neither the preparation gate nor the run-out trigger.
func shotOreSupply(recipes map[int32]datapack.Recipe, orders func(w *World) map[int32]int64) func(w *World) map[int32]int32 {
	return func(w *World) map[int32]int32 {
		want := orders(w)
		out := make(map[int32]int32)
		for _, shot := range slices.Sorted(maps.Keys(want)) {
			r, ok := shotRecipe(recipes, shot)
			if !ok {
				continue
			}
			for item, n := range shotOre(r, want[shot]) {
				out[item] = int32(min(int64(out[item])+n, int64(1<<31-1)))
			}
		}
		return out
	}
}

// shotOrder is how many of want shots the bot orders from the clan crafter:
// as many as the ore it carries covers (oreShots), so an order short of ore
// shrinks and never waits for it; 0 for a shot no recipe makes.
func (b *Brain) shotOrder(shot int32, want int64) int64 {
	if b.cfg.Travel == nil || want <= 0 {
		return 0
	}
	r, ok := shotRecipe(b.cfg.Travel.recipes, shot)
	if !ok {
		return 0
	}
	carried := make(map[int32]int64)
	for _, it := range b.world.Inventory.Items {
		carried[it.TemplateID] += int64(it.Count)
	}
	return min(want, oreShots(r, carried))
}
