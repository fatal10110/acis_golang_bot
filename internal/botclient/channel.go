package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Command channel packets (internal/gameserver/network/serverpackets/
// party.go and usercommand.go, clientpackets/party.go). The uint16 server
// sub-opcodes follow opGameExtended, the client one opGameClientExtended.
const (
	opGameExOpenMPCC                     uint16 = 0x0025
	opGameExCloseMPCC                    uint16 = 0x0026
	opGameExMultiPartyCommandChannelInfo uint16 = 0x0030
	opGameExMPCCShowPartyMemberInfo      uint16 = 0x004a
	opGameExMPCCPartyInfoUpdate          uint16 = 0x005a

	opGameRequestExMPCCShowPartyMembersInfo uint16 = 0x0026
)

// userCommandChannelList is /channellistupdate (network/usercommand.go
// userCommandChannelListUpdate): the server answers a player whose party is
// in a command channel with ExMultiPartyCommandChannelInfo, anyone else with
// nothing.
const userCommandChannelList int32 = 97

// Channel asks. Neither RequestUserCommand nor
// RequestExMPCCShowPartyMembersInfo has a server gate (client_ingame.go
// handleRequestUserCommand, network/party_channel.go
// requestChannelPartyMembers): the frame bucket bounds them, channelAskGap
// spaces any two of them, channelRetry an unanswered one, and
// channelRefresh the lists the bot keeps up to date in an arena, where a
// party's member who joined after its list is not reported (the server
// sends the channel no party's joins or departures).
const (
	channelAskGap  = time.Second
	channelRetry   = 15 * time.Second
	channelRefresh = time.Minute
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

func (ExOpenMPCC) event()                     {}
func (ExCloseMPCC) event()                    {}
func (ExMPCCPartyInfoUpdate) event()          {}
func (ExMPCCShowPartyMemberInfo) event()      {}
func (ExMultiPartyCommandChannelInfo) event() {}

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

// encodeRequestExMPCCShowPartyMembersInfo asks for the members of the
// party the player leaderID is in; the server answers nothing for a player
// offline or in no party (network/party_channel.go
// requestChannelPartyMembers).
func encodeRequestExMPCCShowPartyMembersInfo(leaderID int32) []byte {
	w := wire.NewPacketWriter(opGameClientExtended)
	w.WriteUint16(opGameRequestExMPCCShowPartyMembersInfo)
	w.WriteInt32(leaderID)
	return w.Bytes()
}

// Channel is the command channel the bot's party is in; the zero Channel
// is none. The server pushes no member ids: the bot asks for the parties
// (/channellistupdate) and for each one's members (ruleChannel).
type Channel struct {
	Open bool
	// Parties are the channel's parties by leader object id, the bot's own
	// included.
	Parties map[int32]*ChannelParty
	// listed is set once an overview arrived, at listedAt; listAsked is
	// when the bot last asked for one.
	listed              bool
	listedAt, listAsked time.Time
	// asked is the leader whose member list the bot asked for last: the
	// list that arrives answers it. askedAt is when the bot last asked for
	// an overview or a list.
	asked   int32
	askedAt time.Time
}

// ChannelParty is a party of the bot's channel.
type ChannelParty struct {
	Leader string
	Count  int32
	// Members are the party's member object ids as its last member list
	// gave them, at ListedAt; nil until one arrived. askedAt is when the
	// bot last asked for one.
	Members  map[int32]bool
	ListedAt time.Time
	askedAt  time.Time
}

// Mate reports whether player id is in a party of the bot's channel: a
// party's leader, or a member its last list named.
func (c *Channel) Mate(id int32) bool {
	if !c.Open {
		return false
	}
	for leader, p := range c.Parties {
		if leader == id || p.Members[id] {
			return true
		}
	}
	return false
}

// applyChannel updates the channel model with a channel event.
func (w *World) applyChannel(ev Event, now time.Time) {
	c := &w.Channel
	switch e := ev.(type) {
	case ExOpenMPCC:
		if !c.Open {
			*c = Channel{Open: true, Parties: make(map[int32]*ChannelParty)}
		}
	case ExCloseMPCC:
		*c = Channel{}
	case ExMPCCPartyInfoUpdate:
		if !c.Open {
			return
		}
		if !e.Added {
			delete(c.Parties, e.LeaderID)
			return
		}
		c.party(e.LeaderID, e.LeaderName, e.Count)
	case ExMultiPartyCommandChannelInfo:
		if !c.Open {
			return
		}
		old := c.Parties
		c.Parties, c.listed, c.listedAt = make(map[int32]*ChannelParty, len(e.Parties)), true, now
		for _, row := range e.Parties {
			if p := old[row.LeaderID]; p != nil {
				c.Parties[row.LeaderID] = p
			}
			c.party(row.LeaderID, row.LeaderName, row.Count)
		}
	case ExMPCCShowPartyMemberInfo:
		p := c.Parties[c.asked]
		if !c.Open || p == nil {
			return
		}
		c.asked = 0
		p.Members, p.ListedAt, p.Count = make(map[int32]bool, len(e.Members)), now, int32(len(e.Members))
		for _, m := range e.Members {
			p.Members[m.ObjectID] = true
		}
	}
}

// party records the channel party leaderID leads with count members; a
// changed count makes its member list due again.
func (c *Channel) party(leaderID int32, leader string, count int32) {
	p := c.Parties[leaderID]
	if p == nil {
		p = &ChannelParty{}
		c.Parties[leaderID] = p
	}
	if p.Count != count {
		p.ListedAt, p.askedAt = time.Time{}, time.Time{}
	}
	p.Leader, p.Count = leader, count
}

// channelMate reports whether player id is a member of the bot's command
// channel, which the server never lets the bot attack without force, in an
// arena neither (character_social.go SocialWithoutForce).
func (b *Brain) channelMate(id int32) bool { return b.world.Channel.Mate(id) }

// ruleChannel keeps the bot's command channel known while it is in one:
// the overview first, then each other party's member list, and in an arena
// each again once channelRefresh old (a party's new leader, or a member who
// joined it, is not reported). One ask per channelAskGap.
func (b *Brain) ruleChannel(now time.Time) (Action, bool) {
	c := &b.world.Channel
	if !c.Open || !c.askedAt.IsZero() && now.Sub(c.askedAt) < channelAskGap {
		return Action{}, false
	}
	arena := b.arenaAt(b.world.Self.Position(now)).Arena != 0
	if !c.listed || arena && now.Sub(c.listedAt) >= channelRefresh {
		switch {
		case c.listAsked.IsZero() || now.Sub(c.listAsked) >= channelRetry:
			a := Action{kind: actChannelAsk, payload: encodeRequestUserCommand(userCommandChannelList)}
			if !b.pace.allow(a.gate(), now) {
				return Action{}, false
			}
			c.listAsked, c.askedAt = now, now
			return a, true
		case !c.listed:
			return Action{}, false
		}
	}
	var due int32
	for leader, p := range c.Parties {
		if b.inParty(leader) || !p.due(arena, now) {
			continue
		}
		if due == 0 || leader < due {
			due = leader
		}
	}
	if due == 0 {
		return Action{}, false
	}
	a := Action{kind: actChannelAsk, payload: encodeRequestExMPCCShowPartyMembersInfo(due)}
	if !b.pace.allow(a.gate(), now) {
		return Action{}, false
	}
	c.Parties[due].askedAt, c.asked, c.askedAt = now, due, now
	return a, true
}

// due reports whether the party's member list is to be asked for at now:
// never listed and not asked within channelRetry, or in an arena listed
// channelRefresh ago.
func (p *ChannelParty) due(arena bool, now time.Time) bool {
	if !p.askedAt.IsZero() && now.Sub(p.askedAt) < channelRetry {
		return false
	}
	return p.Members == nil || arena && now.Sub(p.ListedAt) >= channelRefresh
}
