package botwire

import (
	"bytes"
	"testing"
)

func TestDeathEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"DlgAnswer accept", EncodeDlgAnswer(1510, true, 0), []byte{0xc5, 0xe6, 5, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0}},
		{"DlgAnswer decline echoes requester", EncodeDlgAnswer(332, false, 7), []byte{0xc5, 0x4c, 1, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0}},
		{"RequestUserCommand unstuck", EncodeRequestUserCommand(52), []byte{0xaa, 52, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
