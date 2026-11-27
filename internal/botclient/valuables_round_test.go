package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The round tests' stored valuables: a +5 Conjuror's Staff and a second
// Sword of Watershadow.
var (
	storedStaff = WarehouseItem{ObjectID: 201, TemplateID: ledgerStaff, Count: 1, Enchant: 5}
	storedSword = WarehouseItem{ObjectID: 202, TemplateID: ledgerSword, Count: 1}
	staffKey5   = valuableKey{template: ledgerStaff, enchant: 5}
)

// carriedOf is stored row w as the item the withdrawal hands the bot.
func carriedOf(w WarehouseItem) Item {
	return Item{ObjectID: w.ObjectID, TemplateID: w.TemplateID, Count: w.Count, BodySlot: partRHand, Enchant: w.Enchant}
}

// newRoundBrain is a merchant at the origin, the registry's town point in
// restart region 0 with its ad board, the keeper spawned at keeper (in
// sight within 300), its session's look done with stored in its private
// warehouse, carrying 1000 adena and items.
func newRoundBrain(t *testing.T, keeper Point, stored []WarehouseItem, items ...Item) *Brain {
	t.Helper()
	b := walkOn(t, newLedgerBrain(t, append([]Item{adenaStack(1000)}, items...)...))
	m := b.cfg.Travel
	m.spawns, m.keepers = map[int32][]Point{keeperNPC: {keeper}}, map[int32]bool{keeperNPC: true}
	if horizontal2(keeper, Point{}) <= 300*300 {
		b.world.Apply(NPCInfo{ObjectID: keeperObj, NPCID: keeperNPC, Pos: keeper}, t0)
	}
	roundTown(b)
	b.wh.private = WarehouseStatus{Known: true, Contents: slices.Clone(stored)}
	b.syncValuables(t0)
	return b
}

// roundTown makes the origin's town an advertising one: restart region 0
// with its ad board, the fixture's valuables named.
func roundTown(b *Brain) {
	m := b.cfg.Travel
	m.regions = map[datapack.MapTile]int{originTile: 0}
	m.itemNames = map[int32]string{ledgerSword: "Sword of Watershadow", ledgerStaff: "Conjuror's Staff"}
	b.cfg.Ads = NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour)
}

// nearKeeper is the round tests' keeper, in sight 300 east of the anchor.
var nearKeeper = Point{X: 300}

// keeperSteps are rule "valuables"'s visit to the keeper from start:
// select, talk, open the private link cmd, wait for its list.
func keeperSteps(start time.Duration, cmd string) []trainerStep {
	steps := lookSteps(start)
	steps[3].wantPayload = botwire.EncodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_" + cmd)
	steps[4].name = "wait for the " + cmd + " list"
	return steps
}

// roundLine is the Trade line naming every carried entry, in the ads'
// order.
func roundLine(t *testing.T, b *Brain) []byte {
	t.Helper()
	b.syncValuables(t0)
	return botwire.EncodeSay2(adLine(t, b, b.adEntries()...), chatTrade)
}

// firstLine has b start its round at t0 with nothing stored and say its
// first line.
func firstLine(t *testing.T, b *Brain) {
	t.Helper()
	line := roundLine(t, b)
	rule, a := tradeTurn(t, b, t0)
	wantTurn(t, "first line", rule, a, "valuables", actSay, line)
}

// depositSteps are the deposit's visit from start: the keeper's steps,
// then the type 1 list and the deposit request of rows.
func depositSteps(start time.Duration, list []WarehouseItem, rows []warehouseRow) []trainerStep {
	return append(keeperSteps(start, "DepositP"), trainerStep{
		name: "deposit", at: start + 5*time.Second, events: []Event{WarehouseDepositList{Type: privateWarehouseType, Adena: 1000, Items: list}},
		wantRule: "valuables", wantKind: actWarehouseDeposit, wantPayload: botwire.EncodeWarehouseRequest(opGameSendWareHouseDepositList, rows),
	})
}

