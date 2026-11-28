package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// siegeClanSize is the least size of an attacker row: clan id, two empty
// strings, crest id, 0, ally id, two empty strings, ally crest id.
const siegeClanSize = 4 + 2 + 2 + 4 + 4 + 4 + 2 + 2 + 4

// decodeSiegeAttackerList: castle id, 0, 1, 0, count, count again, then per
// clan its row (serverpackets/siege.go FrameSiegeAttackerList).
func decodeSiegeAttackerList(r *wire.Reader) Event {
	id, clans := decodeSiegeList(r, false)
	return SiegeAttackerList{CastleID: id, Clans: clans}
}

// decodeSiegeDefenderList is decodeSiegeAttackerList's layout with a side
// code in each row after the 0 (FrameSiegeDefenderList).
func decodeSiegeDefenderList(r *wire.Reader) Event {
	id, clans := decodeSiegeList(r, true)
	return SiegeDefenderList{CastleID: id, Clans: clans}
}

// decodeSiegeList reads a siege list's header and rows (writeSiegeListHeader,
// writeSiegeClan): clan id, name, leader name, crest id, 0, the side code
// (defender list only), ally id, ally name, "", ally crest id.
func decodeSiegeList(r *wire.Reader, defenders bool) (int32, []SiegeClan) {
	id := r.ReadInt32()
	r.Skip(3 * 4) // 0, 1, 0
	n := int(r.ReadInt32())
	r.Skip(4) // the count again
	size := siegeClanSize
	if defenders {
		size += 4
	}
	clans := make([]SiegeClan, 0, capFor(r, n, size))
	for ; n > 0 && r.Err() == nil; n-- {
		c := SiegeClan{ClanID: r.ReadInt32(), Name: r.ReadString(), Leader: r.ReadString(), CrestID: r.ReadInt32()}
		r.Skip(4) // 0
		if defenders {
			c.Side = r.ReadInt32()
		}
		c.AllyID, c.AllyName = r.ReadInt32(), r.ReadString()
		r.ReadString() // ""
		c.AllyCrest = r.ReadInt32()
		clans = append(clans, c)
	}
	return id, clans
}

// EncodeRequestSiegeAttackerList asks for the attacking clans of castle id
// (clientpackets/siege.go: one int32). The server answers without a
// dialog (network/siege.go requestSiegeAttackerList); the brain sends one
// per bypassGap, at most nine per siege state change.
func EncodeRequestSiegeAttackerList(castle int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestSiegeAttackerList)
	w.WriteInt32(castle)
	return w.Bytes()
}

// EncodeRequestSiegeDefenderList asks for the defending clans of castle id,
// paced like EncodeRequestSiegeAttackerList.
func EncodeRequestSiegeDefenderList(castle int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestSiegeDefenderList)
	w.WriteInt32(castle)
	return w.Bytes()
}
