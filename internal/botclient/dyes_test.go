package botclient

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The dye tests' Symbol Maker (Marsden, NPC 31046) once in sight, and the
// dyes of the symbols they draw (hennas.xml).
const (
	makerObj  = int32(0x20000060)
	makerNPC  = int32(31046)
	dyeStr    = int32(4445) // symbol 1, STR+1 CON-3
	dyeDex    = int32(4450) // symbol 6, DEX+1 CON-3
	dyeStr2   = int32(4481) // symbol 37, STR+1 CON-3
	dyeDex2   = int32(4486) // symbol 42, DEX+1 CON-3
	dyePrice  = int32(5610) // buyLists.xml <product id="4445" price="5610"/>
	classGlad = int32(2)    // Gladiator
)

var (
	firstProfs  = []int32{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54, 56}
	secondProfs = []int32{2, 3, 5, 6, 8, 9, 12, 13, 14, 16, 17, 20, 21, 23, 24, 27, 28, 30, 33, 34, 36, 37, 40, 41, 43, 46, 48, 51, 52, 55, 57}
)

// testHennas are hennas.xml's rows the tests draw, class lists trimmed to
// the first and second professions.
func testHennas() map[int32]datapack.Henna {
	return map[int32]datapack.Henna{
		1:  {SymbolID: 1, DyeID: dyeStr, Price: 37000, Classes: firstProfs},
		6:  {SymbolID: 6, DyeID: dyeDex, Price: 37000, Classes: firstProfs},
		8:  {SymbolID: 8, DyeID: 4452, Price: 37000, Classes: []int32{1, 4, 7, 11, 19, 22, 26, 32, 35, 39, 45, 47, 54, 56}},
		10: {SymbolID: 10, DyeID: 4454, Price: 37000, Classes: []int32{11, 15, 26, 29, 39, 42, 50}},
		37: {SymbolID: 37, DyeID: dyeStr2, Price: 87000, Classes: secondProfs},
		42: {SymbolID: 42, DyeID: dyeDex2, Price: 87000, Classes: secondProfs},
		46: {SymbolID: 46, DyeID: 4490, Price: 87000, Classes: []int32{12, 13, 14, 16, 17, 27, 28, 30, 40, 41, 43, 51, 52}},
	}
}

// dyeStack is n of dye template id as object obj.
func dyeStack(obj, id, n int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: n, SubCategory: 5}
}

// newDyeBrain is a bot of class and level at the origin with the default
// persona and no idle drift, carrying items, a Symbol Maker spawned at
// (5000, 0) on its travel map, and a HennaInfo of slots and worn symbols.
func newDyeBrain(t *testing.T, class, level, slots int32, worn []WornSymbol, items ...Item) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance, p.AutoShots = 0, false
	b, _ := newTestBrain(t, p)
	b.world.Self.ClassID, b.world.Self.Level = class, level
	b.train.doneLevel = level
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{makerNPC: {{X: 5000}}}, hennas: testHennas()}
	b.world.Apply(ItemList{Items: items}, t0)
	b.world.Apply(HennaInfo{MaxSlots: slots, Worn: worn}, t0)
	return walkOn(t, b)
}

// makerPage is the Symbol Maker's page (data/html/symbolmaker/
// SymbolMaker.htm) with obj for %objectId%.
func makerPage(obj int32) NpcHtmlMessage {
	id := strconv.Itoa(int(obj))
	return NpcHtmlMessage{ObjectID: obj, HTML: `<html><body>Symbol Maker:<br><center>` +
		`<a action="bypass -h npc_` + id + `_Draw">Draw a Symbol.</a><br>` +
		`<a action="bypass -h npc_` + id + `_RemoveList">Delete a Symbol.</a><br><br1>` +
		`<a action="bypass -h npc_` + id + `_Link symbolmaker/SymbolMaker-1.htm">"Tell me more about symbols and dyes."</a></center></body></html>`}
}

// makerInSight puts the maker in sight 200 units away, selected.
func makerInSight(b *Brain) {
	b.world.Apply(NPCInfo{ObjectID: makerObj, NPCID: makerNPC, Pos: Point{X: 200}}, t0)
	b.world.Target = makerObj
}

// drawWindow is the draw window offering symbols ids, each of hennas.xml.
func drawWindow(ids ...int32) HennaEquipList {
	l := HennaEquipList{Adena: 1, MaxSlots: 3}
	for _, id := range ids {
		h := testHennas()[id]
		l.Symbols = append(l.Symbols, HennaRow{SymbolID: id, DyeID: h.DyeID, Dyes: hennaDrawDyes, Price: h.Price})
	}
	return l
}

