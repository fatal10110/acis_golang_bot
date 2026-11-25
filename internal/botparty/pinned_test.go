package botparty

import (
	"slices"
	"testing"
	"time"
)

// pinned is established as the panel's pin "Pin", the first leading.
func pinned(cs ...Candidate) (State, []Candidate) {
	st, cs := established(cs...)
	st.Parties[0].Pinned, st.Parties[0].Group = true, "Pin"
	for i := range cs {
		cs[i].Group = "Pin"
	}
	cs[0].Lead = true
	return st, cs
}

func unpin(want ...string) func(*testing.T, State) {
	return func(t *testing.T, st State) {
		t.Helper()
		if !slices.Equal(st.Unpin, want) {
			t.Errorf("Unpin %q, want %q", st.Unpin, want)
		}
	}
}

func TestPlanPinned(t *testing.T) {
	t.Run("forms from its rows out of band, join after the leader is online", func(t *testing.T) {
		f, s := farmer("Farm", 20, 0), support("Sup", 10, 9000)
		f.Group, f.Lead, s.Group = "Pin", true, "Pin"
		f.Online = false
		st := run(t, State{}, []Candidate{f, s}, DefaultPolicy(), []step{
			{parties: map[string][]string{"Farm": {"Sup"}}},
			{
				at: 30 * time.Second, change: mod("farm", func(c *Candidate) { c.Online = true }),
				want: []string{"sup:join Farm"}, deliver: true,
			},
			{at: time.Minute, want: []string{"farm:lead Sup"}},
		})
		if pt := partyByID(st, 0); pt == nil || !pt.Pinned || pt.Group != "Pin" {
			t.Errorf("party %+v, want the pin", pt)
		}
	})

	tests := []struct {
		name  string
		steps []step
	}{
		{"no break applies", []step{
			{change: mods(
				mod("farm", func(c *Candidate) { c.Online, c.Spot = false, "" }),
				mod("heal", func(c *Candidate) { c.HP = 0 }),
				mod("sup", func(c *Candidate) { c.Online, c.Level = false, 5 }))},
			{at: 20 * time.Minute, parties: map[string][]string{"Farm": {"Heal", "Sup"}}},
		}},
		{"member left", []step{
			{
				change: mod("heal", func(c *Candidate) { c.Slot, c.PartyLeft = "", LeftLeft }),
				want:   []string{"sup:join Farm", "farm:lead Sup"}, check: unpin("heal"), deliver: true,
			},
			{at: 30 * time.Second, parties: map[string][]string{"Farm": {"Sup"}}, check: unpin()},
		}},
		{"leader left dissolves", []step{
			{
				change: mod("farm", func(c *Candidate) { c.Slot, c.PartyLeft = "", LeftLeft }),
				want:   []string{"heal:unparty", "sup:unparty"}, why: WhyLeaderLeft, check: unpin("farm", "heal", "sup"),
			},
		}},
		{"timeout rejoins once the leader is online", []step{
			{change: mods(mod("heal", func(c *Candidate) { c.Slot, c.PartyLeft = "", LeftTimeout }),
				mod("farm", func(c *Candidate) { c.Online = false }))},
			{
				at: 30 * time.Second, change: mod("farm", func(c *Candidate) { c.Online = true }),
				want: []string{"heal:join Farm Sup"},
			},
		}},
		{"member set off leaves", []step{
			{
				change: mod("sup", func(c *Candidate) { c.Mode = ModeOff }),
				want:   []string{"heal:join Farm", "farm:lead Heal", "sup:unparty"}, check: unpin("sup"),
			},
		}},
		{"whispered member waits", []step{
			{change: mod("heal", func(c *Candidate) {
				c.Slot, c.Whispered, c.Sent.Answer = "", true, AnswerError
			})},
			{
				at: 30 * time.Second, change: mod("heal", func(c *Candidate) { c.Whispered = false }),
				want: []string{"heal:join Farm Sup"},
			},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, cs := pinned(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
			run(t, st, cs, DefaultPolicy(), tc.steps)
		})
	}
}
