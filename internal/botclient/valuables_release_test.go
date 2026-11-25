package botclient

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Manage list candidates of the release tests: the carried swords 101 and
// 103 (+3), and the +5 staff once withdrawn.
var (
	swordCand101 = StoreItem{ObjectID: 101, TemplateID: ledgerSword, Count: 1, BodySlot: partRHand, Price: 136000, ReferencePrice: 136000}
	swordCand103 = StoreItem{ObjectID: 103, TemplateID: ledgerSword, Count: 1, Enchant: 3, BodySlot: partRHand, Price: 136000, ReferencePrice: 136000}
	staffCand    = StoreItem{ObjectID: 201, TemplateID: ledgerStaff, Count: 1, Enchant: 5, BodySlot: partRHand, Price: 644000, ReferencePrice: 644000}
	sword3Key    = valuableKey{template: ledgerSword, enchant: 3}
)

// release marks entry k released, failing when there is none.
func release(t *testing.T, b *Brain, k valuableKey) {
	t.Helper()
	e, ok := b.cfg.valuables.entries[k]
	if !ok {
		t.Fatalf("no entry %v", k)
	}
	e.released = true
}

// listRows has b's opening take the manage list of cands and returns the
// rows it lists.
func listRows(b *Brain, cands ...StoreItem) []storeRow {
	b.store.step = storeAskManage
	b.storeManageList(PrivateStoreManageListSell{OwnerID: selfID, Adena: 1000, Candidates: cands})
	return b.storeRows()
}

// openingSteps are rule "store"'s opening from start with the manage list
// of cands, listing rows.
func openingSteps(start time.Duration, rows []storeRow, cands ...StoreItem) []trainerStep {
	return []trainerStep{
		{name: "open", at: start, wantRule: "store", wantKind: actStoreManage, wantPayload: encodeRequestPrivateStoreManageSell()},
		{name: "title", at: start + time.Second, events: []Event{PrivateStoreManageListSell{OwnerID: selfID, Adena: 1000, Candidates: cands}}, wantRule: "store", wantKind: actStoreTitle},
		{name: "list", at: start + 2*time.Second, wantRule: "store", wantKind: actStoreList, wantPayload: encodeSetPrivateStoreListSell(false, rows)},
	}
}

// TestValuablesRelease checks the cap and the release to the store
// (docs/valuables/release.md "Tests").
func TestValuablesRelease(t *testing.T) {
	t.Run("cap", testReleaseCap)
	t.Run("released carried: no deposit, the store at the anchor", testReleaseCarried)
	t.Run("not counted", testReleaseNotCounted)
	t.Run("stale round", testReleaseStale)
	t.Run("released in a round", testReleaseInRound)
	t.Run("budget spent", testReleaseBudgetSpent)
	t.Run("look finds a released stored one", testReleaseLook)
	t.Run("store rows", testReleaseRows)
	t.Run("a trade-sold valuable counts in no cap", testReleaseTradeSold)
	t.Run("after a process start the look's units count", testReleaseProcessStart)
	t.Run("fees", testReleaseFees)
}

// testReleaseTradeSold: two carried swords whose asks pass HoldValue
// together release at a cut-off after the first line; with one sold in a
// trade during the round, the other alone stays under it.
func testReleaseTradeSold(t *testing.T) {
	for _, sold := range []bool{false, true} {
		t.Run(map[bool]string{false: "both carried", true: "one trade-sold"}[sold], func(t *testing.T) {
			b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0), swordUnit(103, 3))
			b.cfg.valuables.entries[swordKey].ask = 1200000
			b.cfg.valuables.entries[sword3Key].ask = 1200000
			b.cfg.valuables.openRound(Point{}, t0)
			b.cfg.valuables.round.ads = 1
			if sold {
				if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{ItemRemoved, swordUnit(103, 3)}}}}, t0); err != nil {
					t.Fatal(err)
				}
				b.syncValuables()
			}
			b.endCut(t0)
			if got := b.cfg.valuables.released(swordKey); got != !sold {
				t.Errorf("+0 sword released %v, want %v", got, !sold)
			}
		})
	}
}

