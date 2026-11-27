package botclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// keeperObj is warehouse keeper Walderal (30005) once in sight.
const (
	keeperObj = int32(0x20000050)
	keeperNPC = int32(30005)
)

// Items of the warehouse tests (aCis_datapack/data/xml/items).
const swordTemplate int32 = 2369

var (
	whCarriedSkins = Item{ObjectID: 102, TemplateID: animalSkin, Count: 10, SubCategory: 5}
	whCarriedShirt = Item{ObjectID: 104, TemplateID: leatherShirt, Count: 1, Category: 1, SubCategory: 1}
	whStoredSkins  = WarehouseItem{ObjectID: 0x10000300, TemplateID: animalSkin, Count: 20, SubCategory: 5}
	whStoredSword  = WarehouseItem{ObjectID: 0x10000301, TemplateID: swordTemplate, Count: 1}
)

// keeperPage is a keeper's page with links cmds (data/html/warehouse).
func keeperPage(obj int32, cmds ...string) NpcHtmlMessage {
	html := `<html><body>Warehouse:<br>`
	for _, c := range cmds {
		html += `<a action="bypass -h npc_` + itoa(obj) + `_` + c + `">link</a><br>`
	}
	return NpcHtmlMessage{ObjectID: obj, HTML: html + `</body></html>`}
}

// whWants hands out reqs one per ask and records what each ask saw.
type whWants struct {
	reqs []WarehouseRequest
	seen []WarehouseStatus
}

func (w *whWants) want(_ *World, st WarehouseStatus, _ time.Time) WarehouseRequest {
	w.seen = append(w.seen, st)
	if len(w.reqs) == 0 {
		return WarehouseRequest{}
	}
	r := w.reqs[0]
	w.reqs = w.reqs[1:]
	return r
}

// newWarehouseBrain is a fighter in clan 77 carrying items, the keeper in
// sight 300 away and its spawn on the travel map.
func newWarehouseBrain(t *testing.T, wants *whWants, items ...Item) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance, p.AutoShots = 0, false
	p.Warehouse.Want = wants.want
	b, _ := newTestBrain(t, p)
	b.world.Clan.ID = 77
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{keeperNPC: {{X: 300}}}, keepers: map[int32]bool{keeperNPC: true}}
	b.world.Apply(ItemList{Items: items}, t0)
	b.world.Apply(NPCInfo{ObjectID: keeperObj, NPCID: keeperNPC, Pos: Point{X: 300}}, t0)
	return walkOn(t, b)
}

func depositReq(rows ...warehouseRow) []byte {
	return botwire.EncodeWarehouseRequest(opGameSendWareHouseDepositList, rows)
}

func withdrawReq(rows ...warehouseRow) []byte {
	return botwire.EncodeWarehouseRequest(opGameSendWareHouseWithdrawList, rows)
}

