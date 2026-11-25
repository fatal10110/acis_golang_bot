package botregistry

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
)

// partyRig starts a supervisor with the organizer over bots (accounts hz1,
// hz2, ... in order), every one restored and handed over as a fake run.
func partyRig(t *testing.T, bots ...Bot) (*harness, []*fakeRun, map[string]*fakeBot) {
	t.Helper()
	reg := openPanel(t, t.TempDir())
	for _, b := range bots {
		b.Online = true
		if _, err := reg.NewBot(b); err != nil {
			t.Fatal(err)
		}
	}
	h := startSupervisor(t, reg, newClock(), len(bots), withParty)
	var runs []*fakeRun
	for range bots {
		runs = append(runs, h.next(t))
	}
	fakes := make(map[string]*fakeBot, len(bots))
	for i, b := range bots {
		a := fmt.Sprintf("hz%d", i+1)
		fakes[a] = newFakeBot(b.Name)
		h.handOver(t, a, fakes[a])
	}
	return h, runs, fakes
}

// inParty sets the server's party on the snapshots: leader leads members.
func inParty(leader *fakeBot, members ...*fakeBot) {
	all := []*fakeBot{leader}
	all = append(all, members...)
	var names []string
	for _, f := range all {
		names = append(names, f.Status().Character)
	}
	for _, f := range all {
		own := f.Status().Character
		f.set(func(s *botclient.Status) {
			s.Leader = names[0]
			s.Members = slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == own })
		})
	}
}

func outOfParty(fs ...*fakeBot) {
	for _, f := range fs {
		f.set(func(s *botclient.Status) { s.Leader, s.Members = "", nil })
	}
}

func farmerBot(name string) Bot { return Bot{Name: name, Class: ptr(0), Preset: botparty.PresetFarmer} }

func supportBot(name string) Bot {
	return Bot{Name: name, Class: ptr(10), Preset: botparty.PresetSupport}
}

