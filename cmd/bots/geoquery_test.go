package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestGeoqueryWritesTheParityFile runs bots geopack then bots geoquery on
// the fixture region and checks the file is in geoprobe's dump format:
// sorted unique ids of the four categories, each with its answer fields,
// and the same seed writes the same bytes.
func TestGeoqueryWritesTheParityFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := geotest.Scenes().WriteL2OFF(dir); err != nil {
		t.Fatal(err)
	}
	props := filepath.Join(dir, "geoengine.properties")
	if err := os.WriteFile(props, []byte("GeoDataType = L2OFF\n20_18 =\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "geodata.cache")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"geopack", "-geodata", dir, "-geoengine", props, "-out", cache}, &stdout, &stderr); code != 0 {
		t.Fatalf("geopack exit %d, stderr %q", code, stderr.String())
	}
	write := func(out string) []byte {
		t.Helper()
		stdout.Reset()
		args := []string{"geoquery", "-geodata", dir, "-geoengine", props, "-geodata-cache", cache, "-out", out, "-seed", "3", "-n", "4"}
		if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
			t.Fatalf("geoquery exit %d, stderr %q", code, stderr.String())
		}
		for _, want := range []string{"sites wall 4, ledge 4, multilayer 4, random 4", "queries to " + out} {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout %q, want it to contain %q", stdout.String(), want)
			}
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	first := write(filepath.Join(dir, "a.dump"))
	if second := write(filepath.Join(dir, "b.dump")); !bytes.Equal(first, second) {
		t.Error("the same seed wrote a different file")
	}
	fields := map[string]string{"height": "\theight=", "canmove": "\tresult=", "los": "\tresult=", "validlocation": "\tx="}
	var ids []string
	seen := make(map[string]bool)
	sc := bufio.NewScanner(bytes.NewReader(first))
	for sc.Scan() {
		id, rest, ok := strings.Cut(sc.Text(), "\t")
		cat, _, _ := strings.Cut(id, ":")
		if !ok || !strings.HasPrefix("\t"+rest, fields[cat]) || fields[cat] == "" {
			t.Fatalf("line %q is not a geoprobe record", sc.Text())
		}
		seen[cat] = true
		ids = append(ids, id)
	}
	if len(seen) != len(fields) {
		t.Errorf("categories %v, want all of %v", seen, fields)
	}
	if !slices.IsSorted(ids) || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Error("ids are not sorted and unique")
	}
}

func TestGeoqueryRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	if _, err := geotest.Scenes().WriteL2OFF(dir); err != nil {
		t.Fatal(err)
	}
	props := writeProps(t, "20_18\n")
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no flags", nil, 1, "-geodata and -geoengine are required"},
		{"no listed region", []string{"-geodata", dir, "-geoengine", writeProps(t, "#20_18\n")}, 1, "lists no regions"},
		{"no cache", []string{"-geodata", dir, "-geoengine", props, "-geodata-cache", filepath.Join(dir, "absent")}, 1, "bots geopack"},
		{"unknown flag", []string{"-bogus"}, 2, "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"-out", filepath.Join(t.TempDir(), "q")}, tt.args...)
			if code := runGeoquery(args, &stdout, &stderr); code != tt.code {
				t.Fatalf("exit %d, want %d (stderr %q)", code, tt.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr %q, want it to mention %q", stderr.String(), tt.want)
			}
		})
	}
}
