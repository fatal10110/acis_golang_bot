package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// merchantObj is grocer 30004 once in sight; bookSellerObj is 30094, whose
// list 29 sells spellbooks.
const (
	merchantObj   = int32(0x20000040)
	bookSellerObj = int32(0x20000041)
)

// Items of the shop tests (aCis_datapack/data/xml/items).
const (
	animalSkin   int32 = 1867
	leatherShirt int32 = 1146
	healBook     int32 = 1152
)

// merchantPage is grocer obj's first page (data/html/merchant, reduced to
// its buy and sell links).
func merchantPage(obj int32, lists ...int32) string {
	html := `<html><body>Grocer:<br>`
	for _, l := range lists {
		html += `<a action="bypass -h npc_` + itoa(obj) + `_Buy ` + itoa(l) + `">Buy</a><br>`
	}
	return html + `<a action="bypass -h npc_` + itoa(obj) + `_Sell">Sell</a></body></html>`
}

// shopRows is a window's rows as packet fields.
func shopRows(items ...ShopItem) []any {
	var f []any
	for _, it := range items {
		f = append(f, it.Category, it.ObjectID, it.TemplateID, it.Count, it.SubCategory, uint16(0), it.BodySlot, it.Enchant, uint16(0), uint16(0), it.Price)
	}
	return f
}

func shopWindow(op byte, money, listID int32, count uint16, items ...ShopItem) []byte {
	return packet(op, append([]any{money, listID, count}, shopRows(items...)...)...)
}

var (
	potionOffer = ShopItem{Category: 5, ObjectID: lesserHealingPotion, TemplateID: lesserHealingPotion, SubCategory: 5, Price: 103}
	shotOffer   = ShopItem{Category: 5, ObjectID: spiritshotNoGrade, TemplateID: spiritshotNoGrade, SubCategory: 5, Price: 17}
	bookOffer   = ShopItem{Category: 5, ObjectID: healBook, TemplateID: healBook, Count: 3, SubCategory: 5, Price: 110}
	skinRow     = ShopItem{Category: 5, ObjectID: 102, TemplateID: animalSkin, Count: 10, SubCategory: 5, Price: 5}
)

