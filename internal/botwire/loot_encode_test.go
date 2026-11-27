package botwire

import (
	"bytes"
	"testing"
)

func TestEncodeRequestDropItem(t *testing.T) {
	got := EncodeRequestDropItem(0x40000003, 25, Point{X: 1, Y: -2, Z: 300})
	want := []byte{0x12, 3, 0, 0, 0x40, 25, 0, 0, 0, 1, 0, 0, 0, 0xfe, 0xff, 0xff, 0xff, 0x2c, 1, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeRequestDropItem = % x, want % x", got, want)
	}
}
