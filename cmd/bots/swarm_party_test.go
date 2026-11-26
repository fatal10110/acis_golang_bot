package main

import (
	"io"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

func TestParseSwarmPartyFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name        string
		args        []string
		wantSupport int
		wantOff     bool
		wantErr     string
	}{
		{"defaults: parties on, no support", base, 0, false, ""},
		{"support beside farmers", append(base, "-support", "2"), 2, false, ""},
		{"every bot support", append(base, "-persona", "farmer", "-support", "3"), 3, false, ""},
		{"parties off", append(base, "-no-parties"), 0, true, ""},
		{"more support than bots", append(base, "-support", "4"), 0, false, "-support 4"},
		{"negative support", append(base, "-support", "-1"), 0, false, "-support -1"},
		{"support beside another persona", append(base, "-persona", "support", "-support", "1"), 0, false, "applies to -persona farmer"},
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
			if err != nil {
				t.Fatal(err)
			}
			if o.support != tt.wantSupport || o.noParties != tt.wantOff {
				t.Fatalf("support %d, no parties %v; want %d, %v", o.support, o.noParties, tt.wantSupport, tt.wantOff)
			}
		})
	}
}

// TestSwarmParties: the swarm runs the organizer only with a bot of a party
// preset and not -no-parties.
func TestSwarmParties(t *testing.T) {
	bots := func(presets ...string) []botregistry.OrganizedBot {
		var out []botregistry.OrganizedBot
		for i, p := range presets {
			out = append(out, botregistry.OrganizedBot{Account: "hz" + string(rune('1'+i)), Preset: p, Bot: &botclient.Bot{}})
		}
		return out
	}
	tests := []struct {
		name string
		off  bool
		bots []botregistry.OrganizedBot
		want bool
	}{
		{"farmers", false, bots("farmer", "farmer"), true},
		{"farmers and support", false, bots("farmer", "support"), true},
		{"support alone", false, bots("support"), true},
		{"off", true, bots("farmer", "support"), false},
		{"no party preset", false, bots("town", "merchant", "crafter", "pvp"), false},
		{"no bots", false, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := swarmParties(tt.off, tt.bots, botclient.SystemClock{}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := o != nil; got != tt.want {
				t.Fatalf("organizer %v, want %v", got, tt.want)
			}
		})
	}
}
