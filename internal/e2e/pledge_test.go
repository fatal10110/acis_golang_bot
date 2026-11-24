package e2e

import (
	"bytes"
	"testing"
)

// TestEncodeRequestPledgePower checks the privileges set against the
// server's layout (clientpackets/pledge.go DecodeRequestPledgePower): the
// opcode, then Rank, Action 2 and Privs as little-endian int32s, 13 bytes.
func TestEncodeRequestPledgePower(t *testing.T) {
	tests := []struct {
		name        string
		rank, privs int32
		want        []byte
	}{
		{
			name: "warehouse privilege to the member rank", rank: memberRank, privs: privWarehouseSearch,
			want: []byte{0xc0, 6, 0, 0, 0, 2, 0, 0, 0, 8, 0, 0, 0},
		},
		{
			name: "every privilege to rank 1", rank: 1, privs: 8388606,
			want: []byte{0xc0, 1, 0, 0, 0, 2, 0, 0, 0, 0xfe, 0xff, 0x7f, 0},
		},
		{
			name: "none to the academy rank", rank: 9, privs: 0,
			want: []byte{0xc0, 9, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeRequestPledgePower(tt.rank, tt.privs); !bytes.Equal(got, tt.want) {
				t.Fatalf("encodeRequestPledgePower(%d, %d) = % x, want % x", tt.rank, tt.privs, got, tt.want)
			}
		})
	}
}
