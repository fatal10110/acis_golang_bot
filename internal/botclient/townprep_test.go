package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// newPrepBrain is newTripBrain back in town holding no spot, carrying
// items, with grocer 30004 (Lesser Healing Potions at 103, soulshots at 8)
// spawned at merchantAt.
func newPrepBrain(t *testing.T, merchantAt Point, items ...Item) *Brain {
	t.Helper()
	b := newTripBrain(t, items...)
	b.leaveSpot()
	shops, cheapest := indexShops([]datapack.BuyList{
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: lesserHealingPotion, Price: 103}, {ItemID: 1835, Price: 8}}},
	})
	tm := b.cfg.Travel
	tm.shops, tm.cheapest = shops, cheapest
	tm.spawns = map[int32][]Point{30004: {merchantAt}}
	tm.spawnedSold = spawnedSold(shops, tm.spawns)
	return b
}

// TestTownPrepGate checks the town preparation gate (docs/items.md section
// 7): armed at an arrival, a respawn or a login, a farmer claims no spot
// while a supply of its own is short of its trip target and a merchant
// within reach sells it at a price its budget pays, until a shop round
// ends. A broke bot, one short only of what no merchant in reach sells, or
// only of its Extra supplies leaves at once.
func TestTownPrepGate(t *testing.T) {
	near, far := Point{X: 5000}, Point{X: 50000}
	short := []Item{adenaStack(10000), potionStack(2)}
	tests := []struct {
		name     string
		at       Point
		items    []Item
		arm      func(b *Brain, now time.Time)
		edit     func(b *Brain)
		wantHeld bool
	}{
		{name: "short, sold in reach, paid", at: near, items: short, wantHeld: false},
		{name: "targets met", at: near, items: []Item{adenaStack(10000), potionStack(50)}, wantHeld: true},
		{name: "broke", at: near, items: []Item{potionStack(2)}, wantHeld: true},
		{name: "short of what no merchant in reach sells", at: far, items: short, wantHeld: true},
		{name: "a shop round since armed", at: near, items: short, edit: func(b *Brain) { b.shop.rounds++ }, wantHeld: true},
		{
			name: "short only of an Extra supply", at: near, items: []Item{adenaStack(10000), potionStack(50)},
			edit: func(b *Brain) {
				b.persona.Supply.Extra = func(*World) map[int32]int32 { return map[int32]int32{1835: 100} }
			},
			wantHeld: true,
		},
		{name: "not armed", at: near, items: short, arm: func(*Brain, time.Time) {}, wantHeld: true},
		{
			name: "respawn", at: near, items: short, wantHeld: false,
			arm: func(b *Brain, now time.Time) {
				if err := b.handle([]Event{Revive{ObjectID: selfID}}, now); err != nil {
					t.Fatal(err)
				}
			},
		},
		{name: "login", at: near, items: short, arm: func(b *Brain, now time.Time) { b.start(now) }, wantHeld: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPrepBrain(t, tt.at, tt.items...)
			now := at(time.Minute)
			b.shop.nextAt = now.Add(time.Hour)
			if tt.arm == nil {
				b.armPrep()
			} else {
				tt.arm(b, now)
			}
			if tt.edit != nil {
				tt.edit(b)
			}
			b.ruleSpot(now)
			if b.spot.held != tt.wantHeld {
				t.Fatalf("spot held %v, want %v", b.spot.held, tt.wantHeld)
			}
			if !tt.wantHeld && b.cfg.Spots.Held(0) != 0 {
				t.Fatalf("registry holds %d, want none", b.cfg.Spots.Held(0))
			}
		})
	}
}

// TestTownPrepShop checks that the shop errand tops a short supply up to
// its trip target while the gate holds, starting at once after the arming
// reset, and that the gate opens once the visit is over.
func TestTownPrepShop(t *testing.T) {
	b := newPrepBrain(t, Point{X: 5000}, adenaStack(10000), potionStack(5))
	var target int32
	for _, s := range b.supplies() {
		if s.item == lesserHealingPotion {
			target = s.count
		}
	}
	if len(b.supplyWants()) != 0 {
		t.Fatalf("supplyWants unarmed = %v, want none at %d of %d (over Refill)", b.supplyWants(), 5, target)
	}
	now := at(time.Minute)
	b.shop.nextAt = now.Add(shopRetry)
	b.armPrep()
	want := []buyRow{{Item: lesserHealingPotion, Count: target - 5}}
	if got := b.supplyWants(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("supplyWants preparing = %v, want %v", got, want)
	}
	if rule, a := b.decide(now); rule != "shop" || a.kind != actMove {
		t.Fatalf("decide = rule %q action %d, want a shop walk", rule, a.kind)
	}
	b.shop.end(now.Add(time.Minute), false)
	if b.preparing() {
		t.Fatal("preparing after the visit, want the gate open")
	}
	if len(b.supplyWants()) != 0 {
		t.Fatalf("supplyWants after the gate = %v, want none over Refill", b.supplyWants())
	}
}

