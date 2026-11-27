package botwire

import (
	"bytes"
	"reflect"
	"testing"
)

// TestDecodeEtcStatusUpdate pins the decoder to the server writer
// (serverpackets/etcstatusupdate.go): seven int32s, the force charges first,
// the weight band second, the grade penalty fifth and the death penalty
// last, the whole frame consumed, a frame one byte
// short or long refused.
func TestDecodeEtcStatusUpdate(t *testing.T) {
	raw := packet(OpGameEtcStatusUpdate, 2, 2, 1, 0, 1, 0, 3)
	ev, ok, err := DecodeEvent(raw)
	if err != nil || !ok {
		t.Fatalf("DecodeEvent = %v, %v", ok, err)
	}
	if want := (EtcStatusUpdate{Charges: 2, WeightPenalty: 2, GradePenalty: 1, DeathPenalty: 3}); !reflect.DeepEqual(ev, want) {
		t.Fatalf("decoded %+v, want %+v", ev, want)
	}
	if _, _, err := DecodeEvent(raw[:len(raw)-1]); err == nil {
		t.Error("a frame one byte short decoded")
	}
	if _, _, err := DecodeEvent(append(bytes.Clone(raw), 0)); err == nil {
		t.Error("a frame one byte long decoded")
	}
}

// TestDecodeDeathPenaltyMessage pins the level message to the server
// writer (FrameSystemMessageNumber): one number parameter, the new level.
func TestDecodeDeathPenaltyMessage(t *testing.T) {
	ev, ok, err := DecodeEvent(packet(OpGameSystemMessage, msgDeathPenaltyLevelAdded, 1, sysParamNumber, 4))
	if err != nil || !ok {
		t.Fatalf("DecodeEvent = %v, %v", ok, err)
	}
	if want := (SystemMessage{ID: msgDeathPenaltyLevelAdded, Number: 4}); !reflect.DeepEqual(ev, want) {
		t.Fatalf("decoded %+v, want %+v", ev, want)
	}
	ev, _, err = DecodeEvent(packet(OpGameSystemMessage, 1, 2, sysParamNumber, 7, sysParamNumber, 8))
	if want := (SystemMessage{ID: 1, Number: 7}); err != nil || !reflect.DeepEqual(ev, want) {
		t.Fatalf("decoded %+v, %v, want the first number %+v", ev, err, want)
	}
}
