package botwire

import (
	"errors"
	"testing"
)

// olympiadFrames are the Olympiad frames a competitor gets, each byte for
// byte as the server writes it (serverpackets/olympiad_match.go
// FrameExOlympiadMode, FrameExOlympiadMatchEnd, FrameExOlympiadUserInfo;
// network/olympiad_match.go olympiadMessageFrame with systemmessage.go
// FrameSystemMessageNumber and FrameSystemMessageStringNumber): the
// extended opcode 0xfe, the little-endian sub-opcode, then each field, a
// string as UTF-16LE closed by a zero character.
func olympiadFrames() []struct {
	name string
	raw  []byte
	want Event
} {
	return []struct {
		name string
		raw  []byte
		want Event
	}{
		{"ExOlympiadMode side 2", []byte{0xfe, 0x2b, 0x00, 0x02}, ExOlympiadMode{Mode: OlympiadModeSide2}},
		{"ExOlympiadMode none", []byte{0xfe, 0x2b, 0x00, 0x00}, ExOlympiadMode{Mode: OlympiadModeNone}},
		{"ExOlympiadMatchEnd", []byte{0xfe, 0x2c, 0x00}, ExOlympiadMatchEnd{}},
		{
			"ExOlympiadUserInfo",
			[]byte{
				0xfe, 0x29, 0x00,
				0x01,                   // side
				0x02, 0x00, 0x00, 0x10, // object id
				'M', 0, 'a', 0, 0, 0, // name
				0x58, 0, 0, 0, // class id 88
				0x2c, 0x01, 0, 0, // HP 300
				0x9a, 0x01, 0, 0, // max HP 410
				0x5a, 0, 0, 0, // CP 90
				0xc8, 0, 0, 0, // max CP 200
			},
			ExOlympiadUserInfo{Side: 1, ObjectID: 0x10000002, Name: "Ma", ClassID: 88, HP: 300, MaxHP: 410, CP: 90, MaxCP: 200},
		},
		{
			"SystemMessage 1492 stadium in 30 s",
			[]byte{0x64, 0xd4, 0x05, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 30, 0, 0, 0},
			SystemMessage{ID: 1492, Number: 30},
		},
		{
			"SystemMessage 1657 gained points",
			[]byte{0x64, 0x79, 0x06, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 'M', 0, 0, 0, 1, 0, 0, 0, 6, 0, 0, 0},
			SystemMessage{ID: 1657, Text: [2]string{"M"}, Number: 6},
		},
	}
}

// TestOlympiadFrames decodes each frame, and refuses it one byte short or
// one byte long: each decoder consumes exactly len(payload).
func TestOlympiadFrames(t *testing.T) {
	for _, tt := range olympiadFrames() {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if got != tt.want {
				t.Fatalf("DecodeEvent = %#v, want %#v", got, tt.want)
			}
			// A body one byte short fails; a bodiless frame one byte short
			// has half a sub-opcode, which is dropped.
			if ev, ok, err := DecodeEvent(tt.raw[:len(tt.raw)-1]); err == nil && ok {
				t.Fatalf("short frame decoded as %#v", ev)
			}
			long := append(append([]byte(nil), tt.raw...), 0)
			if _, _, err := DecodeEvent(long); !errors.Is(err, ErrTrailingBytes) {
				t.Fatalf("long frame error = %v, want %v", err, ErrTrailingBytes)
			}
		})
	}
}
