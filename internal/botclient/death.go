package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// ConfirmDlg message ids answered as a resurrection offer
// (serverpackets/confirmdlg.go, network/dlganswer.go).
const (
	confirmResurrect int32 = 1510 // ConfirmDlgResurrectionRequest
	confirmRestore   int32 = 332  // ConfirmDlgRestoreRequest
)

// ConfirmDlg parameter types the server writes (confirmdlg.go).
const (
	confirmParamText int32 = 0 // a string
	confirmParamZone int32 = 7 // a position: x, y, z
)

// userCommandEscape is the /unstuck user command (network/usercommand.go
// userCommandEscape): a five-minute recall cast to the nearest town.
const userCommandEscape int32 = 52

// msgNoUnstuckPetition is the server's refusal of /unstuck where the bot
// cannot use it (SystemMessageNoUnstuckPleaseSendPetition): asking again
// changes nothing.
const msgNoUnstuckPetition int32 = 1043

// Recovery waits.
const (
	// escapeRetry is how long after an /unstuck whose recall cast is not
	// running (refused, or cut short) the bot asks again.
	escapeRetry = 30 * time.Second
)

// ConfirmDlg is a yes/no question from the server, such as a resurrection
// offer (MessageID 1510, Name the reviver).
type ConfirmDlg struct {
	MessageID int32
	// Name is the first text parameter, "" for none.
	Name string
	// Time is the countdown in milliseconds and RequesterID the object id
	// the answer echoes; the server leaves out each one that is zero, so a
	// lone trailing value reads as Time.
	Time, RequesterID int32
}

func (ConfirmDlg) event() {}

// decodeConfirmDlg: message id, parameter count, each parameter's type and
// value (text: a string; zone: x, y, z), then the optional countdown and
// requester id. A parameter type the server never writes leaves the rest
// unparsable: it is skipped.
func decodeConfirmDlg(r *wire.Reader) Event {
	e := ConfirmDlg{MessageID: r.ReadInt32()}
	named := false
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		switch r.ReadInt32() {
		case confirmParamText:
			if s := r.ReadString(); !named {
				e.Name, named = s, true
			}
		case confirmParamZone:
			r.Skip(3 * 4)
		default:
			r.Skip(r.Remaining())
			return e
		}
	}
	if r.Remaining() >= 4 {
		e.Time = r.ReadInt32()
	}
	if r.Remaining() >= 4 {
		e.RequesterID = r.ReadInt32()
	}
	return e
}

// encodeDlgAnswer answers ConfirmDlg msg (accept 1, decline 0), echoing its
// requester id.
func encodeDlgAnswer(msg int32, accept bool, requester int32) []byte {
	w := wire.NewPacketWriter(opGameDlgAnswer)
	w.WriteInt32(msg)
	var v int32
	if accept {
		v = 1
	}
	w.WriteInt32(v)
	w.WriteInt32(requester)
	return w.Bytes()
}

// encodeRequestUserCommand runs a numbered slash command.
func encodeRequestUserCommand(id int32) []byte {
	w := wire.NewPacketWriter(opGameRequestUserCommand)
	w.WriteInt32(id)
	return w.Bytes()
}

// ResurrectOffer is a resurrection offered to the dead bot, open until the
// bot answers it or revives. The server keeps one open at a time.
type ResurrectOffer struct {
	Open        bool
	MessageID   int32
	Reviver     string
	RequesterID int32
}

// applyConfirmDlg opens a resurrection offer made to the dead bot; other
// questions are not the bot's to answer and stay unanswered.
func (w *World) applyConfirmDlg(e ConfirmDlg) {
	if (e.MessageID == confirmResurrect || e.MessageID == confirmRestore) && w.Self.Dead {
		w.Prompts.Resurrect = ResurrectOffer{Open: true, MessageID: e.MessageID, Reviver: e.Name, RequesterID: e.RequesterID}
	}
}

// answerResurrectAction answers the open resurrection offer.
func answerResurrectAction(o ResurrectOffer, accept bool) Action {
	return Action{kind: actDlgAnswer, payload: encodeDlgAnswer(o.MessageID, accept, o.RequesterID)}
}

