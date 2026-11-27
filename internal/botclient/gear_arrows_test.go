package botclient

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Bows and arrows of the arrow tests (aCis_datapack/data/xml/items ids).
const (
	shortBow        int32 = 13
	plainBow        int32 = 14
	strengthenedBow int32 = 274
	elvenBow        int32 = 276
	boneArrow       int32 = 1341
	fineSteelArrow  int32 = 1342
)

// arrowTemplates are gearTemplates plus bows and arrows: a C-grade bow
// with no arrow template of its grade, and a soulshot the index ignores.
var arrowTemplates = func() map[int32]datapack.Item {
	m := maps.Clone(gearTemplates)
	maps.Copy(m, map[int32]datapack.Item{
		shortBow:        {ID: shortBow, Type: "Weapon", BodyPart: "lrhand", Kind: "BOW", Price: 768, Stats: datapack.Stats{PAtk: 16, MAtk: 6, PAtkSpd: 293}},
		plainBow:        {ID: plainBow, Type: "Weapon", BodyPart: "lrhand", Kind: "BOW", Price: 12500, Stats: datapack.Stats{PAtk: 23, MAtk: 9, PAtkSpd: 293}},
		strengthenedBow: {ID: strengthenedBow, Type: "Weapon", BodyPart: "lrhand", Kind: "BOW", Grade: 1, Price: 300000, Stats: datapack.Stats{PAtk: 82, MAtk: 26, PAtkSpd: 293}, Enchants: []string{"pAtk", "mAtk"}},
		elvenBow:        {ID: elvenBow, Type: "Weapon", BodyPart: "lrhand", Kind: "BOW", Grade: 2, Price: 900000, Stats: datapack.Stats{PAtk: 105, MAtk: 32, PAtkSpd: 293}, Enchants: []string{"pAtk", "mAtk"}},
		woodenArrow:     {ID: woodenArrow, Type: "EtcItem", BodyPart: "lhand", Kind: "ARROW", Price: 2},
		boneArrow:       {ID: boneArrow, Type: "EtcItem", BodyPart: "lhand", Kind: "ARROW", Grade: 1, Price: 3},
		// A second D-grade arrow: the lowest id is kept.
		1400: {ID: 1400, Type: "EtcItem", BodyPart: "lhand", Kind: "ARROW", Grade: 1, Price: 3},
	})
	return m
}()

// arrowShop is gearShop plus a bow and the arrows of grades none and D.
var arrowShop = append(slices.Clone(gearShop),
	datapack.Product{ItemID: plainBow, Price: 12500},
	datapack.Product{ItemID: woodenArrow, Price: 2},
	datapack.Product{ItemID: boneArrow, Price: 3},
)

// Bows and arrows worn or carried by the arrow tests.
var (
	wornShortBow   = Item{ObjectID: 301, TemplateID: shortBow, Count: 1, Equipped: true, BodySlot: partLRHand}
	wornStrongBow  = Item{ObjectID: 302, TemplateID: strengthenedBow, Count: 1, Equipped: true, BodySlot: partLRHand}
	carriedBow     = Item{ObjectID: 303, TemplateID: plainBow, Count: 1, BodySlot: partLRHand}
	carriedArrows  = Item{ObjectID: 310, TemplateID: woodenArrow, Count: 500, BodySlot: partLHand}
	fewArrows      = Item{ObjectID: 310, TemplateID: woodenArrow, Count: 50, BodySlot: partLHand}
	boneArrowsOf   = func(n int32) Item { return Item{ObjectID: 311, TemplateID: boneArrow, Count: n, BodySlot: partLHand} }
	wornBoneArrows = func(n int32) Item {
		return Item{ObjectID: 311, TemplateID: boneArrow, Count: n, Equipped: true, BodySlot: partLHand}
	}
)

// newArcherBrain is newGearBrain for a Hawkeye with the bows and arrows
// of arrowTemplates and arrowShop.
func newArcherBrain(t *testing.T, expertise int32, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, expertise, items...)
	b.world.Self.ClassID = classHawkeye
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: arrowShop}})
	gear, shopGear := indexGear(arrowTemplates, cheapest)
	b.cfg.Travel = &TravelMap{
		spawns: map[int32][]Point{gearMerchant: {{X: 5000}}}, shops: shops, cheapest: cheapest,
		gear: gear, shopGear: shopGear, arrows: indexArrows(arrowTemplates),
	}
	return b
}

