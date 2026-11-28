package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestServitorLinesFlag checks -servitor-lines on swarm and ui: off by
// default (D51 answer 2), and every preset's persona takes it.
func TestServitorLinesFlag(t *testing.T) {
	swarm := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	ui := []string{"-secret-file", "s"}
	for _, tt := range []struct {
		name string
		args []string
		want bool
	}{
		{"off by default", nil, false},
		{"on", []string{"-servitor-lines"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			so, err := parseSwarmFlags(append(swarm, tt.args...), io.Discard)
			if err != nil || so.servitorLines != tt.want {
				t.Errorf("swarm: servitor lines, err = %v, %v; want %v, nil", so.servitorLines, err, tt.want)
			}
			uo, err := parseUIFlags(append(ui, tt.args...), io.Discard)
			if err != nil || uo.servitorLines != tt.want {
				t.Errorf("ui: servitor lines, err = %v, %v; want %v, nil", uo.servitorLines, err, tt.want)
			}
			w, cleanup, err := buildShared(wiringOptions{servitorLines: so.servitorLines}, botclient.SystemClock{}, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			for _, p := range presets {
				if got := p.brain(w.shared).Persona.ServitorLines; got != tt.want {
					t.Errorf("%s: Persona.ServitorLines %v, want %v", p.name, got, tt.want)
				}
			}
		})
	}
}
