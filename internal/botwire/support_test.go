package botwire

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

// TestDecodeEffects pins the effect icon decoders to the server writers
// (serverpackets/status_effects.go): each decodes its whole frame, and a
// frame one byte short or long is refused.
func TestDecodeEffects(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{
			"AbnormalStatusUpdate timed and toggle",
			packet(OpGameAbnormalStatusUpdate, uint16(2), int32(skillMight), uint16(2), int32(1150), int32(312), uint16(1), int32(-1)),
			AbnormalStatusUpdate{Effects: []Effect{
				{SkillID: skillMight, Level: 2, Left: 1150 * time.Second},
				{SkillID: 312, Level: 1, Permanent: true},
			}},
		},
		{
			"AbnormalStatusUpdate none",
			packet(OpGameAbnormalStatusUpdate, uint16(0)),
			AbnormalStatusUpdate{Effects: []Effect{}},
		},
		{
			"PartySpelled player",
			packet(OpGamePartySpelled, 0, pcID, 2, int32(skillShield), uint16(3), int32(45), int32(312), uint16(1), int32(0)),
			PartySpelled{ObjectID: pcID, Effects: []Effect{
				{SkillID: skillShield, Level: 3, Left: 45 * time.Second},
				{SkillID: 312, Level: 1, Permanent: true},
			}},
		},
		{
			"PartySpelled servitor none",
			packet(OpGamePartySpelled, 2, npcID, 0),
			PartySpelled{Kind: 2, ObjectID: npcID, Effects: []Effect{}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok, err := DecodeEvent(tc.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v", ok, err)
			}
			if !reflect.DeepEqual(ev, tc.want) {
				t.Fatalf("decoded\n got %+v\nwant %+v", ev, tc.want)
			}
			if _, _, err := DecodeEvent(tc.raw[:len(tc.raw)-1]); err == nil {
				t.Error("a frame one byte short decoded")
			}
			if _, _, err := DecodeEvent(append(bytes.Clone(tc.raw), 0)); err == nil {
				t.Error("a frame one byte long decoded")
			}
		})
	}
	for _, raw := range [][]byte{
		packet(OpGameAbnormalStatusUpdate, uint16(0xffff), 1),
		packet(OpGamePartySpelled, 0, pcID, 0x7fffffff, 1),
	} {
		if _, _, err := DecodeEvent(raw); err == nil {
			t.Errorf("% x with a huge count decoded", raw[:2])
		}
	}
}
