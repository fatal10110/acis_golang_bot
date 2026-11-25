package botclient

import (
	"reflect"
	"testing"
	"time"
)

// Keepers of the private warehouse tests (data/xml/spawnlist; docs/valuables/round.md,
// "Keepers by town").
const (
	rantNPC   = int32(30054) // freightman, Talking Island
	rolfeNPC  = int32(30055) // Talking Island
	dionNPC   = int32(30058) // Dion
	hagosNPC  = int32(31381) // Varka alliance keeper
	rantObj   = int32(0x20000060)
	rolfeObj  = int32(0x20000061)
	hagosObj  = int32(0x20000062)
	privReach = int32(6000) // RoundReach
)

// whCarriedSword is a +5 sword the bot carries.
var whCarriedSword = Item{ObjectID: 106, TemplateID: swordTemplate, Count: 1, Enchant: 5}

// newPrivateBrain is newWarehouseBrain with no Want: only a queued private
// request runs the errand.
func newPrivateBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newWarehouseBrain(t, &whWants{}, items...)
	b.persona.Warehouse.Want = nil
	return b
}

// privateReq is a private request anchored at the origin, where the bot
// stands, within RoundReach.
func privateReq(r WarehouseRequest) WarehouseRequest {
	r.Private, r.Reach = true, privReach
	return r
}

var (
	privateLook     = privateReq(WarehouseRequest{Look: true})
	privateSkins    = privateReq(WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: animalSkin}}})
	privateWithdraw = privateReq(WarehouseRequest{Withdraw: []WarehouseRow{{TemplateID: swordTemplate, Count: 1}}})
)

// queue queues req as the private request (what rule "valuables" does).
func queue(req WarehouseRequest) func(b *Brain) {
	return func(b *Brain) { b.wh.privateWant = req }
}