// escapeAction asks for /unstuck.
func escapeAction() Action {
	return Action{kind: actUserCommand, payload: encodeRequestUserCommand(userCommandEscape)}
}

// RecoverPolicy is how a bot recovers from its death and from losing its
// party leader (rules "dead" and "leader"). Every value is a configurable
// default (D6).
type RecoverPolicy struct {
	// AcceptResurrection accepts a resurrection offered to the dead bot;
	// false declines it.
	AcceptResurrection bool
	// ResurrectWait is how long a dead bot in a party waits for a
	// resurrection before its death policy (Persona.Death) runs; 0 waits
	// none.
	ResurrectWait time.Duration
	// SitByLeader has the bot sit by the body of the leader it follows
	// while the leader lies dead in sight, and stand once it revives.
	SitByLeader bool
	// LeaderWait is how long the bot waits where it stands once its party
	// leader left the party, logged out, or teleported out of sight. Then
	// it whispers Whisper to the leader once ("" whispers nothing) and
	// falls back to the nearest town with /unstuck. 0 never falls back.
	LeaderWait time.Duration
	Whisper    string
	// JoinWait bounds the join hold (docs/parties/members.md, "Join hold"):
	// a bot holding a join slot in no party holds where it stands, whispering
	// nothing, until its leader invites it again or the organizer's unparty
	// frees it; JoinWait after it found itself partyless it clears the slot
	// itself (Status.PartyLeft "timeout") and plays on. 0 holds until the
	// unparty or another clear of the slot.
	JoinWait time.Duration
	// LiftDeathPenalty buys, first in the shop errand's supplies tier, a
	// Scroll: Recovery of the bot's level band per death penalty level,
	// goes home for one when it carries none (supply trip causePenalty),
	// and reads them out of combat (rule "recover"), one level each.
	LiftDeathPenalty bool
}

// defaultRecoverPolicy is the support persona's (docs/plan.md, personas):
// a party member waits two minutes for a resurrection, a bot whose leader
// is gone waits five minutes before it gives up on it, a member of a bot
// party out of it waits six (the organizer's LeaderGone plus two of its
// ticks) for its leader's invitation, and a death penalty is lifted with
// Scrolls: Recovery.
func defaultRecoverPolicy() RecoverPolicy {
	return RecoverPolicy{
		AcceptResurrection: true,
		ResurrectWait:      2 * time.Minute,
		SitByLeader:        true,
		LeaderWait:         5 * time.Minute,
		Whisper:            "where did you go?",
		JoinWait:           6 * time.Minute,
		LiftDeathPenalty:   true,
	}
}

// leaderWatch is the party leader the bot follows and its loss, owned by
// the brain goroutine (rule "leader").
type leaderWatch struct {
	// id and name are the party leader other than the bot it last saw, 0
	// for none.
	id   int32
	name string
	// lost is when the leader left the party, logged out or teleported
	// away, zero while it has not; teleported marks the latter, which ends
	// when the leader is in sight again (seen).
	lost       time.Time
	teleported bool
	seen       bool
	// told is set once the whisper went out; escapeAt is the last /unstuck
	// the rule sent; refused is set by the server's petition refusal of it,
	// which asking again does not change, until the bot is teleported or the
	// loss ends (the bot holds where it was refused meanwhile); home is set
	// once the bot landed in town.
	told     bool
	escapeAt time.Time
	refused  bool
	home     bool
	// sat is set while the bot sits by its dead leader.
	sat bool
	// partyless is when the bot, holding a join slot, found itself in no
	// party, zero while it is in one or holds no join slot (the join hold's
	// JoinWait counts from it).
	partyless time.Time
}

