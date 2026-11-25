package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// The slot limit tests' items (#528): fillerItem is the template of the
// first of the non-stacking items a full bag holds, herbOfLife a herb.
const (
	fillerItem int32 = 3000
	herbOfLife int32 = 8600
	slotLimit  int32 = 80
)

// newSlotLimitBrain is a looting bot with inventory limit lim holding n
// items: adena and n-1 non-stacking items of distinct templates from
// fillerItem on, with the herb templates known and no weights.
func newSlotLimitBrain(t *testing.T, lim int32, n int) *Brain {
	t.Helper()
	b, _ := newTestBrain(t, lootPersona())
	b.cfg.Travel = &TravelMap{herbs: map[int32]bool{herbOfLife: true}}
	b.world.Self.InvLimit = lim
	items := []Item{{ObjectID: 0x50000000, TemplateID: adenaID, Count: 100}}
	for i := range int32(n - 1) {
		items = append(items, Item{ObjectID: 0x50000001 + i, TemplateID: fillerItem + i, Count: 1})
	}
	b.world.Apply(ItemList{Items: items}, t0)
	return b
}

// TestLootSlotLimit checks the slot check of every pickup (slotsFit): at
// the inventory limit the bot leaves an item needing a slot, a
// non-stacking one or a stack of a template it does not hold, and takes
// adena, a herb and a stack of a template it holds; under the limit, with
// the limit unknown, or in a party whose loot the server hands out without
// checking the picker's room, it takes them all.
func TestLootSlotLimit(t *testing.T) {
	sword := GroundItem{ObjectID: junkDrop, TemplateID: 2000, Count: 1}
	newStack := GroundItem{ObjectID: junkDrop, TemplateID: 2001, Stackable: true, Count: 5}
	heldStack := GroundItem{ObjectID: junkDrop, TemplateID: fillerItem, Stackable: true, Count: 5}
	adena := GroundItem{ObjectID: adenaDrop, TemplateID: adenaID, Stackable: true, Count: 120}
	herb := GroundItem{ObjectID: junkDrop, TemplateID: herbOfLife, Count: 1}
	party := func(loot int32) func(*Brain) {
		return func(b *Brain) {
			b.world.Party = Party{LeaderID: selfID, Loot: loot, Members: map[int32]*PartyMate{pcID: {}}}
		}
	}
	tests := []struct {
		name  string
		lim   int32
		n     int
		setup func(*Brain)
		item  GroundItem
		want  bool
	}{
		{name: "at the limit: a non-stacking item", lim: slotLimit, n: 80, item: sword},
		{name: "at the limit: a stack of a new template", lim: slotLimit, n: 80, item: newStack},
		{name: "at the limit: adena", lim: slotLimit, n: 80, item: adena, want: true},
		{name: "at the limit: a herb", lim: slotLimit, n: 80, item: herb, want: true},
		{name: "at the limit: a stack of a held template", lim: slotLimit, n: 80, item: heldStack, want: true},
		{name: "past the limit", lim: slotLimit, n: 81, item: sword},
		{name: "one slot left", lim: slotLimit, n: 79, item: sword, want: true},
		{name: "a dwarf's limit", lim: 100, n: 80, item: sword, want: true},
		{name: "limit unknown", n: 80, item: sword, want: true},
		{name: "finders keepers party: checked", lim: slotLimit, n: 80, setup: party(partyLootFindersKeepers), item: sword},
		{name: "random party loot: not checked", lim: slotLimit, n: 80, setup: party(1), item: sword, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSlotLimitBrain(t, tt.lim, tt.n)
			if tt.setup != nil {
				tt.setup(b)
			}
			if got := b.lootFits(tt.item); got != tt.want {
				t.Fatalf("lootFits = %v, want %v", got, tt.want)
			}
		})
	}
}

// pickedUp is the item ruleLoot picks up at now, recording the click; 0
// for none, and whether the rule holds the decision.
func pickedUp(b *Brain, now time.Time) (int32, bool) {
	a, ok := b.ruleLoot(now)
	if ok && a.kind == actPickup {
		b.loot.sent(a, now)
		return a.object, true
	}
	return 0, ok
}