// privateOpenSteps talks to the keeper and opens its private warehouse
// from start.
func privateOpenSteps(start time.Duration, deposit bool) []trainerStep {
	cmd := "WithdrawP"
	if deposit {
		cmd = "DepositP"
	}
	return []trainerStep{
		{name: "select the keeper", at: start, wantRule: "warehouse", wantKind: actSelect},
		{name: "talk", at: start + time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "warehouse", wantKind: actTalk},
		{name: "wait for the page", at: start + 2*time.Second, wantRule: "warehouse"},
		{
			name: "open", at: start + 3*time.Second, events: []Event{keeperPage(keeperObj, "DepositP", "WithdrawP", "DepositC", "WithdrawC")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_" + cmd),
		},
		{name: "wait for the " + cmd + " list", at: start + 4*time.Second, wantRule: "warehouse"},
	}
}

func chatBypass(obj int32, n string) []byte {
	return encodeRequestBypassToServer("npc_" + itoa(obj) + "_Chat " + n)
}

// TestPrivateWarehouseErrand is the private warehouse errand (V3a,
// docs/valuables/warehouse.md "Tests").
func TestPrivateWarehouseErrand(t *testing.T) {
	t.Run("Start", testPrivateStart)
	t.Run("Links", testPrivateLinks)
	t.Run("Refused chat", testPrivateRefusedChat)
	t.Run("Cancel", testPrivateCancel)
	t.Run("Status", testPrivateStatus)
	t.Run("Slots", testPrivateSlots)
	t.Run("Keeper", testPrivateKeeper)
	t.Run("Marks", testPrivateMarks)
}

func testPrivateStart(t *testing.T) {
	t.Run("no Want, no crafter, no clan", func(t *testing.T) {
		b := newPrivateBrain(t)
		b.world.Clan = Clan{}
		runTrainerSteps(t, b, []trainerStep{{name: "start", setup: queue(privateLook), wantRule: "warehouse", wantKind: actSelect}})
		if !b.wh.running || !b.wh.req.Private || !b.wh.privateWant.empty() {
			t.Fatalf("errand running %v req %+v slot %+v, want the private look running and the slot clear", b.wh.running, b.wh.req, b.wh.privateWant)
		}
	})
	t.Run("ahead of a due clan Want", func(t *testing.T) {
		wants := &whWants{reqs: []WarehouseRequest{{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}}}
		b := newWarehouseBrain(t, wants, adenaStack(1000), whCarriedSkins)
		runTrainerSteps(t, b, []trainerStep{{name: "start", setup: queue(privateLook), wantRule: "warehouse", wantKind: actSelect}})
		if !b.wh.req.Private || !b.wh.req.Look || len(wants.seen) != 0 {
			t.Fatalf("req %+v, Want asked %d times, want the private look first", b.wh.req, len(wants.seen))
		}
	})
	t.Run("empty slot, no Want", func(t *testing.T) {
		b := newPrivateBrain(t)
		runTrainerSteps(t, b, []trainerStep{{name: "nothing"}})
		if b.wh.running {
			t.Fatal("errand running with nothing asked")
		}
	})
	t.Run("stepWarehouse without rule warehouse", func(t *testing.T) {
		b := newPrivateBrain(t)
		b.wh.privateWant = privateLook
		if !b.startWarehouse(t0) {
			t.Fatal("startWarehouse: no request runs")
		}
		if a, ok := b.stepWarehouse(t0); !ok || a.kind != actSelect || a.object != keeperObj {
			t.Fatalf("stepWarehouse = %+v %v, want the keeper selected", a, ok)
		}
	})
}

func testPrivateLinks(t *testing.T) {
	t.Run("DepositP, a type 2 list refused", func(t *testing.T) {
		b := newPrivateBrain(t, adenaStack(1000), whCarriedSkins)
		b.wh.noClan = map[int32]bool{keeperNPC: true} // not read
		steps := privateOpenSteps(0, true)
		steps[0].setup = queue(privateSkins)
		deposit := []WarehouseItem{{ObjectID: 102, TemplateID: animalSkin, Count: 10}}
		steps = append(steps,
			trainerStep{name: "clan list ignored", at: 5 * time.Second, events: []Event{WarehouseDepositList{Type: clanWarehouseType, Items: deposit}}, wantRule: "warehouse"},
			trainerStep{
				name: "private list", at: 6 * time.Second, events: []Event{WarehouseDepositList{Type: privateWarehouseType, Items: deposit}},
				wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: depositReq(warehouseRow{102, animalSkin, 10}),
			},
			trainerStep{name: "deposited", at: 7 * time.Second, events: []Event{InventoryUpdate{Changes: []ItemChange{{ItemRemoved, whCarriedSkins}}}}},
		)
		runTrainerSteps(t, b, steps)
		if got := b.wh.private.Deposit; got.Outcome != WarehouseDone || b.wh.status.Deposit.Outcome != WarehouseNone {
			t.Fatalf("private deposit %+v clan %+v, want done in the private status only", got, b.wh.status.Deposit)
		}
		if len(b.wh.noClan) != 1 || len(b.wh.noPrivate) != 0 {
			t.Fatalf("marks noClan %v noPrivate %v, want none set", b.wh.noClan, b.wh.noPrivate)
		}
	})
	t.Run("WithdrawP prefix", func(t *testing.T) {
		b := newPrivateBrain(t)
		runTrainerSteps(t, b, []trainerStep{
			{name: "select", setup: queue(privateLook), wantRule: "warehouse", wantKind: actSelect},
			{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "warehouse", wantKind: actTalk},
			{
				name: "open", at: 2 * time.Second, events: []Event{keeperPage(keeperObj, "WithdrawC", "WithdrawP 1")},
				wantRule: "warehouse", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_WithdrawP 1"),
			},
		})
	})
	t.Run("Chat 1 page", func(t *testing.T) {
		b := newPrivateBrain(t)
		runTrainerSteps(t, b, []trainerStep{
			{name: "select", setup: queue(privateLook), wantRule: "warehouse", wantKind: actSelect},
			{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "warehouse", wantKind: actTalk},
			{name: "chat link", at: 2 * time.Second, events: []Event{keeperPage(keeperObj, "Chat 1", "Quest")}, wantRule: "warehouse", wantKind: actBypass, wantPayload: chatBypass(keeperObj, "1")},
			{
				name: "open", at: 3 * time.Second, events: []Event{keeperPage(keeperObj, "DepositP", "WithdrawP", "Chat 0")},
				wantRule: "warehouse", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_WithdrawP"),
			},
		})
	})
	for _, tt := range []struct {
		name    string
		deposit bool
		msg     int32
		want    WarehouseOutcome
	}{
		{"282 empty", false, msgNoItemDepositedInW, WarehouseEmpty},
		{"279 refused", true, msgNotEnoughAdena, WarehouseRefused},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newPrivateBrain(t, adenaStack(1000), whCarriedSkins)
			b.wh.status = WarehouseStatus{Known: true, Contents: []WarehouseItem{whStoredSkins}}
			steps := privateOpenSteps(0, tt.deposit)
			steps[0].setup = queue(privateLook)
			if tt.deposit {
				steps[0].setup = queue(privateSkins)
				steps = append(steps, trainerStep{
					name: "request", at: 5 * time.Second, wantRule: "warehouse", wantKind: actWarehouseDeposit,
					events: []Event{WarehouseDepositList{Type: privateWarehouseType, Items: []WarehouseItem{{ObjectID: 102, TemplateID: animalSkin, Count: 10}}}},
				})
			}
			steps = append(steps, trainerStep{name: "answer", at: 6 * time.Second, events: []Event{SystemMessage{ID: tt.msg}}})
			runTrainerSteps(t, b, steps)
			got := b.wh.private.Withdraw
			if tt.deposit {
				got = b.wh.private.Deposit
			}
			if got.Outcome != tt.want || b.wh.running || !b.wh.retryAt.IsZero() {
				t.Fatalf("result %+v running %v retryAt %v, want %d and no retry time", got, b.wh.running, b.wh.retryAt, tt.want)
			}
			if !tt.deposit && (!b.wh.private.Known || b.wh.private.Contents != nil) {
				t.Fatalf("private status %+v, want known empty", b.wh.private)
			}
			if !b.wh.status.Known || len(b.wh.status.Contents) != 1 {
				t.Fatalf("clan status %+v changed", b.wh.status)
			}
		})
	}
}

