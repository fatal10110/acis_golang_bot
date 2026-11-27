package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

const (
	gmID   = int32(0x30000001)
	mateID = int32(0x30000002)
)

var (
	gmRow   = PartyMember{ObjectID: gmID, Name: "GM", CP: 100, MaxCP: 200, HP: 300, MaxHP: 400, MP: 50, MaxMP: 60, Level: 40, ClassID: 88}
	mateRow = PartyMember{ObjectID: mateID, Name: "mate", CP: 1, MaxCP: 2, HP: 3, MaxHP: 4, MP: 5, MaxMP: 6, Level: 7, ClassID: 18}
)

// vitals are the fields of a party window row, in the server's order
// (writePartyMemberVitals).
func vitals(m PartyMember) []any {
	return []any{m.ObjectID, m.Name, m.CP, m.MaxCP, m.HP, m.MaxHP, m.MP, m.MaxMP, m.Level, m.ClassID}
}

func partyWindowAll(leader, loot int32, races []int32, members ...PartyMember) []byte {
	fields := []any{leader, loot, int32(len(members))}
	for i, m := range members {
		fields = append(append(fields, vitals(m)...), 0, races[i])
	}
	return packet(OpGamePartySmallWindowAll, fields...)
}

func TestDecodePartyEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// "GM" in UTF-16LE with its terminator, then loot rule 1.
		{"AskJoinParty bytes", []byte{0x39, 'G', 0, 'M', 0, 0, 0, 1, 0, 0, 0}, AskJoinParty{Requester: "GM", Loot: 1}},
		{"AskJoinParty", packet(OpGameAskJoinParty, "leader", 2), AskJoinParty{Requester: "leader", Loot: 2}},
		{"JoinParty", packet(OpGameJoinParty, 1), JoinParty{Response: 1}},
		{
			"PartySmallWindowAll", partyWindowAll(gmID, 1, []int32{0, 3}, gmRow, mateRow),
			PartySmallWindowAll{LeaderID: gmID, Loot: 1, Members: []PartyMember{gmRow, mateRow}},
		},
		{"PartySmallWindowAll alone", partyWindowAll(selfID, 0, nil), PartySmallWindowAll{LeaderID: selfID}},
		{
			"PartySmallWindowAdd", packet(OpGamePartySmallWindowAdd, append(append([]any{gmID, 2}, vitals(mateRow)...), 0, 0)...),
			PartySmallWindowAdd{LeaderID: gmID, Loot: 2, Member: mateRow},
		},
		{"PartySmallWindowUpdate", packet(OpGamePartySmallWindowUpdate, vitals(gmRow)...), PartySmallWindowUpdate{Member: gmRow}},
		{"PartySmallWindowDelete", packet(OpGamePartySmallWindowDelete, mateID, "mate"), PartySmallWindowDelete{ObjectID: mateID, Name: "mate"}},
		{"PartySmallWindowDeleteAll", packet(OpGamePartySmallWindowDeleteAll), PartySmallWindowDeleteAll{}},
		{
			"PartyMemberPosition", packet(OpGamePartyMemberPosition, 2, selfID, 1, 2, 3, gmID, -4, 5, -6),
			PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: selfID, Pos: Point{1, 2, 3}}, {ObjectID: gmID, Pos: Point{-4, 5, -6}}}},
		},
		{"PartyMemberPosition empty", packet(OpGamePartyMemberPosition, 0), PartyMemberPosition{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodePartyEventsRejects(t *testing.T) {
	all := partyWindowAll(gmID, 1, []int32{0}, gmRow)
	add := packet(OpGamePartySmallWindowAdd, append(append([]any{gmID, 2}, vitals(mateRow)...), 0, 0)...)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"AskJoinParty without loot", packet(OpGameAskJoinParty, "GM"), wire.ErrShortPacket},
		{"WindowAll short of its count", packet(OpGamePartySmallWindowAll, append([]any{gmID, 1, 2}, vitals(gmRow)...)...), wire.ErrShortPacket},
		{"WindowAll count past the frame", packet(OpGamePartySmallWindowAll, gmID, 1, 0x7fffffff), wire.ErrShortPacket},
		{"WindowAll trailing", append(all, 0), ErrTrailingBytes},
		{"WindowAdd without its zeros", add[:len(add)-8], wire.ErrShortPacket},
		{"DeleteAll trailing", packet(OpGamePartySmallWindowDeleteAll, 0), ErrTrailingBytes},
		{"MemberPosition short row", packet(OpGamePartyMemberPosition, 1, gmID, 1, 2), wire.ErrShortPacket},
		{"MemberPosition trailing", packet(OpGamePartyMemberPosition, 0, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
