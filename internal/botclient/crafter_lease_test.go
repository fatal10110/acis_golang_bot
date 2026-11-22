package botclient

import (
	"reflect"
	"testing"
	"time"
)

// TestBrainCrafterLeaseRenewal: every action the withdrawal's errand sends
// renews the board's warehouse lease, so an errand longer than
// craftLeaseTTL keeps it; nothing else renews it.
func TestBrainCrafterLeaseRenewal(t *testing.T) {
	tests := []struct {
		name    string
		phase   crafterPhase
		running bool
		want    bool
	}{
		{name: "withdrawal under way", phase: craftWithdraw, running: true, want: true},
		{name: "withdrawal not started", phase: craftWithdraw},
		{name: "crafting", phase: craftMake, running: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newCrafterBrain(t)
			board, clan := b.cfg.CraftBoard, b.world.Clan.ID
			if !board.TakeLease(clan, selfID, t0) {
				t.Fatal("lease not taken")
			}
			b.craft.phase, b.wh.running = tc.phase, tc.running
			// One send a minute, past craftLeaseTTL.
			for d := time.Minute; d <= 3*time.Minute; d += time.Minute {
				b.craftSent(Action{kind: actTalk}, t0.Add(d))
			}
			if got := board.HoldsLease(clan, selfID, t0.Add(3*time.Minute+time.Second)); got != tc.want {
				t.Fatalf("holds the lease = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBrainCrafterWithdrawDelivers: a crafted order's product an earlier
// cycle failed to deposit rides the withdrawal's deposit; the order is
// delivered once that deposit lands, and stays crafted when it fails.
func TestBrainCrafterWithdrawDelivers(t *testing.T) {
	tests := []struct {
		name      string
		deposit   WarehouseOutcome
		delivered bool
	}{
		{name: "deposit lands", deposit: WarehouseDone, delivered: true},
		{name: "deposit refused", deposit: WarehouseRefused},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newCrafterBrain(t, adenaStack(1000), Item{ObjectID: 300, TemplateID: steel, Count: 1})
			board, clan := b.cfg.CraftBoard, b.world.Clan.ID
			id, _ := board.Post(clan, "Member", steel, 1)
			if !board.MaterialsIn(clan, id) || !board.Claim(clan, id, selfID, t0) || !board.Crafted(clan, id, selfID, t0) {
				t.Fatal("order not crafted")
			}
			b.wh.status.Contents, b.wh.status.Known = []WarehouseItem{whStoredOre}, true
			b.craftPlan(at(time.Second), true)
			c := &b.craft
			if c.phase != craftWithdraw || c.delivering[id] != 1 {
				t.Fatalf("phase %d delivering %v, want the withdrawal carrying the order", c.phase, c.delivering)
			}
			if got, want := c.req.Deposit, []WarehouseRow{{TemplateID: steel, Count: 1}}; !reflect.DeepEqual(got, want) {
				t.Fatalf("withdrawal's deposit = %+v, want %+v", got, want)
			}
			c.asked, c.askedAt = true, at(2*time.Second)
			b.wh.status.Withdraw = WarehouseResult{Outcome: WarehouseDone, At: at(3 * time.Second)}
			b.wh.status.Deposit = WarehouseResult{Outcome: tc.deposit, At: at(3 * time.Second)}
			b.decide(at(4 * time.Second))
			os := board.Orders(clan, at(4*time.Second))
			if tc.delivered {
				if len(os) != 0 || board.Owned(clan, "Member")[steel] != 1 {
					t.Fatalf("orders %+v owned %v, want the order delivered", os, board.Owned(clan, "Member"))
				}
			} else if len(os) != 1 || os[0].State != OrderCrafted {
				t.Fatalf("orders %+v, want the order still crafted", os)
			}
			if c.delivering != nil {
				t.Fatalf("delivering %v after the withdrawal, want none", c.delivering)
			}
		})
	}
}