// swordDeposit is the deposit of the carried sword 101.
var (
	swordDepositList = []WarehouseItem{{ObjectID: 101, TemplateID: ledgerSword, Count: 1}}
	swordDepositRows = []warehouseRow{{Object: 101, Item: ledgerSword, Count: 1}}
)

// wantNoRound checks the round is closed and none of its visits runs.
func wantNoRound(t *testing.T, name string, b *Brain) {
	t.Helper()
	if b.cfg.valuables.round.open || b.round.phase != roundIdle || b.wh.running {
		t.Fatalf("%s: round open %v phase %d errand running %v; want no round", name, b.cfg.valuables.round.open, b.round.phase, b.wh.running)
	}
}

// TestValuablesRoundStart checks when a round starts
// (docs/valuables/round.md "Start", "Tests"), the look already done.
func TestValuablesRoundStart(t *testing.T) {
	t.Run("starts", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		r := b.cfg.valuables.round
		if !r.open || r.anchor != (Point{}) || !b.cfg.valuables.lastStart.Equal(t0) || b.wh.running {
			t.Fatalf("round %+v last start %v errand running %v; want open at the origin since t0, no withdrawal", r, b.cfg.valuables.lastStart, b.wh.running)
		}
	})
	t.Run("a full town within storeTownRadius", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		b.cfg.Stores = NewStoreRegistry([]Point{{X: storeTownRadius - 100}}, 1, 150)
		if _, ok := b.cfg.Stores.claim(Point{X: storeTownRadius}); !ok || b.cfg.Stores.hasRoom(Point{}) {
			t.Fatal("fixture: the town is not full")
		}
		firstLine(t, b)
	})
	t.Run("none", testRoundNone)
	t.Run("unreachable walk back", testRoundUnreachable)
	t.Run("stale round", testRoundStale)
}

// testRoundNone: no round while a start condition fails; one once it
// holds where it is a wait.
func testRoundNone(t *testing.T) {
	p := DefaultValuablePolicy()
	for _, tt := range []struct {
		name  string
		setup func(b *Brain)
		// due, when set, is when the round starts after all.
		due time.Duration
	}{
		{"within RoundGap", func(b *Brain) { b.cfg.valuables.lastStart = t0.Add(time.Second - p.RoundGap) }, time.Second},
		{"store open", func(b *Brain) { b.world.Self.StoreType = 1 }, 0},
		{"out of a registry town", func(b *Brain) { b.cfg.Stores = NewStoreRegistry([]Point{{X: 100000}}, 5, 150) }, 0},
		{"region -1", func(b *Brain) { b.cfg.Travel.regions = nil }, 0},
		{"board: a line within the region gap", func(b *Brain) { b.cfg.Ads.Claim(0, t0.Add(-30*time.Second)) }, 30 * time.Second},
		{"board: the hour cap", func(b *Brain) {
			b.cfg.Ads = NewAdBoard(DefaultAdRegionGap, 1)
			b.cfg.Ads.Claim(1, t0.Add(-time.Minute))
		}, 59 * time.Minute},
		{"gear UseItem", func(b *Brain) { b.gear.obj = 999 }, 0},
		{"enchant exchange", func(b *Brain) { b.ench.step = enchantChosen }, 0},
		{"clan visit unsettled", func(b *Brain) { b.member.asked = true }, 0},
		{"open gap after a clan open", func(b *Brain) { b.wh.openedAt = t0.Add(-30 * time.Second) }, 30 * time.Second},
		{"backoff", func(b *Brain) { b.wh.openedAt, b.wh.timeouts = t0.Add(-time.Minute), 1 }, time.Minute},
		{"retry time", func(b *Brain) { b.wh.retryAt = t0.Add(10 * time.Second) }, 10 * time.Second},
		{"budget spent", func(b *Brain) { b.valuablesSold = b.persona.Valuables.Budget }, 0},
		{"no unmarked private keeper", func(b *Brain) { b.cfg.valuables.noPrivate[keeperNPC] = true }, 0},
		{"look not done", func(b *Brain) { b.wh.private = WarehouseStatus{} }, 0},
		{"stored ones alone", func(b *Brain) {
			b.world.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: swordUnit(101, 0)}}}, t0)
			b.wh.private.Contents = []WarehouseItem{storedSword}
		}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
			tt.setup(b)
			if rule, a := b.decide(t0); a.kind == actSay || b.cfg.valuables.round.open {
				t.Fatalf("decide = rule %q action %d, round open %v; want no round", rule, a.kind, b.cfg.valuables.round.open)
			}
			if tt.due == 0 {
				return
			}
			line := roundLine(t, b)
			rule, a := tradeTurn(t, b, at(tt.due))
			wantTurn(t, "once due", rule, a, "valuables", actSay, line)
		})
	}
}

