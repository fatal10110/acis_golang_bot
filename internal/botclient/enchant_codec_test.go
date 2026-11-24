package botclient

import (
	"bytes"
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
		{"ChooseInventoryItem D weapon scroll", packet(opGameChooseInventoryItem, int32(955)), ChooseInventoryItem{ScrollItemID: 955}},
		{"ChooseInventoryItem blessed S armor scroll", packet(opGameChooseInventoryItem, int32(6578)), ChooseInventoryItem{ScrollItemID: 6578}},
		{"EnchantResult success", packet(opGameEnchantResult, int32(0)), EnchantResult{Code: EnchantSuccess}},
		{"EnchantResult broken with crystals", packet(opGameEnchantResult, int32(1)), EnchantResult{Code: EnchantBrokenWithCrystals}},
		{"EnchantResult cancelled", packet(opGameEnchantResult, int32(2)), EnchantResult{Code: EnchantCancelled}},
		{"EnchantResult unsuccess", packet(opGameEnchantResult, int32(3)), EnchantResult{Code: EnchantUnsuccess}},
		{"EnchantResult broken", packet(opGameEnchantResult, int32(4)), EnchantResult{Code: EnchantBrokenNoCrystals}},
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

func TestDecodeEnchantPacketsRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"choose short", []byte{opGameChooseInventoryItem, 0xbb, 0x03, 0}, wire.ErrShortPacket},
		{"choose empty", []byte{opGameChooseInventoryItem}, wire.ErrShortPacket},
		{"choose trailing", append(packet(opGameChooseInventoryItem, int32(955)), 0), errTrailingBytes},
		{"result short", []byte{opGameEnchantResult, 0, 0}, wire.ErrShortPacket},
		{"result trailing", append(packet(opGameEnchantResult, int32(0)), 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestEncodeRequestEnchantItem pins the request bytes: the opcode and the
// target's object id as int32, as clientpackets/item_ops.go
// DecodeRequestEnchantItem reads them (clientpackets_item_trade_test.go).
func TestEncodeRequestEnchantItem(t *testing.T) {
	tests := []struct {
		name string
		obj  int32
		want []byte
	}{
		{"small id", 7, []byte{0x58, 7, 0, 0, 0}},
		{"world object id", 0x10000123, []byte{0x58, 0x23, 0x01, 0x00, 0x10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeRequestEnchantItem(tt.obj); !bytes.Equal(got, tt.want) {
				t.Fatalf("payload % x, want % x", got, tt.want)
			}
		})
	}
}