// watchLeader updates the watch from the party: a leader that is no longer
// the party's (the party disbanded, or the bot leads it now) is lost; the
// lost leader leading the bot again, or a new leader, ends the loss. A join
// member the server made leader keeps its slot for the handover, so it
// counts no leader lost (docs/parties/leader.md, "Handover").
func (b *Brain) watchLeader(now time.Time) {
	v, w := &b.lead, b.world
	if l := b.cfg.link; l != nil && l.party.Kind == CommandJoin && w.Party.In() && w.Party.LeaderID == w.Self.ObjectID {
		*v = leaderWatch{}
		return
	}
	switch l := b.cfg.link; {
	case w.Party.In() || l == nil || l.party.Kind != CommandJoin:
		v.partyless = time.Time{}
	case v.partyless.IsZero():
		v.partyless = now
	}
	var cur int32
	if p := &w.Party; p.In() && p.LeaderID != w.Self.ObjectID {
		cur = p.LeaderID
	}
	if !v.lost.IsZero() {
		back := cur == v.id && (!v.teleported || v.seen)
		if !back && (cur == 0 || cur == v.id) {
			return
		}
		*v = leaderWatch{}
	}
	switch {
	case cur != 0:
		if v.id != cur {
			*v = leaderWatch{id: cur}
		}
		if m := w.Party.Members[cur]; m != nil && m.Name != "" {
			v.name = m.Name
		}
	case v.id != 0:
		v.lost = now
	}
}

// msgExpelledFromParty is the system message the server sends a member the
// leader expels from a party that outlives it (server party/party.go remove).
const msgExpelledFromParty = 202

// leaderEvent notes the followed party leader's own teleport, a loading
// screen jump that takes it out of the bot's sight, and its coming back in
// sight; the bot's expulsion from the party; the bot's own landing in town
// once its leader is lost; and the server's refusal of the rule's /unstuck.
func (b *Brain) leaderEvent(ev Event, now time.Time) {
	v, self := &b.lead, b.world.Self.ObjectID
	switch e := ev.(type) {
	case TeleportToLocation:
		switch {
		case e.ObjectID == self && !e.Fast && !v.lost.IsZero():
			if !v.escapeAt.IsZero() && !v.refused {
				// The recall the rule asked landed (#335).
				b.escape.landed(now)
			}
			v.home, v.refused = true, false
		case e.ObjectID == v.id && v.id != 0 && !e.Fast && v.lost.IsZero():
			v.lost, v.teleported, v.seen = now, true, false
		}
	case CharInfo:
		if e.ObjectID == v.id && v.teleported {
			v.seen = true
		}
	case SystemMessage:
		// The leader expelling the bot from a party of three or more sends
		// this before PartySmallWindowDeleteAll: the bot was let go, its
		// leader is not lost.
		if e.ID == msgExpelledFromParty {
			*v = leaderWatch{}
		}
		// The server refuses /unstuck in the olympiad, observer mode, the
		// festival, jail and boss zones (network/usercommand.go
		// userCommandEscape). The refusal of the walker's ask is the
		// walker's (observeWalk).
		if e.ID == msgNoUnstuckPetition && !b.escape.walker && !v.escapeAt.IsZero() && !v.lost.IsZero() && !v.refused {
			v.refused = true
			b.log.Warn("unstuck refused: the bot holds by its lost leader's last place until a teleport")
		}
	case PartySmallWindowAll, PartySmallWindowAdd, PartySmallWindowDelete, PartySmallWindowDeleteAll:
		b.watchLeader(now)
	}
}

