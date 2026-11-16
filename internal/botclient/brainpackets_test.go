package botclient

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// le lays out values as the client writes them: int32 and uint16 little
// endian, a byte as is, a string as NUL-terminated UTF-16LE.
func le(vals ...any) []byte {
	var b []byte
	for _, v := range vals {
		switch v := v.(type) {
		case int32:
			b = binary.LittleEndian.AppendUint32(b, uint32(v))
		case int:
			b = binary.LittleEndian.AppendUint32(b, uint32(v))
		case byte:
			b = append(b, v)
		case string:
			for _, u := range utf16.Encode([]rune(v)) {
				b = binary.LittleEndian.AppendUint16(b, u)
			}
			b = append(b, 0, 0)
		default:
			panic("le: unsupported value")
		}
	}
	return b
}

// TestBrainPacketEncoders pins every in-world client packet to the field
// order of the server's decoder (internal/gameserver/network/clientpackets).
func TestBrainPacketEncoders(t *testing.T) {
	to, from := Point{X: 100, Y: -200, Z: -3000}, Point{X: -1, Y: 2, Z: 3}
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			"MoveBackwardToLocation", encodeMoveBackwardToLocation(to, from),
			le(byte(0x01), 100, -200, -3000, -1, 2, 3, 1),
		},
		{
			"Action", encodeAction(0x10203040, from),
			le(byte(0x04), 0x10203040, -1, 2, 3, byte(0)),
		},
		{
			"RequestActionUse sit/stand", encodeRequestActionUse(actionSitStand),
			le(byte(0x45), 0, 0, byte(0)),
		},
		{
			"RequestSocialAction", encodeRequestSocialAction(7),
			le(byte(0x1b), 7),
		},
		{
			"Say2 all", encodeSay2("hi there", chatAll),
			le(byte(0x38), "hi there", 0),
		},
		{
			"ValidatePosition", encodeValidatePosition(to, 49152),
			le(byte(0x48), 100, -200, -3000, 49152, 0),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !bytes.Equal(tc.got, tc.want) {
				t.Fatalf("payload\n got % x\nwant % x", tc.got, tc.want)
			}
		})
	}
}
