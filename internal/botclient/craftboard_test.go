package botclient

import (
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
)

const (
	boardClan  = int32(0x0c1a0001)
	boardOther = int32(0x0c1a0002)
	boardBotA  = int32(0x10000301)
	boardBotB  = int32(0x10000302)
)

// TestCraftBoardLease: one bot of a clan holds the warehouse lease at a
// time; it lapses after craftLeaseTTL on the callers' clock, the holder
// renews it, and each clan has its own.
func TestCraftBoardLease(t *testing.T) {
	type step struct {
		op   string // take, holds, release
		clan int32
		bot  int32
		at   time.Duration
		want bool
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"exclusion", []step{
			{"take", boardClan, boardBotA, 0, true},
			{"take", boardClan, boardBotB, time.Second, false},
			{"holds", boardClan, boardBotA, time.Second, true},
			{"holds", boardClan, boardBotB, time.Second, false},
		}},
		{"other clan is its own", []step{
			{"take", boardClan, boardBotA, 0, true},
			{"take", boardOther, boardBotB, 0, true},
		}},
		{"expiry", []step{
			{"take", boardClan, boardBotA, 0, true},
			{"take", boardClan, boardBotB, craftLeaseTTL - time.Nanosecond, false},
			{"holds", boardClan, boardBotA, craftLeaseTTL, false},
			{"take", boardClan, boardBotB, craftLeaseTTL, true},
			{"holds", boardClan, boardBotB, craftLeaseTTL, true},
		}},
		{"holder renews", []step{
			{"take", boardClan, boardBotA, 0, true},
			{"take", boardClan, boardBotA, time.Minute, true},
			{"take", boardClan, boardBotB, craftLeaseTTL, false},
			{"holds", boardClan, boardBotA, time.Minute + craftLeaseTTL - time.Nanosecond, true},
		}},
		{"release", []step{
			{"take", boardClan, boardBotA, 0, true},
			{"release", boardClan, boardBotB, 0, false},
			{"take", boardClan, boardBotB, 0, false},
			{"release", boardClan, boardBotA, 0, false},
			{"holds", boardClan, boardBotA, 0, false},
			{"take", boardClan, boardBotB, 0, true},
		}},
		{"no clan holds nothing", []step{
			{"take", 0, boardBotA, 0, false},
			{"holds", 0, boardBotA, 0, false},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewCraftBoard()
			for i, s := range tt.steps {
				now := t0.Add(s.at)
				var got bool
				switch s.op {
				case "take":
					got = b.TakeLease(s.clan, s.bot, now)
				case "holds":
					got = b.HoldsLease(s.clan, s.bot, now)
				case "release":
					b.ReleaseLease(s.clan, s.bot)
				}
				if got != s.want {
					t.Fatalf("step %d %s clan %x bot %x at %v = %v, want %v", i, s.op, s.clan, s.bot, s.at, got, s.want)
				}
			}
		})
	}
}

// TestCraftBoardLeaseRace: of bots taking one clan's lease at once, one
// gets it.
func TestCraftBoardLeaseRace(t *testing.T) {
	b := NewCraftBoard()
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := range int32(16) {
		wg.Go(func() {
			if b.TakeLease(boardClan, boardBotA+i, t0) {
				mu.Lock()
				won++
				mu.Unlock()
			}
			b.Orders(boardClan, t0)
		})
	}
	wg.Wait()
	if won != 1 {
		t.Errorf("%d bots took the lease, want 1", won)
	}
}

