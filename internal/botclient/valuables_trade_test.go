package botclient

import (
	"bytes"
	"testing"
	"time"
)

// tradeTurn decides at now after handling evs, sends what it decided and
// returns the rule and action.
func tradeTurn(t *testing.T, b *Brain, now time.Time, evs ...Event) (string, Action) {
	t.Helper()
	if err := b.handle(evs, now); err != nil {
		t.Fatal(err)
	}
	rule, a := b.decide(now)
	if a.kind != actHold {
		if sent, err := b.send(a, now); err != nil || !sent {
			t.Fatalf("at %v: send %d = %v, %v", now.Sub(t0), a.kind, sent, err)
		}
	}
	return rule, a
}

// wantTurn checks a decision's rule, kind and, when set, exact payload.
func wantTurn(t *testing.T, name, rule string, a Action, wantRule string, wantKind actionKind, wantPayload []byte) {
	t.Helper()
	if rule != wantRule || a.kind != wantKind {
		t.Fatalf("%s: decide = rule %q action %d, want rule %q action %d", name, rule, a.kind, wantRule, wantKind)
	}
	if wantPayload != nil && !bytes.Equal(a.payload, wantPayload) {
		t.Fatalf("%s: payload % x, want % x", name, a.payload, wantPayload)
	}
}

// wantWalk checks a decision is rule "valuables"' walk ending within
// arrive of to.
func wantWalk(t *testing.T, name string, b *Brain, rule string, a Action, to Point, arrive int32) {
	t.Helper()
	wantTurn(t, name, rule, a, "valuables", actMove, nil)
	if !a.walk || b.walk.arrive != arrive || horizontal2(b.walk.dest, to) > int64(arrive)*int64(arrive) {
		t.Fatalf("%s: walk %v to %+v within %d, want a walk within %d of %+v", name, a.walk, b.walk.dest, b.walk.arrive, arrive, to)
	}
}

// placePlayer puts player id in sight at p.
func placePlayer(t *testing.T, b *Brain, id int32, p Point) {
	t.Helper()
	b.world.Apply(seen(id, p), t0)
	if o := b.world.Objects[id]; o == nil || o.Kind != KindPlayer {
		t.Fatalf("player %d not in sight", id)
	}
}

// seen is player id's CharInfo at p.
func seen(id int32, p Point) Event {
	return CharInfo{ObjectID: id, Name: "buyer", Pos: p, RunSpeed: 100, Running: true}
}

// teleportSelf lands the bot at p, the players around seen again (a
// teleport clears the known list).
func teleportSelf(p Point, around ...Event) []Event {
	return append([]Event{TeleportToLocation{ObjectID: selfID, Pos: p}}, around...)
}

// inviteBrain is a merchant in an open round anchored at the origin, where
// it stands, carrying the sword, that said its first line at t0, with pcID
// whispered this round standing 600 east.
func inviteBrain(t *testing.T, edit func(*Brain)) *Brain {
	t.Helper()
	b := newAdBrain(t, NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour), 0, swordUnit(101, 0))
	if edit != nil {
		edit(b)
	}
	openAdRound(b, t0)
	if decideAd(t, "first line", b, t0) == nil {
		t.Fatal("first line: none said")
	}
	b.cfg.valuables.talk.to = map[int32]int{pcID: 1}
	placePlayer(t, b, pcID, Point{X: 600})
	return b
}

// inviteSent walks b to pcID and has it send the TradeRequest at 1-2 s,
// returning the time it went out.
func inviteSent(t *testing.T, b *Brain) time.Time {
	t.Helper()
	rule, a := tradeTurn(t, b, at(time.Second))
	wantWalk(t, "legs to the whisperer", b, rule, a, Point{X: 600}, inviteArrive)
	now := at(2 * time.Second)
	rule, a = tradeTurn(t, b, now, teleportSelf(Point{X: 550}, seen(pcID, Point{X: 600}))...)
	wantTurn(t, "request", rule, a, "valuables", actTradeRequest, encodeTradeRequest(pcID))
	return now
}

