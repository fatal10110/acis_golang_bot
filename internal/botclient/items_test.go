package botclient

import (
	"bytes"
	"errors"
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

// itemFields is it as the server writes an item row.
func itemFields(it Item) []any {
	var eq uint16
	if it.Equipped {
		eq = 1
	}
	return []any{
		it.Category, it.ObjectID, it.TemplateID, it.Count, it.SubCategory, it.CustomType1,
		eq, it.BodySlot, it.Enchant, it.CustomType2, it.Augmentation, it.ManaLeft,
	}
}

func itemList(show uint16, items ...Item) []byte {
	fields := []any{show, uint16(len(items))}
	for _, it := range items {
		fields = append(fields, itemFields(it)...)
	}
	return packet(opGameItemList, fields...)
}

func inventoryUpdate(changes ...ItemChange) []byte {
	fields := []any{uint16(len(changes))}
	for _, c := range changes {
		fields = append(append(fields, uint16(c.State)), itemFields(c.Item)...)
	}
	return packet(opGameInventoryUpdate, fields...)
}

func exAutoSoulShot(templateID int32, enabled int32) []byte {
	return packet(opGameExtended, opGameExAutoSoulShot, templateID, enabled)
}

func TestDecodeItemEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{
			// One row as itemlist.go writes it: category 4, object id 0x11,
			// template 57 (adena), count 1000, subcategory 4, custom type 1
			// 0, not equipped, slot 0, enchant 0, custom type 2 0, no
			// augmentation, mana -1.
			"ItemList bytes",
			[]byte{
				0x1b, 0, 0, 1, 0,
				4, 0, 0x11, 0, 0, 0, 57, 0, 0, 0, 0xe8, 0x03, 0, 0, 4, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff,
			},
			ItemList{Items: []Item{{Category: 4, ObjectID: 0x11, TemplateID: 57, Count: 1000, SubCategory: 4, ManaLeft: -1}}},
		},
		{"ItemList", itemList(1, potionRow, swordRow), ItemList{ShowWindow: true, Items: []Item{potionRow, swordRow}}},
		{"ItemList empty", itemList(0), ItemList{}},
		{
			"InventoryUpdate",
			inventoryUpdate(ItemChange{ItemModified, potionRow}, ItemChange{ItemAdded, shotRow}, ItemChange{ItemRemoved, swordRow}),
			InventoryUpdate{Changes: []ItemChange{{ItemModified, potionRow}, {ItemAdded, shotRow}, {ItemRemoved, swordRow}}},
		},
		{"InventoryUpdate empty", inventoryUpdate(), InventoryUpdate{}},
		// 0xfe, sub-opcode 0x0012, template 1835 (0x072b), enabled.
		{"ExAutoSoulShot bytes", []byte{0xfe, 0x12, 0, 0x2b, 0x07, 0, 0, 1, 0, 0, 0}, ExAutoSoulShot{TemplateID: 1835, Enabled: true}},
		{"ExAutoSoulShot disabled", exAutoSoulShot(2509, 0), ExAutoSoulShot{TemplateID: 2509}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeItemEventsRejects(t *testing.T) {
	list := itemList(0, potionRow)
	update := inventoryUpdate(ItemChange{ItemAdded, shotRow})
	shot := exAutoSoulShot(1835, 1)
	tests := []struct {
		name string
		raw  []byte
	}{
		{"ItemList short row", list[:len(list)-1]},
		{"ItemList count past the rows", itemList(0, potionRow)[:5]},
		{"ItemList trailing", append(list, 0)},
		{"InventoryUpdate short row", update[:len(update)-2]},
		{"InventoryUpdate trailing", append(update, 0)},
		{"ExAutoSoulShot short", shot[:len(shot)-1]},
		{"ExAutoSoulShot trailing", append(shot, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ev, ok, err := decodeEvent(tt.raw); err == nil {
				t.Fatalf("decodeEvent = %v, %v, nil; want an error", ev, ok)
			}
		})
	}
	if _, _, err := decodeEvent(append(shot, 0)); !errors.Is(err, errTrailingBytes) {
		t.Fatalf("trailing ExAutoSoulShot: %v, want errTrailingBytes", err)
	}
}

// TestDecodeExtendedDropped: an extended frame with no decoder for its
// sub-opcode, or too short to carry one, is dropped unparsed.
func TestDecodeExtendedDropped(t *testing.T) {
	for _, raw := range [][]byte{
		{0xfe},
		{0xfe, 0x12},
		{0xfe, 0x2e, 0x00, 1, 2, 3}, // ExSendManorList's sub-opcode
		{0xfe, 0x49, 0x00},          // ExUseSharedGroupItem, too short for its layout
		{0xfe, 0x12, 0x01, 1, 2, 3}, // 0x0112: only the low byte matches
	} {
		if ev, ok, err := decodeEvent(raw); ok || err != nil {
			t.Errorf("decodeEvent(% x) = %v, %v, %v; want dropped", raw, ev, ok, err)
		}
	}
}

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
	w.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, drank}, {ItemRemoved, swordRow}, {ItemAdded, second}}}, t0)
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
	w.Apply(InventoryUpdate{Changes: []ItemChange{{ItemRemoved, shotRow}}}, t0)
	if on, known := w.Inventory.AutoShots[1835]; !known || !on {
		t.Fatalf("AutoShots[1835] = %v, %v with a stack left", on, known)
	}
	// The last stack leaves: the state goes with it.
	w.Apply(ExAutoSoulShot{TemplateID: 1835}, t0)
	w.Apply(InventoryUpdate{Changes: []ItemChange{{ItemRemoved, second}}}, t0)
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
	w2.Apply(InventoryUpdate{Changes: []ItemChange{{ItemAdded, potionRow}}}, t0)
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
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, short}}}, at(time.Minute))
	b.world.Apply(ExAutoSoulShot{TemplateID: 1835}, at(time.Minute))
	if rule := step(at(2 * time.Minute)); rule != "" {
		t.Fatalf("re-enabled a disabled shot: rule %q", rule)
	}
	// A restock of the same stack turns it on again.
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, shotRow}}}, at(2*time.Minute))
	if rule := step(at(2*time.Minute + time.Second)); rule != "shots" {
		t.Fatalf("restocked shot not turned on: rule %q", rule)
	}
	b.world.Apply(ExAutoSoulShot{TemplateID: 1835}, at(2*time.Minute))
	// The last stack leaves, a new one arrives: it is turned on again.
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemRemoved, shotRow}}}, at(2*time.Minute))
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemAdded, shotRow}}}, at(2*time.Minute))
	if rule := step(at(3 * time.Minute)); rule != "shots" {
		t.Fatalf("new stack not turned on: rule %q", rule)
	}
	if got := toggles(); !reflect.DeepEqual(got, []int32{1835, 1835, 1835, 1835}) {
		t.Fatalf("toggled %v, want [1835 1835 1835 1835]", got)
	}
}
