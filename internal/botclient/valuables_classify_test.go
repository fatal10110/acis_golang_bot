package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// newValuableMember is newMemberBrain with the default ValuablePolicy, the
// persona's own keep hook hook installed before the member option wraps
// it, and the classification fixture's prices and supply (one an hour, R
// 2): the D blade (250000, graded, sold by no shop), the D sword (150000,
// sold by the gear merchant), the no-grade sword (100000) and the iron
// recipe scroll (366000) are valuable by price and rarity; Spirit Ore
// (5000) is under PriceFloor and Steel (20000, 50 an hour) under ValueMin.
func newValuableMember(t *testing.T, hook func(*World, Item) Hold, items ...Item) *Brain {
	t.Helper()
	b := newMeterBrain(t, classHumanFight, items...)
	b.world.Clan.ID, b.world.Self.Name = memberClan, memberName
	m := b.cfg.Travel
	m.recipes = map[int32]datapack.Recipe{
		20: shotRecipes[20], 317: shotRecipes[317],
		ironRecipe: {
			ID: ironRecipe, ScrollID: ironScroll, Level: 1, SuccessRate: 100, Dwarven: true,
			Materials: []datapack.Ingredient{{ItemID: ironOre, Count: 2}}, Product: datapack.Ingredient{ItemID: steel, Count: 1},
		},
	}
	m.crystals = map[int32]crystalYield{dSword: {grade: 1, crystal: crystalD, count: 100}, dBlade: {grade: 1, crystal: crystalD, count: 200}}
	m.weights[ironOre], m.weights[ironScroll], m.weights[dSword], m.weights[dBlade] = 20, 30, 1500, 1700
	m.prices = map[int32]int32{dBlade: 250000, dSword: 150000, ngSword: 100000, ironScroll: 366000, spiritOre: 5000, steel: 20000}
	m.supply = map[int32]itemSupply{dBlade: {perHour: 1}, dSword: {perHour: 1}, ngSword: {perHour: 1}, ironScroll: {perHour: 1}, spiritOre: {perHour: 1}, steel: {perHour: 50}}
	b.cfg.CraftBoard = NewCraftBoard()
	b.cfg.CraftBoard.Publish(memberClan, CrafterEntry{Bot: memberCrafty, Name: "crafter", Recipes: []int32{20, ironRecipe}, ShotGrades: []int{1}})
	b.persona.Valuables = DefaultValuablePolicy()
	b.persona.Holds = hook
	b.persona.ClanMember = true
	b.wireMember()
	return b
}

