package botclient

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Items of the store tests (aCis_datapack/data/xml/items): wooden arrows
// are sold by NPC shops, the others are loot. The Tutorial Guide a new
// character carries is loot its store cannot price (reference price 1).
const (
	woodenArrow   int32 = 17
	stoneSword    int32 = 2370
	thread        int32 = 1868
	tutorialGuide int32 = 5588
)

var (
	skinLoot   = Item{ObjectID: 102, TemplateID: animalSkin, Count: 10, SubCategory: 5}
	arrowsLoot = Item{ObjectID: 105, TemplateID: woodenArrow, Count: 50, SubCategory: 5}
	swordLoot  = Item{ObjectID: 107, TemplateID: stoneSword, Count: 1, BodySlot: 128}
	threadLoot = Item{ObjectID: 108, TemplateID: thread, Count: 3, SubCategory: 5}
	guideLoot  = Item{ObjectID: 109, TemplateID: tutorialGuide, Count: 1, SubCategory: 5}

	// Candidates of the manage list, their price the reference price.
	skinCand   = StoreItem{Type2: 5, ObjectID: 102, TemplateID: animalSkin, Count: 10, Price: 10, ReferencePrice: 10}
	arrowsCand = StoreItem{Type2: 5, ObjectID: 105, TemplateID: woodenArrow, Count: 50, Price: 2, ReferencePrice: 2}
	swordCand  = StoreItem{Type2: 0, ObjectID: 107, TemplateID: stoneSword, Count: 1, BodySlot: 128, Price: 1000, ReferencePrice: 1000}
	threadCand = StoreItem{Type2: 5, ObjectID: 108, TemplateID: thread, Count: 3, Price: 1, ReferencePrice: 1}
	// swordListed is the sword listed at 700, enchanted +3.
	swordListed = StoreItem{Type2: 0, ObjectID: 107, TemplateID: stoneSword, Count: 1, Enchant: 3, BodySlot: 128, Price: 700, ReferencePrice: 1000}
)

// storeRowFields is a store window's int32 row count and rows as packet
// fields; listed rows carry their reference price.
func storeRowFields(listed bool, items ...StoreItem) []any {
	f := []any{int32(len(items))}
	for _, it := range items {
		f = append(f, it.Type2, it.ObjectID, it.TemplateID, it.Count, uint16(0), it.Enchant, uint16(0), it.BodySlot, it.Price)
		if listed {
			f = append(f, it.ReferencePrice)
		}
	}
	return f
}

func manageList(owner, adena int32, candidates, listed []StoreItem) []byte {
	f := append([]any{owner, int32(0), adena}, storeRowFields(false, candidates...)...)
	return packet(opGamePrivateStoreManageListSell, append(f, storeRowFields(true, listed...)...)...)
}

