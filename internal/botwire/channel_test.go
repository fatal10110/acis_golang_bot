package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Channel fixtures: another party of the bot's channel, led by mateLeader,
// with pcID in it, and a third party led by thirdLeader.
const (
	mateLeader  = int32(0x20000101)
	thirdLeader = int32(0x20000201)
	bystander   = int32(0x20000301)
)

// TestDecodeChannelEvents decodes the command channel frames as the server
// lays them out (serverpackets/party.go FrameExOpenMPCC, FrameExCloseMPCC,
// FrameExMPCCPartyInfoUpdate, FrameExMPCCShowPartyMemberInfo;
// usercommand.go FrameExMultiPartyCommandChannelInfo).
func TestDecodeChannelEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"ExOpenMPCC", []byte{0xfe, 0x25, 0x00}, ExOpenMPCC{}},
		{"ExCloseMPCC", []byte{0xfe, 0x26, 0x00}, ExCloseMPCC{}},
		// Leader "L", id 0x01020304, 3 members, added.
		{"ExMPCCPartyInfoUpdate added", []byte{
			0xfe, 0x5a, 0x00, 'L', 0, 0, 0, 0x04, 0x03, 0x02, 0x01, 3, 0, 0, 0, 1, 0, 0, 0,
		}, ExMPCCPartyInfoUpdate{LeaderName: "L", LeaderID: 0x01020304, Count: 3, Added: true}},
		{
			"ExMPCCPartyInfoUpdate removed", packet(OpGameExtended, OpGameExMPCCPartyInfoUpdate, "Lead", mateLeader, 2, 0),
			ExMPCCPartyInfoUpdate{LeaderName: "Lead", LeaderID: mateLeader, Count: 2},
		},
		// One member "A", id 7, class 10.
		{"ExMPCCShowPartyMemberInfo", []byte{
			0xfe, 0x4a, 0x00, 1, 0, 0, 0, 'A', 0, 0, 0, 7, 0, 0, 0, 10, 0, 0, 0,
		}, ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{Name: "A", ObjectID: 7, ClassID: 10}}}},
		{
			"ExMPCCShowPartyMemberInfo two", packet(OpGameExtended, OpGameExMPCCShowPartyMemberInfo, 2, "Lead", mateLeader, 1, foeName, pcID, 0),
			ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{Name: "Lead", ObjectID: mateLeader, ClassID: 1}, {Name: foeName, ObjectID: pcID}}},
		},
		{"ExMPCCShowPartyMemberInfo empty", packet(OpGameExtended, OpGameExMPCCShowPartyMemberInfo, 0), ExMPCCShowPartyMemberInfo{}},
		// Leader "C", loot 0, 5 members over 1 party: "C", id 9, 5.
		{"ExMultiPartyCommandChannelInfo", []byte{
			0xfe, 0x30, 0x00, 'C', 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0, 'C', 0, 0, 0, 9, 0, 0, 0, 5, 0, 0, 0,
		}, ExMultiPartyCommandChannelInfo{LeaderName: "C", Count: 5, Parties: []ChannelPartyRow{{LeaderName: "C", LeaderID: 9, Count: 5}}}},
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

func TestDecodeChannelEventsRejects(t *testing.T) {
	list := packet(OpGameExtended, OpGameExMPCCShowPartyMemberInfo, 1, "A", 7, 10)
	info := packet(OpGameExtended, OpGameExMultiPartyCommandChannelInfo, "C", 0, 5, 1, "C", 9, 5)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"open trailing", []byte{0xfe, 0x25, 0x00, 0}, ErrTrailingBytes},
		{"close trailing", []byte{0xfe, 0x26, 0x00, 0}, ErrTrailingBytes},
		{"update short", packet(OpGameExtended, OpGameExMPCCPartyInfoUpdate, "L", 1, 2), wire.ErrShortPacket},
		{"update trailing", packet(OpGameExtended, OpGameExMPCCPartyInfoUpdate, "L", 1, 2, 1, 0), ErrTrailingBytes},
		{"members short row", list[:len(list)-4], wire.ErrShortPacket},
		{"members count past the frame", packet(OpGameExtended, OpGameExMPCCShowPartyMemberInfo, 0x7fffffff), wire.ErrShortPacket},
		{"members trailing", append(list, 0), ErrTrailingBytes},
		{"info short row", info[:len(info)-4], wire.ErrShortPacket},
		{"info trailing", append(info, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent err = %v, want %v", err, tt.want)
			}
		})
	}
}