// testReleaseProcessStart: a process start's ledger is empty; the look's
// stored +0 Sword of Watershadow and +5 Conjuror's Staff come in stored
// and count in the next cap (two rows past HoldRows 1).
func testReleaseProcessStart(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, nil)
	if len(b.cfg.valuables.entries) != 0 {
		t.Fatalf("entries %v, want a fresh ledger", b.cfg.valuables.entries)
	}
	c := nextSession(t, b, map[int32][]Point{keeperNPC: {nearKeeper}}, map[int32]NPCInfo{keeperObj: {NPCID: keeperNPC, Pos: nearKeeper}})
	roundTown(c)
	c.world.Apply(ItemList{Items: []Item{adenaStack(1000)}}, t0)
	c.persona.Valuables.HoldRows = 1
	runTrainerSteps(t, c, lookSteps(0))
	tradeTurn(t, c, at(5*time.Second), privateList(storedSword, storedStaff))
	for _, k := range []valuableKey{swordKey, staffKey5} {
		if e := entryOf(t, c, k.template, k.enchant); e.stored != 1 || e.released {
			t.Fatalf("%v %+v, want stored by the look, not released", k, e)
		}
	}
	c.cfg.valuables.openRound(Point{}, at(6*time.Second))
	c.cfg.valuables.round.ads = 1
	c.endCut(at(7 * time.Second))
	for _, k := range []valuableKey{swordKey, staffKey5} {
		if !c.cfg.valuables.released(k) {
			t.Errorf("%v not released, want the stored units counted in the cap", k)
		}
	}
}

// testReleaseFees: the shop reserve's deposit fees count a released
// carried valuable only once the valuables budget is spent.
func testReleaseFees(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0), swordUnit(103, 3))
	b.persona.Valuables.HoldRows = 0
	release(t, b, sword3Key)
	if got := b.valuableFees(); got != warehouseFee {
		t.Errorf("budget left: fees %d, want the kept row's %d", got, warehouseFee)
	}
	b.valuablesSold = b.persona.Valuables.Budget
	if got := b.valuableFees(); got != 2*warehouseFee {
		t.Errorf("budget spent: fees %d, want both rows' %d", got, 2*warehouseFee)
	}
}

// testReleaseCap: the cap over carried and stored units, each entry once.
func testReleaseCap(t *testing.T) {
	scythe := valuableKey{template: 96}
	unit := func(ask, units int64, rounds int) *valuableEntry {
		return &valuableEntry{ask: ask, carried: units, rounds: rounds}
	}
	for _, tt := range []struct {
		name    string
		entries map[valuableKey]*valuableEntry
		want    []valuableKey
	}{
		{"six rows", map[valuableKey]*valuableEntry{swordKey: unit(170000, 6, 2)}, nil},
		{"a seventh row", map[valuableKey]*valuableEntry{swordKey: unit(170000, 6, 0), staffKey5: unit(900000, 1, 0)}, []valuableKey{swordKey, staffKey5}},
		{"a stack takes one row", map[valuableKey]*valuableEntry{{template: ledgerRecipe}: unit(10000, 20, 0)}, nil},
		{
			"asks past HoldValue, the Scythe alone above it",
			map[valuableKey]*valuableEntry{swordKey: unit(170000, 2, 0), staffKey5: {ask: 900000, stored: 2}, scythe: unit(2290000, 1, 2)},
			[]valuableKey{swordKey, staffKey5},
		},
		{"asks at HoldValue", map[valuableKey]*valuableEntry{swordKey: unit(1000000, 2, 0)}, nil},
		{"a third unsold round, alone", map[valuableKey]*valuableEntry{swordKey: unit(170000, 1, 3), staffKey5: unit(900000, 1, 2)}, []valuableKey{swordKey}},
		{"the Scythe after its third round", map[valuableKey]*valuableEntry{scythe: unit(2290000, 1, 3), swordKey: unit(170000, 1, 0)}, []valuableKey{scythe}},
		{"a released one counts", map[valuableKey]*valuableEntry{swordKey: {ask: 1500000, stored: 1, released: true}, staffKey5: unit(600000, 1, 0)}, []valuableKey{swordKey, staffKey5}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := newValuables()
			v.entries = tt.entries
			v.capEntries(DefaultValuablePolicy(), map[int32]bool{ledgerRecipe: true})
			for k, e := range v.entries {
				want := false
				for _, w := range tt.want {
					want = want || w == k
				}
				if e.released != want {
					t.Errorf("%v released %v, want %v", k, e.released, want)
				}
			}
		})
	}
}

