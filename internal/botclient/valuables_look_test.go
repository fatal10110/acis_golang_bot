package botclient

import (
	"reflect"
	"testing"
	"time"
)

// Templates of the look's process start rows: a quest item and a shadow
// item each worth a valuable's score, and one under ValueMin.
const (
	lookQuest  int32 = 7001
	lookShadow int32 = 7002
	lookCheap  int32 = 7003
)

// newLookBrain is newLedgerBrain on walkable ground at the origin, the
// registry's town point, with the keepers spawns and, in sight, the
// keepers sight (object id to template and position).
func newLookBrain(t *testing.T, spawns map[int32][]Point, sight map[int32]NPCInfo) *Brain {
	t.Helper()
	b := walkOn(t, newLedgerBrain(t))
	m := b.cfg.Travel
	m.spawns, m.keepers = spawns, map[int32]bool{}
	for npc := range spawns {
		m.keepers[npc] = true
	}
	for obj, info := range sight {
		info.ObjectID = obj
		b.world.Apply(info, t0)
		m.keepers[info.NPCID] = true
	}
	for id, price := range map[int32]int32{lookQuest: 500000, lookShadow: 500000, lookCheap: 20000} {
		m.prices[id], m.supply[id] = price, itemSupply{perHour: 1, safe: 3}
	}
	m.shadows = map[int32]bool{lookShadow: true}
	return b
}

// newKeeperLookBrain is newLookBrain with the warehouse tests' keeper,
// spawned and in sight 300 east of the bot.
func newKeeperLookBrain(t *testing.T) *Brain {
	t.Helper()
	return newLookBrain(t, map[int32][]Point{keeperNPC: {{X: 300}}}, map[int32]NPCInfo{keeperObj: {NPCID: keeperNPC, Pos: Point{X: 300}}})
}

// nextSession is a new session's brain of the bot b plays: the same
// keepers, the bot's valuables value handed in.
func nextSession(t *testing.T, b *Brain, spawns map[int32][]Point, sight map[int32]NPCInfo) *Brain {
	t.Helper()
	c := newLookBrain(t, spawns, sight)
	c.cfg.valuables = b.cfg.valuables
	c.wireValuables()
	return c
}

// lookSteps are rule "valuables"'s look at the keeper from start: select,
// talk, open the private withdrawal, wait for the list.
func lookSteps(start time.Duration) []trainerStep {
	return []trainerStep{
		{name: "select the keeper", at: start, wantRule: "valuables", wantKind: actSelect},
		{name: "talk", at: start + time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "valuables", wantKind: actTalk},
		{name: "wait for the page", at: start + 2*time.Second, wantRule: "valuables"},
		{
			name: "open", at: start + 3*time.Second, events: []Event{keeperPage(keeperObj, "DepositP", "WithdrawP", "DepositC", "WithdrawC")},
			wantRule: "valuables", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_WithdrawP"),
		},
		{name: "wait for the list", at: start + 4*time.Second, wantRule: "valuables"},
	}
}

// privateList is the private warehouse's withdraw list of items.
func privateList(items ...WarehouseItem) WarehouseWithdrawList {
	return WarehouseWithdrawList{Type: privateWarehouseType, Items: items}
}

// TestValuablesLedgerLook is the session's look at its private warehouse
// and the entries its rows add (docs/valuables/ledger.md "Tests").
func TestValuablesLedgerLook(t *testing.T) {
	t.Run("looks", testLookLooks)
	t.Run("open due", testLookOpenDue)
	t.Run("no look", testLookNoLook)
	t.Run("process start", testLookProcessStart)
	t.Run("reconnect", testLookReconnect)
	t.Run("marks", testLookMarks)
	t.Run("follow", testLookFollow)
	t.Run("combat", testLookCombat)
}