func TestDecodeShopWindows(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// 100 adena, list 14, one row: Lesser Healing Potion, unlimited, at 103.
		{
			"BuyList bytes",
			[]byte{
				0x11, 100, 0, 0, 0, 14, 0, 0, 0, 1, 0,
				5, 0, 0x24, 4, 0, 0, 0x24, 4, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 103, 0, 0, 0,
			},
			BuyList{Money: 100, ListID: 14, Items: []ShopItem{potionOffer}},
		},
		{"BuyList", shopWindow(opGameBuyList, 5000, 29, 2, bookOffer, potionOffer), BuyList{Money: 5000, ListID: 29, Items: []ShopItem{bookOffer, potionOffer}}},
		// A sold-out product of limited stock is counted, not written.
		{"BuyList sold out", shopWindow(opGameBuyList, 5000, 29, 3, potionOffer), BuyList{Money: 5000, ListID: 29, Items: []ShopItem{potionOffer}}},
		{"BuyList empty", shopWindow(opGameBuyList, 0, 14, 0), BuyList{ListID: 14, Items: []ShopItem{}}},
		{"SellList", shopWindow(opGameSellList, 300, 0, 1, skinRow), SellList{Money: 300, Items: []ShopItem{skinRow}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeShopWindowsRejects(t *testing.T) {
	buy := shopWindow(opGameBuyList, 1, 14, 1, potionOffer)
	sell := shopWindow(opGameSellList, 1, 0, 1, skinRow)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"BuyList short header", packet(opGameBuyList, 1, 14), wire.ErrShortPacket},
		{"BuyList short row", buy[:len(buy)-1], wire.ErrShortPacket},
		{"BuyList trailing", append(shopWindow(opGameBuyList, 1, 14, 0), 0), errTrailingBytes},
		{"BuyList rows past the count", shopWindow(opGameBuyList, 1, 14, 1, potionOffer, shotOffer), errTrailingBytes},
		{"SellList short row", sell[:len(sell)-4], wire.ErrShortPacket},
		{"SellList count past the frame", shopWindow(opGameSellList, 1, 0, 2, skinRow), wire.ErrShortPacket},
		{"SellList trailing", append(sell, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestShopEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{
			"RequestBuyItem", encodeRequestBuyItem(14, []buyRow{{1060, 18}, {2509, 129}}),
			[]byte{0x1f, 14, 0, 0, 0, 2, 0, 0, 0, 0x24, 4, 0, 0, 18, 0, 0, 0, 0xcd, 9, 0, 0, 129, 0, 0, 0},
		},
		{
			"RequestSellItem", encodeRequestSellItem(0, []sellRow{{102, 1867, 10}}),
			[]byte{0x1e, 0, 0, 0, 0, 1, 0, 0, 0, 102, 0, 0, 0, 0x4b, 7, 0, 0, 10, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

func TestIndexShops(t *testing.T) {
	shops, cheapest := indexShops([]datapack.BuyList{
		{ID: 15, NPCID: 30004, Products: []datapack.Product{{ItemID: 1060, Price: 120}}},
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: 1060, Price: 103}}},
		{ID: 900, NPCID: 0, Products: []datapack.Product{{ItemID: 1060, Price: 1}}},
	})
	if len(shops) != 1 || len(shops[30004]) != 2 || shops[30004][0].id != 14 || shops[30004][1].id != 15 {
		t.Fatalf("shops = %+v, want 30004's lists 14 and 15 and no game master's", shops)
	}
	if cheapest[1060] != 103 {
		t.Fatalf("cheapest 1060 = %d, want 103", cheapest[1060])
	}
}

// Carried stacks of the shop tests.
var (
	sword      = Item{ObjectID: 104, TemplateID: 2369, Count: 1, SubCategory: 0, Equipped: true, BodySlot: partRHand}
	questItem  = Item{ObjectID: 103, TemplateID: 1000, Count: 1, SubCategory: subCategoryQuest}
	skin       = Item{ObjectID: 102, TemplateID: animalSkin, Count: 10, SubCategory: 5}
	shirt      = Item{ObjectID: 105, TemplateID: leatherShirt, Count: 1, SubCategory: 1}
	adenaStack = func(n int32) Item { return Item{ObjectID: 100, TemplateID: adenaID, Count: n, SubCategory: 4} }
	potionsOf  = func(n int32) Item {
		return Item{ObjectID: 101, TemplateID: lesserHealingPotion, Count: n, SubCategory: 5}
	}
	spiritsOf = func(n int32) Item {
		return Item{ObjectID: 106, TemplateID: spiritshotNoGrade, Count: n, SubCategory: 5}
	}
)

// newShopBrain is a level 15 human mystic at the origin with the default
// persona (no idle drift, no shot toggles) carrying items, grocer 30004
// (list 14) spawned at (5000, 0) and spellbook seller 30094 (list 29) on
// its travel map.
func newShopBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance, p.AutoShots = 0, false
	b, _ := newTestBrain(t, p)
	b.world.Self.ClassID, b.world.Self.Level = classHumanMystic, 15
	shops, cheapest := indexShops([]datapack.BuyList{
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: 1835, Price: 8}, {ItemID: 2509, Price: 17}, {ItemID: 1060, Price: 103}}},
		{ID: 29, NPCID: 30094, Products: []datapack.Product{{ItemID: healBook, Price: 110}}},
	})
	spawns := map[int32][]Point{30004: {{X: 5000}}}
	b.cfg.Travel = &TravelMap{spawns: spawns, shops: shops, cheapest: cheapest, spawnedSold: spawnedSold(shops, spawns)}
	b.world.Apply(ItemList{Items: items}, t0)
	return b
}