// testReleaseCarried: a third unsold round releases the carried sword: no
// deposit, so no walk to the keeper out of the town (Trisha's 30058, 5348
// away); the store opens at the anchor listing the sword first at its
// floor, then the loot.
func testReleaseCarried(t *testing.T) {
	b := newRoundBrain(t, Point{X: 5348}, nil, swordUnit(101, 0), skinLoot)
	b.cfg.Travel.prices[animalSkin] = lootPrices[animalSkin]
	p := b.persona.Valuables
	b.cfg.valuables.entries[swordKey].rounds = p.MaxRounds - 1
	firstLine(t, b)
	if b.cfg.valuables.released(swordKey) {
		t.Fatal("released before the round's end")
	}
	end := p.RoundWait
	floor := b.valuableFloor(swordKey)
	runTrainerSteps(t, b, openingSteps(end, []storeRow{{101, 1, int32(floor)}, {102, 10, 7}}, swordCand101, skinCand))
	if e := entryOf(t, b, ledgerSword, 0); !e.released || e.rounds != p.MaxRounds || e.carried != 1 {
		t.Errorf("sword %+v, want released after its third round, carried", e)
	}
	if b.wh.private.Deposit.Outcome != 0 || b.cfg.valuables.deposited != nil {
		t.Errorf("deposit %+v rows %v, want none", b.wh.private.Deposit, b.cfg.valuables.deposited)
	}
}

// testReleaseNotCounted: a FirstLineWait round and a cut-off before the
// first line release nothing; a cut-off after it runs the cap.
func testReleaseNotCounted(t *testing.T) {
	for _, tt := range []struct {
		name string
		end  func(b *Brain)
		want bool
	}{
		{"FirstLineWait", func(b *Brain) { b.endOpen(t0, false) }, false},
		{"cut-off before the first line", func(b *Brain) { b.endCut(t0) }, false},
		{"cut-off after the first line", func(b *Brain) { b.cfg.valuables.round.ads = 1; b.endCut(t0) }, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
			b.cfg.valuables.entries[swordKey].rounds = b.persona.Valuables.MaxRounds - 1
			b.cfg.valuables.openRound(Point{}, t0)
			tt.end(b)
			if got := b.cfg.valuables.released(swordKey); got != tt.want {
				t.Errorf("released %v, want %v", got, tt.want)
			}
		})
	}
}

// testReleaseStale: a round left open by an ended session, cut after its
// first line, steps its entries at the next session's first decision and
// caps them only once its look is done, which counts the stored units.
func testReleaseStale(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
	b.cfg.valuables.entries[swordKey].rounds = b.persona.Valuables.MaxRounds - 1
	b.cfg.valuables.openRound(Point{}, t0)
	b.cfg.valuables.round.ads = 1
	c := nextSession(t, b, map[int32][]Point{keeperNPC: {nearKeeper}}, map[int32]NPCInfo{keeperObj: {NPCID: keeperNPC, Pos: nearKeeper}})
	roundTown(c)
	c.world.Apply(ItemList{Items: []Item{adenaStack(1000), swordUnit(101, 0)}}, t0)
	runTrainerSteps(t, c, lookSteps(0))
	if e := entryOf(t, c, ledgerSword, 0); e.rounds != c.persona.Valuables.MaxRounds || e.released || c.cfg.valuables.round.open {
		t.Fatalf("look running: sword %+v round open %v; want stepped, not released, the round ended", e, c.cfg.valuables.round.open)
	}
	tradeTurn(t, c, at(5*time.Second), privateList(storedStaff))
	c.decide(at(6 * time.Second))
	if !c.cfg.valuables.released(swordKey) || c.cfg.valuables.capDue {
		t.Errorf("look done: sword released %v cap due %v; want the cap run", c.cfg.valuables.released(swordKey), c.cfg.valuables.capDue)
	}
	if e := entryOf(t, c, ledgerStaff, 5); e.stored != 1 || e.released {
		t.Errorf("staff %+v, want counted stored, not released", e)
	}
}