// TestLootSlotsFullRefused checks SystemMessage 129 with the limit
// unknown: the bot gives the clicked item up at once and leaves items
// needing a slot alone, still taking adena, until it holds fewer items.
func TestLootSlotsFullRefused(t *testing.T) {
	b := newSlotLimitBrain(t, 0, 80)
	sword := func(obj int32) GroundItem {
		return GroundItem{ObjectID: obj, TemplateID: 2000, Pos: Point{X: 100}, Count: 1}
	}
	handleAll(t, b, kill(sword(junkDrop)))
	now := at(time.Second)
	if id, _ := pickedUp(b, now); id != junkDrop {
		t.Fatalf("picks up %d, want %d", id, junkDrop)
	}
	handleAll(t, b, []timed{{time.Second, SystemMessage{ID: msgSlotsFull}}})
	if len(b.loot.items) != 0 {
		t.Fatalf("queue %v after the refusal, want the item given up", b.loot.items)
	}
	handleAll(t, b, kill(sword(0x40000020)))
	if id, ok := pickedUp(b, at(2*time.Second)); id != 0 || ok {
		t.Fatalf("picks up %d (holds %v) with no slot, want nothing", id, ok)
	}
	handleAll(t, b, kill(adenaOnGround))
	if id, _ := pickedUp(b, at(3*time.Second)); id != adenaDrop {
		t.Fatalf("picks up %d, want the adena %d", id, adenaDrop)
	}
	// The adena picked up; then a sale frees a slot.
	handleAll(t, b, []timed{{3 * time.Second, GetItem{PickerID: selfID, ObjectID: adenaDrop}}})
	items := b.world.Inventory.Items
	delete(items, 0x50000001)
	b.world.Apply(ItemList{Items: slices.Collect(maps.Values(items))}, at(4*time.Second))
	handleAll(t, b, kill(sword(0x40000021)))
	if id, _ := pickedUp(b, at(5*time.Second)); id != 0x40000021 {
		t.Fatalf("picks up %d once a slot freed, want %d", id, 0x40000021)
	}
}

// newFullRingBrain is a ground looter (GroundAny) wearing nothing, with
// Gear.Jewelry on and inventory limit lim, holding adena, an unworn Magic
// Ring and non-stacking filler items up to n items.
func newFullRingBrain(t *testing.T, lim int32, n int) *Brain {
	t.Helper()
	items := []Item{{ObjectID: 0x50000000, TemplateID: adenaID, Count: 100}, carried(0x50000001, magicRing, partLRFinger)}
	for i := range int32(n - 2) {
		items = append(items, Item{ObjectID: 0x50000002 + i, TemplateID: fillerItem + i, Count: 1})
	}
	b := newSlotsBrain(t, classMysticHuman, items...)
	b.persona.Gear.Jewelry = true
	b.persona.Loot = defaultLootPolicy()
	b.persona.Loot.Ground = GroundAny
	b.cfg.Travel.herbs = map[int32]bool{herbOfLife: true}
	b.world.Self.InvLimit = lim
	return b
}

// TestLootFullBagFreesRules checks that a bag at its limit with the ground
// covered in items needing a slot leaves the decision to the rules after
// loot: the gear errand wears the unworn ring. With the limit unknown loot
// takes it.
func TestLootFullBagFreesRules(t *testing.T) {
	for _, tt := range []struct {
		name string
		lim  int32
		want string
	}{
		{name: "at the limit", lim: slotLimit, want: "gear"},
		{name: "limit unknown", want: "loot"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newFullRingBrain(t, tt.lim, int(slotLimit))
			var evs []timed
			for i := range int32(10) {
				evs = append(evs, timed{0, SpawnItem{Item: GroundItem{ObjectID: 0x40000100 + i, TemplateID: 2000 + i, Pos: Point{X: 100 + i}, Count: 1}}})
			}
			handleAll(t, b, evs)
			if rule, _ := b.decide(at(lootGrace)); rule != tt.want {
				t.Fatalf("decide = %q, want %q", rule, tt.want)
			}
		})
	}
}

