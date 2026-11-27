package botclient

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// npcTypeWarehouseKeeper is the NPC template type of a warehouse keeper
// (npcs/*.xml; model/actor/npc/folk_warehouse.go warehouseKinds).
const npcTypeWarehouseKeeper = "WarehouseKeeper"

// Clan warehouse errand limits.
const (
	// warehouseOpenGap is MinOpenGap: the least time between two clan
	// warehouse opens of a bot (docs/crafting.md, "Pacing"), except the
	// deposit that follows its own withdrawal. Every unanswered open or
	// request in a row doubles it, up to warehouseBackoffMax.
	warehouseOpenGap    = 60 * time.Second
	warehouseBackoffMax = 30 * time.Minute
	// warehouseReplyTimeout is how long an open or a request waits for its
	// answer. The server refuses in silence a keeper out of reach or not
	// last talked to, karma, a withdrawal without the privilege, and a
	// withdrawal row with a stale object id or a count above what is held
	// (network/warehouse.go warehouseRequestGate, clanWithdrawGate).
	warehouseReplyTimeout = 5 * time.Second
	// warehouseFee is the fee of a deposit, per requested row, rows the
	// server then skips included (network/warehouse.go depositFeePerItem).
	warehouseFee = 30
	// warehouseMaxRows is MaxItemInPacket on the shipped config: a request
	// of more rows is discarded in silence
	// (clientpackets/warehouse.go decodeExactItemRequestBatch).
	warehouseMaxRows = 80
	// clanWarehouseSlots is MaximumWarehouseSlotsForClan, the Go server's
	// default (aCis ships 200): the lower bound, so a deposit never counts
	// on slots the server may not have.
	clanWarehouseSlots = 150
	// privateWarehouseSlots and dwarfWarehouseSlots are WarehouseNoDwarf
	// and WarehouseDwarf, a private warehouse's slots
	// (model/actor/player/character_storage_limits.go); the whLimit stat,
	// which the bot does not see, is left out, so the count only undercounts.
	privateWarehouseSlots = 100
	dwarfWarehouseSlots   = 120
	// warehouseChats is how many "Chat" links a visit follows looking for
	// the keeper's clan warehouse links: some keepers link them from a
	// second page (data/html/warehouse/30083.htm), some not at all.
	warehouseChats = 3
)

// The warehouse types of a list (serverpackets/warehouse.go
// WarehousePrivate, WarehouseClan).
const (
	privateWarehouseType uint16 = 1
	clanWarehouseType    uint16 = 2
)

// System messages of the clan warehouse (serverpackets/systemmessage.go).
const (
	msgSlotsFull          = 129
	msgAlreadyTrading     = 142
	msgNotEnoughAdena     = 279
	msgNoItemDepositedInW = 282
	msgWeightLimit        = 422
	msgNoRightClanWH      = 709
	msgClanLevelForWH     = 710
	msgOverCapacity       = 1036
	msgLeaderOnlyWithdraw = 1039
)

// warehouseRefusals are the system messages that answer a clan warehouse
// open or request with a refusal (network/clan_warehouse.go
// clanWarehouseBypass and clanWithdrawGate; network/warehouse.go
// warehouseRequestGate and sendStoreOutcome).
var warehouseRefusals = []int32{
	msgSlotsFull, msgAlreadyTrading, msgNotEnoughAdena, msgWeightLimit, msgNoRightClanWH,
	msgClanLevelForWH, msgOverCapacity, msgLeaderOnlyWithdraw,
}

// WarehouseItem is one row of a warehouse list: a carried item a deposit
// may move, or an item the warehouse holds.
type WarehouseItem struct {
	ObjectID, TemplateID, Count int32
	// Category and SubCategory are the client's item classes, as an Item's.
	Category, SubCategory    uint16
	CustomType1, CustomType2 uint16
	BodySlot                 int32
	Enchant                  uint16
	// Augmentation is the augment's two attribute words, 0 for none.
	Augmentation int64
}

// WarehouseDepositList opens a deposit window: the warehouse type (2 for a
// clan's), the bot's adena and the items it may deposit.
type WarehouseDepositList struct {
	Type  uint16
	Adena int32
	Items []WarehouseItem
}

