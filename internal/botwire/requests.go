package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// Values the in-world client packets carry (the server's
// internal/gameserver/network/client_loop.go and handler/chat/chat.go).
const (
	// ActionSitStand is the RequestActionUse id that toggles sitting.
	ActionSitStand int32 = 0
	// ActionWalkRun is the RequestActionUse id that toggles running.
	ActionWalkRun int32 = 1
	// ActionServitorAttack orders the servitor onto its owner's target,
	// ActionServitorStop sends it idle, back to following its owner
	// (network/summon.go actionServitorAttack, actionServitorStop). The
	// follow toggle (21) and the unsummon (52) are never sent
	// (docs/servitors.md section 2).
	ActionServitorAttack int32 = 22
	ActionServitorStop   int32 = 23
	// ChatAll is the Say2 channel heard around the speaker; ChatShout and
	// ChatTrade the ones the server delivers to the players of a region.
	ChatAll   int32 = 0
	ChatShout int32 = 1
	ChatTrade int32 = 8
	// MoveByMouse is the MoveBackwardToLocation movement field of a click.
	MoveByMouse int32 = 1
	// RestartVillage is the RequestRestartPoint type of the nearest town
	// (restart_selection.go).
	RestartVillage int32 = 0
)

// EncodeMoveBackwardToLocation asks to walk or run to to from origin, as a
// mouse click: target first, then origin, then the movement field.
func EncodeMoveBackwardToLocation(to, origin Point) []byte {
	w := wire.NewPacketWriter(OpGameMoveBackwardToLocation)
	writePoint(w, to)
	writePoint(w, origin)
	w.WriteInt32(MoveByMouse)
	return w.Bytes()
}

// EncodeAction clicks objectID from origin: the first click selects it, a
// click on the selected object acts on it (attack, talk, pick up).
func EncodeAction(objectID int32, origin Point) []byte {
	w := wire.NewPacketWriter(OpGameAction)
	w.WriteInt32(objectID)
	writePoint(w, origin)
	w.WriteUint8(0) // shift not held
	return w.Bytes()
}

// EncodeRequestActionUse runs action-bar command id with ctrl and shift
// released.
func EncodeRequestActionUse(id int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestActionUse)
	w.WriteInt32(id)
	w.WriteInt32(0) // ctrl
	w.WriteUint8(0) // shift
	return w.Bytes()
}

// EncodeRequestSocialAction plays emote id (the server accepts 2-13).
func EncodeRequestSocialAction(id int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestSocialAction)
	w.WriteInt32(id)
	return w.Bytes()
}

// EncodeSay2 says text on channel; a tell (channel 2) would add the
// recipient, which no caller sends yet.
func EncodeSay2(text string, channel int32) []byte {
	w := wire.NewPacketWriter(OpGameSay2)
	w.WriteString(text)
	w.WriteInt32(channel)
	return w.Bytes()
}

// EncodeValidatePosition reports the character at p facing heading, not on
// a boat.
func EncodeValidatePosition(p Point, heading int32) []byte {
	w := wire.NewPacketWriter(OpGameValidatePosition)
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

// EncodeAppearing tells the server the client has loaded the destination of
// a teleport, which puts the character back in the world.
func EncodeAppearing() []byte {
	return wire.NewPacketWriter(OpGameAppearing).Bytes()
}

// EncodeRequestRestartPoint asks a dead character to restart at the point
// of type kind.
func EncodeRequestRestartPoint(kind int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestRestartPoint)
	w.WriteInt32(kind)
	return w.Bytes()
}

// EncodeAttackRequest force-attacks objectID from origin, as a ctrl-click:
// on the selected object the server attacks it whatever its PvP state, on
// any other object it only selects it (client_ingame.go
// handleAttackRequest). It has Action's layout.
func EncodeAttackRequest(objectID int32, origin Point) []byte {
	w := wire.NewPacketWriter(OpGameAttackRequest)
	w.WriteInt32(objectID)
	writePoint(w, origin)
	w.WriteUint8(0) // shift not held
	return w.Bytes()
}

// EncodeRequestMagicSkillUse casts skill id at the selected target, ctrl
// held for a forced cast, shift released (magic_skill_use.go).
func EncodeRequestMagicSkillUse(id int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(OpGameRequestMagicSkillUse)
	w.WriteInt32(id)
	var c int32
	if ctrl {
		c = 1
	}
	w.WriteInt32(c)
	w.WriteUint8(0) // shift
	return w.Bytes()
}

// EncodeSay2Tell whispers text to the player named target.
func EncodeSay2Tell(target, text string) []byte {
	w := wire.NewPacketWriter(OpGameSay2)
	w.WriteString(text)
	w.WriteInt32(ChatTell)
	w.WriteString(target)
	return w.Bytes()
}

// ChatTell is the Say2 channel of a whisper (say2.go say2Tell): the
// server answers a whisper with CreatureSay of that type to its target
// and an echo to the speaker, named "->" and the target's name
// (chat.go chatTell).
const ChatTell int32 = 2
