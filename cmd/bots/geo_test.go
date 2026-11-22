package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

func TestGeoFlags(t *testing.T) {
	all := []string{"-geodata", "geo", "-geoengine", "geoengine.properties", "-geodata-cache", "geo.cache", "-nav-cache", "nav.cache"}
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		want    geoOptions
		wantErr string
	}{
		{"none", nil, geoOptions{lruBytes: 64 << 20}, ""},
		{"all with the datapack", append([]string{"-datapack", "xml", "-nav-lru-bytes", "1024"}, all...), geoOptions{"geo", "geoengine.properties", "geo.cache", "nav.cache", 1024}, ""},
		{"without the datapack", all, geoOptions{}, "-geodata needs -datapack"},
		{"one missing", append([]string{"-datapack", "xml"}, all[:6]...), geoOptions{}, "go together"},
		{"zero cache", append([]string{"-datapack", "xml", "-nav-lru-bytes", "0"}, all...), geoOptions{}, "-nav-lru-bytes 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseSwarmFlags(append(append([]string{}, base...), tt.args...), io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if o.geo != tt.want {
				t.Fatalf("geo = %+v, want %+v", o.geo, tt.want)
			}
		})
	}
}

// TestWalkReady checks every preset walks, and refuses to start without the
// navigation (D13a).
func TestWalkReady(t *testing.T) {
	for _, p := range presets {
		if err := walkReady(p, nil); err == nil || !strings.Contains(err.Error(), "bots navbuild") {
			t.Errorf("%s without the navigation: %v, want a refusal naming bots navbuild", p.name, err)
		}
		if err := walkReady(p, &botclient.Nav{}); err != nil {
			t.Errorf("%s with the navigation: %v", p.name, err)
		}
	}
}

// TestGeoOpenFails checks a missing geoengine.properties fails the start
// before anything is mapped.
func TestGeoOpenFails(t *testing.T) {
	dir := t.TempDir()
	o := geoOptions{geodata: dir, geoengine: filepath.Join(dir, "missing.properties"), cache: filepath.Join(dir, "geo.cache"), navCache: filepath.Join(dir, "nav.cache"), lruBytes: 1 << 20}
	if n, closeNav, err := o.open(dir, slog.New(slog.DiscardHandler)); err == nil {
		closeNav()
		t.Fatalf("open = %v, want an error", n)
	}
}

// testGeoArgs builds, under a test directory, the synthetic geodata and nav
// caches of the geotest scenes as `bots geopack` and `bots navbuild` write
// them, next to a copy of the datapack fixture holding the scenes'
// doors.xml, and returns the flags a walking preset runs with over them.
func testGeoArgs(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	if _, err := geotest.Scenes().WriteL2OFF(dir); err != nil {
		t.Fatal(err)
	}
	props := filepath.Join(dir, "geoengine.properties")
	if err := os.WriteFile(props, []byte("GeoDataType = L2OFF\nMaxObstacleHeight = 32\nPartOfCharacterHeight = 75\n20_18 =\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	xml := filepath.Join(dir, "xml")
	if err := os.CopyFS(xml, os.DirFS(filepath.Join("..", "..", "internal", "datapack", "testdata", "xml"))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xml, "doors.xml"), []byte(corridorDoorXML()), 0o600); err != nil {
		t.Fatal(err)
	}
	cache, navCache := filepath.Join(dir, "geodata.cache"), filepath.Join(dir, "nav.cache")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"geopack", "-geodata", dir, "-geoengine", props, "-out", cache}, &stdout, &stderr); code != 0 {
		t.Fatalf("geopack exit %d, stderr %q", code, stderr.String())
	}
	args := []string{"navbuild", "-geodata", dir, "-geoengine", props, "-geodata-cache", cache, "-datapack", xml, "-out", navCache, "-workers", "2"}
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("navbuild exit %d, stderr %q", code, stderr.String())
	}
	return []string{"-datapack", xml, "-geodata", dir, "-geoengine", props, "-geodata-cache", cache, "-nav-cache", navCache}
}

// TestGeoOpen opens the caches testGeoArgs builds: the region-list hash is
// logged, and a nav cache built over other doors is refused, naming the
// command that rebuilds it.
func TestGeoOpen(t *testing.T) {
	args := testGeoArgs(t)
	o, err := parseSwarmFlags(append([]string{"-secret-file", "s", "-prefix", "hz", "-bots", "1"}, args...), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	n, closeNav, err := o.geo.open(o.datapack, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if n.Engine == nil || n.Hier == nil || n.Doors == nil {
		t.Errorf("nav %+v, want the engine, the hierarchy and the door table", n)
	}
	closeNav()
	regions := []geodata.Region{{X: geotest.RegionX, Y: geotest.RegionY}}
	if want := "region_hash=" + geodata.RegionListHash(regions); !strings.Contains(logs.String(), want) {
		t.Errorf("log %q, want %q", logs.String(), want)
	}
	if err := os.WriteFile(filepath.Join(o.datapack, "doors.xml"), []byte(corridorDoorXML()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.geo.open(o.datapack, slog.New(slog.DiscardHandler)); !errors.Is(err, nav.ErrNavCacheStale) || !strings.Contains(err.Error(), "bots navbuild") {
		t.Errorf("open over other doors: %v, want ErrNavCacheStale naming bots navbuild", err)
	}
}
