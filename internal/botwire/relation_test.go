package botwire

import (
	"bytes"
	"reflect"
	"testing"
)

// TestRelationChangedBits decodes RelationChanged frames laid out as the
// server writes them (serverpackets/relationchanged.go: opcode 0xce, object
// id, relation, auto-attackable, karma, PvP flag, each an int32) into their
// relation bits (docs/pvp/war.md, D40): a mutual war carries both war bits
// (network/relations.go relationTo), a one-sided war only 0x10000. The
// decoder consumes exactly len(payload).
func TestRelationChangedBits(t *testing.T) {
	tests := []struct {
		name   string
		raw    []byte
		want   RelationChanged
		mutual bool
	}{
		{
			"mutual war, clean",
			[]byte{
				0xce,
				0x02, 0x00, 0x00, 0x20, // object id 0x20000002
				0x00, 0x80, 0x01, 0x00, // relation 0x18000: MutualWar | OneSidedWar
				0x00, 0x00, 0x00, 0x00, // not auto-attackable
				0x00, 0x00, 0x00, 0x00, // karma 0
				0x00, 0x00, 0x00, 0x00, // PvP flag 0
			},
			RelationChanged{ObjectID: pcID, Relation: 0x18000},
			true,
		},
		{
			"mutual war bit alone",
			[]byte{
				0xce,
				0x02, 0x00, 0x00, 0x20,
				0x00, 0x80, 0x00, 0x00, // relation 0x8000
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
			RelationChanged{ObjectID: pcID, Relation: 0x8000},
			true,
		},
		{
			"one-sided war, flagged",
			[]byte{
				0xce,
				0x02, 0x00, 0x00, 0x20,
				0x02, 0x00, 0x01, 0x00, // relation 0x10002: OneSidedWar | PvPFlag
				0x01, 0x00, 0x00, 0x00, // auto-attackable
				0x00, 0x00, 0x00, 0x00,
				0x01, 0x00, 0x00, 0x00, // PvP flag 1
			},
			RelationChanged{ObjectID: pcID, Relation: 0x10002, AutoAttackable: true, PvPFlag: 1},
			false,
		},
		{
			"no war",
			packet(OpGameRelationChanged, pcID, 0x80, 0, 0, 0),
			RelationChanged{ObjectID: pcID, Relation: 0x80},
			false,
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
			if got := ev.(RelationChanged).MutualWar(); got != tc.mutual {
				t.Errorf("MutualWar = %v, want %v", got, tc.mutual)
			}
			if _, _, err := DecodeEvent(tc.raw[:len(tc.raw)-1]); err == nil {
				t.Error("a frame one byte short decoded")
			}
			if _, _, err := DecodeEvent(append(bytes.Clone(tc.raw), 0)); err == nil {
				t.Error("a frame one byte long decoded")
			}
		})
	}
}
