package botwire

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

// TestDecodeCombatEvents pins the combat decoders to the server writers
// (serverpackets attack.go, magic_skill.go, skilllist.go, skillcooltime.go,
// relationchanged.go): each decodes its whole frame, and a frame one byte
// short or long is refused.
func TestDecodeCombatEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{
			"Attack one hit",
			packet(OpGameAttack, npcID, selfID, 37, HitCritical, 10, 20, 30, uint16(0)),
			Attack{AttackerID: npcID, Pos: Point{X: 10, Y: 20, Z: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 37, Flags: HitCritical}}},
		},
		{
			"Attack three hits",
			packet(OpGameAttack, selfID, npcID, 5, byte(0), 1, 2, 3, uint16(2), pcID, 0, HitMiss, int32(7), 9, HitShield|HitSoulshot),
			Attack{AttackerID: selfID, Pos: Point{X: 1, Y: 2, Z: 3}, Hits: []AttackHit{
				{TargetID: npcID, Damage: 5}, {TargetID: pcID, Flags: HitMiss}, {TargetID: 7, Damage: 9, Flags: HitShield | HitSoulshot},
			}},
		},
		{
			"MagicSkillUse",
			packet(OpGameMagicSkillUse, selfID, npcID, 1177, 3, 1500, 4000, 1, 2, 3, 0, 4, 5, 6),
			MagicSkillUse{
				CasterID: selfID, TargetID: npcID, SkillID: 1177, Level: 3, HitTime: 1500 * time.Millisecond, Reuse: 4 * time.Second,
				CasterPos: Point{X: 1, Y: 2, Z: 3}, TargetPos: Point{X: 4, Y: 5, Z: 6},
			},
		},
		{
			"MagicSkillUse with the success flag",
			packet(OpGameMagicSkillUse, npcID, selfID, 4001, 1, 0, 0, 1, 2, 3, 1, uint16(0), 4, 5, 6),
			MagicSkillUse{CasterID: npcID, TargetID: selfID, SkillID: 4001, Level: 1, CasterPos: Point{X: 1, Y: 2, Z: 3}, TargetPos: Point{X: 4, Y: 5, Z: 6}},
		},
		{
			"MagicSkillLaunched with targets",
			packet(OpGameMagicSkillLaunched, selfID, 1177, 3, 2, npcID, pcID),
			MagicSkillLaunched{CasterID: selfID, SkillID: 1177, Level: 3, Targets: []int32{npcID, pcID}},
		},
		{
			"MagicSkillLaunched with no target",
			packet(OpGameMagicSkillLaunched, selfID, 1177, 3, 0, 0),
			MagicSkillLaunched{CasterID: selfID, SkillID: 1177, Level: 3},
		},
		{
			"SkillList",
			packet(OpGameSkillList, 2, 0, 3, 1177, byte(0), 1, 1, 141, byte(1)),
			SkillList{Skills: []KnownSkill{{ID: 1177, Level: 3}, {ID: 141, Level: 1, Passive: true, Disabled: true}}},
		},
		{
			"SkillList empty",
			packet(OpGameSkillList, 0),
			SkillList{Skills: []KnownSkill{}},
		},
		{
			"SkillCoolTime",
			packet(OpGameSkillCoolTime, 1, 1177, 3, 6, 4),
			SkillCoolTime{Timers: []SkillTimer{{SkillID: 1177, Level: 3, Reuse: 6 * time.Second, Remaining: 4 * time.Second}}},
		},
		{
			"RelationChanged",
			packet(OpGameRelationChanged, pcID, 0x2, 1, 120, 1),
			RelationChanged{ObjectID: pcID, AutoAttackable: true, Karma: 120, PvPFlag: 1},
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
}

// TestDecodeCombatHugeCount checks that a count off the wire sizes nothing
// past the frame: the decoder fails on the short read instead.
func TestDecodeCombatHugeCount(t *testing.T) {
	for _, raw := range [][]byte{
		packet(OpGameSkillList, 0x7fffffff),
		packet(OpGameSkillCoolTime, 0x7fffffff, 1),
		packet(OpGameMagicSkillLaunched, 1, 2, 3, 0x7fffffff),
		packet(OpGameAttack, 1, 2, 3, byte(0), 1, 2, 3, uint16(0xffff)),
	} {
		if _, _, err := DecodeEvent(raw); err == nil {
			t.Errorf("% x decoded", raw[:2])
		}
	}
}