// WarehouseWithdrawList opens a withdrawal window: the warehouse type (2
// for a clan's), the bot's adena and the items the warehouse holds.
type WarehouseWithdrawList struct {
	Type  uint16
	Adena int32
	Items []WarehouseItem
}

func (WarehouseDepositList) event()  {}
func (WarehouseWithdrawList) event() {}

// warehouseItemSize is the wire size of a warehouse list row.
const warehouseItemSize = 40

// readWarehouseItem reads a list row: category, object id, template id,
// count, subcategory, custom type 1, body slot, enchant, custom type 2, a
// zero, the object id again, augmentation (serverpackets/warehouse.go
// writeWarehouseItem).
func readWarehouseItem(r *wire.Reader) WarehouseItem {
	it := WarehouseItem{
		Category: r.ReadUint16(), ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(),
		Count: r.ReadInt32(), SubCategory: r.ReadUint16(), CustomType1: r.ReadUint16(),
		BodySlot: r.ReadInt32(), Enchant: r.ReadUint16(), CustomType2: r.ReadUint16(),
	}
	r.ReadUint16()
	r.ReadInt32()
	it.Augmentation = r.ReadInt64()
	return it
}

// readWarehouseList reads a list: warehouse type, adena, row count as
// uint16, then the rows (serverpackets/warehouse.go frameWarehouseList).
// The loop ends at the first short read.
func readWarehouseList(r *wire.Reader) (uint16, int32, []WarehouseItem) {
	typ, adena := r.ReadUint16(), r.ReadInt32()
	n := int(r.ReadUint16())
	items := make([]WarehouseItem, 0, capFor(r, n, warehouseItemSize))
	for ; n > 0 && r.Err() == nil; n-- {
		items = append(items, readWarehouseItem(r))
	}
	return typ, adena, items
}

func decodeWarehouseDepositList(r *wire.Reader) Event {
	typ, adena, items := readWarehouseList(r)
	return WarehouseDepositList{Type: typ, Adena: adena, Items: items}
}

func decodeWarehouseWithdrawList(r *wire.Reader) Event {
	typ, adena, items := readWarehouseList(r)
	return WarehouseWithdrawList{Type: typ, Adena: adena, Items: items}
}

// warehouseRow is one row of a deposit or withdrawal request: an item
// object, its template and how many.
type warehouseRow struct{ object, item, count int32 }

// encodeWarehouseRequest is SendWareHouseDepositList (op 0x31) or
// SendWareHouseWithDrawList (op 0x32): row count, then per row object id
// and count, exactly 4+8*rows bytes after the opcode
// (clientpackets/warehouse.go decodeExactItemRequestBatch).
func encodeWarehouseRequest(op byte, rows []warehouseRow) []byte {
	w := wire.NewPacketWriter(op)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(r.object)
		w.WriteInt32(r.count)
	}
	return w.Bytes()
}

// WarehouseRow is one row a clan warehouse errand asks for: an item
// template and how many, 0 for every one the list shows; Object, when set,
// names the one item object of the template it takes.
type WarehouseRow struct{ TemplateID, Count, Object int32 }

// WarehouseRequest is a warehouse errand: a withdrawal of Withdraw, then a
// deposit of Deposit, either one empty for none. Look opens the withdrawal
// list and sends no withdrawal: the list is a look at the contents
// (WarehouseStatus), Withdraw ignored. The deposit right after the
// withdrawal or the look opens the warehouse again at once; any other open
// waits for MinOpenGap (warehouseOpenGap) since the bot's last, whichever
// warehouse it opened.
//
// Private sends it to the bot's own warehouse instead of its clan's, at the
// nearest keeper within Reach of Anchor (docs/valuables/warehouse.md);
// Anchor and Reach are read for a private request only.
type WarehouseRequest struct {
	Withdraw, Deposit []WarehouseRow
	Look              bool
	Private           bool
	Anchor            Point
	Reach             int32
	// stash marks the half-speed stash's deposit (queueStash): its result
	// is also the errand's stashed.
	stash bool
}

// empty reports whether the request asks for nothing.
func (r WarehouseRequest) empty() bool { return !r.Look && len(r.Withdraw) == 0 && len(r.Deposit) == 0 }

// WarehouseOutcome is how a deposit or a withdrawal of an errand ended.
type WarehouseOutcome uint8