// TestIndexArrows checks the arrow each bow shoots, and that arrows are
// never gear: the server answers a UseItem of arrows with ActionFailed.
func TestIndexArrows(t *testing.T) {
	want := map[int32]int32{shortBow: woodenArrow, plainBow: woodenArrow, strengthenedBow: boneArrow}
	if got := indexArrows(arrowTemplates); !reflect.DeepEqual(got, want) {
		t.Errorf("indexArrows = %v, want %v", got, want)
	}
	gear, _ := indexGear(arrowTemplates, nil)
	for _, id := range []int32{woodenArrow, boneArrow, 1400} {
		if _, ok := gear[id]; ok {
			t.Errorf("arrow %d indexed as gear", id)
		}
	}
	if g := gear[plainBow]; g.kind != "BOW" || g.part != partLRHand {
		t.Errorf("bow gear = %+v, want a two-handed BOW", g)
	}
}

// TestArrowWants checks the arrows the shop errand buys: the ones of the
// wielded bow's grade, under ArrowsMin carried, up to ArrowsMax.
func TestArrowWants(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		edit  func(*Brain)
		want  []buyRow
	}{
		{name: "no bow", items: []Item{wornSword, carriedArrows}},
		{name: "bow carried, worn next", items: []Item{carriedBow}, want: []buyRow{{Item: woodenArrow, Count: 1000}}},
		{name: "bow carried, not worn next", items: []Item{carriedBow}, edit: func(b *Brain) { b.world.Self.ClassID = classTreasureHunter }},
		{name: "no-grade bow, no arrows", items: []Item{wornShortBow}, want: []buyRow{{Item: woodenArrow, Count: 1000}}},
		{name: "D bow, a few of its arrows", items: []Item{wornStrongBow, boneArrowsOf(150)}, want: []buyRow{{Item: boneArrow, Count: 850}}},
		{name: "D bow, arrows worn count", items: []Item{wornStrongBow, wornBoneArrows(150)}, want: []buyRow{{Item: boneArrow, Count: 850}}},
		{name: "D bow, enough arrows", items: []Item{wornStrongBow, boneArrowsOf(200)}},
		{name: "D bow, arrows of another grade", items: []Item{wornStrongBow, carriedArrows}, want: []buyRow{{Item: boneArrow, Count: 1000}}},
		{name: "zero ArrowsMax", items: []Item{wornShortBow}, edit: func(b *Brain) { b.persona.Shop.ArrowsMax = 0 }},
		{name: "no travel map", items: []Item{wornShortBow}, edit: func(b *Brain) { b.cfg.Travel = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newArcherBrain(t, 1, tt.items...)
			b.persona.Gear.Buy = false
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.shopWants(true); !slices.Equal(got, tt.want) {
				t.Errorf("shopWants = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestArrowBuyRows checks a visit's purchase of arrows: after the
// restocks, before the gear, within the adena past Shop.KeepAdena.
func TestArrowBuyRows(t *testing.T) {
	window := BuyList{Money: 2000, Items: []ShopItem{
		{TemplateID: lesserHealingPotion, Price: 100},
		{TemplateID: woodenArrow, Price: 2},
		{TemplateID: boneArrow, Price: 3},
	}}
	b := newArcherBrain(t, 1, adenaStack(2000), wornShortBow)
	b.persona.Shop.Restock = []Restock{{Item: lesserHealingPotion, Min: 1, Max: 5}}
	want := []buyRow{{Item: lesserHealingPotion, Count: 5}, {Item: woodenArrow, Count: 250}}
	if got := b.buyRows(window); !slices.Equal(got, want) {
		t.Errorf("buyRows = %v, want %v", got, want)
	}
}

// TestArrowJunk checks that the wielded bow's arrows are never junk, and
// that other arrows follow the junk rule.
func TestArrowJunk(t *testing.T) {
	tests := []struct {
		name     string
		items    []Item
		wantJunk []int32
	}{
		{name: "no bow: arrows are loot", items: []Item{wornSword, carriedArrows}, wantJunk: []int32{carriedArrows.ObjectID}},
		{name: "no-grade bow keeps its arrows", items: []Item{wornShortBow, carriedArrows}},
		{name: "D bow sells other arrows", items: []Item{wornStrongBow, carriedArrows, boneArrowsOf(50)}, wantJunk: []int32{carriedArrows.ObjectID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newArcherBrain(t, 1, tt.items...)
			got := slices.Sorted(maps.Keys(b.junk()))
			if !slices.Equal(got, tt.wantJunk) {
				t.Errorf("junk = %v, want %v", got, tt.wantJunk)
			}
		})
	}
}

// TestBrainArcherGear checks that an unarmed archer wears and buys a bow,
// a dagger user does not, and that the bot never uses arrows: the server
// arms the bow with them by itself.
func TestBrainArcherGear(t *testing.T) {
	b := newArcherBrain(t, 0, carriedBow, carriedArrows)
	if a, ok := b.ruleGear(at(time.Second)); !ok || a.object != carriedBow.ObjectID {
		t.Fatalf("unarmed archer: ruleGear = %+v %v, want the bow used", a, ok)
	}
	b.world.Self.ClassID = classTreasureHunter
	if a, ok := b.ruleGear(at(2 * time.Second)); ok {
		t.Errorf("dagger user: ruleGear = %+v, want no match", a)
	}

	b = newArcherBrain(t, 0, wornShortBow, carriedArrows)
	if a, ok := b.ruleGear(at(time.Second)); ok {
		t.Errorf("bow worn, arrows carried: ruleGear = %+v, want no UseItem", a)
	}
	if b.wearsNext(carriedArrows) {
		t.Error("wearsNext(arrows) = true")
	}

	b = newArcherBrain(t, 0, adenaStack(30000))
	want := buyRow{Item: plainBow, Count: 1}
	if got := b.gearWants(); !slices.Contains(got, want) {
		t.Errorf("unarmed archer: gearWants = %v, want %v among them", got, want)
	}
}

// TestBrainArmedBowShops drives a bow bought on a visit: the visit ends
// (no next one before shopRetry), or is still to end after its last
// ItemList, the gear errand wears the bow, and with a few of its arrows
// carried the shop errand starts a visit for more at once; with arrows
// enough carried it keeps waiting.
func TestBrainArmedBowShops(t *testing.T) {
	tests := []struct {
		name   string
		items  []Item
		active bool
		shop   bool
	}{
		{"few arrows", []Item{carriedBow, fewArrows, adenaStack(30000)}, false, true},
		{"arrows carried", []Item{carriedBow, carriedArrows, adenaStack(30000)}, false, false},
		{"visit active, few arrows", []Item{carriedBow, fewArrows, adenaStack(30000)}, true, true},
		{"visit active, arrows carried", []Item{carriedBow, carriedArrows, adenaStack(30000)}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newArcherBrain(t, 0, tt.items...)
			if tt.active {
				// The buy's ItemList dropped the last list; the visit
				// ends at the next ruleShop.
				b.shop.v = npcVisit{npcID: gearMerchant, at: Point{X: 5000}, obj: 900}
			} else {
				b.shop.end(at(0), false)
			}
			a, ok := b.ruleGear(at(time.Second))
			if !ok || a.object != carriedBow.ObjectID {
				t.Fatalf("ruleGear = %+v %v, want the bow used", a, ok)
			}
			b.gear.sent(a, at(time.Second))
			bow := b.world.Inventory.Items[carriedBow.ObjectID]
			bow.Equipped = true
			b.world.Inventory.Items[carriedBow.ObjectID] = bow
			if a, ok := b.ruleGear(at(2 * time.Second)); ok {
				t.Fatalf("bow worn: ruleGear = %+v, want no UseItem", a)
			}
			if tt.active {
				if a, ok := b.ruleShop(at(3 * time.Second)); ok || b.shop.v.active() {
					t.Fatalf("ruleShop = %+v %v, want the visit ended", a, ok)
				}
			}
			if got := b.startShop(at(3 * time.Second)); got != tt.shop {
				t.Fatalf("startShop = %v, want %v", got, tt.shop)
			}
			if !tt.shop {
				return
			}
			want := buyRow{Item: woodenArrow, Count: b.persona.Shop.ArrowsMax - fewArrows.Count}
			if got := b.shopWants(false); !slices.Contains(got, want) {
				t.Errorf("shopWants = %v, want %v among them", got, want)
			}
		})
	}
}
