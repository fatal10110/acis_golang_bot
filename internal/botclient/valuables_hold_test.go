package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// newHoldBrain is newRoundBrain with the keeper keeper, the sword 101
// carried and the animal skins as store loot.
func newHoldBrain(t *testing.T, keeper Point) *Brain {
	t.Helper()
	b := newRoundBrain(t, keeper, nil, swordUnit(101, 0), skinLoot)
	b.cfg.Travel.prices[animalSkin] = lootPrices[animalSkin]
	return b
}

// boardHeld holds the region's ad board for the hour cap's 59 minutes.
func boardHeld(b *Brain) {
	b.cfg.Ads = NewAdBoard(DefaultAdRegionGap, 1)
	b.cfg.Ads.Claim(1, t0.Add(-time.Minute))
}

// wantHeld checks the decision at now is rule "valuables" holding the turn
// with a wait: no store, no move.
func wantHeld(t *testing.T, name string, b *Brain, now time.Time) {
	t.Helper()
	if rule, a := b.decide(now); rule != "valuables" || a.kind != actHold {
		t.Fatalf("%s: decide = rule %q action %d, want rule valuables holding", name, rule, a.kind)
	}
}

// wantOpening checks the decision at now opens the store.
func wantOpening(t *testing.T, name string, b *Brain, now time.Time) {
	t.Helper()
	rule, a := b.decide(now)
	wantTurn(t, name, rule, a, "store", actStoreManage, botwire.EncodeRequestPrivateStoreManageSell())
}

// TestValuablesStoreHold: a pending round keeps the store from opening
// for StoreHold at most (docs/valuables/round.md "The store waits for a
// pending round", "Tests").
func TestValuablesStoreHold(t *testing.T) {
	t.Run("held past StoreHold", testHoldPast)
	t.Run("freed within StoreHold", testHoldFreed)
	t.Run("market visit", testHoldMarket)
	t.Run("within RoundGap", func(t *testing.T) {
		b := newHoldBrain(t, nearKeeper)
		boardHeld(b)
		b.cfg.valuables.lastStart = t0.Add(-time.Minute)
		wantOpening(t, "within RoundGap", b, t0)
	})
	t.Run("leaving and returning", testHoldLeave)
	t.Run("no loot", testHoldNoLoot)
	t.Run("look with loot", testHoldLook)
	t.Run("look not done, board held", testHoldLookWait)
	t.Run("Dion", testHoldDion)
	t.Run("own trade request", testHoldTradeRequest)
}

// testHoldPast: the board or a 30 minute backoff held past StoreHold: the
// rule holds the turn StoreHold only, then the store opens with the loot
// only.
func testHoldPast(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(b *Brain)
	}{
		{"board", boardHeld},
		{"30 minute backoff", func(b *Brain) { b.wh.openedAt, b.wh.timeouts = t0, 5 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newHoldBrain(t, nearKeeper)
			tt.setup(b)
			hold := b.persona.Valuables.StoreHold
			wantHeld(t, "pending", b, t0)
			wantHeld(t, "within StoreHold", b, at(hold-time.Second))
			wantOpening(t, "past StoreHold", b, at(hold))
			if !b.storeSells(skinLoot) || b.storeSells(swordUnit(101, 0)) {
				t.Errorf("store sells skins %v, sword %v; want the skins only", b.storeSells(skinLoot), b.storeSells(swordUnit(101, 0)))
			}
		})
	}
}

// testHoldFreed: the board freed within StoreHold starts the round, and no
// store opens until it ends.
func testHoldFreed(t *testing.T) {
	b := newHoldBrain(t, nearKeeper)
	b.cfg.Ads.Claim(0, t0.Add(-30*time.Second))
	wantHeld(t, "board held", b, t0)
	line := roundLine(t, b)
	rule, a := tradeTurn(t, b, at(30*time.Second))
	wantTurn(t, "freed: the first line", rule, a, "valuables", actSay, line)
	if rule, a := b.decide(at(30*time.Second + b.persona.Valuables.RoundWait - time.Second)); rule != "valuables" || a.kind == actStoreManage {
		t.Fatalf("in the round: rule %q action %d, want the round holding the turn", rule, a.kind)
	}
}

// testHoldMarket: a running market visit is stepped to its end by rule
// "market", no store opening; the count runs meanwhile.
func testHoldMarket(t *testing.T) {
	b := newHoldBrain(t, nearKeeper)
	boardHeld(b)
	placePlayer(t, b, pcID, Point{X: 50})
	b.world.Objects[pcID].StoreType = storeTypeSell
	b.market.v.obj = pcID
	if rule, _ := b.decide(t0); rule != "market" {
		t.Fatalf("visit running: rule %q, want market", rule)
	}
	b.world.Objects[pcID].StoreType = 0
	if rule, a := b.decide(at(time.Second)); rule == "store" || a.kind == actStoreManage || b.market.active() {
		t.Fatalf("visit ending: rule %q action %d visit %v, want it ended with no store", rule, a.kind, b.market.active())
	}
	hold := b.persona.Valuables.StoreHold
	wantHeld(t, "after the visit", b, at(2*time.Second))
	wantOpening(t, "StoreHold from the first held decision", b, at(hold))
}

