package botmetrics

import (
	"encoding/json"
	"maps"
	"testing"
	"time"
)

// TestRegistryPvP checks the PvP counters (docs/pvp/modes.md "Panel and
// metrics (X12)"): safe on a nil Bot, summed over the bots, every label
// present from the first snapshot on, an unknown label counting nothing,
// and their JSON names.
func TestRegistryPvP(t *testing.T) {
	var none *Bot
	none.PvPFight("duel")
	none.PvPDeath("pk")
	none.Duel("won")
	none.PvPInterrupt(InterruptCast)

	r := NewRegistry()
	zero := func(names []string) map[string]uint64 {
		m := make(map[string]uint64, len(names))
		for _, n := range names {
			m[n] = 0
		}
		return m
	}
	last := r.Last()
	for name, got := range map[string]map[string]uint64{"fights": last.PvPFights, "deaths": last.PvPDeaths, "duels": last.Duels, "interrupts": last.PvPInterrupts} {
		want := map[string]map[string]uint64{
			"fights": zero(pvpFightKinds[:]), "deaths": zero(pvpDeathKinds[:]),
			"duels": zero(duelResults[:]), "interrupts": zero(interruptResults[:]),
		}[name]
		if !maps.Equal(got, want) {
			t.Errorf("empty %s %v, want %v", name, got, want)
		}
	}

	a, b := r.NewBot(), r.NewBot()
	a.PvPFight("duel")
	b.PvPFight("duel")
	a.PvPFight("pk")
	b.PvPFight("war")
	b.PvPFight("olympiad")
	a.PvPFight("unfought") // no fight kind
	a.PvPDeath("unfought")
	b.PvPDeath("arena")
	a.PvPDeath("war")
	b.PvPDeath("olympiad")
	b.PvPDeath("duel") // a duel defeat is no death
	a.Duel("won")
	b.Duel("tie")
	b.Duel("")
	a.PvPInterrupt(InterruptCast)
	a.PvPInterrupt(InterruptLanded)
	b.PvPInterrupt(InterruptCast)
	b.PvPInterrupt(InterruptTooLate)
	s := r.Collect(time.Unix(0, 0))

	for _, tc := range []struct {
		name      string
		got, want map[string]uint64
	}{
		{"fights", s.PvPFights, map[string]uint64{"duel": 2, "fight_back": 0, "arena": 0, "flagged": 0, "pk": 1, "war": 1, "olympiad": 1}},
		{"deaths", s.PvPDeaths, map[string]uint64{"fight_back": 0, "arena": 1, "flagged": 0, "pk": 0, "war": 1, "olympiad": 1, "unfought": 1}},
		{"duels", s.Duels, map[string]uint64{"won": 1, "lost": 0, "tie": 1}},
		{"interrupts", s.PvPInterrupts, map[string]uint64{"cast": 2, "landed": 1, "too_late": 1}},
	} {
		if !maps.Equal(tc.got, tc.want) {
			t.Errorf("%s %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pvp_fights_total", "pvp_deaths_total", "duels_total", "pvp_interrupts_total"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("snapshot JSON lacks %s", name)
		}
	}
}
