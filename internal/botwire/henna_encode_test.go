package botwire

import (
	"bytes"
	"testing"
)

// TestHennaEncoders pins the request bytes: the opcode and one int32, the
// symbol id, as clientpackets/henna.go decodeHennaInt reads them.
func TestHennaEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestHennaEquip", EncodeRequestHennaEquip(6), []byte{0xbc, 6, 0, 0, 0}},
		{"RequestHennaEquip high id", EncodeRequestHennaEquip(0x0125), []byte{0xbc, 0x25, 1, 0, 0}},
		{"RequestHennaUnequip", EncodeRequestHennaUnequip(37), []byte{0xbf, 37, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("payload % x, want % x", tt.got, tt.want)
			}
		})
	}
}
