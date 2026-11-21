package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestGeopackWritesTheCacheTheBotsMap pins bots geopack end to end: it packs
// the regions geoengine.properties lists (a commented-out line lists none)
// into a cache geodata.Open maps, and reports the blocks and the list hash.
func TestGeopackWritesTheCacheTheBotsMap(t *testing.T) {
	dir := t.TempDir()
	if _, err := geotest.Scenes().WriteL2OFF(dir); err != nil {
		t.Fatal(err)
	}
	props := filepath.Join(dir, "geoengine.properties")
	if err := os.WriteFile(props, []byte("GeoDataType = L2OFF\nMaxObstacleHeight = 32\n#20_19\n20_18 =\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "geodata.cache")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"geopack", "-geodata", dir, "-geoengine", props, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("geopack exit %d, stderr %q", code, stderr.String())
	}
	regions := []geodata.Region{{X: geotest.RegionX, Y: geotest.RegionY}}
	for _, want := range []string{"regions 1 (list hash " + geodata.RegionListHash(regions) + ")", "multilayer", "wrote " + out} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout %q, want it to contain %q", stdout.String(), want)
		}
	}
	c, err := geodata.Open(dir, geodata.L2OFF, regions, out)
	if err != nil {
		t.Fatalf("Open() on the packed cache: %v", err)
	}
	c.Close()
}

func TestGeopackRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.properties")
	if err := os.WriteFile(empty, []byte("GeoDataType = L2OFF\n#20_18\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no flags", nil, 1, "-geodata and -geoengine are required"},
		{"missing properties", []string{"-geodata", dir, "-geoengine", filepath.Join(dir, "absent")}, 1, "absent"},
		{"no listed region", []string{"-geodata", dir, "-geoengine", empty}, 1, "lists no regions"},
		{"listed region file missing", []string{"-geodata", t.TempDir(), "-geoengine", writeProps(t, "20_18\n")}, 1, "20_18_conv.dat"},
		{"unknown flag", []string{"-bogus"}, 2, "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"-out", filepath.Join(t.TempDir(), "c")}, tt.args...)
			if code := runGeopack(args, &stdout, &stderr); code != tt.code {
				t.Fatalf("exit %d, want %d (stderr %q)", code, tt.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr %q, want it to mention %q", stderr.String(), tt.want)
			}
		})
	}
}

func writeProps(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "geoengine.properties")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
