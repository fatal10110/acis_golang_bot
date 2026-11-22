package botclient

import (
	"slices"
	"strconv"
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
	// warehouseChats is how many "Chat" links a visit follows looking for
	// the keeper's clan warehouse links: some keepers link them from a
	// second page (data/html/warehouse/30083.htm), some not at all.
	warehouseChats = 3
)

// clanWarehouseType is the warehouse type of a clan warehouse list
// (serverpackets/warehouse.go WarehouseClan).
const clanWarehouseType uint16 = 2

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

// WarehouseRequest is a clan warehouse errand: a withdrawal of Withdraw,
// then a deposit of Deposit, either one empty for none. Look opens the
// withdrawal list and sends no withdrawal: the list is a look at the
// contents (WarehouseStatus), Withdraw ignored. The deposit right after the
// withdrawal or the look opens the warehouse again at once; any other open
// waits for MinOpenGap (warehouseOpenGap) since the bot's last.
type WarehouseRequest struct {
	Withdraw, Deposit []WarehouseRow
	Look              bool
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
	// WarehouseUnreachable: no keeper with a clan warehouse was reached.
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

// WarehouseStatus is what the errand knows: the last result of each kind,
// and the clan warehouse as the last withdrawal list or empty answer showed
// it (Known false before one), with what the bot moved since.
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
	// whTalk talks to the keeper and follows its pages to the clan link.
	whTalk whStep = iota
	// whOpening waits for the list the clan link opens.
	whOpening
	// whListed has the fresh list: the request goes out.
	whListed
	// whSent waits for the request's answer.
	whSent
)

// warehouseErrand is the bot's clan warehouse errand (rule 9 errand
// "warehouse"). It is owned by the brain goroutine. One transaction is one
// open (a clan link of the keeper's page) and one request.
type warehouseErrand struct {
	v   npcVisit
	req WarehouseRequest
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
	status   WarehouseStatus
	// shunned are the keeper templates not at their spawn, until a time;
	// noClan the ones whose pages link no clan warehouse.
	shunned map[int32]time.Time
	noClan  map[int32]bool
	// retryAt is when a request may start after one that ended before an
	// open: no keeper, or a deposit failing its checks.
	retryAt time.Time
}

