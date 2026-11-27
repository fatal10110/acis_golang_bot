package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// partyInviteTimeout is how long the server keeps a party invitation
// answerable (RequestTimeout, internal/gameserver/trade/book.go): an answer
// sent later reaches no invitation and does nothing.
const partyInviteTimeout = 15 * time.Second

// maxPartyMembers is the largest party (party.MaxMembers on the server).
const maxPartyMembers = 9

// answerPartyAction answers the open party invitation.
func answerPartyAction(accept bool) Action {
	return Action{kind: actAnswerParty, payload: botwire.EncodeRequestAnswerJoinParty(accept)}
}

// leavePartyAction leaves the bot's party.
func leavePartyAction() Action {
	return Action{kind: actLeaveParty, payload: botwire.EncodeRequestWithdrawParty()}
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
		// The channel survives: the server resets the windows on every
		// leader change too, and ends a channel only by ExCloseMPCC
		// (server party/party.go changeLeader, leave).
		*p = Party{}
	case PartyMemberPosition:
		for _, at := range e.Members {
			if m := p.Members[at.ObjectID]; m != nil {
				m.Pos, m.PosAt = at.Pos, now
			}
		}
	}
}
