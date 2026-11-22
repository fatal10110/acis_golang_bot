package botclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The shot recipes of the ore tests, as recipes.xml ships them: Soulshot D
// (recipe 20) from three Soul Ore and a Crystal D, 156 a craft, and
// Spiritshot D (317) from three Spirit Ore and a Crystal D, 100 a craft.
const (
	soulshotD   int32 = 1463
	spiritshotD int32 = 2510
	spiritOre   int32 = 3031
	crystalD    int32 = 1458
)

var shotRecipes = map[int32]datapack.Recipe{
	20: {
		ID: 20, ScrollID: 1804, Level: 2, MPCost: 40, SuccessRate: 100, Dwarven: true,
		Materials: []datapack.Ingredient{{ItemID: soulOre, Count: 3}, {ItemID: crystalD, Count: 1}},
		Product:   datapack.Ingredient{ItemID: soulshotD, Count: 156},
	},
	317: {
		ID: 317, ScrollID: 3032, Level: 2, MPCost: 60, SuccessRate: 100, Dwarven: true,
		Materials: []datapack.Ingredient{{ItemID: spiritOre, Count: 3}, {ItemID: crystalD, Count: 1}},
		Product:   datapack.Ingredient{ItemID: spiritshotD, Count: 100},
	},
}

// TestShotOre checks the ore of a shot order: whole crafts rounded up,
// every material but the crystal; and the shots carried ore covers.
func TestShotOre(t *testing.T) {
	tests := []struct {
		name      string
		shot      int32
		n         int64
		want      map[int32]int64
		carried   map[int32]int64
		wantShots int64
	}{
		{"one craft", soulshotD, 156, map[int32]int64{soulOre: 3}, map[int32]int64{soulOre: 3, crystalD: 0}, 156},
		{"one more shot, one more craft", soulshotD, 157, map[int32]int64{soulOre: 6}, map[int32]int64{soulOre: 5}, 156},
		{"a trip's worth", soulshotD, 1000, map[int32]int64{soulOre: 21}, map[int32]int64{soulOre: 21}, 1092},
		{"spiritshots", spiritshotD, 250, map[int32]int64{spiritOre: 9}, map[int32]int64{soulOre: 9}, 0},
		{"none ordered", soulshotD, 0, nil, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := shotRecipe(shotRecipes, tt.shot)
			if !ok {
				t.Fatalf("no recipe makes %d", tt.shot)
			}
			if got := shotOre(r, tt.n); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("shotOre = %v, want %v", got, tt.want)
			}
			if got := oreShots(r, tt.carried); got != tt.wantShots {
				t.Errorf("oreShots = %d, want %d", got, tt.wantShots)
			}
		})
	}
	if _, ok := shotRecipe(shotRecipes, soulshotNoGrade); ok {
		t.Error("a recipe for a shot none makes")
	}
}

// oreOrders is a member ordering a trip's thousand Soulshots D and a
// Spiritshot no recipe makes.
func oreOrders(*World) map[int32]int64 { return map[int32]int64{soulshotD: 1000, blessedNoGrade: 500} }

// TestShotOreSupply checks that a member's shot order puts the ore it needs
// in the extra supply hook, bought last in the supplies tier.
func TestShotOreSupply(t *testing.T) {
	b := newMeterBrain(t, classHumanFight, wielded(1, ngSword))
	b.cfg.Travel.recipes = shotRecipes
	b.persona.Supply.Extra = shotOreSupply(shotRecipes, oreOrders)
	if got, want := b.persona.Supply.Extra(b.world), map[int32]int32{soulOre: 21}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Extra = %v, want %v", got, want)
	}
	s := b.supplies()
	if last := s[len(s)-1]; last != (supply{item: soulOre, count: 21, extra: true}) {
		t.Fatalf("supplies = %+v, want the ore last, extra", s)
	}
}

// TestShotOrderShortOfOre checks that a member short of ore orders only
// the shots its ore covers, and that the ore it lacks neither holds it in
// town nor sends it home.
func TestShotOrderShortOfOre(t *testing.T) {
	tests := []struct {
		name string
		ore  int32
		want int64
	}{
		{"enough ore", 30, 1000},
		{"short: whole crafts only", 10, 468},
		{"under a craft", 2, 0},
		{"none", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []Item{wielded(1, ngSword), shotStack(2, soulshotNoGrade, 700)}
			if tt.ore > 0 {
				items = append(items, lootStack(3, soulOre, tt.ore))
			}
			b := newMeterBrain(t, classHumanFight, items...)
			b.cfg.Travel.recipes = shotRecipes
			b.persona.Supply.ReturnMinutes = 10
			b.persona.Supply.Extra = shotOreSupply(shotRecipes, oreOrders)
			if got := b.shotOrder(soulshotD, 1000); got != tt.want {
				t.Errorf("shotOrder = %d, want %d", got, tt.want)
			}
			if got := b.supplyCause(); got != causeNone {
				t.Errorf("supplyCause = %d, want none: ore sends no one home", got)
			}
		})
	}
	if got := newMeterBrain(t, classHumanFight).shotOrder(soulshotD, 1000); got != 0 {
		t.Errorf("shotOrder with no recipes = %d, want 0", got)
	}
}

// TestShotOreNoTownHold checks that a member whose own supplies are met
// leaves town while short of the ore its order asks for, though a merchant
// in reach sells it at a price it pays.
func TestShotOreNoTownHold(t *testing.T) {
	b := newPrepBrain(t, Point{X: 5000}, adenaStack(10000), potionStack(50))
	tm := b.cfg.Travel
	tm.shops, tm.cheapest = indexShops([]datapack.BuyList{
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: lesserHealingPotion, Price: 103}, {ItemID: soulOre, Price: 50}}},
	})
	tm.spawnedSold = spawnedSold(tm.shops, tm.spawns)
	tm.recipes = shotRecipes
	b.persona.Supply.Extra = shotOreSupply(shotRecipes, oreOrders)
	now := at(time.Minute)
	b.shop.nextAt = now.Add(time.Hour)
	b.armPrep()
	if b.prepShort() {
		t.Fatal("prepShort: ore holds the member in town")
	}
	b.ruleSpot(now)
	if !b.spot.held {
		t.Fatal("no spot held: the ore order kept the member in town")
	}
}
