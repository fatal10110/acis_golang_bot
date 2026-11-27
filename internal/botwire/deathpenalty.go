package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// EtcStatusUpdate is a character's miscellaneous status: its force charges,
// weight band (0-4), grade penalty (1 while a worn piece is above its
// Expertise grade) and death penalty level (0-15), among flags the bot
// does not use. It carries no
// object id: the server sends the bot's own at entry and at every change,
// and broadcasts another player's to everyone that sees it when its Charm
// of Courage starts or ends (event.EtcStatusBroadcast,
// network/live_player_events.go). World.applyDeathPenalty sorts them out;
// the bot takes its weight band and grade penalty from self-only frames
// instead (weightpenalty.go).
type EtcStatusUpdate struct {
	Charges, WeightPenalty, GradePenalty, DeathPenalty int32
}

func (EtcStatusUpdate) IsEvent() {}

// decodeEtcStatusUpdate: seven int32s, charges, weight penalty, blocked,
// danger area, grade penalty, charm of courage and death penalty level
// (serverpackets/etcstatusupdate.go).
func decodeEtcStatusUpdate(r *wire.Reader) Event {
	e := EtcStatusUpdate{Charges: r.ReadInt32(), WeightPenalty: r.ReadInt32()}
	r.Skip(2 * 4) // blocked, danger area
	e.GradePenalty = r.ReadInt32()
	r.Skip(4) // charm of courage
	e.DeathPenalty = r.ReadInt32()
	return e
}
