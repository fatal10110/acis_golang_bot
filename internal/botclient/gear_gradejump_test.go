package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// wornLongSword is a wielded Long Sword, which a Falchion beats by a little.
var wornLongSword = Item{ObjectID: 201, TemplateID: longSword, Count: 1, Equipped: true, BodySlot: partRHand}

// newJumpBrain is newGearBrain for a fighter of level level with Expertise
// at expertise, wearing a Squire's Shirt and Pants and the weapon given,
// carrying adena and one of the two Scrolls of Escape it takes a trip
// (refilled under a quarter of them, so the second is comfort), merchant
// 30001 selling Salamander Mail too.
func newJumpBrain(t *testing.T, level, expertise, adena int32, weapon Item, items ...Item) *Brain {
	t.Helper()
	carried := append([]Item{weapon, wornShirt, wornPants, adenaStack(adena), {ObjectID: 300, TemplateID: scrollOfEscape, Count: 1}}, items...)
	b := newGearBrain(t, expertise, carried...)
	b.world.Self.Level = level
	b.persona.Supply = SupplyPolicy{Refill: 25, Escapes: 2}
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: append(slices.Clone(gearShop), datapack.Product{ItemID: salamanderMail, Price: 429000})}})
	cheapest[scrollOfEscape] = soePrice
	gear, shopGear := indexGear(gearTemplates, cheapest)
	m := b.cfg.Travel
	m.shops, m.cheapest, m.gear, m.shopGear = shops, cheapest, gear, shopGear
	m.spawnedSold = map[int32]bool{scrollOfEscape: true}
	return b
}

// TestGradeJumpBuys checks a visit's purchase before a grade jump
// (GearPolicy.SaveBefore, SaveWorth): within two levels of Expertise D, a
// small no-grade upgrade is skipped and its adena kept, saving for the D
// weapon skips the comfort tier, a big upgrade is still bought, and at
// level 20 the savings buy the D weapon.
func TestGradeJumpBuys(t *testing.T) {
	const gearKeep = 10000
	window := []ShopItem{
		{TemplateID: falchion, Price: 244000},
		{TemplateID: artisansSword, Price: 644000},
		{TemplateID: salamanderMail, Price: 429000},
		{TemplateID: scrollOfEscape, Price: soePrice},
	}
	falchionAndSoE := []buyRow{{falchion, 1}, {scrollOfEscape, 1}}
	tests := []struct {
		name      string
		level     int32
		expertise int32
		adena     int32
		weapon    Item
		edit      func(b *Brain)
		want      []buyRow
	}{
		{name: "far from the jump: the small upgrade", level: 10, adena: gearKeep + 400000, weapon: wornLongSword, want: falchionAndSoE},
		{name: "three levels before: the small upgrade", level: 17, adena: gearKeep + 400000, weapon: wornLongSword, want: falchionAndSoE},
		{name: "level 18: small upgrade skipped, saving skips comfort", level: 18, adena: gearKeep + 400000, weapon: wornLongSword},
		{name: "level 19: small upgrade skipped, saving skips comfort", level: 19, adena: gearKeep + 400000, weapon: wornLongSword},
		{
			name: "level 18: small upgrade skipped, D weapon out of reach", level: 18, adena: gearKeep + 300000, weapon: wornLongSword,
			want: []buyRow{{scrollOfEscape, 1}},
		},
		{name: "level 18: a big upgrade still bought", level: 18, adena: gearKeep + 400000, weapon: wornSword, want: falchionAndSoE},
		{
			name: "SaveWorth 0 buys every upgrade", level: 18, adena: gearKeep + 400000, weapon: wornLongSword,
			edit: func(b *Brain) { b.persona.Gear.SaveWorth = 0 },
			want: falchionAndSoE,
		},
		{
			name: "SaveWorth 0.5 buys this one", level: 18, adena: gearKeep + 400000, weapon: wornLongSword,
			edit: func(b *Brain) { b.persona.Gear.SaveWorth = 0.5 },
			want: falchionAndSoE,
		},
		{
			name: "SaveBefore 0 never saves", level: 19, adena: gearKeep + 400000, weapon: wornLongSword,
			edit: func(b *Brain) { b.persona.Gear.SaveBefore = 0 },
			want: falchionAndSoE,
		},
		{
			name: "no saving past MaxGrade", level: 18, adena: gearKeep + 400000, weapon: wornLongSword,
			edit: func(b *Brain) { b.persona.Gear.MaxGrade = 0 },
			want: falchionAndSoE,
		},
		{name: "level 20: the savings buy the D weapon", level: 20, expertise: 1, adena: gearKeep + 700000, weapon: wornLongSword, want: []buyRow{{artisansSword, 1}, {scrollOfEscape, 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newJumpBrain(t, tt.level, tt.expertise, tt.adena, tt.weapon)
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.buyRows(BuyList{Money: tt.adena, ListID: 1, Items: window}); !slices.Equal(got, tt.want) {
				t.Fatalf("buyRows = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestJumpGoal checks the grade jump's goal: the next grade's best weapon
// within GearPolicy.SaveBefore levels of its Expertise, its body armor once
// a weapon of that grade is carried, and nothing outside the window, past
// MaxGrade or with none of that grade sold.
func TestJumpGoal(t *testing.T) {
	carriedArtisan := Item{ObjectID: 220, TemplateID: artisansSword, Count: 1, BodySlot: partRHand}
	tests := []struct {
		name      string
		level     int32
		expertise int32
		items     []Item
		edit      func(b *Brain)
		want      []int32
	}{
		{name: "two levels before D: the D weapon", level: 18, want: []int32{artisansSword}},
		{name: "one level before D: the D weapon", level: 19, want: []int32{artisansSword}},
		{name: "three levels before D: none", level: 17},
		{name: "at level 20, Expertise not yet learned: none", level: 20},
		{name: "D weapon carried: the D body armor", level: 18, items: []Item{carriedArtisan}, want: []int32{salamanderMail}},
		{name: "D worn, before C: no C sold", level: 38, expertise: 1},
		{name: "past MaxGrade: none", level: 18, edit: func(b *Brain) { b.persona.Gear.MaxGrade = 0 }},
		{name: "Buy off: none", level: 18, edit: func(b *Brain) { b.persona.Gear.Buy = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newJumpBrain(t, tt.level, tt.expertise, 1000000, wornLongSword, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.jumpGoal(); !slices.Equal(got, tt.want) {
				t.Fatalf("jumpGoal = %v, want %v", got, tt.want)
			}
			if b.gear.ahead {
				t.Fatal("jumpGoal left the plan a grade ahead")
			}
			if goal, _ := b.gearGoal(); slices.ContainsFunc(goal, func(id int32) bool { return b.cfg.Travel.gear[id].grade > b.gearGrade() }) {
				t.Fatalf("gearGoal = %v buys above the worn grade", goal)
			}
		})
	}
}