// testPrivateRefusedChat: a 142 to the Chat 1 bypass is a refusal, no
// mark, and a later request there follows Chat 1 again; clan requests too.
func testPrivateRefusedChat(t *testing.T) {
	clanReq := WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}
	for _, private := range []bool{true, false} {
		name := "clan"
		if private {
			name = "private"
		}
		t.Run(name, func(t *testing.T) {
			wants := &whWants{}
			var again func(b *Brain)
			if private {
				again = queue(privateSkins)
			} else {
				wants.reqs = []WarehouseRequest{clanReq, clanReq}
			}
			b := newWarehouseBrain(t, wants, adenaStack(1000), whCarriedSkins)
			if private {
				b.persona.Warehouse.Want = nil
			}
			first := keeperPage(keeperObj, "Chat 1")
			runTrainerSteps(t, b, []trainerStep{
				{name: "select", setup: again, wantRule: "warehouse", wantKind: actSelect},
				{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "warehouse", wantKind: actTalk},
				{name: "chat link", at: 2 * time.Second, events: []Event{first}, wantRule: "warehouse", wantKind: actBypass, wantPayload: chatBypass(keeperObj, "1")},
				{name: "142 refused", at: 3 * time.Second, events: []Event{SystemMessage{ID: msgAlreadyTrading}}},
			})
			st := b.wh.statusOf(WarehouseRequest{Private: private})
			if got := st.Deposit; got.Outcome != WarehouseRefused || got.Msg != msgAlreadyTrading || b.wh.running {
				t.Fatalf("deposit %+v running %v, want refused by 142", got, b.wh.running)
			}
			if !b.wh.retryAt.Equal(at(3*time.Second + warehouseOpenGap)) {
				t.Fatalf("retryAt %v, want 60 s after the refusal", b.wh.retryAt)
			}
			if len(b.wh.noPrivate) != 0 || b.wh.noClan != nil {
				t.Fatalf("marks noPrivate %v noClan %v, want none", b.wh.noPrivate, b.wh.noClan)
			}
			runTrainerSteps(t, b, []trainerStep{
				{name: "no request before retryAt", at: 3*time.Second + warehouseOpenGap - time.Millisecond, setup: again},
				{name: "talk again", at: 3*time.Second + warehouseOpenGap, wantRule: "warehouse", wantKind: actTalk},
				{name: "Chat 1 again", at: 4*time.Second + warehouseOpenGap, events: []Event{first}, wantRule: "warehouse", wantKind: actBypass, wantPayload: chatBypass(keeperObj, "1")},
			})
		})
	}
}