func TestSupervisorPartyCalls(t *testing.T) {
	t.Run("form by hand, take a member, refusals, spread", func(t *testing.T) {
		off := supportBot("Off")
		off.PartyMode = botparty.ModeOff
		h, runs, f := partyRig(t, farmerBot("Farm"), supportBot("Heal"), farmerBot("Farm2"),
			Bot{Name: "Craft", Class: ptr(0), Preset: "crafter"}, off, supportBot("Partied"), supportBot("Away"))
		ctx := t.Context()
		f["hz6"].set(func(s *botclient.Status) { s.Leader, s.Members = "Someone", []string{"Partied"} })
		f["hz7"].set(func(s *botclient.Status) { s.State = botclient.StateReconnecting })

		id, err := h.sup.FormParty(ctx, "hz1", []string{"hz2"}, false)
		if err != nil || id != 0 {
			t.Fatalf("FormParty = %d, %v", id, err)
		}
		st, _, _ := h.party(t, "hz1")
		if len(st.Parties) != 1 || st.Parties[0].Pinned {
			t.Fatalf("state %+v, want the hand party recorded", st.Parties)
		}
		wantHanded(t, "before the pass", f["hz2"])
		h.partyPass(t)
		wantHanded(t, "first pass", f["hz2"], "join Farm")
		f["hz2"].answer(nil, "join Farm")
		h.partyPass(t)
		wantHanded(t, "second pass", f["hz1"], "lead Heal")
		f["hz1"].answer(nil, "lead Heal")
		inParty(f["hz1"], f["hz2"])
		f["hz1"].set(func(s *botclient.Status) { s.Members, s.Spot = append(s.Members, "Pal"), "Wolves" })
		h.partyPass(t)
		ps, err := h.sup.Parties(ctx)
		if err != nil || !ps.Auto || len(ps.Parties) != 1 {
			t.Fatalf("Parties = %+v, %v", ps, err)
		}
		p := ps.Parties[0]
		if p.State != "active" || p.Leader.Character != "Farm" || len(p.Members) != 1 || p.Members[0].Character != "Heal" ||
			p.Members[0].Level != 20 || p.Members[0].Slot != "join Farm" || !slices.Equal(p.Players, []string{"Pal"}) || p.Spot != "Wolves" {
			t.Errorf("party %+v, want Farm leading Heal and the player Pal at Wolves", p)
		}
		if free := ps.Free; len(free) != 1 || free[0].Account != "hz3" {
			t.Errorf("free pool %+v, want Farm2 alone", free)
		}

		for _, tc := range []struct {
			name    string
			members []string
			want    error
		}{
			{"crafter", []string{"hz4"}, ErrPartyRefused},
			{"off", []string{"hz5"}, ErrPartyRefused},
			{"player's party", []string{"hz6"}, ErrPartyRefused},
			{"offline", []string{"hz7"}, ErrPartyRefused},
			{"no member", nil, ErrBadParty},
			{"named twice", []string{"hz2", "hz2"}, ErrBadParty},
			{"too many", []string{"hz2", "hz4", "hz5", "hz6"}, ErrBadParty},
			{"no bot", []string{"hz99"}, ErrNoBot},
		} {
			if _, err := h.sup.FormParty(ctx, "hz3", tc.members, false); !errors.Is(err, tc.want) {
				t.Errorf("%s: FormParty = %v, want %v", tc.name, err, tc.want)
			}
		}
		if _, err := h.sup.FormParty(ctx, "hz2", []string{"hz3"}, false); !errors.Is(err, ErrPartyRefused) {
			t.Errorf("a support leading an auto party: %v", err)
		}
		// An auto party the next pass would break at once for its spread.
		f["hz3"].set(func(s *botclient.Status) { s.Level = 26 })
		if _, err := h.sup.FormParty(ctx, "hz3", []string{"hz2"}, false); !errors.Is(err, ErrPartyRefused) || !strings.Contains(err.Error(), "spread") {
			t.Errorf("an auto party at a spread of 6: %v", err)
		}
		f["hz3"].set(func(s *botclient.Status) { s.Level = 20 })

		// Farm2 takes Heal: Farm's party breaks (Heal was its last bot
		// member), Heal gets unparty, then join once out of the party.
		if _, err := h.sup.FormParty(ctx, "hz3", []string{"hz2"}, false); err != nil {
			t.Fatal(err)
		}
		h.partyPass(t)
		wantHanded(t, "taken", f["hz1"], "lead Heal", "unparty")
		wantHanded(t, "taken", f["hz2"], "join Farm", "unparty")
		f["hz1"].answer(nil, "")
		f["hz2"].answer(nil, "")
		h.partyPass(t)
		wantHanded(t, "still in the party", f["hz2"], "join Farm", "unparty")
		outOfParty(f["hz1"], f["hz2"])
		h.partyPass(t)
		wantHanded(t, "partyless", f["hz2"], "join Farm", "unparty", "join Farm2")
		f["hz2"].answer(nil, "join Farm2")
		h.partyPass(t)
		wantHanded(t, "lead", f["hz3"], "lead Heal")
		f["hz3"].answer(nil, "lead Heal")
		inParty(f["hz3"], f["hz2"])
		for range 2 {
			h.partyPass(t)
		}
		wantHanded(t, "kept", f["hz2"], "join Farm", "unparty", "join Farm2")
		wantHanded(t, "kept", f["hz1"], "lead Heal", "unparty")
		if s := f["hz2"].Status(); s.Party != "join Farm2" {
			t.Errorf("Heal's slot %q, want join Farm2", s.Party)
		}

		// An auto party breaks at a spread of 6, the leader first.
		f["hz3"].set(func(s *botclient.Status) { s.Level = 26 })
		h.partyPass(t)
		wantHanded(t, "spread", f["hz3"], "lead Heal", "unparty")
		wantHanded(t, "spread", f["hz2"], "join Farm", "unparty", "join Farm2", "unparty")
		m := h.metrics.Collect(t0)
		wantBroken := map[string]uint64{botparty.WhyTaken: 1, botparty.WhySpread: 1}
		if m.PartiesFormed != 2 || !maps.Equal(m.PartiesBroken, wantBroken) {
			t.Errorf("metrics formed %d broken %v, want 2 and %v", m.PartiesFormed, m.PartiesBroken, wantBroken)
		}
		h.stop(t, runs...)
	})

	t.Run("pin, mode off, dissolve", func(t *testing.T) {
		h, runs, f := partyRig(t, farmerBot("Farm"), supportBot("Heal"), supportBot("Sup"))
		ctx := t.Context()
		id, err := h.sup.FormParty(ctx, "hz1", []string{"hz2", "hz3"}, true)
		if err != nil {
			t.Fatal(err)
		}
		rows := func(want map[string]string) {
			t.Helper()
			l := h.list(t)
			for a, g := range want {
				if b := l[a]; b.PartyGroup != g || b.PartyLead != (a == "hz1" && g != "") {
					t.Errorf("%s pin rows %q %v, want %q", a, b.PartyGroup, b.PartyLead, g)
				}
			}
		}
		rows(map[string]string{"hz1": "Farm", "hz2": "Farm", "hz3": "Farm"})
		h.partyPass(t)
		wantHanded(t, "joins", f["hz2"], "join Farm Sup")
		wantHanded(t, "joins", f["hz3"], "join Farm Heal")
		f["hz2"].answer(nil, "join Farm Sup")
		f["hz3"].answer(nil, "join Farm Heal")
		h.partyPass(t)
		wantHanded(t, "lead", f["hz1"], "lead Heal Sup")
		f["hz1"].answer(nil, "lead Heal Sup")
		inParty(f["hz1"], f["hz2"], f["hz3"])

		if err := h.sup.SetPartyMode(ctx, "hz3", "bogus"); !errors.Is(err, ErrBadParty) {
			t.Errorf("SetPartyMode bogus = %v", err)
		}
		if err := h.sup.SetPartyMode(ctx, "hz3", botparty.ModeOff); err != nil {
			t.Fatal(err)
		}
		h.partyPass(t)
		wantHanded(t, "off", f["hz3"], "join Farm Heal", "unparty")
		wantHanded(t, "off", f["hz2"], "join Farm Sup", "join Farm")
		wantHanded(t, "off", f["hz1"], "lead Heal Sup", "lead Heal")
		rows(map[string]string{"hz1": "Farm", "hz2": "Farm", "hz3": ""})
		f["hz1"].answer(nil, "lead Heal")
		f["hz2"].answer(nil, "join Farm")
		f["hz3"].answer(nil, "")

		if err := h.sup.DissolveParty(ctx, 99); !errors.Is(err, ErrNoParty) {
			t.Errorf("DissolveParty(99) = %v", err)
		}
		if err := h.sup.DissolveParty(ctx, id); err != nil {
			t.Fatal(err)
		}
		h.partyPass(t)
		wantHanded(t, "dissolved", f["hz1"], "lead Heal Sup", "lead Heal", "unparty")
		wantHanded(t, "dissolved", f["hz2"], "join Farm Sup", "join Farm", "unparty")
		rows(map[string]string{"hz1": "", "hz2": ""})
		h.stop(t, runs...)
	})

	t.Run("auto switch", func(t *testing.T) {
		h, runs, f := partyRig(t, farmerBot("Farm"), supportBot("Heal"), farmerBot("Farm2"), supportBot("Sup"))
		ctx := t.Context()
		town := botclient.Point{}
		for _, a := range []string{"hz1", "hz2"} {
			f[a].set(func(s *botclient.Status) { s.Town, s.Heals = &town, true })
		}
		f["hz2"].set(func(s *botclient.Status) { s.ClassID = 10 }) // a human mystic heals
		if _, err := h.sup.FormParty(ctx, "hz3", []string{"hz4"}, true); err != nil {
			t.Fatal(err)
		}
		h.partyPass(t)
		wantHanded(t, "auto forms", f["hz2"], "join Farm")
		wantHanded(t, "pin forms", f["hz4"], "join Farm2")
		f["hz2"].answer(nil, "join Farm")
		f["hz4"].answer(nil, "join Farm2")
		h.partyPass(t)
		f["hz1"].answer(nil, "lead Heal")
		f["hz3"].answer(nil, "lead Sup")
		inParty(f["hz1"], f["hz2"])
		inParty(f["hz3"], f["hz4"])
		h.partyPass(t)

		if err := h.sup.SetPartyAuto(ctx, false); err != nil {
			t.Fatal(err)
		}
		if _, err := h.sup.FormParty(ctx, "hz1", []string{"hz2"}, false); !errors.Is(err, ErrPartyRefused) {
			t.Errorf("an auto party formed while off: %v", err)
		}
		h.partyPass(t)
		wantHanded(t, "off", f["hz1"], "lead Heal", "unparty")
		wantHanded(t, "off", f["hz2"], "join Farm", "unparty")
		wantHanded(t, "pin kept", f["hz3"], "lead Sup")
		f["hz1"].answer(nil, "")
		f["hz2"].answer(nil, "")
		outOfParty(f["hz1"], f["hz2"])
		h.partyPass(t)
		if ps, err := h.sup.Parties(ctx); err != nil || ps.Auto || len(ps.Parties) != 1 || !ps.Parties[0].Pinned {
			t.Fatalf("Parties while off = %+v, %v; want the pin alone", ps, err)
		}
		if err := h.sup.SetPartyAuto(ctx, true); err != nil {
			t.Fatal(err)
		}
		// Freed at the off pass: grouped again only after Cooldown.
		for range 8 {
			h.partyPass(t)
		}
		wantHanded(t, "cooling", f["hz2"], "join Farm", "unparty")
		h.partyPass(t)
		wantHanded(t, "cooled down", f["hz2"], "join Farm", "unparty", "join Farm")
		h.stop(t, runs...)
	})
}
