package botwire

import (
	"bytes"
	"testing"
)

// TestEncodeChannelAsks checks the asks against the server's decoders
// (clientpackets/party.go DecodeRequestExMPCCShowPartyMembersInfo: 0xd0,
// sub-opcode 0x0026, leader id; RequestUserCommand: 0xaa, command id).
func TestEncodeChannelAsks(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"members", EncodeRequestExMPCCShowPartyMembersInfo(0x01020304), []byte{0xd0, 0x26, 0x00, 0x04, 0x03, 0x02, 0x01}},
		{"overview", EncodeRequestUserCommand(97), []byte{0xaa, 97, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