// testHoldLeave: leaving the town resets the count; back in it the hold
// runs StoreHold again.
func testHoldLeave(t *testing.T) {
	b := newHoldBrain(t, nearKeeper)
	boardHeld(b)
	hold := b.persona.Valuables.StoreHold
	wantHeld(t, "pending", b, t0)
	away := at(90 * time.Second)
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 100000}}}, away); err != nil {
		t.Fatal(err)
	}
	if rule, a := b.decide(away); rule == "valuables" || a.kind == actStoreManage || !b.held.since.IsZero() {
		t.Fatalf("out of town: rule %q action %d hold %+v, want no hold", rule, a.kind, b.held)
	}
	back := at(100 * time.Second)
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{}}}, back); err != nil {
		t.Fatal(err)
	}
	wantHeld(t, "back", b, back)
	wantHeld(t, "within StoreHold of the return", b, back.Add(hold-time.Second))
	wantOpening(t, "StoreHold after the return", b, back.Add(hold))
}

// testHoldNoLoot: with no loot the board held past StoreHold holds the
// turn StoreHold, then a due supply trip starts.
func testHoldNoLoot(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0), soeStack(1))
	boardHeld(b)
	b.persona.Supply = defaultSupplyPolicy()
	b.persona.Potions = []int32{lesserHealingPotion}
	b.cfg.Travel.spawnedSold = map[int32]bool{lesserHealingPotion: true}
	b.cfg.Travel.weights = map[int32]int32{lesserHealingPotion: 5, scrollOfEscape: 120}
	b.cfg.Spots = NewSpotRegistry([]datapack.Spot{testSpot("s", 0, 0, 3, 6, 9)}, 1)
	b.cfg.Spots.claim([]int{0})
	b.spot.idx, b.spot.held, b.spot.arrived = 0, true, true
	b.supplyTrip.potions = true
	hold := b.persona.Valuables.StoreHold
	wantHeld(t, "pending", b, t0)
	wantHeld(t, "within StoreHold", b, at(hold-time.Second))
	if rule, _ := b.decide(at(hold)); rule != "supply" {
		t.Fatalf("past StoreHold: rule %q, want the supply trip", rule)
	}
}

// answerLook is the look's answer at d: nothing stored, the bot at the
// anchor, so the rule holds the turn.
func answerLook(d time.Duration) trainerStep {
	return trainerStep{name: "look answered", at: d, events: []Event{SystemMessage{ID: msgNoItemDepositedInW}}, wantRule: "valuables"}
}

// testHoldLook: after a look with loot carried no store opens in the 60
// seconds to the next open; then the round starts.
func testHoldLook(t *testing.T) {
	b := newHoldBrain(t, nearKeeper)
	b.wh.private = WarehouseStatus{}
	runTrainerSteps(t, b, append(lookSteps(0), answerLook(5*time.Second)))
	if !b.wh.private.Known {
		t.Fatal("look not done")
	}
	open := 3*time.Second + warehouseOpenGap
	wantHeld(t, "open not due", b, at(open-time.Second))
	line := roundLine(t, b)
	rule, a := tradeTurn(t, b, at(open))
	wantTurn(t, "open due: the first line", rule, a, "valuables", actSay, line)
}

// testHoldLookWait: the look not done and the board held: the look starts
// once the open is due, its wait counting and its run not.
func testHoldLookWait(t *testing.T) {
	b := newHoldBrain(t, nearKeeper)
	boardHeld(b)
	b.wh.private = WarehouseStatus{}
	b.wh.openedAt = t0.Add(30*time.Second - warehouseOpenGap)
	hold := b.persona.Valuables.StoreHold
	wantHeld(t, "open not due", b, t0)
	runTrainerSteps(t, b, append(lookSteps(30*time.Second), answerLook(35*time.Second)))
	// The look ran from 30 s to 35 s: StoreHold ends 5 s late.
	wantHeld(t, "within StoreHold and the look", b, at(hold+5*time.Second-time.Second))
	wantOpening(t, "past StoreHold", b, at(hold+5*time.Second))
}

// testHoldDion: the look's visit to a keeper out of the town radius
// (Trisha's 30058, 5348 away) does not reset the count.
func testHoldDion(t *testing.T) {
	keeper := Point{X: 5348}
	b := newHoldBrain(t, keeper)
	boardHeld(b)
	b.wh.private = WarehouseStatus{}
	b.wh.openedAt = t0.Add(30*time.Second - warehouseOpenGap)
	hold := b.persona.Valuables.StoreHold
	wantHeld(t, "open not due", b, t0)
	steps := lookSteps(31 * time.Second)
	steps[0].events = []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5100}}, NPCInfo{ObjectID: keeperObj, NPCID: keeperNPC, Pos: keeper}}
	steps = append([]trainerStep{{name: "walk to the keeper", at: 30 * time.Second, wantRule: "valuables", wantKind: actMove}}, steps...)
	runTrainerSteps(t, b, append(steps, trainerStep{
		name: "answered: walk back", at: 36 * time.Second, events: []Event{SystemMessage{ID: msgNoItemDepositedInW}},
		wantRule: "valuables", wantKind: actMove, wantTo: Point{}, wantWithin: anchorArrive,
	}))
	if !b.held.since.Equal(t0) {
		t.Fatalf("hold %+v, want counting since t0", b.held)
	}
	runTrainerSteps(t, b, []trainerStep{{name: "back", at: 37 * time.Second, events: []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 50}}}, wantRule: "valuables"}})
	// The look ran from 30 s to 37 s.
	wantHeld(t, "within StoreHold and the look", b, at(hold+7*time.Second-time.Second))
	wantOpening(t, "past StoreHold", b, at(hold+7*time.Second))
}

