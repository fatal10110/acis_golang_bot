package e2e

import (
	"encoding/binary"
	"testing"
)

// etcStatusFrame is an EtcStatusUpdate frame (opcode 0xf3, seven int32s:
// charges, weight penalty, blocked, danger area, grade penalty, charm of
// courage, death penalty level; serverpackets/etcstatusupdate.go).
func etcStatusFrame(weight, deathPenalty int32) []byte {
	raw := []byte{0xf3}
	for _, v := range []int32{0, weight, 0, 0, 0, 0, deathPenalty} {
		raw = binary.LittleEndian.AppendUint32(raw, uint32(v))
	}
	return raw
}

func TestPinnedEntryDeathPenalty(t *testing.T) {
	actionFailed := []byte{0x25}
	tests := []struct {
		name    string
		burst   [][]byte
		level   int32
		ok      bool
		wantErr bool
	}{
		{name: "empty burst", ok: false},
		{name: "no EtcStatusUpdate", burst: [][]byte{actionFailed}, ok: false},
		{name: "no penalty", burst: [][]byte{actionFailed, etcStatusFrame(1, 0)}, level: 0, ok: true},
		{name: "penalty level 3", burst: [][]byte{etcStatusFrame(0, 3), actionFailed}, level: 3, ok: true},
		{name: "the first one counts", burst: [][]byte{etcStatusFrame(0, 2), etcStatusFrame(0, 5)}, level: 2, ok: true},
		{name: "truncated", burst: [][]byte{etcStatusFrame(0, 3)[:20]}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, ok, err := entryDeathPenalty(tt.burst)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if level != tt.level || ok != tt.ok {
				t.Errorf("entryDeathPenalty = %d, %v; want %d, %v", level, ok, tt.level, tt.ok)
			}
		})
	}
}