// TestBrainShopVisit walks a bot low on potions and spiritshots to the
// grocer's spawn, sells its junk, then buys as much as its adena allows.
func TestBrainShopVisit(t *testing.T) {
	b := newShopBrain(t, adenaStack(5000), potionsOf(2), skin, questItem, sword, shirt)
	page := NpcHtmlMessage{ObjectID: merchantObj, HTML: merchantPage(merchantObj, 14)}
	soldPage := NpcHtmlMessage{ObjectID: merchantObj, HTML: `<html><body>Thank you.</body></html>`}
	runTrainerSteps(t, b, []trainerStep{
		{name: "leg to the grocer's spawn", at: time.Second, wantRule: "shop", wantKind: actMove},
		{
			name: "grocer in sight: select", at: 40 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) },
			events:   []Event{NPCInfo{ObjectID: merchantObj, NPCID: 30004, Pos: Point{X: 5000}}},
			wantRule: "shop", wantKind: actSelect,
		},
		{name: "talk", at: 41 * time.Second, events: []Event{MyTargetSelected{ObjectID: merchantObj}}, wantRule: "shop", wantKind: actTalk},
		{name: "wait for the page", at: 42 * time.Second, wantRule: "shop"},
		{name: "sell link", at: 43 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(sellCommand(merchantObj))},
		{name: "wait for the window", at: 44 * time.Second, wantRule: "shop"},
		{
			// The quest item and the equipped sword are never offered; the
			// window leaves out the shirt and lists the potions, kept.
			name: "sell the junk listed", at: 45 * time.Second,
			events:   []Event{SellList{Money: 5000, Items: []ShopItem{skinRow, {ObjectID: 101, TemplateID: lesserHealingPotion, Count: 2, Price: 50}}}},
			wantRule: "shop", wantKind: actSellItem, wantPayload: encodeRequestSellItem(0, []sellRow{{102, animalSkin, 10}}),
		},
		{name: "wait for the sale", at: 46 * time.Second, wantRule: "shop"},
		{
			name: "sold: talk again past the sold page", at: 47 * time.Second,
			events: []Event{
				InventoryUpdate{Changes: []ItemChange{{ItemModified, adenaStack(5050)}, {ItemRemoved, skin}}},
				soldPage,
			},
			wantRule: "shop", wantKind: actTalk,
		},
		{name: "buy link", at: 48 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(buyCommand(merchantObj, 14))},
		{
			// 4050 adena past the 1000 kept: 18 potions (1854), then 129
			// spiritshots of the 200 wanted. No soulshots for a mystic.
			name: "buy", at: 49 * time.Second,
			events:   []Event{BuyList{Money: 5050, ListID: 14, Items: []ShopItem{{TemplateID: 1835, Price: 8}, shotOffer, potionOffer}}},
			wantRule: "shop", wantKind: actBuyItem, wantPayload: encodeRequestBuyItem(14, []buyRow{{lesserHealingPotion, 18}, {spiritshotNoGrade, 129}}),
		},
		{name: "wait for the purchase", at: 50 * time.Second, wantRule: "shop"},
		{
			name: "bought: the visit ends", at: 51 * time.Second,
			events: []Event{ItemList{Items: []Item{adenaStack(1003), potionsOf(20), spiritsOf(129), questItem, sword, shirt}}},
		},
		{name: "not due again", at: 51*time.Second + shopRetry},
	})
	if !b.shop.unsellable[leatherShirt] || b.shop.unsellable[lesserHealingPotion] {
		t.Errorf("unsellable = %v, want the shirt only", b.shop.unsellable)
	}
	if b.shop.v.active() || !b.shop.nextAt.Equal(at(51*time.Second+shopRetry)) || b.shop.shunned[30004].After(t0) {
		t.Errorf("visit %+v, want it over, the next one shopRetry away and the grocer not shunned", b.shop)
	}
}