func testPrivateCancel(t *testing.T) {
	sword := []WarehouseItem{{ObjectID: 0x10000302, TemplateID: swordTemplate, Count: 1}}
	for _, tt := range []struct {
		name  string
		req   WarehouseRequest
		steps func(b *Brain) []trainerStep
		// events are handled after the steps, with no decision.
		events []Event
		// late are answers after the cancel, dropped.
		late     []Event
		opened   bool
		canceled bool
	}{
		{
			name: "walk", req: privateLook, canceled: true,
			steps: func(b *Brain) []trainerStep {
				b.world.Apply(DeleteObject{ObjectID: keeperObj}, t0)
				b.cfg.Travel.spawns[keeperNPC] = []Point{{X: 5000}}
				return []trainerStep{{name: "walk", setup: queue(privateLook), wantRule: "warehouse", wantKind: actMove}}
			},
		},
		{
			name: "talk", req: privateLook, canceled: true,
			steps: func(*Brain) []trainerStep { return privateOpenSteps(0, false)[:2] },
		},
		{
			name: "open link sent", req: privateLook, canceled: true, opened: true,
			steps: func(*Brain) []trainerStep { return privateOpenSteps(0, false) },
			late:  []Event{WarehouseWithdrawList{Type: privateWarehouseType, Items: sword}, SystemMessage{ID: msgNoItemDepositedInW}},
		},
		{
			name: "list received", req: privateWithdraw, canceled: true, opened: true,
			steps:  func(*Brain) []trainerStep { return privateOpenSteps(0, false) },
			events: []Event{WarehouseWithdrawList{Type: privateWarehouseType, Items: sword}},
		},
		{
			name: "request sent", req: privateWithdraw, opened: true,
			steps: func(*Brain) []trainerStep {
				return append(privateOpenSteps(0, false), trainerStep{
					name: "withdraw", at: 5 * time.Second, events: []Event{WarehouseWithdrawList{Type: privateWarehouseType, Items: sword}},
					wantRule: "warehouse", wantKind: actWarehouseWithdraw,
				})
			},
		},
		{
			name: "clan request", req: WarehouseRequest{Look: true}, opened: true,
			steps: func(b *Brain) []trainerStep {
				b.persona.Warehouse.Want = (&whWants{reqs: []WarehouseRequest{{Look: true}}}).want
				return openSteps(0, false)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newPrivateBrain(t)
			steps := tt.steps(b)
			if tt.req.Private && steps[0].setup == nil {
				steps[0].setup = queue(tt.req)
			}
			runTrainerSteps(t, b, steps)
			if err := b.handle(tt.events, at(5*time.Second)); err != nil {
				t.Fatal(err)
			}
			before := b.wh.private
			if got := b.wh.cancelPrivate(); got != tt.canceled {
				t.Fatalf("cancelPrivate = %v, want %v", got, tt.canceled)
			}
			if b.wh.running == tt.canceled {
				t.Fatalf("running %v after cancelPrivate, want %v", b.wh.running, !tt.canceled)
			}
			if !tt.canceled {
				return
			}
			if err := b.handle(tt.late, at(6*time.Second)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(b.wh.private, before) || b.wh.private.Withdraw.Outcome != WarehouseNone {
				t.Fatalf("private status %+v, want unchanged with no result", b.wh.private)
			}
			if !b.wh.retryAt.IsZero() || b.wh.openedAt.IsZero() == tt.opened || tt.opened && !b.wh.openedAt.Equal(at(3*time.Second)) {
				t.Fatalf("retryAt %v openedAt %v, want no retry time and the open kept", b.wh.retryAt, b.wh.openedAt)
			}
		})
	}
}

// testPrivateStatus: a private look, withdrawal and deposit leave a clan
// member's clan view alone; a deposited row keeps its object and enchant.
func testPrivateStatus(t *testing.T) {
	b := newPrivateBrain(t, adenaStack(1000), whCarriedSkins, whCarriedSword)
	clan := WarehouseStatus{
		Known: true, Contents: []WarehouseItem{whStoredSkins},
		Withdraw: WarehouseResult{Outcome: WarehouseListed, At: t0}, Deposit: WarehouseResult{Outcome: WarehouseDone, At: t0},
	}
	b.wh.status = clan
	stored := WarehouseItem{ObjectID: 0x10000301, TemplateID: swordTemplate, Count: 1}
	// The look.
	steps := privateOpenSteps(0, false)
	steps[0].setup = queue(privateLook)
	steps = append(steps, trainerStep{name: "listed", at: 5 * time.Second, events: []Event{WarehouseWithdrawList{Type: privateWarehouseType, Items: []WarehouseItem{stored}}}})
	runTrainerSteps(t, b, steps)
	if got := b.wh.private; !got.Known || !reflect.DeepEqual(got.Contents, []WarehouseItem{stored}) || got.Withdraw.Outcome != WarehouseListed {
		t.Fatalf("private status %+v, want the look's list", got)
	}
	// The deposit of the +5 sword.
	start := 5*time.Second + warehouseOpenGap
	depositSword := privateReq(WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: swordTemplate, Object: whCarriedSword.ObjectID}}})
	steps = []trainerStep{
		{name: "select", at: start, setup: queue(depositSword), wantRule: "warehouse", wantKind: actTalk},
		{
			name: "open", at: start + time.Second, events: []Event{keeperPage(keeperObj, "DepositP", "WithdrawP")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_DepositP"),
		},
		{
			name: "deposit", at: start + 2*time.Second, wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: depositReq(warehouseRow{106, swordTemplate, 1}),
			events: []Event{WarehouseDepositList{Type: privateWarehouseType, Items: []WarehouseItem{
				{ObjectID: 102, TemplateID: animalSkin, Count: 10}, {ObjectID: 106, TemplateID: swordTemplate, Count: 1, Enchant: 5},
			}}},
		},
		{name: "deposited", at: start + 3*time.Second, events: []Event{InventoryUpdate{Changes: []ItemChange{{ItemRemoved, whCarriedSword}}}}},
	}
	runTrainerSteps(t, b, steps)
	want := []WarehouseItem{stored, {ObjectID: 106, TemplateID: swordTemplate, Count: 1, Enchant: 5}}
	if got := b.wh.private; !reflect.DeepEqual(got.Contents, want) || got.Deposit.Outcome != WarehouseDone {
		t.Fatalf("private status %+v, want the +5 sword added with its object", got)
	}
	// The withdrawal naming the deposited object takes it, not the +0.
	start += 2 * warehouseOpenGap
	withdraw := privateReq(WarehouseRequest{Withdraw: []WarehouseRow{{TemplateID: swordTemplate, Count: 1, Object: want[1].ObjectID}}})
	runTrainerSteps(t, b, []trainerStep{
		{name: "talk", at: start, setup: queue(withdraw), wantRule: "warehouse", wantKind: actTalk},
		{name: "open", at: start + time.Second, events: []Event{keeperPage(keeperObj, "DepositP", "WithdrawP")}, wantRule: "warehouse", wantKind: actBypass},
		{
			name: "withdraw", at: start + 2*time.Second, events: []Event{WarehouseWithdrawList{Type: privateWarehouseType, Items: want}},
			wantRule: "warehouse", wantKind: actWarehouseWithdraw, wantPayload: withdrawReq(warehouseRow{106, swordTemplate, 1}),
		},
		{name: "withdrawn", at: start + 3*time.Second, events: []Event{InventoryUpdate{Changes: []ItemChange{{ItemAdded, whCarriedSword}}}}},
	})
	if got := b.wh.private; !reflect.DeepEqual(got.Contents, []WarehouseItem{stored}) || got.Withdraw.Outcome != WarehouseDone {
		t.Fatalf("private status %+v, want the +5 sword taken off", got)
	}
	if !reflect.DeepEqual(b.wh.status, clan) {
		t.Fatalf("clan status %+v, want it unchanged %+v", b.wh.status, clan)
	}
}

