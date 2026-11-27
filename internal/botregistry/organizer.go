package botregistry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
)

// Orderable is a bot as the bot party organizer orders it: a
// *botclient.Bot, or a test's fake whose status and answers the test sets.
type Orderable interface {
	Status() botclient.Status
	Command(c botclient.Command) (<-chan error, error)
}

// partyLink is the organizer's record of one bot's run (docs/parties/
// organizer.md, "Which orders arrived"): newRun marks a run handed over
// since the last pass, sent the last order handed to its bot and what
// became of it, answer its answer channel while the answer is pending.
type partyLink struct {
	newRun bool
	sent   botparty.Sent
	answer <-chan error
}

// poll takes the answer of the pending order, if it came, without waiting.
func (l *partyLink) poll() {
	if l.answer == nil {
		return
	}
	select {
	case err := <-l.answer:
		l.answer = nil
		l.sent.Answer = botparty.AnswerOK
		if err != nil {
			l.sent.Answer = botparty.AnswerError
		}
	default:
	}
}

// partyCommands are the bot commands of the organizer's orders.
var partyCommands = map[botparty.OrderKind]botclient.CommandKind{
	botparty.OrderLead:    botclient.CommandLead,
	botparty.OrderJoin:    botclient.CommandJoin,
	botparty.OrderUnparty: botclient.CommandUnparty,
}

// order hands o to b and records it as the run's Sent: pending with its
// answer channel, not handed at ErrBusy (the next pass plans it again),
// refused at ErrBadCommand, an error otherwise. Bot.Command never waits.
func (l *partyLink) order(b Orderable, o botparty.Order, log *slog.Logger) {
	ch, err := b.Command(botclient.Command{Kind: partyCommands[o.Kind], Arg: strings.Join(o.Names, " ")})
	l.sent, l.answer = botparty.Sent{Order: o}, nil
	switch {
	case err == nil:
		l.sent.Answer, l.answer = botparty.AnswerPending, ch
		log.Info("party order", "account", o.Account, "order", o.Slot())
	case errors.Is(err, botclient.ErrBusy):
		log.Info("party order deferred: mailbox full", "account", o.Account, "order", o.Slot())
	case errors.Is(err, botclient.ErrBadCommand):
		l.sent.Answer = botparty.AnswerBadCommand
		log.Error("party order refused", "account", o.Account, "order", o.Slot(), "err", err)
	default:
		l.sent.Answer = botparty.AnswerError
		log.Info("party order not handed", "account", o.Account, "order", o.Slot(), "err", err)
	}
}

// partyCandidate is account's run of preset as the organizer reads it this
// pass: its snapshot v and its record l. The registry pin and mode are the
// caller's to add.
func partyCandidate(account, preset string, v botclient.Status, l *partyLink) botparty.Candidate {
	c := botparty.Candidate{
		Account:   account,
		Character: v.Character,
		Preset:    preset,
		ClassID:   v.ClassID,
		Level:     v.Level,
		Heals:     v.Heals,
		Position:  partyPoint(v.Position),
		HP:        v.HP,
		Online:    v.State == botclient.StateOnline,
		Leader:    v.Leader,
		Members:   v.Members,
		Spot:      v.Spot,
		Slot:      v.Party,
		PartyLeft: v.PartyLeft,
		NewRun:    l.newRun,
		Sent:      l.sent,
		Order:     v.Order.String(),
		Whispered: v.Whispered,
	}
	if v.Town != nil {
		t := partyPoint(*v.Town)
		c.Town = &t
	}
	return c
}

func partyPoint(p botclient.Point) botparty.Point {
	return botparty.Point{X: p.X, Y: p.Y, Z: p.Z}
}

// countParties counts the parties a pass formed, those of an id from next
// on, and broke (the parties a pass returns broken are its own: the next
// pass forgets them) into m, when set.
func countParties(st botparty.State, next int, m *botmetrics.Registry, log *slog.Logger) {
	for _, pt := range st.Parties {
		if pt.ID >= next && m != nil {
			m.PartyFormed()
		}
		if pt.State == botparty.Broken {
			log.Info("party broken", "id", pt.ID, "leader", pt.Leader.Account, "why", pt.Why)
			if m != nil {
				m.PartyBroken(pt.Why)
			}
		}
	}
}