// openSteps talks to the keeper and opens its clan warehouse from start.
func openSteps(start time.Duration, deposit bool) []trainerStep {
	cmd := "WithdrawC"
	if deposit {
		cmd = "DepositC"
	}
	return []trainerStep{
		{name: "select the keeper", at: start, wantRule: "warehouse", wantKind: actSelect},
		{name: "talk", at: start + time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "warehouse", wantKind: actTalk},
		{name: "wait for the page", at: start + 2*time.Second, wantRule: "warehouse"},
		{
			name: "open", at: start + 3*time.Second, events: []Event{keeperPage(keeperObj, "DepositP", "WithdrawP", "DepositC", "WithdrawC")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer(clanWarehouseCommand(keeperObj, deposit)),
		},
		{name: "wait for the " + cmd + " list", at: start + 4*time.Second, wantRule: "warehouse"},
	}
}

// TestBrainWarehouseDeposit deposits the carried skins: one open, one
// request after the deposit list, then no open before MinOpenGap.
func TestBrainWarehouseDeposit(t *testing.T) {
	wants := &whWants{reqs: []WarehouseRequest{{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}, {Deposit: []WarehouseRow{{TemplateID: leatherShirt}}}}}
	b := newWarehouseBrain(t, wants, adenaStack(1000), whCarriedSkins, whCarriedShirt)
	steps := openSteps(0, true)
	steps = append(steps,
		trainerStep{
			name: "deposit what the list shows", at: 5 * time.Second,
			events: []Event{WarehouseDepositList{Type: clanWarehouseType, Adena: 1000, Items: []WarehouseItem{
				{ObjectID: 102, TemplateID: animalSkin, Count: 10}, {ObjectID: 104, TemplateID: leatherShirt, Count: 1},
			}}},
			wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: depositReq(warehouseRow{Object: 102, Item: animalSkin, Count: 10}),
		},
		trainerStep{name: "wait for the deposit", at: 6 * time.Second, wantRule: "warehouse"},
		trainerStep{
			name: "deposited", at: 7 * time.Second,
			events: []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: whCarriedSkins}, {State: ItemModified, Item: adenaStack(970)}}}},
		},
		trainerStep{name: "no open before MinOpenGap", at: 3*time.Second + warehouseOpenGap - time.Millisecond},
		trainerStep{name: "open due again", at: 3*time.Second + warehouseOpenGap, wantRule: "warehouse", wantKind: actTalk},
	)
	runTrainerSteps(t, b, steps)
	if got := b.wh.status.Deposit; got.Outcome != WarehouseDone || !got.At.Equal(at(7*time.Second)) ||
		!reflect.DeepEqual(got.Rows, []WarehouseRow{{TemplateID: animalSkin, Count: 10, Object: 102}}) {
		t.Fatalf("deposit result = %+v, want done at 7s, moving the skins", got)
	}
	if b.wh.status.Known {
		t.Fatalf("contents known with no withdrawal list seen")
	}
}

// TestBrainWarehouseWithdrawThenDeposit withdraws only what the fresh list
// shows, then deposits at once: the deposit after its own withdrawal waits
// for no MinOpenGap.
func TestBrainWarehouseWithdrawThenDeposit(t *testing.T) {
	wants := &whWants{reqs: []WarehouseRequest{{
		Withdraw: []WarehouseRow{{TemplateID: animalSkin, Count: 25}, {TemplateID: 9999, Count: 1}},
		Deposit:  []WarehouseRow{{TemplateID: leatherShirt}},
	}}}
	b := newWarehouseBrain(t, wants, adenaStack(1000), whCarriedShirt)
	steps := openSteps(0, false)
	steps = append(steps,
		trainerStep{
			name: "withdraw no more than listed", at: 5 * time.Second,
			events:   []Event{WarehouseWithdrawList{Type: clanWarehouseType, Adena: 1000, Items: []WarehouseItem{whStoredSkins, whStoredSword}}},
			wantRule: "warehouse", wantKind: actWarehouseWithdraw, wantPayload: withdrawReq(warehouseRow{Object: whStoredSkins.ObjectID, Item: animalSkin, Count: 20}),
		},
		trainerStep{name: "wait for the withdrawal", at: 6 * time.Second, wantRule: "warehouse"},
		trainerStep{
			name: "withdrawn: talk again for the deposit", at: 7 * time.Second,
			events:   []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: Item{ObjectID: 105, TemplateID: animalSkin, Count: 20}}}}},
			wantRule: "warehouse", wantKind: actTalk,
		},
		trainerStep{
			name: "deposit open at once", at: 8 * time.Second, events: []Event{keeperPage(keeperObj, "DepositC", "WithdrawC")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer(clanWarehouseCommand(keeperObj, true)),
		},
		trainerStep{
			name: "deposit", at: 9 * time.Second,
			events:   []Event{WarehouseDepositList{Type: clanWarehouseType, Adena: 1000, Items: []WarehouseItem{{ObjectID: 104, TemplateID: leatherShirt, Count: 1}}}},
			wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: depositReq(warehouseRow{Object: 104, Item: leatherShirt, Count: 1}),
		},
		trainerStep{name: "deposited", at: 10 * time.Second, events: []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: whCarriedShirt}}}}},
		trainerStep{name: "no open before MinOpenGap since the deposit's", at: 8*time.Second + warehouseOpenGap - time.Millisecond},
	)
	runTrainerSteps(t, b, steps)
	st := b.wh.status
	if st.Withdraw.Outcome != WarehouseDone || st.Deposit.Outcome != WarehouseDone {
		t.Fatalf("results = %+v / %+v, want both done", st.Withdraw, st.Deposit)
	}
	want := []WarehouseItem{whStoredSword, {TemplateID: leatherShirt, Count: 1}}
	if !st.Known || !reflect.DeepEqual(st.Contents, want) {
		t.Fatalf("contents = %v %+v, want %+v", st.Known, st.Contents, want)
	}
}

