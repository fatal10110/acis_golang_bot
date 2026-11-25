package botparty

import (
	"testing"
	"time"
)

func TestPlanSkipsIneligible(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Candidate)
		want   int
	}{
		{"eligible", func(*Candidate) {}, 1},
		{"merchant", func(c *Candidate) { c.Preset = "merchant" }, 0},
		{"crafter", func(c *Candidate) { c.Preset = "crafter" }, 0},
		{"town", func(c *Candidate) { c.Preset = "town" }, 0},
		{"mode off", func(c *Candidate) { c.Mode = ModeOff }, 0},
		{"standing order", func(c *Candidate) { c.Order = "stay" }, 0},
		{"whispered", func(c *Candidate) { c.Whispered = true }, 0},
		{"reconnecting", func(c *Candidate) { c.Online = false }, 0},
		{"player's party", func(c *Candidate) { c.Leader, c.Members = "Player", []string{"Heal"} }, 0},
		{"no town", func(c *Candidate) { c.Town = nil }, 0},
		{"name unknown", func(c *Candidate) { c.Character = "" }, 0},
		{"slot held", func(c *Candidate) { c.Slot = "join Other" }, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := []Candidate{farmer("Farm", 20, 0), healer("Heal", 20, 100)}
			tc.change(&cs[1])
			got, os := formed(cs, DefaultPolicy())
			if len(got) != tc.want {
				t.Errorf("parties %v, want %d", got, tc.want)
			}
			if tc.want == 0 && tc.name != "slot held" && len(os) != 0 {
				t.Errorf("orders %q, want none", strs(os))
			}
		})
	}
}