// TestShopDue checks when a visit starts.
func TestShopDue(t *testing.T) {
	junk := func(n int) []Item {
		var out []Item
		for i := range n {
			out = append(out, Item{ObjectID: 200 + int32(i), TemplateID: 1867 + int32(i), Count: 1, SubCategory: 5})
		}
		return out
	}
	stocked := []Item{adenaStack(5000), potionsOf(20), spiritsOf(100)}
	tests := []struct {
		name     string
		items    []Item
		setup    func(b *Brain)
		wantKind actionKind // actHold: no visit
	}{
		{name: "stocked", items: stocked},
		{name: "potions low", items: []Item{adenaStack(5000), potionsOf(4), spiritsOf(100)}, wantKind: actMove},
		{name: "potions at the minimum", items: []Item{adenaStack(5000), potionsOf(5), spiritsOf(100)}},
		{name: "spiritshots low", items: []Item{adenaStack(5000), potionsOf(20), spiritsOf(49)}, wantKind: actMove},
		{
			name: "a fighter wants soulshots", items: stocked, wantKind: actMove,
			setup: func(b *Brain) { b.world.Self.ClassID = 0 },
		},
		{name: "adena all kept", items: []Item{adenaStack(1000), potionsOf(0), spiritsOf(0)}},
		{name: "adena all kept, junk to sell", items: append([]Item{adenaStack(1000), potionsOf(0), spiritsOf(100)}, junk(1)...), wantKind: actMove},
		{name: "junk at SellAt", items: append(stocked, junk(5)...), wantKind: actMove},
		{name: "junk under SellAt", items: append(stocked, junk(4)...)},
		{
			name: "junk at SellAt, wants no merchant sells", items: append([]Item{adenaStack(5000), potionsOf(20), spiritsOf(100)}, junk(5)...), wantKind: actMove,
			setup: func(b *Brain) { b.shop.wantBook(9999) },
		},
		{name: "never sells", items: append(stocked, junk(5)...), setup: func(b *Brain) { b.persona.Shop.SellAt = 0 }},
		{name: "spawn beyond reach", items: []Item{adenaStack(5000)}, setup: func(b *Brain) { b.persona.Shop.Reach = 4999 }},
		{name: "no travel map", items: []Item{adenaStack(5000)}, setup: func(b *Brain) { b.cfg.Travel = nil }},
		{
			name: "a merchant in sight beyond reach", items: []Item{adenaStack(5000)}, wantKind: actSelect,
			setup: func(b *Brain) {
				b.persona.Shop.Reach = 0
				b.world.Apply(NPCInfo{ObjectID: merchantObj, NPCID: 30004, Pos: Point{X: 200}}, t0)
			},
		},
		{
			name: "another NPC in sight", items: []Item{adenaStack(5000)},
			setup: func(b *Brain) {
				b.persona.Shop.Reach = 0
				b.world.Apply(NPCInfo{ObjectID: merchantObj, NPCID: 30005, Pos: Point{X: 200}}, t0)
			},
		},
		{
			name: "grocer shunned", items: []Item{adenaStack(5000)},
			setup: func(b *Brain) { b.shop.shunned = map[int32]time.Time{30004: at(time.Hour)} },
		},
		{name: "in combat", items: []Item{adenaStack(5000)}, setup: func(b *Brain) { b.world.Self.InCombat = true }},
		{name: "cooling down", items: []Item{adenaStack(5000)}, setup: func(b *Brain) { b.shop.nextAt = at(2 * time.Second) }},
		{
			name: "a wanted book only the seller out of reach sells", items: stocked,
			setup: func(b *Brain) { b.shop.wantBook(healBook) },
		},
		{
			name: "books not bought", items: stocked,
			setup: func(b *Brain) {
				b.persona.Shop.Books = false
				b.shop.wantBook(healBook)
				b.cfg.Travel.spawns[30094] = []Point{{Y: 100}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The sword its spiritshots charge.
			b := newShopBrain(t, slices.Concat(tt.items, []Item{sword})...)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(at(time.Second))
			if a.kind != tt.wantKind || (tt.wantKind != actHold && rule != "shop") {
				t.Fatalf("decide = %q %d, want %d", rule, a.kind, tt.wantKind)
			}
		})
	}
}

// TestShopGivesUp checks a part of a visit the server leaves unanswered or
// the merchant does not offer is not tried again before shopRetry.
func TestShopGivesUp(t *testing.T) {
	inSight := func(b *Brain) {
		b.world.Apply(NPCInfo{ObjectID: merchantObj, NPCID: 30004, Pos: Point{X: 200}}, t0)
		b.world.Target = merchantObj
	}
	low := []Item{adenaStack(5000), potionsOf(0), spiritsOf(100)}
	junk := []Item{
		adenaStack(5000), potionsOf(20), spiritsOf(100),
		{ObjectID: 200, TemplateID: 1867, Count: 1},
		{ObjectID: 201, TemplateID: 1868, Count: 1},
		{ObjectID: 202, TemplateID: 1869, Count: 1},
		{ObjectID: 203, TemplateID: 1870, Count: 1},
		{ObjectID: 204, TemplateID: 1871, Count: 1},
	}
	page := NpcHtmlMessage{ObjectID: merchantObj, HTML: merchantPage(merchantObj, 14)}
	window := SellList{Money: 5000}
	var sale []sellRow
	for _, it := range junk[3:] {
		window.Items = append(window.Items, ShopItem{ObjectID: it.ObjectID, TemplateID: it.TemplateID, Count: it.Count, Price: 1})
		sale = append(sale, sellRow{it.ObjectID, it.TemplateID, it.Count})
	}
	// The server refuses a sale in silence (network/sell.go
	// requestSellItem): neither the window nor the sale is answered.
	tests := []struct {
		name      string
		items     []Item
		steps     []trainerStep
		wantShun  bool
		wantUnsel bool
	}{
		{
			name: "sell window never opens", items: junk,
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: inSight, wantRule: "shop", wantKind: actTalk},
				{name: "sell link", at: 2 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(sellCommand(merchantObj))},
				{name: "waiting", at: 2*time.Second + shopReplyTimeout - 1, wantRule: "shop"},
				{name: "no buy list: given up", at: 2*time.Second + shopReplyTimeout},
			},
		},
		{
			name: "sale unanswered", items: junk,
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: inSight, wantRule: "shop", wantKind: actTalk},
				{name: "sell link", at: 2 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass},
				{name: "sell", at: 3 * time.Second, events: []Event{window}, wantRule: "shop", wantKind: actSellItem, wantPayload: encodeRequestSellItem(0, sale)},
				{name: "waiting", at: 3*time.Second + shopReplyTimeout - 1, wantRule: "shop"},
				{name: "no buy list: given up", at: 3*time.Second + shopReplyTimeout},
			},
		},
		{
			name: "sale unanswered, then the buy list", items: append(slices.Clone(low), junk[3:]...),
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: inSight, wantRule: "shop", wantKind: actTalk},
				{name: "sell link", at: 2 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass},
				{name: "sell", at: 3 * time.Second, events: []Event{window}, wantRule: "shop", wantKind: actSellItem},
				{name: "waiting", at: 3*time.Second + shopReplyTimeout - 1, wantRule: "shop"},
				{name: "talk again for the buy list", at: 3*time.Second + shopReplyTimeout, wantRule: "shop", wantKind: actTalk},
				{name: "buy link", at: 4*time.Second + shopReplyTimeout, events: []Event{page}, wantRule: "shop", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(buyCommand(merchantObj, 14))},
				{name: "given up", at: 4*time.Second + 2*shopReplyTimeout},
			},
		},
		{
			name: "purchase unanswered", items: low,
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: inSight, wantRule: "shop", wantKind: actTalk},
				{name: "buy link", at: 2 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass},
				{name: "buy", at: 3 * time.Second, events: []Event{BuyList{Money: 5000, ListID: 14, Items: []ShopItem{potionOffer}}}, wantRule: "shop", wantKind: actBuyItem},
				{name: "waiting", at: 3*time.Second + shopReplyTimeout - 1, wantRule: "shop"},
				{name: "refused: given up", at: 3*time.Second + shopReplyTimeout},
			},
		},
		{
			name: "buy window never opens", items: low,
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: inSight, wantRule: "shop", wantKind: actTalk},
				{name: "buy link", at: 2 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass},
				{name: "a window of another list changes nothing", at: 3 * time.Second, events: []Event{BuyList{ListID: 15}}, wantRule: "shop"},
				{name: "given up", at: 2*time.Second + shopReplyTimeout},
			},
		},
		{
			name: "nothing sellable", items: junk, wantUnsel: true,
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: inSight, wantRule: "shop", wantKind: actTalk},
				{name: "sell link", at: 2 * time.Second, events: []Event{page}, wantRule: "shop", wantKind: actBypass},
				{name: "the -empty page ends it", at: 3 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: merchantObj, HTML: "<html><body>Nothing to buy.</body></html>"}}},
			},
		},
		{
			name: "page without the links", items: low, wantShun: true,
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: inSight, wantRule: "shop", wantKind: actTalk},
				{name: "no buy link", at: 2 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: merchantObj, HTML: merchantPage(merchantObj, 15)}}},
			},
		},
		{
			name: "grocer not at its spawn", items: low, wantShun: true,
			steps: []trainerStep{
				{name: "nobody there", at: time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) }},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newShopBrain(t, slices.Concat(tt.items, []Item{sword})...)
			runTrainerSteps(t, b, tt.steps)
			end := tt.steps[len(tt.steps)-1].at
			if b.shop.v.active() || !b.shop.nextAt.Equal(at(end+shopRetry)) {
				t.Fatalf("visit %+v, want it over until shopRetry", b.shop)
			}
			if shun := b.shop.shunned[30004].After(t0); shun != tt.wantShun {
				t.Errorf("grocer shunned = %v, want %v", shun, tt.wantShun)
			}
			if unsel := b.shop.unsellable[1867]; unsel != tt.wantUnsel {
				t.Errorf("junk unsellable = %v, want %v", unsel, tt.wantUnsel)
			}
			// No request goes out again before shopRetry.
			for d := end + time.Second; d < end+shopRetry; d += time.Minute {
				if rule, a := b.decide(at(d)); a.kind != actHold {
					t.Fatalf("decide at %v = %q %d, want nothing", d, rule, a.kind)
				}
			}
		})
	}
}