// spent is the InventoryUpdate after a draw or a removal: obj's stack of
// template id left with n, removed at 0.
func spent(obj, id, n int32) InventoryUpdate {
	st := ItemModified
	if n == 0 {
		st = ItemRemoved
	}
	return InventoryUpdate{Changes: []ItemChange{{State: st, Item: dyeStack(obj, id, n)}}}
}

func symbolIDs(hs []datapack.Henna) []int32 {
	var out []int32
	for _, h := range hs {
		out = append(out, h.SymbolID)
	}
	return out
}

// TestDyePlan checks the symbols each profession draws, healers included,
// and those it removes: the table's rows within the class list, into the
// free slots only, none for a base class or with every slot taken.
func TestDyePlan(t *testing.T) {
	tests := []struct {
		name               string
		class, slots       int32
		worn               []WornSymbol
		edit               func(b *Brain)
		wantRemove, wantDr []int32
	}{
		{name: "warrior", class: 1, slots: 2, wantDr: []int32{1, 6}},
		{name: "human knight", class: 4, slots: 2, wantDr: []int32{1, 6}},
		{name: "wizard", class: 11, slots: 2, wantDr: []int32{8, 8}},
		{name: "cleric", class: 15, slots: 2, wantDr: []int32{10, 10}},
		{name: "elven oracle", class: 29, slots: 2, wantDr: []int32{10, 10}},
		{name: "shillien oracle", class: 42, slots: 2, wantDr: []int32{10, 10}},
		{name: "orc shaman", class: 50, slots: 2, wantDr: []int32{10, 10}},
		{name: "gladiator", class: classGlad, slots: 3, wantDr: []int32{37, 42, 37}},
		{name: "bishop", class: 16, slots: 3, wantDr: []int32{46, 46, 46}},
		{name: "one worn: the other", class: 1, slots: 2, worn: []WornSymbol{{1, 1}}, wantDr: []int32{6}},
		{name: "a worn symbol off the table keeps its slot", class: 1, slots: 2, worn: []WornSymbol{{8, 8}}, wantDr: []int32{1}},
		{name: "full slots", class: 1, slots: 2, worn: []WornSymbol{{1, 1}, {6, 6}}},
		{name: "base class: no slot", class: classHumanFight, slots: 0},
		{
			name: "second profession: inactive first-profession symbols go", class: classGlad, slots: 3,
			worn: []WornSymbol{{1, 0}, {6, 0}}, wantRemove: []int32{1, 6}, wantDr: []int32{37, 42, 37},
		},
		{
			name: "an inactive symbol the datapack lacks keeps its slot", class: classGlad, slots: 3,
			worn: []WornSymbol{{99, 0}}, wantDr: []int32{37, 42},
		},
		{
			name: "a row outside the class list is skipped", class: 15, slots: 2,
			edit:   func(b *Brain) { b.persona.Dyes.Symbols = map[int32][]int32{15: {8, 10}} },
			wantDr: []int32{10},
		},
		{
			name: "a symbol the datapack lacks is skipped", class: 1, slots: 2,
			edit:   func(b *Brain) { b.persona.Dyes.Symbols = map[int32][]int32{1: {99, 1}} },
			wantDr: []int32{1},
		},
		{name: "before HennaInfo", class: 1, slots: 2, edit: func(b *Brain) { b.world.Symbols = Symbols{} }},
		{name: "no travel map", class: 1, slots: 2, edit: func(b *Brain) { b.cfg.Travel = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDyeBrain(t, tt.class, 30, tt.slots, tt.worn)
			if tt.edit != nil {
				tt.edit(b)
			}
			remove, draw := b.dyePlan()
			if got := symbolIDs(remove); !slices.Equal(got, tt.wantRemove) {
				t.Errorf("remove = %v, want %v", got, tt.wantRemove)
			}
			if got := symbolIDs(draw); !slices.Equal(got, tt.wantDr) {
				t.Errorf("draw = %v, want %v", got, tt.wantDr)
			}
		})
	}
}

