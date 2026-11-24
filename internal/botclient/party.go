package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// partyInviteTimeout is how long the server keeps a party invitation
// answerable (RequestTimeout, internal/gameserver/trade/book.go): an answer
// sent later reaches no invitation and does nothing.
const partyInviteTimeout = 15 * time.Second

// maxPartyMembers is the largest party (party.MaxMembers on the server).
const maxPartyMembers = 9

// AskJoinParty invites the bot into the party of the player named
// Requester, under loot rule Loot.
type AskJoinParty struct {
	Requester string
	Loot      int32
}

// JoinParty tells the bot, as an inviter, its invitee's answer: 1 accepted.
type JoinParty struct{ Response int32 }

// PartyMember is one row of the party window: a member's vitals. Gauges are
// the server's values truncated toward zero.
type PartyMember struct {
	ObjectID       int32
	Name           string
	CP, MaxCP      int32
	HP, MaxHP      int32
	MP, MaxMP      int32
	Level, ClassID int32
}

// PartySmallWindowAll is the whole party window, sent to a member who joins
// and to every member on a leader change: the leader, the loot rule and
// every member but the receiver.
type PartySmallWindowAll struct {
	LeaderID, Loot int32
	Members        []PartyMember
}

// PartySmallWindowAdd adds Member to the window of the members already in
// the party.
type PartySmallWindowAdd struct {
	LeaderID, Loot int32
	Member         PartyMember
}

// PartySmallWindowDelete removes the member ObjectID, who left or was
// expelled, from the window of the members who stay.
type PartySmallWindowDelete struct {
	ObjectID int32
	Name     string
}

// PartySmallWindowDeleteAll clears the window: the receiver left, the party
// dispersed, or a new leader's PartySmallWindowAll follows.
type PartySmallWindowDeleteAll struct{}

// PartySmallWindowUpdate refreshes a member's row after one of its gauges
// crossed a segment of the window's bar, so the values are coarse.
type PartySmallWindowUpdate struct{ Member PartyMember }

// PartyMemberAt is one member's position in PartyMemberPosition.
type PartyMemberAt struct {
	ObjectID int32
	Pos      Point
}

// PartyMemberPosition reports where every member, the receiver included,
// is: 6 s after the party forms, then every 12 s
// (internal/gameserver/network/party_channel.go).
type PartyMemberPosition struct{ Members []PartyMemberAt }

func (AskJoinParty) event()              {}
func (JoinParty) event()                 {}
func (PartySmallWindowAll) event()       {}
func (PartySmallWindowAdd) event()       {}
func (PartySmallWindowDelete) event()    {}
func (PartySmallWindowDeleteAll) event() {}
func (PartySmallWindowUpdate) event()    {}
func (PartyMemberPosition) event()       {}

// decodeAskJoinParty: requester name, loot rule (serverpackets/party.go).
func decodeAskJoinParty(r *wire.Reader) Event {
	return AskJoinParty{Requester: r.ReadString(), Loot: r.ReadInt32()}
}

// readPartyMember reads the vitals row: object id, name, CP, max CP, HP,
// max HP, MP, max MP, level, class id (writePartyMemberVitals).
func readPartyMember(r *wire.Reader) PartyMember {
	return PartyMember{
		ObjectID: r.ReadInt32(), Name: r.ReadString(),
		CP: r.ReadInt32(), MaxCP: r.ReadInt32(),
		HP: r.ReadInt32(), MaxHP: r.ReadInt32(),
		MP: r.ReadInt32(), MaxMP: r.ReadInt32(),
		Level: r.ReadInt32(), ClassID: r.ReadInt32(),
	}
}

// decodePartySmallWindowAll: leader id, loot rule, member count, then per
// member its vitals row, a zero and its race, both dropped. The count comes
// off the wire, so the loop ends at the first short read.
func decodePartySmallWindowAll(r *wire.Reader) Event {
	e := PartySmallWindowAll{LeaderID: r.ReadInt32(), Loot: r.ReadInt32()}
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		m := readPartyMember(r)
		r.Skip(2 * 4)
		e.Members = append(e.Members, m)
	}
	return e
}

// decodePartySmallWindowAdd: leader id, loot rule, the vitals row, then two
// zeros.
func decodePartySmallWindowAdd(r *wire.Reader) Event {
	e := PartySmallWindowAdd{LeaderID: r.ReadInt32(), Loot: r.ReadInt32(), Member: readPartyMember(r)}
	r.Skip(2 * 4)
	return e
}

// decodePartySmallWindowDelete: object id, name.
func decodePartySmallWindowDelete(r *wire.Reader) Event {
	return PartySmallWindowDelete{ObjectID: r.ReadInt32(), Name: r.ReadString()}
}