// TestLootStallFreesRules checks that after lootMisses ground items in a
// row clicked for Loot.Timeout without a pickup, with the limit unknown,
// the decision goes to the rules after loot: the gear errand wears the
// unworn ring.
func TestLootStallFreesRules(t *testing.T) {
	b := newFullRingBrain(t, 0, 10)
	var evs []timed
	for i := range int32(lootMisses + 1) {
		evs = append(evs, timed{0, SpawnItem{Item: GroundItem{ObjectID: 0x40000100 + i, TemplateID: 2000, Pos: Point{X: 100}, Count: 1}}})
	}
	handleAll(t, b, evs)
	now := at(lootGrace)
	for i := range int32(lootMisses) {
		if id, _ := pickedUp(b, now); id != 0x40000100+i {
			t.Fatalf("miss %d: picks up %d, want %d", i, id, 0x40000100+i)
		}
		now = now.Add(b.persona.Loot.Timeout)
	}
	if rule, _ := b.decide(now); rule != "gear" {
		t.Fatalf("after %d misses decide = %q, want %q", lootMisses, rule, "gear")
	}
}

// TestLootStall checks the loot stall: after lootMisses ground items in a
// row clicked for Loot.Timeout without a pickup the bot leaves ground items
// alone for lootPause, still taking its own kill's drop, and its next
// pickup ends the pause.
func TestLootStall(t *testing.T) {
	b := newSlotLimitBrain(t, 0, 10)
	b.persona.Loot.Ground = GroundAny
	timeout := b.persona.Loot.Timeout
	stray := func(obj int32) timed {
		return timed{0, SpawnItem{Item: GroundItem{ObjectID: obj, TemplateID: 2000, Pos: Point{X: 100}, Count: 1}}}
	}
	handleAll(t, b, []timed{stray(0x40000100), stray(0x40000101), stray(0x40000102), stray(0x40000103)})
	now := at(lootGrace)
	for i := range int32(lootMisses) {
		if id, _ := pickedUp(b, now); id != 0x40000100+i {
			t.Fatalf("miss %d: picks up %d, want %d", i, id, 0x40000100+i)
		}
		now = now.Add(timeout)
	}
	if id, ok := pickedUp(b, now); id != 0 || ok {
		t.Fatalf("after %d misses picks up %d (holds %v), want the ground left alone", lootMisses, id, ok)
	}
	handleAll(t, b, []timed{stray(0x40000104)})
	if id, ok := pickedUp(b, now.Add(lootGrace)); id != 0 || ok {
		t.Fatalf("paused: picks up %d (holds %v), want nothing", id, ok)
	}
	// Its own kill's drop it takes, and the pickup ends the pause.
	evs := kill(GroundItem{ObjectID: adenaDrop, TemplateID: adenaID, Pos: Point{X: 100}, Stackable: true, Count: 5})
	for i := range evs {
		evs[i].at = now.Sub(t0) + lootGrace
	}
	handleAll(t, b, evs)
	now = now.Add(lootGrace)
	if id, _ := pickedUp(b, now); id != adenaDrop {
		t.Fatalf("paused: picks up %d, want its own drop %d", id, adenaDrop)
	}
	// The item seen during the pause is queued once it ends.
	handleAll(t, b, []timed{{now.Sub(t0), GetItem{PickerID: selfID, ObjectID: adenaDrop}}})
	if id, _ := pickedUp(b, now); id != 0x40000104 {
		t.Fatalf("after a pickup picks up %d, want %d", id, 0x40000104)
	}
}

