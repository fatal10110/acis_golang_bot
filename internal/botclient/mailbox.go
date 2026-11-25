package botclient

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// mailboxSize bounds a Bot's mailbox: commands past it answer ErrBusy.
const mailboxSize = 4

// CommandKind is an order of the control panel (docs/plan.md section 8,
// "Commands").
type CommandKind uint8

// The panel's commands. Stay, go, follow and assist are standing orders: the
// Bot keeps the latest one and applies it again at every session start.
// Leave and logout are one-off. Lead and join are standing too, in the
// Bot's party slot beside its standing order; unparty, which clears the
// slot, is one-off (docs/parties/orders.md). The panel's raw command route
// takes only stay to logout.
const (
	CommandStay CommandKind = iota + 1
	CommandGo
	CommandFollow
	CommandAssist
	CommandLeave
	CommandLogout
	CommandLead
	CommandJoin
	CommandUnparty
)

var commandNames = [...]string{
	CommandStay:    "stay",
	CommandGo:      "go",
	CommandFollow:  "follow",
	CommandAssist:  "assist",
	CommandLeave:   "leave",
	CommandLogout:  "logout",
	CommandLead:    "lead",
	CommandJoin:    "join",
	CommandUnparty: "unparty",
}

func (k CommandKind) String() string {
	if int(k) < len(commandNames) && commandNames[k] != "" {
		return commandNames[k]
	}
	return fmt.Sprintf("CommandKind(%d)", uint8(k))
}

// Command is an order of the control panel to one bot. Arg is the spot of
// a go and the player's name of a follow or an assist, the character names
// of a lead or a join (the leader first), single-spaced, and empty for the
// others. The zero Command is no order.
type Command struct {
	Kind CommandKind
	Arg  string
}

// String is the command as the panel writes it: "stay", "go Wolves",
// "follow Name"; "" for no order.
func (c Command) String() string {
	switch {
	case c.Kind == 0:
		return ""
	case c.Arg == "":
		return c.Kind.String()
	}
	return c.Kind.String() + " " + c.Arg
}

// valid reports a known command with the argument it takes.
func (c Command) valid() bool {
	switch c.Kind {
	case CommandStay, CommandLeave, CommandLogout, CommandUnparty:
		return c.Arg == ""
	case CommandGo, CommandFollow, CommandAssist:
		return strings.TrimSpace(c.Arg) == c.Arg && c.Arg != ""
	case CommandLead, CommandJoin:
		return validPartyNames(c.Arg)
	}
	return false
}

// maxPartyNames is the most names a lead or a join takes: the other members
// of a full party (maxPartyMembers).
const maxPartyNames = maxPartyMembers - 1

// validPartyNames reports 1 to maxPartyNames character names, single-spaced,
// each 1 to 16 ASCII letters or digits, none twice in any case.
func validPartyNames(arg string) bool {
	names := strings.Split(arg, " ")
	if len(names) > maxPartyNames {
		return false
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		k := strings.ToLower(n)
		if !validPartyName(n) || seen[k] {
			return false
		}
		seen[k] = true
	}
	return true
}

