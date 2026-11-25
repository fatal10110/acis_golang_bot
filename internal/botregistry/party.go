package botregistry

import (
	"cmp"
	"errors"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
)

// partyPass is one pass of the bot party organizer (docs/parties/organizer.md,
// "Supervisor wiring"), on the supervisor goroutine, which owns its State: it
// polls the answers of the orders handed out, reads every live run's
// snapshot, plans, hands the orders to the bots and clears the registry pins
// the pass ended. It never waits on a bot: Bot.Command answers at once. It
// sends no packet itself; each bot's brain paces what an order causes
// (invites: docs/parties/server.md, "Pacing").
func (s *Supervisor) partyPass() {
	cands := s.candidates()
	next := s.party.NextID
	st, orders := botparty.Plan(s.party, cands, *s.cfg.Party, s.cfg.Clock.Now())
	s.party = st
	s.countParties(st, next)
	for _, c := range cands {
		s.runs[c.Account].newRun = false
	}
	for _, o := range orders {
		s.order(o)
	}
	s.unpin(st.Unpin)
}

// countParties counts the parties the pass formed, those of an id from
// next on, and broke (the parties a pass returns broken are its own: the
// next pass forgets them).
func (s *Supervisor) countParties(st botparty.State, next int) {
	for _, pt := range st.Parties {
		if pt.ID >= next && s.cfg.Metrics != nil {
			s.cfg.Metrics.PartyFormed()
		}
		if pt.State == botparty.Broken {
			s.log.Info("party broken", "id", pt.ID, "leader", pt.Leader.Account, "why", pt.Why)
			if s.cfg.Metrics != nil {
				s.cfg.Metrics.PartyBroken(pt.Why)
			}
		}
	}
}

// candidates are the registry's bots with a live run as the organizer reads
// them this pass, in registry order. A run's answer is polled before its
// snapshot is read: the brain publishes the slot it set before it answers.
func (s *Supervisor) candidates() []botparty.Candidate {
	var out []botparty.Candidate
	for _, b := range s.cfg.Registry.Bots() {
		st := s.runs[b.Account]
		if st == nil || st.bot == nil || st.stopping {
			continue
		}
		st.poll()
		v := st.bot.Status()
		c := botparty.Candidate{
			Account:   b.Account,
			Character: cmp.Or(v.Character, b.Name),
			Preset:    b.Preset,
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
			NewRun:    st.newRun,
			Sent:      st.sent,
			Order:     v.Order.String(),
			Whispered: v.Whispered,
			Mode:      b.PartyMode,
			Group:     b.PartyGroup,
			Lead:      b.PartyLead,
		}
		if v.Town != nil {
			t := partyPoint(*v.Town)
			c.Town = &t
		}
		out = append(out, c)
	}
	return out
}

func partyPoint(p botclient.Point) botparty.Point {
	return botparty.Point{X: p.X, Y: p.Y, Z: p.Z}
}

// poll takes the answer of the run's pending order, if it came, without
// waiting.
func (st *runState) poll() {
	if st.answer == nil {
		return
	}
	select {
	case err := <-st.answer:
		st.answer = nil
		st.sent.Answer = botparty.AnswerOK
		if err != nil {
			st.sent.Answer = botparty.AnswerError
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

// order hands o to the bot of its account's live run and records it as the
// run's Sent: pending with its answer channel, not handed at ErrBusy (the
// next pass plans it again), refused at ErrBadCommand, an error otherwise.
// A bot with no live run gets nothing: its record waits for one.
func (s *Supervisor) order(o botparty.Order) {
	st := s.runs[o.Account]
	if st == nil || st.bot == nil || st.stopping {
		return
	}
	ch, err := st.bot.Command(botclient.Command{Kind: partyCommands[o.Kind], Arg: strings.Join(o.Names, " ")})
	st.sent, st.answer = botparty.Sent{Order: o}, nil
	switch {
	case err == nil:
		st.sent.Answer, st.answer = botparty.AnswerPending, ch
		s.log.Info("party order", "account", o.Account, "order", o.Slot())
	case errors.Is(err, botclient.ErrBusy):
		s.log.Info("party order deferred: mailbox full", "account", o.Account, "order", o.Slot())
	case errors.Is(err, botclient.ErrBadCommand):
		st.sent.Answer = botparty.AnswerBadCommand
		s.log.Error("party order refused", "account", o.Account, "order", o.Slot(), "err", err)
	default:
		st.sent.Answer = botparty.AnswerError
		s.log.Info("party order not handed", "account", o.Account, "order", o.Slot(), "err", err)
	}
}

// unpin clears the registry pin of each account, saved at once, so no pass
// and no restart forms it again (organizer.md, "Pinned parties").
func (s *Supervisor) unpin(accounts []string) {
	for _, a := range accounts {
		b, ok := s.cfg.Registry.Bot(a)
		if !ok || b.PartyGroup == "" && !b.PartyLead {
			continue
		}
		if err := s.cfg.Registry.Update(a, func(b *Bot) { b.PartyGroup, b.PartyLead = "", false }); err != nil {
			s.log.Error("registry update failed", "account", a, "err", err)
			continue
		}
		s.log.Info("bot left its pinned party", "account", a, "group", b.PartyGroup)
	}
}