// TestDyesDue checks when a visit starts: with the dyes and the fee of a
// symbol in hand, or an inactive symbol to remove, at a maker in sight or
// within Reach; never for a base class, with full slots, short of the dyes
// or of the fee past the reserve, while backing off, or out of reach.
func TestDyesDue(t *testing.T) {
	const rich = 200000
	both := []Item{adenaStack(rich), dyeStack(200, dyeStr, 10), dyeStack(201, dyeDex, 10)}
	tests := []struct {
		name         string
		class, slots int32
		worn         []WornSymbol
		items        []Item
		edit         func(b *Brain)
		want         bool
	}{
		{name: "dyes and fee in hand", class: 1, slots: 2, items: both, want: true},
		{name: "one symbol's dyes", class: 1, slots: 2, items: []Item{adenaStack(rich), dyeStack(201, dyeDex, 10)}, want: true},
		{name: "nine dyes", class: 1, slots: 2, items: []Item{adenaStack(rich), dyeStack(200, dyeStr, 9)}},
		{name: "fee past the reserve short", class: 1, slots: 2, items: []Item{adenaStack(37000), dyeStack(200, dyeStr, 10)}},
		{name: "base class", class: classHumanFight, slots: 0, items: both},
		{name: "full slots", class: 1, slots: 2, worn: []WornSymbol{{1, 1}, {6, 6}}, items: both},
		{name: "removal only", class: classGlad, slots: 3, worn: []WornSymbol{{1, 0}}, items: []Item{adenaStack(rich)}, want: true},
		{name: "removal fee short", class: classGlad, slots: 3, worn: []WornSymbol{{1, 0}}, items: []Item{adenaStack(1000)}},
		{name: "backing off", class: 1, slots: 2, items: both, edit: func(b *Brain) { b.dyes.retryAt = at(time.Minute) }},
		{name: "maker out of reach", class: 1, slots: 2, items: both, edit: func(b *Brain) { b.persona.Dyes.Reach = 4000 }},
		{
			name: "party member: maker out of sight", class: 1, slots: 2, items: both,
			edit: func(b *Brain) { b.world.Party.LeaderID = pcID },
		},
		{
			name: "party member: maker in sight", class: 1, slots: 2, items: both, want: true,
			edit: func(b *Brain) { b.world.Party.LeaderID = pcID; makerInSight(b) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDyeBrain(t, tt.class, 30, tt.slots, tt.worn, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.startDyes(at(time.Second)); got != tt.want {
				t.Fatalf("startDyes = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBrainDyes draws a warrior's two symbols: it talks to the maker,
// follows the page's Draw link, then sends one RequestHennaEquip per
// answer, the next only once HennaInfo and the InventoryUpdate behind it
// came; with both drawn the visit ends with no back-off.
func TestBrainDyes(t *testing.T) {
	b := newDyeBrain(t, 1, 30, 2, nil, adenaStack(200000), dyeStack(200, dyeStr, 10), dyeStack(201, dyeDex, 10))
	runTrainerSteps(t, b, []trainerStep{
		{name: "talk", at: time.Second, setup: makerInSight, wantRule: "dyes", wantKind: actTalk},
		{name: "wait for the page", at: 2 * time.Second, wantRule: "dyes"},
		{name: "the Draw link", at: 3 * time.Second, events: []Event{makerPage(makerObj)}, wantRule: "dyes", wantKind: actBypass, wantPayload: masterBypass(makerObj, "Draw")},
		{name: "wait for the window", at: 4 * time.Second, wantRule: "dyes"},
		{name: "draw symbol 1", at: 5 * time.Second, events: []Event{drawWindow(1, 6)}, wantRule: "dyes", wantKind: actHennaEquip, wantPayload: encodeRequestHennaEquip(1)},
		{name: "wait for HennaInfo", at: 6 * time.Second, wantRule: "dyes"},
		{name: "wait for the inventory", at: 7 * time.Second, events: []Event{HennaInfo{MaxSlots: 2, Worn: []WornSymbol{{1, 1}}}, SystemMessage{ID: 877}}, wantRule: "dyes"},
		{name: "draw symbol 6", at: 8 * time.Second, events: []Event{spent(200, dyeStr, 0)}, wantRule: "dyes", wantKind: actHennaEquip, wantPayload: encodeRequestHennaEquip(6)},
		{name: "wait again", at: 9 * time.Second, wantRule: "dyes"},
	})
	if err := b.handle([]Event{HennaInfo{MaxSlots: 2, Worn: []WornSymbol{{1, 1}, {6, 6}}}, spent(201, dyeDex, 0)}, at(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.ruleDyes(at(10 * time.Second)); ok {
		t.Fatal("ruleDyes matched with both symbols drawn")
	}
	if b.dyes.v.active() || !b.dyes.retryAt.IsZero() {
		t.Fatalf("visit %+v, want it ended with no back-off", b.dyes)
	}
}

// TestBrainDyesSecondProfession takes a gladiator wearing its first
// profession's inactive symbols to the maker: it removes both, one per
// answer, then draws the second profession's rows into all 3 slots.
func TestBrainDyesSecondProfession(t *testing.T) {
	b := newDyeBrain(t, classGlad, 40, 3, []WornSymbol{{1, 0}, {6, 0}},
		adenaStack(1000000), dyeStack(200, dyeStr2, 20), dyeStack(201, dyeDex2, 10))
	runTrainerSteps(t, b, []trainerStep{
		{name: "talk", at: time.Second, setup: makerInSight, wantRule: "dyes", wantKind: actTalk},
		{name: "remove symbol 1", at: 2 * time.Second, events: []Event{makerPage(makerObj)}, wantRule: "dyes", wantKind: actHennaUnequip, wantPayload: encodeRequestHennaUnequip(1)},
		{name: "wait for HennaInfo", at: 3 * time.Second, wantRule: "dyes"},
		{
			name: "remove symbol 6", at: 4 * time.Second,
			events:   []Event{HennaInfo{MaxSlots: 3, Worn: []WornSymbol{{6, 0}}}, spent(202, dyeStr, 5)},
			wantRule: "dyes", wantKind: actHennaUnequip, wantPayload: encodeRequestHennaUnequip(6),
		},
		{
			name: "the Draw link", at: 5 * time.Second,
			events:   []Event{HennaInfo{MaxSlots: 3}, spent(203, dyeDex, 5)},
			wantRule: "dyes", wantKind: actBypass, wantPayload: masterBypass(makerObj, "Draw"),
		},
		{name: "draw symbol 37", at: 6 * time.Second, events: []Event{drawWindow(37, 42)}, wantRule: "dyes", wantKind: actHennaEquip, wantPayload: encodeRequestHennaEquip(37)},
		{
			name: "draw symbol 42", at: 7 * time.Second,
			events:   []Event{HennaInfo{MaxSlots: 3, Worn: []WornSymbol{{37, 37}}}, spent(200, dyeStr2, 10)},
			wantRule: "dyes", wantKind: actHennaEquip, wantPayload: encodeRequestHennaEquip(42),
		},
		{
			name: "draw symbol 37 again", at: 8 * time.Second,
			events:   []Event{HennaInfo{MaxSlots: 3, Worn: []WornSymbol{{37, 37}, {42, 42}}}, spent(201, dyeDex2, 0)},
			wantRule: "dyes", wantKind: actHennaEquip, wantPayload: encodeRequestHennaEquip(37),
		},
	})
	if err := b.handle([]Event{HennaInfo{MaxSlots: 3, Worn: []WornSymbol{{37, 37}, {42, 42}, {37, 37}}}, spent(200, dyeStr2, 0)}, at(9*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.ruleDyes(at(9 * time.Second)); ok || b.dyes.v.active() || !b.dyes.retryAt.IsZero() {
		t.Fatalf("visit %+v, want it ended with no back-off", b.dyes)
	}
}

// TestBrainDyesBacksOff checks that a refused or unanswered request ends
// the visit for RetryAfter: a refusal at once, a silence (the server drops
// an unknown symbol) or a HennaInfo with no InventoryUpdate after
// dyeReplyTimeout, a draw window that never comes, and a window that does
// not offer the symbol.
func TestBrainDyesBacksOff(t *testing.T) {
	const sentAt = time.Second
	tests := []struct {
		name string
		// listed starts the visit with the window open, so the first
		// request is a draw; else it is the Draw link.
		listed bool
		window HennaEquipList
		events []Event
		// after is when the visit is checked, from the request.
		after time.Duration
		// wantKind is the first request, actHold for none.
		wantKind actionKind
	}{
		{name: "CantDrawSymbol", listed: true, events: []Event{SystemMessage{ID: msgCantDrawSymbol}}, wantKind: actHennaEquip},
		{name: "SymbolsFull", listed: true, events: []Event{SystemMessage{ID: msgSymbolsFull}}, wantKind: actHennaEquip},
		{name: "YouNotEnoughAdena", listed: true, events: []Event{SystemMessage{ID: msgNotEnoughAdena}}, wantKind: actHennaEquip},
		{name: "NotEnoughItems", listed: true, events: []Event{SystemMessage{ID: msgNotEnoughItems}}, wantKind: actHennaEquip},
		{name: "silence", listed: true, after: dyeReplyTimeout, wantKind: actHennaEquip},
		{name: "HennaInfo, no inventory", listed: true, events: []Event{HennaInfo{MaxSlots: 2, Worn: []WornSymbol{{1, 1}}}}, after: dyeReplyTimeout, wantKind: actHennaEquip},
		{name: "no window", after: dyeReplyTimeout, wantKind: actBypass},
		{name: "symbol not offered", listed: true, window: drawWindow(8)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDyeBrain(t, 1, 30, 2, nil, adenaStack(200000), dyeStack(200, dyeStr, 10))
			makerInSight(b)
			b.world.Apply(makerPage(makerObj), t0)
			b.dyes.v = npcVisit{npcID: makerNPC, at: Point{X: 200}, obj: makerObj, opened: true}
			if tt.listed {
				b.dyes.listed, b.dyes.list = true, drawWindow(1).Symbols
				if tt.window.Symbols != nil {
					b.dyes.list = tt.window.Symbols
				}
			}
			now := at(sentAt)
			rule, a := b.decide(now)
			if tt.wantKind == actHold {
				if rule == "dyes" || b.dyes.v.active() || !b.dyes.retryAt.Equal(now.Add(visitShunFor)) {
					t.Fatalf("decide = %q, visit %+v: want it ended for visitShunFor", rule, b.dyes)
				}
				return
			}
			if rule != "dyes" || a.kind != tt.wantKind {
				t.Fatalf("decide = %q %d, want dyes %d", rule, a.kind, tt.wantKind)
			}
			if sent, err := b.send(a, now); err != nil || !sent {
				t.Fatalf("send = %v, %v", sent, err)
			}
			if tt.after > 0 {
				if err := b.handle(tt.events, now); err != nil {
					t.Fatal(err)
				}
				if rule, _ := b.decide(now.Add(tt.after - time.Millisecond)); rule != "dyes" {
					t.Fatalf("decide before the timeout = %q, want dyes holding", rule)
				}
			}
			end := now.Add(tt.after)
			if tt.after == 0 {
				end = now.Add(time.Second)
				if err := b.handle(tt.events, end); err != nil {
					t.Fatal(err)
				}
			}
			if rule, _ := b.decide(end); rule == "dyes" || b.dyes.v.active() || !b.dyes.retryAt.Equal(end.Add(visitShunFor)) {
				t.Fatalf("decide = %q, visit %+v: want it ended for visitShunFor at %v", rule, b.dyes, end)
			}
		})
	}
}

// TestDyeWants checks the shop errand's dye purchase: 10 dyes per symbol,
// less what the bot carries, under the gear tier's floor with every
// planned symbol's fee left, and only for symbols paid in full.
func TestDyeWants(t *testing.T) {
	const floor, sym = 10000, 10*int64(dyePrice) + 37000
	tests := []struct {
		name    string
		class   int32
		adena   int64
		items   []Item
		want    []buyRow
		wantFee int64
	}{
		{name: "both symbols", class: 1, adena: floor + 2*sym, want: []buyRow{{dyeStr, 10}, {dyeDex, 10}}, wantFee: 74000},
		{name: "the second short", class: 1, adena: floor + 2*sym - 1, want: []buyRow{{dyeStr, 10}}, wantFee: 37000},
		{name: "none paid in full", class: 1, adena: floor + sym - 1},
		{name: "carried dyes", class: 1, adena: floor + 2*sym, items: []Item{dyeStack(200, dyeStr, 4)}, want: []buyRow{{dyeStr, 6}, {dyeDex, 10}}, wantFee: 74000},
		{name: "dyes in hand: the fee only", class: 1, adena: floor + 74000, items: []Item{dyeStack(200, dyeStr, 10), dyeStack(201, dyeDex, 10)}, wantFee: 74000},
		{name: "base class", class: classHumanFight, adena: floor + 2*sym},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDyeBudgetBrain(t, tt.class, int32(tt.adena), tt.items...)
			rows, fee := b.dyeWants()
			if !slices.Equal(rows, tt.want) || fee != tt.wantFee {
				t.Fatalf("dyeWants = %v, fee %d; want %v, fee %d", rows, fee, tt.want, tt.wantFee)
			}
		})
	}
}

// newDyeBudgetBrain is newBudgetBrain for a bot of class with adena and
// items, the gear tier's floor at 10000, a merchant selling both
// first-profession fighter dyes at dyePrice, and slots for its class.
func newDyeBudgetBrain(t *testing.T, class, adena int32, items ...Item) *Brain {
	t.Helper()
	b := newBudgetBrain(t, adena, items...)
	b.persona.Gear.KeepAdena, b.persona.Gear.Buy = 10000, false
	b.persona.Supply = SupplyPolicy{}
	b.world.Self.ClassID, b.world.Self.Level = class, 30
	m := b.cfg.Travel
	m.hennas = testHennas()
	m.cheapest[dyeStr], m.cheapest[dyeDex] = dyePrice, dyePrice
	slots := int32(2)
	if class == classHumanFight {
		slots = 0
	}
	b.world.Apply(HennaInfo{MaxSlots: slots}, t0)
	return b
}

// TestBuyDyes checks the gear tier's dye rows at a window: 10 per symbol
// bought before the draw, the fee kept past the floor, and a row the
// window cannot fill in full not bought.
func TestBuyDyes(t *testing.T) {
	const floor, sym = 10000, 10*int32(dyePrice) + 37000
	window := []ShopItem{{TemplateID: dyeStr, Price: dyePrice}, {TemplateID: dyeDex, Price: dyePrice}}
	tests := []struct {
		name   string
		adena  int32
		money  int32 // the window's adena when it differs
		window []ShopItem
		want   []buyRow
	}{
		{name: "both symbols' dyes", adena: floor + 2*sym, window: window, want: []buyRow{{dyeStr, 10}, {dyeDex, 10}}},
		{name: "the fee kept", adena: floor + 2*sym, money: floor + 2*sym - 1, window: window, want: []buyRow{{dyeStr, 10}}},
		{
			name: "a short stock is not bought", adena: floor + 2*sym,
			window: []ShopItem{{TemplateID: dyeStr, Price: dyePrice, Count: 9}, {TemplateID: dyeDex, Price: dyePrice}},
			want:   []buyRow{{dyeDex, 10}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDyeBudgetBrain(t, 1, tt.adena)
			money := tt.adena
			if tt.money != 0 {
				money = tt.money
			}
			if got := b.buyRows(BuyList{Money: money, ListID: 1, Items: tt.window}); !slices.Equal(got, tt.want) {
				t.Fatalf("buyRows = %v, want %v", got, tt.want)
			}
		})
	}
	b := newDyeBudgetBrain(t, 1, floor+2*sym)
	if got := b.shopWants(false); !slices.Contains(got, buyRow{dyeStr, 10}) || !slices.Contains(got, buyRow{dyeDex, 10}) {
		t.Fatalf("shopWants = %v, want both symbols' dyes: they make a visit due", got)
	}
}

// TestDyesKept checks that a planned symbol's dyes are never junk until
// drawn: with the slots full, leftovers are.
func TestDyesKept(t *testing.T) {
	dyes := dyeStack(200, dyeStr, 10)
	b := newDyeBrain(t, 1, 30, 2, nil, dyes)
	if _, ok := b.junk()[dyes.ObjectID]; ok {
		t.Fatal("planned dyes are junk")
	}
	if !b.keepsItem(dyes) {
		t.Fatal("planned dyes not kept: a store or a trade would let them go")
	}
	b.world.Apply(HennaInfo{MaxSlots: 2, Worn: []WornSymbol{{1, 1}, {6, 6}}}, t0)
	if _, ok := b.junk()[dyes.ObjectID]; !ok {
		t.Fatal("dyes with every slot drawn not junk")
	}
}

// TestDefaultDyeTableShipped checks every row of the default dye table
// against the shipped hennas.xml when BOT_DATAPACK_XML names a real
// data/xml directory, and skips otherwise: the symbol exists, lists the
// class, and costs 37000 for a first profession and 87000 for a second.
func TestDefaultDyeTableShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	hennas, err := datapack.LoadHennas(filepath.Join(dir, "hennas.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for class, symbols := range defaultDyeTable() {
		price := int32(87000)
		if slices.Contains(firstProfessions, class) {
			price = 37000
		}
		for _, id := range symbols {
			h, ok := hennas[id]
			if !ok || !slices.Contains(h.Classes, class) || h.Price != price {
				t.Errorf("class %d symbol %d = %+v, want it listing the class at %d", class, id, h, price)
			}
		}
	}
}