// TestTownArrival checks a supply trip's arrival in town, by the Scroll of
// Escape's teleport or on foot: it resets the shop errand's next visit,
// arms the gate and calls the trip in town hook once. A walk that finds no
// town is no arrival, nor a teleport with no trip.
func TestTownArrival(t *testing.T) {
	later := at(time.Hour)
	tests := []struct {
		name string
		edit func(b *Brain)
		run  func(t *testing.T, b *Brain, now time.Time)
		want int
		// armed is the gate after the arrival's turn: on foot, the spot
		// rule of that turn opened it again, since no merchant on this
		// map sells what is short.
		armed bool
	}{
		{
			name: "scroll landing",
			run: func(t *testing.T, b *Brain, now time.Time) {
				b.supplyTrip.since, b.supplyTrip.escapeAt = now.Add(-time.Minute), now.Add(-20*time.Second)
				if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: town}}, now); err != nil {
					t.Fatal(err)
				}
			},
			want: 1, armed: true,
		},
		{
			name: "walk",
			run: func(t *testing.T, b *Brain, now time.Time) {
				b.supplyTrip.potions = true
				b.decide(now)
				b.world.Self.move = stopAt(Point{X: town.X - 500})
				b.decide(now.Add(time.Second))
			},
			want: 1,
		},
		{
			name: "walk with no town",
			edit: func(b *Brain) { b.cfg.Travel.gatekeepers = nil },
			run: func(t *testing.T, b *Brain, now time.Time) {
				b.supplyTrip.potions = true
				b.decide(now)
			},
		},
		{
			name: "teleport of another cause during the trip",
			run: func(t *testing.T, b *Brain, now time.Time) {
				b.supplyTrip.since = now.Add(-time.Minute)
				if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: town}}, now); err != nil {
					t.Fatal(err)
				}
				if b.supplyTrip.active() {
					t.Fatal("trip active after the teleport, want it ended")
				}
			},
		},
		{
			name: "teleport with no trip",
			run: func(t *testing.T, b *Brain, now time.Time) {
				if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: town}}, now); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t)
			hooked := 0
			b.persona.Supply.InTown = func(*World) { hooked++ }
			if tt.edit != nil {
				tt.edit(b)
			}
			now := at(time.Minute)
			b.shop.nextAt = later
			tt.run(t, b, now)
			if arrived := tt.want > 0; b.shop.nextAt.IsZero() != arrived || b.prep.armed != tt.armed {
				t.Fatalf("nextAt %v armed %v, want reset %v and armed %v", b.shop.nextAt, b.prep.armed, arrived, tt.armed)
			}
			// A later turn in town calls the hook no more.
			b.decide(now.Add(2 * time.Second))
			if hooked != tt.want {
				t.Fatalf("hook called %d times, want %d", hooked, tt.want)
			}
		})
	}
}

// TestSupplyCauseHandOff checks that the weight the trip in town hook will
// hand off sheds toward ReturnLoad: a heavy bot carrying nothing to sell
// but its hand-off starts a trip once that is ShedMin of the limit, under
// the half-speed band (TestSupplyCauseSlowBand has the band itself).
func TestSupplyCauseHandOff(t *testing.T) {
	tests := []struct {
		name    string
		handOff func(*World) int64
		want    tripCause
	}{
		{name: "no hook", want: causeNone},
		{name: "under ShedMin", handOff: func(*World) int64 { return 9999 }, want: causeNone},
		{name: "ShedMin to hand off", handOff: func(*World) int64 { return 10000 }, want: causeLoad},
		{name: "negative", handOff: func(*World) int64 { return -50000 }, want: causeNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, potionStack(20))
			b.world.Self.Load = 66500
			b.persona.Supply.HandOff = tt.handOff
			if got := b.supplyDue(at(time.Minute)); got != tt.want {
				t.Fatalf("supplyDue = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestTownPrepGoToSpot checks that an order to a spot disarms the town
// preparation gate, so the bot at its spot buys no supply above Refill
// percent of its target.
func TestTownPrepGoToSpot(t *testing.T) {
	b := newPrepBrain(t, Point{X: 5000}, adenaStack(10000), potionStack(5))
	b.armPrep()
	if !b.refills(5, 50) {
		t.Fatal("refills armed = false, want true under the target")
	}
	if err := b.goToSpot("s", at(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if b.prep.armed {
		t.Fatal("gate armed after goToSpot, want disarmed")
	}
	if b.refills(40, 50) {
		t.Fatal("refills after goToSpot at 80% = true, want false over Refill")
	}
}
