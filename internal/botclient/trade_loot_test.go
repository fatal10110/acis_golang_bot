package botclient

import (
	"bytes"
	"testing"
	"time"
)

// lootPrices are the reference prices of the store test templates.
var lootPrices = map[int32]int32{animalSkin: 10, woodenArrow: 2, stoneSword: 1000, thread: 1}

// newLootTrader is a bot that is no merchant (no store budget) carrying
// items, with the player pcID standing 100 units away and the store test
// templates' reference prices on its travel map.
func newLootTrader(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newStoreBrain(t, 0, items...)
	b.cfg.Travel.prices = lootPrices
	near := Point{Y: 100}
	b.world.Objects[pcID].move = motion{from: near, to: near}
	return b
}

// TestTradeLootPrice checks that a bot that never opened a store prices
// its loot for trade at the store's price, 1.4 times the NPC buy-back,
// and leaves unpriced what NPC shops sell, what the bot keeps, and what
// has no reference price or would sell at nothing.
func TestTradeLootPrice(t *testing.T) {
	tests := []struct {
		name  string
		id    int32
		edit  func(b *Brain)
		want  int32
		price bool
	}{
		{name: "loot", id: animalSkin, want: 7, price: true},
		{name: "gear loot", id: stoneSword, want: 700, price: true},
		{name: "an NPC shop sells it", id: woodenArrow},
		{name: "worth nothing to an NPC", id: thread},
		{name: "no reference price", id: 9999},
		{name: "kept", id: animalSkin, edit: func(b *Brain) { b.persona.Shop.Keep = []int32{animalSkin} }},
		{name: "no travel map", id: animalSkin, edit: func(b *Brain) { b.cfg.Travel = nil }},
		{name: "policy price first", id: woodenArrow, edit: func(b *Brain) { b.persona.Trade.Prices = map[int32]int32{woodenArrow: 3} }, want: 3, price: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLootTrader(t)
			if tt.edit != nil {
				tt.edit(b)
			}
			if p, ok := b.tradePrice(tt.id); p != tt.want || ok != tt.price {
				t.Fatalf("tradePrice(%d) = %d, %v, want %d, %v", tt.id, p, ok, tt.want, tt.price)
			}
		})
	}
}

// TestTradeLootMatchesStore checks that the trade price of loot and the
// store's price of it from a later manage list agree under one session
// markup drawn from the persona's range.
func TestTradeLootMatchesStore(t *testing.T) {
	for seed := range 20 {
		b := newLootTrader(t, swordLoot)
		b.persona.Store.MinMarkup, b.persona.Store.MaxMarkup = 120, 150
		for range seed {
			b.rng.IntN(2)
		}
		traded, ok := b.tradePrice(stoneSword)
		if !ok {
			t.Fatal("tradePrice(sword) unpriced")
		}
		b.store.step = storeAskManage
		b.storeManageList(PrivateStoreManageListSell{OwnerID: selfID, Candidates: []StoreItem{swordCand}})
		if stored := b.store.prices[stoneSword]; stored != traded || traded < 600 || traded > 750 {
			t.Fatalf("seed %d: trade price %d, store price %d, want equal, 1.2-1.5 times the buy-back of 500", seed, traded, stored)
		}
	}
}

// TestTradeLootGoods checks which inventory item a trade offers when
// nothing has a policy price: loot the store would sell, never an
// equipped, quest, augmented or NPC shop item.
func TestTradeLootGoods(t *testing.T) {
	skinRow := TradeItem{ObjectID: skinLoot.ObjectID, TemplateID: animalSkin, Count: 10, SubCategory: 5}
	swordRow := TradeItem{ObjectID: swordLoot.ObjectID, TemplateID: stoneSword, Count: 1}
	arrowsRow := TradeItem{ObjectID: arrowsLoot.ObjectID, TemplateID: woodenArrow, Count: 50, SubCategory: 5}
	equipped, augmented := swordLoot, swordLoot
	equipped.Equipped, augmented.Augmentation = true, 1
	questSkin := skinLoot
	questSkin.SubCategory = subCategoryQuest
	tests := []struct {
		name  string
		items []Item
		rows  []TradeItem
		want  int32
	}{
		{name: "loot", items: []Item{arrowsLoot, skinLoot}, rows: []TradeItem{arrowsRow, skinRow}, want: skinLoot.ObjectID},
		{name: "NPC shop item only", items: []Item{arrowsLoot}, rows: []TradeItem{arrowsRow}},
		{name: "equipped", items: []Item{equipped}, rows: []TradeItem{swordRow}},
		{name: "augmented", items: []Item{augmented}, rows: []TradeItem{swordRow}},
		{name: "quest item", items: []Item{questSkin}, rows: []TradeItem{skinRow}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLootTrader(t, tt.items...)
			row, ok := b.tradeGoods(tt.rows)
			if ok != (tt.want != 0) || row.ObjectID != tt.want {
				t.Fatalf("tradeGoods = %+v, %v, want object %d", row, ok, tt.want)
			}
		})
	}
}

// TestBrainTradeLoot sells a non-merchant's loot to a player by trade at
// the store price: the request is accepted, the skins offered, and the
// trade confirmed once the partner's adena covers 10 at 7.
func TestBrainTradeLoot(t *testing.T) {
	b := newLootTrader(t, adenaStack(10), skinLoot, arrowsLoot)
	b.persona.Trade = defaultTradePolicy()
	out := b.out.(*recordSender)
	skinRow := TradeItem{ObjectID: skinLoot.ObjectID, TemplateID: animalSkin, Count: 10, SubCategory: 5}
	arrowsRow := TradeItem{ObjectID: arrowsLoot.ObjectID, TemplateID: woodenArrow, Count: 50, SubCategory: 5}
	steps := []struct {
		name string
		evs  []Event
		want []byte
	}{
		{"request", []Event{SendTradeRequest{SenderID: pcID}}, encodeAnswerTradeRequest(true)},
		{"window opens", []Event{TradeStart{PartnerID: pcID, Items: []TradeItem{arrowsRow, skinRow}}}, encodeAddTradeItem(skinLoot.ObjectID, 10)},
		{"partner pays short", []Event{TradeOwnAdd{Item: skinRow}, TradeOtherAdd{Item: adenaRow(69)}}, nil},
		{"partner pays", []Event{TradeOtherAdd{Item: adenaRow(1)}}, encodeTradeDone(true)},
	}
	now := t0
	for _, s := range steps {
		now = now.Add(5 * time.Second)
		if err := b.handle(s.evs, now); err != nil {
			t.Fatal(err)
		}
		sent := len(out.frames)
		for _, w := range []time.Time{b.decisionAt(), b.decisionAt().Add(time.Second)} {
			if err := b.wake(w); err != nil {
				t.Fatal(err)
			}
			now = w
		}
		var got []byte
		for _, f := range out.frames[sent:] {
			if f[0] == opGameAnswerTradeRequest || f[0] == opGameAddTradeItem || f[0] == opGameTradeDone {
				if got != nil {
					t.Fatalf("%s: sent % x, then % x", s.name, got, f)
				}
				got = f
			}
		}
		if !bytes.Equal(got, s.want) {
			t.Fatalf("%s: sent % x, want % x", s.name, got, s.want)
		}
	}
}