// testLookLooks: a fresh session's first town visit looks with nothing
// carried, a round started within RoundGap or not, then walks back within
// anchorArrive and lets go; the look runs once a session.
func testLookLooks(t *testing.T) {
	for _, tt := range []struct {
		name   string
		answer Event
	}{
		{"empty: 282", SystemMessage{ID: msgNoItemDepositedInW}},
		{"a type 1 list", privateList(WarehouseItem{ObjectID: 301, TemplateID: animalSkin, Count: 3})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newKeeperLookBrain(t)
			b.persona.Valuables.RoundGap = 24 * time.Hour
			b.cfg.valuables.openRound(Point{}, t0.Add(-time.Minute))
			b.cfg.valuables.endRound()
			steps := append(lookSteps(0), trainerStep{
				name: "answer: walk back", at: 5 * time.Second,
				events:   []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 250}}, tt.answer},
				wantRule: "valuables", wantKind: actMove, wantTo: Point{}, wantWithin: anchorArrive,
			})
			runTrainerSteps(t, b, steps)
			if !b.wh.private.Known || b.wh.running || !b.wh.privateWant.empty() {
				t.Fatalf("private status known %v, errand running %v, queued %+v; want the look done", b.wh.private.Known, b.wh.running, b.wh.privateWant)
			}
			if b.walk.arrive != anchorArrive {
				t.Errorf("walk back arrives within %d, want %d", b.walk.arrive, anchorArrive)
			}
			now := at(time.Minute)
			b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 50}}}, now)
			for _, now := range []time.Time{now.Add(time.Second), now.Add(time.Hour)} {
				if rule, _ := b.decide(now); rule == "valuables" || b.wh.running {
					t.Fatalf("at the anchor at %v: rule %q, errand running %v; want rule valuables let go and no second look", now.Sub(t0), rule, b.wh.running)
				}
			}
			if len(b.cfg.valuables.entries) != 0 {
				t.Errorf("entries %v, want none: no valuable carried or stored", b.cfg.valuables.entries)
			}
		})
	}
}

// testLookOpenDue: the look waits for the errand's open gap and retry
// time.
func testLookOpenDue(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(b *Brain)
		due   time.Duration
	}{
		{"open gap", func(b *Brain) { b.wh.openedAt = t0.Add(-30 * time.Second) }, 30 * time.Second},
		{"backoff", func(b *Brain) { b.wh.openedAt, b.wh.timeouts = t0.Add(-time.Minute), 1 }, time.Minute},
		{"retry time", func(b *Brain) { b.wh.retryAt = t0.Add(10 * time.Second) }, 10 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newKeeperLookBrain(t)
			tt.setup(b)
			if rule, _ := b.decide(at(tt.due - time.Second)); rule == "valuables" || b.wh.running {
				t.Fatalf("before the open is due: rule %q, errand running %v; want no look", rule, b.wh.running)
			}
			runTrainerSteps(t, b, []trainerStep{{name: "due", at: tt.due, wantRule: "valuables", wantKind: actSelect}})
			if !b.wh.req.Look || !b.wh.req.Private || b.wh.req.Reach != b.persona.Valuables.RoundReach || b.wh.req.Anchor != (Point{}) {
				t.Errorf("request %+v, want a private look anchored where the bot stands within RoundReach", b.wh.req)
			}
		})
	}
}

