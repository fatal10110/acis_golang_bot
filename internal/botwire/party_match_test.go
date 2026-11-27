package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// matchRoom is a room a player named Lea leads, free and open to levels
// 1-20.
var matchRoom = PartyRoom{ID: 5, Title: "exp", Location: 1, MinLevel: 1, MaxLevel: 20, Members: 1, MaxMembers: 4, Leader: "Lea"}

// roomRow is r's fields in the server's order (FramePartyMatchList).
func roomRow(r PartyRoom) []any {
	return []any{r.ID, r.Title, r.Location, r.MinLevel, r.MaxLevel, r.Members, r.MaxMembers, r.Leader}
}

func TestDecodePartyMatchEvents(t *testing.T) {
	other := PartyRoom{ID: 9, Title: "", Location: -1, MinLevel: 40, MaxLevel: 80, Members: 3, MaxMembers: 12, Leader: "Kay"}
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// The server's frame of one room: flag 1, count 1, id 2, title
		// "A", location 3, levels 1-20, 1 of 4 members, leader "L".
		{"PartyMatchList bytes", []byte{
			0x96, 1, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 'A', 0, 0, 0, 3, 0, 0, 0,
			1, 0, 0, 0, 20, 0, 0, 0, 1, 0, 0, 0, 4, 0, 0, 0, 'L', 0, 0, 0,
		}, PartyMatchList{Rooms: []PartyRoom{{ID: 2, Title: "A", Location: 3, MinLevel: 1, MaxLevel: 20, Members: 1, MaxMembers: 4, Leader: "L"}}}},
		{
			"PartyMatchList two", packet(OpGamePartyMatchList, append(append([]any{1, 2}, roomRow(matchRoom)...), roomRow(other)...)...),
			PartyMatchList{Rooms: []PartyRoom{matchRoom, other}},
		},
		{"PartyMatchList empty", packet(OpGamePartyMatchList, 0, 0), PartyMatchList{}},
		{
			"PartyMatchDetail", packet(OpGamePartyMatchDetail, 5, 4, 1, 20, 2, 7, "exp"),
			PartyMatchDetail{RoomID: 5, MaxMembers: 4, MinLevel: 1, MaxLevel: 20, Loot: 2, Location: 7, Title: "exp"},
		},
		{"ExClosePartyRoom", []byte{0xfe, 0x0f, 0x00}, ExClosePartyRoom{}},
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

func TestDecodePartyMatchEventsRejects(t *testing.T) {
	list := packet(OpGamePartyMatchList, append([]any{1, 1}, roomRow(matchRoom)...)...)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"list short of its count", packet(OpGamePartyMatchList, append([]any{1, 2}, roomRow(matchRoom)...)...), wire.ErrShortPacket},
		{"list count past the frame", packet(OpGamePartyMatchList, 1, 0x7fffffff), wire.ErrShortPacket},
		{"list short row", list[:len(list)-4], wire.ErrShortPacket},
		{"list trailing", append(list, 0), ErrTrailingBytes},
		{"detail without title", packet(OpGamePartyMatchDetail, 5, 4, 1, 20, 2, 7), wire.ErrShortPacket},
		{"close trailing", []byte{0xfe, 0x0f, 0x00, 0}, ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