// testReleaseInRound: a round with a released stored and a released
// carried valuable: the withdrawal takes the stored one, no line names
// either, a whisper naming one gets no haggle answer, the trade offers
// neither, their rounds do not step and the deposit leaves them carried.
func testReleaseInRound(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff}, swordUnit(101, 0), swordUnit(103, 3))
	release(t, b, staffKey5)
	release(t, b, sword3Key)
	runTrainerSteps(t, b, append(keeperSteps(0, "WithdrawP"), trainerStep{
		name: "withdraw the released staff", at: 5 * time.Second, events: []Event{privateList(storedStaff)}, wantRule: "valuables", wantKind: actWarehouseWithdraw,
		wantPayload: encodeWarehouseRequest(opGameSendWareHouseWithdrawList, []warehouseRow{{object: 201, item: ledgerStaff, count: 1}}),
	}))
	now := at(6 * time.Second)
	if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{ItemAdded, carriedOf(storedStaff)}}}}, now); err != nil {
		t.Fatal(err)
	}
	b.syncValuables()
	if keys := b.adEntries(); !reflect.DeepEqual(keys, []valuableKey{swordKey}) {
		t.Fatalf("line entries %v, want the +0 sword only", keys)
	}
	line := roundLine(t, b)
	rule, a := tradeTurn(t, b, now)
	wantTurn(t, "the first line", rule, a, "valuables", actSay, line)
	if got := replyTo(t, b, pcID, "Alden", "conjuror's staff?", now.Add(time.Second)); strings.Contains(strings.ToLower(got), "conjuror") {
		t.Errorf("whisper naming the staff: reply %q, want none naming it", got)
	}
	goods := b.valuableGoods(pcID, []TradeItem{rowOf(carriedOf(storedStaff)), rowOf(swordUnit(103, 3)), rowOf(swordUnit(101, 0))})
	if len(goods) != 1 || goods[0].key != swordKey || goods[0].row.ObjectID != 101 {
		t.Errorf("trade goods %+v, want the +0 sword", goods)
	}
	end := now.Add(b.persona.Valuables.RoundWait)
	tradeTurn(t, b, end)
	if len(b.wh.req.Deposit) != 1 || b.wh.req.Deposit[0].Object != 101 {
		t.Errorf("deposit %+v, want the +0 sword only", b.wh.req.Deposit)
	}
	for _, k := range []valuableKey{staffKey5, sword3Key} {
		if e := entryOf(t, b, k.template, k.enchant); e.rounds != 0 || e.carried != 1 {
			t.Errorf("%v %+v, want carried, rounds not stepped", k, e)
		}
	}
	if e := entryOf(t, b, ledgerSword, 0); e.rounds != 1 {
		t.Errorf("sword %+v, want rounds stepped", e)
	}
}