// testRoundUnreachable: no route back to the anchor after the withdrawal
// ends the round with no ad line or deposit, the units carried.
func testRoundUnreachable(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff}, swordUnit(101, 0))
	runTrainerSteps(t, b, append(keeperSteps(0, "WithdrawP"), trainerStep{
		name: "withdraw", at: 5 * time.Second, events: []Event{privateList(storedStaff)}, wantRule: "valuables", wantKind: actWarehouseWithdraw,
		wantPayload: botwire.EncodeWarehouseRequest(opGameSendWareHouseWithdrawList, []warehouseRow{{Object: 201, Item: ledgerStaff, Count: 1}}),
	}))
	b.walk.nav = nil
	rule, a := tradeTurn(t, b, at(6*time.Second), InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: carriedOf(storedStaff)}}}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 1000}})
	if rule == "valuables" || a.kind == actSay {
		t.Fatalf("no route back: rule %q action %d, want the round ended", rule, a.kind)
	}
	wantNoRound(t, "no route back", b)
	if e := entryOf(t, b, ledgerStaff, 5); e.carried != 1 || e.rounds != 0 {
		t.Errorf("staff %+v, want carried, no round counted", e)
	}
}

// testRoundStale: a round open when its session ends is ended by the next
// session's first decision as a cut-off, counted after its first line.
func testRoundStale(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
	firstLine(t, b)
	before := entryOf(t, b, ledgerSword, 0)
	c := nextSession(t, b, map[int32][]Point{keeperNPC: {nearKeeper}}, nil)
	c.world.Apply(ItemList{Items: []Item{adenaStack(1000), swordUnit(101, 0)}}, t0)
	rule, a := c.decide(at(time.Minute))
	if rule == "valuables" && a.kind == actSay || c.cfg.valuables.round.open {
		t.Fatalf("next session: rule %q action %d round open %v; want the round ended", rule, a.kind, c.cfg.valuables.round.open)
	}
	p := c.persona.Valuables
	ref := c.cfg.Travel.valueScore(p, ledgerSword, 0).ref
	if e := entryOf(t, c, ledgerSword, 0); e.rounds != 1 || e.ask != p.lowered(before.ask, ref, c.valuableFloor(swordKey)) {
		t.Errorf("sword %+v, want rounds 1 and the ask a RoundStep below %d", e, before.ask)
	}
}

