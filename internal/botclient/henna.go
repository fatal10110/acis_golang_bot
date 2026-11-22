package botclient

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// System messages of the Symbol Maker (serverpackets/henna.go,
// systemmessage.go). YouNotEnoughAdena is msgNotEnoughAdena.
const (
	msgNotEnoughItems = 351
	msgCantDrawSymbol = 899
	msgSymbolsFull    = 900
)

// Symbol costs (model/henna/henna.go): a draw takes hennaDrawDyes of the
// symbol's dye, and a removal costs a fifth of the draw price.
const (
	hennaDrawDyes       = 10
	hennaRemoveFraction = 5
)

// HennaRow is one symbol of the draw window: its id, its dye, the dyes a
// draw takes and the draw price.
type HennaRow struct {
	SymbolID, DyeID int32
	Dyes, Price     int32
}

// HennaEquipList is the Symbol Maker's draw window: the bot's adena, the
// symbol slots its class has, and the symbols its class may use whose dye
// it carries, in ascending symbol id order (symbolmaker.go Drawable).
type HennaEquipList struct {
	Adena, MaxSlots int32
	Symbols         []HennaRow
}

// WornSymbol is one symbol the bot wears: its id, and Active, the same id
// when its class can use it, else 0 (model/henna/list.go): an inactive
// symbol stays in its slot and adds nothing.
type WornSymbol struct {
	SymbolID, Active int32
}

// HennaInfo is the bot's symbols: the stats they add (INT, STR, CON, MEN,
// DEX, WIT), the slots its class has, and the symbols worn, in slot order.
type HennaInfo struct {
	INT, STR, CON, MEN, DEX, WIT uint8
	MaxSlots                     int32
	Worn                         []WornSymbol
}

func (HennaEquipList) event() {}
func (HennaInfo) event()      {}

// decodeHennaEquipList: adena, max slots, count, then per symbol its id,
// dye, the dyes a draw takes, the price and a constant 1
// (serverpackets/henna.go FrameHennaEquipList). The constant is dropped.
// The loop ends at the first short read.
func decodeHennaEquipList(r *wire.Reader) Event {
	e := HennaEquipList{Adena: r.ReadInt32(), MaxSlots: r.ReadInt32()}
	n := int(r.ReadInt32())
	e.Symbols = make([]HennaRow, 0, capFor(r, n, 20))
	for ; n > 0 && r.Err() == nil; n-- {
		row := HennaRow{SymbolID: r.ReadInt32(), DyeID: r.ReadInt32(), Dyes: r.ReadInt32(), Price: r.ReadInt32()}
		r.ReadInt32()
		e.Symbols = append(e.Symbols, row)
	}
	return e
}

// decodeHennaInfo: INT, STR, CON, MEN, DEX and WIT as one byte each, max
// slots, count, then per symbol its id and its active id
// (serverpackets/hennainfo.go frameHennaSummary). The loop ends at the
// first short read.
func decodeHennaInfo(r *wire.Reader) Event {
	e := HennaInfo{
		INT: r.ReadUint8(), STR: r.ReadUint8(), CON: r.ReadUint8(),
		MEN: r.ReadUint8(), DEX: r.ReadUint8(), WIT: r.ReadUint8(),
		MaxSlots: r.ReadInt32(),
	}
	n := int(r.ReadInt32())
	e.Worn = make([]WornSymbol, 0, capFor(r, n, 8))
	for ; n > 0 && r.Err() == nil; n-- {
		e.Worn = append(e.Worn, WornSymbol{SymbolID: r.ReadInt32(), Active: r.ReadInt32()})
	}
	return e
}

// encodeRequestHennaEquip draws symbolID (clientpackets/henna.go: one
// int32).
func encodeRequestHennaEquip(symbolID int32) []byte {
	w := wire.NewPacketWriter(opGameRequestHennaEquip)
	w.WriteInt32(symbolID)
	return w.Bytes()
}

// encodeRequestHennaUnequip removes the worn symbolID
// (clientpackets/henna.go: one int32).
func encodeRequestHennaUnequip(symbolID int32) []byte {
	w := wire.NewPacketWriter(opGameRequestHennaUnequip)
	w.WriteInt32(symbolID)
	return w.Bytes()
}

// hennaEquipAction draws symbolID.
func hennaEquipAction(symbolID int32) Action {
	return Action{kind: actHennaEquip, payload: encodeRequestHennaEquip(symbolID), item: symbolID}
}

// hennaUnequipAction removes the worn symbolID.
func hennaUnequipAction(symbolID int32) Action {
	return Action{kind: actHennaUnequip, payload: encodeRequestHennaUnequip(symbolID), item: symbolID}
}
