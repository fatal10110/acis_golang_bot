package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// craftOneSteel crafts the cycle's one steel from the carried ore at 2 s:
// the ore (object 200) goes, the steel lands on object 300, which carried
// had before.
func craftOneSteel(t *testing.T, b *Brain, carried int32) {
	t.Helper()
	sendCraft(t, b, at(2*time.Second))
	add := ItemChange{State: ItemAdded, Item: Item{ObjectID: 300, TemplateID: steel, Count: 1}}
	if carried > 0 {
		add = ItemChange{State: ItemModified, Item: Item{ObjectID: 300, TemplateID: steel, Count: carried + 1}}
	}
	evs := []Event{madeInfo(RecipeMadeSuccess), InventoryUpdate{Changes: []ItemChange{
		{State: ItemRemoved, Item: Item{ObjectID: 200, TemplateID: ironOre}}, add,
	}}}
	if err := b.handle(evs, at(2*time.Second+100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
}

// TestBrainCrafterClanShotsDeposit (#688): the shots a chain for the clan
// makes are deposited at the cycle's end as the clan's, the bot's own
// shots staying carried; an order's chain's shots go as the order's, not
// counted as the clan's.
func TestBrainCrafterClanShotsDeposit(t *testing.T) {
	tests := []struct {
		name string
		// own is the steel the bot carries ahead of the cycle, its own.
		own int32
		// order posts an order of one steel with its materials in.
		order         bool
		wantClanMade  map[int32]int64
		wantDelivered int64
	}{
		{name: "clan chain, no own shots", wantClanMade: map[int32]int64{steel: 1}},
		{name: "clan chain, own shots stay", own: 5, wantClanMade: map[int32]int64{steel: 1}},
		{name: "order chain", own: 5, order: true, wantDelivered: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []Item{adenaStack(1000), {ObjectID: 200, TemplateID: ironOre, Count: 2}}
			if tt.own > 0 {
				items = append(items, Item{ObjectID: 300, TemplateID: steel, Count: tt.own})
			}
			b := newCrafterBrain(t, items...)
			asShot(b)
			board, clan := b.cfg.CraftBoard, b.world.Clan.ID
			var id OrderID
			if tt.order {
				id, _ = board.Post(clan, "Member", steel, 1)
				board.MaterialsIn(clan, id)
			}
			b.craftPlan(at(time.Second), false)
			craftOneSteel(t, b, tt.own)
			c := &b.craft
			if !maps.Equal(c.clanMade, tt.wantClanMade) {
				t.Fatalf("clanMade %v, want %v", c.clanMade, tt.wantClanMade)
			}
			b.decide(at(3 * time.Second))
			if c.phase != craftDeposit || !slices.Equal(c.req.Deposit, []WarehouseRow{{TemplateID: steel, Count: 1}}) {
				t.Fatalf("phase %d deposit %+v, want one steel deposited", c.phase, c.req.Deposit)
			}
			if !maps.Equal(c.clanDepositing, tt.wantClanMade) || c.delivering[id] != tt.wantDelivered {
				t.Fatalf("clanDepositing %v delivering %v, want %v and %d", c.clanDepositing, c.delivering, tt.wantClanMade, tt.wantDelivered)
			}
			c.asked, c.askedAt = true, at(4*time.Second)
			if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{
				{State: ItemModified, Item: Item{ObjectID: 300, TemplateID: steel, Count: tt.own}},
			}}}, at(5*time.Second)); err != nil {
				t.Fatal(err)
			}
			b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseDone, At: at(5 * time.Second)}
			b.decide(at(6 * time.Second))
			if c.phase != craftIdle || len(c.clanMade) != 0 || b.carriedLoose(steel) != int64(tt.own) {
				t.Fatalf("phase %d clanMade %v carried %d, want the cycle over, nothing owed, the own %d kept",
					c.phase, c.clanMade, b.carriedLoose(steel), tt.own)
			}
		})
	}
}

