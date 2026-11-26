package main

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestPresetPvPModes checks the PvP modes of every preset (docs/pvp/modes.md,
// D35): pvp fights back, takes duels, hunts and visits arenas with
// SupportFlagged on and PK off; every other preset fights back only, with
// SupportFlagged off, and so declines duels.
func TestPresetPvPModes(t *testing.T) {
	pvp := botclient.PvPPolicy{FightFlagged: true, Duels: true, Hunt: true, Arenas: true, SupportFlagged: true}
	other := botclient.PvPPolicy{FightFlagged: true}
	for _, p := range presets {
		want := other
		if p.name == "pvp" {
			want = pvp
		}
		if got := p.persona().PvP; got != want {
			t.Errorf("preset %s: PvP %+v, want %+v", p.name, got, want)
		}
	}
	if _, ok := findPreset("pvp"); !ok {
		t.Fatal("no pvp preset")
	}
}