// TestValuablesTradeInvite checks the round's trade request
// (docs/valuables/trades.md "Asking for a trade", D32): legs to a whisperer
// in the area, one TradeRequest, none more to it that round, and the walk
// back to the anchor before the next line or request.
func TestValuablesTradeInvite(t *testing.T) {
	for _, tt := range []struct {
		name string
		// answer ends the request at sent plus after; nil is silence.
		answer Event
		after  time.Duration
	}{
		{"declined: 119", SystemMessage{ID: msgTradeDeclined}, time.Second},
		{"store setting up: 340", SystemMessage{ID: msgStoreUnderWay}, time.Second},
		{"store mode: 1065", SystemMessage{ID: msgTradeInStoreMode}, time.Second},
		{"blocks everyone: 940", SystemMessage{ID: msgBlockedEverything, Text: [2]string{"buyer"}}, time.Second},
		{"busy: 153", SystemMessage{ID: msgS1IsBusyTryLater, Text: [2]string{"buyer"}}, time.Second},
		{"silence: 15 s", nil, tradeRequestTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := inviteBrain(t, nil)
			sent := inviteSent(t, b)
			// A second whisperer in the area while the request is open.
			b.cfg.valuables.talk.to[pcID+1] = 1
			placePlayer(t, b, pcID+1, Point{X: 500})
			rule, a := tradeTurn(t, b, sent.Add(tt.after-time.Millisecond))
			wantTurn(t, "request open", rule, a, "valuables", actHold, nil)
			var evs []Event
			if tt.answer != nil {
				evs = append(evs, tt.answer)
			}
			now := sent.Add(tt.after)
			rule, a = tradeTurn(t, b, now, evs...)
			wantWalk(t, "walk back", b, rule, a, Point{}, anchorArrive)
			// Back at the anchor the second whisperer is asked, never pcID again.
			around := []Event{seen(pcID, Point{X: 600}), seen(pcID+1, Point{X: 500})}
			rule, a = tradeTurn(t, b, now.Add(time.Second), teleportSelf(Point{X: 50}, around...)...)
			wantWalk(t, "legs to the second", b, rule, a, Point{X: 500}, inviteArrive)
			rule, a = tradeTurn(t, b, now.Add(2*time.Second), teleportSelf(Point{X: 480}, around...)...)
			wantTurn(t, "second request", rule, a, "valuables", actTradeRequest, encodeTradeRequest(pcID+1))
			rule, a = tradeTurn(t, b, now.Add(3*time.Second), SystemMessage{ID: msgTradeDeclined})
			wantWalk(t, "walk back again", b, rule, a, Point{}, anchorArrive)
			for _, d := range []time.Duration{4 * time.Second, 10 * time.Second, 30 * time.Second} {
				rule, a = tradeTurn(t, b, now.Add(d), teleportSelf(Point{X: 50}, around...)...)
				wantTurn(t, "nobody left", rule, a, "valuables", actHold, nil)
			}
		})
	}

	t.Run("trade, then the walk back before the next line", func(t *testing.T) {
		b := inviteBrain(t, nil)
		sent := inviteSent(t, b)
		start := TradeStart{PartnerID: pcID, Items: []TradeItem{{ObjectID: 101, TemplateID: ledgerSword, Count: 1}}}
		rule, a := tradeTurn(t, b, sent.Add(time.Second), start)
		wantTurn(t, "offer", rule, a, "prompt", actTradeAdd, encodeAddTradeItem(101, 1))
		// The trade ends after AdEvery: the walk back comes before the line.
		now := at(b.persona.Valuables.AdEvery + time.Second)
		rule, a = tradeTurn(t, b, now, SendTradeDone{})
		wantWalk(t, "walk back", b, rule, a, Point{}, anchorArrive)
		if err := b.handle(teleportSelf(Point{X: 50}), now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if line := decideAd(t, "line at the anchor", b, now.Add(time.Second)); line == nil {
			t.Fatal("no line back at the anchor")
		}
	})

	// oneLine has the round say one line only, so no later line comes
	// before the request.
	oneLine := func(b *Brain) { b.persona.Valuables.AdLines = 1 }
	gated := []struct {
		name  string
		edit  func(*Brain)
		setup func(*Brain)
		now   time.Duration
	}{
		{name: "out of sight", setup: func(b *Brain) { delete(b.world.Objects, pcID) }, now: time.Second},
		{name: "not whispered", setup: func(b *Brain) { b.cfg.valuables.talk.to = nil }, now: time.Second},
		{name: "beyond InviteReach", setup: func(b *Brain) { placePlayer(t, b, pcID, Point{X: 1300}) }, now: time.Second},
		{
			name: "outside RoundReach less 100 of the anchor",
			edit: func(b *Brain) { b.persona.Valuables.RoundReach = 650 },
			now:  time.Second,
		},
		{name: "warehouse errand running", setup: func(b *Brain) { b.wh.running = true }, now: time.Second},
		{name: "budget spent", setup: func(b *Brain) { b.valuablesSold = b.persona.Valuables.Budget }, now: time.Second},
		{name: "within 15 s of RoundWait", edit: oneLine, now: 5*time.Minute - tradeRequestTimeout},
	}
	for _, tt := range gated {
		t.Run("none: "+tt.name, func(t *testing.T) {
			b := inviteBrain(t, tt.edit)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := tradeTurn(t, b, at(tt.now))
			if a.kind == actMove || a.kind == actTradeRequest {
				t.Fatalf("decide = rule %q action %d, want no legs or request", rule, a.kind)
			}
			if b.cfg.valuables.round.invite.partner != 0 {
				t.Fatalf("invite %+v, want none", b.cfg.valuables.round.invite)
			}
		})
	}

	t.Run("asked just before the cut-off", func(t *testing.T) {
		b := inviteBrain(t, oneLine)
		rule, a := tradeTurn(t, b, at(5*time.Minute-tradeRequestTimeout-time.Second))
		wantWalk(t, "legs", b, rule, a, Point{X: 600}, inviteArrive)
		// The cut-off passes on the way: no request, the walk back.
		rule, a = tradeTurn(t, b, at(5*time.Minute-tradeRequestTimeout), teleportSelf(Point{X: 550}, seen(pcID, Point{X: 600}))...)
		wantWalk(t, "walk back", b, rule, a, Point{}, anchorArrive)
	})
}

