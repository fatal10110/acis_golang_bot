package botclient

import (
	"testing"
	"time"
)

// TestMemberVisitUnderLoad reproduces the visit of #470: a member whose
// load at its visit leaves no room under Supply.BuyLoad (the live member had
// looted other players' ground items up to 63 % of its limit) wants no
// shots (supplies caps them by weight), so its visit deposits its Soul Ore
// as a kept material and posts no order; a light member posts one.
func TestMemberVisitUnderLoad(t *testing.T) {
	const maxLoad = 109020
	tests := []struct {
		name string
		// load is the member's load at its visit, in percent of maxLoad.
		load      int64
		wantOrder bool
	}{
		{"light", 5, true},
		{"loaded past BuyLoad", 63, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMemberBrain(t, wielded(1, dSword), lootStack(3, soulOre, 60))
			b.cfg.Travel.weights[dSoulshot] = 3
			b.world.Self.MaxLoad = maxLoad
			b.world.Self.Load = int32(maxLoad * tt.load / 100)
			board, now := b.cfg.CraftBoard, at(time.Minute)

			b.townArrival()
			req := b.persona.Warehouse.Want(b.world, b.wh.status, now)
			var ore int32
			for _, r := range req.Deposit {
				if r.TemplateID == soulOre {
					ore += r.Count
				}
			}
			if ore == 0 {
				t.Fatalf("visit = %+v, want the Soul Ore deposited", req)
			}

			end := now.Add(10 * time.Second)
			rows := make([]WarehouseRow, 0, len(req.Deposit))
			for _, r := range req.Deposit {
				r.Object = 3
				rows = append(rows, r)
			}
			b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseDone, At: end, Rows: rows}
			if _, ok := b.ruleMember(end); ok {
				t.Fatal("ruleMember took the turn")
			}
			var posted []CraftOrder
			for _, o := range board.Orders(memberClan, end) {
				if o.Member == memberName {
					posted = append(posted, o)
				}
			}
			if got := len(posted) > 0; got != tt.wantOrder {
				t.Errorf("posted %+v, want an order %v", posted, tt.wantOrder)
			}
		})
	}
}
