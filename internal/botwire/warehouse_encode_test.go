package botwire

import (
	"bytes"
	"testing"
)

// TestWarehouseEncoders pins the request bytes: exactly 4+8*rows after the
// opcode, or the server discards the request in silence.
func TestWarehouseEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{
			"SendWareHouseDepositList", EncodeWarehouseRequest(OpGameSendWareHouseDepositList, []WarehouseRow{{102, animalSkin, 10}, {100, adenaID, 1000}}),
			[]byte{0x31, 2, 0, 0, 0, 102, 0, 0, 0, 10, 0, 0, 0, 100, 0, 0, 0, 0xe8, 3, 0, 0},
		},
		{
			"SendWareHouseWithDrawList", EncodeWarehouseRequest(OpGameSendWareHouseWithdrawList, []WarehouseRow{{0x10000203, 2369, 1}}),
			[]byte{0x32, 1, 0, 0, 0, 3, 2, 0, 0x10, 1, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
		if rows := int(tt.got[1]); len(tt.got) != 1+4+8*rows {
			t.Errorf("%s: %d bytes for %d rows", tt.name, len(tt.got), rows)
		}
	}
}
