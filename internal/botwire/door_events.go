package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// DoorInfo reports a door entering sight: its world object id, its doors.xml
// id and whether it is closed. The server sends it, then a DoorStatusUpdate,
// when a door enters a character's sight (network/visibility.go).
type DoorInfo struct {
	ObjectID int32
	DoorID   int32
	Closed   bool
}

// DoorStatusUpdate reports a door's state: sent with DoorInfo and broadcast
// to every character that sees the door when it opens or closes.
type DoorStatusUpdate struct {
	ObjectID int32
	DoorID   int32
	Closed   bool
}

func (DoorInfo) IsEvent() {}

func (DoorStatusUpdate) IsEvent() {}

// decodeDoorInfo: object id, door id, show-HP flag, 1, closed flag, max HP,
// HP, 0, 0, every field an int32 (serverpackets/worldobjects.go
// FrameDoorInfo). The flag is the server's !Opened, so 1 is closed.
func decodeDoorInfo(r *wire.Reader) Event {
	ev := DoorInfo{ObjectID: r.ReadInt32(), DoorID: r.ReadInt32()}
	r.Skip(8) // show-HP flag, the constant 1
	ev.Closed = r.ReadInt32() != 0
	r.Skip(16) // max HP, HP, two zeros
	return ev
}

// decodeDoorStatusUpdate: object id, closed flag, damage stage, show-HP flag,
// door id, max HP, HP, every field an int32 (serverpackets/worldobjects.go
// FrameDoorStatusUpdate).
func decodeDoorStatusUpdate(r *wire.Reader) Event {
	ev := DoorStatusUpdate{ObjectID: r.ReadInt32(), Closed: r.ReadInt32() != 0}
	r.Skip(8) // damage stage, show-HP flag
	ev.DoorID = r.ReadInt32()
	r.Skip(8) // max HP, HP
	return ev
}
