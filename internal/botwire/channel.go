package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// Command channel packets (internal/gameserver/network/serverpackets/
// party.go and usercommand.go, clientpackets/party.go). The uint16 server
// sub-opcodes follow OpGameExtended, the client one OpGameClientExtended.
const (
	OpGameExOpenMPCC                     uint16 = 0x0025
	OpGameExCloseMPCC                    uint16 = 0x0026
	OpGameExMultiPartyCommandChannelInfo uint16 = 0x0030
	OpGameExMPCCShowPartyMemberInfo      uint16 = 0x004a
	OpGameExMPCCPartyInfoUpdate          uint16 = 0x005a

	OpGameRequestExMPCCShowPartyMembersInfo uint16 = 0x0026
)

// ExOpenMPCC opens the command channel window: the bot's party joined a
// channel, or the bot joined a party in one (party/channel.go add,
// formChannel; party/party.go join).
type ExOpenMPCC struct{}

// ExCloseMPCC closes the command channel window: the bot left its party, or
// its party left the channel, or the channel was dissolved. It alone ends
// the channel in the world model; a party window reset does not.
type ExCloseMPCC struct{}

// ExMPCCPartyInfoUpdate adds (Added) or removes a party of the bot's
// channel: its leader and member count. The party joining is not sent the
// parties already in (party/channel.go add).
type ExMPCCPartyInfoUpdate struct {
	LeaderName      string
	LeaderID, Count int32
	Added           bool
}

// ExMPCCShowPartyMemberInfo lists one party's members, the answer to
// RequestExMPCCShowPartyMembersInfo.
type ExMPCCShowPartyMemberInfo struct {
	Members []ChannelMember
}

// ChannelMember is a row of ExMPCCShowPartyMemberInfo.
type ChannelMember struct {
	Name              string
	ObjectID, ClassID int32
}

// ExMultiPartyCommandChannelInfo is the channel overview, the answer to
// /channellistupdate: its leader, its member count over every party, and
// each party, the bot's own included.
type ExMultiPartyCommandChannelInfo struct {
	LeaderName string
	Count      int32
	Parties    []ChannelPartyRow
}

// ChannelPartyRow is a party row of ExMultiPartyCommandChannelInfo.
type ChannelPartyRow struct {
	LeaderName      string
	LeaderID, Count int32
}

func (ExOpenMPCC) IsEvent() {}

func (ExCloseMPCC) IsEvent() {}

func (ExMPCCPartyInfoUpdate) IsEvent() {}

func (ExMPCCShowPartyMemberInfo) IsEvent() {}

func (ExMultiPartyCommandChannelInfo) IsEvent() {}

// decodeExMPCCPartyInfoUpdate: leader name, leader id, member count, then 1
// for an added party, 0 for a removed one (FrameExMPCCPartyInfoUpdate).
func decodeExMPCCPartyInfoUpdate(r *wire.Reader) Event {
	return ExMPCCPartyInfoUpdate{LeaderName: r.ReadString(), LeaderID: r.ReadInt32(), Count: r.ReadInt32(), Added: r.ReadInt32() != 0}
}

// decodeExMPCCShowPartyMemberInfo: the row count, then per row name, object
// id and class id (FrameExMPCCShowPartyMemberInfo). The count comes off the
// wire, so the loop ends at the first short read.
func decodeExMPCCShowPartyMemberInfo(r *wire.Reader) Event {
	var e ExMPCCShowPartyMemberInfo
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		e.Members = append(e.Members, ChannelMember{Name: r.ReadString(), ObjectID: r.ReadInt32(), ClassID: r.ReadInt32()})
	}
	return e
}

// decodeExMultiPartyCommandChannelInfo: leader name, a loot field always 0,
// the member count, the party count, then per party leader name, leader id
// and member count (FrameExMultiPartyCommandChannelInfo).
func decodeExMultiPartyCommandChannelInfo(r *wire.Reader) Event {
	e := ExMultiPartyCommandChannelInfo{LeaderName: r.ReadString()}
	r.Skip(4)
	e.Count = r.ReadInt32()
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		e.Parties = append(e.Parties, ChannelPartyRow{LeaderName: r.ReadString(), LeaderID: r.ReadInt32(), Count: r.ReadInt32()})
	}
	return e
}

// EncodeRequestExMPCCShowPartyMembersInfo asks for the members of the
// party the player leaderID is in; the server answers nothing for a player
// offline or in no party (network/party_channel.go
// requestChannelPartyMembers).
func EncodeRequestExMPCCShowPartyMembersInfo(leaderID int32) []byte {
	w := wire.NewPacketWriter(OpGameClientExtended)
	w.WriteUint16(OpGameRequestExMPCCShowPartyMembersInfo)
	w.WriteInt32(leaderID)
	return w.Bytes()
}