// Outcomes of a deposit or a withdrawal.
const (
	// WarehouseNone: none ran yet.
	WarehouseNone WarehouseOutcome = iota
	// WarehouseDone: the server moved the rows (InventoryUpdate).
	WarehouseDone
	// WarehouseEmpty: the warehouse holds nothing (NoItemDepositedInWH).
	WarehouseEmpty
	// WarehouseNothing: the fresh list had none of the requested items.
	WarehouseNothing
	// WarehouseRefused: the server refused, or the bot did for the
	// server's reason (an open trade); Msg is the system message.
	WarehouseRefused
	// WarehouseShort: the bot's adena does not pay the deposit fee.
	WarehouseShort
	// WarehouseFull: the warehouse has no slot for any new kind.
	WarehouseFull
	// WarehouseTimeout: the server answered nothing (a silent refusal).
	WarehouseTimeout
	// WarehouseUnreachable: no keeper with the warehouse asked for was
	// reached.
	WarehouseUnreachable
	// WarehouseListed: a look got its fresh list (or the empty answer is
	// WarehouseEmpty); no withdrawal went out.
	WarehouseListed
)

// WarehouseResult is the end of the last deposit or withdrawal: its
// outcome, the system message of a refusal, and when; Rows are the rows a
// WarehouseDone moved.
type WarehouseResult struct {
	Outcome WarehouseOutcome
	Msg     int32
	At      time.Time
	Rows    []WarehouseRow
}

// WarehouseStatus is what the errand knows of one warehouse, the clan's or
// the bot's own: the last result of each kind, and the warehouse as the last
// withdrawal list or empty answer showed it (Known false before one), with
// what the bot moved since.
type WarehouseStatus struct {
	Withdraw, Deposit WarehouseResult
	Contents          []WarehouseItem
	Known             bool
}

// WarehousePolicy is the clan warehouse errand (rule 9 errand
// "warehouse"). Want is asked for an errand when none runs, the bot is in a
// clan and an open is due; a zero request asks for none, and nil never
// runs the errand. A crafter (Persona.Crafter) asks its cycle's requests
// instead, ahead of Want. Reach is how far the nearest warehouse keeper's spawn
// may be (from the travel map, BrainConfig.Travel); 0 visits only a keeper
// in sight.
type WarehousePolicy struct {
	Want  func(w *World, st WarehouseStatus, now time.Time) WarehouseRequest
	Reach int32
}

func defaultWarehousePolicy() WarehousePolicy { return WarehousePolicy{Reach: 8000} }

// whStep is where a warehouse transaction stands.
type whStep uint8

const (
	// whTalk talks to the keeper and follows its pages to the open link.
	whTalk whStep = iota
	// whOpening waits for the list the open link opens.
	whOpening
	// whListed has the fresh list: the request goes out.
	whListed
	// whSent waits for the request's answer.
	whSent
)

// warehouseErrand is the bot's warehouse errand (rule 9 errand
// "warehouse"), clan or private. It is owned by the brain goroutine. One
// transaction is one open (a clan or private link of the keeper's page)
// and one request.
type warehouseErrand struct {
	v   npcVisit
	req WarehouseRequest
	// privateWant is the one private request queued for the next start,
	// ahead of the clan's (startWarehouse); empty for none.
	privateWant WarehouseRequest
	// running is set while a request runs.
	running bool
	step    whStep
	// next is the step the bypass the rule returned starts once sent.
	next whStep
	// chats are the "Chat" links the visit followed; talked is set from a
	// talk to the keeper until its page, and firstPage while the open page
	// is the one a talk opened.
	chats     []string
	talked    bool
	firstPage bool
	// list is the fresh list of the open; rows the request sent.
	list []WarehouseItem
	rows []warehouseRow
	// openedAt is the bot's last open, zero before; exempt lets the deposit
	// after its own withdrawal open at once; timeouts counts unanswered
	// opens and requests in a row, reset only by an answer that ends a
	// transaction (a list is none).
	openedAt time.Time
	exempt   bool
	timeouts int
	// status is the clan warehouse's, private the bot's own.
	status, private WarehouseStatus
	// stashed is how the last half-speed stash ended (queueStash), zero
	// before one: its deposit's result, or the outcome that kept it from
	// starting.
	stashed WarehouseResult
	// shunned are the keeper templates not at their spawn, until a time;
	// noClan the ones whose pages link no clan warehouse, noPrivate no
	// private one. noPrivate may be handed in: it is read and filled in
	// place.
	shunned   holds[int32]
	noClan    map[int32]bool
	noPrivate map[int32]bool
	// retryAt is when a request may start after one that ended before an
	// open: no keeper, or a deposit failing its checks.
	retryAt time.Time
}

