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

// TestLootGroundFitSupplies checks the reserve on GroundFit's own path
// (#573): a ground sword that beats the worn one (groundTakes, beats) is
// left while it would take the room the two Scrolls of Escape the bot
// lacks need under Supply.BuyLoad, taken while the room is there; a
// persona with no Loot.StopAbove sets no loot weight cap, the reserve
// among them.
func TestLootGroundFitSupplies(t *testing.T) {
	const swordWeight, short = 1500, 2 * 120
	sword := Item{ObjectID: 201, TemplateID: shortSword, Equipped: true, BodySlot: partRHand}
	for _, tt := range []struct {
		name      string
		load      int32
		stopAbove int
		want      bool
	}{
		{name: "room for the sword and the scrolls", load: 45000 - swordWeight - short, stopAbove: 65, want: true},
		{name: "the sword takes the scrolls' room", load: 45000 - swordWeight - short + 1, stopAbove: 65},
		{name: "no weight cap", load: 60000, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newGearBrain(t, 0, sword)
			b.persona.Loot = defaultLootPolicy()
			b.persona.Loot.StopAbove = tt.stopAbove
			b.persona.Supply = defaultSupplyPolicy()
			b.cfg.Travel.weights = map[int32]int32{scrollOfEscape: 120, longSword: swordWeight, shortSword: 1000}
			b.world.Self.MaxLoad = 100000
			b.world.Self.Load = tt.load
			if got, ok := b.supplyShort(); !ok || got != short {
				t.Fatalf("supplyShort = %d, %v, want %d, true", got, ok, short)
			}
			item := GroundItem{ObjectID: junkDrop, TemplateID: longSword, Pos: b.world.Self.Position(t0), Count: 1}
			b.world.Ground = map[int32]GroundItem{item.ObjectID: item}
			b.lootEvent(DropItem{DropperID: npcID, Item: item}, t0)
			if got := b.nextLoot(t0.Add(lootGrace)) == junkDrop; got != tt.want {
				t.Fatalf("nextLoot takes the sword = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLootGroundSuppliesRecheck checks that a ground item the reserve left
// goes back to the ground record, not out of the bot's sight for good: once
// the supplies it lacked are carried, a later scan takes it.
func TestLootGroundSuppliesRecheck(t *testing.T) {
	b := newTripBrain(t, potionStack(20))
	b.persona.Loot = defaultLootPolicy()
	b.persona.Loot.Ground = GroundAny
	b.world.Self.Load = 50000
	item := GroundItem{ObjectID: junkDrop, TemplateID: junkMat, Pos: Point{X: 900}, Stackable: true, Count: 100}
	b.world.Ground = map[int32]GroundItem{item.ObjectID: item}
	b.lootEvent(DropItem{DropperID: npcID, Item: item}, t0)
	now := t0.Add(lootGrace)
	if got := b.nextLoot(now); got != 0 {
		t.Fatalf("nextLoot = %d while the scrolls lack the room, want 0", got)
	}
	b.world.Apply(ItemList{Items: []Item{potionStack(20), soeStack(2)}}, now)
	if got := b.nextLoot(now.Add(lootGrace - time.Millisecond)); got != 0 {
		t.Fatalf("nextLoot = %d before the item's grace, want 0", got)
	}
	if got := b.nextLoot(now.Add(lootGrace)); got != junkDrop {
		t.Fatalf("nextLoot = %d once the scrolls are carried, want the item %d", got, junkDrop)
	}
}