// TestTrainerBuysSpellbook checks a skill whose spellbook the bot lacks is
// skipped, the book bought at a merchant selling it, and the trainer
// visited again.
func TestTrainerBuysSpellbook(t *testing.T) {
	b := newTrainerBrain(t, 200)
	b.persona.AutoShots = false
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 29, NPCID: 30094, Products: []datapack.Product{{ItemID: healBook, Price: 110}}}})
	b.cfg.Travel = &TravelMap{shops: shops, cheapest: cheapest}
	b.world.Apply(ItemList{Items: []Item{adenaStack(2000), potionsOf(20), spiritsOf(100), sword}}, t0)
	b.world.Apply(NPCInfo{ObjectID: bookSellerObj, NPCID: 30094, Pos: Point{X: -300}}, t0)
	heal1 := AcquirableSkill{skillHeal, 1, 160}
	book := AcquireSkillInfo{ID: skillHeal, Level: 1, Cost: 160, Requirements: []SkillRequirement{{Type: spellbookRequirement, ItemID: healBook, Count: 1}}}
	runTrainerSteps(t, b, []trainerStep{
		{name: "select", wantRule: "train", wantKind: actSelect},
		{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: trainerObj}}, wantRule: "train", wantKind: actTalk},
		{name: "skill list", at: 2 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}}, wantRule: "train", wantKind: actBypass},
		{name: "cost", at: 3 * time.Second, events: []Event{AcquireSkillList{Skills: []AcquirableSkill{heal1}}}, wantRule: "train", wantKind: actSkillInfo},
		{name: "book missing: no learn, to the seller", at: 4 * time.Second, events: []Event{book}, wantRule: "shop", wantKind: actSelect},
		{name: "talk", at: 5 * time.Second, events: []Event{MyTargetSelected{ObjectID: bookSellerObj}}, wantRule: "shop", wantKind: actTalk},
		{name: "buy link", at: 6 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: bookSellerObj, HTML: merchantPage(bookSellerObj, 29)}}, wantRule: "shop", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(buyCommand(bookSellerObj, 29))},
		{name: "buy one", at: 7 * time.Second, events: []Event{BuyList{Money: 2000, ListID: 29, Items: []ShopItem{bookOffer}}}, wantRule: "shop", wantKind: actBuyItem, wantPayload: encodeRequestBuyItem(29, []buyRow{{healBook, 1}})},
		{
			name: "bought: back to the trainer", at: 8 * time.Second,
			events:   []Event{ItemList{Items: []Item{adenaStack(1890), potionsOf(20), spiritsOf(100), {ObjectID: 300, TemplateID: healBook, Count: 1}}}},
			wantRule: "train", wantKind: actSelect, wantPayload: encodeAction(trainerObj, Point{}),
		},
	})
	if len(b.shop.books) != 0 {
		t.Errorf("books = %v, want none wanted", b.shop.books)
	}
}