// depositing reports whether the transaction under way is the deposit.
func (e *warehouseErrand) depositing() bool { return len(e.req.Withdraw) == 0 && !e.req.Look }

// statusOf is the status of req's warehouse: the bot's own for a private
// request, else the clan's.
func (e *warehouseErrand) statusOf(req WarehouseRequest) *WarehouseStatus {
	if req.Private {
		return &e.private
	}
	return &e.status
}

// openDue reports whether an open may go out at now: MinOpenGap after the
// last, doubled per timeout in a row, or at once for an exempt deposit.
func (e *warehouseErrand) openDue(now time.Time) bool {
	if e.exempt || e.openedAt.IsZero() {
		return true
	}
	gap := warehouseOpenGap
	for range min(e.timeouts, 5) {
		gap *= 2
	}
	return !now.Before(e.openedAt.Add(min(gap, warehouseBackoffMax)))
}

// result records the end of the transaction under way.
func (e *warehouseErrand) result(o WarehouseOutcome, msg int32, now time.Time) {
	r := WarehouseResult{Outcome: o, Msg: msg, At: now}
	if o == WarehouseDone {
		for _, row := range e.rows {
			r.Rows = append(r.Rows, WarehouseRow{TemplateID: row.item, Count: row.count, Object: row.object})
		}
	}
	if st := e.statusOf(e.req); e.depositing() {
		st.Deposit = r
		if e.req.stash {
			e.stashed = r
		}
	} else {
		st.Withdraw = r
	}
}

// endTransaction ends the transaction under way with outcome o; after a
// withdrawal the deposit, if any, runs next on the same keeper.
func (e *warehouseErrand) endTransaction(o WarehouseOutcome, msg int32, now time.Time) {
	e.result(o, msg, now)
	e.step, e.list, e.rows, e.chats, e.talked, e.firstPage = whTalk, nil, nil, nil, false, false
	e.v.opened, e.v.waitUntil = false, time.Time{}
	if !e.depositing() {
		e.req.Withdraw, e.req.Look = nil, false
		e.exempt = len(e.req.Deposit) > 0
		if e.exempt {
			return
		}
	}
	e.end()
}

// end ends the request.
func (e *warehouseErrand) end() {
	e.v, e.req, e.running, e.exempt = npcVisit{}, WarehouseRequest{}, false, false
	e.step, e.list, e.rows, e.chats, e.talked, e.firstPage = whTalk, nil, nil, nil, false, false
}

// fail ends the whole request with outcome o (and system message msg) for
// each part left; the next request waits MinOpenGap.
func (e *warehouseErrand) fail(o WarehouseOutcome, msg int32, now time.Time) {
	e.retryAt = now.Add(warehouseOpenGap)
	e.giveUp(o, msg, now)
}

// giveUp ends the whole request with outcome o (and system message msg) for
// each part left, setting no retry time.
func (e *warehouseErrand) giveUp(o WarehouseOutcome, msg int32, now time.Time) {
	st := e.statusOf(e.req)
	if len(e.req.Withdraw) > 0 || e.req.Look {
		st.Withdraw = WarehouseResult{Outcome: o, Msg: msg, At: now}
	}
	if len(e.req.Deposit) > 0 {
		st.Deposit = WarehouseResult{Outcome: o, Msg: msg, At: now}
		if e.req.stash {
			e.stashed = st.Deposit
		}
	}
	e.end()
}

// cancelPrivate ends the running private request before its request
// packet went out (the round's cut-off, docs/valuables/round-end.md): at
// the walk, the talk, the open link sent or the list received. It records
// no result and sets no retry time; a sent open keeps its openedAt and its
// late list or empty answer is dropped (warehouseEvent). A request already
// sent, or a clan request, keeps running. It reports whether it ended one.
func (e *warehouseErrand) cancelPrivate() bool {
	if !e.running || !e.req.Private || e.step == whSent {
		return false
	}
	e.end()
	return true
}

