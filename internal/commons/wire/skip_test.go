package wire

import (
	"errors"
	"testing"
)

func TestReaderSkip(t *testing.T) {
	tests := []struct {
		name      string
		skip      int
		wantErr   error
		wantRest  int
		wantAfter byte
	}{
		{"within", 2, nil, 1, 3},
		{"to the end", 3, nil, 0, 0},
		{"past the end", 4, ErrShortPacket, 3, 0},
		{"negative", -1, ErrShortPacket, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReader([]byte{1, 2, 3})
			r.Skip(tt.skip)
			if !errors.Is(r.Err(), tt.wantErr) || r.Remaining() != tt.wantRest {
				t.Fatalf("Skip(%d): err %v, remaining %d; want %v, %d", tt.skip, r.Err(), r.Remaining(), tt.wantErr, tt.wantRest)
			}
			if got := r.ReadUint8(); tt.wantErr == nil && tt.wantRest > 0 && got != tt.wantAfter {
				t.Fatalf("byte after Skip = %d, want %d", got, tt.wantAfter)
			}
		})
	}
}