// TestBrainWarehouseEmpty takes NoItemDepositedInWH for an empty warehouse,
// not a failure, and goes on to the deposit.
func TestBrainWarehouseEmpty(t *testing.T) {
	wants := &whWants{reqs: []WarehouseRequest{{Withdraw: []WarehouseRow{{TemplateID: animalSkin}}, Deposit: []WarehouseRow{{TemplateID: leatherShirt}}}}}
	b := newWarehouseBrain(t, wants, adenaStack(1000), whCarriedShirt)
	b.wh.status = WarehouseStatus{Contents: []WarehouseItem{whStoredSkins}, Known: true}
	steps := openSteps(0, false)
	steps = append(steps,
		trainerStep{
			name: "empty: talk again for the deposit", at: 5 * time.Second,
			events: []Event{SystemMessage{ID: msgNoItemDepositedInW}}, wantRule: "warehouse", wantKind: actTalk,
		},
		trainerStep{
			name: "deposit open", at: 6 * time.Second, events: []Event{keeperPage(keeperObj, "DepositC")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer(clanWarehouseCommand(keeperObj, true)),
		},
	)
	runTrainerSteps(t, b, steps)
	st := b.wh.status
	if st.Withdraw.Outcome != WarehouseEmpty || !st.Known || len(st.Contents) != 0 {
		t.Fatalf("status = %+v, want an empty warehouse known", st)
	}
}

// TestBrainWarehouseRefused ends the transaction on each answered refusal,
// at the open or at the request, with its system message.
func TestBrainWarehouseRefused(t *testing.T) {
	tests := []struct {
		msg     int32
		deposit bool
		// atRequest refuses the request, else the open.
		atRequest bool
	}{
		{msgNoRightClanWH, false, false},
		{msgClanLevelForWH, true, false},
		{msgAlreadyTrading, true, true},
		{msgOverCapacity, true, true},
		{msgNotEnoughAdena, true, true},
		{msgSlotsFull, false, true},
		{msgWeightLimit, false, true},
		{msgLeaderOnlyWithdraw, false, true},
	}
	for _, tt := range tests {
		t.Run(itoa(tt.msg), func(t *testing.T) {
			req := WarehouseRequest{Withdraw: []WarehouseRow{{TemplateID: animalSkin}}}
			list := Event(WarehouseWithdrawList{Type: clanWarehouseType, Items: []WarehouseItem{whStoredSkins}})
			kind := actWarehouseWithdraw
			if tt.deposit {
				req = WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}
				list = WarehouseDepositList{Type: clanWarehouseType, Items: []WarehouseItem{{ObjectID: 102, TemplateID: animalSkin, Count: 10}}}
				kind = actWarehouseDeposit
			}
			b := newWarehouseBrain(t, &whWants{reqs: []WarehouseRequest{req}}, adenaStack(1000), whCarriedSkins)
			steps := openSteps(0, tt.deposit)
			if tt.atRequest {
				steps = append(steps,
					trainerStep{name: "request", at: 5 * time.Second, events: []Event{list}, wantRule: "warehouse", wantKind: kind},
					trainerStep{name: "refused", at: 6 * time.Second, events: []Event{SystemMessage{ID: tt.msg}}},
				)
			} else {
				steps = append(steps, trainerStep{name: "refused", at: 5 * time.Second, events: []Event{SystemMessage{ID: tt.msg}}})
			}
			runTrainerSteps(t, b, steps)
			got := b.wh.status.Withdraw
			if tt.deposit {
				got = b.wh.status.Deposit
			}
			if got.Outcome != WarehouseRefused || got.Msg != tt.msg || b.wh.running {
				t.Fatalf("result = %+v running %v, want refused by %d and the errand over", got, b.wh.running, tt.msg)
			}
		})
	}
}

