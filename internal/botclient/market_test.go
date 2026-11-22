package botclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// sellerObj runs a private sell store in the market tests; greaterPotion
// is the Greater Healing Potion, which no merchant sells.
const (
	sellerObj           = int32(0x10000050)
	greaterPotion int32 = 1539
)

// newMarketBrain is a level 15 human mystic at the origin carrying items,
// whose only supply is a trip target of 10 Greater Healing Potions and 50
// Lesser Healing Potions (SupplyPolicy.Extra), with grocer 30004 (Lesser
// Healing Potion at 103) spawned at (5000, 0) on its travel map and the
// reference prices 300 and 100 of the two potions.
func newMarketBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance, p.AutoShots = 0, false
	p.Potions, p.Supply.Escapes = nil, 0
	p.Supply.Extra = func(*World) map[int32]int32 { return map[int32]int32{greaterPotion: 10, lesserHealingPotion: 50} }
	p.Gear.Buy = false
	b, _ := newTestBrain(t, p)
	b.world.Self.ClassID, b.world.Self.Level = classHumanMystic, 15
	shops, cheapest := indexShops([]datapack.BuyList{
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: lesserHealingPotion, Price: 103}}},
	})
	spawns := map[int32][]Point{30004: {{X: 5000}}}
	b.cfg.Travel = &TravelMap{
		spawns: spawns, shops: shops, cheapest: cheapest, spawnedSold: spawnedSold(shops, spawns),
		prices: map[int32]int32{greaterPotion: 300, lesserHealingPotion: 100},
	}
	b.world.Apply(ItemList{Items: items}, t0)
	return walkOn(t, b)
}

// sellStore is player id's sell store at x on the x axis.
func sellStore(id, x int32) CharInfo {
	return CharInfo{ObjectID: id, Name: "seller", Pos: Point{X: x}, StoreType: storeTypeSell}
}

// TestMarketVisit walks a bot short of Greater Healing Potions into range
// of a sell store before it clicks, opens the store's list with a second
// click, buys the rows under the price cap, cheapest first, with one
// request, and leaves the store alone afterwards.
func TestMarketVisit(t *testing.T) {
	b := newMarketBrain(t, adenaStack(10000), potionsOf(50))
	list := PrivateStoreListSell{OwnerID: sellerObj, BuyerAdena: 10000, Items: []StoreItem{
		{ObjectID: 901, TemplateID: greaterPotion, Count: 20, Price: 450, ReferencePrice: 300},
		{ObjectID: 902, TemplateID: greaterPotion, Count: 5, Price: 460, ReferencePrice: 300},
		{ObjectID: 900, TemplateID: greaterPotion, Count: 4, Price: 400, ReferencePrice: 300},
	}}
	bought := InventoryUpdate{Changes: []ItemChange{
		{ItemModified, adenaStack(10000 - 4*400 - 6*450)},
		{ItemAdded, Item{ObjectID: 950, TemplateID: greaterPotion, Count: 10, SubCategory: 5}},
	}}
	runTrainerSteps(t, b, []trainerStep{
		{name: "store in sight: leg into range first", at: time.Second, events: []Event{sellStore(sellerObj, 600)}, wantRule: "market", wantKind: actMove},
		{
			name: "in range: select", at: 10 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 520}) },
			wantRule: "market", wantKind: actSelect,
		},
		{name: "second click opens the list", at: 11 * time.Second, events: []Event{MyTargetSelected{ObjectID: sellerObj}}, wantRule: "market", wantKind: actTalk},
		{name: "wait for the list", at: 12 * time.Second, wantRule: "market"},
		{
			// 9000 adena past the reserve: the 4 at 400, then 6 of the 20
			// at 450; the row at 460 is above 1.5 x the reference price.
			name: "buy", at: 13 * time.Second, events: []Event{list},
			wantRule: "market", wantKind: actStoreBuy,
			wantPayload: encodeRequestPrivateStoreBuy(sellerObj, []storeRow{{900, 4, 400}, {901, 6, 450}}),
		},
		{name: "one request per list", at: 14 * time.Second, events: []Event{list}, wantRule: "market"},
		{name: "bought: the visit ends", at: 15 * time.Second, events: []Event{bought}},
	})
	if b.market.active() || !b.market.cooled[sellerObj].Equal(at(15*time.Second+b.persona.Market.Cooldown)) {
		t.Fatalf("visit %+v, want it over and the seller cooled for Cooldown", b.market)
	}
}

// TestMarketCooldown leaves a store alone for MarketPolicy.Cooldown after a
// visit, also one whose list never came.
func TestMarketCooldown(t *testing.T) {
	b := newMarketBrain(t, adenaStack(10000), potionsOf(50))
	b.world.Self.move = stopAt(Point{X: 520})
	cool := b.persona.Market.Cooldown
	runTrainerSteps(t, b, []trainerStep{
		{name: "select", at: time.Second, events: []Event{sellStore(sellerObj, 600)}, wantRule: "market", wantKind: actSelect},
		{name: "click", at: 2 * time.Second, events: []Event{MyTargetSelected{ObjectID: sellerObj}}, wantRule: "market", wantKind: actTalk},
		{name: "wait for the list", at: 2*time.Second + marketReplyTimeout - time.Millisecond, wantRule: "market"},
		{name: "no list: the visit ends", at: 2*time.Second + marketReplyTimeout},
		{name: "cooling down", at: 2*time.Second + marketReplyTimeout + cool - time.Second},
		{name: "cooled: visited again", at: 2*time.Second + marketReplyTimeout + cool, wantRule: "market", wantKind: actTalk},
	})
}