// decodePartyMemberPosition: member count, then per member object id and
// position. The count comes off the wire, so the loop ends at the first
// short read.
func decodePartyMemberPosition(r *wire.Reader) Event {
	var e PartyMemberPosition
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		e.Members = append(e.Members, PartyMemberAt{ObjectID: r.ReadInt32(), Pos: readPoint(r)})
	}
	return e
}

// encodeRequestJoinParty invites the player named target into the bot's
// party under loot rule loot (party.go on the server: 0 finders keepers,
// 1 random, 2 random with spoil; another rule is ignored).
func encodeRequestJoinParty(target string, loot int32) []byte {
	w := wire.NewPacketWriter(opGameRequestJoinParty)
	w.WriteString(target)
	w.WriteInt32(loot)
	return w.Bytes()
}

// encodeRequestAnswerJoinParty answers the pending party invitation: 1
// accepts, 0 declines.
func encodeRequestAnswerJoinParty(accept bool) []byte {
	w := wire.NewPacketWriter(opGameRequestAnswerJoinParty)
	var v int32
	if accept {
		v = 1
	}
	w.WriteInt32(v)
	return w.Bytes()
}

// encodeRequestWithdrawParty leaves the bot's party; in none it does
// nothing.
func encodeRequestWithdrawParty() []byte {
	return wire.NewPacketWriter(opGameRequestWithdrawParty).Bytes()
}

// answerPartyAction answers the open party invitation.
func answerPartyAction(accept bool) Action {
	return Action{kind: actAnswerParty, payload: encodeRequestAnswerJoinParty(accept)}
}

// leavePartyAction leaves the bot's party.
func leavePartyAction() Action {
	return Action{kind: actLeaveParty, payload: encodeRequestWithdrawParty()}
}

// acceptParty is the invitation policy that accepts every invitation.
func acceptParty(*World, string, time.Time) bool { return true }

// PartyInvite is a party invitation waiting for the bot's answer.
type PartyInvite struct {
	Open      bool
	Requester string
	Loot      int32
	// At is when the invitation arrived.
	At time.Time
}

// Pending reports whether the invitation is open and still answerable at
// now.
func (p PartyInvite) Pending(now time.Time) bool {
	return p.Open && now.Sub(p.At) < partyInviteTimeout
}

// Party is the party the bot is in; the zero Party is none.
type Party struct {
	// LeaderID is the leader's object id, the bot's own when it leads; 0
	// while the bot is in no party.
	LeaderID int32
	Loot     int32
	// Members are the other members by object id; the bot is not one.
	Members map[int32]*PartyMate
}

// In reports whether the bot is in a party.
func (p *Party) In() bool { return p.LeaderID != 0 }

// PartyMate is another member of the bot's party: its latest vitals row,
// its last reported position and its effects.
type PartyMate struct {
	PartyMember
	// Pos is where PartyMemberPosition last reported the member, at PosAt;
	// PosAt is zero until the first report.
	Pos   Point
	PosAt time.Time
	// Buffs are its effects as its last PartySpelled listed them, and
	// BuffLevels the skill level of each, by skill id; nil until one
	// arrives, which the server sends only for a member who ever held an
	// effect.
	Buffs      Buffs
	BuffLevels map[int32]int32
}

// applyParty updates the prompt and the party model with a party event.
func (w *World) applyParty(ev Event, now time.Time) {
	p := &w.Party
	switch e := ev.(type) {
	case AskJoinParty:
		w.Prompts.Party = PartyInvite{Open: true, Requester: e.Requester, Loot: e.Loot, At: now}
	case PartySmallWindowAll:
		w.Prompts.Party = PartyInvite{}
		old := p.Members
		*p = Party{LeaderID: e.LeaderID, Loot: e.Loot, Members: make(map[int32]*PartyMate, maxPartyMembers)}
		for _, m := range e.Members {
			mate := &PartyMate{PartyMember: m}
			if o := old[m.ObjectID]; o != nil {
				// A leader change resends the window, not the effects.
				mate.Buffs, mate.BuffLevels = o.Buffs, o.BuffLevels
			}
			p.Members[m.ObjectID] = mate
		}
	case PartySmallWindowAdd:
		if p.Members == nil {
			p.Members = make(map[int32]*PartyMate, maxPartyMembers)
		}
		p.LeaderID, p.Loot = e.LeaderID, e.Loot
		if m := p.Members[e.Member.ObjectID]; m != nil {
			m.PartyMember = e.Member
		} else {
			p.Members[e.Member.ObjectID] = &PartyMate{PartyMember: e.Member}
		}
	case PartySmallWindowUpdate:
		if m := p.Members[e.Member.ObjectID]; m != nil {
			m.PartyMember = e.Member
		}
	case PartySmallWindowDelete:
		delete(p.Members, e.ObjectID)
	case PartySmallWindowDeleteAll:
		*p = Party{}
	case PartyMemberPosition:
		for _, at := range e.Members {
			if m := p.Members[at.ObjectID]; m != nil {
				m.Pos, m.PosAt = at.Pos, now
			}
		}
	}
}