// TestWarehouseDepositChecks checks a deposit before it goes out: the fee
// of 30 adena a row, an open trade, and the slots of new kinds when the
// contents are known.
func TestWarehouseDepositChecks(t *testing.T) {
	full := make([]WarehouseItem, clanWarehouseSlots)
	for i := range full {
		full[i] = WarehouseItem{ObjectID: int32(0x10000400 + i), TemplateID: int32(5000 + i), Count: 1}
	}
	fullWithSkins := append([]WarehouseItem{{ObjectID: 0x10000300, TemplateID: animalSkin, Count: 5}}, full[1:]...)
	both := []WarehouseRow{{TemplateID: animalSkin}, {TemplateID: leatherShirt}}
	tests := []struct {
		name     string
		adena    int32
		trade    bool
		contents []WarehouseItem
		known    bool
		wantRows []warehouseRow
		want     WarehouseOutcome
		wantMsg  int32
	}{
		{name: "fee paid", adena: 60, wantRows: []warehouseRow{{Object: 102, Item: animalSkin, Count: 10}, {Object: 104, Item: leatherShirt, Count: 1}}},
		{name: "fee short", adena: 59, want: WarehouseShort},
		{name: "open trade", adena: 1000, trade: true, want: WarehouseRefused, wantMsg: msgAlreadyTrading},
		{name: "full: the merging stack only", adena: 1000, contents: fullWithSkins, known: true, wantRows: []warehouseRow{{Object: 102, Item: animalSkin, Count: 10}}},
		{name: "full: no new kind", adena: 1000, contents: full, known: true, want: WarehouseFull},
		{name: "one slot free", adena: 1000, contents: full[1:], known: true, wantRows: []warehouseRow{{Object: 102, Item: animalSkin, Count: 10}}},
		{name: "contents unknown: no slot check", adena: 1000, contents: full, wantRows: []warehouseRow{{Object: 102, Item: animalSkin, Count: 10}, {Object: 104, Item: leatherShirt, Count: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newWarehouseBrain(t, &whWants{}, adenaStack(tt.adena), whCarriedSkins, whCarriedShirt)
			if tt.trade {
				b.world.Trade = Trade{PartnerID: 9}
			}
			b.wh.req = WarehouseRequest{Deposit: both}
			b.wh.status = WarehouseStatus{Contents: tt.contents, Known: tt.known}
			rows, o, msg := b.depositRows(b.carriedStock())
			if o != tt.want || msg != tt.wantMsg || !reflect.DeepEqual(rows, tt.wantRows) {
				t.Fatalf("depositRows = %v, %d, %d; want %v, %d, %d", rows, o, msg, tt.wantRows, tt.want, tt.wantMsg)
			}
		})
	}
}

// TestWarehouseCheckedBeforeOpen ends a deposit its carried items fail
// before any open, and asks again only MinOpenGap later.
func TestWarehouseCheckedBeforeOpen(t *testing.T) {
	wants := &whWants{reqs: []WarehouseRequest{{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}, {Deposit: []WarehouseRow{{TemplateID: animalSkin}}}}}
	b := newWarehouseBrain(t, wants, adenaStack(29), whCarriedSkins)
	runTrainerSteps(t, b, []trainerStep{
		{name: "fee short: no open"},
		{name: "not asked again", at: warehouseOpenGap - time.Millisecond},
	})
	if got := b.wh.status.Deposit.Outcome; got != WarehouseShort || len(wants.seen) != 1 {
		t.Fatalf("deposit = %d after %d asks, want short after one", got, len(wants.seen))
	}
	runTrainerSteps(t, b, []trainerStep{{name: "asked again", at: warehouseOpenGap, setup: func(b *Brain) {
		b.world.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: adenaStack(30)}}}, t0)
	}, wantRule: "warehouse", wantKind: actSelect}})
}