// tradeOpen opens a trade window with pcID standing 100 north at now,
// offering rows.
func tradeOpen(t *testing.T, b *Brain, now time.Time, rows ...TradeItem) {
	t.Helper()
	placePlayer(t, b, pcID, Point{Y: 100})
	if err := b.handle([]Event{TradeStart{PartnerID: pcID, Items: rows}}, now); err != nil {
		t.Fatal(err)
	}
}

func rowOf(it Item) TradeItem {
	return TradeItem{ObjectID: it.ObjectID, TemplateID: it.TemplateID, Count: it.Count, Enchant: it.Enchant}
}

func adenaPaid(n int32) TradeItem { return TradeItem{ObjectID: 900, TemplateID: adenaID, Count: n} }

// TestValuablesTradeConfirm checks a valuable's trade
// (docs/valuables/trades.md "Trades", release.md "Valuables budget
// (D34)"): offered in a round only, the partner's named or agreed entry
// first, at the agreed price, within the budget's offer rule, counted in
// the valuables budget on success.
func TestValuablesTradeConfirm(t *testing.T) {
	sword := swordUnit(101, 0)
	staff := Item{ObjectID: 102, TemplateID: ledgerStaff, Count: 1, BodySlot: partRHand}
	recipes := lootStack(103, ledgerRecipe, 5)
	newBrain := func(t *testing.T, round bool) *Brain {
		t.Helper()
		b := newAdBrain(t, NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour), 0, sword, staff)
		// The session's look ran: the private warehouse holds none.
		b.wh.private.Known = true
		if round {
			openAdRound(b, t0)
		}
		b.syncValuables(t0)
		return b
	}
	agree := func(b *Brain, k valuableKey, price int64, agreed bool) {
		v := b.cfg.valuables
		if v.talk.deals == nil {
			v.talk.deals = make(map[whisperDeal]whisperPrice)
		}
		v.talk.deals[whisperDeal{pcID, k}] = whisperPrice{current: price, agreed: agreed}
	}

	t.Run("agreed sword: short, cancel at Timeout", func(t *testing.T) {
		b := newBrain(t, true)
		agree(b, swordKey, 150000, true)
		tradeOpen(t, b, t0, rowOf(staff), rowOf(sword))
		rule, a := tradeTurn(t, b, at(time.Second))
		wantTurn(t, "offer", rule, a, "prompt", actTradeAdd, encodeAddTradeItem(101, 1))
		rule, a = tradeTurn(t, b, at(2*time.Second), TradeOwnAdd{Item: rowOf(sword)}, TradeOtherAdd{Item: adenaPaid(149999)})
		wantTurn(t, "short", rule, a, "prompt", actHold, nil)
		rule, a = tradeTurn(t, b, at(time.Minute-time.Millisecond))
		wantTurn(t, "short still", rule, a, "prompt", actHold, nil)
		rule, a = tradeTurn(t, b, at(time.Minute))
		wantTurn(t, "Timeout", rule, a, "prompt", actTradeCancel, nil)
	})

	t.Run("confirm while short: cancel at once", func(t *testing.T) {
		b := newBrain(t, true)
		agree(b, swordKey, 150000, true)
		tradeOpen(t, b, t0, rowOf(sword))
		tradeTurn(t, b, at(time.Second))
		rule, a := tradeTurn(t, b, at(2*time.Second), TradeOwnAdd{Item: rowOf(sword)}, TradeOtherAdd{Item: adenaPaid(100000)}, TradePressOtherOk{})
		wantTurn(t, "cancel", rule, a, "prompt", actTradeCancel, nil)
	})

	t.Run("paid: confirm, counted, entry gone, next line", func(t *testing.T) {
		b := newBrain(t, true)
		agree(b, swordKey, 150000, true)
		b.store.sold = 0
		tradeOpen(t, b, t0, rowOf(staff), rowOf(sword))
		tradeTurn(t, b, at(time.Second))
		rule, a := tradeTurn(t, b, at(2*time.Second), TradeOwnAdd{Item: rowOf(sword)}, TradeOtherAdd{Item: adenaPaid(150000)})
		wantTurn(t, "confirm", rule, a, "prompt", actTradeConfirm, encodeTradeDone(true))
		settle := []Event{
			TradePressOwnOk{},
			TradePressOtherOk{},
			InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: sword}}},
			SendTradeDone{Success: true},
		}
		if err := b.handle(settle, at(3*time.Second)); err != nil {
			t.Fatal(err)
		}
		if b.valuablesSold != 150000 || b.store.sold != 0 {
			t.Fatalf("valuables sold %d, store sold %d; want 150000 and 0", b.valuablesSold, b.store.sold)
		}
		b.syncValuables(t0)
		if _, ok := b.cfg.valuables.entries[swordKey]; ok {
			t.Fatal("sword entry still there after the sale")
		}
		if got, want := decideAd(t, "next line", b, at(4*time.Second)), encodeSay2(adLine(t, b, staffKey), chatTrade); !bytes.Equal(got, want) {
			t.Fatalf("next line % x, want % x (the staff alone)", got, want)
		}
	})

	t.Run("no round: not offered, request declined", func(t *testing.T) {
		b := newBrain(t, false)
		rule, a := tradeTurn(t, b, at(time.Second), SendTradeRequest{SenderID: pcID})
		wantTurn(t, "decline", rule, a, "prompt", actAnswerTrade, encodeAnswerTradeRequest(false))
		tradeOpen(t, b, at(2*time.Second), rowOf(sword), rowOf(staff))
		rule, a = tradeTurn(t, b, at(3*time.Second))
		wantTurn(t, "nothing to offer", rule, a, "prompt", actTradeCancel, nil)
	})

	t.Run("Every holds back all but an agreed partner", func(t *testing.T) {
		for _, agreed := range []bool{false, true} {
			b := newBrain(t, true)
			placePlayer(t, b, pcID, Point{Y: 100})
			agree(b, swordKey, 150000, agreed)
			b.tradedAt[pcID] = t0
			rule, a := tradeTurn(t, b, at(time.Second), SendTradeRequest{SenderID: pcID})
			wantTurn(t, "answer", rule, a, "prompt", actAnswerTrade, encodeAnswerTradeRequest(agreed))
		}
	})

	t.Run("goods: agreed, else named, else top-scored", func(t *testing.T) {
		b := newBrain(t, true)
		top := b.adEntries()
		if len(top) != 2 {
			t.Fatalf("entries by score %v, want the sword and the staff", top)
		}
		objOf := map[valuableKey]int32{swordKey: 101, staffKey: 102}
		rows := []TradeItem{rowOf(sword), rowOf(staff)}
		check := func(name string, want valuableKey) {
			t.Helper()
			goods := b.valuableGoods(pcID, rows)
			if len(goods) != 1 || goods[0].key != want || goods[0].row.ObjectID != objOf[want] {
				t.Fatalf("%s: goods %+v, want %v alone", name, goods, want)
			}
		}
		check("no deal", top[0])
		agree(b, top[1], 1000000, false)
		check("named", top[1])
		agree(b, top[0], 1000000, true)
		check("agreed over named", top[0])
	})

	t.Run("budget: offered while under it, ceil(left/unit) units", func(t *testing.T) {
		b := newAdBrain(t, NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour), 0, recipes)
		openAdRound(b, t0)
		b.syncValuables(t0)
		unit := entryOf(t, b, ledgerRecipe, 0).ask
		budget := b.persona.Valuables.Budget
		for _, tt := range []struct {
			sold  int64
			units int32
			ok    bool
		}{
			{0, int32(min(5, ceilDiv(budget, unit))), true},
			{budget - unit - 1, 2, true},
			{budget - 1, 1, true},
			{budget, 0, false},
		} {
			b.valuablesSold = tt.sold
			goods := b.valuableGoods(pcID, []TradeItem{rowOf(recipes)})
			if ok := len(goods) == 1; ok != tt.ok || ok && goods[0].row.Count != tt.units {
				t.Fatalf("sold %d: goods %+v, want %d units %v", tt.sold, goods, tt.units, tt.ok)
			}
		}
	})

	t.Run("budget spent: request declined, nothing offered", func(t *testing.T) {
		b := newBrain(t, true)
		placePlayer(t, b, pcID, Point{Y: 100})
		b.valuablesSold = b.persona.Valuables.Budget
		rule, a := tradeTurn(t, b, at(time.Second), SendTradeRequest{SenderID: pcID})
		wantTurn(t, "decline", rule, a, "prompt", actAnswerTrade, encodeAnswerTradeRequest(false))
	})
}

