package botclient

import (
	"errors"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// echoPrefix starts the name of the echo of the bot's own whisper.
const echoPrefix = "->"

// inviteMeText is what the bot whispers back to a GM's "invite me", and a
// Support member to a player's (playerInviteMe): the bot invites only the
// bots its lead slot lists and players who whisper join, never the GM, so
// the GM stays its party's leader.
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
	// cmdCraft is the craft order of a clan crafter (docs/crafting.md
	// section 8): the clan leader may give it too, and in clan chat.
	cmdCraft
	// cmdJoin is any player's ask to join a bot party (D28,
	// docs/parties/players.md): it never reaches the authority.
	cmdJoin
	// cmdDuel is any player's ask for the bot's challenge
	// (docs/pvp/finding.md "Sources"), for a persona that takes duels: it
	// never reaches the authority.
	cmdDuel
)

// command is a parsed whispered command; spot is the argument of "go",
// item and count those of "craft" (count 0: out of bounds).
type command struct {
	kind  commandKind
	spot  string
	item  string
	count int64
}

// parseCommand reads text as a command: one of "follow", "assist", "stay",
// "go <spot>", "invite me", "leave", "logout", "join" and "craft <item
// name> [count]", in any case, with any spacing around and between the words.
// Any other text is no command.
func parseCommand(text string) (command, bool) {
	f := strings.Fields(text)
	if len(f) == 0 {
		return command{}, false
	}
	verb := strings.ToLower(f[0])
	if verb == "craft" {
		return parseCraft(f[1:]), true
	}
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
		case "join":
			return command{kind: cmdJoin}, true
		case "duel":
			return command{kind: cmdDuel}, true
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

// tellAction whispers text to the player named target. It passes the chat
// gate (chatGap): whispers have no server gate (chat.go chatTell), so the
// bot keeps the pace of its all-channel chat.
func tellAction(target, text string) Action {
	return Action{kind: actTell, payload: botwire.EncodeSay2Tell(target, text)}
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
	// withdraw asks rule "command" for a party withdrawal ahead of the
	// reply. It is set only in a party (unparty), dropped unsent once the
	// bot is found in no party or answers an invitation, and kept by the
	// resets that keep the reply (docs/parties/orders.md, "Withdrawal and
	// reply in order").
	withdraw bool
	// whispered is set while a GM's whispered follow, assist, stay or go
	// holds the session; a panel order replaces it.
	whispered bool
}

// heard takes a chat line at now: a whisper from another player that
// parses as a command goes to the authority, other lines never do; any
// other whisper may get the valuables responder's reply (whisperHeard) or
// a chat reply (chatHeard), a command never does,
// whoever sent it. A craft order, by whisper or in clan chat, goes to
// heardCraft: the clan leader's needs no authority. A join goes to
// heardJoin and never to the authority; a bot with no party slot takes it
// as chat. The sender is the line's object id, which the server fills
// from the speaker.
func (b *Brain) heard(e CreatureSay, now time.Time) {
	if e.ObjectID == 0 || e.ObjectID == b.world.Self.ObjectID || strings.HasPrefix(e.Name, echoPrefix) {
		return
	}
	switch e.Type {
	case chatTell:
	case chatClan:
		b.heardClan(e, now)
		return
	default:
		return
	}
	cmd, ok := parseCommand(e.Text)
	if ok && (cmd.kind == cmdJoin && !b.slotted() || cmd.kind == cmdDuel && !b.persona.PvP.Duels) {
		ok = false
	}
	if !ok {
		if !b.whisperHeard(e, now) {
			b.chatHeard(e, now)
		}
		return
	}
	t := tell{sender: e.ObjectID, name: e.Name, cmd: cmd}
	switch cmd.kind {
	case cmdCraft:
		b.heardCraft(t, now)
		return
	case cmdJoin:
		b.heardJoin(t, now)
		return
	case cmdDuel:
		b.heardDuel(t, now)
		return
	}
	b.askAuthority(t, now)
}

// askAuthority asks the authority about t at now and obeys an answer it
// gives at once; a database check's answer comes later (verdicts). With no
// authority every command is refused, so an invite me takes the player
// path (playerInviteMe).
func (b *Brain) askAuthority(t tell, now time.Time) {
	if b.cfg.Authority == nil {
		if t.cmd.kind == cmdInviteMe {
			b.playerInviteMe(t, now)
		}
		return
	}
	if v, done := b.cfg.Authority.ask(t, now, b.verdicts); done {
		b.obey(v, now)
	}
}

// obey carries out an authorized command at now and reacts to it. A
// refused one is dropped, but a refused invite me takes the player path
// (playerInviteMe): a failed check counts as refused.
func (b *Brain) obey(v verdict, now time.Time) {
	if !v.ok {
		if v.cmd.kind == cmdInviteMe {
			b.playerInviteMe(v.tell, now)
		}
		return
	}
	o := &b.orders
	switch v.cmd.kind {
	case cmdFollow, cmdAssist, cmdStay, cmdGo, cmdInviteMe, cmdLeave:
		// The order, and a GM's invite me or leave, take the bot out of its
		// bot party and clear its slot first: ruleCommand sends the
		// withdrawal ahead of any move or reply.
		b.clearSlot()
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
		// internal/gameserver/network/partymatch.go, withdrawParty), so the
		// bot sends one only from a party, through the withdraw flag: a
		// disband before it goes drops it.
		if b.world.Party.In() {
			o.withdraw = true
		}
	case cmdCraft:
		b.craftOrder(v.tell, OrderByGM, now)
	case cmdLogout:
		b.logout.commanded = true
		if !b.logout.requested {
			b.requestLogout(now)
		}
	}
	switch v.cmd.kind {
	case cmdFollow, cmdAssist, cmdStay, cmdGo:
		// A whispered order replaces the panel's standing one for the
		// session, and holds it against a lead or a join (ErrWhispered).
		b.orderPending, o.whispered = false, true
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
	b.spot.idx, b.spot.held, b.spot.at, b.spot.retryAt, b.spot.weight = i, true, b.spotDest(i, now), now, 1
	// An order to a spot overrides the town preparation gate: a gate left
	// armed would top supplies up to their targets at the spot. It ends a
	// town bot's town spell as well (farmNow).
	b.prep = townPrep{}
	b.farmNow(now)
	return nil
}

// ruleCommand sends the one-off packet a command asked for, ahead of every
// errand: a party withdrawal (orders.withdraw) first, then the reply. While
// the pacer holds it back (a whisper inside the chat gate) it passes, so the
// survival rules below it still run. A withdrawal the bot is found in no
// party for is dropped, with the leader watch: the party it was for is gone.
func (b *Brain) ruleCommand(now time.Time) (Action, bool) {
	if b.orders.withdraw {
		if !b.world.Party.In() {
			b.orders.withdraw, b.lead = false, leaderWatch{}
		} else {
			// RequestWithdrawParty has no server gate but the frame bucket
			// (server network/partymatch.go, withdrawParty).
			a := leavePartyAction()
			if !b.pace.allow(a.gate(), now) {
				return Action{}, false
			}
			return a, true
		}
	}
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