// TestLootStallPauseEnds checks that ground pickups resume lootPause after
// the stall.
func TestLootStallPauseEnds(t *testing.T) {
	b := newSlotLimitBrain(t, 0, 10)
	b.persona.Loot.Ground = GroundAny
	b.loot.pauseUntil = at(lootPause)
	handleAll(t, b, []timed{{0, SpawnItem{Item: GroundItem{ObjectID: junkDrop, TemplateID: 2000, Pos: Point{X: 100}, Count: 1}}}})
	if id, ok := pickedUp(b, at(lootPause-time.Millisecond)); id != 0 || ok {
		t.Fatalf("paused: picks up %d (holds %v), want nothing", id, ok)
	}
	if id, _ := pickedUp(b, at(lootPause)); id != junkDrop {
		t.Fatalf("pause over: picks up %d, want %d", id, junkDrop)
	}
}

// TestGroundTakesJewelrySlots checks that a fitting persona takes no
// jewelry piece from the ground for a slot its bag already fills with
// pieces it would wear: two rings or earrings, one necklace.
func TestGroundTakesJewelrySlots(t *testing.T) {
	ring := func(obj int32) Item { return carried(obj, magicRing, partLRFinger) }
	tests := []struct {
		name  string
		items []Item
		id    int32
		want  bool
	}{
		{name: "no ring carried", id: magicRing, want: true},
		{name: "one unworn ring", items: []Item{ring(301)}, id: magicRing, want: true},
		{name: "two unworn rings", items: []Item{ring(301), ring(302)}, id: magicRing},
		{name: "two unworn rings, an earring", items: []Item{ring(301), ring(302)}, id: apprenticeEar, want: true},
		{name: "one unworn necklace", items: []Item{carried(303, necklaceOfMagic, partNeck)}, id: necklaceOfMagic},
		{
			name: "a pair worn, one beating the lesser", id: ringOfKnowledge, want: true,
			items: []Item{wornAt(208, magicRing, partLRFinger), wornAt(209, ringOfAnguish, partLRFinger)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSlotsBrain(t, classMysticHuman, tt.items...)
			b.persona.Gear.Jewelry = true
			b.persona.Loot = defaultLootPolicy()
			if got := b.groundTakes(GroundItem{TemplateID: tt.id, Count: 1}); got != tt.want {
				t.Fatalf("groundTakes(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestJunkSpareJewelry checks a bag full of unworn jewelry: the gear errand
// wears the best pieces, one necklace, two earrings and two rings, and the
// rest is junk to sell, before and after it wears them.
func TestJunkSpareJewelry(t *testing.T) {
	var items []Item
	obj := int32(300)
	add := func(id, part int32, n int) {
		for range n {
			items = append(items, carried(obj, id, part))
			obj++
		}
	}
	add(necklaceOfMagic, partNeck, 3)
	add(magicRing, partLRFinger, 5)
	add(apprenticeEar, partLREar, 4)
	count := func(junk map[int32]Item) map[int32]int {
		out := make(map[int32]int)
		for _, it := range junk {
			out[it.TemplateID]++
		}
		return out
	}
	want := map[int32]int{necklaceOfMagic: 2, magicRing: 3, apprenticeEar: 2}
	b := newSlotsBrain(t, classMysticHuman, items...)
	b.persona.Gear.Jewelry = true
	if got := count(b.junk()); !maps.Equal(got, want) {
		t.Fatalf("junk = %v, want %v", got, want)
	}
	a, ok := b.ruleGear(t0)
	if !ok || a.kind != actUseItem {
		t.Fatalf("ruleGear = %v %v, want a piece used", a.kind, ok)
	}
	// Worn: the first necklace, earrings and rings.
	worn := map[int32]int32{300: partNeck, 303: partLRFinger, 304: partLRFinger, 308: partLREar, 309: partLREar}
	for i, it := range items {
		if part, ok := worn[it.ObjectID]; ok {
			items[i].Equipped, items[i].BodySlot = true, part
		}
	}
	b.world.Apply(ItemList{Items: items}, t0)
	if got := count(b.junk()); !maps.Equal(got, want) {
		t.Fatalf("worn: junk = %v, want %v", got, want)
	}
}
