package botmetrics

import (
	"maps"
	"testing"
)

// TestMetricsDecodeErrors: DecodeErrors and LoginDecodeError count per bot
// and sum in the snapshot by opcode, only the opcodes seen, apart from the
// aborts; a nil Bot records nothing.
func TestMetricsDecodeErrors(t *testing.T) {
	r := NewRegistry()
	if got := r.Last().DecodeErrors; got == nil || len(got) != 0 {
		t.Fatalf("Last before a collection: decode errors %v, want an empty map", got)
	}
	a, b := r.NewBot(), r.NewBot()
	a.DecodeErrors(0x04, 0, 1)
	b.DecodeErrors(0x04, 7, 2) // the sub-opcode of a non-extended frame is ignored
	b.DecodeErrors(0x06, 0, 0)
	a.DecodeErrors(0xfe, 0x0012, 3)
	a.DecodeErrors(0xfe, 0x0100, 1)
	b.LoginDecodeError(0x00)
	a.LoginDecodeError(0x07)
	var none *Bot
	none.DecodeErrors(0x04, 0, 1)
	none.LoginDecodeError(0x00)
	s := r.Collect(t0)
	want := map[string]uint64{"0x04": 3, "0xfe:0x0012": 3, "0xfe:other": 1, "login:0x00": 1, "login:0x07": 1}
	if !maps.Equal(s.DecodeErrors, want) || s.Aborts != 0 {
		t.Fatalf("decode errors %v, aborts %d; want %v, 0", s.DecodeErrors, s.Aborts, want)
	}
}
