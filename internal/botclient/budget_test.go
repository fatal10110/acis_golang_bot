package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// soePrice is a Scroll of Escape's price at the budget tests' merchant.
const soePrice int32 = 400

// newBudgetBrain is newGearBrain for a level 1 fighter's worn gear and
// adena carried, carrying two Scrolls of Escape a trip (refilled under a
// quarter of them) that a spawned merchant sells at soePrice.
func newBudgetBrain(t *testing.T, adena int32, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, 0, append([]Item{wornSword, wornShirt, wornPants, adenaStack(adena)}, items...)...)
	b.persona.Supply = SupplyPolicy{Refill: 25, Escapes: 2}
	m := b.cfg.Travel
	m.cheapest[scrollOfEscape] = soePrice
	m.spawnedSold = map[int32]bool{scrollOfEscape: true}
	return b
}

// TestBudgetTiers checks a visit's one budget against fixed adena: the
// reserve (ShopPolicy.KeepAdena, at least one Scroll of Escape) is never
// spent, the supplies are bought before the gear above GearPolicy.KeepAdena,
// a gear goal within reach skips the comfort tier, and a broke bot buys
// what the reserve leaves.
func TestBudgetTiers(t *testing.T) {
	const keep, gearKeep = 1000, 10000
	carriedSoE := Item{ObjectID: 300, TemplateID: scrollOfEscape, Count: 1}
	soeOnly := []ShopItem{{TemplateID: scrollOfEscape, Price: soePrice}}
	armor := append([]ShopItem{{TemplateID: leatherShirt22, Price: 2430}}, soeOnly...)
	goal := append([]ShopItem{{TemplateID: falchion, Price: 244000}}, soeOnly...)
	tests := []struct {
		name   string
		adena  int32
		items  []Item
		edit   func(b *Brain)
		window []ShopItem
		want   []buyRow
	}{
		{name: "broke: what the reserve leaves", adena: keep + 500, window: soeOnly, want: []buyRow{{scrollOfEscape, 1}}},
		{name: "broke: nothing under the reserve", adena: keep + 399, window: soeOnly},
		{
			name: "reserve: one SoE over a lower KeepAdena", adena: 1000, window: soeOnly,
			edit: func(b *Brain) { b.persona.Shop.KeepAdena = 0 },
			want: []buyRow{{scrollOfEscape, 1}},
		},
		{name: "supplies before gear", adena: gearKeep + 2430 + 400, window: armor, want: []buyRow{{scrollOfEscape, 2}}},
		{
			name: "no supplies: the gear", adena: gearKeep + 2430 + 400, window: armor,
			edit: func(b *Brain) { b.persona.Supply.Escapes = 0 },
			want: []buyRow{{leatherShirt22, 1}},
		},
		{name: "comfort tops the supplies up", adena: gearKeep + 100000, items: []Item{carriedSoE}, window: goal, want: []buyRow{{scrollOfEscape, 1}}},
		{name: "saving for the goal skips comfort", adena: gearKeep + 130000, items: []Item{carriedSoE}, window: goal},
		{
			name: "no saving below SaveFrom", adena: gearKeep + 130000, items: []Item{carriedSoE}, window: goal,
			edit: func(b *Brain) { b.persona.Gear.SaveFrom = 90 },
			want: []buyRow{{scrollOfEscape, 1}},
		},
		{
			name: "SaveFrom 0 never saves", adena: gearKeep + 130000, items: []Item{carriedSoE}, window: goal,
			edit: func(b *Brain) { b.persona.Gear.SaveFrom = 0 },
			want: []buyRow{{scrollOfEscape, 1}},
		},
		{name: "the goal bought, then comfort", adena: gearKeep + 244000 + 400, items: []Item{carriedSoE}, window: goal, want: []buyRow{{falchion, 1}, {scrollOfEscape, 1}}},
		{
			name: "comfort off", adena: gearKeep + 100000, items: []Item{carriedSoE}, window: goal,
			edit: func(b *Brain) { b.persona.Shop.Comfort = false },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBudgetBrain(t, tt.adena, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			got := b.buyRows(BuyList{Money: tt.adena, ListID: 1, Items: tt.window})
			if !slices.Equal(got, tt.want) {
				t.Fatalf("buyRows = %v, want %v", got, tt.want)
			}
			var cost int64
			for _, r := range got {
				j := slices.IndexFunc(tt.window, func(it ShopItem) bool { return it.TemplateID == r.item })
				cost += int64(r.count) * int64(tt.window[j].Price)
			}
			if left := int64(tt.adena) - cost; left < b.reserve() {
				t.Fatalf("left %d adena, under the reserve %d", left, b.reserve())
			}
		})
	}
}