// TestValuablesReadvertise: a new valuable with two stored withdraws them
// first, names all three in one line, and steps their asks after the
// round (docs/valuables/round-end.md "Tests").
func TestValuablesReadvertise(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff, storedSword}, swordUnit(101, 0))
	staffAsk, swordAsk := entryOf(t, b, ledgerStaff, 5).ask, entryOf(t, b, ledgerSword, 0).ask
	runTrainerSteps(t, b, append(keeperSteps(0, "WithdrawP"), trainerStep{
		name: "withdraw both", at: 5 * time.Second, events: []Event{privateList(storedStaff, storedSword)}, wantRule: "valuables", wantKind: actWarehouseWithdraw,
		wantPayload: botwire.EncodeWarehouseRequest(opGameSendWareHouseWithdrawList, []warehouseRow{{Object: 201, Item: ledgerStaff, Count: 1}, {Object: 202, Item: ledgerSword, Count: 1}}),
	}))
	now := at(6 * time.Second)
	if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: carriedOf(storedStaff)}, {State: ItemAdded, Item: carriedOf(storedSword)}}}}, now); err != nil {
		t.Fatal(err)
	}
	line := roundLine(t, b)
	if keys := b.adEntries(); len(keys) != 2 || entryOf(t, b, ledgerSword, 0).carried != 2 || entryOf(t, b, ledgerStaff, 5).carried != 1 {
		t.Fatalf("entries %v, want the two swords and the staff carried", b.cfg.valuables.entries)
	}
	rule, a := tradeTurn(t, b, now)
	wantTurn(t, "one line naming all three", rule, a, "valuables", actSay, line)

	end := now.Add(b.persona.Valuables.RoundWait)
	rule, a = tradeTurn(t, b, end)
	wantTurn(t, "deposit after the round: the keeper still selected", rule, a, "valuables", actTalk, nil)
	p := b.persona.Valuables
	for _, c := range []struct {
		k      valuableKey
		before int64
	}{{staffKey5, staffAsk}, {swordKey, swordAsk}} {
		ref := b.cfg.Travel.valueScore(p, c.k.template, int(c.k.enchant)).ref
		if e := entryOf(t, b, c.k.template, c.k.enchant); e.rounds != 1 || e.ask != p.lowered(c.before, ref, b.valuableFloor(c.k)) {
			t.Errorf("%v: %+v, want rounds 1 and the ask a RoundStep below %d", c.k, e, c.before)
		}
	}
	if len(b.wh.req.Deposit) != 3 {
		t.Errorf("deposit %+v, want the three units", b.wh.req.Deposit)
	}
}

// TestValuablesFirstLineWait: the board holding the first line past
// FirstLineWait after the withdrawal's walk back ends the round with no
// Trade line, deposits, and steps no rounds or asks.
func TestValuablesFirstLineWait(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff}, swordUnit(101, 0))
	b.cfg.Ads = NewAdBoard(DefaultAdRegionGap, 1)
	runTrainerSteps(t, b, append(keeperSteps(0, "WithdrawP"), trainerStep{
		name: "withdraw", at: 5 * time.Second, events: []Event{privateList(storedStaff)}, wantRule: "valuables", wantKind: actWarehouseWithdraw,
	}))
	if !b.cfg.Ads.Claim(1, at(5*time.Second)) {
		t.Fatal("another region's line not claimed")
	}
	staffAsk, swordAsk := entryOf(t, b, ledgerStaff, 5).ask, entryOf(t, b, ledgerSword, 0).ask
	back := 6 * time.Second
	wait := b.persona.Valuables.FirstLineWait
	runTrainerSteps(t, b, []trainerStep{
		{name: "back at the anchor: board held", at: back, events: []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: carriedOf(storedStaff)}}}}, wantRule: "valuables"},
		{name: "still held", at: back + wait - time.Second, wantRule: "valuables"},
		{name: "FirstLineWait: the deposit, the keeper still selected", at: back + wait, wantRule: "valuables", wantKind: actTalk},
	})
	if b.cfg.valuables.round.open || len(b.wh.req.Deposit) != 2 {
		t.Fatalf("round open %v deposit %+v; want the round ended and both units deposited", b.cfg.valuables.round.open, b.wh.req.Deposit)
	}
	if s, w := entryOf(t, b, ledgerStaff, 5), entryOf(t, b, ledgerSword, 0); s.rounds != 0 || w.rounds != 0 || s.ask != staffAsk || w.ask != swordAsk {
		t.Errorf("staff %+v sword %+v; want rounds and asks unchanged", s, w)
	}
}

