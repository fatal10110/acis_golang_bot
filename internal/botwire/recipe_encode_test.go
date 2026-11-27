package botwire

import (
	"bytes"
	"testing"
)

// TestRecipeEncoders pins the request bytes: the opcode and one int32, as
// clientpackets/recipe.go decodeRecipeInt reads them.
func TestRecipeEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestRecipeBookOpen dwarven", EncodeRequestRecipeBookOpen(true), []byte{0xac, 0, 0, 0, 0}},
		{"RequestRecipeBookOpen common", EncodeRequestRecipeBookOpen(false), []byte{0xac, 1, 0, 0, 0}},
		{"RequestRecipeItemMakeSelf", EncodeRequestRecipeItemMakeSelf(20), []byte{0xaf, 20, 0, 0, 0}},
		{"RequestRecipeItemMakeSelf high id", EncodeRequestRecipeItemMakeSelf(0x0102), []byte{0xaf, 2, 1, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("payload % x, want % x", tt.got, tt.want)
			}
		})
	}
}
