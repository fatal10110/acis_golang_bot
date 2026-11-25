package botclient

import (
	"testing"
	"time"
)

// multiBrain is a merchant carrying the sword (101) and the staff (102) in
// an open round, pcID having agreed a price for both: the sword at 150000,
// the staff at 300000. It returns the entries by score, the order the bot
// offers them in, with their rows and prices.
func multiBrain(t *testing.T) (b *Brain, keys []valuableKey, rows map[valuableKey]TradeItem, price map[valuableKey]int64) {
	t.Helper()
	sword := swordUnit(101, 0)
	staff := Item{ObjectID: 102, TemplateID: ledgerStaff, Count: 1, BodySlot: partRHand}
	b = newAdBrain(t, NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour), 0, sword, staff)
	b.wh.private.Known = true
	openAdRound(b, t0)
	b.syncValuables()
	keys = b.adEntries()
	if len(keys) != 2 {
		t.Fatalf("entries by score %v, want the sword and the staff", keys)
	}
	rows = map[valuableKey]TradeItem{swordKey: rowOf(sword), staffKey: rowOf(staff)}
	price = map[valuableKey]int64{swordKey: 150000, staffKey: 300000}
	b.cfg.valuables.talk.deals = map[whisperDeal]whisperPrice{
		{pcID, swordKey}: {current: price[swordKey], agreed: true},
		{pcID, staffKey}: {current: price[staffKey], agreed: true},
	}
	return b, keys, rows, price
}

