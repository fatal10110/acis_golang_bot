package botclient

import (
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Made-up light armor of the grade jump tests: the D pieces dChest, dLegs
// and dHead (set D), the no-grade full armor ngFull, and the no-grade set
// NG (ngChest, which barely beats a Squire's Shirt, and ngLegs, which
// doubles Squire's Pants).
const (
	dChest  int32 = 92001 // grade D, P. Def 80
	dLegs   int32 = 92002 // grade D, P. Def 50
	dHead   int32 = 92003 // grade D, P. Def 30
	ngFull  int32 = 92010 // full armor, P. Def 60
	ngChest int32 = 92020 // P. Def 34
	ngLegs  int32 = 92021 // P. Def 40
)

// jumpTemplates are gearTemplates and the grade jump tests' pieces.
var jumpTemplates = func() map[int32]datapack.Item {
	m := maps.Clone(gearTemplates)
	add := func(id int32, part string, grade int, price int32, pDef float64) {
		m[id] = datapack.Item{ID: id, Type: "Armor", BodyPart: part, Kind: "LIGHT", Grade: grade, Price: price, Stats: datapack.Stats{PDef: pDef}}
	}
	add(dChest, "chest", 1, 200000, 80)
	add(dLegs, "legs", 1, 120000, 50)
	add(dHead, "head", 1, 50000, 30)
	add(ngFull, "fullarmor", 0, 100000, 60)
	add(ngChest, "chest", 0, 30000, 34)
	add(ngLegs, "legs", 0, 30000, 40)
	return m
}()

// jumpSets are the grade jump tests' armorSets.xml rows.
var jumpSets = map[int32]datapack.ArmorSet{
	dChest:  {Name: "D", Chest: dChest, Legs: dLegs, Head: dHead},
	ngChest: {Name: "NG", Chest: ngChest, Legs: ngLegs},
}

// newJumpShopBrain is newJumpBrain whose merchant sells the Artisan's
// Sword (the D weapon, gearShop's) and products, over jumpTemplates and,
// with sets, jumpSets.
func newJumpShopBrain(t *testing.T, level, adena int32, products []datapack.Product, sets bool, items ...Item) *Brain {
	t.Helper()
	b := newJumpBrain(t, level, 0, adena, wornLongSword, items...)
	m := b.cfg.Travel
	m.shops, m.cheapest = indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: append([]datapack.Product{{ItemID: artisansSword, Price: 644000}}, products...)}})
	m.cheapest[scrollOfEscape] = soePrice
	m.gear, m.shopGear = indexGear(jumpTemplates, m.cheapest)
	if sets {
		m.sets, m.setPieces = indexSets(jumpSets, m.gear)
	}
	return b
}

// TestJumpGoalArmor checks the grade jump's armor goal once a weapon of the
// next grade is carried: that grade's body and legs pieces with no set of
// it planned, and the planned set's pieces with one.
func TestJumpGoalArmor(t *testing.T) {
	carriedArtisan := Item{ObjectID: 220, TemplateID: artisansSword, Count: 1, BodySlot: partRHand}
	dPieces := []datapack.Product{{ItemID: dChest, Price: 200000}, {ItemID: dLegs, Price: 120000}, {ItemID: dHead, Price: 50000}}
	tests := []struct {
		name string
		sets bool
		want []int32
	}{
		{name: "no D set: the D body and legs", want: []int32{dChest, dLegs}},
		{name: "D set within reach: its pieces", sets: true, want: []int32{dChest, dLegs, dHead}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newJumpShopBrain(t, 18, 1000000, dPieces, tt.sets, carriedArtisan)
			if got := b.jumpGoal(); !slices.Equal(got, tt.want) {
				t.Fatalf("jumpGoal = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGradeJumpWorth checks GearPolicy.SaveWorth's rule in the grade jump's
// window, saving for the 644000 Artisan's Sword: a piece filling an empty
// slot is bought; full armor is judged against the worn shirt and pants
// both; a set's pieces are judged by the set's combined gain, and with the
// set not worth it, piece by piece.
func TestGradeJumpWorth(t *testing.T) {
	const adena = 10000 + 400000
	soe := ShopItem{TemplateID: scrollOfEscape, Price: soePrice}
	ngSet := func(price int32) []datapack.Product {
		return []datapack.Product{{ItemID: ngChest, Price: price}, {ItemID: ngLegs, Price: price}}
	}
	tests := []struct {
		name     string
		level    int32
		products []datapack.Product
		sets     bool
		want     []buyRow
	}{
		{
			// No worn helmet to gain over.
			name: "level 18: a helmet for the empty head slot", level: 18,
			products: []datapack.Product{{ItemID: leatherHelmet, Price: 10200}},
			want:     []buyRow{{leatherHelmet, 1}},
		},
		{
			name: "level 10: the full armor", level: 10,
			products: []datapack.Product{{ItemID: ngFull, Price: 100000}},
			want:     []buyRow{{ngFull, 1}, {scrollOfEscape, 1}},
		},
		{
			// 60 over 33+20 is 13%, under its 16% share of the D weapon;
			// over the shirt alone it would be 82%.
			name: "level 18: the full armor skipped, the pants counted", level: 18,
			products: []datapack.Product{{ItemID: ngFull, Price: 100000}},
		},
		{
			// 13% over its 6% share.
			name: "level 18: a cheaper full armor bought", level: 18,
			products: []datapack.Product{{ItemID: ngFull, Price: 40000}},
			want:     []buyRow{{ngFull, 1}},
		},
		{
			// The chest gains 3% for a 5% share, the legs 100% for 5%.
			name: "level 18, no set: only the legs", level: 18,
			products: ngSet(30000),
			want:     []buyRow{{ngLegs, 1}},
		},
		{
			// Together 74 over 53 is 40%, over their 9% share.
			name: "level 18: the set bought by its combined gain", level: 18,
			products: ngSet(30000), sets: true,
			want: []buyRow{{ngChest, 1}, {ngLegs, 1}},
		},
		{
			// 40% under their 47% share: the legs alone, 100% for 23%.
			// The 250000 left is under half the D weapon: no saving.
			name: "level 18: the dear set skipped, its legs bought", level: 18,
			products: ngSet(150000), sets: true,
			want: []buyRow{{ngLegs, 1}, {scrollOfEscape, 1}},
		},
		{
			name: "level 10: the dear set bought", level: 10,
			products: ngSet(150000), sets: true,
			want: []buyRow{{ngChest, 1}, {ngLegs, 1}, {scrollOfEscape, 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newJumpShopBrain(t, tt.level, adena, tt.products, tt.sets)
			window := []ShopItem{{TemplateID: artisansSword, Price: 644000}}
			for _, p := range tt.products {
				window = append(window, ShopItem{TemplateID: p.ItemID, Price: p.Price})
			}
			window = append(window, soe)
			if got := b.buyRows(BuyList{Money: adena, ListID: 1, Items: window}); !slices.Equal(got, tt.want) {
				t.Fatalf("buyRows = %v, want %v", got, tt.want)
			}
		})
	}
}
