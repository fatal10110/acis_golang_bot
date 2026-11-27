package botclient

import (
	"testing"
	"time"
)

// asShot makes steel one of the bot's own supplies, as Soulshot D is: the
// crafting stock leaves it out (craftStock), so only a crafted order
// deposits it.
func asShot(b *Brain) {
	b.cfg.Travel.shots = map[int32]shotItem{steel: {}}
}

// TestBrainCrafterCarriedOrderFlow replays #577 (TestLiveClanMember,
// logs of heads 84f7eaf and aac2d5a): the member's deposit of the order's
// materials lands, the crafter's cycle plans a chain for the clan on them
// before the member posts the order (Claimed 0, Chains 1) and keeps the
// shots it made as supplies; the next cycle claims the order with nothing
// left to craft it from. That cycle delivers the carried shots instead of
// ending "nothing to craft".
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
		{ItemRemoved, Item{ObjectID: 200, TemplateID: ironOre}}, {ItemAdded, Item{ObjectID: 300, TemplateID: steel, Count: 1}},
	}}}
	if err := b.handle(evs, at(2*time.Second+100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	b.decide(at(3 * time.Second))
	if b.craft.phase != craftIdle {
		t.Fatalf("phase %d, want the cycle over with the shots carried", b.craft.phase)
	}
	id, _ := board.Post(clan, "Member", steel, 1)
	board.MaterialsIn(clan, id)
	b.craftPlan(at(2*time.Minute), true)
	if os := board.Orders(clan, at(2*time.Minute)); len(os) != 1 || os[0].State != OrderCrafted {
		t.Fatalf("orders %+v, want the order crafted from the carried shots", os)
	}
	if c := &b.craft; c.phase != craftDeposit || c.delivering[id] != 1 {
		t.Fatalf("phase %d delivering %v, want the deposit carrying the order", c.phase, c.delivering)
	}
	b.craft.asked, b.craft.askedAt = true, at(2*time.Minute+time.Second)
	b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseDone, At: at(2*time.Minute + 2*time.Second)}
	b.decide(at(2*time.Minute + 3*time.Second))
	if os := board.Orders(clan, at(2*time.Minute+3*time.Second)); len(os) != 0 || board.Owned(clan, "Member")[steel] != 1 {
		t.Fatalf("orders %+v owned %v, want the order delivered", os, board.Owned(clan, "Member"))
	}
}

// TestBrainCrafterCarriedOrder: a claimed order the plan holds no chain
// for is crafted from the bag only for its whole count and beyond what the
// bot's crafted orders take; an order the plan crafts is not filled from
// the bag; a plan without the warehouse list never fills (the order's
// materials may still lie in the pool).
func TestBrainCrafterCarriedOrder(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		count int64
		// crafted posts an earlier order of one steel, crafted by the bot.
		crafted bool
		// noList plans without the warehouse list (craftPlan(now, false)).
		noList    bool
		wantState OrderState
		wantPhase crafterPhase
	}{
		{
			name:  "whole count carried",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 3}}, count: 3,
			wantState: OrderCrafted, wantPhase: craftDeposit,
		},
		{
			name:  "part of the count carried",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 2}}, count: 3,
			wantState: OrderClaimed, wantPhase: craftIdle,
		},
		{
			name:  "equipped copies do not count",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 1, Equipped: true}}, count: 1,
			wantState: OrderClaimed, wantPhase: craftIdle,
		},
		{
			name:  "carried for a crafted order",
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 1}}, count: 1, crafted: true,
			wantState: OrderClaimed, wantPhase: craftDeposit,
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
			items: []Item{{ObjectID: 300, TemplateID: steel, Count: 3}}, count: 3, noList: true,
			wantState: OrderClaimed, wantPhase: craftIdle,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newCrafterBrain(t, append([]Item{adenaStack(1000)}, tt.items...)...)
			asShot(b)
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
			b.craftPlan(now, !tt.noList)
			var got CraftOrder
			for _, o := range board.Orders(clan, now) {
				if o.ID == id {
					got = o
				}
			}
			if got.State != tt.wantState || got.Crafter != selfID {
				t.Fatalf("order %+v, want %v by the crafter", got, tt.wantState)
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
