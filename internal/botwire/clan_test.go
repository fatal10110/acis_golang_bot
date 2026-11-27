package botwire

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

const (
	clanID    = int32(0x0a000001)
	otherClan = int32(0x0a000002)
)

// clanRoster writes PledgeShowMemberListAll in the server's order
// (pledgememberlist.go): the header, then per member its row and sponsor
// flag.
func clanRoster(id, pledgeType int32, name, leader string, members ...string) []byte {
	fields := []any{
		wire.BoolInt32(pledgeType != 0), id, pledgeType, name, leader,
		7, 3, 0, 0, 0, 120, 0, 0, 0, "", 0, 0, int32(len(members)),
	}
	for i, m := range members {
		fields = append(fields, m, 20+int32(i), 18, 0, 1, int32(0x10000100)+int32(i), 0)
	}
	return packet(OpGamePledgeShowMemberListAll, fields...)
}

func TestDecodeClanEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Object id 0x30000001, then "Cr" in UTF-16LE with its terminator.
		{"AskJoinPledge bytes", []byte{0x32, 1, 0, 0, 0x30, 'C', 0, 'r', 0, 0, 0}, AskJoinPledge{RequesterID: gmID, Clan: "Cr"}},
		{"AskJoinPledge", packet(OpGameAskJoinPledge, pcID, "Crafters"), AskJoinPledge{RequesterID: pcID, Clan: "Crafters"}},
		{"JoinPledge", packet(OpGameJoinPledge, clanID), JoinPledge{ClanID: clanID}},
		{
			"PledgeShowMemberListAll", clanRoster(clanID, 0, "Crafters", "GM", "GM", "bot7"),
			PledgeShowMemberListAll{ClanID: clanID, Name: "Crafters", Leader: "GM"},
		},
		{
			"PledgeShowMemberListAll sub-unit", clanRoster(clanID, 100, "Guard", "Captain"),
			PledgeShowMemberListAll{ClanID: clanID, PledgeType: 100, Name: "Guard", Leader: "Captain"},
		},
		{"PledgeShowMemberListDeleteAll", packet(OpGamePledgeShowMemberListDeleteAll), PledgeShowMemberListDeleteAll{}},
		{
			"SystemMessage clan invitation", packet(OpGameSystemMessage, msgInvitedToClan, 2, 0, "Alden", 0, "Crafters"),
			SystemMessage{ID: msgInvitedToClan, Text: [2]string{"Alden", "Crafters"}},
		},
		{
			"SystemMessage keeps two texts", packet(OpGameSystemMessage, 9, 4, 1, 5, 0, "a", 0, "b", 0, "c"),
			SystemMessage{ID: 9, Text: [2]string{"a", "b"}, Number: 5},
		},
		{"CreatureSay clan", packet(OpGameCreatureSay, pcID, chatClan, "Alden", "hi all"), CreatureSay{ObjectID: pcID, Type: chatClan, Name: "Alden", Text: "hi all"}},
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

func TestDecodeClanEventsRejects(t *testing.T) {
	roster := clanRoster(clanID, 0, "Crafters", "GM", "GM")
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"AskJoinPledge without clan name", packet(OpGameAskJoinPledge, pcID), wire.ErrShortPacket},
		{"AskJoinPledge trailing", append(packet(OpGameAskJoinPledge, pcID, "Cr"), 0), ErrTrailingBytes},
		{"JoinPledge short", packet(OpGameJoinPledge, uint16(1)), wire.ErrShortPacket},
		{"roster short of its count", roster[:len(roster)-4], wire.ErrShortPacket},
		{"roster count past the frame", packet(OpGamePledgeShowMemberListAll, 0, clanID, 0, "C", "L", 0, 0, 0, 0, 0, 0, 0, 0, 0, "", 0, 0, 0x7fffffff), wire.ErrShortPacket},
		{"roster trailing", append(roster, 0), ErrTrailingBytes},
		{"DeleteAll trailing", packet(OpGamePledgeShowMemberListDeleteAll, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestRecordedClanFixtures checks the clan frames TestLiveClanInvite
// recorded against the live server decode to sane fields: a clan id, names.
// Ids and names vary between runs and are not pinned; TestRecordedFixtures
// checks each decodes exactly.
func TestRecordedClanFixtures(t *testing.T) {
	tests := []struct {
		op   byte
		sane func(Event) bool
	}{
		{OpGameAskJoinPledge, func(ev Event) bool {
			e, ok := ev.(AskJoinPledge)
			return ok && e.RequesterID != 0 && e.Clan != ""
		}},
		{OpGameJoinPledge, func(ev Event) bool {
			e, ok := ev.(JoinPledge)
			return ok && e.ClanID != 0
		}},
		{OpGamePledgeShowMemberListAll, func(ev Event) bool {
			e, ok := ev.(PledgeShowMemberListAll)
			return ok && e.ClanID != 0 && e.PledgeType == 0 && e.Name != "" && e.Leader != ""
		}},
		{OpGamePledgeShowMemberListDeleteAll, func(ev Event) bool {
			_, ok := ev.(PledgeShowMemberListDeleteAll)
			return ok
		}},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%02x.bin", tt.op)
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "recorded", name))
			if err != nil {
				t.Fatal(err)
			}
			ev, ok, err := DecodeEvent(raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", ev, ok, err)
			}
			if !tt.sane(ev) {
				t.Errorf("decoded %+v, want non-zero ids and non-empty names", ev)
			}
		})
	}
}