// TestValuablesTradeMulti checks a trade with a partner who agreed prices
// for two entries (docs/valuables/trades.md "Trades", #403): both offered in
// one window, an add a turn, each after the server took the one before,
// confirmed only on adena covering the sum, the sum counted in the
// valuables budget; the budget left caps the set.
func TestValuablesTradeMulti(t *testing.T) {
	// offerBoth opens the window and takes both adds, the second after
	// the first's TradeOwnAdd, with paid adena from the partner.
	offerBoth := func(t *testing.T, b *Brain, keys []valuableKey, rows map[valuableKey]TradeItem, paid int64) {
		t.Helper()
		first, second := rows[keys[0]], rows[keys[1]]
		tradeOpen(t, b, t0, rows[staffKey], rows[swordKey])
		rule, a := tradeTurn(t, b, at(time.Second))
		wantTurn(t, "first add", rule, a, "prompt", actTradeAdd, encodeAddTradeItem(first.ObjectID, 1))
		rule, a = tradeTurn(t, b, at(1500*time.Millisecond))
		wantTurn(t, "first add unanswered", rule, a, "prompt", actHold, nil)
		rule, a = tradeTurn(t, b, at(2*time.Second), TradeOwnAdd{Item: first}, TradeOtherAdd{Item: adenaPaid(int32(paid))})
		wantTurn(t, "second add", rule, a, "prompt", actTradeAdd, encodeAddTradeItem(second.ObjectID, 1))
		if err := b.handle([]Event{TradeOwnAdd{Item: second}}, at(3*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("short of the sum: hold, cancel at Timeout", func(t *testing.T) {
		b, keys, rows, price := multiBrain(t)
		offerBoth(t, b, keys, rows, price[swordKey]+price[staffKey]-1)
		rule, a := tradeTurn(t, b, at(4*time.Second))
		wantTurn(t, "short", rule, a, "prompt", actHold, nil)
		rule, a = tradeTurn(t, b, at(time.Minute))
		wantTurn(t, "Timeout", rule, a, "prompt", actTradeCancel, nil)
	})

	t.Run("confirm while short of the sum: cancel at once", func(t *testing.T) {
		b, keys, rows, price := multiBrain(t)
		offerBoth(t, b, keys, rows, price[swordKey]+price[staffKey]-1)
		rule, a := tradeTurn(t, b, at(4*time.Second), TradePressOtherOk{})
		wantTurn(t, "cancel", rule, a, "prompt", actTradeCancel, nil)
	})

	t.Run("partner confirms before the second add: cancel", func(t *testing.T) {
		b, keys, rows, price := multiBrain(t)
		tradeOpen(t, b, t0, rows[staffKey], rows[swordKey])
		tradeTurn(t, b, at(time.Second))
		paid := adenaPaid(int32(price[swordKey] + price[staffKey]))
		rule, a := tradeTurn(t, b, at(2*time.Second), TradeOwnAdd{Item: rows[keys[0]]}, TradeOtherAdd{Item: paid}, TradePressOtherOk{})
		wantTurn(t, "cancel", rule, a, "prompt", actTradeCancel, nil)
	})

	t.Run("paid the sum: confirm, both counted, both entries gone", func(t *testing.T) {
		b, keys, rows, price := multiBrain(t)
		sum := price[swordKey] + price[staffKey]
		offerBoth(t, b, keys, rows, sum)
		rule, a := tradeTurn(t, b, at(4*time.Second))
		wantTurn(t, "confirm", rule, a, "prompt", actTradeConfirm, encodeTradeDone(true))
		settle := []Event{
			TradePressOwnOk{},
			TradePressOtherOk{},
			InventoryUpdate{Changes: []ItemChange{
				{State: ItemRemoved, Item: b.world.Inventory.Items[101]},
				{State: ItemRemoved, Item: b.world.Inventory.Items[102]},
			}},
			SendTradeDone{Success: true},
		}
		if err := b.handle(settle, at(5*time.Second)); err != nil {
			t.Fatal(err)
		}
		if b.valuablesSold != sum {
			t.Fatalf("valuables sold %d, want the sum %d", b.valuablesSold, sum)
		}
		b.syncValuables()
		for _, k := range keys {
			if _, ok := b.cfg.valuables.entries[k]; ok {
				t.Fatalf("entry %v still there after the sale", k)
			}
		}
	})

	t.Run("budget covers the first only: it alone, its price due", func(t *testing.T) {
		b, keys, rows, price := multiBrain(t)
		b.valuablesSold = b.persona.Valuables.Budget - price[keys[0]]
		tradeOpen(t, b, t0, rows[staffKey], rows[swordKey])
		rule, a := tradeTurn(t, b, at(time.Second))
		wantTurn(t, "add", rule, a, "prompt", actTradeAdd, encodeAddTradeItem(rows[keys[0]].ObjectID, 1))
		rule, a = tradeTurn(t, b, at(2*time.Second), TradeOwnAdd{Item: rows[keys[0]]}, TradeOtherAdd{Item: adenaPaid(int32(price[keys[0]]))})
		wantTurn(t, "confirm", rule, a, "prompt", actTradeConfirm, encodeTradeDone(true))
		if got := b.world.Trade.sale; got != price[keys[0]] {
			t.Fatalf("sale %d, want %d", got, price[keys[0]])
		}
	})

	t.Run("goods: agreed set, capped by the budget left", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			left   func(price map[valuableKey]int64, keys []valuableKey) int64
			agreed int // how many of keys are agreed; the rest named
			want   int
		}{
			{name: "both agreed", left: func(map[valuableKey]int64, []valuableKey) int64 { return 0 }, agreed: 2, want: 2},
			{name: "one agreed, one named", left: func(map[valuableKey]int64, []valuableKey) int64 { return 0 }, agreed: 1, want: 1},
			{name: "left is the first's price", left: func(p map[valuableKey]int64, k []valuableKey) int64 { return p[k[0]] }, agreed: 2, want: 1},
			{name: "left is one over the first's price", left: func(p map[valuableKey]int64, k []valuableKey) int64 { return p[k[0]] + 1 }, agreed: 2, want: 2},
			{name: "left under the first's price", left: func(map[valuableKey]int64, []valuableKey) int64 { return 1 }, agreed: 2, want: 1},
		} {
			b, keys, rows, price := multiBrain(t)
			if l := tt.left(price, keys); l > 0 {
				b.valuablesSold = b.persona.Valuables.Budget - l
			}
			for _, k := range keys[tt.agreed:] {
				b.cfg.valuables.talk.deals[whisperDeal{pcID, k}] = whisperPrice{current: price[k]}
			}
			goods := b.valuableGoods(pcID, []TradeItem{rows[staffKey], rows[swordKey]})
			if len(goods) != tt.want {
				t.Fatalf("%s: goods %+v, want %d", tt.name, goods, tt.want)
			}
			for i, g := range goods {
				if g.key != keys[i] || g.row != rows[keys[i]] {
					t.Fatalf("%s: goods[%d] %+v, want %v %+v", tt.name, i, g, keys[i], rows[keys[i]])
				}
			}
		}
	})
}