// TestValuableClassification checks the valuable test and its precedence
// (docs/valuables/score.md "Valuable test", D31): the bot's keeps and the
// persona's own hook, then the gear errand, then valuable, then the clan
// member's hold, then loot; a held valuable is in no NPC sale, store row
// or trade offer outside a round.
func TestValuableClassification(t *testing.T) {
	spareBlade := spare(20, dBlade)
	plain := spare(22, ngSword)
	keepPlain := func(_ *World, it Item) Hold { return Hold{Keep: it.TemplateID == ngSword} }
	tests := []struct {
		name  string
		wield int32
		it    Item
		extra []Item
		hook  func(*World, Item) Hold
		setup func(t *testing.T, b *Brain)
		want  bool
	}{
		{name: "graded gear it would not wear", wield: dBlade, it: spareBlade, want: true},
		{name: "no grade", wield: dBlade, it: plain, want: true},
		{name: "recipe scroll", wield: dBlade, it: lootStack(10, ironScroll, 1), want: true},
		{name: "equipped", wield: dBlade, it: wielded(1, dBlade)},
		{name: "worn next", wield: ngSword, it: spareBlade, setup: func(t *testing.T, b *Brain) {
			if !b.wearsNext(spareBlade) {
				t.Fatal("fixture: the gear errand does not wear the D blade over the no-grade sword")
			}
		}},
		{name: "fallen back to", wield: dSword, it: plain, extra: []Item{shotStack(11, dSoulshot, 5000)}, setup: func(t *testing.T, b *Brain) {
			b.world.Clan.ID = 0 // no crafter on the board makes the D shots
			if w := b.worn(); !b.fallsBack(plain, &w) {
				t.Fatal("fixture: the bot does not fall back to the no-grade sword")
			}
		}},
		{name: "kept by the bot", wield: dBlade, it: plain, setup: func(_ *testing.T, b *Brain) { b.persona.Shop.Keep = []int32{ngSword} }},
		{name: "kept by the persona's own hook", wield: dBlade, it: plain, hook: keepPlain},
		{name: "quest item", wield: dBlade, it: Item{ObjectID: 22, TemplateID: ngSword, Count: 1, SubCategory: subCategoryQuest}},
		{name: "augmented", wield: dBlade, it: Item{ObjectID: 22, TemplateID: ngSword, Count: 1, BodySlot: partRHand, Augmentation: 7}},
		{name: "shadow item", wield: dBlade, it: plain, setup: func(_ *testing.T, b *Brain) { b.cfg.Travel.shadows = map[int32]bool{ngSword: true} }},
		{name: "sold by a counted buylist", wield: dBlade, it: spare(21, dSword)},
		{name: "under PriceFloor", wield: dBlade, it: lootStack(13, spiritOre, 1)},
		{name: "under ValueMin", wield: dBlade, it: lootStack(13, steel, 1)},
		{name: "policy off", wield: dBlade, it: spareBlade, setup: func(_ *testing.T, b *Brain) { b.persona.Valuables = ValuablePolicy{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := append([]Item{wielded(1, tt.wield)}, tt.extra...)
			if tt.it.ObjectID != 1 {
				items = append(items, tt.it)
			}
			b := newValuableMember(t, tt.hook, items...)
			if tt.setup != nil {
				tt.setup(t, b)
			}
			if got := b.valuable(tt.it); got != tt.want {
				t.Errorf("valuable = %v, want %v", got, tt.want)
			}
			if b.member.inHold {
				t.Error("inHold left set")
			}
		})
	}

	t.Run("clan member", func(t *testing.T) {
		recipe, otherRecipe, ore := lootStack(10, ironScroll, 1), lootStack(12, 3032, 1), lootStack(13, ironOre, 7)
		spareSword := spare(21, dSword)
		items := []Item{wielded(1, dBlade), recipe, otherRecipe, ore, spareBlade, spareSword, plain}
		on := newValuableMember(t, nil, items...)
		off := newValuableMember(t, nil, items...)
		off.persona.Valuables = ValuablePolicy{}
		for _, tt := range []struct {
			it      Item
			on, off Hold
		}{
			{recipe, Hold{}, Hold{Keep: true}},
			{spareBlade, Hold{}, Hold{Keep: true, NoWear: true}},
			{plain, Hold{}, Hold{}},
			{otherRecipe, Hold{Keep: true}, Hold{Keep: true}},
			{ore, Hold{Keep: true}, Hold{Keep: true}},
			{spareSword, Hold{Keep: true, NoWear: true}, Hold{Keep: true, NoWear: true}},
		} {
			if got := on.hold(tt.it); got != tt.on {
				t.Errorf("hold(%d) with valuables = %+v, want %+v", tt.it.TemplateID, got, tt.on)
			}
			if got := off.hold(tt.it); got != tt.off {
				t.Errorf("hold(%d) without valuables = %+v, want %+v", tt.it.TemplateID, got, tt.off)
			}
		}
		objects := func(b *Brain) []int32 {
			var out []int32
			for _, r := range b.memberDeposit(nil) {
				if r.Object != 0 {
					out = append(out, r.Object)
				}
			}
			return out
		}
		if got, want := objects(on), []int32{12, 13, 21}; !slices.Equal(got, want) {
			t.Errorf("deposit with valuables = %v, want %v: no valuable", got, want)
		}
		if got, want := objects(off), []int32{10, 12, 13, 20, 21}; !slices.Equal(got, want) {
			t.Errorf("deposit without valuables = %v, want %v as today", got, want)
		}
		if got, want := off.memberHandOff()-on.memberHandOff(), int64(1700+30); got != want {
			t.Errorf("hand-off weight difference = %d, want %d: the blade and the recipe scroll", got, want)
		}
		on.persona.Supply.InTown(on.world)
		off.persona.Supply.InTown(off.world)
		if got, want := off.member.fees-on.member.fees, int64(2*warehouseFee); got != want {
			t.Errorf("arrival fees difference = %d, want %d: two rows fewer", got, want)
		}
		if on.member.inHold || off.member.inHold {
			t.Error("inHold left set")
		}
	})

	t.Run("no sale outside a round", func(t *testing.T) {
		sword := swordUnit(101, 0)
		swordRow := TradeItem{ObjectID: sword.ObjectID, TemplateID: ledgerSword, Count: 1}
		lootRow := TradeItem{ObjectID: swordLoot.ObjectID, TemplateID: stoneSword, Count: 1}
		for _, enabled := range []bool{true, false} {
			b := newLedgerBrain(t, sword, swordLoot)
			b.cfg.Travel.prices[stoneSword] = 1000
			if !enabled {
				b.persona.Valuables = ValuablePolicy{}
			}
			junk := b.junk()
			if _, ok := junk[sword.ObjectID]; ok == enabled {
				t.Errorf("enabled %v: sword in junk %v", enabled, ok)
			}
			if _, ok := junk[swordLoot.ObjectID]; !ok {
				t.Errorf("enabled %v: loot not junk", enabled)
			}
			b.store.candidates = []StoreItem{
				{ObjectID: sword.ObjectID, TemplateID: ledgerSword, Count: 1, BodySlot: partRHand, ReferencePrice: 136000},
				swordCand,
			}
			var listed []int32
			for _, r := range b.storeRows() {
				listed = append(listed, r.Object)
			}
			if want := map[bool][]int32{true: {swordLoot.ObjectID}, false: {sword.ObjectID, swordLoot.ObjectID}}[enabled]; !slices.Equal(listed, want) {
				t.Errorf("enabled %v: store rows %v, want %v", enabled, listed, want)
			}
			b.persona.Trade.Prices = map[int32]int32{ledgerSword: 200000}
			row, ok := b.tradeGoods([]TradeItem{swordRow, lootRow})
			if want := map[bool]int32{true: lootRow.ObjectID, false: swordRow.ObjectID}[enabled]; !ok || row.ObjectID != want {
				t.Errorf("enabled %v: trade offers %+v, %v; want object %d", enabled, row, ok, want)
			}
		}
	})
}
