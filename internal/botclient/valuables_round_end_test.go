package botclient

import (
	"slices"
	"testing"
	"time"
)

// TestValuablesRoundNoOpen: with no open due at the round's end (a retry
// time or a backoff still running) the round ends with its valuables
// carried: no keeper visit and no wait, rounds and asks stepped
// (docs/valuables/round-end.md "Steps" 5).
func TestValuablesRoundNoOpen(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(b *Brain, end time.Time)
	}{
		{"retry time", func(b *Brain, end time.Time) { b.wh.retryAt = end.Add(time.Minute) }},
		{"backoff", func(b *Brain, end time.Time) { b.wh.openedAt, b.wh.timeouts = end.Add(-time.Minute), 1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newRoundBrain(t, nearKeeper, nil, swordUnit(101, 0))
			firstLine(t, b)
			ask := entryOf(t, b, ledgerSword, 0).ask
			end := at(b.persona.Valuables.RoundWait)
			tt.setup(b, end)
			if b.wh.openDue(end) && !end.Before(b.wh.retryAt) {
				t.Fatal("fixture: an open is due at the end")
			}
			if rule, a := b.decide(end); rule == "valuables" || a.kind == actSelect || a.kind == actTalk {
				t.Fatalf("no open due: rule %q action %d, want the rule let go with no keeper visit", rule, a.kind)
			}
			wantNoRound(t, "no open due", b)
			if e := entryOf(t, b, ledgerSword, 0); e.carried != 1 || e.stored != 0 || e.rounds != 1 || e.ask >= ask {
				t.Errorf("sword %+v, want carried, rounds 1 and the ask below %d", e, ask)
			}
		})
	}
}

// newMarksBrain is newRoundBrain with the keepers Rant, 200 east, and,
// with rolfe, Rolfe 300 east, both in sight and within RoundReach.
func newMarksBrain(t *testing.T, rolfe bool, stored []WarehouseItem, items ...Item) *Brain {
	t.Helper()
	b := newRoundBrain(t, Point{X: 100000}, stored, items...)
	m := b.cfg.Travel
	m.spawns, m.keepers = map[int32][]Point{rantNPC: {{X: 200}}}, map[int32]bool{rantNPC: true}
	b.world.Apply(NPCInfo{ObjectID: rantObj, NPCID: rantNPC, Pos: Point{X: 200}}, t0)
	if rolfe {
		m.spawns[rolfeNPC], m.keepers[rolfeNPC] = []Point{{X: 300}}, true
		b.world.Apply(NPCInfo{ObjectID: rolfeObj, NPCID: rolfeNPC, Pos: Point{X: 300}}, t0)
	}
	return b
}

// markSteps are a visit to Rant from start up to its first page coming
// back: select, talk, the chat link, the freight page. The first page again
// marks Rant.
func markSteps(start time.Duration) []trainerStep {
	return []trainerStep{
		{name: "select Rant", at: start, wantRule: "valuables", wantKind: actSelect},
		{name: "talk", at: start + time.Second, events: []Event{MyTargetSelected{ObjectID: rantObj}}, wantRule: "valuables", wantKind: actTalk},
		{name: "chat link", at: start + 2*time.Second, events: []Event{keeperPage(rantObj, "Chat 1")}, wantRule: "valuables", wantKind: actBypass, wantPayload: chatBypass(rantObj, "1")},
		{name: "freight page: talk again", at: start + 3*time.Second, events: []Event{keeperPage(rantObj, "DepositF", "WithdrawF", "Chat 0")}, wantRule: "valuables", wantKind: actTalk},
	}
}

// TestValuablesRoundMarks: a keeper whose pages link no private warehouse
// is marked, with no open sent, and the same decision queues the round's
// request again at the next unmarked keeper within RoundReach of the
// anchor, with the same rows; with none left the walk back follows
// (docs/valuables/round.md "Back to the anchor").
func TestValuablesRoundMarks(t *testing.T) {
	staffRows := []WarehouseRow{{TemplateID: ledgerStaff, Count: 1, Object: 201}}
	swordRows := []WarehouseRow{{TemplateID: ledgerSword, Count: 1, Object: 101}}
	for _, tt := range []struct {
		name    string
		deposit bool
		rolfe   bool
	}{
		{"withdrawal: Rolfe next", false, true},
		{"withdrawal: none left", false, false},
		{"deposit: Rolfe next", true, true},
		{"deposit: none left", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stored []WarehouseItem
			if !tt.deposit {
				stored = []WarehouseItem{storedStaff}
			}
			b := newMarksBrain(t, tt.rolfe, stored, swordUnit(101, 0))
			var start time.Duration
			if tt.deposit {
				firstLine(t, b)
				start = b.persona.Valuables.RoundWait
			}
			line := roundLine(t, b)
			runTrainerSteps(t, b, markSteps(start))
			rule, a := tradeTurn(t, b, at(start+4*time.Second), keeperPage(rantObj, "Chat 1"))
			if !b.cfg.valuables.noPrivate[rantNPC] {
				t.Fatalf("marks %v, want Rant marked", b.cfg.valuables.noPrivate)
			}
			switch {
			case tt.rolfe:
				wantTurn(t, "marked: Rolfe at once", rule, a, "valuables", actSelect, nil)
				got, want, phase := b.wh.req.Withdraw, staffRows, roundWithdraw
				if tt.deposit {
					got, want, phase = b.wh.req.Deposit, swordRows, roundDeposit
				}
				if b.wh.v.npcID != rolfeNPC || !slices.Equal(got, want) || b.round.phase != phase {
					t.Errorf("request %+v at keeper %d phase %d; want rows %+v at Rolfe in phase %d", b.wh.req, b.wh.v.npcID, b.round.phase, want, phase)
				}
			case !tt.deposit:
				wantTurn(t, "marked, none left: back at the anchor, the first line", rule, a, "valuables", actSay, line)
				if e := entryOf(t, b, ledgerStaff, 5); b.wh.running || e.stored != 1 {
					t.Errorf("errand running %v staff %+v; want no request and the staff stored", b.wh.running, e)
				}
			default:
				if rule == "valuables" {
					t.Fatalf("marked, none left: rule %q action %d, want the walk back done and the rule let go", rule, a.kind)
				}
				wantNoRound(t, "none left", b)
				if e := entryOf(t, b, ledgerSword, 0); e.carried != 1 {
					t.Errorf("sword %+v, want carried", e)
				}
			}
		})
	}
}

