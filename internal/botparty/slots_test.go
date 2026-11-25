package botparty

import (
	"reflect"
	"testing"
	"time"
)

func TestPlanFormationOrder(t *testing.T) {
	joins := []string{"heal:join Farm Sup", "sup:join Farm Heal"}
	cs := []Candidate{farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200)}
	st := run(t, State{}, cs, DefaultPolicy(), []step{
		{want: joins},
		{at: 30 * time.Second, change: func(cs []Candidate) {
			answer(cs, []Order{{Account: "heal", Kind: OrderJoin, Names: []string{"Farm", "Sup"}}}, AnswerOK)
			answer(cs, []Order{{Account: "sup", Kind: OrderJoin, Names: []string{"Farm", "Heal"}}}, AnswerPending)
		}},
		{
			at: time.Minute, change: mod("sup", func(c *Candidate) { c.Sent.Answer = AnswerError }),
			want: []string{"sup:join Farm Heal"},
		},
		{at: 90 * time.Second, change: func(cs []Candidate) {
			answer(cs, []Order{{Account: "sup", Kind: OrderJoin, Names: []string{"Farm", "Heal"}}}, AnswerOK)
		}, want: []string{"farm:lead Heal Sup"}},
	})
	if pt := partyByID(st, 0); pt == nil || pt.State != Forming {
		t.Errorf("party %+v, want forming", pt)
	}
}

func TestPlanSlotResent(t *testing.T) {
	join := Order{Account: "sup", Kind: OrderJoin, Names: []string{"Farm", "Heal"}}
	resend := []string{"sup:join Farm Heal"}
	tests := []struct {
		name  string
		steps []step
	}{
		{"new run, no slot", []step{
			{change: mod("sup", func(c *Candidate) { c.NewRun, c.Slot = true, "" }), want: resend},
		}},
		{"busy", []step{
			{change: mod("sup", func(c *Candidate) { c.Sent, c.Slot = Sent{Order: join}, "" }), want: resend},
		}},
		{"error answer", []step{
			{change: mod("sup", func(c *Candidate) { c.Sent, c.Slot = Sent{Order: join, Answer: AnswerError}, "" }), want: resend},
		}},
		{"pending, empty slot, kept", []step{
			{change: mod("sup", func(c *Candidate) { c.Sent, c.Slot = Sent{Order: join, Answer: AnswerPending}, "" })},
			{at: 10 * time.Minute},
			{at: 20 * time.Minute, check: func(t *testing.T, st State) {
				if len(partyByID(st, 1).Members) != 2 || len(st.Freed) != 0 {
					t.Errorf("party %+v, freed %v: want sup kept, none freed", partyByID(st, 1), st.Freed)
				}
			}},
		}},
		{"answered nil then empty: dropped", []step{
			{
				change:  mod("sup", func(c *Candidate) { c.Slot, c.PartyLeft = "", LeftTimeout }),
				want:    []string{"heal:join Farm", "farm:lead Heal"},
				parties: map[string][]string{"Farm": {"Heal"}},
			},
		}},
		{"leader cleared: break", []step{
			{
				change: mod("farm", func(c *Candidate) { c.Slot, c.PartyLeft = "", LeftLeft }),
				want:   []string{"heal:unparty", "sup:unparty"}, why: WhyLeaderCleared,
			},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
			run(t, st, cs, DefaultPolicy(), tc.steps)
		})
	}
}

func TestPlanSlotDiffers(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
	}{
		{"another leader", []step{
			{change: mod("sup", func(c *Candidate) { c.Slot = "join Other Heal" }), want: []string{"sup:join Farm Heal"}},
		}},
		{"a shorter lead", []step{
			{change: mod("farm", func(c *Candidate) { c.Slot = "lead Heal" }), want: []string{"farm:lead Heal Sup"}},
		}},
		{"bad command breaks, never resent", []step{
			{
				change: mod("sup", func(c *Candidate) { c.Sent.Answer, c.Slot = AnswerBadCommand, "" }),
				want:   []string{"farm:unparty", "heal:unparty"}, why: WhyBadCommand, deliver: true,
			},
			{at: 30 * time.Second, parties: map[string][]string{}},
			{at: 6 * time.Minute, change: mods(partyless("farm"), partyless("heal")), want: []string{"heal:join Farm"}, parties: map[string][]string{"Farm": {"Heal"}}},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
			run(t, st, cs, DefaultPolicy(), tc.steps)
		})
	}

	t.Run("refused unparty not resent", func(t *testing.T) {
		c := healer("Heal", 20, 100)
		c.Slot = "join Other"
		c.Sent = Sent{Order: Order{Account: "heal", Kind: OrderUnparty}, Answer: AnswerBadCommand}
		if _, os := Plan(State{}, []Candidate{c}, DefaultPolicy(), t0); len(os) != 0 {
			t.Errorf("orders %q, want none", strs(os))
		}
	})
}