// validPartyName reports 1 to 16 ASCII letters or digits.
func validPartyName(n string) bool {
	if n == "" || len(n) > 16 {
		return false
	}
	for _, r := range n {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// Errors Bot.Command returns at once, and the answers of commands a
// session's brain could not carry out.
var (
	// ErrBusy answers a command while the bot's mailbox is full.
	ErrBusy = errors.New("bot busy")
	// ErrStopped answers a command to a bot whose Run returned, or one
	// still waiting in its mailbox then.
	ErrStopped = errors.New("bot stopped")
	// ErrBadCommand answers an unknown command or one with the wrong
	// argument.
	ErrBadCommand = errors.New("bad command")
	// ErrNoSpots, ErrUnknownSpot and ErrSpotFull answer a go the bot
	// cannot carry out (spotReply is the whispered reply of each).
	ErrNoSpots     = errors.New("no spots known")
	ErrUnknownSpot = errors.New(replyUnknownSpot)
	ErrSpotFull    = errors.New(replySpotFull)
	// ErrNotInSight answers a follow or an assist sent to a running
	// session when no player of that name is in sight.
	ErrNotInSight = errors.New("player not in sight")
	// ErrNoParty answers a leave while the bot is in no party.
	ErrNoParty = errors.New("not in a party")
	// ErrWhispered answers a lead or a join while a GM's whispered follow,
	// assist, stay or go holds the session (Status.Whispered).
	ErrWhispered = errors.New("held by a GM's whispered order")
)

// mail is a command in a Bot's mailbox. queued is set when it came while no
// session ran: a follow or an assist then waits for its player to come in
// sight instead of being refused. answer has room for the one answer.
//
// A mail with inspect set is an inspect instead (Bot.Inspect): the brain
// answers it on inspect, which has room for the one Inspection, by
// deadline on the bot's clock.
type mail struct {
	cmd    Command
	queued bool
	answer chan error

	inspect  chan Inspection
	deadline time.Time
}

// standing is a Bot's standing order: the latest stay, go, follow or assist
// of the panel, owned by Run's goroutine. id is the object id of the player
// of a follow or an assist, 0 until it was seen.
type standing struct {
	cmd Command
	id  int32
}

// botLink is what a Bot lends each session's brain: its mailbox, its
// standing order, its party slot, its status board and the crafter's last
// cycle end. The brain runs on Run's goroutine, so it writes the order, the
// slot, the cycle end and publishes the status as their one owner. A Bot
// plays one run, so a new run starts with no slot and no known leader.
type botLink struct {
	mail  chan mail
	order standing
	// party is the party slot: the latest lead or join of the panel, the
	// zero Command for none (docs/parties/orders.md). leaders are the
	// leaders every join of this run named (slotAnswer).
	party   Command
	leaders map[string]bool
	// partyLeft is why the brain last cleared the slot itself
	// (Status.PartyLeft), "" while a slot is set or after an unparty.
	partyLeft string
	board     statusBoard
	// crafter is how the crafter's last cycle ended and until when its
	// breaker holds: the next session's brain resumes from it.
	crafter craftEnded
}

// Command hands c to the bot and returns the channel its one answer comes
// on: nil once a session carried it out, else why not (ErrNoSpots,
// ErrUnknownSpot, ErrSpotFull, ErrNotInSight, ErrNoParty, ErrWhispered,
// ErrStopped). It
// never waits for the bot: a full mailbox answers ErrBusy, a stopped bot
// ErrStopped. A command that comes between sessions waits for the next
// one. No command sends a whisper: every packet goes out through the
// brain's rules and pacer ("stay", "command", the logout steps).
func (b *Bot) Command(c Command) (<-chan error, error) {
	if !c.valid() {
		return nil, fmt.Errorf("%w: %q", ErrBadCommand, c.String())
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return nil, ErrStopped
	}
	m := mail{cmd: c, queued: !b.inSession, answer: make(chan error, 1)}
	select {
	case b.link.mail <- m:
		return m.answer, nil
	default:
		return nil, ErrBusy
	}
}

// setInSession records whether a session's brain runs, for the commands
// that come meanwhile (mail.queued).
func (b *Bot) setInSession(in bool) {
	b.mu.Lock()
	b.inSession = in
	b.mu.Unlock()
}

// stop marks the bot stopped and refuses the commands left in its mailbox,
// closing the reply of an inspect left there: no command comes in after it.
func (b *Bot) stop() {
	b.mu.Lock()
	b.stopped = true
	b.mu.Unlock()
	for {
		select {
		case m := <-b.link.mail:
			if m.inspect != nil {
				close(m.inspect)
				continue
			}
			m.answer <- ErrStopped
		default:
			return
		}
	}
}

// command carries out a panel command at now and answers it, or takes an
// inspect.
func (b *Brain) command(m mail, now time.Time) {
	if m.inspect != nil {
		b.inspectAsked(m, now)
		return
	}
	err := b.panelOrder(m.cmd, m.queued, now)
	m.answer <- err
	if err == nil {
		b.wakeBy(now.Add(b.reactDelay()))
	}
}

// panelOrder carries out c at now, a standing order in place of the bot's
// orders and its Bot's standing one. A follow or an assist resolves its
// player in sight by name; queued, it waits for the player instead of
// being refused. A standing order carried out and a leave clear the party
// slot first (clearSlot); a refused one keeps it.
func (b *Brain) panelOrder(c Command, queued bool, now time.Time) error {
	o := &b.orders
	switch c.Kind {
	case CommandStay:
		b.clearSlot()
		b.holdStill()
	case CommandGo:
		if err := b.goToSpot(c.Arg, now); err != nil {
			return err
		}
		b.clearSlot()
		*o = orders{roam: true, reply: o.reply, withdraw: o.withdraw}
	case CommandFollow, CommandAssist:
		id, name, ok := b.playerInSight(0, c.Arg)
		if !ok && !queued {
			return ErrNotInSight
		}
		if ok {
			c.Arg = name
		}
		b.clearSlot()
		b.orderPlayer(c.Kind, id)
		b.orderPending = !ok
		b.cfg.link.order = standing{cmd: c, id: id}
		return nil
	case CommandLeave:
		// A partyless bot holding a slot clears it and answers nil. The
		// server ignores a withdrawal from no party (server
		// internal/gameserver/network/partymatch.go, withdrawParty), so the
		// bot sends one only from a party, through the withdraw flag: a
		// disband before it goes drops it.
		cleared := b.clearSlot()
		if !b.world.Party.In() {
			if cleared {
				return nil
			}
			return ErrNoParty
		}
		o.withdraw = true
		return nil
	case CommandLogout:
		b.logout.commanded = true
		if !b.logout.requested {
			b.requestLogout(now)
		}
		return nil
	case CommandLead, CommandJoin:
		if o.whispered {
			return ErrWhispered
		}
		b.setPartySlot(c)
		return nil
	case CommandUnparty:
		b.unparty()
		return nil
	}
	b.orderPending = false
	b.cfg.link.order = standing{cmd: c}
	return nil
}

// holdStill makes the bot stay where it stands, dropping its other orders.
func (b *Brain) holdStill() {
	b.orders = orders{stay: true, reply: b.orders.reply, withdraw: b.orders.withdraw}
	b.assist = assistPlan{}
}

// orderPlayer makes the player id, 0 for one not seen yet, the one the bot
// follows or assists (kind), dropping its other orders.
func (b *Brain) orderPlayer(kind CommandKind, id int32) {
	o := &b.orders
	*o = orders{reply: o.reply, withdraw: o.withdraw}
	if kind == CommandFollow {
		o.follow = id
	} else {
		o.assist = id
	}
}

// applyStanding gives a new session its Bot's standing order. A go to a
// spot now full leaves the bot to pick its own; a follow or an assist waits
// for its player to come in sight (resolveOrder).
func (b *Brain) applyStanding(now time.Time) {
	if b.cfg.link == nil {
		return
	}
	switch s := b.cfg.link.order; s.cmd.Kind {
	case CommandStay:
		b.holdStill()
	case CommandGo:
		b.orders.roam = true
		_ = b.goToSpot(s.cmd.Arg, now)
	case CommandFollow, CommandAssist:
		b.orderPending = true
	}
}

// resolveOrder follows or assists the player of a standing order not
// resolved yet in this session once it is in sight, found by its object id
// or its name, and keeps both in the standing order.
func (b *Brain) resolveOrder() {
	if !b.orderPending {
		return
	}
	s := &b.cfg.link.order
	id, name, ok := b.playerInSight(s.id, s.cmd.Arg)
	if !ok {
		return
	}
	b.orderPending = false
	s.id, s.cmd.Arg = id, name
	b.orderPlayer(s.cmd.Kind, id)
}

// playerInSight finds a player in sight: the one of object id id when it is
// one, else the one named name in any case. It returns its id and name.
func (b *Brain) playerInSight(id int32, name string) (int32, string, bool) {
	if o, ok := b.world.Objects[id]; ok && o.Kind == KindPlayer {
		return id, o.Name, true
	}
	for oid, o := range b.world.Objects {
		if o.Kind == KindPlayer && strings.EqualFold(o.Name, name) {
			return oid, o.Name, true
		}
	}
	return 0, "", false
}