// TestValuablesWhisperDeals checks the deals the responder records for the
// trade's goods: an entry named alone, an agreement, and a counter after it.
func TestValuablesWhisperDeals(t *testing.T) {
	b := newWhisperBrain(t, swordUnit(101, 0))
	openAdRound(b, t0)
	deal := func() (whisperPrice, bool) {
		d, ok := b.cfg.valuables.talk.deals[whisperDeal{pcID, swordKey}]
		return d, ok
	}
	talk(t, b, pcID, "Alden", t0, whisperLine{"sword of watershadow?", "Sword of Watershadow 170k, trade me"})
	if d, ok := deal(); !ok || d.agreed || d.current != 170000 {
		t.Fatalf("named: deal %+v %v, want the ask, not agreed", d, ok)
	}
	talk(t, b, pcID, "Alden", at(time.Minute), whisperLine{"200k", "ok 170k, trade me"})
	if d, _ := deal(); !d.agreed || !b.agreedDeal(pcID) {
		t.Fatalf("agreed: deal %+v, want agreed", d)
	}
	talk(t, b, pcID, "Alden", at(2*time.Minute), whisperLine{"140k", "155k lowest"})
	if d, _ := deal(); d.agreed || d.current != 155000 {
		t.Fatalf("countered: deal %+v, want 155000 not agreed", d)
	}
}
