package main

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestParseCrafterFlags: the crafter preset needs -datapack, and
// -clan-inviters applies to it alone.
func TestParseCrafterFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name         string
		args         []string
		wantInviters []string
		wantErr      string
	}{
		{"crafter", append(base, "-persona", "crafter", "-datapack", "dp"), nil, ""},
		{"crafter without datapack", append(base, "-persona", "crafter"), nil, "-datapack"},
		{"inviters", append(base, "-persona", "crafter", "-datapack", "dp", "-clan-inviters", " Alden, ,Grae "), []string{"Alden", "Grae"}, ""},
		{"inviters for another preset", append(base, "-clan-inviters", "Alden"), nil, "-clan-inviters applies to -persona crafter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseSwarmFlags(tt.args, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !slices.Equal(o.clanInviters, tt.wantInviters) {
				t.Fatalf("inviters = %q, %v, want %q", o.clanInviters, err, tt.wantInviters)
			}
		})
	}
}

// TestCrafterPresetBrain: the crafter preset plays the crafter's cycle as
// a Dwarven Fighter, farms the spots, and accepts the clan invitation of
// the -clan-inviters names only when some are set.
func TestCrafterPresetBrain(t *testing.T) {
	p, ok := findPreset("crafter")
	if !ok {
		t.Fatal("crafter not in the catalog")
	}
	if p.class == nil || *p.class != botclient.DwarvenFighter || !p.datapack || !p.farms {
		t.Fatalf("crafter preset %+v: want a Dwarven Fighter farming with the datapack", p)
	}
	t0 := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	w, err := botclient.NewWorld(botclient.Self{ObjectID: 0x10000201, Name: "bot"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		inviters []string
		want     bool
	}{{nil, true}, {[]string{"Alden"}, false}, {[]string{"Player"}, true}} {
		sh := shared{
			spots: botclient.NewSpotRegistry(nil, 1), travel: &botclient.TravelMap{}, claims: botclient.NewPartyClaims(),
			auth: botclient.NewAuthority(botclient.AuthorityConfig{}), board: botclient.NewCraftBoard(), clanInviters: tt.inviters,
		}
		cfg := p.brain(sh)
		if cfg.Persona.Crafter.CheckEvery == 0 || cfg.Spots != sh.spots || cfg.CraftBoard != sh.board {
			t.Fatalf("crafter brain: crafter %+v spots %p board %p", cfg.Persona.Crafter, cfg.Spots, cfg.CraftBoard)
		}
		if got := cfg.Persona.JoinClan(w, "Player", "Crafters", t0); got != tt.want {
			t.Errorf("inviters %q: Player's invitation accepted %v, want %v", tt.inviters, got, tt.want)
		}
	}
}
