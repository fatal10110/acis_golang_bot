package botwire

import (
	"bytes"
	"testing"
)

func TestPartyMatchEncoders(t *testing.T) {
	// The rooms window's filter near me (-2) and my levels (0).
	const nearMe, levelsMine int32 = -2, 0
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestListPartyWaiting", EncodeRequestListPartyWaiting(1, nearMe, levelsMine), []byte{0x6f, 1, 0, 0, 0, 0xfe, 0xff, 0xff, 0xff, 0, 0, 0, 0}},
		{"RequestJoinPartyRoom", EncodeRequestJoinPartyRoom(5, nearMe, levelsMine), []byte{0x71, 5, 0, 0, 0, 0xfe, 0xff, 0xff, 0xff, 0, 0, 0, 0}},
		{
			"RequestManagePartyRoom", EncodeRequestManagePartyRoom(0, 4, 1, 20, 2, "A"),
			[]byte{0x70, 0, 0, 0, 0, 4, 0, 0, 0, 1, 0, 0, 0, 20, 0, 0, 0, 2, 0, 0, 0, 'A', 0, 0, 0},
		},
		{"RequestWithdrawPartyRoom", EncodeRequestWithdrawPartyRoom(5), []byte{0xd0, 0x03, 0x00, 5, 0, 0, 0, 0, 0, 0, 0}},
		{"RequestExitPartyMatchingWaitingRoom", EncodeRequestExitPartyMatchingWaitingRoom(), []byte{0xd0, 0x17, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % x, want % x", tt.got, tt.want)
			}
		})
	}
}
