package botwire

import (
	"bytes"
	"reflect"
	"testing"
)

// TestUserInfoSiegeRelation decodes UserInfo's relation field into the
// bot's own siege state (userinfo.go userInfoRelation: attacker 0x180,
// defender 0x80, the clan leader bit 0x40 beside them) and RelationChanged's
// siege bits into the record (relationchanged.go: InSiege 0x200, Attacker
// 0x400, Ally 0x800, Enemy 0x1000; docs/pvp/siege.md). Each decoder
// consumes exactly len(payload).
func TestUserInfoSiegeRelation(t *testing.T) {
	users := []struct {
		name     string
		relation int32
		side     SiegeSide
	}{
		{"no siege", 0, SiegeNone},
		{"clan leader, no siege", 0x40, SiegeNone},
		{"attacker", 0x180, SiegeAttacker},
		{"defender", 0x80, SiegeDefender},
		{"attacking clan leader", 0x1c0, SiegeAttacker},
		{"defending clan leader", 0xc0, SiegeDefender},
	}
	for _, tc := range users {
		t.Run("UserInfo "+tc.name, func(t *testing.T) {
			u := testUserInfo
			u.Relation = tc.relation
			raw := userInfoPacket(u, 120, 80, 1.3, 0)
			ev, ok, err := DecodeEvent(raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v", ok, err)
			}
			got := ev.(UserInfo)
			if got.Relation != tc.relation {
				t.Fatalf("Relation = %#x, want %#x", got.Relation, tc.relation)
			}
			if s := got.SiegeSide(); s != tc.side {
				t.Errorf("SiegeSide = %d, want %d", s, tc.side)
			}
			if _, _, err := DecodeEvent(raw[:len(raw)-1]); err == nil {
				t.Error("a frame one byte short decoded")
			}
		})
	}
	relations := []struct {
		name string
		raw  []byte
		want RelationChanged
	}{
		{
			"an enemy attacker",
			[]byte{
				0xce,
				0x02, 0x00, 0x00, 0x20, // object id 0x20000002
				0x00, 0x16, 0x00, 0x00, // relation 0x1600: InSiege | Attacker | Enemy
				0x01, 0x00, 0x00, 0x00, // auto-attackable
				0x00, 0x00, 0x00, 0x00, // karma 0
				0x00, 0x00, 0x00, 0x00, // PvP flag 0
			},
			RelationChanged{ObjectID: pcID, Relation: RelationInSiege | RelationAttacker | RelationEnemy, AutoAttackable: true},
		},
		{
			"an allied defender",
			packet(OpGameRelationChanged, pcID, 0xa00, 0, 0, 0),
			RelationChanged{ObjectID: pcID, Relation: RelationInSiege | RelationAlly},
		},
		{
			"a defending clan leader",
			packet(OpGameRelationChanged, pcID, 0x1280, 1, 0, 0),
			RelationChanged{ObjectID: pcID, Relation: RelationInSiege | RelationEnemy | 0x80, AutoAttackable: true},
		},
	}
	for _, tc := range relations {
		t.Run("RelationChanged "+tc.name, func(t *testing.T) {
			ev, ok, err := DecodeEvent(tc.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v", ok, err)
			}
			if !reflect.DeepEqual(ev, tc.want) {
				t.Fatalf("decoded\n got %+v\nwant %+v", ev, tc.want)
			}
		})
	}
}

// TestSiegeListDecode decodes the siege lists laid out as the server
// writes them (serverpackets/siege.go FrameSiegeAttackerList,
// FrameSiegeDefenderList: castle id, 0, 1, 0, count twice, then per clan
// its id, name, leader, crest id, 0, the side code in the defender list
// only, ally id, ally name, "", ally crest id, strings as UTF-16LE closed by
// a zero character). Each decoder consumes exactly len(payload).
func TestSiegeListDecode(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{
			"attackers of castle 5, one clan",
			[]byte{
				0xca,
				0x05, 0x00, 0x00, 0x00, // castle id 5
				0x00, 0x00, 0x00, 0x00, // 0
				0x01, 0x00, 0x00, 0x00, // 1
				0x00, 0x00, 0x00, 0x00, // 0
				0x01, 0x00, 0x00, 0x00, // count 1
				0x01, 0x00, 0x00, 0x00, // count 1 again
				0x01, 0x00, 0x00, 0x0a, // clan id 0x0a000001
				'A', 0, 0, 0, // name "A"
				'L', 0, 0, 0, // leader "L"
				0x07, 0x00, 0x00, 0x00, // crest id 7
				0x00, 0x00, 0x00, 0x00, // 0
				0x00, 0x00, 0x00, 0x00, // ally id 0
				0, 0, // ally name ""
				0, 0, // ""
				0x00, 0x00, 0x00, 0x00, // ally crest id 0
			},
			SiegeAttackerList{CastleID: 5, Clans: []SiegeClan{{ClanID: 0x0a000001, Name: "A", Leader: "L", CrestID: 7}}},
		},
		{
			"attackers, none",
			packet(OpGameSiegeAttackerList, 9, 0, 1, 0, 0, 0),
			SiegeAttackerList{CastleID: 9, Clans: []SiegeClan{}},
		},
		{
			"defenders of castle 2, the owner then a waiting clan",
			packet(OpGameSiegeDefenderList, 2, 0, 1, 0, 2, 2,
				0x0a000002, "Own", "Lord", 0, 0, SiegeDefenderOwner, 0x0b000001, "Al", "", 3,
				0x0a000003, "W", "X", 0, 0, SiegeDefenderPending, 0, "", "", 0),
			SiegeDefenderList{CastleID: 2, Clans: []SiegeClan{
				{ClanID: 0x0a000002, Name: "Own", Leader: "Lord", Side: SiegeDefenderOwner, AllyID: 0x0b000001, AllyName: "Al", AllyCrest: 3},
				{ClanID: 0x0a000003, Name: "W", Leader: "X", Side: SiegeDefenderPending},
			}},
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

// TestSiegeEncoders checks the list requests byte for byte
// (clientpackets/siege.go DecodeRequestSiegeList: the opcode, then the
// castle id as an int32).
func TestSiegeEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"RequestSiegeAttackerList", EncodeRequestSiegeAttackerList(7), []byte{0xa2, 7, 0, 0, 0}},
		{"RequestSiegeDefenderList", EncodeRequestSiegeDefenderList(9), []byte{0xa3, 9, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
