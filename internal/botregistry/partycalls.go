package botregistry

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
)

var (
	// ErrNoOrganizer refuses a party call to a supervisor with no party
	// organizer (Config.Party nil: a swarm).
	ErrNoOrganizer = errors.New("no party organizer")
	// ErrNoParty refuses a call for a party the organizer has no live
	// record of.
	ErrNoParty = errors.New("no such party")
	// ErrBadParty refuses a malformed party call: a member count out of 1
	// to botparty.MaxSize-1, a bot named twice, an unknown party mode.
	ErrBadParty = errors.New("bad party request")
	// ErrPartyRefused refuses a hand-formed party one of its bots cannot
	// join now (docs/parties/panel.md, "FormParty").
	ErrPartyRefused = errors.New("party refused")
)

// PartyBot is a bot of a party or of the free pool as the panel shows it:
// its class, level and role from its own snapshot, else from its registry
// row.
type PartyBot struct {
	Account   string
	Character string
	ClassID   *int32
	Level     int32
	Preset    string
	// Role is the build's role (botclient.Role), "" without a build.
	Role   string
	Online bool
	// Slot is the bot's party slot (botclient.Status.Party).
	Slot string
	// Town is the town of the travel map nearest the bot, nil for none.
	Town *botclient.Point
}

// PartyInfo is one bot party of the organizer.
type PartyInfo struct {
	ID      int
	Leader  PartyBot
	Members []PartyBot
	// Players are the leader's live party members that are no bot of the
	// party: names only.
	Players []string
	// Spot is the leader's spot, held or farmed beside.
	Spot     string
	State    string
	Why      string
	Since    time.Time
	FormedAt time.Time
	Pinned   bool
	Group    string
}

// Parties is the organizer's view: the auto switch, every party and the
// free pool, the bots that may be grouped.
type Parties struct {
	Auto    bool
	Parties []PartyInfo
	Free    []PartyBot
}

// Parties returns every party of the organizer, the free pool and the auto
// switch.
func (s *Supervisor) Parties(ctx context.Context) (Parties, error) {
	var out Parties
	err := s.request(ctx, func() error {
		if s.cfg.Party == nil {
			return ErrNoOrganizer
		}
		out = s.parties()
		return nil
	})
	return out, err
}

func (s *Supervisor) parties() Parties {
	out := Parties{Auto: !s.party.AutoOff}
	inParty := make(map[string]bool)
	for _, pt := range s.party.Parties {
		info := PartyInfo{
			ID: pt.ID, Leader: s.partyBot(pt.Leader.Account), State: pt.State.String(), Why: pt.Why,
			Since: pt.Since, FormedAt: pt.FormedAt, Pinned: pt.Pinned, Group: pt.Group,
		}
		names := []string{pt.Leader.Character}
		for _, m := range pt.Members {
			info.Members = append(info.Members, s.partyBot(m.Account))
			names = append(names, m.Character)
		}
		if v, ok := s.snapshot(pt.Leader.Account); ok {
			info.Spot = v.Spot
			if v.State == botclient.StateOnline && v.Leader == v.Character {
				for _, n := range v.Members {
					if !slices.Contains(names, n) {
						info.Players = append(info.Players, n)
					}
				}
			}
		}
		if pt.State != botparty.Broken {
			inParty[pt.Leader.Account] = true
			for _, m := range pt.Members {
				inParty[m.Account] = true
			}
		}
		out.Parties = append(out.Parties, info)
	}
	for _, b := range s.cfg.Registry.Bots() {
		v, ok := s.snapshot(b.Account)
		if !ok || inParty[b.Account] || v.State != botclient.StateOnline || b.PartyMode == botparty.ModeOff ||
			!partyPreset(b.Preset) || v.Leader != "" || v.Order.String() != "" || v.Whispered {
			continue
		}
		out.Free = append(out.Free, s.partyBot(b.Account))
	}
	return out
}

// snapshot is the latest snapshot of account's live run.
func (s *Supervisor) snapshot(account string) (botclient.Status, bool) {
	st := s.runs[account]
	if st == nil || st.bot == nil || st.stopping {
		return botclient.Status{}, false
	}
	return st.bot.Status(), true
}

func (s *Supervisor) partyBot(account string) PartyBot {
	b, _ := s.cfg.Registry.Bot(account)
	pb := PartyBot{Account: account, Character: b.Name, ClassID: b.Class, Preset: b.Preset}
	v, ok := s.snapshot(account)
	if !ok {
		return pb
	}
	pb.Character = cmp.Or(v.Character, b.Name)
	if v.Character != "" {
		pb.ClassID = &v.ClassID
		pb.Level = v.Level
	}
	if v.Build != nil {
		pb.Role = v.Build.Role.String()
	}
	pb.Online, pb.Slot = v.State == botclient.StateOnline, v.Party
	if v.Town != nil {
		t := *v.Town
		pb.Town = &t
	}
	return pb
}

func partyPreset(preset string) bool {
	return preset == botparty.PresetFarmer || preset == botparty.PresetSupport
}

// FormParty forms a party by hand: the bot of account leader leads the
// bots of members, a pin when pin is set (its registry rows written, kept
// over restarts), else an auto party that band, composition, every break
// and the auto switch govern. It refuses a bot offline, set off, in a party
// the organizer did not form, or of a preset other than farmer and support
// (D26); an auto party also a leader that is no farmer, a bot with a
// standing order and any while auto parties are off. A whispered bot is
// refused either way: no pin order replaces a whispered one. Each bot
// leaves its other party; the next pass orders the new one
// (docs/parties/panel.md). It returns the party's id.
func (s *Supervisor) FormParty(ctx context.Context, leader string, members []string, pin bool) (int, error) {
	var id int
	err := s.request(ctx, func() error {
		var err error
		id, err = s.formParty(leader, members, pin)
		return err
	})
	return id, err
}

