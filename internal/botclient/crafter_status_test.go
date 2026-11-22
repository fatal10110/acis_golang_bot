package botclient

import (
	"reflect"
	"testing"
	"time"
)

// TestBrainCrafterStatus checks the crafter's part of the snapshot: the
// book and Create Item, the board's open orders and the bot's claims, the
// phase and the plan's chains, and the last cycle's end, which survives the
// cycle's reset, with the breaker's hold while it lasts.
func TestBrainCrafterStatus(t *testing.T) {
	const checkEvery, returnEvery, hold = 5 * time.Minute, 30 * time.Minute, 30 * time.Minute
	end := at(10 * time.Second)
	tests := []struct {
		name  string
		setup func(t *testing.T) *Brain
		at    time.Time
		want  *CrafterStatus
	}{
		{
			name: "no crafter's cycle",
			setup: func(t *testing.T) *Brain {
				b := newCrafterBrain(t)
				b.cfg.CraftBoard = nil
				return b
			},
			at: end,
		},
		{
			name: "idle before any cycle, an order of another crafter open",
			setup: func(t *testing.T) *Brain {
				b := newCrafterBrain(t)
				b.craft.nextCheck = at(time.Hour)
				b.world.Recipes.CommonKnown = false
				board := b.cfg.CraftBoard
				id, _ := board.PostCommand(77, OrderByGM, "Gm", steel, 1)
				board.Claim(77, id, selfID+1, t0)
				return b
			},
			at:   end,
			want: &CrafterStatus{CreateItem: 1, Recipes: 1, Orders: 1, Phase: "idle", NextCheck: at(time.Hour)},
		},
		{
			name: "making its plan, its own order claimed",
			setup: func(t *testing.T) *Brain {
				b := makingBrain(t)
				board := b.cfg.CraftBoard
				id, _ := board.PostCommand(77, OrderByGM, "Gm", steel, 1)
				board.Claim(77, id, selfID, t0)
				board.Post(77, "Member", steel, 2)
				return b
			},
			at:   end,
			want: &CrafterStatus{CreateItem: 1, Recipes: 1, BookKnown: true, Orders: 2, Claimed: 1, Phase: "make", Chains: 1},
		},
		{
			name: "crafted",
			setup: func(t *testing.T) *Brain {
				b := makingBrain(t)
				b.craft.crafted, b.craft.chain = true, 1
				b.endCraftCycle(end, true)
				return b
			},
			at: end,
			want: &CrafterStatus{
				CreateItem: 1, Recipes: 1, BookKnown: true, Phase: "idle", NextCheck: end.Add(checkEvery),
				Last: CraftCycleEnd{At: end, Outcome: CraftCycleCrafted},
			},
		},
		{
			name: "nothing to craft",
			setup: func(t *testing.T) *Brain {
				b := newCrafterBrain(t)
				b.endCraftCycle(end, true)
				return b
			},
			at: end,
			want: &CrafterStatus{
				CreateItem: 1, Recipes: 1, BookKnown: true, Phase: "idle", NextCheck: end.Add(returnEvery),
				Last: CraftCycleEnd{At: end, Outcome: CraftCycleNothing},
			},
		},
		{
			name: "aborted",
			setup: func(t *testing.T) *Brain {
				b := makingBrain(t)
				b.endCraftCycle(end, false)
				return b
			},
			at: end,
			want: &CrafterStatus{
				CreateItem: 1, Recipes: 1, BookKnown: true, Phase: "idle", NextCheck: end.Add(checkEvery),
				Last: CraftCycleEnd{At: end, Outcome: CraftCycleAborted},
			},
		},
		{
			name: "breaker tripped: held",
			setup: func(t *testing.T) *Brain {
				b := makingBrain(t)
				b.tripCrafter(end)
				return b
			},
			at: end.Add(hold - time.Second),
			want: &CrafterStatus{
				CreateItem: 1, Recipes: 1, BookKnown: true, Phase: "idle", NextCheck: end.Add(hold),
				Last: CraftCycleEnd{At: end, Outcome: CraftCycleTripped}, BreakerUntil: end.Add(hold),
			},
		},
		{
			name: "breaker tripped: hold over",
			setup: func(t *testing.T) *Brain {
				b := makingBrain(t)
				b.tripCrafter(end)
				return b
			},
			at: end.Add(hold),
			want: &CrafterStatus{
				CreateItem: 1, Recipes: 1, BookKnown: true, Phase: "idle", NextCheck: end.Add(hold),
				Last: CraftCycleEnd{At: end, Outcome: CraftCycleTripped},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.setup(t)
			linkBrain(b, standing{})
			b.publishStatus(tt.at, true)
			if got := b.Status().Crafter; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("crafter status = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestBrainCrafterStatusFresh checks that every snapshot carries its own
// CrafterStatus: a published one never changes.
func TestBrainCrafterStatusFresh(t *testing.T) {
	b := makingBrain(t)
	linkBrain(b, standing{})
	b.publishStatus(at(time.Second), true)
	first := b.Status().Crafter
	b.endCraftCycle(at(2*time.Second), false)
	b.publishStatus(at(2*time.Second), true)
	if first.Phase != "make" || first.Last.Outcome != CraftCycleNone {
		t.Fatalf("published status changed to %+v", first)
	}
	if got := b.Status().Crafter; got == first || got.Last.Outcome != CraftCycleAborted {
		t.Fatalf("next status = %+v, want a fresh one, aborted", got)
	}
}