// depositing reports whether the transaction under way is the deposit.
func (e *warehouseErrand) depositing() bool { return len(e.req.Withdraw) == 0 && !e.req.Look }

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
	if e.depositing() {
		e.status.Deposit = r
	} else {
		e.status.Withdraw = r
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

// fail ends the whole request with outcome o for each part left.
func (e *warehouseErrand) fail(o WarehouseOutcome, now time.Time) {
	e.retryAt = now.Add(warehouseOpenGap)
	if len(e.req.Withdraw) > 0 || e.req.Look {
		e.status.Withdraw = WarehouseResult{Outcome: o, At: now}
	}
	if len(e.req.Deposit) > 0 {
		e.status.Deposit = WarehouseResult{Outcome: o, At: now}
	}
	e.end()
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
	cmd := "_WithdrawC"
	if deposit {
		cmd = "_DepositC"
	}
	return "npc_" + strconv.Itoa(int(obj)) + cmd
}

// ruleWarehouse is a rule 9 errand: with a request from
// Persona.Warehouse.Want, the bot walks to the nearest warehouse keeper,
// talks to it, follows its pages to the clan warehouse link, opens the
// warehouse and sends one request per open, the withdrawal before the
// deposit. Opens are at least MinOpenGap (warehouseOpenGap) apart, except
// the deposit after its own withdrawal, and one request goes out per open,
// after its list; links pass the bypass gate (gateBypass); legs and clicks
// the frame pacer, a visit spending at most visitClicks clicks.
func (b *Brain) ruleWarehouse(now time.Time) (Action, bool) {
	e := &b.wh
	if b.world.Self.InCombat || (!e.running && !b.startWarehouse(now)) {
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
		e.fail(WarehouseTimeout, now)
		return Action{}, false
	case whListed:
		return b.warehouseRequest(now)
	}
	if v.opened {
		return b.warehouseLink(now)
	}
	a, ok := b.approach(v, now)
	if !ok {
		if e.shunned == nil {
			e.shunned = make(map[int32]time.Time)
		}
		e.shunned[v.npcID] = now.Add(visitShunFor)
		e.fail(WarehouseUnreachable, now)
		return Action{}, false
	}
	return a, true
}

// warehouseLink is the next link of the keeper's open page: the clan link
// once an open is due, else a "Chat" link not followed yet. A keeper whose
// first page leads to no clan link has no clan warehouse: it is left out
// and the request ends, to be asked again at another keeper.
func (b *Brain) warehouseLink(now time.Time) (Action, bool) {
	e := &b.wh
	v := &e.v
	if cmd := clanWarehouseCommand(v.obj, e.depositing()); b.world.Dialog.Has(cmd) {
		if !e.openDue(now) {
			return Action{}, true
		}
		e.next = whOpening
		return bypassTo(v.obj, cmd), true
	}
	if len(e.chats) < warehouseChats {
		for _, l := range b.world.Dialog.Links {
			if strings.HasPrefix(l, "npc_"+strconv.Itoa(int(v.obj))+"_Chat ") && !strings.HasSuffix(l, " 0") && !slices.Contains(e.chats, l) {
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
	if e.noClan == nil {
		e.noClan = make(map[int32]bool)
	}
	e.noClan[v.npcID] = true
	e.fail(WarehouseUnreachable, now)
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
// (rows of new kinds past the free slots are left out), and adena for the
// fee on top of any adena deposited. A failed check is its outcome.
func (b *Brain) depositRows(stock []whStock) ([]warehouseRow, WarehouseOutcome, int32) {
	e := &b.wh
	if b.world.Trade.Open() {
		return nil, WarehouseRefused, msgAlreadyTrading
	}
	rows := pickRows(e.req.Deposit, stock)
	if len(rows) == 0 {
		return nil, WarehouseNothing, 0
	}
	if st := &e.status; st.Known {
		free := clanWarehouseSlots - len(st.Contents)
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

// startWarehouse starts a request when one is due: the bot is in a clan,
// an open is due and the crafter's cycle (warehouseWant) or
// Persona.Warehouse.Want asks for one. A deposit whose
// carried items fail its checks ends before any open. The keeper is the
// nearest one in sight, else the nearest spawn within Reach. It reports
// whether a request runs.
func (b *Brain) startWarehouse(now time.Time) bool {
	e, p := &b.wh, &b.persona.Warehouse
	m := b.cfg.Travel
	if (p.Want == nil && !b.crafting()) || m == nil || len(m.keepers) == 0 || b.world.Clan.ID == 0 ||
		!e.openDue(now) || now.Before(e.retryAt) {
		return false
	}
	req := b.warehouseWant(now)
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
	usable := func(npcID int32) bool {
		return m.keepers[npcID] && !e.noClan[npcID] && !now.Before(e.shunned[npcID])
	}
	var best npcVisit
	var bestDist int64
	for id, o := range b.world.Objects {
		if o.Kind != KindNPC || o.Dead || !usable(o.NPCID) {
			continue
		}
		at := o.Position(now)
		if d := dist2(from, at); !best.active() || d < bestDist {
			best, bestDist = npcVisit{npcID: o.NPCID, at: at, obj: id}, d
		}
	}
	if !best.active() {
		reach := int64(p.Reach) * int64(p.Reach)
		for npcID := range m.keepers {
			if !usable(npcID) {
				continue
			}
			for _, at := range m.spawns[npcID] {
				if d := dist2(from, at); d <= reach && (!best.active() || d < bestDist) {
					best, bestDist = npcVisit{npcID: npcID, at: at}, d
				}
			}
		}
	}
	if !best.active() {
		e.fail(WarehouseUnreachable, now)
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
		if e.step == whOpening && e.depositing() && m.Type == clanWarehouseType {
			e.list, e.step, e.v.waitUntil = m.Items, whListed, time.Time{}
		}
	case WarehouseWithdrawList:
		if e.step == whOpening && !e.depositing() && m.Type == clanWarehouseType {
			e.list, e.step, e.v.waitUntil = m.Items, whListed, time.Time{}
			e.status.Contents, e.status.Known = slices.Clone(m.Items), true
		}
	case SystemMessage:
		if e.step != whOpening && e.step != whSent {
			return
		}
		switch {
		case m.ID == msgNoItemDepositedInW && e.step == whOpening && !e.depositing():
			e.timeouts = 0
			e.status.Contents, e.status.Known = nil, true
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

// trackContents applies the rows the server just moved to the known
// contents: a withdrawal takes its counts off, a deposit adds to a stack
// it merges into or adds a row of its kind.
func (b *Brain) trackContents() {
	e := &b.wh
	st := &e.status
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
		st.Contents = append(st.Contents, WarehouseItem{TemplateID: r.item, Count: r.count})
	}
	st.Contents = slices.DeleteFunc(st.Contents, func(it WarehouseItem) bool { return it.Count <= 0 })
}