// TestBudgetComfortNeverDue checks that the comfort tier makes no visit
// due: a bot short of a full trip target but over Refill percent of it
// wants nothing.
func TestBudgetComfortNeverDue(t *testing.T) {
	b := newBudgetBrain(t, 100000, Item{ObjectID: 300, TemplateID: scrollOfEscape, Count: 1})
	b.persona.Gear.Buy = false
	if got := b.shopWants(true); len(got) != 0 {
		t.Fatalf("shopWants = %v, want none", got)
	}
}

// TestKeepsLoadout checks that what the loadout buys is never junk: the
// Scroll of Escape (the keep set) with no Escapes wanted, and an Extra
// supply while the hook asks for it.
func TestKeepsLoadout(t *testing.T) {
	soe := Item{ObjectID: 4, TemplateID: scrollOfEscape, Count: 1}
	ore := Item{ObjectID: 5, TemplateID: soulOre, Count: 10}
	b := newMeterBrain(t, classHumanFight, wielded(1, ngSword), soe, ore)
	b.persona.Supply.Escapes = 0
	b.persona.Supply.Extra = func(*World) map[int32]int32 { return map[int32]int32{soulOre: 500} }
	junk := b.junk()
	for _, it := range []Item{soe, ore} {
		if _, ok := junk[it.ObjectID]; ok {
			t.Errorf("junk holds %d, want it kept", it.TemplateID)
		}
	}
	b.persona.Supply.Extra = nil
	if _, ok := b.junk()[ore.ObjectID]; !ok {
		t.Error("no Extra supply: ore not junk")
	}
}

// TestKeepHook checks that what the persona's keep hook keeps, a material,
// a no-grade weapon and an unworn D-grade weapon no buylist sells, is
// neither sold to a merchant, listed in the store nor priced or offered in
// a trade.
func TestKeepHook(t *testing.T) {
	dLoot := Item{ObjectID: 110, TemplateID: dSword, Count: 1, BodySlot: partRHand}
	for _, kept := range []Item{skinLoot, swordLoot, dLoot} {
		b := newLootTrader(t, adenaStack(1000), skinLoot, swordLoot, dLoot)
		m := b.cfg.Travel
		m.prices = maps.Clone(lootPrices)
		m.prices[dSword] = 150000
		m.gear = map[int32]gearItem{dSword: {part: partRHand, kind: "SWORD", grade: 1}}
		if _, shopSells := m.cheapest[dSword]; shopSells {
			t.Fatal("a buylist sells the D-grade sword")
		}
		rows := []TradeItem{{ObjectID: kept.ObjectID, TemplateID: kept.TemplateID, Count: 1}}
		if _, ok := b.junk()[kept.ObjectID]; !ok || !b.storeLoot(kept) {
			t.Fatalf("%d without a hook: junk %v, storeLoot %v, want both", kept.TemplateID, ok, b.storeLoot(kept))
		}
		if _, ok := b.tradeGoods(rows); !ok {
			t.Fatalf("%d without a hook: not offered", kept.TemplateID)
		}
		b.persona.Holds = func(_ *World, it Item) Hold { return Hold{Keep: it.ObjectID == kept.ObjectID} }
		if _, ok := b.junk()[kept.ObjectID]; ok {
			t.Errorf("%d kept: junk", kept.TemplateID)
		}
		if b.storeLoot(kept) {
			t.Errorf("%d kept: store loot", kept.TemplateID)
		}
		if p, ok := b.tradePrice(kept); ok {
			t.Errorf("%d kept: trade price %d", kept.TemplateID, p)
		}
		if row, ok := b.tradeGoods(rows); ok {
			t.Errorf("%d kept: offered %+v", kept.TemplateID, row)
		}
	}
}

// TestKeepHookNoWear checks that the gear errand does not wear a piece the
// keep hook keeps and marks NoWear, and that it is not junk either.
func TestKeepHookNoWear(t *testing.T) {
	b := newGearBrain(t, 0, wornSword, newSword)
	b.persona.Holds = func(_ *World, it Item) Hold {
		return Hold{Keep: it.ObjectID == newSword.ObjectID, NoWear: it.ObjectID == newSword.ObjectID}
	}
	if b.wearsNext(newSword) {
		t.Error("wearsNext(kept no-wear sword)")
	}
	if a, ok := b.ruleGear(at(time.Second)); ok {
		t.Errorf("ruleGear = %+v, want no match", a)
	}
	if _, ok := b.junk()[newSword.ObjectID]; ok {
		t.Error("kept sword is junk")
	}
	b.persona.Holds = func(*World, Item) Hold { return Hold{Keep: true} }
	if a, ok := b.ruleGear(at(time.Second)); !ok || a.object != newSword.ObjectID {
		t.Errorf("kept, wearable: ruleGear = %+v, %v, want the long sword", a, ok)
	}
}