// TestCraftBoardOrders walks an order through posted → materials deposited
// → claimed → crafted → delivered, with the transitions out of turn
// refused and a lapsed claim back to materials deposited.
func TestCraftBoardOrders(t *testing.T) {
	type step struct {
		op        string // in, claim, crafted, deliver
		bot       int32
		at        time.Duration
		want      bool
		wantState OrderState
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"whole path", []step{
			{"in", 0, 0, true, OrderMaterialsIn},
			{"claim", boardBotA, 0, true, OrderClaimed},
			{"crafted", boardBotA, time.Minute, true, OrderCrafted},
			{"deliver", boardBotA, time.Minute, true, OrderDelivered},
		}},
		{"out of turn", []step{
			{"claim", boardBotA, 0, false, OrderPosted},
			{"crafted", boardBotA, 0, false, OrderPosted},
			{"deliver", boardBotA, 0, false, OrderPosted},
			{"in", 0, 0, true, OrderMaterialsIn},
			{"in", 0, 0, false, OrderMaterialsIn},
			{"crafted", boardBotA, 0, false, OrderMaterialsIn},
			{"claim", boardBotA, 0, true, OrderClaimed},
			{"deliver", boardBotA, 0, false, OrderClaimed},
		}},
		{"one claim at a time", []step{
			{"in", 0, 0, true, OrderMaterialsIn},
			{"claim", boardBotA, 0, true, OrderClaimed},
			{"claim", boardBotB, time.Minute, false, OrderClaimed},
			{"crafted", boardBotB, time.Minute, false, OrderClaimed},
		}},
		{"claim lapses", []step{
			{"in", 0, 0, true, OrderMaterialsIn},
			{"claim", boardBotA, 0, true, OrderClaimed},
			{"crafted", boardBotA, craftClaimTTL, false, OrderMaterialsIn},
			{"claim", boardBotB, craftClaimTTL, true, OrderClaimed},
		}},
		{"holder renews", []step{
			{"in", 0, 0, true, OrderMaterialsIn},
			{"claim", boardBotA, 0, true, OrderClaimed},
			{"claim", boardBotA, 9 * time.Minute, true, OrderClaimed},
			{"claim", boardBotB, craftClaimTTL, false, OrderClaimed},
			{"crafted", boardBotA, 9*time.Minute + craftClaimTTL - time.Nanosecond, true, OrderCrafted},
		}},
		{"crafted keeps its crafter", []step{
			{"in", 0, 0, true, OrderMaterialsIn},
			{"claim", boardBotA, 0, true, OrderClaimed},
			{"crafted", boardBotA, 0, true, OrderCrafted},
			{"claim", boardBotB, 2 * craftClaimTTL, false, OrderCrafted},
			{"deliver", boardBotB, 2 * craftClaimTTL, false, OrderCrafted},
			{"deliver", boardBotA, 2 * craftClaimTTL, true, OrderDelivered},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewCraftBoard()
			id, ok := b.Post(boardClan, "Member", 1463, 1000)
			if !ok {
				t.Fatal("Post refused")
			}
			for i, s := range tt.steps {
				now := t0.Add(s.at)
				var got bool
				state := OrderState(255)
				switch s.op {
				case "in":
					got = b.MaterialsIn(boardClan, id)
				case "claim":
					got = b.Claim(boardClan, id, s.bot, now)
				case "crafted":
					got = b.Crafted(boardClan, id, s.bot, now)
				case "deliver":
					var o CraftOrder
					o, got = b.Deliver(boardClan, id, s.bot)
					if got {
						state = o.State
					}
				}
				if orders := b.Orders(boardClan, now); len(orders) == 1 {
					state = orders[0].State
				}
				if got != s.want || state != s.wantState {
					t.Fatalf("step %d %s bot %x at %v = %v, state %v; want %v, %v", i, s.op, s.bot, s.at, got, state, s.want, s.wantState)
				}
			}
		})
	}
}

// TestCraftBoardPost: an order needs a clan, a member and something to make;
// ids are unique across clans and Orders lists them in posting order.
func TestCraftBoardPost(t *testing.T) {
	b := NewCraftBoard()
	for _, bad := range []struct {
		clan    int32
		member  string
		product int32
		count   int64
	}{{0, "M", 1463, 1}, {boardClan, "", 1463, 1}, {boardClan, "M", 0, 1}, {boardClan, "M", 1463, 0}} {
		if _, ok := b.Post(bad.clan, bad.member, bad.product, bad.count); ok {
			t.Errorf("Post(%x, %q, %d, %d) accepted", bad.clan, bad.member, bad.product, bad.count)
		}
	}
	a, _ := b.Post(boardClan, "A", 1463, 10)
	o, _ := b.Post(boardOther, "O", 1463, 10)
	c, _ := b.Post(boardClan, "C", 1464, 20)
	if a == o || o == c || a == c {
		t.Errorf("ids %d %d %d, want unique", a, o, c)
	}
	got := b.Orders(boardClan, t0)
	want := []CraftOrder{
		{ID: a, Member: "A", Product: 1463, Count: 10},
		{ID: c, Member: "C", Product: 1464, Count: 20},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Orders = %+v, want %+v", got, want)
	}
}

