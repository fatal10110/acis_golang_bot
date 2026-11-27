package botclient

import (
	"testing"
	"time"
)

// TestLootGroundSupplies checks that a ground item the bot did not fight
// for is left while picking it up would take the weight the supplies it
// lacks need under Supply.BuyLoad (#573): two Scrolls of Escape short weigh
// 240 of the 45000 BuyLoad leaves, the 100 stems 1000. An item it keeps,
// the drop of a creature it fought, and a bot short of no supply take it
// up to Loot.StopAbove as before.
func TestLootGroundSupplies(t *testing.T) {
	for _, tt := range []struct {
		name  string
		items []Item
		load  int32
		id    int32
		own   bool
		want  bool
	}{
		{name: "room for the item and the scrolls", items: []Item{potionStack(20)}, load: 43760, id: junkMat, want: true},
		{name: "the item takes the scrolls' room", items: []Item{potionStack(20)}, load: 43761, id: junkMat},
		{name: "one scroll short", items: []Item{potionStack(20), soeStack(1)}, load: 43880, id: junkMat, want: true},
		{name: "no supply short", items: []Item{potionStack(20), soeStack(2)}, load: 60000, id: junkMat, want: true},
		{name: "an item it keeps", items: []Item{potionStack(20)}, load: 60000, id: keptMat, want: true},
		{name: "its own kill's drop", items: []Item{potionStack(20)}, load: 60000, id: junkMat, own: true, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, tt.items...)
			b.persona.Loot = defaultLootPolicy()
			b.persona.Loot.Ground = GroundAny
			b.persona.Shop.Keep = []int32{keptMat}
			b.world.Self.Load = tt.load
			item := GroundItem{ObjectID: junkDrop, TemplateID: tt.id, Pos: Point{X: 900}, Stackable: true, Count: 100}
			b.world.Ground = map[int32]GroundItem{item.ObjectID: item}
			if tt.own {
				b.loot.fought = map[int32]time.Time{npcID: t0}
			}
			b.lootEvent(DropItem{DropperID: npcID, Item: item}, t0)
			got := b.nextLoot(t0.Add(lootGrace)) == junkDrop
			if got != tt.want {
				t.Fatalf("nextLoot takes the item = %v, want %v", got, tt.want)
			}
		})
	}
}
