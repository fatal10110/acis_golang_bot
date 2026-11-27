package botwire

import (
	"bytes"
	"testing"
)

func TestPartyEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestJoinParty", EncodeRequestJoinParty("GM", 1), []byte{0x29, 'G', 0, 'M', 0, 0, 0, 1, 0, 0, 0}},
		{"RequestAnswerJoinParty accept", EncodeRequestAnswerJoinParty(true), []byte{0x2a, 1, 0, 0, 0}},
		{"RequestAnswerJoinParty decline", EncodeRequestAnswerJoinParty(false), []byte{0x2a, 0, 0, 0, 0}},
		{"RequestWithdrawParty", EncodeRequestWithdrawParty(), []byte{0x2b}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
