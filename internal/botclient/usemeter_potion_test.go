package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// healingPotion is the Healing Potion, sold by town merchants but not by
// the newbie village grocers.
const healingPotion int32 = 1061

// townGrocer is a merchant selling the Healing Potion too.
const townGrocer int32 = 30084

// TestSupplyPotion checks which HP potion the bot carries for a trip: the
// best one a merchant it can visit sells (in sight, or spawned within
// Shop.Reach), else the best one any spawned merchant sells, and that the
// shop errand buys that one up to its trip target.
func TestSupplyPotion(t *testing.T) {
	tests := []struct {
		name     string
		townAt   Point // spawn of townGrocer
		inSight  bool  // townGrocer stands in sight too
		villager bool  // shotGrocer sells the Lesser Healing Potion
		want     int32
	}{
		{"village grocer near, town grocer far", Point{X: 50000}, false, true, lesserHealingPotion},
		{"town grocer near", Point{X: 6000}, false, true, healingPotion},
		{"town grocer in sight beyond reach", Point{X: 50000}, true, true, healingPotion},
		{"nothing near sells one", Point{X: 50000}, false, false, healingPotion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMeterBrain(t, classHumanFight, wielded(1, ngSword), shotStack(2, soulshotNoGrade, 900))
			village := []datapack.Product{{ItemID: soulshotNoGrade, Price: 8}, {ItemID: scrollOfEscape, Price: 400}}
			if tt.villager {
				village = append(village, datapack.Product{ItemID: lesserHealingPotion, Price: 40})
			}
			shops, cheapest := indexShops([]datapack.BuyList{
				{ID: 1, NPCID: shotGrocer, Products: village},
				{ID: 3, NPCID: townGrocer, Products: []datapack.Product{{ItemID: lesserHealingPotion, Price: 40}, {ItemID: healingPotion, Price: 330}}},
			})
			m := b.cfg.Travel
			m.shops, m.cheapest = shops, cheapest
			m.spawns = map[int32][]Point{shotGrocer: {{X: 5000}}, townGrocer: {tt.townAt}}
			m.spawnedSold = spawnedSold(m.shops, m.spawns)
			m.weights[healingPotion] = 18
			if tt.inSight {
				b.world.Apply(NPCInfo{ObjectID: 900, NPCID: townGrocer, Pos: tt.townAt}, t0)
			}
			if got := b.supplyPotion(); got != tt.want {
				t.Fatalf("supplyPotion = %d, want %d", got, tt.want)
			}
			target := b.tripTarget(tt.want)
			if target <= 0 {
				t.Fatalf("tripTarget(%d) = %d, want > 0", tt.want, target)
			}
			want := buyRow{item: tt.want, count: target}
			if got := b.shopWants(true); !slices.Contains(got, want) {
				t.Fatalf("shopWants = %v, want it to hold %v", got, want)
			}
		})
	}
}

// TestShotTargetRestock checks that a Restock row for the shot a weapon
// buys sets its target as supplies does, in place of the use meter's.
func TestShotTargetRestock(t *testing.T) {
	tests := []struct {
		name    string
		restock []Restock
		want    int32
	}{
		{"use meter", nil, 3600},
		{"restock row", []Restock{{Item: soulshotNoGrade, Min: 100, Max: 800}}, 800},
		{"row of another class", []Restock{{Item: soulshotNoGrade, Min: 100, Max: 800, For: MysticClasses}}, 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMeterBrain(t, classHumanFight, wielded(1, ngSword))
			b.persona.Shop.Restock = tt.restock
			w := b.cfg.Travel.weapons[ngSword]
			if got := b.shotTarget(w); got != tt.want {
				t.Fatalf("shotTarget = %d, want %d", got, tt.want)
			}
			if got := b.supplies()[0]; got.item != soulshotNoGrade || got.count != tt.want {
				t.Fatalf("supplies()[0] = %v, want %d of %d", got, tt.want, soulshotNoGrade)
			}
		})
	}
}