// testReleaseBudgetSpent: with the valuables budget spent no released
// stored one is withdrawn, none is listed, and the off-round deposit takes
// the released carried ones.
func testReleaseBudgetSpent(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff}, swordUnit(101, 0), swordUnit(103, 3))
	release(t, b, staffKey5)
	release(t, b, sword3Key)
	b.valuablesSold = b.persona.Valuables.Budget
	if rows := b.storedRows(); len(rows) != 0 || b.releaseDue(t0) {
		t.Errorf("stored rows %+v release due %v, want none", rows, b.releaseDue(t0))
	}
	if rows := listRows(b, swordCand103); len(rows) != 0 {
		t.Errorf("store rows %+v, want none", rows)
	}
	b.store.step, b.store.candidates = storeIdle, nil
	runTrainerSteps(t, b, depositSteps(0, []WarehouseItem{{ObjectID: 101, TemplateID: ledgerSword, Count: 1}, {ObjectID: 103, TemplateID: ledgerSword, Count: 1, Enchant: 3}},
		[]warehouseRow{{object: 101, item: ledgerSword, count: 1}, {object: 103, item: ledgerSword, count: 1}}))
}

// testReleaseLook: a look finding a released stored valuable, loot carried:
// no store and no move until the open is due, then the off-round
// withdrawal, the walk back and the opening listing it first at its floor
// (reference plus five scrolls), then the loot; with a backoff past
// StoreHold the store opens at StoreHold with the loot only.
func testReleaseLook(t *testing.T) {
	setup := func(t *testing.T) *Brain {
		b := newRoundBrain(t, nearKeeper, nil, skinLoot)
		b.cfg.Travel.prices[animalSkin] = lootPrices[animalSkin]
		b.wh.private = WarehouseStatus{}
		b.cfg.valuables.entries[staffKey5] = &valuableEntry{ask: b.firstAsk(staffKey5), stored: 1, released: true}
		runTrainerSteps(t, b, append(lookSteps(0), trainerStep{name: "look listed", at: 5 * time.Second, events: []Event{privateList(storedStaff)}, wantRule: "valuables"}))
		if !b.wh.private.Known || !b.cfg.valuables.released(staffKey5) {
			t.Fatalf("look not done or staff not released: %+v", b.cfg.valuables.entries)
		}
		return b
	}
	t.Run("withdrawn, listed first", func(t *testing.T) {
		b := setup(t)
		open := 3*time.Second + warehouseOpenGap
		wantHeld(t, "open not due", b, at(open-time.Second))
		// The look's keeper is still selected: the visit talks at once.
		runTrainerSteps(t, b, append(keeperSteps(open-time.Second, "WithdrawP")[1:], trainerStep{
			name: "withdraw the staff", at: open + 4*time.Second, events: []Event{privateList(storedStaff)}, wantRule: "valuables", wantKind: actWarehouseWithdraw,
			wantPayload: encodeWarehouseRequest(opGameSendWareHouseWithdrawList, []warehouseRow{{object: 201, item: ledgerStaff, count: 1}}),
		}))
		floor := b.valuableFloor(staffKey5)
		if floor != 644000+5*50000 {
			t.Errorf("+5 staff floor %d, want reference plus five scrolls", floor)
		}
		withdrawn := trainerStep{
			name: "withdrawn: walk back", at: open + 6*time.Second,
			events:   []Event{InventoryUpdate{Changes: []ItemChange{{ItemAdded, carriedOf(storedStaff)}}}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 250}}},
			wantRule: "valuables", wantKind: actMove, wantTo: Point{}, wantWithin: anchorArrive,
		}
		steps := openingSteps(open+8*time.Second, []storeRow{{201, 1, int32(floor)}, {102, 10, 7}}, staffCand, skinCand)
		steps[0].events = []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 50}}}
		runTrainerSteps(t, b, append([]trainerStep{withdrawn}, steps...))
	})
	t.Run("backoff past StoreHold", func(t *testing.T) {
		b := setup(t)
		b.wh.timeouts = 5
		hold := b.persona.Valuables.StoreHold
		// The count runs from the look's last decision, at 5 s.
		wantHeld(t, "pending", b, at(6*time.Second))
		wantHeld(t, "within StoreHold", b, at(5*time.Second+hold-time.Second))
		runTrainerSteps(t, b, openingSteps(5*time.Second+hold, []storeRow{{102, 10, 7}}, skinCand))
	})
}

