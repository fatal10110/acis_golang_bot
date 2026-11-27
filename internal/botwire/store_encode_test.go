package botwire

import (
	"bytes"
	"testing"
)

func TestStoreEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestPrivateStoreManageSell", EncodeRequestPrivateStoreManageSell(), []byte{0x73}},
		{"RequestPrivateStoreQuitSell", EncodeRequestPrivateStoreQuitSell(), []byte{0x76}},
		{"SetPrivateStoreMsgSell", EncodeSetPrivateStoreMsgSell("ab"), []byte{0x77, 'a', 0, 'b', 0, 0, 0}},
		{
			"SetPrivateStoreListSell", EncodeSetPrivateStoreListSell(false, []StoreRow{{102, 10, 7}}),
			[]byte{0x74, 0, 0, 0, 0, 1, 0, 0, 0, 102, 0, 0, 0, 10, 0, 0, 0, 7, 0, 0, 0},
		},
		{
			"SetPrivateStoreListSell packaged", EncodeSetPrivateStoreListSell(true, nil),
			[]byte{0x74, 1, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			"RequestPrivateStoreBuy", EncodeRequestPrivateStoreBuy(selfID, []StoreRow{{107, 1, 700}}),
			[]byte{0x79, 1, 0, 0, 0x10, 1, 0, 0, 0, 107, 0, 0, 0, 1, 0, 0, 0, 0xbc, 2, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