// TestCraftBoardPlanOrders: the planner reads the orders the crafter holds
// a live claim on, in posting order.
func TestCraftBoardPlanOrders(t *testing.T) {
	b := NewCraftBoard()
	post := func(product int32, count int64, claimer int32) {
		id, _ := b.Post(boardClan, "M", product, count)
		b.MaterialsIn(boardClan, id)
		if claimer != 0 {
			b.Claim(boardClan, id, claimer, t0)
		}
	}
	post(1463, 100, boardBotA)
	post(1464, 200, boardBotB)
	post(1465, 300, 0)
	post(1466, 400, boardBotA)
	want := []craftplan.Order{{Product: 1463, Count: 100}, {Product: 1466, Count: 400}}
	if got := b.PlanOrders(boardClan, boardBotA, t0.Add(time.Minute)); !slices.Equal(got, want) {
		t.Errorf("PlanOrders = %+v, want %+v", got, want)
	}
	if got := b.PlanOrders(boardClan, boardBotA, t0.Add(craftClaimTTL)); len(got) != 0 {
		t.Errorf("PlanOrders after the claims lapsed = %+v, want none", got)
	}
}

// TestCraftBoardLedger: delivered products count as their member's, a
// member takes only its own, and the planner's reservations are every
// member's counts plus the other crafters' chain reservations.
func TestCraftBoardLedger(t *testing.T) {
	b := NewCraftBoard()
	deliver := func(member string, product int32, count int64) {
		id, _ := b.Post(boardClan, member, product, count)
		b.MaterialsIn(boardClan, id)
		b.Claim(boardClan, id, boardBotA, t0)
		b.Crafted(boardClan, id, boardBotA, t0)
		if _, ok := b.Deliver(boardClan, id, boardBotA); !ok {
			t.Fatalf("Deliver %s %d refused", member, product)
		}
	}
	deliver("Alice", 1463, 1000)
	deliver("Alice", 1463, 500)
	deliver("Bob", 1464, 300)
	b.Reserve(boardClan, boardBotA, 1890, 4)
	b.Reserve(boardClan, boardBotB, 1890, 6)
	b.Reserve(boardClan, boardBotB, 1463, 7)

	if got, want := b.Owned(boardClan, "Alice"), map[int32]int64{1463: 1500}; !maps.Equal(got, want) {
		t.Errorf("Owned(Alice) = %v, want %v", got, want)
	}
	if got := b.Take(boardClan, "Bob", 1463, 10); got != 0 {
		t.Errorf("Bob took %d of Alice's 1463, want 0", got)
	}
	if got := b.Take(boardClan, "Alice", 1463, 2000); got != 1500 {
		t.Errorf("Alice took %d, want her 1500", got)
	}
	if got := b.Owned(boardClan, "Alice"); len(got) != 0 {
		t.Errorf("Owned(Alice) after taking all = %v, want none", got)
	}

	tests := []struct {
		crafter int32
		want    map[int32]int64
	}{
		{boardBotA, map[int32]int64{1464: 300, 1890: 6, 1463: 7}},
		{boardBotB, map[int32]int64{1464: 300, 1890: 4}},
		{0, map[int32]int64{1464: 300, 1890: 10, 1463: 7}},
	}
	for _, tt := range tests {
		if got := b.Reserved(boardClan, tt.crafter); !maps.Equal(got, tt.want) {
			t.Errorf("Reserved(%x) = %v, want %v", tt.crafter, got, tt.want)
		}
	}
	b.Unreserve(boardClan, boardBotB, 1890, 100)
	b.Unreserve(boardClan, boardBotB, 1463, 2)
	if got, want := b.Reserved(boardClan, boardBotA), map[int32]int64{1464: 300, 1463: 5}; !maps.Equal(got, want) {
		t.Errorf("Reserved after Unreserve = %v, want %v", got, want)
	}
	if got := b.Reserved(boardOther, boardBotA); len(got) != 0 {
		t.Errorf("Reserved of another clan = %v, want none", got)
	}
}

