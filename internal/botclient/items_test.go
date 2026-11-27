package botclient

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

var (
	potionRow = Item{
		ObjectID: 0x30000001, TemplateID: 1061, Count: 20, Category: 4, SubCategory: 5,
		BodySlot: 0, ManaLeft: -1,
	}
	swordRow = Item{
		ObjectID: 0x30000002, TemplateID: 2369, Count: 1, SubCategory: 0, CustomType1: 1,
		Equipped: true, BodySlot: 0x4000, Enchant: 3, CustomType2: 2, Augmentation: 77,
	}
	shotRow = Item{ObjectID: 0x30000003, TemplateID: 1835, Count: 500, Category: 4, SubCategory: 5}
)

func TestItemEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"UseItem", encodeUseItem(0x30000001), []byte{0x14, 1, 0, 0, 0x30, 0, 0, 0, 0}},
		{"RequestAutoSoulShot on", encodeRequestAutoSoulShot(1835, true), []byte{0xd0, 0x05, 0, 0x2b, 0x07, 0, 0, 1, 0, 0, 0}},
		{"RequestAutoSoulShot off", encodeRequestAutoSoulShot(1835, false), []byte{0xd0, 0x05, 0, 0x2b, 0x07, 0, 0, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

func TestWorldInventory(t *testing.T) {
	w := newTestWorld(t)
	w.Apply(ItemList{Items: []Item{potionRow, swordRow, shotRow}}, t0)
	if it, ok := w.Inventory.ByTemplate(1061); !ok || it != potionRow {
		t.Fatalf("ByTemplate(1061) = %+v, %v", it, ok)
	}
	w.Apply(ExAutoSoulShot{TemplateID: 1835, Enabled: true}, t0)

	drank := potionRow
	drank.Count = 19
	second := shotRow
	second.ObjectID = 0x30000009
	w.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: drank}, {State: ItemRemoved, Item: swordRow}, {State: ItemAdded, Item: second}}}, t0)
	if it, _ := w.Inventory.ByTemplate(1061); it.Count != 19 {
		t.Fatalf("potion count %d, want 19", it.Count)
	}
	if _, ok := w.Inventory.ByTemplate(2369); ok {
		t.Fatal("removed sword still carried")
	}
	if it, _ := w.Inventory.ByTemplate(1835); it.ObjectID != shotRow.ObjectID {
		t.Fatalf("ByTemplate(1835) = %#x, want the lowest object id %#x", it.ObjectID, shotRow.ObjectID)
	}

	// One stack of two leaves: the auto-use state stays.
	w.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: shotRow}}}, t0)
	if on, known := w.Inventory.AutoShots[1835]; !known || !on {
		t.Fatalf("AutoShots[1835] = %v, %v with a stack left", on, known)
	}
	// The last stack leaves: the state goes with it.
	w.Apply(ExAutoSoulShot{TemplateID: 1835}, t0)
	w.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: second}}}, t0)
	if _, known := w.Inventory.AutoShots[1835]; known {
		t.Fatal("auto-use state kept after the last stack left")
	}

	// A new ItemList replaces the items and forgets shots no longer carried.
	w.Apply(ExAutoSoulShot{TemplateID: 2509, Enabled: true}, t0)
	w.Apply(ItemList{Items: []Item{swordRow}}, t0)
	if len(w.Inventory.Items) != 1 || len(w.Inventory.AutoShots) != 0 {
		t.Fatalf("after ItemList: items %v, auto shots %v", w.Inventory.Items, w.Inventory.AutoShots)
	}
	// An update before any list builds the inventory.
	w2 := newTestWorld(t)
	w2.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: potionRow}}}, t0)
	if _, ok := w2.Inventory.ByTemplate(1061); !ok {
		t.Fatal("InventoryUpdate before ItemList dropped the item")
	}
}