// TestMarketStart checks which stores a visit starts at.
func TestMarketStart(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		setup func(b *Brain)
		want  bool
	}{
		{name: "sell store", want: true},
		{name: "package store", setup: func(b *Brain) {
			info := sellStore(sellerObj, 600)
			info.StoreType = storeTypePackageSell
			b.world.Apply(info, t0)
		}},
		{name: "buy store", setup: func(b *Brain) {
			info := sellStore(sellerObj, 600)
			info.StoreType = 3
			b.world.Apply(info, t0)
		}},
		{name: "flagged owner", setup: func(b *Brain) { b.world.Apply(RelationChanged{ObjectID: sellerObj, PvPFlag: 1}, t0) }},
		{name: "karma owner", setup: func(b *Brain) { b.world.Apply(RelationChanged{ObjectID: sellerObj, Karma: 50}, t0) }},
		{name: "attackable without force, kept across CharInfo", setup: func(b *Brain) {
			b.world.Apply(RelationChanged{ObjectID: sellerObj, AutoAttackable: true}, t0)
			b.world.Apply(sellStore(sellerObj, 600), t0)
		}},
		{name: "dead owner", setup: func(b *Brain) {
			info := sellStore(sellerObj, 600)
			info.Dead = true
			b.world.Apply(info, t0)
		}},
		{name: "owner cooling down", setup: func(b *Brain) { b.market.cooled = map[int32]time.Time{sellerObj: at(time.Minute)} }},
		{name: "nothing wanted", items: []Item{adenaStack(10000), potionsOf(50), {ObjectID: 950, TemplateID: greaterPotion, Count: 3}}},
		{name: "no adena past the reserve", items: []Item{adenaStack(1000), potionsOf(50)}},
		{name: "shop visit under way", setup: func(b *Brain) { b.shop.v = npcVisit{npcID: 30004, at: Point{X: 5000}} }},
		{name: "store buying off", setup: func(b *Brain) { b.persona.Market.MaxOverReference = 0 }},
		{name: "no travel map", setup: func(b *Brain) { b.cfg.Travel = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items := tc.items
			if items == nil {
				items = []Item{adenaStack(10000), potionsOf(50)}
			}
			b := newMarketBrain(t, items...)
			b.world.Apply(sellStore(sellerObj, 600), t0)
			if tc.setup != nil {
				tc.setup(b)
			}
			if got := b.startMarket(at(time.Second)); got != tc.want {
				t.Fatalf("startMarket = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMarketRows checks the rows a store window buys: the price caps, the
// budget and package stores.
func TestMarketRows(t *testing.T) {
	ghp := func(obj, count, price, ref int32) StoreItem {
		return StoreItem{ObjectID: obj, TemplateID: greaterPotion, Count: count, Price: price, ReferencePrice: ref}
	}
	lesser := func(obj, count, price int32) StoreItem {
		return StoreItem{ObjectID: obj, TemplateID: lesserHealingPotion, Count: count, Price: price, ReferencePrice: 100}
	}
	tests := []struct {
		name   string
		potion int32 // Lesser Healing Potions carried
		adena  int32
		list   PrivateStoreListSell
		want   []storeRow
	}{
		{
			name: "at most 1.5 x the reference price", potion: 50, adena: 10000,
			list: PrivateStoreListSell{Items: []StoreItem{ghp(1, 10, 451, 300), ghp(2, 2, 450, 300)}},
			want: []storeRow{{2, 2, 450}},
		},
		{
			name: "the travel map's reference price for a row without one", potion: 50, adena: 10000,
			list: PrivateStoreListSell{Items: []StoreItem{ghp(1, 10, 460, 0), ghp(2, 3, 440, 0)}},
			want: []storeRow{{2, 3, 440}},
		},
		{
			// 1.5 x 100 would allow 150; the grocer sells at 103.
			name: "never above a spawned merchant's price", potion: 0, adena: 100000,
			list: PrivateStoreListSell{Items: []StoreItem{lesser(1, 50, 104), lesser(2, 20, 103), lesser(3, 10, 90)}},
			want: []storeRow{{3, 10, 90}, {2, 20, 103}},
		},
		{
			name: "no price known", potion: 50, adena: 10000,
			list: PrivateStoreListSell{Items: []StoreItem{{ObjectID: 1, TemplateID: 5000, Count: 1, Price: 1}}},
		},
		{
			// 2000 past the 1000 reserve: 5 at 400.
			name: "within the budget", potion: 50, adena: 3000,
			list: PrivateStoreListSell{Items: []StoreItem{ghp(1, 10, 400, 300)}},
			want: []storeRow{{1, 5, 400}},
		},
		{
			name: "nothing past the reserve", potion: 50, adena: 1399,
			list: PrivateStoreListSell{Items: []StoreItem{ghp(1, 10, 400, 300)}},
		},
		{
			// In the supplies' order (Extra by template id): 5000 past the
			// reserve buys the 40 Lesser Healing Potions wanted at 100
			// (4000), then 2 Greater ones at 400.
			name: "the budget spent in the supplies' order", potion: 10, adena: 6000,
			list: PrivateStoreListSell{Items: []StoreItem{ghp(1, 10, 400, 300), lesser(2, 50, 100)}},
			want: []storeRow{{2, 40, 100}, {1, 2, 400}},
		},
		{
			name: "a package store sells only whole", potion: 50, adena: 10000,
			list: PrivateStoreListSell{Packaged: true, Items: []StoreItem{ghp(1, 10, 300, 300)}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items := []Item{adenaStack(tc.adena)}
			if tc.potion > 0 {
				items = append(items, potionsOf(tc.potion))
			}
			b := newMarketBrain(t, items...)
			tc.list.OwnerID, tc.list.BuyerAdena = sellerObj, tc.adena
			if got := b.marketRows(tc.list); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("marketRows = %v, want %v", got, tc.want)
			}
		})
	}
}