// TestValuablesRoundSessions checks the round across sessions
// (docs/valuables/round.md "Tests"): after a reconnect the look comes
// before any round or deposit, and a new valuable is deposited after its
// round and counted stored; with the gear errand wearing the stored staff
// next, the withdrawal takes it (the row test reads no carry) and once
// carried it leaves the ledger and the line (docs/valuables/ledger.md).
func TestValuablesRoundSessions(t *testing.T) {
	t.Run("reconnect", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, nil)
		c := nextSession(t, b, map[int32][]Point{keeperNPC: {nearKeeper}}, map[int32]NPCInfo{keeperObj: {NPCID: keeperNPC, Pos: nearKeeper}})
		roundTown(c)
		c.world.Apply(ItemList{Items: []Item{adenaStack(1000), swordUnit(101, 0)}}, t0)
		runTrainerSteps(t, c, lookSteps(0))
		if c.cfg.valuables.round.open || !c.wh.req.Look {
			t.Fatalf("round open %v request %+v; want the look first", c.cfg.valuables.round.open, c.wh.req)
		}
		// The round is pending on the open: the rule holds the turn with a
		// wait (StoreHold, V7b).
		if rule, a := tradeTurn(t, c, at(5*time.Second), privateList()); rule != "valuables" || a.kind != actHold || c.cfg.valuables.round.open {
			t.Fatalf("look listed: rule %q action %d, want the look done and no round, the rule waiting", rule, a.kind)
		}
		due := 3*time.Second + warehouseOpenGap
		if rule, a := c.decide(at(due - time.Second)); a.kind != actHold || c.cfg.valuables.round.open {
			t.Fatalf("open not due: rule %q action %d round open %v; want no round", rule, a.kind, c.cfg.valuables.round.open)
		}
		line := roundLine(t, c)
		rule, a := tradeTurn(t, c, at(due))
		wantTurn(t, "open due: the first line", rule, a, "valuables", actSay, line)
		end := due + c.persona.Valuables.RoundWait
		steps := depositSteps(end, swordDepositList, swordDepositRows)
		// The look's keeper target dropped meanwhile: the deposit selects it.
		steps[0].setup = func(b *Brain) { b.world.Target = 0 }
		runTrainerSteps(t, c, steps)
		tradeTurn(t, c, at(end+6*time.Second), InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: swordUnit(101, 0)}, {State: ItemModified, Item: adenaStack(970)}}})
		if e := entryOf(t, c, ledgerSword, 0); e.stored != 1 || e.carried != 0 {
			t.Errorf("sword %+v, want counted stored", e)
		}
	})
	t.Run("the gear errand wears the staff next", func(t *testing.T) {
		b := newRoundBrain(t, nearKeeper, []WarehouseItem{storedStaff, storedSword}, swordUnit(101, 0))
		b.world.Self.ClassID, b.persona.Gear.Equip, b.persona.Gear.Classes = classHumanFight, true, defaultGearClasses()
		class := b.gearClass()
		if len(class.Weapons) == 0 {
			t.Fatal("fixture: the class wields nothing")
		}
		b.cfg.Travel.gear = map[int32]gearItem{ledgerStaff: {part: partRHand, kind: class.Weapons[0]}}
		if !b.wearsNext(carriedOf(storedStaff)) {
			t.Fatal("fixture: the gear errand does not wear the staff next")
		}
		runTrainerSteps(t, b, append(keeperSteps(0, "WithdrawP"), trainerStep{
			name: "withdraw both", at: 5 * time.Second, events: []Event{privateList(storedStaff, storedSword)}, wantRule: "valuables", wantKind: actWarehouseWithdraw,
			wantPayload: encodeWarehouseRequest(opGameSendWareHouseWithdrawList, []warehouseRow{{object: 201, item: ledgerStaff, count: 1}, {object: 202, item: ledgerSword, count: 1}}),
		}))
		now := at(6 * time.Second)
		if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: carriedOf(storedStaff)}, {State: ItemAdded, Item: carriedOf(storedSword)}}}}, now); err != nil {
			t.Fatal(err)
		}
		line := roundLine(t, b)
		rule, a := tradeTurn(t, b, now)
		wantTurn(t, "the first line: the two swords", rule, a, "valuables", actSay, line)
		if e, ok := b.cfg.valuables.entries[staffKey5]; ok && (e.carried > 0 || e.stored > 0) {
			t.Errorf("staff %+v, want it out of the ledger", e)
		}
		if keys, rows := b.adEntries(), b.carriedRows(); !slices.Equal(keys, []valuableKey{swordKey}) || len(rows) != 2 {
			t.Errorf("line entries %v deposit rows %+v; want the two swords only", keys, rows)
		}
	})
}