// TestTrainerLearnsWithBook checks a skill whose spellbook the bot carries
// is learned.
func TestTrainerLearnsWithBook(t *testing.T) {
	b := newTrainerBrain(t, 200)
	b.world.Apply(ItemList{Items: []Item{{ObjectID: 300, TemplateID: healBook, Count: 1}}}, t0)
	b.train.npc, b.train.step, b.train.pending = trainerObj, trainPick, AcquirableSkill{skillHeal, 1, 160}
	ev := AcquireSkillInfo{ID: skillHeal, Level: 1, Cost: 160, Requirements: []SkillRequirement{{Type: spellbookRequirement, ItemID: healBook, Count: 1}}}
	if err := b.handle([]Event{ev}, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	if rule, a := b.decide(at(time.Second)); rule != "train" || a.kind != actLearnSkill {
		t.Fatalf("decide = %q %d, want a learn", rule, a.kind)
	}
}

// TestShopKeepsBoughtBook checks a bought spellbook is not junk while the
// bot carries it with no trainer in sight, and is held no more once a learn
// uses it.
func TestShopKeepsBoughtBook(t *testing.T) {
	book := Item{ObjectID: 300, TemplateID: healBook, Count: 1}
	var junk []Item
	window := SellList{Money: 5000}
	var sale []sellRow
	for i := range 5 {
		it := Item{ObjectID: 200 + int32(i), TemplateID: 1867 + int32(i), Count: 1, SubCategory: 5}
		junk = append(junk, it)
		window.Items = append(window.Items, ShopItem{ObjectID: it.ObjectID, TemplateID: it.TemplateID, Count: 1, Price: 1})
		sale = append(sale, sellRow{it.ObjectID, it.TemplateID, 1})
	}
	window.Items = append(window.Items, ShopItem{ObjectID: book.ObjectID, TemplateID: healBook, Count: 1, Price: 55})
	carrying := append([]Item{adenaStack(5000), potionsOf(20), spiritsOf(100), book}, junk...)
	b := newShopBrain(t)
	b.shop.wantBook(healBook)
	runTrainerSteps(t, b, []trainerStep{
		{name: "book bought", events: []Event{ItemList{Items: carrying}}, wantRule: "shop", wantKind: actMove},
		{
			name: "talk", at: time.Second,
			setup: func(b *Brain) {
				b.world.Apply(NPCInfo{ObjectID: merchantObj, NPCID: 30004, Pos: Point{X: 200}}, t0)
				b.world.Target = merchantObj
			},
			wantRule: "shop", wantKind: actTalk,
		},
		{name: "sell link", at: 2 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: merchantObj, HTML: merchantPage(merchantObj, 14)}}, wantRule: "shop", wantKind: actBypass},
		{name: "the junk sold, the book kept", at: 3 * time.Second, events: []Event{window}, wantRule: "shop", wantKind: actSellItem, wantPayload: encodeRequestSellItem(0, sale)},
	})
	if !b.shop.held[healBook] || len(b.shop.books) != 0 {
		t.Fatalf("held %v, books %v: want the book held, none wanted", b.shop.held, b.shop.books)
	}
	if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{ItemRemoved, book}}}}, at(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if b.shop.held[healBook] {
		t.Errorf("held %v after the learn used the book, want it empty", b.shop.held)
	}
}
