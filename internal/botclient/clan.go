package botclient

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// clanInviteTimeout is how long the server keeps a clan invitation
// answerable (clan.InviteTimeout, internal/gameserver/clan/invite.go): an
// answer sent later reaches no invitation and does nothing.
const clanInviteTimeout = 15 * time.Second

// msgInvitedToClan is the system message S1HasInvitedYouToJoinTheClanS2,
// its text parameters the inviter's name and the clan's: the server sends
// it right before AskJoinPledge (network/clan.go requestJoinPledge).
const msgInvitedToClan = 67

// encodeRequestAnswerJoinPledge answers the pending clan invitation: 1
// accepts, 0 declines (clientpackets/pledge.go, any non-zero accepts).
func encodeRequestAnswerJoinPledge(accept bool) []byte {
	w := wire.NewPacketWriter(opGameRequestAnswerJoinPledge)
	w.WriteInt32(wire.BoolInt32(accept))
	return w.Bytes()
}

// encodeRequestJoinPledge invites the player whose object id is target
// into the sender's clan, sub-unit pledgeType (0 the main clan)
// (clientpackets/pledge.go).
func encodeRequestJoinPledge(target, pledgeType int32) []byte {
	w := wire.NewPacketWriter(opGameRequestJoinPledge)
	w.WriteInt32(target)
	w.WriteInt32(pledgeType)
	return w.Bytes()
}

// answerClanAction answers the open clan invitation.
func answerClanAction(accept bool) Action {
	return Action{kind: actAnswerClan, payload: encodeRequestAnswerJoinPledge(accept)}
}

// AcceptClanFrom is the clan invitation policy of a bot that joins a
// player's clan (the crafter, D20): while clanless it accepts the
// invitation of any player, or, when inviters names some, only of a player
// so named (names compare case-insensitively, as character names do).
// Authority.Hooks adds a known GM's invitation on top.
func AcceptClanFrom(inviters ...string) func(w *World, requester, clan string, now time.Time) bool {
	inviters = slices.Clone(inviters)
	return func(w *World, requester, _ string, _ time.Time) bool {
		if w.Clan.ID != 0 {
			return false
		}
		return len(inviters) == 0 || slices.ContainsFunc(inviters, func(n string) bool { return strings.EqualFold(n, requester) })
	}
}

// ClanInvite is a clan invitation waiting for the bot's answer.
type ClanInvite struct {
	Open bool
	// RequesterID is the inviter's object id and Requester its name, ""
	// when neither the system message before the invitation nor an object
	// in sight told it.
	RequesterID int32
	Requester   string
	Clan        string
	// At is when the invitation arrived.
	At time.Time
}

// Pending reports whether the invitation is open and still answerable at
// now.
func (c ClanInvite) Pending(now time.Time) bool {
	return c.Open && now.Sub(c.At) < clanInviteTimeout
}

// Clan is the bot's clan; the zero Clan is none.
type Clan struct {
	// ID is the clan id, 0 while the bot is in no clan.
	ID int32
	// Name and Leader are the clan's name and its leader's name from the
	// main roster (PledgeShowMemberListAll), "" until one arrives.
	Name, Leader string
}

// clanInviter is the inviter and clan the last S1HasInvitedYouToJoinTheClanS2
// named, kept for the AskJoinPledge that follows it.
type clanInviter struct{ name, clan string }

// applyClan updates the clan prompt and the clan model with a clan event.
func (w *World) applyClan(ev Event, now time.Time) {
	switch e := ev.(type) {
	case SystemMessage:
		if e.ID == msgInvitedToClan {
			w.clanInviter = clanInviter{name: e.Text[0], clan: e.Text[1]}
		}
	case AskJoinPledge:
		name := ""
		if o := w.Objects[e.RequesterID]; o != nil && o.Kind == KindPlayer {
			name = o.Name
		} else if w.clanInviter.clan == e.Clan {
			name = w.clanInviter.name
		}
		w.clanInviter = clanInviter{}
		w.Prompts.Clan = ClanInvite{Open: true, RequesterID: e.RequesterID, Requester: name, Clan: e.Clan, At: now}
	case JoinPledge:
		w.Prompts.Clan = ClanInvite{}
		if w.Clan.ID != e.ClanID {
			w.Clan = Clan{ID: e.ClanID}
		}
	case PledgeShowMemberListAll:
		if e.PledgeType == 0 {
			w.Clan = Clan{ID: e.ClanID, Name: e.Name, Leader: e.Leader}
		}
	case PledgeShowMemberListDeleteAll:
		w.Clan = Clan{}
	}
}

// applyClanID keeps the clan id UserInfo carries: a new id forgets the
// old clan's name and leader, 0 the clan.
func (w *World) applyClanID(id int32) {
	if w.Clan.ID != id {
		w.Clan = Clan{ID: id}
	}
}