// testReleaseRows: the store lists released valuables first, by score at
// their floor, a row's units at most ceil(left / floor) of the valuables
// budget left, the listed value taken off left; the loot under the store
// budget follows.
func testReleaseRows(t *testing.T) {
	recipe := Item{ObjectID: 110, TemplateID: ledgerRecipe, Count: 5, SubCategory: 5}
	recipeCand := StoreItem{Type2: 5, ObjectID: 110, TemplateID: ledgerRecipe, Count: 5, Price: 366000, ReferencePrice: 366000}
	swordCand104 := swordCand101
	swordCand104.ObjectID = 104
	for _, tt := range []struct {
		name      string
		items     []Item
		cands     []StoreItem
		left      int64
		storeSold bool
		want      func(b *Brain) []storeRow
	}{
		{
			name: "the store budget spent: the valuables only", items: []Item{swordUnit(101, 0), skinLoot}, cands: []StoreItem{skinCand, swordCand101},
			left: 2000000, storeSold: true,
			want: func(b *Brain) []storeRow { return []storeRow{{101, 1, int32(b.valuableFloor(swordKey))}} },
		},
		{
			name: "by score, then the loot", items: []Item{swordUnit(101, 0), carriedOf(storedStaff), skinLoot}, cands: []StoreItem{skinCand, swordCand101, staffCand},
			left: 2000000,
			want: func(b *Brain) []storeRow {
				return []storeRow{{201, 1, int32(b.valuableFloor(staffKey5))}, {101, 1, int32(b.valuableFloor(swordKey))}, {102, 10, 7}}
			},
		},
		{
			name: "left for two of three rows", items: []Item{swordUnit(101, 0), swordUnit(103, 0), swordUnit(104, 0)},
			cands: []StoreItem{swordCand101, {ObjectID: 103, TemplateID: ledgerSword, Count: 1, BodySlot: partRHand}, swordCand104}, left: 200000,
			want: func(b *Brain) []storeRow {
				f := int32(b.valuableFloor(swordKey))
				return []storeRow{{101, 1, f}, {103, 1, f}}
			},
		},
		{
			name: "a row above left", items: []Item{swordUnit(101, 0)}, cands: []StoreItem{swordCand101}, left: 1,
			want: func(b *Brain) []storeRow { return []storeRow{{101, 1, int32(b.valuableFloor(swordKey))}} },
		},
		{name: "left spent", items: []Item{swordUnit(101, 0)}, cands: []StoreItem{swordCand101}, left: 0},
		{
			name: "a stack: ceil(left / floor) units", items: []Item{recipe}, cands: []StoreItem{recipeCand}, left: 800000,
			want: func(b *Brain) []storeRow {
				return []storeRow{{110, 3, int32(b.valuableFloor(valuableKey{template: ledgerRecipe}))}}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newRoundBrain(t, nearKeeper, nil, tt.items...)
			b.cfg.Travel.prices[animalSkin] = lootPrices[animalSkin]
			for k := range b.cfg.valuables.entries {
				release(t, b, k)
			}
			b.valuablesSold = b.persona.Valuables.Budget - tt.left
			if tt.storeSold {
				b.store.sold = b.persona.Store.Budget
			}
			var want []storeRow
			if tt.want != nil {
				want = tt.want(b)
			}
			if ready := b.storeReady(t0); ready != (want != nil) {
				t.Errorf("storeReady %v, want %v", ready, want != nil)
			}
			if got := listRows(b, tt.cands...); !reflect.DeepEqual(got, want) {
				t.Errorf("rows %v, want %v", got, want)
			}
		})
	}
}