// TestBrainCrafterClanShotsFailedDeposit (#688): the clan's shots a
// failed deposit leaves carried carry over to the next cycle, at most what
// the bot still carries, and that cycle deposits them; a claimed order
// takes them first.
func TestBrainCrafterClanShotsFailedDeposit(t *testing.T) {
	tests := []struct {
		name string
		// left is the steel carried once the deposit failed.
		left int32
		// order posts an order of one steel ahead of the next cycle.
		order        bool
		wantClanMade map[int32]int64
		wantDeposit  []WarehouseRow
	}{
		{name: "carried", left: 6, wantClanMade: map[int32]int64{steel: 1}, wantDeposit: []WarehouseRow{{TemplateID: steel, Count: 1}}},
		{name: "spent since", left: 0},
		{name: "an order takes them", left: 6, order: true, wantClanMade: map[int32]int64{steel: 1}, wantDeposit: []WarehouseRow{{TemplateID: steel, Count: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newCrafterBrain(t, adenaStack(1000), Item{ObjectID: 200, TemplateID: ironOre, Count: 2},
				Item{ObjectID: 300, TemplateID: steel, Count: 5})
			asShot(b)
			board, clan := b.cfg.CraftBoard, b.world.Clan.ID
			b.craftPlan(at(time.Second), false)
			craftOneSteel(t, b, 5)
			b.decide(at(3 * time.Second))
			c := &b.craft
			c.asked, c.askedAt = true, at(4*time.Second)
			if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{
				{State: ItemModified, Item: Item{ObjectID: 300, TemplateID: steel, Count: tt.left}},
			}}}, at(5*time.Second)); err != nil {
				t.Fatal(err)
			}
			b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseRefused, At: at(5 * time.Second)}
			b.decide(at(6 * time.Second))
			if c.phase != craftIdle || !maps.Equal(c.clanMade, tt.wantClanMade) {
				t.Fatalf("phase %d clanMade %v, want the cycle over owing %v", c.phase, c.clanMade, tt.wantClanMade)
			}
			var id OrderID
			if tt.order {
				id, _ = board.Post(clan, "Member", steel, 1)
				board.MaterialsIn(clan, id)
			}
			b.wh.status.Known, b.wh.status.Contents = true, nil
			b.craftPlan(at(2*time.Minute), true)
			if len(tt.wantDeposit) == 0 {
				if c.phase != craftIdle {
					t.Fatalf("phase %d deposit %+v, want nothing deposited", c.phase, c.req.Deposit)
				}
				return
			}
			if c.phase != craftDeposit || !slices.Equal(c.req.Deposit, tt.wantDeposit) {
				t.Fatalf("phase %d deposit %+v, want %+v", c.phase, c.req.Deposit, tt.wantDeposit)
			}
			if tt.order && (c.delivering[id] != 1 || len(c.clanMade) != 0 || len(c.clanDepositing) != 0) {
				t.Fatalf("delivering %v clanMade %v clanDepositing %v, want the order's steel only",
					c.delivering, c.clanMade, c.clanDepositing)
			}
		})
	}
}

// TestBrainCrafterPoolOrder (#688): a claimed order the plan holds no
// chain for is delivered at once from the warehouse's free count of its
// product, for its whole count, never from a member's reserved count.
func TestBrainCrafterPoolOrder(t *testing.T) {
	tests := []struct {
		name   string
		stored int32
		count  int64
		// reserved delivers an earlier order of this many steel to another
		// member: the board reserves them.
		reserved      int64
		wantDelivered bool
	}{
		{name: "whole count free", stored: 3, count: 3, wantDelivered: true},
		{name: "part of the count", stored: 2, count: 3},
		{name: "reserved for a member", stored: 4, count: 3, reserved: 2},
		{name: "free beyond the reserved", stored: 5, count: 3, reserved: 2, wantDelivered: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newCrafterBrain(t, adenaStack(1000))
			asShot(b)
			board, clan := b.cfg.CraftBoard, b.world.Clan.ID
			if tt.reserved > 0 {
				first, _ := board.Post(clan, "Other", steel, tt.reserved)
				board.MaterialsIn(clan, first)
				board.Claim(clan, first, selfID, at(0))
				board.Crafted(clan, first, selfID, at(0))
				board.Deliver(clan, first, selfID, tt.reserved)
			}
			id, _ := board.Post(clan, "Member", steel, tt.count)
			board.MaterialsIn(clan, id)
			b.wh.status.Known = true
			b.wh.status.Contents = []WarehouseItem{{ObjectID: 0x10000500, TemplateID: steel, Count: tt.stored}}
			now := at(time.Minute)
			b.craftPlan(now, true)
			delivered := board.Owned(clan, "Member")[steel] == tt.count
			if delivered != tt.wantDelivered || b.craft.phase != craftIdle {
				t.Fatalf("owned %v phase %d, want delivered %v and the cycle over",
					board.Owned(clan, "Member"), b.craft.phase, tt.wantDelivered)
			}
			if !tt.wantDelivered {
				if os := board.Orders(clan, now); len(os) != 1 || os[0].ID != id || os[0].State != OrderClaimed {
					t.Fatalf("orders %+v, want the order still claimed", os)
				}
			}
		})
	}
}