func TestPlanBreaks(t *testing.T) {
	all := []string{"farm:unparty", "heal:unparty", "sup:unparty"}
	dropSup := []string{"heal:join Farm", "farm:lead Heal", "sup:unparty"}
	leaderOff := mod("farm", func(c *Candidate) { c.Online = false })
	member := func(a string, want bool) func(*testing.T, State) {
		return func(t *testing.T, st State) {
			t.Helper()
			in := false
			for _, m := range partyByID(st, 1).Members {
				in = in || m.Account == a
			}
			if in != want {
				t.Errorf("%s a member %v, want %v", a, in, want)
			}
		}
	}
	leaderGone := func(want time.Duration) func(*testing.T, State) {
		return func(t *testing.T, st State) {
			t.Helper()
			if got := partyByID(st, 1).LeaderGoneSince; want < 0 && !got.IsZero() || want >= 0 && !got.Equal(t0.Add(want)) {
				t.Errorf("LeaderGoneSince %v, want t0+%v", got, want)
			}
		}
	}
	tests := []struct {
		name  string
		two   bool
		extra []Candidate
		steps []step
	}{
		{name: "leader gone, stale snapshot naming its party", steps: []step{
			{change: leaderOff, check: leaderGone(0)},
			{at: 4 * time.Minute},
			{at: 5 * time.Minute, want: all, why: WhyLeaderGone},
		}},
		{name: "no healer", steps: []step{
			{change: mod("heal", func(c *Candidate) { c.HP = 0 })},
			{at: 9 * time.Minute},
			{at: 10 * time.Minute, want: all, why: WhyNoHealer},
		}},
		{name: "no spot bans the pairs", steps: []step{
			{change: mod("farm", func(c *Candidate) { c.Spot = "" })},
			{at: 14 * time.Minute},
			{at: 15 * time.Minute, want: all, why: WhyNoSpot, check: func(t *testing.T, st State) {
				until := t0.Add(45 * time.Minute)
				for _, k := range [][2]string{{"farm", "heal"}, {"farm", "sup"}} {
					if got := st.PairBans[k]; !got.Equal(until) {
						t.Errorf("PairBans[%v] %v, want %v", k, got, until)
					}
				}
				if _, ok := st.PairBans[[2]string{"heal", "sup"}]; ok {
					t.Errorf("members banned with each other")
				}
			}},
		}},
		{name: "spot back clears no spot", steps: []step{
			{change: mod("farm", func(c *Candidate) { c.Spot = "" })},
			{
				at: time.Minute, change: mod("farm", func(c *Candidate) { c.Spot = "beside Wolves" }),
				check: func(t *testing.T, st State) {
					if got := partyByID(st, 1).NoSpotSince; !got.IsZero() {
						t.Errorf("NoSpotSince %v, want zero", got)
					}
				},
			},
			{at: 16 * time.Minute},
		}},
		{name: "spread 5 holds, a player above uncounted", steps: []step{
			{change: mods(mod("sup", func(c *Candidate) { c.Level = 15 }),
				mod("farm", func(c *Candidate) { c.Members = append(c.Members, "Player") }))},
		}},
		{name: "spread 6 breaks", steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Level = 14 }), want: all, why: WhySpread},
		}},
		{name: "two, member cleared: unparty to the leader alone", two: true, steps: []step{
			{
				change: mod("heal", func(c *Candidate) { c.Slot, c.PartyLeft = "", LeftLeft }),
				want:   []string{"farm:unparty"}, why: WhyNoMember,
			},
		}},
		{name: "gone member keeps its place, clears on return", steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Online = false })},
			{at: 4 * time.Minute, check: member("sup", true)},
			{
				at: 4*time.Minute + 30*time.Second, change: mod("sup", func(c *Candidate) { c.Online = true }),
				check: func(t *testing.T, st State) {
					if got := partyByID(st, 1).GoneSince; len(got) != 0 {
						t.Errorf("GoneSince %v, want none", got)
					}
				},
			},
			{at: 9 * time.Minute, check: member("sup", true)},
		}},
		{name: "gone member dropped at LeaderGone, filled next pass", extra: []Candidate{support("Stwo", 20, 300)}, steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Online = false })},
			{at: 5 * time.Minute, want: dropSup, deliver: true, check: func(t *testing.T, st State) {
				if got := st.Freed["sup"]; !got.Equal(t0.Add(5 * time.Minute)) {
					t.Errorf("Freed[sup] %v, want t0+5m", got)
				}
			}, parties: map[string][]string{"Farm": {"Heal"}}},
			{
				at: 5*time.Minute + 30*time.Second, want: []string{"heal:join Farm Stwo", "stwo:join Farm Heal"},
				parties: map[string][]string{"Farm": {"Heal", "Stwo"}},
			},
		}},
		{name: "member set off dropped", steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Mode = ModeOff }), want: dropSup},
		}},
		{name: "member preset changed dropped", steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Preset = "town" }), want: dropSup},
		}},
		{name: "member standing order dropped", steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Order = "follow Player" }), want: dropSup},
		}},
		{name: "member whispered dropped", steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Whispered = true }), want: dropSup},
		}},
		{name: "behind dropped, pair banned", steps: []step{
			{change: mod("sup", func(c *Candidate) { c.Position.X = 5000 })},
			{at: 9 * time.Minute},
			{at: 10 * time.Minute, want: dropSup, check: func(t *testing.T, st State) {
				if got := st.PairBans[[2]string{"farm", "sup"}]; !got.Equal(t0.Add(40 * time.Minute)) {
					t.Errorf("PairBans farm-sup %v, want t0+40m", got)
				}
			}},
		}},
		{name: "stale position starts no BehindSince", steps: []step{
			{
				change: mod("sup", func(c *Candidate) { c.Online, c.Position.X = false, 5000 }),
				check: func(t *testing.T, st State) {
					if got := partyByID(st, 1).BehindSince; len(got) != 0 {
						t.Errorf("BehindSince %v, want none", got)
					}
				},
			},
		}},
		{name: "two, member gone: timer on its return", two: true, steps: []step{
			{change: mods(mod("heal", func(c *Candidate) { c.Online = false }), partyless("farm")), check: leaderGone(-1)},
			{
				at: 4 * time.Minute, change: mods(mod("heal", func(c *Candidate) { c.Online = true }), partyless("heal")),
				check: leaderGone(4 * time.Minute),
			},
			{at: 8*time.Minute + 59*time.Second},
			{at: 9 * time.Minute, want: []string{"farm:unparty", "heal:unparty"}, why: WhyLeaderGone},
		}},
		{name: "dropped with join pending: unparty once answered", steps: []step{
			{
				change: mod("sup", func(c *Candidate) { c.Mode, c.Sent.Answer = ModeOff, AnswerPending }),
				want:   []string{"heal:join Farm", "farm:lead Heal"}, deliver: true,
			},
			{
				at: 30 * time.Second, change: mod("sup", func(c *Candidate) { c.Sent.Answer = AnswerOK }),
				want: []string{"sup:unparty"},
			},
		}},
		{name: "lead undelivered starts no LeaderGoneSince", steps: []step{
			{
				change: mods(partyless("farm"), partyless("heal"), partyless("sup"),
					mod("farm", func(c *Candidate) { c.Slot, c.Sent = "", Sent{} })),
				want: []string{"farm:lead Heal Sup"}, check: leaderGone(-1),
			},
			{at: 30 * time.Second, change: mod("farm", func(c *Candidate) {
				c.Sent = Sent{Order: Order{Account: "farm", Kind: OrderLead, Names: []string{"Heal", "Sup"}}, Answer: AnswerPending}
			}), check: leaderGone(-1)},
			{
				at: time.Minute, change: mod("farm", func(c *Candidate) { c.Sent.Answer, c.Slot = AnswerOK, "lead Heal Sup" }),
				check: leaderGone(time.Minute),
			},
			{at: 5*time.Minute + 59*time.Second},
			{at: 6 * time.Minute, want: all, why: WhyLeaderGone},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := []Candidate{farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200)}
			if tc.two {
				cs = cs[:2]
			}
			st, cs := established(cs...)
			cs = append(cs, tc.extra...)
			run(t, st, cs, DefaultPolicy(), tc.steps)
		})
	}
}