// testLookNoLook: no look while an errand runs, out of a registry town,
// with no unmarked private keeper within RoundReach, called away, or with
// the policy disabled.
func testLookNoLook(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(b *Brain)
		// look is set for the control case.
		look bool
	}{
		{name: "control: looks", look: true},
		{name: "warehouse request", setup: func(b *Brain) { b.wh.running = true }},
		{name: "market visit", setup: func(b *Brain) { b.market.v.obj = 9 }},
		{name: "gear UseItem", setup: func(b *Brain) { b.gear.obj = 9 }},
		{name: "crafter cycle", setup: func(b *Brain) { b.craft.phase = craftWithdraw }},
		{name: "enchant exchange", setup: func(b *Brain) { b.ench.step = enchantUsed }},
		{name: "clan visit unsettled", setup: func(b *Brain) { b.member.asked = true }},
		{name: "store open", setup: func(b *Brain) { b.world.Self.StoreType = storeTypeSell }},
		{name: "trade window", setup: func(b *Brain) { b.world.Trade.PartnerID = pcID }},
		{name: "out of a registry town", setup: func(b *Brain) { b.world.Self.resample(t0, Point{X: 3500}) }},
		{name: "no private keeper in reach", setup: func(b *Brain) { b.cfg.Travel.spawns[keeperNPC] = []Point{{X: 6500}} }},
		{name: "the keeper marked", setup: func(b *Brain) { b.wh.noPrivate[keeperNPC] = true }},
		{name: "called: follow", setup: func(b *Brain) { b.orders.follow = pcID }},
		{name: "policy disabled", setup: func(b *Brain) { b.persona.Valuables = ValuablePolicy{} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newKeeperLookBrain(t)
			if tt.setup != nil {
				tt.setup(b)
			}
			running := b.wh.running
			_, ok := b.ruleValuables(t0)
			if ok != tt.look {
				t.Fatalf("rule valuables matched %v, want %v", ok, tt.look)
			}
			if !tt.look && (b.wh.running != running || !b.wh.privateWant.empty()) {
				t.Errorf("errand running %v queued %+v, want no request", b.wh.running, b.wh.privateWant)
			}
		})
	}
}

// testLookProcessStart: after a process start the look's rows add entries
// for the valuables stored (rounds 0, the first ask, one stored unit), none
// for an augmented, quest, shadow or under-ValueMin row.
func testLookProcessStart(t *testing.T) {
	b := newKeeperLookBrain(t)
	list := privateList(
		WarehouseItem{ObjectID: 301, TemplateID: ledgerSword, Count: 1, BodySlot: partRHand},
		WarehouseItem{ObjectID: 302, TemplateID: ledgerStaff, Count: 1, BodySlot: partRHand, Enchant: 5},
		WarehouseItem{ObjectID: 303, TemplateID: ledgerSword, Count: 1, BodySlot: partRHand, Augmentation: 0x00010002},
		WarehouseItem{ObjectID: 304, TemplateID: lookQuest, Count: 1, SubCategory: subCategoryQuest},
		WarehouseItem{ObjectID: 305, TemplateID: lookShadow, Count: 1},
		WarehouseItem{ObjectID: 306, TemplateID: lookCheap, Count: 1},
	)
	runTrainerSteps(t, b, append(lookSteps(0), trainerStep{name: "list", at: 5 * time.Second, events: []Event{list}}))
	if got := len(b.cfg.valuables.entries); got != 2 {
		t.Fatalf("entries %v, want two: the +0 sword and the +5 staff", b.cfg.valuables.entries)
	}
	if e := entryOf(t, b, ledgerSword, 0); e.rounds != 0 || e.ask != 170000 || e.carried != 0 || e.stored != 1 {
		t.Errorf("+0 sword entry = %+v, want rounds 0, ask 170000, one stored unit (not the augmented one)", e)
	}
	if e := entryOf(t, b, ledgerStaff, 5); e.rounds != 0 || e.ask != 845250+250000 || e.carried != 0 || e.stored != 1 {
		t.Errorf("+5 staff entry = %+v, want rounds 0, ask 1095250 with five scrolls, one stored unit", e)
	}
}