// TestValuablesHoldsTurn: a round holds the turn, steps its own deposit,
// and a follow cuts it off (docs/valuables/round-end.md "Cut-off").
func TestValuablesHoldsTurn(t *testing.T) {
	follow := func(b *Brain) { b.orders.follow = pcID }
	t.Run("holds the turn", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		b.persona.Warehouse.Want = func(*World, WarehouseStatus, time.Time) WarehouseRequest {
			return WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: adenaID, Count: 100}}}
		}
		b.world.Clan = Clan{ID: 1}
		firstLine(t, b)
		runTrainerSteps(t, b, []trainerStep{{name: "between lines", at: 30 * time.Second, wantRule: "valuables"}})
		if b.wh.running {
			t.Fatal("clan warehouse errand started in the round")
		}
	})
	t.Run("follow during the withdrawal", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff}, swordUnit(101, 0))
		runTrainerSteps(t, b, keeperSteps(0, "WithdrawP")[:2])
		staffAsk := entryOf(t, b, ledgerStaff, 5).ask
		follow(b)
		if rule, _ := b.decide(at(3 * time.Second)); rule == "valuables" {
			t.Fatal("rule valuables matched while following")
		}
		wantNoRound(t, "follow", b)
		if e := entryOf(t, b, ledgerStaff, 5); e.stored != 1 || e.rounds != 0 || e.ask != staffAsk || !b.wh.retryAt.IsZero() {
			t.Errorf("staff %+v retry %v; want stored, no round counted, no retry time", e, b.wh.retryAt)
		}
	})
	t.Run("follow after the first line", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		ask := entryOf(t, b, ledgerSword, 0).ask
		follow(b)
		if rule, _ := b.decide(at(10 * time.Second)); rule == "valuables" {
			t.Fatal("rule valuables matched while following")
		}
		wantNoRound(t, "follow", b)
		if e := entryOf(t, b, ledgerSword, 0); e.rounds != 1 || e.ask >= ask || e.carried != 1 {
			t.Errorf("sword %+v, want carried, rounds 1 and the ask below %d", e, ask)
		}
	})
	t.Run("follow while the deposit walks to a far keeper", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			call func(b *Brain)
		}{
			{"a follow order", follow},
			{"the party leader moving off", func(b *Brain) {
				b.world.Party = Party{LeaderID: pcID, Members: map[int32]*PartyMate{pcID: {}}}
			}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				b := newRoundBrain(t, Point{X: 5348}, nil, swordUnit(101, 0))
				firstLine(t, b)
				end := at(b.persona.Valuables.RoundWait)
				rule, a := tradeTurn(t, b, end)
				wantTurn(t, "walk to the keeper", rule, a, "valuables", actMove, nil)
				tt.call(b)
				placePlayer(t, b, pcID, Point{X: -1500})
				if rule, _ := b.decide(end.Add(time.Second)); rule != "follow" {
					t.Fatalf("called away: rule %q, want rule follow's legs", rule)
				}
				wantNoRound(t, "follow", b)
				if e := entryOf(t, b, ledgerSword, 0); e.carried != 1 || !b.wh.retryAt.IsZero() || b.wh.private.Deposit.Outcome != WarehouseNone {
					t.Errorf("sword %+v retry %v deposit %+v; want carried, no retry time, no result", e, b.wh.retryAt, b.wh.private.Deposit)
				}
			})
		}
	})
	t.Run("follow at the list", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		end := b.persona.Valuables.RoundWait
		runTrainerSteps(t, b, keeperSteps(end, "DepositP"))
		now := at(end + 5*time.Second)
		if err := b.handle([]Event{WarehouseDepositList{Type: privateWarehouseType, Adena: 1000, Items: swordDepositList}}, now); err != nil {
			t.Fatal(err)
		}
		follow(b)
		if rule, a := b.decide(now); rule == "valuables" || a.kind == actWarehouseDeposit {
			t.Fatalf("follow at the list: rule %q action %d, want no deposit request", rule, a.kind)
		}
		wantNoRound(t, "follow", b)
		if e := entryOf(t, b, ledgerSword, 0); e.carried != 1 || b.wh.private.Deposit.Outcome != WarehouseNone {
			t.Errorf("sword %+v deposit %+v; want carried, no result", e, b.wh.private.Deposit)
		}
	})
	t.Run("follow once the deposit went out", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			answer []Event
			after  time.Duration
			stored int64
		}{
			{"answered", []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: swordUnit(101, 0)}, {State: ItemModified, Item: adenaStack(970)}}}}, time.Second, 1},
			{"timed out", nil, warehouseReplyTimeout, 0},
		} {
			t.Run(tt.name, func(t *testing.T) {
				b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
				firstLine(t, b)
				end := b.persona.Valuables.RoundWait
				runTrainerSteps(t, b, depositSteps(end, swordDepositList, swordDepositRows))
				sent := end + 5*time.Second
				follow(b)
				runTrainerSteps(t, b, []trainerStep{{name: "held, no legs", at: sent + time.Second/2, wantRule: "valuables"}})
				if rule, _ := tradeTurn(t, b, at(sent+tt.after), tt.answer...); rule == "valuables" {
					t.Fatalf("after the answer: rule valuables, want it let go")
				}
				wantNoRound(t, "after the answer", b)
				if e := b.cfg.valuables.entries[swordKey]; e == nil || e.stored != tt.stored || e.carried != 1-tt.stored {
					t.Errorf("sword %+v, want %d stored", e, tt.stored)
				}
			})
		}
	})
}

