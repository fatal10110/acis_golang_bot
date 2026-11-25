package botclient

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

// TestLootGroundPrune checks that the ground record (groundSeen) forgets an
// item as World.Ground does (#495): when it leaves sight, and all of them at
// the bot's own teleport with a loading screen, also for a finders keepers
// member, whose record no groundScan prunes. Whatever World.Ground keeps the
// record keeps too.
func TestLootGroundPrune(t *testing.T) {
	const other = junkDrop + 1
	otherDrop := strayDrop
	otherDrop.ObjectID = other
	seen := []timed{{0, SpawnItem{Item: strayDrop}}, {0, DropItem{DropperID: pcID, Item: otherDrop}}}
	member := func(b *Brain) {
		b.world.Party = Party{LeaderID: pcID, Loot: partyLootFindersKeepers, Members: map[int32]*PartyMate{pcID: {}}}
	}
	tests := []struct {
		name  string
		setup func(b *Brain)
		ev    Event
		want  []int32 // the items left in the record
	}{
		{name: "an item leaving sight leaves the record", ev: DeleteObject{ObjectID: junkDrop}, want: []int32{other}},
		{name: "a member leaves it out of the record", setup: member, ev: DeleteObject{ObjectID: junkDrop}, want: []int32{other}},
		{name: "another object leaving sight keeps it", ev: DeleteObject{ObjectID: npcID}, want: []int32{junkDrop, other}},
		{name: "the bot's teleport clears the record", ev: TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5000}}, want: nil},
		{name: "a member's teleport clears the record", setup: member, ev: TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5000}}, want: nil},
		{name: "a fast teleport keeps it", ev: TeleportToLocation{ObjectID: selfID, Pos: Point{X: 10}, Fast: true}, want: []int32{junkDrop, other}},
		{name: "another's teleport keeps it", ev: TeleportToLocation{ObjectID: pcID, Pos: Point{X: 5000}}, want: []int32{junkDrop, other}},
		{name: "the bot's death keeps it", ev: Die{ObjectID: selfID}, want: []int32{junkDrop, other}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := lootPersona()
			p.Loot.Ground = GroundAny
			b, _ := newTestBrain(t, p)
			if tt.setup != nil {
				tt.setup(b)
			}
			handleAll(t, b, append(seen, timed{time.Second, tt.ev}))
			if len(b.loot.ground) != len(tt.want) {
				t.Fatalf("record = %v, want %v", b.loot.ground, tt.want)
			}
			for _, id := range tt.want {
				if _, ok := b.loot.ground[id]; !ok {
					t.Fatalf("record = %v, want %v", b.loot.ground, tt.want)
				}
				if _, ok := b.world.Ground[id]; !ok {
					t.Fatalf("world ground lost %d", id)
				}
			}
			if len(b.world.Ground) != len(tt.want) {
				t.Fatalf("world ground = %v, want %v", b.world.Ground, tt.want)
			}
		})
	}
}

// BenchmarkGroundSeen sights ground items with n of them in sight and in
// the record, as a burst of SpawnItem does around a crowded spot (#495):
// the cost of a sighting must not grow with n.
func BenchmarkGroundSeen(bm *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		bm.Run(fmt.Sprint(n), func(bm *testing.B) {
			w, err := NewWorld(Self{ObjectID: selfID, Name: "bot7"}, t0)
			if err != nil {
				bm.Fatal(err)
			}
			p := lootPersona()
			p.Loot.Ground = GroundAny
			b := NewBrain(w, NewEvents(64), &recordSender{}, newFakeClock(t0), BrainConfig{Persona: p, Rand: rand.New(rand.NewPCG(1, 2))})
			evs := make([]SpawnItem, n)
			for i := range evs {
				it := strayDrop
				it.ObjectID = int32(1_000_000 + i)
				evs[i] = SpawnItem{Item: it}
				b.world.Apply(evs[i], t0)
				b.lootEvent(evs[i], t0)
			}
			bm.ReportAllocs()
			for i := 0; bm.Loop(); i++ {
				b.lootEvent(evs[i%n], t0)
			}
		})
	}
}
