package botclient

import (
	"slices"
	"testing"
	"time"
)

// asShot makes steel a shot, as Soulshot D is: the crafting stock leaves
// it out (craftStock), so only a crafted order or a chain for the clan
// deposits it.
func asShot(b *Brain) {
	b.cfg.Travel.shots = map[int32]shotItem{steel: {}}
}

// TestBrainCrafterCarriedOrderFlow replays #577 (TestLiveClanMember,
// logs of heads 84f7eaf and aac2d5a): the member's deposit of the order's
// materials lands, the crafter's cycle plans a chain for the clan on them
// before the member posts the order (Claimed 0, Chains 1) and deposits the
// shots it made as the clan's (#688); the next cycle claims the order with
// nothing left to craft it from. That cycle delivers the order from the
// shots in the warehouse instead of ending "nothing to craft".
func TestBrainCrafterCarriedOrderFlow(t *testing.T) {
	b := newCrafterBrain(t, adenaStack(1000), Item{ObjectID: 200, TemplateID: ironOre, Count: 2})
	asShot(b)
	board, clan := b.cfg.CraftBoard, b.world.Clan.ID
	b.craftPlan(at(time.Second), false)
	if c := &b.craft; len(c.plan.Chains) != 1 || c.plan.Chains[0].Order != -1 {
		t.Fatalf("plan %+v, want one chain for the clan", c.plan.Chains)
	}
	sendCraft(t, b, at(2*time.Second))
	evs := []Event{madeInfo(RecipeMadeSuccess), InventoryUpdate{Changes: []ItemChange{
		{State: ItemRemoved, Item: Item{ObjectID: 200, TemplateID: ironOre}}, {State: ItemAdded, Item: Item{ObjectID: 300, TemplateID: steel, Count: 1}},
	}}}
	if err := b.handle(evs, at(2*time.Second+100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	b.decide(at(3 * time.Second))
	if c := &b.craft; c.phase != craftDeposit || !slices.Equal(c.req.Deposit, []WarehouseRow{{TemplateID: steel, Count: 1}}) {
		t.Fatalf("phase %d deposit %+v, want the clan's shots deposited", c.phase, c.req.Deposit)
	}
	b.craft.asked, b.craft.askedAt = true, at(4*time.Second)
	if err := b.handle([]Event{InventoryUpdate{Changes: []ItemChange{
		{State: ItemRemoved, Item: Item{ObjectID: 300, TemplateID: steel}},
	}}}, at(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseDone, At: at(5 * time.Second)}
	b.decide(at(6 * time.Second))
	if c := &b.craft; c.phase != craftIdle || len(c.clanMade) != 0 {
		t.Fatalf("phase %d clanMade %v, want the cycle over with nothing owed", c.phase, c.clanMade)
	}
	id, _ := board.Post(clan, "Member", steel, 1)
	board.MaterialsIn(clan, id)
	b.wh.status.Known = true
	b.wh.status.Contents = []WarehouseItem{{ObjectID: 0x10000500, TemplateID: steel, Count: 1}}
	b.craftPlan(at(2*time.Minute), true)
	if os := board.Orders(clan, at(2*time.Minute)); len(os) != 0 || board.Owned(clan, "Member")[steel] != 1 {
		t.Fatalf("orders %+v owned %v, want the order delivered from the warehouse", os, board.Owned(clan, "Member"))
	}
	if b.craft.phase != craftIdle {
		t.Fatalf("phase %d, want the cycle over", b.craft.phase)
	}
}

// TestBrainCrafterCarriedOrder: a claimed order the plan holds no chain
// for is crafted from the bag only for its whole count and beyond what the
// bot's crafted orders take; an own supply (a shot) only out of what the
// chains for the clan made, never the bot's own (#688); an order the plan
// crafts is not filled from the bag; a plan without the warehouse list
// never fills (the order's materials may still lie in the pool).
func TestBrainCrafterCarriedOrder(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		count int64
		// crafted posts an earlier order of one steel, crafted by the bot.
		crafted bool
		// noList plans without the warehouse list (craftPlan(now, false)).
		noList bool
		// notShot leaves steel an ordinary product, not an own supply.
		notShot bool
		// clanMade is the steel the chains for the clan made.
		clanMade int64
		// stored is the steel the warehouse list holds.
		stored        int32
		wantState     OrderState
		wantPhase     crafterPhase
		wantClanMade  int64
		wantDelivered bool
	}{
		{
			name:  "own shots carried: not filled",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 3}}, count: 3,
			wantState: OrderClaimed, wantPhase: craftIdle,
		},
		{
			name:  "own shots carried, the warehouse holds the count",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 3}}, count: 3, stored: 3,
			wantState: OrderDelivered, wantPhase: craftIdle, wantDelivered: true,
		},
		{
			name:  "the clan's shots cover the count",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 5}}, count: 3, clanMade: 4,
			wantState: OrderCrafted, wantPhase: craftDeposit, wantClanMade: 1,
		},
		{
			name:  "the clan's shots cover part of the count",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 5}}, count: 3, clanMade: 2,
			wantState: OrderClaimed, wantPhase: craftDeposit, wantClanMade: 2,
		},
		{
			name:  "the clan's shots cover part, the warehouse the count",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 5}}, count: 3, clanMade: 2, stored: 3,
			wantState: OrderDelivered, wantPhase: craftDeposit, wantClanMade: 2, wantDelivered: true,
		},
		{
			name:  "not an own supply: whole count carried",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 3}}, count: 3, notShot: true,
			wantState: OrderCrafted, wantPhase: craftDeposit,
		},
		{
			name:  "part of the count carried",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 2}}, count: 3, notShot: true,
			wantState: OrderClaimed, wantPhase: craftDeposit,
		},
		{
			name:  "equipped copies do not count",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 1, Equipped: true}}, count: 1, notShot: true,
			wantState: OrderClaimed, wantPhase: craftIdle,
		},
		{
			name:  "carried for a crafted order",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 1}}, count: 1, crafted: true, clanMade: 1,
			wantState: OrderClaimed, wantPhase: craftDeposit, wantClanMade: 1,
		},
		{
			name: "materials carried: crafted, not filled",
			items: []Item{
				{ObjectID: 200, TemplateID: ironOre, Count: 2}, {ObjectID: 300, TemplateID: steel, Count: 1},
			},
			count: 1, wantState: OrderClaimed, wantPhase: craftMake,
		},
		{
			name:  "planned without the list: not filled",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 3}}, count: 3, noList: true, notShot: true,
			wantState: OrderClaimed, wantPhase: craftDeposit,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newCrafterBrain(t, append([]Item{adenaStack(1000)}, tt.items...)...)
			if !tt.notShot {
				asShot(b)
			}
			if tt.clanMade > 0 {
				b.craft.clanMade = map[int32]int64{steel: tt.clanMade}
			}
			board, clan := b.cfg.CraftBoard, b.world.Clan.ID
			var first OrderID
			if tt.crafted {
				first, _ = board.Post(clan, "Member", steel, 1)
				board.MaterialsIn(clan, first)
				board.Claim(clan, first, selfID, at(0))
				board.Crafted(clan, first, selfID, at(0))
			}
			id, _ := board.Post(clan, "Member", steel, tt.count)
			board.MaterialsIn(clan, id)
			now := at(time.Minute)
			b.wh.status.Known, b.wh.status.Contents = true, nil
			if tt.stored > 0 {
				b.wh.status.Contents = []WarehouseItem{{ObjectID: 0x10000500, TemplateID: steel, Count: tt.stored}}
			}
			b.craftPlan(now, !tt.noList)
			if delivered := board.Owned(clan, "Member")[steel] == tt.count; delivered != tt.wantDelivered {
				t.Fatalf("owned %v, want delivered %v", board.Owned(clan, "Member"), tt.wantDelivered)
			}
			if !tt.wantDelivered {
				var got CraftOrder
				for _, o := range board.Orders(clan, now) {
					if o.ID == id {
						got = o
					}
				}
				if got.State != tt.wantState || got.Crafter != selfID {
					t.Fatalf("order %+v, want %v by the crafter", got, tt.wantState)
				}
			}
			if got := b.craft.clanMade[steel]; got != tt.wantClanMade {
				t.Fatalf("clanMade %d, want %d", got, tt.wantClanMade)
			}
			c := &b.craft
			if c.phase != tt.wantPhase {
				t.Fatalf("phase %d, want %d", c.phase, tt.wantPhase)
			}
			if _, ok := c.delivering[id]; ok != (tt.wantState == OrderCrafted) {
				t.Fatalf("delivering %v, order %d noted %v", c.delivering, id, ok)
			}
			if tt.crafted && c.delivering[first] != 1 {
				t.Fatalf("delivering %v, want the crafted order's steel", c.delivering)
			}
		})
	}
}