// testHoldTradeRequest: the store opening after a round waits for the
// bot's own unanswered TradeRequest: up to 15 s, or its refusal.
func testHoldTradeRequest(t *testing.T) {
	for _, tt := range []struct {
		name string
		// answer, when set, refuses the request at refused after the end.
		answer  []Event
		refused time.Duration
	}{
		{"unanswered", nil, 0},
		{"refused", []Event{SystemMessage{ID: msgTradeDeclined}}, 2 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newHoldBrain(t, nearKeeper)
			firstLine(t, b)
			end := at(b.persona.Valuables.RoundWait)
			b.wh.retryAt = end.Add(time.Minute)
			sent := end.Add(-5 * time.Second)
			b.cfg.valuables.round.invite = valuableInvite{partner: pcID, sent: sent}
			if rule, a := b.decide(end); rule == "store" || a.kind == actStoreManage {
				t.Fatalf("round end: rule %q action %d, want no store", rule, a.kind)
			}
			wantNoRound(t, "round end", b)
			if tt.answer != nil {
				now := end.Add(tt.refused)
				if err := b.handle(tt.answer, now); err != nil {
					t.Fatal(err)
				}
				wantOpening(t, "refused", b, now)
				return
			}
			if rule, a := b.decide(sent.Add(tradeRequestTimeout - time.Second)); rule == "store" || a.kind == actStoreManage {
				t.Fatalf("request unanswered: rule %q action %d, want no store", rule, a.kind)
			}
			wantOpening(t, "request expired", b, sent.Add(tradeRequestTimeout))
		})
	}
}

// TestValuablesOffRound: with the valuables budget spent the carried ones
// are deposited outside a round, once per new find
// (docs/valuables/round-end.md "Off-round errands", "Tests").
func TestValuablesOffRound(t *testing.T) {
	t.Run("deposit", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		b.valuablesSold = b.persona.Valuables.Budget
		runTrainerSteps(t, b, append(depositSteps(0, swordDepositList, swordDepositRows), trainerStep{
			name: "deposited: walk back", at: 6 * time.Second,
			events:   []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: swordUnit(101, 0)}, {State: ItemModified, Item: adenaStack(970)}}}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 250}}},
			wantRule: "valuables", wantKind: actMove, wantTo: Point{}, wantWithin: anchorArrive,
		}))
		if e := entryOf(t, b, ledgerSword, 0); e.stored != 1 || e.carried != 0 {
			t.Fatalf("sword %+v, want stored", e)
		}
		if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 50}}}, at(7*time.Second)); err != nil {
			t.Fatal(err)
		}
		if rule, a := b.decide(at(7 * time.Second)); rule == "valuables" || a.kind == actSay {
			t.Fatalf("back: rule %q action %d, want the rule let go", rule, a.kind)
		}
		if v := b.cfg.valuables; v.round.ads != 0 || !v.lastStart.IsZero() {
			t.Errorf("round %+v last start %v, want no round", v.round, v.lastStart)
		}
	})
	t.Run("fee short", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		b.valuablesSold = b.persona.Valuables.Budget
		adena := func(n int32, now time.Time) {
			t.Helper()
			if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: adenaStack(n)}}}}, now); err != nil {
				t.Fatal(err)
			}
		}
		adena(10, t0)
		if rule, _ := b.decide(t0); rule == "valuables" || b.wh.running || !b.wh.retryAt.Equal(at(warehouseOpenGap)) {
			t.Fatalf("fee short: rule %q running %v retry %v, want the deposit failed", rule, b.wh.running, b.wh.retryAt)
		}
		adena(1000, at(time.Second))
		for _, d := range []time.Duration{warehouseOpenGap, warehouseBackoffMax + time.Minute} {
			if rule, a := b.decide(at(d)); rule == "valuables" || a.kind == actSelect || b.wh.running {
				t.Fatalf("at %v: rule %q action %d, want no retry", d, rule, a.kind)
			}
		}
		now := at(warehouseBackoffMax + 2*time.Minute)
		if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: swordUnit(102, 0)}}}}, now); err != nil {
			t.Fatal(err)
		}
		if rule, a := b.decide(now); rule != "valuables" || a.kind != actSelect || b.wh.req.Deposit == nil {
			t.Fatalf("new find: rule %q action %d request %+v, want the deposit at the keeper", rule, a.kind, b.wh.req)
		}
	})
}
