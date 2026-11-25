package main

import (
	"io"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestSpawnEventsFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "default the server's", want: []string{"extra_mob", "18age", "start_weapon"}},
		{name: "set", args: []string{"-spawn-events", "extra_mob; christmas;;"}, want: []string{"extra_mob", "christmas"}},
		{name: "none", args: []string{"-spawn-events", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseSwarmFlags(append([]string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}, tt.args...), io.Discard)
			if err != nil {
				t.Fatalf("parseSwarmFlags: %v", err)
			}
			cfg := supplyConfig(o.spawnEvents)
			if !slices.Equal(cfg.SpawnEvents, tt.want) {
				t.Fatalf("SpawnEvents %q, want %q", cfg.SpawnEvents, tt.want)
			}
			def := botclient.DefaultSupplyConfig()
			if cfg.KillFloor != def.KillFloor || cfg.NoRespawnCycle != def.NoRespawnCycle || cfg.SpoilWeight != def.SpoilWeight || cfg.SevenSignsShare != def.SevenSignsShare {
				t.Fatalf("supply config %+v, want the defaults but the events", cfg)
			}
		})
	}
}
