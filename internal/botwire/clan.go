package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// AskJoinPledge invites the bot into the clan named Clan, by the player
// whose object id is RequesterID.
type AskJoinPledge struct {
	RequesterID int32
	Clan        string
}

// JoinPledge tells the bot it joined the clan ClanID.
type JoinPledge struct{ ClanID int32 }

// PledgeShowMemberListAll is one roster of the bot's clan: the main clan's
// when PledgeType is 0, else a sub-unit's, whose Name and Leader are the
// sub-unit's. The server sends the main roster first, on entering the world
// in a clan and on joining one (network/clan_view.go pledgeListFrames).
type PledgeShowMemberListAll struct {
	ClanID, PledgeType int32
	Name, Leader       string
}

// PledgeShowMemberListDeleteAll clears and disables the clan window: the
// bot left its clan, or the clan was dissolved (network/clan.go
// sendLeftClan).
type PledgeShowMemberListDeleteAll struct{}

func (AskJoinPledge) IsEvent() {}

func (JoinPledge) IsEvent() {}

func (PledgeShowMemberListAll) IsEvent() {}

func (PledgeShowMemberListDeleteAll) IsEvent() {}

// decodeAskJoinPledge: requester object id, clan name
// (serverpackets/pledge.go FrameAskJoinPledge).
func decodeAskJoinPledge(r *wire.Reader) Event {
	return AskJoinPledge{RequesterID: r.ReadInt32(), Clan: r.ReadString()}
}

// decodePledgeShowMemberListAll: sub-unit flag, clan id, pledge type, name,
// leader name, crest, level, castle, clan hall, rank, reputation,
// dissolving, 0, ally id, ally name, ally crest, at war, member count, then
// per member name, level, class, sex, race, online object id and sponsor
// flag, all dropped (serverpackets/pledgememberlist.go). The count comes
// off the wire, so the loop ends at the first short read.
func decodePledgeShowMemberListAll(r *wire.Reader) Event {
	r.Skip(4)
	e := PledgeShowMemberListAll{ClanID: r.ReadInt32(), PledgeType: r.ReadInt32(), Name: r.ReadString(), Leader: r.ReadString()}
	r.Skip(9 * 4) // crest, level, castle, hall, rank, reputation, dissolving, 0, ally id
	r.ReadString()
	r.Skip(2 * 4) // ally crest, at war
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		r.ReadString()
		r.Skip(6 * 4) // level, class, sex, race, online object id, sponsor
	}
	return e
}

// EncodeRequestAnswerJoinPledge answers the pending clan invitation: 1
// accepts, 0 declines (clientpackets/pledge.go, any non-zero accepts).
func EncodeRequestAnswerJoinPledge(accept bool) []byte {
	w := wire.NewPacketWriter(OpGameRequestAnswerJoinPledge)
	w.WriteInt32(wire.BoolInt32(accept))
	return w.Bytes()
}

// EncodeRequestJoinPledge invites the player whose object id is target
// into the sender's clan, sub-unit pledgeType (0 the main clan)
// (clientpackets/pledge.go).
func EncodeRequestJoinPledge(target, pledgeType int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestJoinPledge)
	w.WriteInt32(target)
	w.WriteInt32(pledgeType)
	return w.Bytes()
}