// TestBrainWarehouseTimeout takes an open the server never answers as a
// silent refusal: the request ends and the next open waits twice
// MinOpenGap.
func TestBrainWarehouseTimeout(t *testing.T) {
	req := WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}
	wants := &whWants{reqs: []WarehouseRequest{req, req}}
	b := newWarehouseBrain(t, wants, adenaStack(1000), whCarriedSkins)
	steps := openSteps(0, true)
	steps = append(steps,
		trainerStep{name: "still waiting", at: 3*time.Second + warehouseReplyTimeout - time.Millisecond, wantRule: "warehouse"},
		trainerStep{name: "unanswered", at: 3*time.Second + warehouseReplyTimeout},
		trainerStep{name: "backed off past MinOpenGap", at: 3*time.Second + 2*warehouseOpenGap - time.Millisecond},
		trainerStep{name: "open due", at: 3*time.Second + 2*warehouseOpenGap, wantRule: "warehouse", wantKind: actTalk},
	)
	runTrainerSteps(t, b, steps)
	if got := b.wh.status.Deposit.Outcome; got != WarehouseTimeout || b.wh.timeouts != 1 {
		t.Fatalf("deposit = %d, timeouts %d; want a timeout counted", got, b.wh.timeouts)
	}
}

// TestBrainWarehouseRequestTimeoutBackoff keeps counting requests the
// server never answers after a list: the list is no answer to the
// transaction, so two silent refusals in a row wait four times MinOpenGap.
func TestBrainWarehouseRequestTimeoutBackoff(t *testing.T) {
	req := WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}
	wants := &whWants{reqs: []WarehouseRequest{req, req, req}}
	b := newWarehouseBrain(t, wants, adenaStack(1000), whCarriedSkins)
	list := WarehouseDepositList{Type: clanWarehouseType, Adena: 1000, Items: []WarehouseItem{{ObjectID: 102, TemplateID: animalSkin, Count: 10}}}
	sent := depositReq(warehouseRow{Object: 102, Item: animalSkin, Count: 10})
	second := 3*time.Second + 2*warehouseOpenGap
	steps := openSteps(0, true)
	steps = append(steps,
		trainerStep{name: "deposit", at: 5 * time.Second, events: []Event{list}, wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: sent},
		trainerStep{name: "deposit unanswered", at: 5*time.Second + warehouseReplyTimeout},
		trainerStep{name: "talk again", at: second, wantRule: "warehouse", wantKind: actTalk},
		trainerStep{
			name: "open again", at: second + time.Second, events: []Event{keeperPage(keeperObj, "DepositC", "WithdrawC")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer(clanWarehouseCommand(keeperObj, true)),
		},
		trainerStep{name: "deposit again", at: second + 2*time.Second, events: []Event{list}, wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: sent},
		trainerStep{name: "unanswered again", at: second + 2*time.Second + warehouseReplyTimeout},
		trainerStep{name: "backed off past twice MinOpenGap", at: second + time.Second + 4*warehouseOpenGap - time.Millisecond},
		trainerStep{name: "open due", at: second + time.Second + 4*warehouseOpenGap, wantRule: "warehouse", wantKind: actTalk},
	)
	runTrainerSteps(t, b, steps)
	if b.wh.timeouts != 2 {
		t.Fatalf("timeouts = %d, want the two unanswered requests counted", b.wh.timeouts)
	}
}

// TestWarehouseOpenGapBackoff doubles the gap per timeout in a row, up to
// warehouseBackoffMax.
func TestWarehouseOpenGapBackoff(t *testing.T) {
	tests := []struct {
		timeouts int
		gap      time.Duration
	}{{0, time.Minute}, {1, 2 * time.Minute}, {2, 4 * time.Minute}, {4, 16 * time.Minute}, {5, warehouseBackoffMax}, {9, warehouseBackoffMax}}
	for _, tt := range tests {
		e := warehouseErrand{openedAt: t0, timeouts: tt.timeouts}
		if e.openDue(t0.Add(tt.gap-time.Millisecond)) || !e.openDue(t0.Add(tt.gap)) {
			t.Errorf("%d timeouts: open not due exactly %v after the last", tt.timeouts, tt.gap)
		}
	}
	if e := (warehouseErrand{openedAt: t0, exempt: true}); !e.openDue(t0) {
		t.Error("the deposit after its own withdrawal waits")
	}
}

