package botwire

import (
	"bytes"
	"testing"
)

func TestShopEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{
			"RequestBuyItem", EncodeRequestBuyItem(14, []BuyRow{{1060, 18}, {2509, 129}}),
			[]byte{0x1f, 14, 0, 0, 0, 2, 0, 0, 0, 0x24, 4, 0, 0, 18, 0, 0, 0, 0xcd, 9, 0, 0, 129, 0, 0, 0},
		},
		{
			"RequestSellItem", EncodeRequestSellItem(0, []SellRow{{102, 1867, 10}}),
			[]byte{0x1e, 0, 0, 0, 0, 1, 0, 0, 0, 102, 0, 0, 0, 0x4b, 7, 0, 0, 10, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
