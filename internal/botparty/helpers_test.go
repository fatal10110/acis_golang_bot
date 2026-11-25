package botparty

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// t0 is every test's fixed now.
var t0 = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

var (
	townA = Point{X: 0, Y: 0}
	townB = Point{X: 100000, Y: 0}
)

const (
	classFighter   int32 = 0
	classOrcMystic int32 = 49
)

// cand is an online, eligible bot of town A at x, account the lowercased name.
func cand(name, preset string, class, level, x int32) Candidate {
	town := townA
	return Candidate{
		Account:   strings.ToLower(name),
		Character: name,
		Preset:    preset,
		ClassID:   class,
		Level:     level,
		Heals:     true,
		Town:      &town,
		Position:  Point{X: x},
		HP:        100,
		Online:    true,
	}
}

func farmer(name string, level, x int32) Candidate {
	return cand(name, PresetFarmer, classFighter, level, x)
}

// healer is a human mystic support: a healer from level 7.
func healer(name string, level, x int32) Candidate {
	return cand(name, PresetSupport, classHumanMystic, level, x)
}

// support is an orc mystic support: never a healer.
func support(name string, level, x int32) Candidate {
	return cand(name, PresetSupport, classOrcMystic, level, x)
}

func find(cs []Candidate, account string) *Candidate {
	for i := range cs {
		if cs[i].Account == account {
			return &cs[i]
		}
	}
	panic("no candidate " + account)
}

// mod changes the candidate of account.
func mod(account string, f func(*Candidate)) func([]Candidate) {
	return func(cs []Candidate) { f(find(cs, account)) }
}

// partyless takes the bot of account out of its live party.
func partyless(account string) func([]Candidate) {
	return mod(account, func(c *Candidate) { c.Leader, c.Members = "", nil })
}

// mods runs each change.
func mods(fs ...func([]Candidate)) func([]Candidate) {
	return func(cs []Candidate) {
		for _, f := range fs {
			f(cs)
		}
	}
}

// strs are the orders as "account:slot".
func strs(os []Order) []string {
	var out []string
	for _, o := range os {
		out = append(out, o.Account+":"+o.Slot())
	}
	return out
}

// parties are the live parties as leader name to member names.
func parties(st State) map[string][]string {
	out := make(map[string][]string)
	for _, pt := range st.Parties {
		if pt.State != Broken {
			out[pt.Leader.Character] = pt.memberNames()
		}
	}
	return out
}

func partyByID(st State, id int) *Party {
	for i := range st.Parties {
		if st.Parties[i].ID == id {
			return &st.Parties[i]
		}
	}
	return nil
}

// answer sets each ordered bot's Sent to the order with a, and for AnswerOK its slot.
func answer(cs []Candidate, os []Order, a Answer) {
	for _, o := range os {
		c := find(cs, o.Account)
		c.NewRun = false
		c.Sent = Sent{Order: o, Answer: a}
		if a != AnswerOK {
			continue
		}
		c.PartyLeft = ""
		c.Slot = ""
		if o.Kind != OrderUnparty {
			c.Slot = o.Slot()
		}
	}
}

// established is an active party of cs, the first leading: every slot delivered, the server's
// party formed, the leader at a spot.
func established(cs ...Candidate) (State, []Candidate) {
	pt := Party{
		ID:          1,
		Leader:      Member{Account: cs[0].Account, Character: cs[0].Character},
		FormedAt:    t0.Add(-time.Hour),
		State:       Active,
		GoneSince:   map[string]time.Time{},
		BehindSince: map[string]time.Time{},
		Ordered:     map[string]bool{cs[0].Account: true},
	}
	for _, c := range cs[1:] {
		pt.Members = append(pt.Members, Member{Account: c.Account, Character: c.Character})
		pt.Ordered[c.Account] = true
	}
	var os []Order
	for i := range cs[1:] {
		os = append(os, pt.joinOrder(pt.Members[i]))
	}
	os = append(os, Order{Account: cs[0].Account, Kind: OrderLead, Names: pt.memberNames()})
	answer(cs, os, AnswerOK)
	names := []string{cs[0].Character}
	names = append(names, pt.memberNames()...)
	for i := range cs {
		cs[i].Leader = cs[0].Character
		cs[i].Members = slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == cs[i].Character })
	}
	cs[0].Spot = "Wolves"
	return State{Parties: []Party{pt}, Freed: map[string]time.Time{}, PairBans: map[[2]string]time.Time{}, NextID: 2}, cs
}

// step is one pass of a scenario: change the candidates, plan at t0+at, check.
type step struct {
	at     time.Duration
	change func([]Candidate)
	// want are the orders, "account:slot".
	want []string
	// why is party 1's break cause, "" for party 1 not broken.
	why string
	// parties, when set, are the live parties after the pass.
	parties map[string][]string
	// deliver answers the pass's orders nil before the next step.
	deliver bool
	// check, when set, checks the pass's state.
	check func(t *testing.T, st State)
}

// run plans each step in turn, one pass's state the next's.
func run(t *testing.T, st State, cs []Candidate, p Policy, steps []step) State {
	t.Helper()
	for i, s := range steps {
		if s.change != nil {
			s.change(cs)
		}
		next, os := Plan(st, cs, p, t0.Add(s.at))
		name := fmt.Sprintf("step %d (at %v)", i, s.at)
		if got := strs(os); !slices.Equal(got, s.want) {
			t.Errorf("%s: orders %q, want %q", name, got, s.want)
		}
		pt := partyByID(next, 1)
		switch {
		case s.why != "" && (pt == nil || pt.State != Broken || pt.Why != s.why):
			t.Errorf("%s: party 1 %+v, want broken %q", name, pt, s.why)
		case s.why == "" && pt != nil && pt.State == Broken:
			t.Errorf("%s: party 1 broken %q, want not", name, pt.Why)
		}
		if s.parties != nil {
			if got := parties(next); !reflect.DeepEqual(got, s.parties) {
				t.Errorf("%s: parties %v, want %v", name, got, s.parties)
			}
		}
		if s.check != nil {
			s.check(t, next)
		}
		if s.deliver {
			answer(cs, os, AnswerOK)
		}
		st = next
	}
	return st
}
