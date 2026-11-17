package botclient

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// Values the in-world client packets carry (the server's
// internal/gameserver/network/client_loop.go and handler/chat/chat.go).
const (
	// actionSitStand is the RequestActionUse id that toggles sitting.
	actionSitStand int32 = 0
	// chatAll is the Say2 channel heard around the speaker.
	chatAll int32 = 0
	// moveByMouse is the MoveBackwardToLocation movement field of a click.
	moveByMouse int32 = 1
	// restartVillage is the RequestRestartPoint type of the nearest town
	// (restart_selection.go).
	restartVillage int32 = 0
)

// encodeMoveBackwardToLocation asks to walk or run to to from origin, as a
// mouse click: target first, then origin, then the movement field.
func encodeMoveBackwardToLocation(to, origin Point) []byte {
	w := wire.NewPacketWriter(opGameMoveBackwardToLocation)
	writePoint(w, to)
	writePoint(w, origin)
	w.WriteInt32(moveByMouse)
	return w.Bytes()
}

// encodeAction clicks objectID from origin: the first click selects it, a
// click on the selected object acts on it (attack, talk, pick up).
func encodeAction(objectID int32, origin Point) []byte {
	w := wire.NewPacketWriter(opGameAction)
	w.WriteInt32(objectID)
	writePoint(w, origin)
	w.WriteUint8(0) // shift not held
	return w.Bytes()
}

// encodeRequestActionUse runs action-bar command id with ctrl and shift
// released.
func encodeRequestActionUse(id int32) []byte {
	w := wire.NewPacketWriter(opGameRequestActionUse)
	w.WriteInt32(id)
	w.WriteInt32(0) // ctrl
	w.WriteUint8(0) // shift
	return w.Bytes()
}

// encodeRequestSocialAction plays emote id (the server accepts 2-13).
func encodeRequestSocialAction(id int32) []byte {
	w := wire.NewPacketWriter(opGameRequestSocialAction)
	w.WriteInt32(id)
	return w.Bytes()
}

// encodeSay2 says text on channel; a tell (channel 2) would add the
// recipient, which no caller sends yet.
func encodeSay2(text string, channel int32) []byte {
	w := wire.NewPacketWriter(opGameSay2)
	w.WriteString(text)
	w.WriteInt32(channel)
	return w.Bytes()
}

// encodeValidatePosition reports the character at p facing heading, not on
// a boat.
func encodeValidatePosition(p Point, heading int32) []byte {
	w := wire.NewPacketWriter(opGameValidatePosition)
	writePoint(w, p)
	w.WriteInt32(heading)
	w.WriteInt32(0) // boat id
	return w.Bytes()
}

func writePoint(w *wire.Writer, p Point) {
	w.WriteInt32(p.X)
	w.WriteInt32(p.Y)
	w.WriteInt32(p.Z)
}

// encodeAppearing tells the server the client has loaded the destination of
// a teleport, which puts the character back in the world.
func encodeAppearing() []byte {
	return wire.NewPacketWriter(opGameAppearing).Bytes()
}

// encodeRequestRestartPoint asks a dead character to restart at the point
// of type kind.
func encodeRequestRestartPoint(kind int32) []byte {
	w := wire.NewPacketWriter(opGameRequestRestartPoint)
	w.WriteInt32(kind)
	return w.Bytes()
}