// TestBrainWarehouseChatPages follows a keeper's "Chat" pages to the clan
// link, and leaves out a keeper whose pages have none.
func TestBrainWarehouseChatPages(t *testing.T) {
	t.Run("second page", func(t *testing.T) {
		b := newWarehouseBrain(t, &whWants{reqs: []WarehouseRequest{{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}}}, adenaStack(1000), whCarriedSkins)
		first := keeperPage(keeperObj, "Chat 1", "Chat 2", "Quest")
		runTrainerSteps(t, b, []trainerStep{
			{name: "select", wantRule: "warehouse", wantKind: actSelect},
			{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "warehouse", wantKind: actTalk},
			{name: "first chat link", at: 2 * time.Second, events: []Event{first}, wantRule: "warehouse", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_Chat 1")},
			{name: "freight page: talk again", at: 3 * time.Second, events: []Event{keeperPage(keeperObj, "DepositF", "WithdrawF", "Chat 0")}, wantRule: "warehouse", wantKind: actTalk},
			{name: "next chat link", at: 4 * time.Second, events: []Event{first}, wantRule: "warehouse", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(keeperObj) + "_Chat 2")},
			{name: "clan page: open", at: 5 * time.Second, events: []Event{keeperPage(keeperObj, "DepositC", "WithdrawC", "Chat 0")}, wantRule: "warehouse", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer(clanWarehouseCommand(keeperObj, true))},
		})
	})
	t.Run("no clan warehouse", func(t *testing.T) {
		b := newWarehouseBrain(t, &whWants{reqs: []WarehouseRequest{{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}}}, adenaStack(1000), whCarriedSkins)
		first := keeperPage(keeperObj, "Chat 1", "Quest")
		runTrainerSteps(t, b, []trainerStep{
			{name: "select", wantRule: "warehouse", wantKind: actSelect},
			{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: keeperObj}}, wantRule: "warehouse", wantKind: actTalk},
			{name: "chat link", at: 2 * time.Second, events: []Event{first}, wantRule: "warehouse", wantKind: actBypass},
			{name: "freight page: talk again", at: 3 * time.Second, events: []Event{keeperPage(keeperObj, "DepositF", "Chat 0")}, wantRule: "warehouse", wantKind: actTalk},
			{name: "first page again, nothing left: given up", at: 4 * time.Second, events: []Event{first}},
		})
		if !b.wh.noClan[keeperNPC] || b.wh.status.Deposit.Outcome != WarehouseUnreachable || b.wh.running {
			t.Fatalf("errand %+v, want the keeper left out and the deposit unreachable", b.wh)
		}
	})
}

// TestBrainWarehouseWalk walks to the nearest keeper spawn within Reach
// while none is in sight; none within Reach ends the request.
func TestBrainWarehouseWalk(t *testing.T) {
	req := WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}
	for _, tt := range []struct {
		name  string
		spawn Point
		want  actionKind
		rule  string
	}{
		{"within reach", Point{X: 5000}, actMove, "warehouse"},
		{"out of reach", Point{X: 9000}, actHold, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newWarehouseBrain(t, &whWants{reqs: []WarehouseRequest{req}}, adenaStack(1000), whCarriedSkins)
			b.world.Apply(DeleteObject{ObjectID: keeperObj}, t0)
			b.cfg.Travel.spawns[keeperNPC] = []Point{tt.spawn}
			runTrainerSteps(t, b, []trainerStep{{name: "start", wantRule: tt.rule, wantKind: tt.want}})
			if tt.want == actHold && b.wh.status.Deposit.Outcome != WarehouseUnreachable {
				t.Fatalf("deposit = %+v, want unreachable", b.wh.status.Deposit)
			}
		})
	}
}

// TestWarehouseNotDue runs no errand outside a clan, with no Want or no
// keeper on the map.
func TestWarehouseNotDue(t *testing.T) {
	req := WarehouseRequest{Deposit: []WarehouseRow{{TemplateID: animalSkin}}}
	for _, tt := range []struct {
		name  string
		setup func(b *Brain)
	}{
		{"no clan", func(b *Brain) { b.world.Clan = Clan{} }},
		{"no Want", func(b *Brain) { b.persona.Warehouse.Want = nil }},
		{"no keeper", func(b *Brain) { b.cfg.Travel.keepers = nil }},
		{"in combat", func(b *Brain) { b.world.Self.InCombat = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newWarehouseBrain(t, &whWants{reqs: []WarehouseRequest{req}}, adenaStack(1000), whCarriedSkins)
			tt.setup(b)
			if a, ok := b.ruleWarehouse(t0); ok || b.wh.running {
				t.Fatalf("ruleWarehouse = %+v, %v; want no errand", a, ok)
			}
		})
	}
}