// TestValuablesRoundDeposit: at RoundWait after the first line the round
// deposits its carried valuables at the keeper, 30 a row, walks back and
// lets the store open with the loot; an after-round whisper gets "not
// now".
func TestValuablesRoundDeposit(t *testing.T) {
	t.Run("deposit, walk back, store", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0), skinLoot)
		b.cfg.Travel.prices[animalSkin] = lootPrices[animalSkin]
		firstLine(t, b)
		end := b.persona.Valuables.RoundWait
		list := append(slices.Clone(swordDepositList), WarehouseItem{ObjectID: 102, TemplateID: animalSkin, Count: 10})
		runTrainerSteps(t, b, append(depositSteps(end, list, swordDepositRows), trainerStep{
			name: "deposited: walk back", at: end + 6*time.Second,
			events:   []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: swordUnit(101, 0)}, {State: ItemModified, Item: adenaStack(970)}}}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 250}}},
			wantRule: "valuables", wantKind: actMove, wantTo: Point{}, wantWithin: anchorArrive,
		}))
		if e := entryOf(t, b, ledgerSword, 0); e.stored != 1 || e.carried != 0 {
			t.Fatalf("sword %+v, want counted stored", e)
		}
		now := at(end + 10*time.Second)
		if got := replyTo(t, b, pcID, "Alden", "sword of watershadow?", now); got != "Sword of Watershadow not now, wts again soon" {
			t.Errorf("after-round whisper: reply %q", got)
		}
		runTrainerSteps(t, b, []trainerStep{{
			name: "at the anchor: the store", at: end + 30*time.Second, events: []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 50}}},
			wantRule: "store", wantKind: actStoreManage,
		}})
	})
	t.Run("open timeout", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		end := b.persona.Valuables.RoundWait
		runTrainerSteps(t, b, keeperSteps(end, "DepositP"))
		if rule, _ := tradeTurn(t, b, at(end+3*time.Second+warehouseReplyTimeout)); rule == "valuables" {
			t.Fatal("open timed out: rule valuables still holds")
		}
		wantNoRound(t, "timed out", b)
		if e := entryOf(t, b, ledgerSword, 0); e.carried != 1 || b.wh.private.Deposit.Outcome != WarehouseTimeout {
			t.Errorf("sword %+v deposit %+v; want carried, timed out", e, b.wh.private.Deposit)
		}
	})
}

