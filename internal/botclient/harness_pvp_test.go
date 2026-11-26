package botclient

import (
	"bytes"
	"testing"
)

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
