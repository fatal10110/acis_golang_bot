package botwire

import (
	"bytes"
	"testing"
)

func TestItemEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"UseItem", EncodeUseItem(0x30000001), []byte{0x14, 1, 0, 0, 0x30, 0, 0, 0, 0}},
		{"RequestAutoSoulShot on", EncodeRequestAutoSoulShot(1835, true), []byte{0xd0, 0x05, 0, 0x2b, 0x07, 0, 0, 1, 0, 0, 0}},
		{"RequestAutoSoulShot off", EncodeRequestAutoSoulShot(1835, false), []byte{0xd0, 0x05, 0, 0x2b, 0x07, 0, 0, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

// TestEncodeRequestItemList checks the frame: the opcode alone
// (clientpackets.OpcodeRequestItemList, no payload).
func TestEncodeRequestItemList(t *testing.T) {
	if got, want := EncodeRequestItemList(), []byte{0x0f}; !bytes.Equal(got, want) {
		t.Fatalf("RequestItemList = % x, want % x", got, want)
	}
}

func TestCrystallizeEncoder(t *testing.T) {
	got := EncodeRequestCrystallizeItem(0x10000400, 1)
	if want := []byte{0x72, 0x00, 0x04, 0x00, 0x10, 1, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("payload % x, want % x", got, want)
	}
}
