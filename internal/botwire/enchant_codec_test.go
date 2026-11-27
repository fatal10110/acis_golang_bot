package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// TestDecodeEnchantPackets decodes ChooseInventoryItem and EnchantResult
// as serverpackets/enchant.go writes them: one int32 each, consumed whole.
func TestDecodeEnchantPackets(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"ChooseInventoryItem D weapon scroll", packet(OpGameChooseInventoryItem, int32(955)), ChooseInventoryItem{ScrollItemID: 955}},
		{"ChooseInventoryItem blessed S armor scroll", packet(OpGameChooseInventoryItem, int32(6578)), ChooseInventoryItem{ScrollItemID: 6578}},
		{"EnchantResult success", packet(OpGameEnchantResult, int32(0)), EnchantResult{Code: EnchantSuccess}},
		{"EnchantResult broken with crystals", packet(OpGameEnchantResult, int32(1)), EnchantResult{Code: EnchantBrokenWithCrystals}},
		{"EnchantResult cancelled", packet(OpGameEnchantResult, int32(2)), EnchantResult{Code: EnchantCancelled}},
		{"EnchantResult unsuccess", packet(OpGameEnchantResult, int32(3)), EnchantResult{Code: EnchantUnsuccess}},
		{"EnchantResult broken", packet(OpGameEnchantResult, int32(4)), EnchantResult{Code: EnchantBrokenNoCrystals}},
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

func TestDecodeEnchantPacketsRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"choose short", []byte{OpGameChooseInventoryItem, 0xbb, 0x03, 0}, wire.ErrShortPacket},
		{"choose empty", []byte{OpGameChooseInventoryItem}, wire.ErrShortPacket},
		{"choose trailing", append(packet(OpGameChooseInventoryItem, int32(955)), 0), ErrTrailingBytes},
		{"result short", []byte{OpGameEnchantResult, 0, 0}, wire.ErrShortPacket},
		{"result trailing", append(packet(OpGameEnchantResult, int32(0)), 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
