package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Item is one item the bot carries, as ItemList and InventoryUpdate report
// it.
type Item struct {
	ObjectID, TemplateID int32
	Count                int32
	// Category and SubCategory are the client's item classes
	// (item.Template.Category on the server): weapon or jewelry, armor,
	// money or etc item; and weapon, armor, accessory, quest, money or
	// other.
	Category, SubCategory    uint16
	CustomType1, CustomType2 uint16
	// Equipped is set for an item on the paperdoll, in BodySlot.
	Equipped bool
	BodySlot int32
	Enchant  uint16
	// Augmentation is the augment's attributes, 0 for none.
	Augmentation int32
	// ManaLeft is the shadow item's mana in minutes as the client shows it.
	ManaLeft int32
}

// ItemList is the whole inventory, sent at world entry and whenever the
// server replaces the client's item state wholesale.
type ItemList struct {
	// ShowWindow asks the client to open the inventory window.
	ShowWindow bool
	Items      []Item
}

// ItemState is what an InventoryUpdate entry did to its item
// (itemcontainer.UpdateState on the server).
type ItemState uint16

// InventoryUpdate states; 0 is the client's "unchanged" and is never sent.
const (
	ItemAdded    ItemState = 1
	ItemModified ItemState = 2
	ItemRemoved  ItemState = 3
)

// ItemChange is one entry of an InventoryUpdate: the item as it is after
// the change.
type ItemChange struct {
	State ItemState
	Item  Item
}

// InventoryUpdate reports items added, changed or removed since the last
// report: after a use, a pickup, a purchase, an equip.
type InventoryUpdate struct{ Changes []ItemChange }

// ExAutoSoulShot acknowledges an auto-use toggle of the shot template
// TemplateID, and reports a disable when its stock runs out.
type ExAutoSoulShot struct {
	TemplateID int32
	Enabled    bool
}

func (ItemList) IsEvent() {}

func (InventoryUpdate) IsEvent() {}

func (ExAutoSoulShot) IsEvent() {}

// readItem reads an item row: category, object id, template id, count,
// subcategory, custom type 1, equipped, body slot, enchant, custom type 2,
// augmentation, mana (writeItemList and writeInventoryUpdate).
func readItem(r *wire.Reader) Item {
	return Item{
		Category: r.ReadUint16(), ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(),
		Count: r.ReadInt32(), SubCategory: r.ReadUint16(), CustomType1: r.ReadUint16(),
		Equipped: r.ReadUint16() != 0, BodySlot: r.ReadInt32(), Enchant: r.ReadUint16(),
		CustomType2: r.ReadUint16(), Augmentation: r.ReadInt32(), ManaLeft: r.ReadInt32(),
	}
}

// decodeItemList: show-window flag and item count as uint16, then the
// rows. The count comes off the wire, so the loop ends at the first short
// read.
func decodeItemList(r *wire.Reader) Event {
	e := ItemList{ShowWindow: r.ReadUint16() != 0}
	for n := r.ReadUint16(); n > 0 && r.Err() == nil; n-- {
		e.Items = append(e.Items, readItem(r))
	}
	return e
}

// decodeInventoryUpdate: entry count as uint16, then per entry its state
// as uint16 and the item row. The loop ends at the first short read.
func decodeInventoryUpdate(r *wire.Reader) Event {
	var e InventoryUpdate
	for n := r.ReadUint16(); n > 0 && r.Err() == nil; n-- {
		e.Changes = append(e.Changes, ItemChange{State: ItemState(r.ReadUint16()), Item: readItem(r)})
	}
	return e
}

// decodeExAutoSoulShot: template id, enabled as int32.
func decodeExAutoSoulShot(r *wire.Reader) Event {
	return ExAutoSoulShot{TemplateID: r.ReadInt32(), Enabled: r.ReadInt32() != 0}
}