func TestDecodeStoreWindows(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Owner 0x10000001, not packaged, 1000 adena; the skin a candidate
		// at its reference price 10, the sword +3 listed at 700 of 1000.
		{
			"PrivateStoreManageListSell bytes",
			[]byte{
				0x9a, 1, 0, 0, 0x10, 0, 0, 0, 0, 0xe8, 3, 0, 0,
				1, 0, 0, 0,
				5, 0, 0, 0, 102, 0, 0, 0, 0x4b, 7, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0,
				1, 0, 0, 0,
				0, 0, 0, 0, 107, 0, 0, 0, 0x42, 9, 0, 0, 1, 0, 0, 0, 0, 0, 3, 0, 0, 0, 128, 0, 0, 0, 0xbc, 2, 0, 0, 0xe8, 3, 0, 0,
			},
			PrivateStoreManageListSell{OwnerID: selfID, Adena: 1000, Candidates: []StoreItem{skinCand}, Listed: []StoreItem{swordListed}},
		},
		{
			"PrivateStoreManageListSell empty",
			manageList(selfID, 0, nil, nil),
			PrivateStoreManageListSell{OwnerID: selfID, Candidates: []StoreItem{}, Listed: []StoreItem{}},
		},
		{
			"PrivateStoreListSell",
			packet(opGamePrivateStoreListSell, append([]any{selfID, int32(1), int32(5000)}, storeRowFields(true, swordListed)...)...),
			PrivateStoreListSell{OwnerID: selfID, Packaged: true, BuyerAdena: 5000, Items: []StoreItem{swordListed}},
		},
		{
			"PrivateStoreMsgSell",
			packet(opGamePrivateStoreMsgSell, selfID, "Loot for sale"),
			PrivateStoreMsgSell{OwnerID: selfID, Title: "Loot for sale"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeStoreWindowsRejects(t *testing.T) {
	manage := manageList(selfID, 1, []StoreItem{skinCand}, nil)
	list := packet(opGamePrivateStoreListSell, append([]any{selfID, int32(0), int32(1)}, storeRowFields(true, swordListed)...)...)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"manage list short header", packet(opGamePrivateStoreManageListSell, selfID, int32(0)), wire.ErrShortPacket},
		{"manage list no listed count", manage[:len(manage)-4], wire.ErrShortPacket},
		{"manage list trailing", append(manage, 0), errTrailingBytes},
		{"manage list count past the frame", packet(opGamePrivateStoreManageListSell, selfID, int32(0), int32(1), int32(2)), wire.ErrShortPacket},
		{"store list short row", list[:len(list)-4], wire.ErrShortPacket},
		{"store list trailing", append(list, 0), errTrailingBytes},
		{"store title unterminated", packet(opGamePrivateStoreMsgSell, selfID, []byte{'a', 0}), wire.ErrShortPacket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestStoreEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestPrivateStoreManageSell", encodeRequestPrivateStoreManageSell(), []byte{0x73}},
		{"RequestPrivateStoreQuitSell", encodeRequestPrivateStoreQuitSell(), []byte{0x76}},
		{"SetPrivateStoreMsgSell", encodeSetPrivateStoreMsgSell("ab"), []byte{0x77, 'a', 0, 'b', 0, 0, 0}},
		{
			"SetPrivateStoreListSell", encodeSetPrivateStoreListSell(false, []storeRow{{102, 10, 7}}),
			[]byte{0x74, 0, 0, 0, 0, 1, 0, 0, 0, 102, 0, 0, 0, 10, 0, 0, 0, 7, 0, 0, 0},
		},
		{
			"SetPrivateStoreListSell packaged", encodeSetPrivateStoreListSell(true, nil),
			[]byte{0x74, 1, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			"RequestPrivateStoreBuy", encodeRequestPrivateStoreBuy(selfID, []storeRow{{107, 1, 700}}),
			[]byte{0x79, 1, 0, 0, 0x10, 1, 0, 0, 0, 107, 0, 0, 0, 1, 0, 0, 0, 0xbc, 2, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

// newStoreBrain is a merchant at the origin of a town with budget, a 140 %
// markup and the default persona (no idle drift) carrying items, wooden
// arrows on an NPC shop's list and the store test templates' reference
// prices (lootPrices) on its travel map.
func newStoreBrain(t *testing.T, budget int64, items ...Item) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	p.Store.Budget, p.Store.MinMarkup, p.Store.MaxMarkup = budget, 140, 140
	b, _ := newTestBrain(t, p)
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 1, NPCID: 30001, Products: []datapack.Product{{ItemID: woodenArrow, Price: 2}}}})
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{}, shops: shops, cheapest: cheapest, prices: lootPrices}
	b.cfg.Stores = NewStoreRegistry([]Point{{}}, 5, 150)
	b.world.Apply(ItemList{Items: items}, t0)
	return b
}

// TestBrainStoreOpen opens a seated merchant's store of its loot, then
// counts its sales once a buyer emptied it.
func TestBrainStoreOpen(t *testing.T) {
	b := newStoreBrain(t, 1000, adenaStack(1000), potionsOf(5), skinLoot, arrowsLoot, swordLoot, threadLoot, questItem, sword)
	b.world.Self.Sitting = true
	manage := PrivateStoreManageListSell{OwnerID: selfID, Adena: 1000, Candidates: []StoreItem{skinCand, arrowsCand, swordCand, threadCand}}
	runTrainerSteps(t, b, []trainerStep{
		{name: "stand up", at: time.Second, wantRule: "store", wantKind: actSitStand},
		{name: "wait for the stand to end", at: 2 * time.Second, events: []Event{ChangeWaitType{ObjectID: selfID, Type: WaitStanding}}, wantRule: "store"},
		{name: "open", at: 5 * time.Second, wantRule: "store", wantKind: actStoreManage, wantPayload: encodeRequestPrivateStoreManageSell()},
		{name: "wait for the manage list", at: 6 * time.Second, wantRule: "store"},
		{name: "title", at: 7 * time.Second, events: []Event{manage}, wantRule: "store", wantKind: actStoreTitle, wantPayload: encodeSetPrivateStoreMsgSell("Loot for sale")},
		{
			// The skin at 7 (1.4 times 5) and the sword at 700; never the
			// arrows an NPC sells, nor the thread, worth nothing to an NPC.
			name: "list", at: 8 * time.Second, wantRule: "store", wantKind: actStoreList,
			wantPayload: encodeSetPrivateStoreListSell(false, []storeRow{{102, 10, 7}, {107, 1, 700}}),
		},
		{name: "wait for the store", at: 9 * time.Second, wantRule: "store"},
		{name: "open for business", at: 10 * time.Second, setup: func(b *Brain) { b.world.Self.StoreType = storeTypeSell }, wantRule: "store"},
		{
			name: "sold out", at: time.Minute,
			setup: func(b *Brain) { b.world.Self.StoreType = 0 },
			events: []Event{InventoryUpdate{Changes: []ItemChange{
				{ItemModified, adenaStack(1770)}, {ItemRemoved, skinLoot}, {ItemRemoved, swordLoot},
			}}},
			wantRule: "store",
		},
		// The arrows and the thread are no loot to sell: back to play.
		{name: "closed", at: time.Minute + time.Second},
	})
	st := &b.store
	if st.active() || st.claimed || b.cfg.Stores.Open() != 0 || st.sold != 770 || !st.nextAt.Equal(at(time.Minute+5*time.Minute)) {
		t.Errorf("store %+v, registry %d open, want it ended, released, 770 sold and the next opening Retry away", *st, b.cfg.Stores.Open())
	}
	if p, ok := b.tradePrice(Item{TemplateID: animalSkin}); !ok || p != 7 {
		t.Errorf("tradePrice(skin) = %d, %v, want the store's 7", p, ok)
	}
	if _, ok := b.tradePrice(Item{TemplateID: woodenArrow}); ok {
		t.Error("tradePrice(arrows) priced, want an NPC shop's item unpriced")
	}
}

// TestStoreRowsBudget checks that a store lists no more than the budget
// left.
func TestStoreRowsBudget(t *testing.T) {
	tests := []struct {
		name         string
		budget, sold int64
		want         []storeRow
	}{
		{name: "all", budget: 1000, want: []storeRow{{102, 10, 7}, {107, 1, 700}}},
		{name: "the sword past the budget left", budget: 1000, sold: 900, want: []storeRow{{102, 10, 7}}},
		{name: "part of the stack", budget: 50, want: []storeRow{{102, 7, 7}}},
		{name: "nothing left", budget: 50, sold: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStoreBrain(t, tt.budget, adenaStack(1000), skinLoot, swordLoot)
			b.store.sold = tt.sold
			b.store.step = storeAskManage
			b.storeManageList(PrivateStoreManageListSell{OwnerID: selfID, Candidates: []StoreItem{skinCand, swordCand}})
			if got := b.storeRows(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("storeRows = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStoreRowsCap checks the row cap and the adena cap of the server's
// FillSell.
func TestStoreRowsCap(t *testing.T) {
	b := newStoreBrain(t, 1<<40, adenaStack(1<<31-1-700), skinLoot, swordLoot)
	b.store.step = storeAskManage
	b.storeManageList(PrivateStoreManageListSell{OwnerID: selfID, Candidates: []StoreItem{swordCand, skinCand}})
	if got, want := b.storeRows(), []storeRow{{107, 1, 700}}; !reflect.DeepEqual(got, want) {
		t.Errorf("storeRows near the adena cap = %v, want %v", got, want)
	}
	b.persona.Store.MaxRows = 1
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, adenaStack(0)}}}, t0)
	if got, want := b.storeRows(), []storeRow{{107, 1, 700}}; !reflect.DeepEqual(got, want) {
		t.Errorf("storeRows of one row = %v, want %v", got, want)
	}
}

// TestStorePrice checks the markup bounds: 1.2 to 1.5 times the buy-back
// price, under the reference price.
func TestStorePrice(t *testing.T) {
	for _, ref := range []int32{1, 2, 3, 10, 999, 1000, 1 << 30} {
		for seed := range uint64(20) {
			b := newStoreBrain(t, 1)
			b.persona.Store.MinMarkup, b.persona.Store.MaxMarkup = 120, 150
			b.rng = rand.New(rand.NewPCG(seed, 7))
			got := int64(b.storePrice(ref))
			buyBack, m := int64(ref/2), int64(b.store.markup)
			if m < 120 || m > 150 {
				t.Fatalf("markup %d, want 120-150", m)
			}
			if got == 0 && buyBack*m/100 == 0 {
				continue
			}
			if got != buyBack*m/100 || got*100 <= buyBack*120-100 || got*100 > buyBack*150 || got >= int64(ref) {
				t.Fatalf("storePrice(%d) = %d at markup %d, want 1.2-1.5 times %d and under %d", ref, got, b.store.markup, buyBack, ref)
			}
		}
	}
}

// TestStoreStart checks when an opening starts.
func TestStoreStart(t *testing.T) {
	now := at(time.Minute)
	tests := []struct {
		name  string
		items []Item
		setup func(b *Brain)
		// open is set when the decision opens the store.
		open bool
	}{
		{name: "loot in town", open: true},
		{name: "no loot", items: []Item{adenaStack(10), potionsOf(5), arrowsLoot, questItem, sword}},
		// Half of reference price 1 is 0: the store cannot price it (#120).
		{name: "only the Tutorial Guide", items: []Item{adenaStack(10), guideLoot}},
		{name: "unpriced loot", items: []Item{adenaStack(10), guideLoot, threadLoot}},
		{name: "no reference price", setup: func(b *Brain) { b.cfg.Travel.prices = nil }},
		{name: "Tutorial Guide and priced loot", items: []Item{adenaStack(10), guideLoot, skinLoot}, open: true},
		{
			name: "priced by a manage list", open: true, setup: func(b *Brain) {
				b.cfg.Travel.prices, b.store.prices = nil, map[int32]int32{animalSkin: 14}
			},
		},
		{name: "no budget", setup: func(b *Brain) { b.persona.Store.Budget = 0 }},
		{name: "budget spent", setup: func(b *Brain) { b.store.sold = b.persona.Store.Budget }},
		{name: "no registry", setup: func(b *Brain) { b.cfg.Stores = nil }},
		{name: "no travel map", setup: func(b *Brain) { b.cfg.Travel = nil }},
		{name: "retry pending", setup: func(b *Brain) { b.store.nextAt = now.Add(time.Second) }},
		{name: "in combat", setup: func(b *Brain) { b.world.Self.InCombat = true }},
		{name: "PvP flag", setup: func(b *Brain) { b.world.Self.PvPFlag = 1 }},
		{name: "casting", setup: func(b *Brain) { b.castAt = now }},
		{name: "moving", setup: func(b *Brain) { b.world.Self.move = motion{to: Point{X: 500}, start: now, speed: 100} }},
		{name: "trade request", setup: func(b *Brain) { b.world.Prompts.Trade = TradeInvite{Open: true, PartnerID: pcID, At: now} }},
		{name: "shop visit", setup: func(b *Brain) { b.shop.v.npcID = 30004 }},
		{name: "out of town", setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 5000}) }},
		{name: "next to another store", setup: func(b *Brain) { b.cfg.Stores.claim(Point{X: 100}) }},
		{
			name: "town full", setup: func(b *Brain) {
				for i := range 5 {
					b.cfg.Stores.claim(Point{X: 200 + 200*int32(i)})
				}
			},
		},
		{name: "standing up", setup: func(b *Brain) { b.store.stoodAt = now.Add(-time.Second) }},
		{name: "following", setup: func(b *Brain) { b.orders.follow = pcID }},
		{name: "assisting", setup: func(b *Brain) { b.orders.assist = pcID }},
		{
			name: "party member", setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: pcID, Members: map[int32]*PartyMate{pcID: {}}}
			},
		},
		{
			name: "party leader", open: true, setup: func(b *Brain) {
				b.world.Party = Party{LeaderID: b.world.Self.ObjectID, Members: map[int32]*PartyMate{pcID: {}}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := tt.items
			if items == nil {
				items = []Item{adenaStack(10), skinLoot}
			}
			b := newStoreBrain(t, 1000, items...)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(now)
			if got := a.kind == actStoreManage; got != tt.open {
				t.Fatalf("decide = rule %q action %d, want open %v", rule, a.kind, tt.open)
			}
			if !tt.open && rule == "store" && tt.name != "standing up" {
				t.Fatalf("decide = rule %q, want the store rule passed over", rule)
			}
		})
	}
}

