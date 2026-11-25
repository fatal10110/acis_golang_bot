package botclient

import (
	"bytes"
	"testing"
)

// TestEncodeAnswerTradeRequest pins the harness's answer to a trade
// request: AnswerTradeRequest 0x44 and the int32 response, 1 accepting and
// 0 declining (clientpackets AnswerTradeRequest).
func TestEncodeAnswerTradeRequest(t *testing.T) {
	tests := []struct {
		name   string
		accept bool
		want   []byte
	}{
		{"accept", true, []byte{0x44, 1, 0, 0, 0}},
		{"decline", false, []byte{0x44, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeAnswerTradeRequest(tt.accept)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("EncodeAnswerTradeRequest(%v) = % x, want % x", tt.accept, got, tt.want)
			}
			if !bytes.Equal(got, encodeAnswerTradeRequest(tt.accept)) {
				t.Errorf("EncodeAnswerTradeRequest(%v) differs from the bot's own answer", tt.accept)
			}
		})
	}
}