// testPrivateSlots: a private deposit counts against the private contents
// and 100 slots, 120 for a dwarf.
func testPrivateSlots(t *testing.T) {
	for _, tt := range []struct {
		name   string
		race   int32
		stored int
		full   bool
	}{
		{"human, 99 stored", RaceHuman, 99, false},
		{"human, 100 stored", RaceHuman, 100, true},
		{"dwarf, 100 stored", RaceDwarf, 100, false},
		{"dwarf, 120 stored", RaceDwarf, 120, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newPrivateBrain(t, adenaStack(1000), whCarriedSword)
			b.world.Self.Race = tt.race
			b.wh.private.Known = true
			for i := range tt.stored {
				b.wh.private.Contents = append(b.wh.private.Contents, WarehouseItem{ObjectID: int32(0x10000400 + i), TemplateID: int32(5000 + i), Count: 1})
			}
			b.wh.status = WarehouseStatus{Known: true} // an empty clan warehouse is not counted
			b.wh.privateWant = privateReq(WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: swordTemplate}}})
			started := b.startWarehouse(t0)
			if full := b.wh.private.Deposit.Outcome == WarehouseFull; started == tt.full || full != tt.full {
				t.Fatalf("started %v deposit %+v, want full %v", started, b.wh.private.Deposit, tt.full)
			}
		})
	}
}

