package botclient

import (
	"bytes"
	"testing"
)

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
