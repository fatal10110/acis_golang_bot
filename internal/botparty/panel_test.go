package botparty

import (
	"slices"
	"testing"
	"time"
)

func memberOf(c Candidate) Member { return Member{Account: c.Account, Character: c.Character} }

func TestFormByHand(t *testing.T) {
	t.Run("free bots: join first, then lead, the record an auto party", func(t *testing.T) {
		f, s := farmer("Farm", 20, 0), support("Sup", 20, 100)
		st, id := Form(State{}, memberOf(f), []Member{memberOf(s)}, false, "", t0)
		if pt := partyByID(st, id); pt == nil || pt.Pinned || pt.Group != "" || pt.Since != t0 || st.NextID != id+1 {
			t.Fatalf("formed %+v (next id %d), want an auto party", pt, st.NextID)
		}
		run(t, st, []Candidate{f, s}, DefaultPolicy(), []step{
			{want: []string{"sup:join Farm"}, deliver: true},
			{at: 30 * time.Second, want: []string{"farm:lead Sup"}, parties: map[string][]string{"Farm": {"Sup"}}},
		})
	})

	t.Run("a member of an auto party leaves it, unparty, then join once partyless", func(t *testing.T) {
		st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
		f2 := farmer("Farm2", 20, 300)
		cs = append(cs, f2)
		st, id := Form(st, memberOf(f2), []Member{memberOf(*find(cs, "sup"))}, false, "", t0)
		if id != 2 {
			t.Fatalf("id %d, want 2", id)
		}
		st = run(t, st, cs, DefaultPolicy(), []step{
			{
				want:    []string{"heal:join Farm", "farm:lead Heal", "sup:unparty"},
				parties: map[string][]string{"Farm": {"Heal"}, "Farm2": {"Sup"}}, deliver: true,
			},
			// Unpartied but still in the server's party: no join yet.
			{at: 30 * time.Second},
			{at: time.Minute, change: partyless("sup"), want: []string{"sup:join Farm2"}, deliver: true},
			{at: 90 * time.Second, want: []string{"farm2:lead Sup"}, deliver: true},
			// It keeps its new slot.
			{at: 2 * time.Minute, parties: map[string][]string{"Farm": {"Heal"}, "Farm2": {"Sup"}}},
		})
		if _, ok := st.Freed["sup"]; ok {
			t.Errorf("the taken bot cools down: Freed %v", st.Freed)
		}
	})

	t.Run("the leader of a pin breaks it, its rows rewritten", func(t *testing.T) {
		st, cs := pinned(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
		st, _ = Form(st, memberOf(cs[0]), []Member{memberOf(cs[1])}, true, "Farm", t0)
		if want := []string{"heal", "farm"}; !slices.Equal(st.Unpin, want) {
			t.Fatalf("Form's Unpin %q, want %q", st.Unpin, want)
		}
		st = run(t, st, cs, DefaultPolicy(), []step{{
			want: []string{"heal:unparty", "farm:unparty", "sup:unparty"}, why: WhyTaken, check: unpin("sup"),
		}})
		if pt := partyByID(st, 2); pt == nil || !pt.Pinned || pt.Group != "Farm" {
			t.Errorf("new party %+v, want the pin Farm", pt)
		}
		if _, ok := st.Freed["farm"]; ok {
			t.Errorf("the taken leader cools down: Freed %v", st.Freed)
		}
	})

	t.Run("pin false: a spread of 6 breaks it", func(t *testing.T) {
		f, s := farmer("Farm", 26, 0), support("Sup", 20, 100)
		st, _ := Form(State{NextID: 1}, memberOf(f), []Member{memberOf(s)}, false, "", t0)
		run(t, st, []Candidate{f, s}, DefaultPolicy(), []step{{why: WhySpread}})
	})

	t.Run("pinned: a spread of 6 holds", func(t *testing.T) {
		f, s := farmer("Farm", 26, 0), support("Sup", 20, 100)
		st, _ := Form(State{NextID: 1}, memberOf(f), []Member{memberOf(s)}, true, "Farm", t0)
		run(t, st, []Candidate{f, s}, DefaultPolicy(), []step{{want: []string{"sup:join Farm"}}})
	})
}

func TestDissolve(t *testing.T) {
	if _, ok := Dissolve(State{}, 1); ok {
		t.Fatal("dissolved a party that does not exist")
	}
	t.Run("auto", func(t *testing.T) {
		st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
		next, ok := Dissolve(st, 1)
		if !ok || st.Parties[0].Dissolve != "" {
			t.Fatalf("Dissolve = %v, or it changed its input", ok)
		}
		run(t, next, cs, DefaultPolicy(), []step{
			{want: []string{"farm:unparty", "heal:unparty", "sup:unparty"}, why: WhyDissolved, check: unpin()},
		})
	})
	t.Run("pinned: rows cleared", func(t *testing.T) {
		st, cs := pinned(farmer("Farm", 20, 0), healer("Heal", 20, 100), support("Sup", 20, 200))
		st, _ = Dissolve(st, 1)
		run(t, st, cs, DefaultPolicy(), []step{
			{want: []string{"farm:unparty", "heal:unparty", "sup:unparty"}, why: WhyDissolved, check: unpin("farm", "heal", "sup")},
		})
	})
}

func TestPlanAutoOff(t *testing.T) {
	t.Run("off dissolves auto parties, leader first, and forms none; on waits the cool-down", func(t *testing.T) {
		st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100))
		st.AutoOff = true
		unpartied := mods(partyless("farm"), partyless("heal"))
		run(t, st, cs, DefaultPolicy(), []step{
			{want: []string{"farm:unparty", "heal:unparty"}, why: WhyAutoOff, deliver: true},
			{at: 30 * time.Second, change: unpartied, parties: map[string][]string{}},
			{at: time.Minute, check: func(t *testing.T, st State) {
				if len(st.Parties) != 0 {
					t.Errorf("parties %+v while off", st.Parties)
				}
			}},
		})
		cs2 := []Candidate{farmer("Farm", 20, 0), healer("Heal", 20, 100)}
		on := State{Freed: map[string]time.Time{"farm": t0, "heal": t0}, NextID: 2}
		run(t, on, cs2, DefaultPolicy(), []step{
			{at: 5*time.Minute - time.Second, parties: map[string][]string{}},
			{at: 5 * time.Minute, want: []string{"heal:join Farm"}},
		})
	})
	t.Run("pins kept", func(t *testing.T) {
		st, cs := pinned(farmer("Farm", 20, 0), healer("Heal", 20, 100))
		st.AutoOff = true
		run(t, st, cs, DefaultPolicy(), []step{{parties: map[string][]string{"Farm": {"Heal"}}}})
	})
	t.Run("free bots form nothing", func(t *testing.T) {
		run(t, State{AutoOff: true}, []Candidate{farmer("Farm", 20, 0), healer("Heal", 20, 100)}, DefaultPolicy(),
			[]step{{parties: map[string][]string{}}})
	})
}
