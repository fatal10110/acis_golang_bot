package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// WarehouseItem is one row of a warehouse list: a carried item a deposit
// may move, or an item the warehouse holds.
type WarehouseItem struct {
	ObjectID, TemplateID, Count int32
	// Category and SubCategory are the client's item classes, as an Item's.
	Category, SubCategory    uint16
	CustomType1, CustomType2 uint16
	BodySlot                 int32
	Enchant                  uint16
	// Augmentation is the augment's two attribute words, 0 for none.
	Augmentation int64
}

// WarehouseDepositList opens a deposit window: the warehouse type (2 for a
// clan's), the bot's adena and the items it may deposit.
type WarehouseDepositList struct {
	Type  uint16
	Adena int32
	Items []WarehouseItem
}

// WarehouseWithdrawList opens a withdrawal window: the warehouse type (2
// for a clan's), the bot's adena and the items the warehouse holds.
type WarehouseWithdrawList struct {
	Type  uint16
	Adena int32
	Items []WarehouseItem
}

func (WarehouseDepositList) IsEvent() {}

func (WarehouseWithdrawList) IsEvent() {}

// warehouseItemSize is the wire size of a warehouse list row.
const warehouseItemSize = 40

// readWarehouseItem reads a list row: category, object id, template id,
// count, subcategory, custom type 1, body slot, enchant, custom type 2, a
// zero, the object id again, augmentation (serverpackets/warehouse.go
// writeWarehouseItem).
func readWarehouseItem(r *wire.Reader) WarehouseItem {
	it := WarehouseItem{
		Category: r.ReadUint16(), ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(),
		Count: r.ReadInt32(), SubCategory: r.ReadUint16(), CustomType1: r.ReadUint16(),
		BodySlot: r.ReadInt32(), Enchant: r.ReadUint16(), CustomType2: r.ReadUint16(),
	}
	r.ReadUint16()
	r.ReadInt32()
	it.Augmentation = r.ReadInt64()
	return it
}

// readWarehouseList reads a list: warehouse type, adena, row count as
// uint16, then the rows (serverpackets/warehouse.go frameWarehouseList).
// The loop ends at the first short read.
func readWarehouseList(r *wire.Reader) (uint16, int32, []WarehouseItem) {
	typ, adena := r.ReadUint16(), r.ReadInt32()
	n := int(r.ReadUint16())
	items := make([]WarehouseItem, 0, capFor(r, n, warehouseItemSize))
	for ; n > 0 && r.Err() == nil; n-- {
		items = append(items, readWarehouseItem(r))
	}
	return typ, adena, items
}

func decodeWarehouseDepositList(r *wire.Reader) Event {
	typ, adena, items := readWarehouseList(r)
	return WarehouseDepositList{Type: typ, Adena: adena, Items: items}
}

func decodeWarehouseWithdrawList(r *wire.Reader) Event {
	typ, adena, items := readWarehouseList(r)
	return WarehouseWithdrawList{Type: typ, Adena: adena, Items: items}
}