// testLookReconnect: the next session keeps an entry with a stored unit
// unchanged while its private status is unknown; its look finds the unit.
func testLookReconnect(t *testing.T) {
	b := newKeeperLookBrain(t)
	runTrainerSteps(t, b, append(lookSteps(0), trainerStep{
		name: "list", at: 5 * time.Second, events: []Event{privateList(WarehouseItem{ObjectID: 301, TemplateID: ledgerSword, Count: 1})},
	}))
	e := b.cfg.valuables.entries[valuableKey{template: ledgerSword}]
	e.rounds, e.ask = 2, 150000
	want := *e

	c := nextSession(t, b, map[int32][]Point{keeperNPC: {{X: 300}}}, map[int32]NPCInfo{keeperObj: {NPCID: keeperNPC, Pos: Point{X: 300}}})
	runTrainerSteps(t, c, lookSteps(0)[:1])
	if got := entryOf(t, c, ledgerSword, 0); got.rounds != want.rounds || got.ask != want.ask || got.stored != 1 || got.carried != 0 {
		t.Fatalf("entry with the status unknown = %+v, want it unchanged: %+v", got, want)
	}
	runTrainerSteps(t, c, append(lookSteps(0)[1:], trainerStep{
		name: "list", at: 5 * time.Second, events: []Event{privateList(WarehouseItem{ObjectID: 301, TemplateID: ledgerSword, Count: 1})},
	}))
	if got := entryOf(t, c, ledgerSword, 0); got.rounds != 2 || got.ask != 150000 || got.stored != 1 {
		t.Errorf("entry after the look = %+v, want rounds 2, ask 150000, the stored unit", got)
	}
}

// testLookMarks: with V3a's keeper fixtures, a look ended on a new mark is
// started again at once at the next keeper; marks survive the session, so
// the next one goes straight to Rolfe, and at Teranu Mardui no look runs
// again.
func testLookMarks(t *testing.T) {
	t.Run("Rant, then Rolfe", func(t *testing.T) {
		spawns := map[int32][]Point{rantNPC: {{X: 200}}, rolfeNPC: {{X: 300}}}
		sight := map[int32]NPCInfo{rantObj: {NPCID: rantNPC, Pos: Point{X: 200}}, rolfeObj: {NPCID: rolfeNPC, Pos: Point{X: 300}}}
		b := newLookBrain(t, spawns, sight)
		first := keeperPage(rantObj, "Chat 1")
		runTrainerSteps(t, b, []trainerStep{
			{name: "select Rant", wantRule: "valuables", wantKind: actSelect},
			{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: rantObj}}, wantRule: "valuables", wantKind: actTalk},
			{name: "chat link", at: 2 * time.Second, events: []Event{first}, wantRule: "valuables", wantKind: actBypass, wantPayload: chatBypass(rantObj, "1")},
			{name: "freight page: talk again", at: 3 * time.Second, events: []Event{keeperPage(rantObj, "DepositF", "WithdrawF", "Chat 0")}, wantRule: "valuables", wantKind: actTalk},
			{name: "first page again: marked, Rolfe at once", at: 4 * time.Second, events: []Event{first}, wantRule: "valuables", wantKind: actSelect},
		})
		if !b.cfg.valuables.noPrivate[rantNPC] || b.wh.v.npcID != rolfeNPC || !b.wh.req.Look {
			t.Fatalf("marks %v, look %+v at keeper %d; want Rant marked in the bot's set and the look at Rolfe", b.cfg.valuables.noPrivate, b.wh.req, b.wh.v.npcID)
		}
		c := nextSession(t, b, spawns, sight)
		runTrainerSteps(t, c, []trainerStep{{name: "next session: Rolfe", wantRule: "valuables", wantKind: actSelect}})
		if c.wh.v.npcID != rolfeNPC {
			t.Errorf("next session's look at keeper %d, want Rolfe", c.wh.v.npcID)
		}
	})
	t.Run("Teranu Mardui", func(t *testing.T) {
		spawns := map[int32][]Point{hagosNPC: {{X: 300}}}
		sight := map[int32]NPCInfo{hagosObj: {NPCID: hagosNPC, Pos: Point{X: 300}}}
		b := newLookBrain(t, spawns, sight)
		runTrainerSteps(t, b, []trainerStep{
			{name: "select Hagos", wantRule: "valuables", wantKind: actSelect},
			{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: hagosObj}}, wantRule: "valuables", wantKind: actTalk},
		})
		if err := b.handle([]Event{hagosPage()}, at(2*time.Second)); err != nil {
			t.Fatal(err)
		}
		if rule, _ := b.decide(at(2 * time.Second)); rule == "valuables" || b.wh.running || !b.cfg.valuables.noPrivate[hagosNPC] {
			t.Fatalf("rule %q, errand running %v, marks %v; want Hagos marked and the look let go", rule, b.wh.running, b.cfg.valuables.noPrivate)
		}
		for i, s := range []*Brain{b, nextSession(t, b, spawns, sight)} {
			if _, ok := s.ruleValuables(at(time.Hour)); ok || s.wh.running {
				t.Errorf("session %d: rule valuables matched %v, errand running %v; want no look at Teranu Mardui", i+1, ok, s.wh.running)
			}
		}
	})
}