func (s *Supervisor) formParty(leader string, members []string, pin bool) (int, error) {
	if s.cfg.Party == nil {
		return 0, ErrNoOrganizer
	}
	accounts := append([]string{leader}, members...)
	if len(members) < 1 || len(members) > botparty.MaxSize-1 {
		return 0, fmt.Errorf("%w: %d members, want 1 to %d", ErrBadParty, len(members), botparty.MaxSize-1)
	}
	for i, a := range accounts {
		if slices.Contains(accounts[:i], a) {
			return 0, fmt.Errorf("%w: %s named twice", ErrBadParty, a)
		}
	}
	if !pin && s.party.AutoOff {
		return 0, fmt.Errorf("%w: auto parties are off", ErrPartyRefused)
	}
	var lead botparty.Member
	var ms []botparty.Member
	for _, a := range accounts {
		m, err := s.formable(a, a == leader, pin)
		if err != nil {
			return 0, err
		}
		if a == leader {
			lead = m
		} else {
			ms = append(ms, m)
		}
	}
	st, id := botparty.Form(s.party, lead, ms, pin, lead.Character, s.cfg.Clock.Now())
	s.unpin(st.Unpin)
	st.Unpin = nil
	s.party = st
	if pin {
		for _, a := range accounts {
			if err := s.cfg.Registry.Update(a, func(b *Bot) { b.PartyGroup, b.PartyLead = lead.Character, a == leader }); err != nil {
				s.log.Error("registry update failed", "account", a, "err", err)
			}
		}
	}
	if s.cfg.Metrics != nil {
		s.cfg.Metrics.PartyFormed()
	}
	s.log.Info("party formed by the panel", "id", id, "leader", leader, "members", members, "pinned", pin)
	return id, nil
}

// formable is the party member of account, or why it cannot join a
// hand-formed party now.
func (s *Supervisor) formable(account string, isLeader, pin bool) (botparty.Member, error) {
	b, ok := s.cfg.Registry.Bot(account)
	if !ok {
		return botparty.Member{}, fmt.Errorf("%w: %s", ErrNoBot, account)
	}
	refuse := func(why string) (botparty.Member, error) {
		return botparty.Member{}, fmt.Errorf("%w: %s %s", ErrPartyRefused, account, why)
	}
	v, ok := s.snapshot(account)
	switch {
	case !ok || v.State != botclient.StateOnline || v.Character == "":
		return refuse("is offline")
	case b.PartyMode == botparty.ModeOff:
		return refuse("is set off")
	case !partyPreset(b.Preset):
		return refuse(fmt.Sprintf("plays %q, not farmer or support", b.Preset))
	case !pin && isLeader && b.Preset != botparty.PresetFarmer:
		return refuse("is no farmer to lead an auto party")
	case v.Whispered:
		return refuse("follows a whispered order")
	case !pin && v.Order.String() != "":
		return refuse("has a standing order")
	case v.Leader != "" && !s.organized(account, v.Leader):
		return refuse("is in a party the organizer did not form")
	}
	return botparty.Member{Account: account, Character: v.Character}, nil
}

// organized reports account in a live party of the organizer led by the
// character leader.
func (s *Supervisor) organized(account, leader string) bool {
	for _, pt := range s.party.Parties {
		if pt.State == botparty.Broken || pt.Leader.Character != leader {
			continue
		}
		if pt.Leader.Account == account || slices.ContainsFunc(pt.Members, func(m botparty.Member) bool { return m.Account == account }) {
			return true
		}
	}
	return false
}

// DissolveParty breaks the party id on the next pass: an unparty to each
// member, the leader first, one order at a time, and a pin's rows cleared.
func (s *Supervisor) DissolveParty(ctx context.Context, id int) error {
	return s.request(ctx, func() error {
		if s.cfg.Party == nil {
			return ErrNoOrganizer
		}
		st, ok := botparty.Dissolve(s.party, id)
		if !ok {
			return fmt.Errorf("%w: %d", ErrNoParty, id)
		}
		s.party = st
		s.log.Info("party dissolved by the panel", "id", id)
		return nil
	})
}

// SetPartyMode sets a bot's party mode: "" takes part in bot parties,
// botparty.ModeOff never. The next pass drops a bot set off from its party,
// pinned or not.
func (s *Supervisor) SetPartyMode(ctx context.Context, account, mode string) error {
	return s.request(ctx, func() error {
		if mode != "" && mode != botparty.ModeOff {
			return fmt.Errorf("%w: party mode %q", ErrBadParty, mode)
		}
		b, ok := s.cfg.Registry.Bot(account)
		if !ok {
			return ErrNoBot
		}
		if b.PartyMode == mode {
			return nil
		}
		return s.cfg.Registry.Update(account, func(b *Bot) { b.PartyMode = mode })
	})
}

// SetPartyAuto switches auto parties on or off. Off, the next pass forms
// none and dissolves every auto party; pins are kept. On, they form from
// the next pass, each freed bot after its cool-down.
func (s *Supervisor) SetPartyAuto(ctx context.Context, on bool) error {
	return s.request(ctx, func() error {
		if s.cfg.Party == nil {
			return ErrNoOrganizer
		}
		if s.party.AutoOff == !on {
			return nil
		}
		s.party.AutoOff = !on
		s.log.Info("auto parties switched", "on", on)
		return nil
	})
}