// TestCraftBoardReleaseBot: a stopping bot gives back its lease, crafter
// entry, claims and chain reservations; the products of an order it
// crafted stay its to deliver, and another bot keeps what it holds.
func TestCraftBoardReleaseBot(t *testing.T) {
	b := NewCraftBoard()
	b.TakeLease(boardClan, boardBotA, t0)
	b.Publish(boardClan, CrafterEntry{Bot: boardBotA, Name: "A", Recipes: []int32{20}})
	b.Publish(boardClan, CrafterEntry{Bot: boardBotB, Name: "B"})
	b.Reserve(boardClan, boardBotA, 1890, 4)
	b.Reserve(boardClan, boardBotB, 1891, 2)
	claimed := func(bot int32) OrderID {
		id, _ := b.Post(boardClan, "M", 1463, 10)
		b.MaterialsIn(boardClan, id)
		b.Claim(boardClan, id, bot, t0)
		return id
	}
	mine, theirs, made := claimed(boardBotA), claimed(boardBotB), claimed(boardBotA)
	b.Crafted(boardClan, made, boardBotA, t0)

	b.ReleaseBot(boardBotA)

	if !b.TakeLease(boardClan, boardBotB, t0) {
		t.Error("lease still held after its bot stopped")
	}
	if got := b.Crafters(boardClan); len(got) != 1 || got[0].Bot != boardBotB {
		t.Errorf("crafters %+v, want only B", got)
	}
	if got, want := b.Reserved(boardClan, 0), map[int32]int64{1891: 2}; !maps.Equal(got, want) {
		t.Errorf("Reserved = %v, want %v", got, want)
	}
	states := map[OrderID]CraftOrder{}
	for _, o := range b.Orders(boardClan, t0) {
		states[o.ID] = o
	}
	if o := states[mine]; o.State != OrderMaterialsIn || o.Crafter != 0 {
		t.Errorf("released claim %+v, want materials deposited and no crafter", o)
	}
	if o := states[theirs]; o.State != OrderClaimed || o.Crafter != boardBotB {
		t.Errorf("other bot's claim %+v, want kept", o)
	}
	if o := states[made]; o.State != OrderCrafted || o.Crafter != boardBotA {
		t.Errorf("crafted order %+v, want kept with its crafter", o)
	}
}

// TestCraftBoardCopies: what the board hands out and takes in are copies.
func TestCraftBoardCopies(t *testing.T) {
	b := NewCraftBoard()
	recipes := []int32{20, 21}
	b.Publish(boardClan, CrafterEntry{Bot: boardBotA, Recipes: recipes, ShotGrades: []int{1}})
	recipes[0] = 99
	got := b.Crafters(boardClan)
	got[0].Recipes[1], got[0].ShotGrades[0] = 99, 5
	if again := b.Crafters(boardClan); !slices.Equal(again[0].Recipes, []int32{20, 21}) || !slices.Equal(again[0].ShotGrades, []int{1}) {
		t.Errorf("entry %+v changed through a caller's slice", again[0])
	}

	id, _ := b.Post(boardClan, "M", 1463, 10)
	b.Orders(boardClan, t0)[0].State = OrderCrafted
	if o := b.Orders(boardClan, t0)[0]; o.ID != id || o.State != OrderPosted {
		t.Errorf("order %+v changed through a returned copy", o)
	}

	b.Reserve(boardClan, boardBotA, 1890, 4)
	b.Reserved(boardClan, 0)[1890] = 100
	if got := b.Reserved(boardClan, 0)[1890]; got != 4 {
		t.Errorf("reserved %d after a caller's write, want 4", got)
	}
	b.MaterialsIn(boardClan, id)
	b.Claim(boardClan, id, boardBotA, t0)
	b.Crafted(boardClan, id, boardBotA, t0)
	b.Deliver(boardClan, id, boardBotA)
	b.Owned(boardClan, "M")[1463] = 100
	if got := b.Owned(boardClan, "M")[1463]; got != 10 {
		t.Errorf("owned %d after a caller's write, want 10", got)
	}
}

// TestCraftBoardRunReleases: a brain's session ending gives back what the
// bot holds on the board.
func TestCraftBoardRunReleases(t *testing.T) {
	b, _ := newTestBrain(t, Persona{})
	board := NewCraftBoard()
	b.cfg.CraftBoard = board
	board.TakeLease(boardClan, selfID, t0)
	done := make(chan struct{})
	close(done)
	if err := b.Run(done, nil); err != nil {
		t.Fatal(err)
	}
	if !board.TakeLease(boardClan, boardBotB, t0) {
		t.Error("lease still held after Run returned")
	}
}
