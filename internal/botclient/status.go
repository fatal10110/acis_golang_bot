package botclient

import (
	"cmp"
	"fmt"
	"slices"
	"sync/atomic"
	"time"
)

// statusEvery is the least time between two snapshots a session's brain
// publishes.
const statusEvery = time.Second

// BotState is where a Bot is in its loop of sessions.
type BotState uint8

// The states of a Bot.
const (
	// StateNew is a bot whose Run has not started.
	StateNew BotState = iota
	// StateWaiting waits for its turn of the login gate.
	StateWaiting
	// StateEntering logs in and enters the world.
	StateEntering
	// StateOnline plays a session in the world.
	StateOnline
	// StateReconnecting waits before its next attempt; Status.Err is why
	// the last one ended.
	StateReconnecting
	// StateStopped is a bot whose Run returned; Status.Err is why.
	StateStopped
)

var stateNames = [...]string{
	StateNew:          "new",
	StateWaiting:      "waiting for the gate",
	StateEntering:     "entering",
	StateOnline:       "online",
	StateReconnecting: "reconnecting",
	StateStopped:      "stopped",
}

func (s BotState) String() string {
	if int(s) < len(stateNames) {
		return stateNames[s]
	}
	return fmt.Sprintf("BotState(%d)", uint8(s))
}

// Status is a snapshot of a Bot (Bot.Status). It is immutable once
// published: Members is shared by every copy and must not be changed.
// Between sessions it keeps what the last session saw of the character.
type Status struct {
	// At is when it was taken, on the bot's clock.
	At    time.Time
	State BotState
	// Character is the character's name, "" until known.
	Character      string
	ClassID, Level int32
	HP, MaxHP      int32
	MP, MaxMP      int32
	CP, MaxCP      int32
	Position       Point
	// Town is the town of the travel map nearest to Position, nil without
	// a travel map (BrainConfig.Travel).
	Town *Point
	// Preset is the persona preset the bot plays (BotConfig.Preset).
	Preset string
	// Rule is the rule of the latest decision while online.
	Rule string
	// Leader is the party leader's name, the bot's own when it leads, and
	// Members the other members' names in order; empty outside a party.
	Leader  string
	Members []string
	// Order is the standing order of the panel, the zero Command for none.
	Order Command
	// Reconnects counts the bot's attempts after its first.
	Reconnects int
	// Err is why the latest attempt ended, or why the bot stopped; nil
	// while none ended.
	Err error
}

// statusBoard is a Bot's published snapshot. Run's goroutine is its one
// writer; any goroutine reads it.
type statusBoard struct {
	cur atomic.Pointer[Status]
	// at is the brain's last publication, owned by the writer.
	at time.Time
}

func (s *statusBoard) load() Status    { return *s.cur.Load() }
func (s *statusBoard) store(st Status) { s.cur.Store(&st) }

// Status is the bot's latest snapshot. It is safe from any goroutine.
func (b *Bot) Status() Status { return b.link.board.load() }

// publish publishes the bot's offline state, keeping what the last session
// saw; err, when set, becomes the snapshot's error. It runs on Run's
// goroutine.
func (b *Bot) publish(state BotState, err error) {
	st := b.link.board.load()
	st.At, st.State, st.Rule = b.cfg.Clock.Now(), state, ""
	st.Character, st.Order, st.Reconnects = cmp.Or(b.char.Name, st.Character), b.link.order.cmd, b.reconnects
	if err != nil {
		st.Err = err
	}
	b.link.board.store(st)
}

// publishStatus publishes the bot's snapshot online at now: at once when
// force, else at most once every statusEvery.
func (b *Brain) publishStatus(now time.Time, force bool) {
	l := b.cfg.link
	if l == nil || !force && now.Sub(l.board.at) < statusEvery {
		return
	}
	l.board.at = now
	st, w := l.board.load(), b.world
	s := &w.Self
	st.At, st.State, st.Rule, st.Order = now, StateOnline, b.lastRule, l.order.cmd
	st.Character, st.ClassID, st.Level = s.Name, s.ClassID, s.Level
	st.HP, st.MaxHP, st.MP, st.MaxMP, st.CP, st.MaxCP = s.HP, s.MaxHP, s.MP, s.MaxMP, s.CP, s.MaxCP
	st.Position = s.Position(now)
	st.Town = b.nearestTown(st.Position)
	st.Leader, st.Members = "", nil
	if p := &w.Party; p.In() {
		if p.LeaderID == s.ObjectID {
			st.Leader = s.Name
		} else if m := p.Members[p.LeaderID]; m != nil {
			st.Leader = m.Name
		}
		for _, m := range p.Members {
			st.Members = append(st.Members, m.Name)
		}
		slices.Sort(st.Members)
	}
	l.board.store(st)
}

// nearestTown is the town of the travel map nearest to at, nil without a
// travel map or towns.
func (b *Brain) nearestTown(at Point) *Point {
	if b.cfg.Travel == nil {
		return nil
	}
	if b.towns == nil {
		b.towns = b.cfg.Travel.Towns()
	}
	var best *Point
	for i := range b.towns {
		if best == nil || dist2D(at, b.towns[i]) < dist2D(at, *best) {
			best = &b.towns[i]
		}
	}
	if best == nil {
		return nil
	}
	t := *best
	return &t
}
