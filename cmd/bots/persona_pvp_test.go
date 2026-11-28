package main

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestPresetPvPModes checks the PvP modes of every preset (docs/pvp/modes.md,
// D35): pvp fights back, takes duels, hunts and visits arenas with
// SupportFlagged on and PK off, weighs PvP value in its gear at 0.5,
// carries five of each cure and keeps its CP potions (D37), stocking ten
// (D43); every other
// preset fights back only, with SupportFlagged off, and so declines duels,
// and compares gear by Value alone. Olympiad is off for every preset (D42);
// a bot that turns it on competes at its third profession too (#729).
func TestPresetPvPModes(t *testing.T) {
	pvp := botclient.PvPPolicy{
		FightFlagged: true, Duels: true, Hunt: true, Arenas: true, FightWar: true, Siege: true, SupportFlagged: true,
		GearWeight: 0.5, CureStock: 5, CPStock: 10, KeepCPPotions: true,
	}
	other := botclient.PvPPolicy{FightFlagged: true}
	for _, p := range presets {
		want := other
		if p.name == "pvp" {
			want = pvp
		}
		if got := p.persona().PvP; got != want {
			t.Errorf("preset %s: PvP %+v, want %+v", p.name, got, want)
		}
		if p.persona().PvP.Olympiad {
			t.Errorf("preset %s: Olympiad on, want off (D42)", p.name)
		}
		if !p.persona().Olympiad.ThirdClass {
			t.Errorf("preset %s: OlympiadPolicy.ThirdClass off, want on (#729)", p.name)
		}
	}
	if _, ok := findPreset("pvp"); !ok {
		t.Fatal("no pvp preset")
	}
}
