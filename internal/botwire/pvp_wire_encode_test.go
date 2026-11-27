package botwire

import (
	"bytes"
	"testing"
)

// The requests encode the server's readImpl field order: the payloads the
// server's own decoder tests read (clientpackets/duel_test.go; RequestTargetCancel
// clientpackets/targetaction.go, one uint16).
func TestPvPWireEncode(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"RequestDuelStart party", EncodeRequestDuelStart("Ab", true), []byte{0xd0, 0x27, 0x00, 'A', 0, 'b', 0, 0, 0, 1, 0, 0, 0}},
		{"RequestDuelStart", EncodeRequestDuelStart("Ab", false), []byte{0xd0, 0x27, 0x00, 'A', 0, 'b', 0, 0, 0, 0, 0, 0, 0}},
		{"RequestDuelAnswerStart accepted", EncodeRequestDuelAnswerStart(false, true), []byte{0xd0, 0x28, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0}},
		{"RequestDuelAnswerStart party declined", EncodeRequestDuelAnswerStart(true, false), []byte{0xd0, 0x28, 0x00, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"RequestTargetCancel", EncodeRequestTargetCancel(), []byte{0x37, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("encoded % x, want % x", tt.got, tt.want)
			}
		})
	}
}

// TestHarnessDuelEncoder checks the exported duel challenge against the
// server's layout (clientpackets/duel.go RequestDuelStart: 0xd0 sub 0x0027,
// the name, the party flag).
func TestHarnessDuelEncoder(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"one on one", EncodeRequestDuelStart("Ab", false), []byte{0xd0, 0x27, 0x00, 'A', 0, 'b', 0, 0, 0, 0, 0, 0, 0}},
		{"party", EncodeRequestDuelStart("Ab", true), []byte{0xd0, 0x27, 0x00, 'A', 0, 'b', 0, 0, 0, 1, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("% x, want % x", tt.got, tt.want)
			}
		})
	}
}
