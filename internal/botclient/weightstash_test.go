package botclient

import (
	"reflect"
	"testing"
	"time"
)

// stashMat is a second kept loot template of the stash tests, heavier than
// keptMat.
const stashMat int32 = 1866

// newStashTripBrain is newTripBrain at 67 % of its limit, keeping keptMat
// (Shop.Keep), with warehouse keeper keeperNPC on the travel map.
func newStashTripBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newTripBrain(t, append([]Item{potionStack(20)}, items...)...)
	b.persona.Shop.Keep = []int32{keptMat, stashMat}
	b.cfg.Travel.weights[stashMat] = 30
	b.cfg.Travel.keepers = map[int32]bool{keeperNPC: true}
	b.cfg.Travel.spawns = map[int32][]Point{keeperNPC: {town}}
	b.world.Self.Load = 67000
	return b
}

// TestStashCause checks the half-speed stash trigger (#449): in the
// half-speed band with nothing to shed, kept items the bot does not use
// send it home; under the band, with junk to sell (the load trigger), or
// with only worn, quest, used, held or weightless items, no stash trip.
func TestStashCause(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		load  int32
		edit  func(b *Brain)
		want  tripCause
	}{
		{name: "kept items at 67 %", items: []Item{lootStack(60, keptMat, 500)}, want: causeStash},
		{name: "kept items past ReturnLoad, under the band", items: []Item{lootStack(60, keptMat, 500)}, load: 65000},
		{name: "junk to sell", items: []Item{lootStack(60, keptMat, 500), lootStack(61, junkMat, 10)}, want: causeLoad},
		{name: "potions only", load: 67000},
		{name: "worn", items: []Item{{ObjectID: 60, TemplateID: keptMat, Count: 1, Equipped: true}}},
		{name: "quest item", items: []Item{{ObjectID: 60, TemplateID: keptMat, Count: 5, SubCategory: subCategoryQuest}}},
		{name: "held by the keep hook", items: []Item{lootStack(60, keptMat, 500)}, edit: func(b *Brain) {
			b.persona.Holds = func(_ *World, it Item) Hold { return Hold{Keep: it.TemplateID == keptMat} }
		}},
		{name: "weight unknown", items: []Item{lootStack(60, keptMat, 500)}, edit: func(b *Brain) { delete(b.cfg.Travel.weights, keptMat) }},
		{name: "ReturnLoad 0", items: []Item{lootStack(60, keptMat, 500)}, edit: func(b *Brain) { b.persona.Supply.ReturnLoad = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStashTripBrain(t, tt.items...)
			if tt.load != 0 {
				b.world.Self.Load = tt.load
			}
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.supplyCause(); got != tt.want {
				t.Fatalf("supplyCause = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestStashDue checks the stash trip's guard: it starts with a keeper on
// the travel map, and is held stashRetry after a stash that moved nothing,
// or with no keeper on the map, which Status reports.
func TestStashDue(t *testing.T) {
	now := at(2 * time.Hour)
	tests := []struct {
		name      string
		last      WarehouseResult
		noKeepers bool
		want      tripCause
		wantStash WarehouseOutcome
	}{
		{name: "keeper on the map", want: causeStash},
		{name: "no keeper on the map", noKeepers: true, wantStash: WarehouseUnreachable},
		{name: "full 30 min ago", last: WarehouseResult{Outcome: WarehouseFull, At: now.Add(-30 * time.Minute)}, wantStash: WarehouseFull},
		{name: "full an hour ago", last: WarehouseResult{Outcome: WarehouseFull, At: now.Add(-stashRetry)}, want: causeStash, wantStash: WarehouseFull},
		{name: "done 30 min ago", last: WarehouseResult{Outcome: WarehouseDone, At: now.Add(-30 * time.Minute)}, want: causeStash, wantStash: WarehouseDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStashTripBrain(t, lootStack(60, keptMat, 500))
			b.wh.stashed = tt.last
			if tt.noKeepers {
				b.cfg.Travel.keepers = nil
			}
			if got := b.supplyDue(now); got != tt.want {
				t.Fatalf("supplyDue = %d, want %d", got, tt.want)
			}
			if got := b.wh.stashed.Outcome; got != tt.wantStash {
				t.Fatalf("stashed = %d, want %d", got, tt.wantStash)
			}
		})
	}
}

// TestStashRows checks what the stash deposits: the kept stacks the bot
// does not use, heaviest first; never adena, potions or a valuable, nor
// the junk the shop visit sells while selling is on (SellAt).
func TestStashRows(t *testing.T) {
	kept := []WarehouseRow{
		{TemplateID: stashMat, Count: 200, Object: 61},
		{TemplateID: keptMat, Count: 500, Object: 60},
	}
	tests := []struct {
		name   string
		sellAt int
		want   []WarehouseRow
	}{
		{name: "selling on: junk left to the shop", sellAt: 5, want: kept},
		{name: "selling off: junk stashed", want: append(kept, WarehouseRow{TemplateID: junkMat, Count: 10, Object: 62})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStashTripBrain(t, adenaStack(5000), lootStack(60, keptMat, 500), lootStack(61, stashMat, 200), lootStack(62, junkMat, 10))
			b.persona.Shop.SellAt = tt.sellAt
			if got := b.stashRows(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("stashRows = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// newStashBrain is newPrivateBrain at 70 % of its limit back from a supply
// trip (stashDue), carrying items, the skins of a known weight and kept
// (Shop.Keep).
func newStashBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newPrivateBrain(t, items...)
	b.cfg.Travel.weights = map[int32]int32{animalSkin: 50}
	b.persona.Warehouse.Reach = privReach
	b.persona.Shop.Keep = []int32{animalSkin}
	b.world.Self.Load, b.world.Self.MaxLoad = 70000, 100000
	b.stashDue = true
	return b
}

// TestStashDeposit runs the stash after an arrival: the private deposit of
// the kept skins at the keeper in sight, its result in Status.
func TestStashDeposit(t *testing.T) {
	b := newStashBrain(t, adenaStack(1000), whCarriedSkins)
	deposit := []WarehouseItem{{ObjectID: 102, TemplateID: animalSkin, Count: 10}}
	steps := privateOpenSteps(0, true)
	steps = append(steps,
		trainerStep{
			name: "private list", at: 5 * time.Second, events: []Event{WarehouseDepositList{Type: privateWarehouseType, Items: deposit}},
			wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: depositReq(warehouseRow{102, animalSkin, 10}),
		},
		trainerStep{name: "deposited", at: 6 * time.Second, events: []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: whCarriedSkins}}}}},
	)
	runTrainerSteps(t, b, steps)
	if b.stashDue || b.wh.running {
		t.Fatalf("stashDue %v, errand running %v, want the stash over", b.stashDue, b.wh.running)
	}
	want := []WarehouseRow{{TemplateID: animalSkin, Count: 10, Object: 102}}
	if got := b.wh.stashed; got.Outcome != WarehouseDone || !reflect.DeepEqual(got.Rows, want) {
		t.Fatalf("stashed = %+v, want done with %+v", got, want)
	}
}

// TestStashStatus checks that Status reports the last stash.
func TestStashStatus(t *testing.T) {
	for _, r := range []WarehouseResult{
		{},
		{Outcome: WarehouseUnreachable, At: t0},
		{Outcome: WarehouseDone, At: t0, Rows: []WarehouseRow{{TemplateID: animalSkin, Count: 10, Object: 102}}},
	} {
		b, _ := newCommandBrain(t, DefaultPersona())
		l := linkBrain(b, standing{})
		b.wh.stashed = r
		b.publishStatus(t0, true)
		if got := l.board.load().Stash; !reflect.DeepEqual(got, r) {
			t.Fatalf("Status.Stash = %+v, want %+v", got, r)
		}
	}
}

// TestStashStart checks when an arrival stashes: only slowed, with kept
// items to stash, the shop visit over and no stash held; with no keeper or
// no adena for the fee it records why.
func TestStashStart(t *testing.T) {
	now := at(time.Hour)
	tests := []struct {
		name      string
		items     []Item
		edit      func(b *Brain)
		running   bool
		due       bool
		wantStash WarehouseOutcome
	}{
		{name: "slowed with kept skins", items: []Item{adenaStack(1000), whCarriedSkins}, running: true},
		{name: "under the band", items: []Item{adenaStack(1000), whCarriedSkins}, edit: func(b *Brain) { b.world.Self.Load = 66000 }},
		{name: "nothing to stash", items: []Item{adenaStack(1000)}},
		{name: "junk skins left to the shop", items: []Item{adenaStack(1000), whCarriedSkins}, edit: func(b *Brain) { b.persona.Shop.Keep = nil }},
		{name: "not due", items: []Item{adenaStack(1000), whCarriedSkins}, edit: func(b *Brain) { b.stashDue = false }},
		{name: "shop visit on", items: []Item{adenaStack(1000), whCarriedSkins}, edit: func(b *Brain) { b.shop.v = npcVisit{npcID: 1, at: Point{X: 500}} }, due: true},
		{name: "held", items: []Item{adenaStack(1000), whCarriedSkins}, edit: func(b *Brain) {
			b.wh.stashed = WarehouseResult{Outcome: WarehouseFull, At: now.Add(-time.Minute)}
		}, wantStash: WarehouseFull},
		{name: "fee short", items: []Item{adenaStack(29), whCarriedSkins}, wantStash: WarehouseShort},
		{name: "no keeper on the map", items: []Item{adenaStack(1000), whCarriedSkins}, edit: func(b *Brain) { b.cfg.Travel.keepers = nil }, wantStash: WarehouseUnreachable},
		{name: "no keeper within reach", items: []Item{adenaStack(1000), whCarriedSkins}, edit: func(b *Brain) {
			b.cfg.Travel.spawns[keeperNPC] = []Point{{X: 9000}}
			b.world.Objects = map[int32]*Object{}
		}, wantStash: WarehouseUnreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStashBrain(t, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			b.queueStash(now)
			if b.wh.running != tt.running || b.stashDue != tt.due {
				t.Fatalf("errand running %v stashDue %v, want %v %v", b.wh.running, b.stashDue, tt.running, tt.due)
			}
			if tt.running {
				want := []WarehouseRow{{TemplateID: animalSkin, Count: 10, Object: 102}}
				if r := b.wh.req; !r.Private || !r.stash || !reflect.DeepEqual(r.Deposit, want) {
					t.Fatalf("request %+v, want the private stash of %+v", r, want)
				}
			}
			if got := b.wh.stashed.Outcome; got != tt.wantStash {
				t.Fatalf("stashed = %d, want %d", got, tt.wantStash)
			}
		})
	}
}
