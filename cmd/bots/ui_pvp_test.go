package main

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// TestUIRunnerPvPModes checks a panel run's brain config plays the bot's
// PvP mode switches from the registry over its preset's defaults, keeps
// fight back and SupportFlagged the preset's, and plays the defaults
// without switches (docs/pvp/modes.md).
func TestUIRunnerPvPModes(t *testing.T) {
	u := uiRunner{shared: shared{claims: botclient.NewPartyClaims()}}
	pvp, _ := findPreset("pvp")
	farmer, _ := findPreset("farmer")
	tests := []struct {
		name  string
		p     preset
		modes *botregistry.PvPModes
		want  botclient.PvPPolicy
	}{
		{"pvp defaults", pvp, nil, pvpPersona().PvP},
		{
			"pvp with PK on and arenas off", pvp, &botregistry.PvPModes{Duels: true, Hunt: true, PK: true},
			botclient.PvPPolicy{FightFlagged: true, Duels: true, Hunt: true, ForceAttack: true, SupportFlagged: true},
		},
		{"pvp with every switch off", pvp, &botregistry.PvPModes{}, botclient.PvPPolicy{FightFlagged: true, SupportFlagged: true}},
		{"farmer defaults", farmer, nil, botclient.PvPPolicy{FightFlagged: true}},
		{"farmer taking duels", farmer, &botregistry.PvPModes{Duels: true}, botclient.PvPPolicy{FightFlagged: true, Duels: true}},
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
