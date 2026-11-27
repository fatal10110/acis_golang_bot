package botclient

import "github.com/fatal10110/acis_golang_bot/internal/botwire"

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

// hennaEquipAction draws symbolID.
func hennaEquipAction(symbolID int32) Action {
	return Action{kind: actHennaEquip, payload: botwire.EncodeRequestHennaEquip(symbolID), item: symbolID}
}

// hennaUnequipAction removes the worn symbolID.
func hennaUnequipAction(symbolID int32) Action {
	return Action{kind: actHennaUnequip, payload: botwire.EncodeRequestHennaUnequip(symbolID), item: symbolID}
}