// TestBrainPotion: rule 3 drinks the best potion whose reuse ran out, and
// flees once none is ready.
func TestBrainPotion(t *testing.T) {
	greater := Item{ObjectID: 0x30000011, TemplateID: 1539, Count: 1}
	lesser := Item{ObjectID: 0x30000012, TemplateID: 1060, Count: 5}
	tests := []struct {
		name    string
		items   []Item
		setup   func(b *Brain)
		want    actionKind
		wantObj int32
	}{
		{name: "best potion first", items: []Item{lesser, greater}, want: actUseItem, wantObj: greater.ObjectID},
		{
			name: "no other potion while one heals", items: []Item{lesser, greater},
			setup: func(b *Brain) { b.usedAt[1539] = at(time.Second) },
			want:  actMove,
		},
		{
			name: "no potion while a lesser one heals", items: []Item{lesser, greater},
			setup: func(b *Brain) { b.usedAt[1060] = at(10*time.Second - potionReuse + time.Millisecond) },
			want:  actMove,
		},
		{
			name: "best potion again once the heal ran out", items: []Item{lesser, greater},
			setup: func(b *Brain) { b.usedAt[1060] = at(10*time.Second - potionReuse) },
			want:  actUseItem, wantObj: greater.ObjectID,
		},
		{
			name: "reuse over drinks again", items: []Item{greater},
			setup: func(b *Brain) { b.usedAt[1539] = at(10*time.Second - potionReuse) },
			want:  actUseItem, wantObj: greater.ObjectID,
		},
		{
			name: "flees while every potion cools", items: []Item{greater},
			setup: func(b *Brain) { b.usedAt[1539] = at(time.Second) },
			want:  actMove,
		},
		{name: "flees with no potion", items: []Item{swordRow}, want: actMove},
		{name: "flees with an empty stack", items: []Item{{ObjectID: 1, TemplateID: 1061}}, want: actMove},
		{
			name: "stands up before drinking", items: []Item{greater},
			setup: func(b *Brain) { b.world.Self.Sitting = true },
			want:  actSitStand,
		},
		{
			name: "drinks on the way while fleeing", items: []Item{greater},
			setup: func(b *Brain) { b.world.Apply(MoveToLocation{ObjectID: selfID, Dest: Point{X: -5000}}, t0) },
			want:  actUseItem, wantObj: greater.ObjectID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, out := newTestBrain(t, Persona{CriticalHP: 20, FleeDistance: 600, Potions: DefaultPersona().Potions})
			b.world.Self.InCombat, b.world.Self.HP, b.world.Target = true, 10, npcID
			b.world.Apply(ItemList{Items: tt.items}, t0)
			if tt.setup != nil {
				tt.setup(b)
			}
			// A flee walks: it needs ground under it.
			if tt.want == actMove {
				onGround(t, b)
			}
			now := at(10 * time.Second)
			rule, a := b.decide(now)
			if rule != "critical" || a.kind != tt.want {
				t.Fatalf("decide = %q kind %d, want critical kind %d", rule, a.kind, tt.want)
			}
			if tt.want != actUseItem {
				return
			}
			if want := encodeUseItem(tt.wantObj); !bytes.Equal(a.payload, want) {
				t.Fatalf("payload % x, want % x", a.payload, want)
			}
			if sent, err := b.send(a, now); !sent || err != nil {
				t.Fatalf("send = %v, %v", sent, err)
			}
			if len(out.frames) != 1 || b.usedAt[a.item] != now {
				t.Fatalf("sent %d frames, usedAt %v", len(out.frames), b.usedAt)
			}
			// No potion, the one just drunk or another, during its reuse.
			if _, again := b.decide(now.Add(potionReuse - time.Millisecond)); again.kind == actUseItem {
				t.Fatalf("drank potion %d during the reuse of %d", again.item, a.item)
			}
		})
	}
}

// TestBrainShots: auto use of the shot the wielded weapon charges is asked
// once, again only when the server left the request unanswered for
// shotRetry, and not after the server reported a state, until a new stack
// arrives. A bot without a datapack takes its weapon as a no-grade one:
// a fighter's no-grade soulshots, not the spiritshots it carries.
func TestBrainShots(t *testing.T) {
	spirit := Item{ObjectID: 0x30000021, TemplateID: 2509, Count: 100}
	weapon := Item{ObjectID: 0x30000022, TemplateID: 2369, Count: 1, Equipped: true, BodySlot: partRHand}
	b, out := newTestBrain(t, Persona{AutoShots: true})
	b.world.Apply(ItemList{Items: []Item{potionRow, shotRow, spirit, weapon}}, t0)

	step := func(now time.Time) string {
		t.Helper()
		rule, a := b.decide(now)
		if a.kind != actHold {
			if _, err := b.send(a, now); err != nil {
				t.Fatal(err)
			}
		}
		return rule
	}
	toggles := func() []int32 {
		var ids []int32
		for _, f := range out.frames {
			if f[0] == opGameClientExtended {
				ids = append(ids, int32(f[3])|int32(f[4])<<8)
			}
		}
		return ids
	}

	for i, want := range []string{"shots", ""} {
		if rule := step(at(time.Duration(i+1) * time.Second)); rule != want {
			t.Fatalf("decision %d: rule %q, want %q", i, rule, want)
		}
	}
	if got := toggles(); !reflect.DeepEqual(got, []int32{1835}) {
		t.Fatalf("toggled %v, want [1835]", got)
	}
	// Left unanswered, 1835 is asked again after shotRetry.
	if rule := step(at(time.Second + shotRetry - time.Millisecond)); rule != "" {
		t.Fatalf("asked again before shotRetry: rule %q", rule)
	}
	if rule := step(at(time.Second + shotRetry)); rule != "shots" {
		t.Fatalf("no retry after shotRetry: rule %q", rule)
	}
	b.world.Apply(ExAutoSoulShot{TemplateID: 1835, Enabled: true}, at(time.Minute))
	// The server disables 1835 on a short stack, still carried: it stays
	// off while the stack does not grow.
	short := shotRow
	short.Count = 1
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: short}}}, at(time.Minute))
	b.world.Apply(ExAutoSoulShot{TemplateID: 1835}, at(time.Minute))
	if rule := step(at(2 * time.Minute)); rule != "" {
		t.Fatalf("re-enabled a disabled shot: rule %q", rule)
	}
	// A restock of the same stack turns it on again.
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: shotRow}}}, at(2*time.Minute))
	if rule := step(at(2*time.Minute + time.Second)); rule != "shots" {
		t.Fatalf("restocked shot not turned on: rule %q", rule)
	}
	b.world.Apply(ExAutoSoulShot{TemplateID: 1835}, at(2*time.Minute))
	// The last stack leaves, a new one arrives: it is turned on again.
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: shotRow}}}, at(2*time.Minute))
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: shotRow}}}, at(2*time.Minute))
	if rule := step(at(3 * time.Minute)); rule != "shots" {
		t.Fatalf("new stack not turned on: rule %q", rule)
	}
	if got := toggles(); !reflect.DeepEqual(got, []int32{1835, 1835, 1835, 1835}) {
		t.Fatalf("toggled %v, want [1835 1835 1835 1835]", got)
	}
}
