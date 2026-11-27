package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

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
