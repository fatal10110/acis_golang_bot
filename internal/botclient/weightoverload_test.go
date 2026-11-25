package botclient

import (
	"bytes"
	"slices"
	"testing"
	"time"
)

// The weight overload tests' items (#488): heavyMat is sellable loot
// heavier per piece than junkMat, trashMat is Loot.Junk, and unknownMat a
// template the loaded weight table lacks.
const (
	heavyMat   int32 = 1866
	trashMat   int32 = 1867
	unknownMat int32 = 1868
)

// newOverloadBrain is newTripBrain with heavyMat and trashMat weighed and
// trashMat in Loot.Junk, carrying items at load of a 100000 limit.
func newOverloadBrain(t *testing.T, load int32, items ...Item) *Brain {
	t.Helper()
	b := newTripBrain(t, items...)
	b.cfg.Travel.weights[heavyMat] = 50
	b.cfg.Travel.weights[trashMat] = 5
	b.persona.Loot = defaultLootPolicy()
	b.persona.Loot.Junk = []int32{trashMat}
	b.persona.Shop.Keep = []int32{keptMat}
	b.world.Self.Load = load
	return b
}

// TestRuleOverweight checks the overweight recovery's pick: from the
// immobile band a bot drops its junk first, then the heaviest stack it
// would sell, never a worn, kept or quest item, nor a stack it just asked
// to drop; under the immobile band it starts nothing, and with no item
// weights known it drops only junk.
func TestRuleOverweight(t *testing.T) {
	sell := lootStack(60, junkMat, 500)  // 5000
	heavy := lootStack(61, heavyMat, 50) // 2500 at 50 a piece: lighter stack
	trash := lootStack(62, trashMat, 10)
	kept := lootStack(63, keptMat, 9000)
	worn := Item{ObjectID: 64, TemplateID: heavyMat, Count: 1, Equipped: true}
	quest := Item{ObjectID: 65, TemplateID: heavyMat, Count: 900, SubCategory: subCategoryQuest}
	tests := []struct {
		name  string
		items []Item
		load  int32
		edit  func(b *Brain)
		want  int32 // the object dropped, 0 for none
		hold  bool  // the rule matches without an action
	}{
		{name: "117 %: junk first", items: []Item{sell, heavy, trash}, load: 117000, want: 62},
		{name: "117 %: the heaviest sold stack", items: []Item{heavy, sell, kept}, load: 117000, want: 60},
		{name: "100 % is immobile", items: []Item{sell}, load: 100000, want: 60},
		{name: "99 % is not", items: []Item{sell, trash}, load: 99999},
		{name: "kept, worn and quest items stay", items: []Item{kept, worn, quest}, load: 117000},
		{
			name: "a stack asked to drop waits dropRetry", items: []Item{sell, heavy}, load: 117000,
			edit: func(b *Brain) { b.loot.sent(dropAction(sell, Point{}), at(0)) }, want: 61,
		},
		{
			name: "no weights: junk only", items: []Item{sell, trash}, load: 117000,
			edit: func(b *Brain) { b.cfg.Travel = nil }, want: 62,
		},
		{
			name: "no weights, no junk", items: []Item{sell}, load: 117000,
			edit: func(b *Brain) { b.cfg.Travel = nil },
		},
		{
			name: "a lowered limit", items: []Item{sell}, load: 70000,
			edit: func(b *Brain) { b.world.Self.MaxLoad = 65000 }, want: 60,
		},
		{
			name: "unknown limit", items: []Item{sell}, load: 117000,
			edit: func(b *Brain) { b.world.Self.MaxLoad = 0 },
		},
		{
			name: "waits out a move", items: []Item{sell}, load: 117000, hold: true,
			edit: func(b *Brain) {
				b.world.Apply(MoveToLocation{ObjectID: b.world.Self.ObjectID, Dest: Point{X: 1000}}, at(time.Second))
			},
		},
	}
	now := at(time.Second)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newOverloadBrain(t, tt.load, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			a, ok := b.ruleOverweight(now)
			var got int32
			if ok && a.kind == actDrop {
				got = a.object
			}
			if got != tt.want || ok != (tt.want != 0 || tt.hold) {
				t.Fatalf("ruleOverweight = %v drop %d, want drop %d (hold %v)", ok, got, tt.want, tt.hold)
			}
		})
	}
}

// TestOverweightShedsAtOnce checks the recovery end to end through the
// decision table: a farmer at 117 % drops on its next decision, ahead of
// the supply trip and farming, one stack at a time as the load falls, past
// the immobile band down to under the half-speed band, and then stops.
func TestOverweightShedsAtOnce(t *testing.T) {
	stacks := []Item{lootStack(60, junkMat, 2000), lootStack(61, heavyMat, 300), lootStack(62, heavyMat, 200), lootStack(63, junkMat, 1000)}
	b := newOverloadBrain(t, 117000, append([]Item{potionStack(20)}, stacks...)...)
	weight := map[int32]int32{60: 20000, 61: 15000, 62: 10000, 63: 10000}
	now := at(time.Second)
	var dropped []int32
	for range 10 {
		rule, a := b.decide(now)
		if rule != "overweight" {
			break
		}
		if a.kind != actDrop {
			t.Fatalf("decide = %q %v, want a drop", rule, a.kind)
		}
		b.loot.sent(a, now)
		dropped = append(dropped, a.object)
		b.world.Self.Load -= weight[a.object]
		now = now.Add(dropGap)
	}
	// 117000 -> 97000 -> 82000 -> 72000 -> 62000: under 66.6 % after four.
	if want := []int32{60, 61, 62, 63}; !slices.Equal(dropped, want) {
		t.Fatalf("dropped %v, want %v", dropped, want)
	}
	if b.shedding {
		t.Fatal("still shedding under the half-speed band")
	}
	if rule, _ := b.decide(now); rule == "overweight" {
		t.Fatal("overweight matches under the half-speed band")
	}
}