// TestValuablesStoreSplit checks the store's sales split between the
// valuables and the store budget, and the after-round replies naming a
// released valuable (docs/valuables/release.md "Tests").
func TestValuablesStoreSplit(t *testing.T) {
	t.Run("split", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0), skinLoot, swordLoot)
		b.cfg.Travel.prices[animalSkin], b.cfg.Travel.prices[stoneSword] = lootPrices[animalSkin], lootPrices[stoneSword]
		release(t, b, swordKey)
		floor := b.valuableFloor(swordKey)
		rows := listRows(b, skinCand, swordCand101, swordCand)
		if want := []storeRow{{101, 1, int32(floor)}, {102, 10, 7}, {107, 1, 700}}; !reflect.DeepEqual(rows, want) {
			t.Fatalf("rows %v, want %v", rows, want)
		}
		b.storeSent(Action{kind: actStoreList}, t0)
		b.store.step = storeOpen
		paid := int32(1000 + floor + 700)
		if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{ItemModified, adenaStack(paid)}, {ItemRemoved, swordUnit(101, 0)}, {ItemRemoved, swordLoot}}}}, at(time.Second)); err != nil {
			t.Fatal(err)
		}
		b.closeStore(at(time.Second))
		if b.valuablesSold != floor || b.store.sold != 700 {
			t.Errorf("valuables sold %d store sold %d, want %d and 700", b.valuablesSold, b.store.sold, floor)
		}
		b.syncValuables()
		if _, ok := b.cfg.valuables.entries[swordKey]; ok || b.cfg.valuables.rows(b.cfg.Travel.stackable) != 0 {
			t.Errorf("entries %v, want the sold sword out of the ledger", b.cfg.valuables.entries)
		}
	})
	t.Run("a stack listed in part", func(t *testing.T) {
		recipe := Item{ObjectID: 110, TemplateID: ledgerRecipe, Count: 5, SubCategory: 5}
		recipeCand := StoreItem{Type2: 5, ObjectID: 110, TemplateID: ledgerRecipe, Count: 5, Price: 366000, ReferencePrice: 366000}
		b := newRoundBrain(t, nearKeeper, nil, recipe)
		k := valuableKey{template: ledgerRecipe}
		release(t, b, k)
		floor := b.valuableFloor(k)
		b.valuablesSold = b.persona.Valuables.Budget - 2*floor - 1
		if rows := listRows(b, recipeCand); len(rows) != 1 || rows[0].count != 3 {
			t.Fatalf("rows %v, want 3 of the 5 recipes", rows)
		}
		before := b.valuablesSold
		b.storeSent(Action{kind: actStoreList}, t0)
		b.store.step = storeOpen
		recipe.Count = 4
		if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{ItemModified, adenaStack(int32(1000 + floor))}, {ItemModified, recipe}}}}, at(time.Second)); err != nil {
			t.Fatal(err)
		}
		b.closeStore(at(time.Second))
		if b.valuablesSold-before != floor || b.store.sold != 0 {
			t.Errorf("valuables sold %d store sold %d, want %d and 0", b.valuablesSold-before, b.store.sold, floor)
		}
	})
	t.Run("replies", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff}, swordUnit(101, 0))
		release(t, b, swordKey)
		release(t, b, staffKey5)
		b.cfg.valuables.talk.ended = t0
		now := at(time.Minute)
		if got := replyTo(t, b, pcID, "Alden", "sword of watershadow?", now); got != "Sword of Watershadow not now, wts again soon" {
			t.Errorf("carried, no store: reply %q", got)
		}
		listRows(b, swordCand101)
		b.store.step = storeOpen
		b.world.Self.StoreType = storeTypeSell
		want := "Sword of Watershadow in my store " + priceText(b.valuableFloor(swordKey))
		if got := replyTo(t, b, pcID+1, "Bran", "sword of watershadow?", now.Add(time.Minute)); got != want {
			t.Errorf("listed: reply %q, want %q", got, want)
		}
		if got := replyTo(t, b, pcID+2, "Cid", "conjuror's staff?", now.Add(2*time.Minute)); got != "+5 Conjuror's Staff (D) not now, wts again soon" && !strings.HasSuffix(got, "Staff not now, wts again soon") {
			t.Errorf("stored: reply %q, want not now", got)
		}
	})
}