// TestStoreEnds checks how an opening fails and a store closes: the claim
// goes back and the next opening waits for Retry.
func TestStoreEnds(t *testing.T) {
	now := at(time.Minute)
	tests := []struct {
		name   string
		step   storeStep
		setup  func(b *Brain)
		events []Event
		// quit is set when the decision takes the manage state down.
		quit bool
	}{
		{name: "no manage list", step: storeAskManage, setup: func(b *Brain) { b.store.waitUntil = now.Add(-time.Millisecond) }, quit: true},
		{name: "list refused", step: storeListed, events: []Event{PrivateStoreManageListSell{OwnerID: selfID}}, quit: true},
		{name: "store never shown", step: storeListed, setup: func(b *Brain) { b.store.waitUntil = now.Add(-time.Millisecond) }, quit: true},
		{name: "nothing to list", step: storeTitled, quit: true},
		{
			name: "teleported", step: storeOpen,
			setup:  func(b *Brain) { b.world.Self.StoreType = storeTypeSell },
			events: []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 100}}},
		},
		{name: "killed", step: storeOpen, setup: func(b *Brain) { b.world.Self.StoreType = storeTypeSell }, events: []Event{Die{ObjectID: selfID}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStoreBrain(t, 1000, adenaStack(10), skinLoot)
			id, _ := b.cfg.Stores.claim(Point{})
			b.store.claim, b.store.claimed, b.store.step = id, true, tt.step
			if tt.setup != nil {
				tt.setup(b)
			}
			if err := b.handle(tt.events, now); err != nil {
				t.Fatal(err)
			}
			_, a := b.decide(now)
			if got := a.kind == actStoreQuit; got != tt.quit {
				t.Fatalf("decide = action %d, want quit %v", a.kind, tt.quit)
			}
			if tt.quit {
				if !bytes.Equal(a.payload, encodeRequestPrivateStoreQuitSell()) {
					t.Fatalf("payload % x, want RequestPrivateStoreQuitSell", a.payload)
				}
				if sent, err := b.send(a, now); !sent || err != nil {
					t.Fatalf("send = %v, %v", sent, err)
				}
			}
			if b.store.active() || b.store.claimed || b.cfg.Stores.Open() != 0 || !b.store.nextAt.Equal(now.Add(5*time.Minute)) {
				t.Errorf("store %+v, registry %d open, want it ended and released", b.store, b.cfg.Stores.Open())
			}
		})
	}
}