// TestLootNeverPastLimit checks the pickup's hard cap: whatever
// Loot.StopAbove sets, no pickup takes the load to the weight limit; an
// item the loaded weight table lacks is never picked up, while adena and
// an item that weighs nothing are.
func TestLootNeverPastLimit(t *testing.T) {
	tests := []struct {
		name     string
		item     GroundItem
		load     int32
		stop     int
		noWeight bool
		want     bool
	}{
		{name: "StopAbove 0, under the limit", item: GroundItem{TemplateID: heavyMat, Count: 10}, load: 99000, want: true},
		{name: "StopAbove 0, a stack crossing 100 %", item: GroundItem{TemplateID: heavyMat, Count: 20}, load: 99000},
		{name: "StopAbove 120 still caps at 100 %", item: GroundItem{TemplateID: heavyMat, Count: 20}, load: 99000, stop: 120},
		{name: "unknown weight", item: GroundItem{TemplateID: unknownMat, Count: 1}, load: 1000, stop: 65},
		{name: "adena at 117 %", item: GroundItem{TemplateID: adenaID, Count: 100}, load: 117000, stop: 65, want: true},
		{name: "weightless at 117 %", item: GroundItem{TemplateID: trashMat, Count: 1}, load: 117000, stop: 65, want: true},
		{name: "no weights, at the limit", item: GroundItem{TemplateID: heavyMat, Count: 1}, load: 100000, noWeight: true},
		{name: "no weights, under it", item: GroundItem{TemplateID: heavyMat, Count: 1}, load: 99999, noWeight: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newOverloadBrain(t, tt.load)
			b.cfg.Travel.weights[trashMat] = 0
			b.persona.Loot.StopAbove = tt.stop
			if tt.noWeight {
				b.cfg.Travel.weights = nil
			}
			if got := b.lootFits(tt.item); got != tt.want {
				t.Fatalf("lootFits = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTradeReceivableWeight checks the trade inflow: an exempt partner's
// offer is refused when it would take the load past Trade.MaxLoad, or
// holds an item the loaded weight table lacks; adena always passes, and
// with no item weights known only the load carried counts.
func TestTradeReceivableWeight(t *testing.T) {
	tests := []struct {
		name     string
		offer    []TradeItem
		load     int32
		noWeight bool
		want     bool
	}{
		{name: "fits under 50 %", offer: []TradeItem{{TemplateID: heavyMat, Count: 100}}, load: 40000, want: true},
		{name: "would pass 50 %", offer: []TradeItem{{TemplateID: heavyMat, Count: 100}, {TemplateID: junkMat, Count: 600}}, load: 40000},
		{name: "a stack past the limit", offer: []TradeItem{{TemplateID: heavyMat, Count: 2000}}, load: 10000},
		{name: "unknown weight", offer: []TradeItem{{TemplateID: unknownMat, Count: 1}}, load: 1000},
		{name: "adena over 50 %", offer: []TradeItem{{TemplateID: adenaID, Count: 1000}}, load: 90000, want: true},
		{name: "no weights: the load alone", offer: []TradeItem{{TemplateID: heavyMat, Count: 2000}}, load: 10000, noWeight: true, want: true},
		{name: "no weights, over 50 %", offer: []TradeItem{{TemplateID: heavyMat, Count: 1}}, load: 51000, noWeight: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newOverloadBrain(t, tt.load)
			b.persona.Trade = defaultTradePolicy()
			b.world.Trade.Other = tt.offer
			if tt.noWeight {
				b.cfg.Travel.weights = nil
			}
			if got := b.tradeReceivable(true); got != tt.want {
				t.Fatalf("tradeReceivable = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOverweightDropsWhereTheServerHolds replays the live run's frames
// (TestLiveWeightOverload at fe41b6f): the bot starts a walk, a GM grant
// takes it to 117 % and the server's UserInfo sets its speeds to 0 where it
// stands, with no StopMove. The drop names that spot, inside the server's
// drop range (network/inventory.go dropInRange), not the walk's end.
func TestOverweightDropsWhereTheServerHolds(t *testing.T) {
	origin := Point{X: -84141, Y: 244623, Z: -3728}
	dest := Point{X: -84424, Y: 244072, Z: -3705}
	chains := lootStack(60, heavyMat, 2015)
	b := newOverloadBrain(t, 0, chains)
	self := b.world.Self.ObjectID
	info := func(run, walk, load int32) UserInfo {
		return UserInfo{ObjectID: self, Name: "bot7", RunSpeed: run, WalkSpeed: walk, Running: true, Pos: origin, Load: load, MaxLoad: 109020}
	}
	start := at(time.Second)
	b.world.Apply(info(126, 88, 26000), start)
	b.world.Apply(MoveToLocation{ObjectID: self, Origin: origin, Dest: dest}, start.Add(5*time.Millisecond))
	b.world.Apply(info(0, 0, 127561), start.Add(43*time.Millisecond))
	for _, d := range []time.Duration{140 * time.Millisecond, 10 * time.Second} {
		now := start.Add(d)
		if p := b.world.Self.Position(now); p != origin {
			t.Fatalf("at +%v the bot stands at %v, want %v", d, p, origin)
		}
		a, ok := b.ruleOverweight(now)
		if want := dropAction(chains, origin); !ok || a.kind != actDrop || !bytes.Equal(a.payload, want.payload) {
			t.Fatalf("at +%v ruleOverweight = %v %v, want the chains dropped at %v", d, ok, a.kind, origin)
		}
	}
}
