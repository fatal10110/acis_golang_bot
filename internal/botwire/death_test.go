package botwire

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

func TestDecodeConfirmDlg(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want ConfirmDlg
	}{
		// 1510, one text parameter "GM" in UTF-16LE with its terminator.
		{"resurrection bytes", []byte{0xed, 0xe6, 5, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 'G', 0, 'M', 0, 0, 0}, ConfirmDlg{MessageID: 1510, Name: "GM"}},
		{"restore", packet(OpGameConfirmDlg, 332, 1, 0, "cleric"), ConfirmDlg{MessageID: 332, Name: "cleric"}},
		{
			"summon friend with countdown and requester",
			packet(OpGameConfirmDlg, 1842, 2, 0, "GM", 7, 1, 2, 3, 30000, gmID),
			ConfirmDlg{MessageID: 1842, Name: "GM", Time: 30000, RequesterID: gmID},
		},
		{"summon friend bare", packet(OpGameConfirmDlg, 1842, 2, 0, "GM", 7, 1, 2, 3), ConfirmDlg{MessageID: 1842, Name: "GM"}},
		{"gate", packet(OpGameConfirmDlg, 1140, 0, 0, 0), ConfirmDlg{MessageID: 1140}},
		{"second text is not the name", packet(OpGameConfirmDlg, 1, 2, 0, "a", 0, "b"), ConfirmDlg{MessageID: 1, Name: "a"}},
		{"unknown parameter type skips the rest", packet(OpGameConfirmDlg, 1, 2, 0, "a", 9, 1, 2, 3, 4, 5), ConfirmDlg{MessageID: 1, Name: "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v", ok, err)
			}
			if ev != tt.want {
				t.Fatalf("DecodeEvent = %+v, want %+v", ev, tt.want)
			}
		})
	}
}

func TestDecodeConfirmDlgRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"no count", packet(OpGameConfirmDlg, 1510), wire.ErrShortPacket},
		{"count past the frame", packet(OpGameConfirmDlg, 1510, 2, 0, "GM"), wire.ErrShortPacket},
		{"short zone", packet(OpGameConfirmDlg, 1842, 1, 7, 1, 2), wire.ErrShortPacket},
		{"partial tail", append(packet(OpGameConfirmDlg, 1510, 1, 0, "GM"), 1, 2), ErrTrailingBytes},
		{"third tail value", packet(OpGameConfirmDlg, 1140, 0, 0, 0, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
