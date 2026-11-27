package botwire

import (
	"errors"
	"reflect"
	"testing"
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
	return packet(OpGameItemList, fields...)
}

func inventoryUpdate(changes ...ItemChange) []byte {
	fields := []any{uint16(len(changes))}
	for _, c := range changes {
		fields = append(append(fields, uint16(c.State)), itemFields(c.Item)...)
	}
	return packet(OpGameInventoryUpdate, fields...)
}

func exAutoSoulShot(templateID int32, enabled int32) []byte {
	return packet(OpGameExtended, OpGameExAutoSoulShot, templateID, enabled)
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
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeEvent = %+v, want %+v", got, tt.want)
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
			if ev, ok, err := DecodeEvent(tt.raw); err == nil {
				t.Fatalf("DecodeEvent = %v, %v, nil; want an error", ev, ok)
			}
		})
	}
	if _, _, err := DecodeEvent(append(shot, 0)); !errors.Is(err, ErrTrailingBytes) {
		t.Fatalf("trailing ExAutoSoulShot: %v, want ErrTrailingBytes", err)
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
		if ev, ok, err := DecodeEvent(raw); ok || err != nil {
			t.Errorf("DecodeEvent(% x) = %v, %v, %v; want dropped", raw, ev, ok, err)
		}
	}
}