// sent records a packet of the errand that went out at now.
func (e *warehouseErrand) sent(a Action, now time.Time) {
	v := &e.v
	if !e.running || v.obj == 0 || a.object != v.obj {
		return
	}
	v.sent(a, now)
	switch a.kind {
	case actTalk:
		e.talked = true
	case actBypass:
		if e.next == whOpening {
			e.step, e.openedAt, e.exempt = whOpening, now, false
			v.waitUntil = now.Add(warehouseReplyTimeout)
		}
		e.next = whTalk
	case actWarehouseDeposit, actWarehouseWithdraw:
		e.step = whSent
		v.waitUntil = now.Add(warehouseReplyTimeout)
	}
}

// clanWarehouseCommand is keeper obj's clan deposit or withdrawal link
// (folk_warehouse.go warehouseCommand: DepositC and WithdrawC, exact).
func clanWarehouseCommand(obj int32, deposit bool) string {
	cmd := "WithdrawC"
	if deposit {
		cmd = "DepositC"
	}
	return npcBypass(obj, cmd)
}

// openLink is the link of the open page that opens the transaction under
// way at keeper obj, "" for none: the clan link, or the private deposit
// link (DepositP, exact) or the first private withdrawal link (WithdrawP, a
// prefix; folk_warehouse.go warehouseCommand).
func (b *Brain) openLink(obj int32) string {
	e, d := &b.wh, &b.world.Dialog
	if !e.req.Private {
		if cmd := clanWarehouseCommand(obj, e.depositing()); d.Has(cmd) {
			return cmd
		}
		return ""
	}
	if e.depositing() {
		if cmd := npcBypass(obj, "DepositP"); d.Has(cmd) {
			return cmd
		}
		return ""
	}
	withdraw := npcBypass(obj, "WithdrawP")
	for _, l := range d.Links {
		if strings.HasPrefix(l, withdraw) {
			return l
		}
	}
	return ""
}

// ruleWarehouse is a rule 9 errand: with a request from
// Persona.Warehouse.Want (or a queued private one), the bot walks to the
// nearest warehouse keeper, talks to it, follows its pages to the clan (or
// private) warehouse link, opens the warehouse and sends one request per
// open, the withdrawal before the deposit. Opens are at least MinOpenGap
// (warehouseOpenGap) apart, except the deposit after its own withdrawal,
// and one request goes out per open, after its list; links pass the bypass
// gate (gateBypass); legs and clicks the frame pacer, a visit spending at
// most visitClicks clicks.
func (b *Brain) ruleWarehouse(now time.Time) (Action, bool) {
	if b.world.Self.InCombat {
		return Action{}, false
	}
	if !b.wh.running {
		b.queueStash(now)
	}
	if !b.wh.running && !b.startWarehouse(now) {
		return Action{}, false
	}
	return b.stepWarehouse(now)
}

// stepWarehouse steps the running request, under rule "warehouse" or the
// rule that started it, with ruleWarehouse's pacing.
func (b *Brain) stepWarehouse(now time.Time) (Action, bool) {
	e := &b.wh
	if !e.running {
		return Action{}, false
	}
	v := &e.v
	switch e.step {
	case whOpening, whSent:
		if now.Before(v.waitUntil) {
			return Action{}, true
		}
		// Unanswered: a silent refusal. The request ends and the next
		// open waits longer.
		e.timeouts++
		e.fail(WarehouseTimeout, 0, now)
		return Action{}, false
	case whListed:
		return b.warehouseRequest(now)
	}
	if v.opened {
		return b.warehouseLink(now)
	}
	a, ok := b.approach(v, now)
	if !ok {
		e.shunned.set(v.npcID, now.Add(visitShunFor))
		e.fail(WarehouseUnreachable, 0, now)
		return Action{}, false
	}
	return a, true
}

