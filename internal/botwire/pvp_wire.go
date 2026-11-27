package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// decodeExDuelAskStart: requester name, int32 party flag.
func decodeExDuelAskStart(r *wire.Reader) Event {
	return ExDuelAskStart{Requester: r.ReadString(), Party: r.ReadInt32() != 0}
}

// decodeExDuelUpdateUserInfo: name, object id, class id, level, then HP, max
// HP, MP, max MP, CP, max CP, each an int32.
func decodeExDuelUpdateUserInfo(r *wire.Reader) Event {
	u := ExDuelUpdateUserInfo{Name: r.ReadString(), ObjectID: r.ReadInt32()}
	u.ClassID, u.Level = r.ReadInt32(), r.ReadInt32()
	u.HP, u.MaxHP = r.ReadInt32(), r.ReadInt32()
	u.MP, u.MaxMP = r.ReadInt32(), r.ReadInt32()
	u.CP, u.MaxCP = r.ReadInt32(), r.ReadInt32()
	return u
}

// EncodeRequestDuelStart challenges the player named name to a duel, or its
// party to a party duel. Its sender's pacing lands with its rule (X4); the
// per-connection frame bucket bounds it meanwhile.
func EncodeRequestDuelStart(name string, party bool) []byte {
	w := wire.NewPacketWriter(OpGameClientExtended)
	w.WriteUint16(OpGameRequestDuelStart)
	w.WriteString(name)
	w.WriteInt32(wire.BoolInt32(party))
	return w.Bytes()
}

// EncodeRequestDuelAnswerStart answers an ExDuelAskStart: party echoes the
// kind of duel asked, accepted takes it. The middle int32 is unused by the
// server.
func EncodeRequestDuelAnswerStart(party, accepted bool) []byte {
	w := wire.NewPacketWriter(OpGameClientExtended)
	w.WriteUint16(OpGameRequestDuelAnswerStart)
	w.WriteInt32(wire.BoolInt32(party))
	w.WriteInt32(0)
	w.WriteInt32(wire.BoolInt32(accepted))
	return w.Bytes()
}

// EncodeRequestTargetCancel is the Esc key: unselect 0 stops the bot's cast
// inside its interrupt window and keeps the target, else clears the target
// (network/targeting.go). The brain's guards send one per own cast
// (botclient guardState.escFor), within the per-connection frame limits
// (network/floodprotector.go).
func EncodeRequestTargetCancel() []byte {
	w := wire.NewPacketWriter(OpGameRequestTargetCancel)
	w.WriteUint16(0) // unselect
	return w.Bytes()
}
