package botwire

import (
	"bytes"
	"testing"
)

func TestClanEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"accept", EncodeRequestAnswerJoinPledge(true), []byte{0x25, 1, 0, 0, 0}},
		{"decline", EncodeRequestAnswerJoinPledge(false), []byte{0x25, 0, 0, 0, 0}},
		{"invite", EncodeRequestJoinPledge(0x20000002, 0), []byte{0x24, 2, 0, 0, 0x20, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