// warehouseLink is the next link of the keeper's open page: the open link
// (openLink) once an open is due, else a "Chat" link not followed yet. A
// keeper whose first page leads to no open link has no warehouse of the
// kind: it is marked (noClan, noPrivate) and the request ends, to be asked
// again at another keeper. A private request ended so sets no retry time:
// no open went out.
func (b *Brain) warehouseLink(now time.Time) (Action, bool) {
	e := &b.wh
	v := &e.v
	if cmd := b.openLink(v.obj); cmd != "" {
		if !e.openDue(now) {
			return Action{}, true
		}
		e.next = whOpening
		return bypassTo(v.obj, cmd), true
	}
	if len(e.chats) < warehouseChats {
		for _, l := range b.world.Dialog.Links {
			if strings.HasPrefix(l, npcBypass(v.obj, "Chat ")) && !strings.HasSuffix(l, " 0") && !slices.Contains(e.chats, l) {
				e.chats = append(e.chats, l)
				return bypassTo(v.obj, l), true
			}
		}
	}
	if !e.firstPage && len(e.chats) < warehouseChats {
		// A second page without the link: back to the first for the next.
		v.opened = false
		return b.approach(v, now)
	}
	if e.req.Private {
		if e.noPrivate == nil {
			e.noPrivate = make(map[int32]bool)
		}
		e.noPrivate[v.npcID] = true
		e.giveUp(WarehouseUnreachable, 0, now)
		return Action{}, false
	}
	if e.noClan == nil {
		e.noClan = make(map[int32]bool)
	}
	e.noClan[v.npcID] = true
	e.fail(WarehouseUnreachable, 0, now)
	return Action{}, false
}

// warehouseRequest is the request of the fresh list, or ends the
// transaction when the list or a check leaves nothing to send.
func (b *Brain) warehouseRequest(now time.Time) (Action, bool) {
	e := &b.wh
	stock := make([]whStock, 0, len(e.list))
	for _, it := range e.list {
		stock = append(stock, whStock{it.ObjectID, it.TemplateID, it.Count})
	}
	if e.req.Look {
		e.endTransaction(WarehouseListed, 0, now)
		return Action{}, false
	}
	if !e.depositing() {
		rows := pickRows(e.req.Withdraw, stock)
		if len(rows) == 0 {
			e.endTransaction(WarehouseNothing, 0, now)
			return Action{}, false
		}
		e.rows = rows
		return Action{kind: actWarehouseWithdraw, payload: encodeWarehouseRequest(opGameSendWareHouseWithdrawList, rows), object: e.v.obj}, true
	}
	rows, o, msg := b.depositRows(stock)
	if o != WarehouseNone {
		e.endTransaction(o, msg, now)
		return Action{}, false
	}
	e.rows = rows
	return Action{kind: actWarehouseDeposit, payload: encodeWarehouseRequest(opGameSendWareHouseDepositList, rows), object: e.v.obj}, true
}

// whStock is an item a request may pick rows from.
type whStock struct{ object, item, count int32 }

// pickRows picks the rows of want from stock, in want's order then
// stock's, each count no more than stock holds, a row naming an object
// that object only, at most warehouseMaxRows.
func pickRows(want []WarehouseRow, stock []whStock) []warehouseRow {
	var rows []warehouseRow
	for _, w := range want {
		left := w.Count
		for _, s := range stock {
			if s.item != w.TemplateID || s.count <= 0 || (w.Count > 0 && left <= 0) || (w.Object != 0 && s.object != w.Object) {
				continue
			}
			n := s.count
			if w.Count > 0 {
				n = min(n, left)
				left -= n
			}
			if len(rows) == warehouseMaxRows {
				return rows
			}
			rows = append(rows, warehouseRow{object: s.object, item: s.item, count: n})
		}
	}
	return rows
}

// depositRows picks the deposit's rows from stock and checks them: no open
// trade, a slot for each new kind while the warehouse contents are known
// (rows of new kinds past the free slots are left out; a clan warehouse has
// clanWarehouseSlots, a private one privateWarehouseSlots or, for a dwarf,
// dwarfWarehouseSlots), and adena for the fee on top of any adena
// deposited. A failed check is its outcome.
func (b *Brain) depositRows(stock []whStock) ([]warehouseRow, WarehouseOutcome, int32) {
	e := &b.wh
	if b.world.Trade.Open() {
		return nil, WarehouseRefused, msgAlreadyTrading
	}
	rows := pickRows(e.req.Deposit, stock)
	if len(rows) == 0 {
		return nil, WarehouseNothing, 0
	}
	if st := e.statusOf(e.req); st.Known {
		free := clanWarehouseSlots - len(st.Contents)
		if e.req.Private {
			free = privateWarehouseSlots - len(st.Contents)
			if b.world.Self.Race == RaceDwarf {
				free = dwarfWarehouseSlots - len(st.Contents)
			}
		}
		kept := rows[:0]
		for _, r := range rows {
			if merges(r, st.Contents) {
				kept = append(kept, r)
			} else if free > 0 {
				free--
				kept = append(kept, r)
			}
		}
		if rows = kept; len(rows) == 0 {
			return nil, WarehouseFull, 0
		}
	}
	need := int64(warehouseFee) * int64(len(rows))
	for _, r := range rows {
		if r.item == adenaID {
			need += int64(r.count)
		}
	}
	if int64(b.carried(adenaID)) < need {
		return nil, WarehouseShort, 0
	}
	return rows, WarehouseNone, 0
}

