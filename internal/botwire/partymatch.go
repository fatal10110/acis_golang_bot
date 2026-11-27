package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// PartyRoom is one row of the rooms list.
type PartyRoom struct {
	ID         int32
	Title      string
	Location   int32
	MinLevel   int32
	MaxLevel   int32
	Members    int32
	MaxMembers int32
	Leader     string
}

// PartyMatchList answers RequestListPartyWaiting: the rooms the asker may
// browse, none full (partymatch Rooms).
type PartyMatchList struct{ Rooms []PartyRoom }

// PartyMatchDetail shows the terms of the room the receiver is in: sent
// when it enters one, and when the room's terms change.
type PartyMatchDetail struct {
	RoomID, MaxMembers, MinLevel, MaxLevel, Loot, Location int32
	Title                                                  string
}

// ExClosePartyRoom closes the receiver's room window: it left the room,
// was ousted, or the room was disbanded.
type ExClosePartyRoom struct{}

func (PartyMatchList) IsEvent() {}

func (PartyMatchDetail) IsEvent() {}

func (ExClosePartyRoom) IsEvent() {}

// decodePartyMatchList: a flag (1 when rows follow), the row count, then
// per row id, title, location, min level, max level, members, max members
// and leader name (FramePartyMatchList). The count comes off the wire, so
// the loop ends at the first short read.
func decodePartyMatchList(r *wire.Reader) Event {
	var e PartyMatchList
	r.Skip(4)
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		e.Rooms = append(e.Rooms, PartyRoom{
			ID: r.ReadInt32(), Title: r.ReadString(), Location: r.ReadInt32(),
			MinLevel: r.ReadInt32(), MaxLevel: r.ReadInt32(),
			Members: r.ReadInt32(), MaxMembers: r.ReadInt32(), Leader: r.ReadString(),
		})
	}
	return e
}

// decodePartyMatchDetail: room id, max members, min level, max level,
// loot, location, title (FramePartyMatchDetail).
func decodePartyMatchDetail(r *wire.Reader) Event {
	return PartyMatchDetail{
		RoomID: r.ReadInt32(), MaxMembers: r.ReadInt32(), MinLevel: r.ReadInt32(),
		MaxLevel: r.ReadInt32(), Loot: r.ReadInt32(), Location: r.ReadInt32(), Title: r.ReadString(),
	}
}