// planPass is the step of an organizer pass the supervisor's (partyPass)
// and the Organizer's share (docs/parties/organizer.md): it plans cands
// over st with policy at now, counts the parties formed and broken into m,
// clears the new-run mark of every candidate's record and hands each order
// to its account's bot. link resolves an account to its record and its bot,
// both nil for an account with no live bot, which gets no order. planPass
// holds no state and starts nothing: it runs on the caller's goroutine,
// which owns st and the records, and returns the state for the caller to
// keep, whose Unpin the caller applies or ignores.
func planPass(st botparty.State, cands []botparty.Candidate, policy botparty.Policy, now time.Time, m *botmetrics.Registry, log *slog.Logger, link func(account string) (*partyLink, Orderable)) botparty.State {
	next := st.NextID
	st, orders := botparty.Plan(st, cands, policy, now)
	countParties(st, next, m, log)
	for _, c := range cands {
		if l, _ := link(c.Account); l != nil {
			l.newRun = false
		}
	}
	for _, o := range orders {
		if l, b := link(o.Account); l != nil && b != nil {
			l.order(b, o, log)
		}
	}
	return st
}

// OrganizedBot is a bot of a fixed set the Organizer orders: its account
// and the preset it plays.
type OrganizedBot struct {
	Account string
	Preset  string
	Bot     Orderable
}

// OrganizerConfig configures an Organizer.
type OrganizerConfig struct {
	// Bots are the bots organized, each running on its own goroutine for
	// the Organizer's whole run.
	Bots   []OrganizedBot
	Policy botparty.Policy
	Clock  botclient.Clock
	// Metrics, when set, counts the parties formed and broken.
	Metrics *botmetrics.Registry
	// Log receives the organizer's reports; nil discards them.
	Log *slog.Logger
}

// Organizer is the bot party organizer of a fixed set of bots with no
// registry: `bots swarm`'s (D25). It plans as the supervisor does
// (docs/parties/organizer.md), every PartyTick, through botparty.Plan,
// which forms at most FormPerPass parties a pass and keeps one invitation
// at a time; orders go through Bot.Command, which never waits. It sends no
// packet itself: each bot's brain paces what an order causes
// (docs/parties/server.md, "Pacing"). With no registry there is no pin and
// no party mode: Plan's Unpin is ignored.
type Organizer struct {
	cfg OrganizerConfig
	log *slog.Logger
	// Owned by Run.
	links map[string]*partyLink
	bots  map[string]Orderable
	state botparty.State
}

// NewOrganizer returns the organizer of cfg; Run starts it.
func NewOrganizer(cfg OrganizerConfig) (*Organizer, error) {
	if cfg.Clock == nil {
		return nil, errors.New("organizer: a clock is required")
	}
	if cfg.Policy.Size < 2 || cfg.Policy.Size > botparty.MaxSize {
		return nil, fmt.Errorf("organizer: party size %d: want 2 to %d", cfg.Policy.Size, botparty.MaxSize)
	}
	o := &Organizer{
		cfg: cfg, log: cfg.Log, links: make(map[string]*partyLink, len(cfg.Bots)),
		bots: make(map[string]Orderable, len(cfg.Bots)),
	}
	if o.log == nil {
		o.log = slog.New(slog.DiscardHandler)
	}
	for _, b := range cfg.Bots {
		if b.Bot == nil || b.Account == "" {
			return nil, errors.New("organizer: every bot needs an account and a bot")
		}
		if o.links[b.Account] != nil {
			return nil, fmt.Errorf("organizer: account %s twice", b.Account)
		}
		// Every bot starts a new run: it holds no order yet.
		o.links[b.Account] = &partyLink{newRun: true}
		o.bots[b.Account] = b.Bot
	}
	return o, nil
}

// Run plans the parties every PartyTick on its injected clock until ctx is
// done, then stops its timer and returns. Call it once, on the goroutine
// that owns the Organizer.
func (o *Organizer) Run(ctx context.Context) {
	t := o.cfg.Clock.NewTimer(PartyTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C():
			o.pass()
			t.Reset(PartyTick)
		}
	}
}

// pass is one organizer pass: it polls the answers of the orders handed
// out, reads every bot's snapshot, plans and hands the orders out.
func (o *Organizer) pass() {
	cands := make([]botparty.Candidate, 0, len(o.cfg.Bots))
	for _, b := range o.cfg.Bots {
		l := o.links[b.Account]
		l.poll()
		cands = append(cands, partyCandidate(b.Account, b.Preset, b.Bot.Status(), l))
	}
	o.state = planPass(o.state, cands, o.cfg.Policy, o.cfg.Clock.Now(), o.cfg.Metrics, o.log, o.link)
}

// link is the record and the bot of account (planPass); an account of no
// bot of the set has neither.
func (o *Organizer) link(account string) (*partyLink, Orderable) {
	b := o.bots[account]
	if b == nil {
		return nil, nil
	}
	return o.links[account], b
}