// TestStoreRunReleases checks that a session ending gives the claim back.
func TestStoreRunReleases(t *testing.T) {
	b := newStoreBrain(t, 1000)
	id, _ := b.cfg.Stores.claim(Point{})
	b.store.claim, b.store.claimed, b.store.step = id, true, storeOpen
	done := make(chan struct{})
	close(done)
	if err := b.Run(done, nil); err != nil {
		t.Fatal(err)
	}
	if b.cfg.Stores.Open() != 0 {
		t.Errorf("registry %d open after Run, want 0", b.cfg.Stores.Open())
	}
}

// TestWorldTeleportClosesStore: the server closes the bot's store on a
// loading-screen teleport and says nothing.
func TestWorldTeleportClosesStore(t *testing.T) {
	for _, fast := range []bool{false, true} {
		w := newTestWorld(t)
		w.Self.StoreType = storeTypeSell
		w.Apply(TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5}, Fast: fast}, t0)
		if want := map[bool]uint8{false: 0, true: storeTypeSell}[fast]; w.Self.StoreType != want {
			t.Errorf("fast %v: StoreType %d, want %d", fast, w.Self.StoreType, want)
		}
	}
}

func TestStoreRegistry(t *testing.T) {
	r := NewStoreRegistry([]Point{{}, {X: 100000}}, 2, 150)
	steps := []struct {
		at   Point
		want bool
	}{
		{Point{}, true},
		{Point{X: 100}, false}, // within spacing
		{Point{X: 200}, true},
		{Point{X: 400}, false}, // town full
		{Point{X: 100000}, true},
		{Point{X: 50000}, false}, // no town
	}
	var ids []int
	for _, s := range steps {
		id, ok := r.claim(s.at)
		if ok != s.want || r.hasRoom(s.at) {
			t.Fatalf("claim(%+v) = %v, want %v, and no room left there", s.at, ok, s.want)
		}
		if ok {
			ids = append(ids, id)
		}
	}
	r.release(ids[0])
	if !r.hasRoom(Point{X: 400}) {
		t.Error("no room after a release, want the town's place back")
	}
	for _, id := range ids {
		r.release(id)
	}
	if r.Open() != 0 {
		t.Errorf("Open = %d after every release, want 0", r.Open())
	}
}

