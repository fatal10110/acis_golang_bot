package botclient

import (
	"errors"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// chatTell is the Say2 channel of a whisper (say2.go say2Tell): the
// server answers a whisper with CreatureSay of that type to its target
// and an echo to the speaker, named "->" and the target's name
// (chat.go chatTell).
const chatTell int32 = 2

// echoPrefix starts the name of the echo of the bot's own whisper.
const echoPrefix = "->"

// inviteMeText is what the bot whispers back to a GM's "invite me": the bot
// invites nobody, so the GM stays its party's leader.
const inviteMeText = "invite me"

// Replies to a "go" the bot cannot carry out.
const (
	replyNoSpots     = "I know no spots"
	replyUnknownSpot = "unknown spot"
	replySpotFull    = "that spot is full"
)

// commandKind is a command a GM whispers to a bot.
type commandKind uint8

const (
	cmdFollow commandKind = iota + 1
	cmdAssist
	cmdStay
	cmdGo
	cmdInviteMe
	cmdLeave
	cmdLogout
)

// command is a parsed whispered command; spot is the argument of "go".
type command struct {
	kind commandKind
	spot string
}

// parseCommand reads text as a command: one of "follow", "assist", "stay",
// "go <spot>", "invite me", "leave" and "logout", in any case, with any
// spacing around and between the words. Any other text is no command.
func parseCommand(text string) (command, bool) {
	f := strings.Fields(text)
	if len(f) == 0 {
		return command{}, false
	}
	verb := strings.ToLower(f[0])
	if len(f) == 1 {
		switch verb {
		case "follow":
			return command{kind: cmdFollow}, true
		case "assist":
			return command{kind: cmdAssist}, true
		case "stay":
			return command{kind: cmdStay}, true
		case "leave":
			return command{kind: cmdLeave}, true
		case "logout":
			return command{kind: cmdLogout}, true
		}
		return command{}, false
	}
	if len(f) == 2 {
		switch {
		case verb == "go":
			return command{kind: cmdGo, spot: f[1]}, true
		case verb == "invite" && strings.EqualFold(f[1], "me"):
			return command{kind: cmdInviteMe}, true
		}
	}
	return command{}, false
}

// encodeSay2Tell whispers text to the player named target.
func encodeSay2Tell(target, text string) []byte {
	w := wire.NewPacketWriter(opGameSay2)
	w.WriteString(text)
	w.WriteInt32(chatTell)
	w.WriteString(target)
	return w.Bytes()
}

// tellAction whispers text to the player named target. It passes the chat
// gate (chatGap): whispers have no server gate (chat.go chatTell), so the
// bot keeps the pace of its all-channel chat.
func tellAction(target, text string) Action {
	return Action{kind: actTell, payload: encodeSay2Tell(target, text)}
}

// orders are the commands in force, owned by the brain goroutine: the GM
// whispered ones last for the session, the panel's standing ones are set
// again at each session start (applyStanding).
type orders struct {
	// stay holds the bot where it stands (rule "stay").
	stay bool
	// roam stops the bot from following its party leader after a "go".
	roam bool
	// follow and assist are the commanders the bot follows and assists in
	// place of its party leader, 0 for none.
	follow, assist int32
	// reply is a one-off packet a command asks for (a whisper back, a
	// party withdrawal), sent by rule "command". It is a single slot: a
	// later command's reply replaces one not yet sent.
	reply Action
}

// heard takes a chat line at now: a whisper from another player that
// parses as a command goes to the authority, other lines never do; any
// other whisper may get a chat reply (chatHeard), a command never does,
// whoever sent it. The sender is the line's object id, which the server
// fills from the speaker.
func (b *Brain) heard(e CreatureSay, now time.Time) {
	if e.Type != chatTell || e.ObjectID == 0 || e.ObjectID == b.world.Self.ObjectID || strings.HasPrefix(e.Name, echoPrefix) {
		return
	}
	cmd, ok := parseCommand(e.Text)
	if !ok {
		b.chatHeard(e, now)
		return
	}
	if b.cfg.Authority == nil {
		return
	}
	if v, done := b.cfg.Authority.ask(tell{sender: e.ObjectID, name: e.Name, cmd: cmd}, now, b.verdicts); done {
		b.obey(v, now)
	}
}

// obey carries out an authorized command at now and reacts to it.
func (b *Brain) obey(v verdict, now time.Time) {
	if !v.ok {
		return
	}
	o := &b.orders
	switch v.cmd.kind {
	case cmdFollow, cmdAssist, cmdStay, cmdGo:
		// A whispered order replaces the panel's standing one for the
		// session.
		b.orderPending = false
	}
	switch v.cmd.kind {
	case cmdFollow:
		o.stay, o.roam, o.follow = false, false, v.sender
	case cmdAssist:
		o.stay, o.assist = false, v.sender
	case cmdStay:
		b.holdStill()
	case cmdGo:
		o.stay, o.roam, o.follow = false, true, 0
		if err := b.goToSpot(v.cmd.spot, now); err != nil {
			o.reply = tellAction(v.name, spotReply(err))
		}
	case cmdInviteMe:
		if p := &b.world.Party; !p.In() || p.LeaderID != v.sender {
			o.reply = tellAction(v.name, inviteMeText)
		}
	case cmdLeave:
		// The server ignores a withdrawal from no party (server
		// internal/gameserver/network/partymatch.go, withdrawParty).
		if b.world.Party.In() {
			o.reply = leavePartyAction()
		}
	case cmdLogout:
		b.logout.commanded = true
		if !b.logout.requested {
			b.requestLogout(now)
		}
	}
	b.wakeBy(now.Add(b.reactDelay()))
}

// spotReply is the whispered reply to a go goToSpot refused with err.
func spotReply(err error) string {
	if errors.Is(err, ErrNoSpots) {
		return replyNoSpots
	}
	return err.Error()
}

// goToSpot makes the farming spot named name (a spawnlist maker, in any
// case) the bot's spot, within the spot's cap. It returns why it cannot
// (ErrNoSpots, ErrUnknownSpot, ErrSpotFull).
func (b *Brain) goToSpot(name string, now time.Time) error {
	r := b.cfg.Spots
	if r == nil {
		return ErrNoSpots
	}
	i, ok := r.index(name)
	switch {
	case !ok:
		return ErrUnknownSpot
	case b.spot.held && b.spot.idx == i:
		return nil
	}
	if _, ok := r.claim([]int{i}); !ok {
		return ErrSpotFull
	}
	b.leaveSpot()
	b.spot.idx, b.spot.held, b.spot.retryAt = i, true, now
	return nil
}

// ruleCommand sends the one-off packet a command asked for, ahead of every
// errand. While the pacer holds it back (a whisper inside the chat gate) it
// passes, so the survival rules below it still run.
func (b *Brain) ruleCommand(now time.Time) (Action, bool) {
	r := b.orders.reply
	if r.kind == actHold || !b.pace.allow(r.gate(), now) {
		return Action{}, false
	}
	return r, true
}

// ruleStay holds a bot told to stay where it stands: it stops a move under
// way and then sends nothing. Rules ahead of it (prompts, fighting back,
// healing, buffing) still run.
func (b *Brain) ruleStay(now time.Time) (Action, bool) {
	if !b.orders.stay {
		return Action{}, false
	}
	if s := &b.world.Self; s.Moving(now) {
		at := s.Position(now)
		return moveAction(at, at), true
	}
	return Action{}, true
}

// followed is whom the bot follows: the commander it was told to follow,
// else the leader of its party unless a "go" sent it elsewhere; 0 for
// nobody.
func (b *Brain) followed() int32 {
	o, p := &b.orders, &b.world.Party
	switch {
	case o.follow != 0:
		return o.follow
	case o.roam || !p.In():
		return 0
	}
	return p.LeaderID
}

// assisted is whom the bot assists: the commander it was told to assist,
// else the leader of its party when that is another; 0 for nobody.
func (b *Brain) assisted() int32 {
	if b.orders.assist != 0 {
		return b.orders.assist
	}
	if p := &b.world.Party; p.In() && p.LeaderID != b.world.Self.ObjectID {
		return p.LeaderID
	}
	return 0
}
