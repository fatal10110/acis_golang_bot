package main

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// TestUIRunnerPvPModes checks a panel run's brain config plays the bot's
// PvP mode switches from the registry over its preset's defaults, keeps
// the rest of the preset's policy (fight back, SupportFlagged, gear and
// cures), and plays the defaults
// without switches (docs/pvp/modes.md).
func TestUIRunnerPvPModes(t *testing.T) {
	u := uiRunner{shared: shared{claims: botclient.NewPartyClaims()}}
	pvp, _ := findPreset("pvp")
	farmer, _ := findPreset("farmer")
	// switched is the pvp persona's policy with the five switches set: the
	// rest (fight back, SupportFlagged, gear and cure settings) stays the
	// preset's.
	switched := func(duels, hunt, arenas, pk, war bool) botclient.PvPPolicy {
		p := pvpPersona().PvP
		p.Duels, p.Hunt, p.Arenas, p.ForceAttack, p.FightWar = duels, hunt, arenas, pk, war
		return p
	}
	tests := []struct {
		name  string
		p     preset
		modes *botregistry.PvPModes
		want  botclient.PvPPolicy
	}{
		{"pvp defaults", pvp, nil, pvpPersona().PvP},
		{"pvp with PK on and arenas off", pvp, &botregistry.PvPModes{Duels: true, Hunt: true, PK: true, War: true}, switched(true, true, false, true, true)},
		{"pvp with War off", pvp, &botregistry.PvPModes{Duels: true, Hunt: true, Arenas: true}, switched(true, true, true, false, false)},
		{"pvp with every switch off", pvp, &botregistry.PvPModes{}, switched(false, false, false, false, false)},
		{"farmer defaults", farmer, nil, botclient.PvPPolicy{FightFlagged: true}},
		{"farmer taking duels", farmer, &botregistry.PvPModes{Duels: true}, botclient.PvPPolicy{FightFlagged: true, Duels: true}},
		{"farmer at war", farmer, &botregistry.PvPModes{War: true}, botclient.PvPPolicy{FightFlagged: true, FightWar: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := u.brain(tt.p, botregistry.Run{Account: "hz1", Preset: tt.p.name, PvP: tt.modes})
			if cfg.Persona.PvP != tt.want {
				t.Fatalf("PvP %+v, want %+v", cfg.Persona.PvP, tt.want)
			}
		})
	}
}
