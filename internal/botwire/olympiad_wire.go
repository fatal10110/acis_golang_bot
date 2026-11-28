package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// decodeExOlympiadUserInfo: uint8 side, object id, name, class id, then HP,
// max HP, CP, max CP, each an int32 (FrameExOlympiadUserInfo).
func decodeExOlympiadUserInfo(r *wire.Reader) Event {
	u := ExOlympiadUserInfo{Side: r.ReadUint8(), ObjectID: r.ReadInt32(), Name: r.ReadString(), ClassID: r.ReadInt32()}
	u.HP, u.MaxHP = r.ReadInt32(), r.ReadInt32()
	u.CP, u.MaxCP = r.ReadInt32(), r.ReadInt32()
	return u
}
