package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseStoreFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		want    storeOptions
		wantErr string
	}{
		{"defaults", base, storeOptions{budget: defaultStoreBudget, perTown: 5, spacing: 150}, ""},
		{
			"merchants", append(base, "-persona", "merchant", "-datapack", "xml", "-store-budget", "70000", "-stores-per-town", "3", "-store-spacing", "200"),
			storeOptions{budget: 70000, perTown: 3, spacing: 200},
			"",
		},
		{"budget of another preset", append(base, "-datapack", "xml", "-store-budget", "1"), storeOptions{}, "-store-budget applies to -persona merchant"},
		{"zero budget", append(base, "-persona", "merchant", "-datapack", "xml", "-store-budget", "0"), storeOptions{}, "-store-budget 0"},
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

// TestStoreRegistry builds the merchants' store registry over the towns of
// the spots fixture's travel map, one gatekeeper.
func TestStoreRegistry(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"npcs/20000-20999.xml": spotsNPCs, "spawnlist/19_22.xml": spotsSpawnlist, "teleports.xml": spotsTeleports, "buyLists.xml": spotsBuyLists, "items/0000-0099.xml": spotsItems, "skills/2000-2099.xml": spotsSkills, "recipes.xml": spotsRecipes, "armorSets.xml": spotsArmorSets, "zones/WaterZone.xml": "<list/>"} {
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
	if stores := (storeOptions{budget: 50000, perTown: 5, spacing: 150}).registry(travel, log); stores.Towns() != 1 {
		t.Fatalf("store towns %d, want 1", stores.Towns())
	}
}
