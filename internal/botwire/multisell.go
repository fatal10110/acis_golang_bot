package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// MultiSellItem is a product or an ingredient of a multisell entry: an
// item template, how many one unit of the entry gives or takes, and its
// enchant level. BodySlot is a product's body part (0 for an ingredient or
// an item without a template); Type2 is the item's type2, 65535 for an item
// without a template.
type MultiSellItem struct {
	ItemID, BodySlot int32
	Type2            uint16
	Count            int32
	Enchant          uint16
}

// MultiSellEntry is one row of a multisell window: its id, numbered from 1
// across the whole list (what MultiSellChoose names), whether every product
// stacks (a non-stackable one is traded one unit at a time), and what one
// unit gives and takes. The adena an entry takes, the castle tax included,
// is its last ingredient (server multisell.Entry.prepare).
type MultiSellEntry struct {
	ID                    int32
	Stackable             bool
	Products, Ingredients []MultiSellItem
}

// MultiSellList is one page of the multisell window on list ListID: Page
// numbered from 1, Finished on the last page, PageSize rows a page at most.
type MultiSellList struct {
	ListID, Page int32
	Finished     bool
	PageSize     int32
	Entries      []MultiSellEntry
}

func (MultiSellList) IsEvent() {}

// The wire sizes of a multisell entry's header, a product and an
// ingredient (serverpackets/multisell.go).
const (
	multiSellEntrySize      = 4 + 4 + 4 + 1 + 2 + 2
	multiSellProductSize    = 2 + 4 + 2 + 4 + 2 + 4 + 4
	multiSellIngredientSize = 2 + 2 + 4 + 2 + 4 + 4
)

// decodeMultiSellList: list id, page, finished, page size and the entry
// count of this page as int32, then per entry its id, two zeros, the
// stackable byte, the product and ingredient counts as uint16, the
// products (item id as uint16, body part, type2, count, enchant, two
// zeros) and the ingredients (item id as uint16, type2, count, enchant,
// two zeros) (serverpackets/multisell.go FrameMultiSellList).
func decodeMultiSellList(r *wire.Reader) Event {
	l := MultiSellList{ListID: r.ReadInt32(), Page: r.ReadInt32(), Finished: r.ReadInt32() != 0, PageSize: r.ReadInt32()}
	n := int(r.ReadInt32())
	l.Entries = make([]MultiSellEntry, 0, capFor(r, n, multiSellEntrySize))
	for ; n > 0 && r.Err() == nil; n-- {
		e := MultiSellEntry{ID: r.ReadInt32()}
		r.Skip(8) // two zeros
		e.Stackable = r.ReadUint8() != 0
		np, ni := int(r.ReadUint16()), int(r.ReadUint16())
		e.Products = make([]MultiSellItem, 0, capFor(r, np, multiSellProductSize))
		for ; np > 0 && r.Err() == nil; np-- {
			it := MultiSellItem{ItemID: int32(r.ReadUint16()), BodySlot: r.ReadInt32(), Type2: r.ReadUint16()}
			it.Count, it.Enchant = r.ReadInt32(), r.ReadUint16()
			r.Skip(8) // two zeros
			e.Products = append(e.Products, it)
		}
		e.Ingredients = make([]MultiSellItem, 0, capFor(r, ni, multiSellIngredientSize))
		for ; ni > 0 && r.Err() == nil; ni-- {
			it := MultiSellItem{ItemID: int32(r.ReadUint16()), Type2: r.ReadUint16()}
			it.Count, it.Enchant = r.ReadInt32(), r.ReadUint16()
			r.Skip(8) // two zeros
			e.Ingredients = append(e.Ingredients, it)
		}
		l.Entries = append(l.Entries, e)
	}
	return l
}

// EncodeMultiSellChoose trades amount units of entry entryID of the
// multisell list listID the server last showed: list id, entry id and
// amount (clientpackets/multisell.go).
func EncodeMultiSellChoose(listID, entryID, amount int32) []byte {
	w := wire.NewPacketWriter(OpGameMultiSellChoose)
	w.WriteInt32(listID)
	w.WriteInt32(entryID)
	w.WriteInt32(amount)
	return w.Bytes()
}