// keeperBrain is a bot at from with the travel map's keepers spawns and,
// in sight, the keepers sight (object id to template and position).
func keeperBrain(t *testing.T, from Point, spawns map[int32][]Point, sight map[int32]NPCInfo) *Brain {
	t.Helper()
	b := newPrivateBrain(t)
	b.world.Apply(DeleteObject{ObjectID: keeperObj}, t0)
	b.world.Self.resample(t0, from)
	m := &TravelMap{spawns: spawns, keepers: map[int32]bool{}}
	for npc := range spawns {
		m.keepers[npc] = true
	}
	for obj, info := range sight {
		info.ObjectID = obj
		b.world.Apply(info, t0)
		m.keepers[info.NPCID] = true
	}
	b.cfg.Travel = m
	return b
}

func testPrivateKeeper(t *testing.T) {
	trisha := Point{X: 15631, Y: 142885, Z: -2704}
	far := map[int32]NPCInfo{keeperObj: {NPCID: keeperNPC, Pos: Point{X: -1500}}}
	twoInSight := map[int32]NPCInfo{rantObj: {NPCID: rantNPC, Pos: Point{X: 300}}, rolfeObj: {NPCID: rolfeNPC, Pos: Point{X: 500}}}
	for _, tt := range []struct {
		name      string
		from      Point
		spawns    map[int32][]Point
		sight     map[int32]NPCInfo
		req       WarehouseRequest
		noPrivate map[int32]bool
		// wantNPC is the keeper taken, 0 for none (unreachable).
		wantNPC int32
		wantObj int32
	}{
		{
			name: "nearest within reach of the anchor, not a nearer one in sight", spawns: map[int32][]Point{rolfeNPC: {{X: 4000}}}, sight: far,
			req: privateReq(WarehouseRequest{Look: true, Anchor: Point{X: 5000}}), wantNPC: rolfeNPC,
		},
		{
			name: "none within reach of the anchor", spawns: map[int32][]Point{rolfeNPC: {{X: 4000}}}, sight: far,
			req: privateReq(WarehouseRequest{Look: true, Anchor: Point{X: 20000}}),
		},
		{
			name: "Dion: from Trisha's spawn to 30058", from: trisha, spawns: map[int32][]Point{dionNPC: {{X: 20750, Y: 144432, Z: -3068}}},
			req: privateReq(WarehouseRequest{Look: true, Anchor: trisha}), wantNPC: dionNPC,
		},
		{
			name: "a marked keeper in sight is not taken", sight: twoInSight, noPrivate: map[int32]bool{rantNPC: true},
			req: privateLook, wantNPC: rolfeNPC, wantObj: rolfeObj,
		},
		{
			name: "every keeper within reach marked", sight: twoInSight, spawns: map[int32][]Point{rantNPC: {{X: 300}}, rolfeNPC: {{X: 500}}},
			noPrivate: map[int32]bool{rantNPC: true, rolfeNPC: true}, req: privateLook,
		},
		{
			name: "a clan request neither reads noPrivate nor the anchor", sight: twoInSight, noPrivate: map[int32]bool{rantNPC: true},
			req: WarehouseRequest{Look: true, Anchor: Point{X: 50000}, Reach: 1}, wantNPC: rantNPC, wantObj: rantObj,
		},
		{
			name: "a clan request: a spawn within the persona's Reach of the bot", spawns: map[int32][]Point{rolfeNPC: {{X: 7000}}},
			req: WarehouseRequest{Look: true}, wantNPC: rolfeNPC,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := keeperBrain(t, tt.from, tt.spawns, tt.sight)
			b.wh.noPrivate = tt.noPrivate
			if tt.req.Private {
				b.wh.privateWant = tt.req
			} else {
				b.persona.Warehouse.Want = (&whWants{reqs: []WarehouseRequest{tt.req}}).want
			}
			started := b.startWarehouse(t0)
			if tt.wantNPC == 0 {
				if started || b.wh.statusOf(tt.req).Withdraw.Outcome != WarehouseUnreachable {
					t.Fatalf("started %v at %d, look %+v, want unreachable", started, b.wh.v.npcID, b.wh.statusOf(tt.req).Withdraw)
				}
				return
			}
			if !started || b.wh.v.npcID != tt.wantNPC || b.wh.v.obj != tt.wantObj {
				t.Fatalf("started %v at keeper %d obj %#x, want %d obj %#x", started, b.wh.v.npcID, b.wh.v.obj, tt.wantNPC, tt.wantObj)
			}
		})
	}
}