// merges reports whether deposit row r joins a stack the warehouse holds,
// taking no slot: a template it holds, stackable as far as the counts tell
// (more than one on either side). A count of one on both sides is counted
// as a new slot.
func merges(r warehouseRow, contents []WarehouseItem) bool {
	return slices.ContainsFunc(contents, func(it WarehouseItem) bool {
		return it.TemplateID == r.item && (it.Count > 1 || r.count > 1)
	})
}

// startWarehouse starts a request when one is due: an open is due and the
// queued private request (privateWant), else, the bot in a clan, the
// crafter's cycle (warehouseWant) or Persona.Warehouse.Want asks for one.
// A deposit whose carried items fail its checks ends before any open. The
// keeper of a clan request is the nearest one in sight, else the nearest
// spawn within Persona.Warehouse.Reach; of a private request the nearest
// one in sight within the request's Reach of its Anchor, else the nearest
// such spawn. It reports whether a request runs.
func (b *Brain) startWarehouse(now time.Time) bool {
	e, p := &b.wh, &b.persona.Warehouse
	m := b.cfg.Travel
	if (p.Want == nil && !b.crafting() && e.privateWant.empty()) || m == nil || len(m.keepers) == 0 ||
		!e.openDue(now) || now.Before(e.retryAt) {
		return false
	}
	var req WarehouseRequest
	if !e.privateWant.empty() {
		req, e.privateWant = e.privateWant, WarehouseRequest{}
		req.Private = true
	} else if b.world.Clan.ID != 0 {
		req = b.warehouseWant(now)
	}
	if req.empty() {
		return false
	}
	e.req, e.running = req, true
	if e.depositing() {
		if _, o, msg := b.depositRows(b.carriedStock()); o != WarehouseNone {
			e.result(o, msg, now)
			e.end()
			e.retryAt = now.Add(warehouseOpenGap)
			return false
		}
	}
	from := b.world.Self.Position(now)
	reach, anchorReach := int64(p.Reach)*int64(p.Reach), int64(req.Reach)*int64(req.Reach)
	usable := func(npcID int32, at Point) bool {
		if !m.keepers[npcID] || e.shunned.held(npcID, now) {
			return false
		}
		if req.Private {
			return !e.noPrivate[npcID] && dist2(req.Anchor, at) <= anchorReach
		}
		return !e.noClan[npcID]
	}
	var best npcVisit
	var bestDist int64
	for id, o := range b.world.Objects {
		if o.Kind != KindNPC || o.Dead {
			continue
		}
		if at := o.Position(now); usable(o.NPCID, at) {
			if d := dist2(from, at); !best.active() || d < bestDist {
				best, bestDist = npcVisit{npcID: o.NPCID, at: at, obj: id}, d
			}
		}
	}
	if !best.active() {
		for npcID := range m.keepers {
			for _, at := range m.spawns[npcID] {
				if !usable(npcID, at) {
					continue
				}
				if d := dist2(from, at); (req.Private || d <= reach) && (!best.active() || d < bestDist) {
					best, bestDist = npcVisit{npcID: npcID, at: at}, d
				}
			}
		}
	}
	if !best.active() {
		e.fail(WarehouseUnreachable, 0, now)
		return false
	}
	e.v = best
	return true
}

// carriedStock is what the bot carries that a deposit may pick from: its
// unequipped items, by object id.
func (b *Brain) carriedStock() []whStock {
	var stock []whStock
	for _, it := range b.world.Inventory.Items {
		if !it.Equipped {
			stock = append(stock, whStock{it.ObjectID, it.TemplateID, it.Count})
		}
	}
	slices.SortFunc(stock, func(a, b whStock) int { return int(a.object) - int(b.object) })
	return stock
}

