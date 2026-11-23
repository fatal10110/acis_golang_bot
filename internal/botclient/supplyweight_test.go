package botclient

import (
	"testing"
	"time"
)

// TestWeightDefaultsUnderHalfSpeed checks the weight defaults against the
// server's bands (character_weight_penalty.go): supplies stop under the
// first at 50 %, and the trip home, the pickup cap and the junk drop come
// under the half-speed band at 66.6 %, the trip above the supplies.
func TestWeightDefaultsUnderHalfSpeed(t *testing.T) {
	s, l := defaultSupplyPolicy(), defaultLootPolicy()
	under := func(pct, band1000 int) bool { return pct > 0 && pct*1000 < band1000*100 }
	for _, c := range []struct {
		name string
		pct  int
		band int
	}{
		{"Supply.BuyLoad", s.BuyLoad, 500},
		{"Supply.ReturnLoad", s.ReturnLoad, 666},
		{"Loot.StopAbove", l.StopAbove, 666},
		{"Loot.DropAbove", l.DropAbove, 666},
	} {
		if !under(c.pct, c.band) {
			t.Errorf("%s = %d %%, want under %d.%d %%", c.name, c.pct, c.band/10, c.band%10)
		}
	}
	if s.ReturnLoad <= s.BuyLoad {
		t.Errorf("Supply.ReturnLoad %d %% not above BuyLoad %d %%", s.ReturnLoad, s.BuyLoad)
	}
}

// TestSupplyCauseHalfSpeed checks the load trigger under the half-speed
// band: a farmer at 67 % with ShedMin of junk to sell goes home, one at
// 67 % with only kept items does not, and the trip starts past 60 %.
func TestSupplyCauseHalfSpeed(t *testing.T) {
	tests := []struct {
		name string
		item Item
		load int32
		want tripCause
	}{
		{name: "67 % with junk to sell", item: lootStack(60, junkMat, 1000), load: 67000, want: causeLoad},
		{name: "67 % with nothing to shed", item: lootStack(60, keptMat, 5000), load: 67000},
		{name: "at ReturnLoad", item: lootStack(60, junkMat, 1000), load: 60000},
		{name: "past ReturnLoad", item: lootStack(60, junkMat, 1000), load: 60001, want: causeLoad},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, potionStack(20), tt.item)
			b.persona.Shop.Keep = []int32{keptMat}
			b.world.Self.Load = tt.load
			if got := b.supplyCause(); got != tt.want {
				t.Fatalf("supplyCause = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestLootStopAbove checks the pickup cap: a farmer whose load the trip
// cannot shed gives up a drop that would bring it to Loot.StopAbove (65 %),
// and stops waiting on it, but still picks up what weighs nothing; under
// the cap, or with no cap, it picks up. With no item weights known, the
// load alone decides.
func TestLootStopAbove(t *testing.T) {
	kept := GroundItem{ObjectID: junkDrop, TemplateID: keptMat, Pos: Point{X: 900}, Stackable: true, Count: 5}
	adena := GroundItem{ObjectID: adenaDrop, TemplateID: 57, Pos: Point{X: 900}, Stackable: true, Count: 120}
	tests := []struct {
		name string
		item GroundItem
		load int32
		edit func(b *Brain)
		want bool
	}{
		{name: "67 % with nothing to shed", item: kept, load: 67000},
		{name: "under the cap", item: kept, load: 60000, want: true},
		{name: "the drop would reach the cap", item: kept, load: 64950},
		{name: "the drop stays under the cap", item: kept, load: 64949, want: true},
		{name: "adena past the cap", item: adena, load: 67000, want: true},
		{name: "StopAbove 0", item: kept, load: 99000, edit: func(b *Brain) { b.persona.Loot.StopAbove = 0 }, want: true},
		{name: "no weights, under the cap", item: kept, load: 64999, edit: func(b *Brain) { b.cfg.Travel.weights = nil }, want: true},
		{name: "no weights, at the cap", item: adena, load: 65000, edit: func(b *Brain) { b.cfg.Travel.weights = nil }},
	}
	now := at(time.Second)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, potionStack(20))
			b.persona.Loot = defaultLootPolicy()
			b.persona.Shop.Keep = []int32{keptMat}
			b.world.Self.Load = tt.load
			if tt.edit != nil {
				tt.edit(b)
			}
			b.world.Ground = map[int32]GroundItem{tt.item.ObjectID: tt.item}
			b.loot.items = []int32{tt.item.ObjectID}
			if c := b.supplyCause(); c != causeNone {
				t.Fatalf("supplyCause = %d, want none", c)
			}
			a, ok := b.ruleLoot(now)
			if got := ok && a.kind == actPickup && a.object == tt.item.ObjectID; got != tt.want {
				t.Fatalf("ruleLoot = %v kind %d object %d, want pickup %v", ok, a.kind, a.object, tt.want)
			}
			if q := b.lootQueued(now); q != tt.want {
				t.Fatalf("lootQueued = %v, want %v", q, tt.want)
			}
		})
	}
}
