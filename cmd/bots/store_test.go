package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestParseStoreFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		want    storeOptions
		wantErr string
	}{
		{"defaults", base, storeOptions{perTown: 5, spacing: 150}, ""},
		{
			"merchants", append(base, "-datapack", "xml", "-store-budget", "50000", "-stores-per-town", "3", "-store-spacing", "200"),
			storeOptions{budget: 50000, perTown: 3, spacing: 200},
			"",
		},
		{"budget without datapack", append(base, "-store-budget", "1"), storeOptions{}, "-store-budget needs -datapack"},
		{"negative budget", append(base, "-store-budget", "-1"), storeOptions{}, "-store-budget"},
		{"no store per town", append(base, "-stores-per-town", "0"), storeOptions{}, "-stores-per-town"},
		{"negative spacing", append(base, "-store-spacing", "-1"), storeOptions{}, "-store-spacing"},
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
			if err != nil || o.store != tt.want {
				t.Fatalf("store options = %+v, %v, want %+v", o.store, err, tt.want)
			}
		})
	}
}

// TestStoreApply makes the bots merchants over the towns of the spots
// fixture's travel map, one gatekeeper.
func TestStoreApply(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"npcs/20000-20999.xml": spotsNPCs, "spawnlist/19_22.xml": spotsSpawnlist, "teleports.xml": spotsTeleports, "buyLists.xml": spotsBuyLists, "items/0000-0099.xml": spotsItems} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, travel, err := loadSpots(dir, 1, log)
	if err != nil {
		t.Fatal(err)
	}
	cfg := botclient.BrainConfig{Persona: botclient.DefaultPersona(), Travel: travel}
	storeOptions{perTown: 5, spacing: 150}.apply(&cfg, log)
	if cfg.Stores != nil || cfg.Persona.Store.Budget != 0 {
		t.Fatalf("no budget: stores %v, budget %d, want none", cfg.Stores, cfg.Persona.Store.Budget)
	}
	storeOptions{budget: 50000, perTown: 5, spacing: 150}.apply(&cfg, log)
	if cfg.Stores == nil || cfg.Stores.Towns() != 1 || cfg.Persona.Store.Budget != 50000 {
		t.Fatalf("stores %v, budget %d, want one town and 50000", cfg.Stores, cfg.Persona.Store.Budget)
	}
}