// hagosPage is an allied player's page of Hagos: a private deposit link,
// the withdrawal a quest bypass (VarkaSilenosSupport/31381-2.htm).
func hagosPage() NpcHtmlMessage {
	return NpcHtmlMessage{ObjectID: hagosObj, HTML: `<html><body>Warehouse Keeper Hagos:<br>` +
		`<a action="bypass -h npc_` + itoa(hagosObj) + `_DepositP">Deposit an item.</a><br>` +
		`<a action="bypass -h Quest VarkaSilenosSupport Withdraw">Pick up an item.</a></body></html>`}
}

// testPrivateMarks: a keeper whose pages lack the private link is marked
// noPrivate in a handed-in set, the request ends unreachable with no
// retry time and no open, and the next request goes to the next keeper.
func testPrivateMarks(t *testing.T) {
	t.Run("freightman Rant, then Rolfe", func(t *testing.T) {
		b := keeperBrain(t, Point{}, nil, map[int32]NPCInfo{rantObj: {NPCID: rantNPC, Pos: Point{X: 200}}, rolfeObj: {NPCID: rolfeNPC, Pos: Point{X: 300}}})
		handed := map[int32]bool{}
		b.wh.noPrivate = handed
		first := keeperPage(rantObj, "Chat 1")
		runTrainerSteps(t, b, []trainerStep{
			{name: "select Rant", setup: queue(privateLook), wantRule: "warehouse", wantKind: actSelect},
			{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: rantObj}}, wantRule: "warehouse", wantKind: actTalk},
			{name: "chat link", at: 2 * time.Second, events: []Event{first}, wantRule: "warehouse", wantKind: actBypass, wantPayload: chatBypass(rantObj, "1")},
			{name: "freight page: talk again", at: 3 * time.Second, events: []Event{keeperPage(rantObj, "DepositF", "WithdrawF", "Chat 0")}, wantRule: "warehouse", wantKind: actTalk},
			{name: "first page again: marked", at: 4 * time.Second, events: []Event{first}},
		})
		if !handed[rantNPC] || b.wh.noClan != nil || b.wh.private.Withdraw.Outcome != WarehouseUnreachable {
			t.Fatalf("handed set %v noClan %v look %+v, want Rant marked in the handed set and the look unreachable", handed, b.wh.noClan, b.wh.private.Withdraw)
		}
		if !b.wh.retryAt.IsZero() || !b.wh.openedAt.IsZero() {
			t.Fatalf("retryAt %v openedAt %v, want no retry time and no open", b.wh.retryAt, b.wh.openedAt)
		}
		runTrainerSteps(t, b, []trainerStep{{name: "the next request: Rolfe", at: 5 * time.Second, setup: queue(privateLook), wantRule: "warehouse", wantKind: actSelect}})
		if b.wh.v.npcID != rolfeNPC {
			t.Fatalf("keeper %d, want Rolfe", b.wh.v.npcID)
		}
		// A new errand given the set skips Rant.
		c := keeperBrain(t, Point{}, nil, map[int32]NPCInfo{rantObj: {NPCID: rantNPC, Pos: Point{X: 200}}, rolfeObj: {NPCID: rolfeNPC, Pos: Point{X: 300}}})
		c.wh.noPrivate, c.wh.privateWant = handed, privateLook
		if !c.startWarehouse(t0) || c.wh.v.npcID != rolfeNPC {
			t.Fatalf("new errand at keeper %d, want Rolfe", c.wh.v.npcID)
		}
	})
	for _, tt := range []struct {
		name string
		page NpcHtmlMessage
		req  WarehouseRequest
		// open is the link the request opens; "" marks Hagos.
		open string
	}{
		{"Hagos with no link", NpcHtmlMessage{ObjectID: hagosObj, HTML: `<html><body>Warehouse Keeper Hagos:<br>Ha!</body></html>`}, privateLook, ""},
		{"Hagos allied: a look marks it", hagosPage(), privateLook, ""},
		{"Hagos allied: a deposit opens", hagosPage(), privateSkins, "npc_" + itoa(hagosObj) + "_DepositP"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := keeperBrain(t, Point{}, nil, map[int32]NPCInfo{hagosObj: {NPCID: hagosNPC, Pos: Point{X: 300}}})
			b.world.Apply(ItemList{Items: []Item{adenaStack(1000), whCarriedSkins}}, t0)
			last := trainerStep{name: "page: marked", at: 2 * time.Second, events: []Event{tt.page}}
			if tt.open != "" {
				last.wantRule, last.wantKind, last.wantPayload = "warehouse", actBypass, encodeRequestBypassToServer(tt.open)
			}
			runTrainerSteps(t, b, []trainerStep{
				{name: "select", setup: queue(tt.req), wantRule: "warehouse", wantKind: actSelect},
				{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: hagosObj}}, wantRule: "warehouse", wantKind: actTalk},
				last,
			})
			if marked := b.wh.noPrivate[hagosNPC]; marked != (tt.open == "") {
				t.Fatalf("Hagos marked %v, want %v", marked, tt.open == "")
			}
			if tt.open == "" && (b.wh.running || b.wh.private.Withdraw.Outcome != WarehouseUnreachable || !b.wh.retryAt.IsZero()) {
				t.Fatalf("running %v look %+v retryAt %v, want unreachable with no retry time", b.wh.running, b.wh.private.Withdraw, b.wh.retryAt)
			}
		})
	}
}