// warehouseEvent moves the errand on with the server's answer ev.
func (b *Brain) warehouseEvent(ev Event, now time.Time) {
	e := &b.wh
	if !e.running {
		return
	}
	switch m := ev.(type) {
	case NpcHtmlMessage:
		if e.step == whTalk && e.v.page(m) {
			e.firstPage, e.talked = e.talked, false
		}
	case WarehouseDepositList:
		if e.step == whOpening && e.depositing() && m.Type == e.listType() {
			e.list, e.step, e.v.waitUntil = m.Items, whListed, time.Time{}
		}
	case WarehouseWithdrawList:
		if e.step == whOpening && !e.depositing() && m.Type == e.listType() {
			e.list, e.step, e.v.waitUntil = m.Items, whListed, time.Time{}
			st := e.statusOf(e.req)
			st.Contents, st.Known = slices.Clone(m.Items), true
		}
	case SystemMessage:
		if e.step == whTalk && m.ID == msgAlreadyTrading && len(e.chats) > 0 {
			// The request gate refused a "Chat" link (network/bypass.go,
			// folk_bypass.go): a refusal, not a keeper without the link.
			e.fail(WarehouseRefused, m.ID, now)
			return
		}
		if e.step != whOpening && e.step != whSent {
			return
		}
		switch {
		case m.ID == msgNoItemDepositedInW && e.step == whOpening && !e.depositing():
			e.timeouts = 0
			st := e.statusOf(e.req)
			st.Contents, st.Known = nil, true
			e.endTransaction(WarehouseEmpty, 0, now)
		case slices.Contains(warehouseRefusals, m.ID):
			e.timeouts = 0
			e.endTransaction(WarehouseRefused, m.ID, now)
		}
	case InventoryUpdate:
		if e.step != whSent || !b.movedRows(m) {
			return
		}
		e.timeouts = 0
		b.trackContents()
		if e.req.Private && e.depositing() {
			// Only the valuables round and its off-round errand deposit
			// privately (queueRound).
			b.metrics.ValuablesDeposited()
		}
		e.endTransaction(WarehouseDone, 0, now)
	}
}

// movedRows reports whether update u moved the request's rows: a deposit
// changes or removes one of their objects, a withdrawal adds or changes an
// item of one of their templates.
func (b *Brain) movedRows(u InventoryUpdate) bool {
	e := &b.wh
	for _, c := range u.Changes {
		for _, r := range e.rows {
			if e.depositing() && c.Item.ObjectID == r.object || !e.depositing() && c.State != ItemRemoved && c.Item.TemplateID == r.item {
				return true
			}
		}
	}
	return false
}

// listType is the warehouse type of the lists the request under way
// opens.
func (e *warehouseErrand) listType() uint16 {
	if e.req.Private {
		return privateWarehouseType
	}
	return clanWarehouseType
}

// trackContents applies the rows the server just moved to the known
// contents: a withdrawal takes its counts off, a deposit adds to a stack
// it merges into or adds a row of its kind, a private one with the item's
// object id and enchant (from the deposit list).
func (b *Brain) trackContents() {
	e := &b.wh
	st := e.statusOf(e.req)
	if !st.Known {
		return
	}
	for _, r := range e.rows {
		if !e.depositing() {
			for i := range st.Contents {
				if st.Contents[i].ObjectID == r.object {
					st.Contents[i].Count -= r.count
				}
			}
			continue
		}
		if i := slices.IndexFunc(st.Contents, func(it WarehouseItem) bool { return merges(r, []WarehouseItem{it}) }); i >= 0 {
			st.Contents[i].Count += r.count
			continue
		}
		it := WarehouseItem{TemplateID: r.item, Count: r.count}
		if e.req.Private {
			it.ObjectID = r.object
			if i := slices.IndexFunc(e.list, func(l WarehouseItem) bool { return l.ObjectID == r.object }); i >= 0 {
				it.Enchant = e.list[i].Enchant
			}
		}
		st.Contents = append(st.Contents, it)
	}
	st.Contents = slices.DeleteFunc(st.Contents, func(it WarehouseItem) bool { return it.Count <= 0 })
}