// TestStoreRegistryRace claims places of one town from many bots at once:
// no more than its cap get one.
func TestStoreRegistryRace(t *testing.T) {
	r := NewStoreRegistry([]Point{{}}, 5, 150)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ids []int
	for i := range 16 {
		wg.Go(func() {
			if id, ok := r.claim(Point{X: int32(i) * 160}); ok {
				mu.Lock()
				ids = append(ids, id)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(ids) != 5 || r.Open() != 5 {
		t.Fatalf("%d claims, %d open, want 5", len(ids), r.Open())
	}
	for _, id := range ids {
		wg.Go(func() { r.release(id) })
	}
	wg.Wait()
	if r.Open() != 0 {
		t.Errorf("Open = %d, want 0", r.Open())
	}
}

func TestTravelMapTowns(t *testing.T) {
	m := &TravelMap{gatekeepers: []gatekeeper{{at: Point{}}, {at: Point{X: 1000}}, {at: Point{X: 50000}}, {at: Point{X: 52000}}}}
	if got, want := m.Towns(), []Point{{}, {X: 50000}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Towns = %v, want %v", got, want)
	}
}

func TestStoreTitleFits(t *testing.T) {
	for title, want := range map[string]bool{
		"Loot for sale":                        true,
		"abcdefghijklmnopqrstuvwxyzabc":        true,
		"abcdefghijklmnopqrstuvwxyzabcd":       false,
		"\U0001F600" + strings.Repeat("a", 27): true, // a surrogate pair counts two
		"\U0001F600" + strings.Repeat("a", 28): false,
	} {
		if got := storeTitleFits(title); got != want {
			t.Errorf("storeTitleFits(%q) = %v, want %v", title, got, want)
		}
	}
}