// ruleLeader handles the edge cases of the leader the bot follows. A leader
// lying dead in sight: the bot sits by the body once there (rule "follow"
// takes it there) and stands when the leader revives. A party leader that
// left the party, logged out or teleported away: the bot holds where it
// stands for LeaderWait, whispers Whisper to it once, then casts /unstuck
// and holds until the recall lands it in town; a leader teleported away is
// waited for there, while one that left frees the bot. A leader back or a
// new leader ends it. While told to follow a commander, only the body
// matters. A bot holding a join slot in no party holds in the join hold
// instead (joinHold).
//
// The whisper passes the chat gate (gateChat). /unstuck (RequestUserCommand)
// has no server gate: escapeRetry spaces a repeat while no recall cast runs,
// the brain's escapeBreaker (shared with the walker) allows at most
// escapeTries in escapeEvery (3 in 10 min) and none for escapeEvery after a
// recall the bot asked landed, and the server's petition refusal stops the
// rule's asks until a teleport or the loss ends (#335).
func (b *Brain) ruleLeader(now time.Time) (Action, bool) {
	b.watchLeader(now)
	if a, ok := b.byLeaderBody(now); ok {
		return a, true
	}
	if b.joinHold(now) {
		return Action{}, true
	}
	v, p, s := &b.lead, &b.persona.Recover, &b.world.Self
	if v.lost.IsZero() || p.LeaderWait <= 0 || b.orders.follow != 0 || b.slotLeaderLost() {
		return Action{}, false
	}
	switch {
	case now.Before(v.lost.Add(p.LeaderWait)):
		return Action{}, true
	case !v.told && p.Whisper != "" && v.name != "":
		a := tellAction(v.name, p.Whisper)
		if !b.pace.allow(a.gate(), now) {
			return Action{}, true
		}
		v.told = true
		return a, true
	case v.home:
		// In town: wait there for a leader still the party's.
		return Action{}, b.followed() == v.id
	case v.refused || b.escape.open(now):
		return Action{}, true
	case s.Casting(now) || !v.escapeAt.IsZero() && now.Sub(v.escapeAt) < escapeRetry:
		return Action{}, true
	case s.Sitting:
		return b.toggle(now), true
	}
	return escapeAction(), true
}

// joinHold reports whether the join hold keeps the bot where it stands
// (docs/parties/members.md, "Join hold"): holding a join slot in no party,
// it waits for its leader's invitation, claiming nothing, for any loss of
// its leader and between invitations; it neither whispers nor casts
// /unstuck. It holds a join member the server made leader the same way
// while its handover runs (rule "party"), so the bot neither follows itself
// nor claims a spot. The rules ahead of "leader" (attacked, heal, rest,
// rebuff, the errands) still run. JoinWait after the bot found itself
// partyless the brain clears the slot (Status.PartyLeft "timeout") and
// resets the leader watch, ending the hold: the bot plays its persona where
// it stands.
func (b *Brain) joinHold(now time.Time) bool {
	l, v := b.cfg.link, &b.lead
	if l == nil || l.party.Kind != CommandJoin {
		return false
	}
	if b.world.Party.In() {
		return b.leading()
	}
	w := b.persona.Recover.JoinWait
	if w <= 0 || v.partyless.IsZero() || now.Before(v.partyless.Add(w)) {
		return true
	}
	slot := l.party
	l.party, l.partyLeft = Command{}, partyLeftTimeout
	b.leaveSlot(slot)
	b.log.Info("join hold timed out: party slot cleared", "leader", slotLeader(slot), "wait", w)
	return false
}

// slotLeaderLost reports whether the lost leader is a bot the party slot
// names: bots of a bot party never whisper each other (docs/plan.md, D29),
// and a join member the server made leader keeps its slot for the handover
// instead of waiting (docs/parties/members.md, "Clearing from no party").
func (b *Brain) slotLeaderLost() bool {
	l := b.cfg.link
	return l != nil && l.party.Kind != 0 && b.lead.name != "" && slotNames(l.party, b.lead.name)
}

// byLeaderBody sits the bot by the body of the leader it follows, within
// the follow policy's MaxDist+Repath of it (anywhere when it follows from
// no distance), and stands it up once the leader is no longer dead.
func (b *Brain) byLeaderBody(now time.Time) (Action, bool) {
	v, s, f := &b.lead, &b.world.Self, &b.persona.Follow
	id := b.followed()
	o := b.world.Objects[id]
	if !b.persona.Recover.SitByLeader || id == 0 || id == s.ObjectID || o == nil || !o.Dead {
		if v.sat {
			if s.Sitting {
				return b.toggle(now), true
			}
			v.sat = false
		}
		return Action{}, false
	}
	reach := max(int64(f.MaxDist)+int64(f.Repath), 0)
	if s.Moving(now) || f.MaxDist > 0 && dist2(s.Position(now), o.Position(now)) > reach*reach {
		return Action{}, false
	}
	v.sat = true
	if !s.Sitting {
		return b.toggle(now), true
	}
	return Action{}, true
}