// testLookFollow: a follow on the walk to the keeper ends the look: no
// walk back, the next legs rule "follow"'s; it runs on the next town
// visit.
func testLookFollow(t *testing.T) {
	b := newLookBrain(t, map[int32][]Point{keeperNPC: {{X: 2000}}}, nil)
	runTrainerSteps(t, b, []trainerStep{{name: "walk to the keeper", wantRule: "valuables", wantKind: actMove}})
	runTrainerSteps(t, b, []trainerStep{{name: "follow", at: time.Second, setup: func(b *Brain) { b.orders.follow = pcID }, wantRule: "follow"}})
	if b.wh.running || !b.wh.privateWant.empty() || b.wh.private.Known || b.look.phase != lookIdle {
		t.Fatalf("errand running %v queued %+v known %v phase %d; want the look ended, not done", b.wh.running, b.wh.privateWant, b.wh.private.Known, b.look.phase)
	}
	if _, ok := b.ruleValuables(at(2 * time.Second)); ok {
		t.Fatal("rule valuables matched while following")
	}
	b.orders.follow = 0
	now := at(time.Minute)
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{}}}, now); err != nil {
		t.Fatal(err)
	}
	if rule, a := b.decide(now.Add(time.Second)); rule != "valuables" || a.kind != actMove || !b.wh.req.Look {
		t.Errorf("next town visit: rule %q action %d request %+v, want the look walking to the keeper", rule, a.kind, b.wh.req)
	}
}

// testLookCombat: in combat on the way to the keeper the look holds, its
// request kept, and lets rule valuables go; out of combat it steps the
// same request again.
func testLookCombat(t *testing.T) {
	b := newLookBrain(t, map[int32][]Point{keeperNPC: {{X: 2000}}}, nil)
	runTrainerSteps(t, b, []trainerStep{{name: "walk to the keeper", wantRule: "valuables", wantKind: actMove}})
	req, keeper, anchor := b.wh.req, b.wh.v, b.look.anchor
	b.world.Self.InCombat = true
	if _, ok := b.ruleValuables(at(time.Second)); ok {
		t.Fatal("rule valuables matched in combat")
	}
	if !b.wh.running || b.look.phase != lookRunning || !reflect.DeepEqual(b.wh.req, req) {
		t.Fatalf("in combat: errand running %v phase %d request %+v; want the look held, request %+v", b.wh.running, b.look.phase, b.wh.req, req)
	}
	b.world.Self.InCombat = false
	rule, _ := b.decide(at(2 * time.Second))
	if rule != "valuables" || !b.wh.running || !reflect.DeepEqual(b.wh.req, req) || b.wh.v.npcID != keeper.npcID || b.wh.v.at != keeper.at || b.look.anchor != anchor {
		t.Errorf("out of combat: rule %q running %v request %+v keeper %d at %v anchor %v; want rule valuables stepping request %+v at keeper %d at %v, anchor %v",
			rule, b.wh.running, b.wh.req, b.wh.v.npcID, b.wh.v.at, b.look.anchor, req, keeper.npcID, keeper.at, anchor)
	}
}