func TestPlanCooldown(t *testing.T) {
	tests := []struct {
		name string
		st   State
		at   time.Duration
		want int
	}{
		{"freed 4m59s ago", State{Freed: map[string]time.Time{"heal": t0}}, 5*time.Minute - time.Second, 0},
		{"freed 5m ago", State{Freed: map[string]time.Time{"heal": t0}}, 5 * time.Minute, 1},
		{"pair banned", State{PairBans: map[[2]string]time.Time{{"farm", "heal"}: t0.Add(30 * time.Minute)}}, 29 * time.Minute, 0},
		{"pair ban over", State{PairBans: map[[2]string]time.Time{{"farm", "heal"}: t0.Add(30 * time.Minute)}}, 30 * time.Minute, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := []Candidate{farmer("Farm", 20, 0), healer("Heal", 20, 100)}
			st, _ := Plan(tc.st, cs, DefaultPolicy(), t0.Add(tc.at))
			if got := len(parties(st)); got != tc.want {
				t.Errorf("parties %v, want %d", parties(st), tc.want)
			}
		})
	}

	t.Run("fallen behind not re-paired", func(t *testing.T) {
		st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100), healer("Sup", 20, 200))
		run(t, st, cs, DefaultPolicy(), []step{
			{change: mod("sup", func(c *Candidate) { c.Position.X = 5000 })},
			{at: 10 * time.Minute, want: []string{"heal:join Farm", "farm:lead Heal", "sup:unparty"}, deliver: true},
			{
				at: 16 * time.Minute, change: mods(partyless("sup"), mod("sup", func(c *Candidate) { c.Position.X = 200 })),
				parties: map[string][]string{"Farm": {"Heal"}},
			},
			{
				at: 40 * time.Minute, want: []string{"heal:join Farm Sup", "sup:join Farm Heal"},
				parties: map[string][]string{"Farm": {"Heal", "Sup"}},
			},
		})
	})
}

func TestPlanState(t *testing.T) {
	t.Run("forming", func(t *testing.T) {
		st, _ := Plan(State{}, []Candidate{farmer("Farm", 20, 0), healer("Heal", 20, 100)}, DefaultPolicy(), t0)
		want := Party{
			Leader:      Member{Account: "farm", Character: "Farm"},
			Members:     []Member{{Account: "heal", Character: "Heal"}},
			FormedAt:    t0,
			State:       Forming,
			GoneSince:   map[string]time.Time{},
			BehindSince: map[string]time.Time{},
			Ordered:     map[string]bool{"heal": true},
		}
		if len(st.Parties) != 1 || !reflect.DeepEqual(st.Parties[0], want) || st.NextID != 1 {
			t.Errorf("state %+v, want one party %+v", st, want)
		}
	})

	t.Run("timers set and cleared", func(t *testing.T) {
		st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
		run(t, st, cs, DefaultPolicy(), []step{
			{change: mod("farm", func(c *Candidate) { c.Spot = "" }), check: func(t *testing.T, st State) {
				if got := partyByID(st, 1).NoSpotSince; !got.Equal(t0) {
					t.Errorf("NoSpotSince %v, want t0", got)
				}
			}},
			{at: time.Minute, change: mod("farm", func(c *Candidate) { c.Spot = "Wolves" }), check: func(t *testing.T, st State) {
				if got := partyByID(st, 1).NoSpotSince; !got.IsZero() {
					t.Errorf("NoSpotSince %v, want zero", got)
				}
			}},
		})
	})

	t.Run("leader snapshot listing a member: active", func(t *testing.T) {
		st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100))
		st.Parties[0].State = Forming
		find(cs, "farm").Members = []string{"Player"}
		run(t, st, cs, DefaultPolicy(), []step{
			{check: func(t *testing.T, st State) {
				if got := partyByID(st, 1).State; got != Forming {
					t.Errorf("state %v, want forming", got)
				}
			}},
			{
				at: 30 * time.Second, change: mod("farm", func(c *Candidate) { c.Members = []string{"Player", "Heal"} }),
				check: func(t *testing.T, st State) {
					if got := partyByID(st, 1).State; got != Active {
						t.Errorf("state %v, want active", got)
					}
				},
			},
		})
	})

	t.Run("input unchanged", func(t *testing.T) {
		build := func() (State, []Candidate) {
			st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 5000))
			st.Parties[0].BehindSince["sup"] = t0.Add(-10 * time.Minute)
			st.Freed["old"] = t0.Add(-time.Hour)
			st.PairBans[pairKey("a", "b")] = t0.Add(-time.Minute)
			st.Unpin = []string{"x"}
			return st, append(cs, farmer("Ftwo", 20, 300), healer("Htwo", 20, 400))
		}
		st, cs := build()
		want, wantCs := build()
		next, _ := Plan(st, cs, DefaultPolicy(), t0)
		if !reflect.DeepEqual(st, want) || !reflect.DeepEqual(cs, wantCs) {
			t.Errorf("Plan changed its input")
		}
		if got := next.Freed["sup"]; !got.Equal(t0) {
			t.Errorf("Freed[sup] %v, want t0", got)
		}
		if _, ok := next.PairBans[pairKey("farm", "sup")]; !ok {
			t.Errorf("no PairBans farm-sup: %v", next.PairBans)
		}
		if len(next.Freed) != 1 || len(next.PairBans) != 1 || next.Unpin != nil {
			t.Errorf("expired entries kept: freed %v, bans %v, unpin %v", next.Freed, next.PairBans, next.Unpin)
		}
	})
}
