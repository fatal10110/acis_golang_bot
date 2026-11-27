package botwire

import (
	"bytes"
	"testing"
)

func TestTradeEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"TradeRequest", EncodeTradeRequest(pcID), []byte{0x15, 2, 0, 0, 0x20}},
		{"AnswerTradeRequest accept", EncodeAnswerTradeRequest(true), []byte{0x44, 1, 0, 0, 0}},
		{"AnswerTradeRequest decline", EncodeAnswerTradeRequest(false), []byte{0x44, 0, 0, 0, 0}},
		{"AddTradeItem", EncodeAddTradeItem(goodsID, 10), []byte{0x16, 0, 0, 0, 0, 1, 0, 0, 0x40, 10, 0, 0, 0}},
		{"TradeDone confirm", EncodeTradeDone(true), []byte{0x17, 1, 0, 0, 0}},
		{"TradeDone cancel", EncodeTradeDone(false), []byte{0x17, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

// TestEncodeAnswerTradeRequest pins the answer to a trade request:
// AnswerTradeRequest 0x44 and the int32 response, 1 accepting and 0
// declining (clientpackets AnswerTradeRequest).
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
		})
	}
}
