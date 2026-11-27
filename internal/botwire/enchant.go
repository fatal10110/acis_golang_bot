package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// ChooseInventoryItem asks the client to pick the item to enchant with the
// scroll it used: the scroll's template id, not its object id
// (serverpackets/enchant.go FrameChooseInventoryItem).
type ChooseInventoryItem struct{ ScrollItemID int32 }

// EnchantResult closes the enchant window with the outcome of a
// RequestEnchantItem (serverpackets/enchant.go FrameEnchantResult).
type EnchantResult struct{ Code EnchantCode }

// EnchantCode is an EnchantResult's outcome (serverpackets.EnchantResult).
type EnchantCode int32

// EnchantResult codes.
const (
	EnchantSuccess            EnchantCode = 0
	EnchantBrokenWithCrystals EnchantCode = 1
	EnchantCancelled          EnchantCode = 2
	EnchantUnsuccess          EnchantCode = 3
	EnchantBrokenNoCrystals   EnchantCode = 4
)

func (ChooseInventoryItem) IsEvent() {}

func (EnchantResult) IsEvent() {}

// decodeChooseInventoryItem: the scroll's template id as int32.
func decodeChooseInventoryItem(r *wire.Reader) Event {
	return ChooseInventoryItem{ScrollItemID: r.ReadInt32()}
}

// decodeEnchantResult: the result code as int32.
func decodeEnchantResult(r *wire.Reader) Event {
	return EnchantResult{Code: EnchantCode(r.ReadInt32())}
}