// TestValuablesGrace: the round's end put off by an open trade or an
// agreed buyer in sight, ended at once by the budget, and the deposit
// waiting for pending trade requests (docs/valuables/round-end.md
// "Steps").
func TestValuablesGrace(t *testing.T) {
	agreed := func(b *Brain) {
		b.cfg.valuables.talk.deals = map[whisperDeal]whisperPrice{{pcID, swordKey}: {current: 150000, agreed: true}}
		placePlayer(t, b, pcID, Point{X: 600})
	}
	t.Run("agreed buyer in sight", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			events []Event
			after  time.Duration
		}{
			{"leaves sight", []Event{DeleteObject{ObjectID: pcID}}, 30 * time.Second},
			{"past Grace", nil, DefaultValuablePolicy().Grace},
		} {
			t.Run(tt.name, func(t *testing.T) {
				b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
				firstLine(t, b)
				agreed(b)
				end := b.persona.Valuables.RoundWait
				runTrainerSteps(t, b, []trainerStep{
					{name: "put off", at: end, wantRule: "valuables"},
					{name: "ends", at: end + tt.after, events: tt.events, wantRule: "valuables", wantKind: actSelect},
				})
			})
		}
	})
	t.Run("open trade", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		placePlayer(t, b, pcID, Point{X: 100})
		end := at(b.persona.Valuables.RoundWait)
		if err := b.handle([]Event{TradeStart{PartnerID: pcID}}, end.Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		b.decide(end)
		if !b.cfg.valuables.round.open || b.wh.running {
			t.Fatalf("trade open at RoundWait: round open %v errand running %v; want the end put off", b.cfg.valuables.round.open, b.wh.running)
		}
		rule, a := tradeTurn(t, b, end.Add(30*time.Second), SendTradeDone{})
		wantTurn(t, "trade over: the deposit", rule, a, "valuables", actSelect, nil)
	})
	t.Run("budget reached", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		b.valuablesSold = b.persona.Valuables.Budget
		rule, a := tradeTurn(t, b, at(time.Minute))
		wantTurn(t, "budget: the deposit at once", rule, a, "valuables", actSelect, nil)
		if e := entryOf(t, b, ledgerSword, 0); b.cfg.valuables.round.open || e.rounds != 1 {
			t.Errorf("round open %v sword %+v; want the round ended and counted", b.cfg.valuables.round.open, e)
		}
	})
	t.Run("own request pending", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			events []Event
			after  time.Duration
		}{
			{"refused", []Event{SystemMessage{ID: msgTradeDeclined}}, time.Second},
			{"expired", nil, 10 * time.Second},
		} {
			t.Run(tt.name, func(t *testing.T) {
				b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
				firstLine(t, b)
				end := b.persona.Valuables.RoundWait
				b.cfg.valuables.round.invite = valuableInvite{partner: pcID, sent: at(end - 5*time.Second)}
				runTrainerSteps(t, b, []trainerStep{
					{name: "ended: the request pending", at: end, wantRule: "valuables"},
					{name: "still pending", at: end + tt.after - time.Millisecond, wantRule: "valuables"},
					{name: "answered: the deposit", at: end + tt.after, events: tt.events, wantRule: "valuables", wantKind: actSelect},
				})
			})
		}
	})
	t.Run("a player's request", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		placePlayer(t, b, pcID, Point{X: 100})
		end := b.persona.Valuables.RoundWait
		rule, _ := tradeTurn(t, b, at(end), SendTradeRequest{SenderID: pcID})
		if rule != "prompt" || b.wh.running {
			t.Fatalf("request at the end: rule %q errand running %v; want rule prompt answering first", rule, b.wh.running)
		}
		rule, a := tradeTurn(t, b, at(end+time.Second))
		wantTurn(t, "answered: the deposit", rule, a, "valuables", actSelect, nil)
	})
	t.Run("a party invitation", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		end := b.persona.Valuables.RoundWait
		rule, _ := tradeTurn(t, b, at(end), AskJoinParty{Requester: "buyer"})
		if rule != "prompt" || b.wh.running {
			t.Fatalf("invitation at the end: rule %q errand running %v; want rule prompt answering first", rule, b.wh.running)
		}
		rule, a := tradeTurn(t, b, at(end+time.Second))
		wantTurn(t, "answered: the deposit", rule, a, "valuables", actSelect, nil)
	})
	t.Run("no request in the extension's last 15 s", func(t *testing.T) {
		grace := DefaultValuablePolicy().Grace
		for _, tt := range []struct {
			name  string
			after time.Duration
			ask   bool
		}{
			{"before", grace - tradeRequestTimeout - time.Millisecond, true},
			{"within", grace - tradeRequestTimeout, false},
		} {
			t.Run(tt.name, func(t *testing.T) {
				b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
				firstLine(t, b)
				agreed(b)
				end := at(b.persona.Valuables.RoundWait)
				rule, a := tradeTurn(t, b, end)
				wantTurn(t, "put off", rule, a, "valuables", actHold, nil)
				if !b.cfg.valuables.round.extended {
					t.Fatal("round not extended")
				}
				b.cfg.valuables.talk.to = map[int32]int{pcID: 1}
				rule, a = tradeTurn(t, b, end.Add(tt.after))
				if tt.ask {
					wantWalk(t, "legs to the whisperer", b, rule, a, Point{X: 600}, inviteArrive)
					return
				}
				wantTurn(t, "no request", rule, a, "valuables", actHold, nil)
				if b.cfg.valuables.round.invite.partner != 0 {
					t.Errorf("invite %+v, want none", b.cfg.valuables.round.invite)
				}
			})
		}
	})
	t.Run("142 at the open", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
		firstLine(t, b)
		end := b.persona.Valuables.RoundWait
		runTrainerSteps(t, b, keeperSteps(end, "DepositP"))
		if rule, _ := tradeTurn(t, b, at(end+5*time.Second), SystemMessage{ID: msgAlreadyTrading}); rule == "valuables" {
			t.Fatal("refused: rule valuables still holds")
		}
		wantNoRound(t, "refused", b)
		d := b.wh.private.Deposit
		if e := entryOf(t, b, ledgerSword, 0); e.carried != 1 || d.Outcome != WarehouseRefused || d.Msg != msgAlreadyTrading || len(b.wh.noPrivate) != 0 {
			t.Errorf("sword %+v deposit %+v marks %v; want carried, refused 142, keeper unmarked", e, d, b.wh.noPrivate)
		}
		if b.wh.openDue(at(end + 3*time.Second + warehouseOpenGap - time.Second)) {
			t.Error("next open due within 60 s of the refused one")
		}
	})
}

// TestValuablesFees checks valuableFees in the shop reserve: 30 a row for
// the larger of HoldRows and the carried valuable rows; none disabled.
func TestValuablesFees(t *testing.T) {
	b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
	if got, want := b.valuableFees(), int64(warehouseFee*b.persona.Valuables.HoldRows); got != want {
		t.Errorf("one row: fees %d, want HoldRows' %d", got, want)
	}
	b.persona.Valuables.HoldRows = 0
	if got := b.valuableFees(); got != warehouseFee {
		t.Errorf("HoldRows 0: fees %d, want one row's %d", got, warehouseFee)
	}
	b.persona.Valuables = ValuablePolicy{}
	if got := b.valuableFees(); got != 0 {
		t.Errorf("disabled: fees %d, want 0", got)
	}
}
