package main

import (
	"io"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestParsePersonaFlag(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{"default", base, personaSupport, ""},
		{"town", append(base, "-persona", "town", "-datapack", "dp"), personaTown, ""},
		{"town without datapack", append(base, "-persona", "town"), "", "-datapack"},
		{"support", append(base, "-persona", "support"), personaSupport, ""},
		{"unknown", append(base, "-persona", "merchant"), "", "-persona"},
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
			if err != nil || o.persona != tt.want {
				t.Fatalf("persona = %q, %v, want %q", o.persona, err, tt.want)
			}
		})
	}
}

// TestApplyPersona gives town bots the town persona and no spot registry,
// and support bots the default persona with their spots.
func TestApplyPersona(t *testing.T) {
	spots := botclient.NewSpotRegistry(nil, 1)
	tests := []struct {
		persona   string
		wantHops  bool
		wantSpots bool
	}{
		{personaSupport, false, true},
		{personaTown, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.persona, func(t *testing.T) {
			cfg := botclient.BrainConfig{Spots: spots}
			applyPersona(tt.persona, &cfg)
			if got := cfg.Persona.Town.HopMax > 0; got != tt.wantHops {
				t.Fatalf("town hops %v, want %v", got, tt.wantHops)
			}
			if got := cfg.Spots != nil; got != tt.wantSpots {
				t.Fatalf("spots %v, want %v", got, tt.wantSpots)
			}
		})
	}
}
