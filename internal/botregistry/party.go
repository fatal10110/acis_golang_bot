package botregistry

import (
	"cmp"

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
	s.party = planPass(s.party, s.candidates(), *s.cfg.Party, s.cfg.Clock.Now(), s.cfg.Metrics, s.log, s.partyLink)
	s.unpin(s.party.Unpin)
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
		st.party.poll()
		c := partyCandidate(b.Account, b.Preset, st.bot.Status(), &st.party)
		c.Character = cmp.Or(c.Character, b.Name)
		c.Mode, c.Group, c.Lead = b.PartyMode, b.PartyGroup, b.PartyLead
		out = append(out, c)
	}
	return out
}

// partyLink is the organizer's record and the bot of account's live run
// (planPass); an account with no live run has neither, and its record waits
// for one.
func (s *Supervisor) partyLink(account string) (*partyLink, Orderable) {
	st := s.runs[account]
	if st == nil || st.bot == nil || st.stopping {
		return nil, nil
	}
	return &st.party, st.bot
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
