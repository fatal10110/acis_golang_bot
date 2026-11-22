package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// corridorDoorXML is the fixture corridor door as doors.xml lists it.
func corridorDoorXML() string {
	d := geotest.CorridorDoor()
	var loc strings.Builder
	for _, c := range d.Coordinates {
		fmt.Fprintf(&loc, `<loc x="%d" y="%d" z="%d"/>`, c.X, c.Y, d.Position.Z)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<list>
	<door id="%d" type="DOOR" level="1" name="%s">
		<position x="%d" y="%d" z="%d"/>
		<coordinates>%s</coordinates>
		<stats hp="1" pDef="1" mDef="1" height="%d"/>
	</door>
</list>`, d.ID, d.Name, d.Position.X, d.Position.Y, d.Position.Z, loc.String(), d.Height)
}

// TestNavbuildWritesTheNavCache builds the nav cache of the fixture region
// from its packed geodata cache, doors.xml and WaterZone.xml, then opens it
// under the key the geodata cache, doors.xml and the water zones give, and
// refuses it once doors.xml or the water changes.
func TestNavbuildWritesTheNavCache(t *testing.T) {
	dir := t.TempDir()
	if _, err := geotest.Scenes().WriteL2OFF(dir); err != nil {
		t.Fatal(err)
	}
	props := filepath.Join(dir, "geoengine.properties")
	if err := os.WriteFile(props, []byte("GeoDataType = L2OFF\n20_18 =\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	xml := filepath.Join(dir, "xml")
	if err := os.Mkdir(xml, 0o700); err != nil {
		t.Fatal(err)
	}
	doors := filepath.Join(xml, "doors.xml")
	if err := os.WriteFile(doors, []byte(corridorDoorXML()), 0o600); err != nil {
		t.Fatal(err)
	}
	writeWaterZones(t, xml)
	cache, out := filepath.Join(dir, "geodata.cache"), filepath.Join(dir, "nav.cache")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"geopack", "-geodata", dir, "-geoengine", props, "-out", cache}, &stdout, &stderr); code != 0 {
		t.Fatalf("geopack exit %d, stderr %q", code, stderr.String())
	}
	stdout.Reset()
	args := []string{"navbuild", "-geodata", dir, "-geoengine", props, "-geodata-cache", cache, "-datapack", xml, "-out", out, "-workers", "2"}
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("navbuild exit %d, stderr %q", code, stderr.String())
	}
	for _, want := range []string{"clusters 64", "doors 1 (0 skipped)", "entrance nodes", "wrote " + out} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout %q, want it to contain %q", stdout.String(), want)
		}
	}

	c, err := geodata.Open(dir, geodata.L2OFF, []geodata.Region{{X: geotest.RegionX, Y: geotest.RegionY}}, cache)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	water, err := loadWater(xml)
	if err != nil {
		t.Fatal(err)
	}
	key := nav.NavKey{GeoCRC: c.CRC(), DoorsHash: nav.DoorsHash([]byte(corridorDoorXML())), ClusterCells: nav.DefaultClusterCells, WaterHash: water.Hash()}
	g, err := nav.OpenNavCache(out, key)
	if err != nil {
		t.Fatalf("OpenNavCache: %v", err)
	}
	if g.Clusters() != 64 || g.Nodes() == 0 {
		t.Errorf("nav cache holds %d clusters and %d nodes", g.Clusters(), g.Nodes())
	}
	g.Close()
	if _, err := nav.OpenNavCache(out, nav.NavKey{GeoCRC: key.GeoCRC, DoorsHash: key.DoorsHash, ClusterCells: key.ClusterCells}); !errors.Is(err, nav.ErrNavCacheStale) {
		t.Errorf("without the water: %v, want ErrNavCacheStale", err)
	}
	key.DoorsHash = nav.DoorsHash([]byte(corridorDoorXML() + "\n"))
	if _, err := nav.OpenNavCache(out, key); !errors.Is(err, nav.ErrNavCacheStale) {
		t.Errorf("after doors.xml changed: %v, want ErrNavCacheStale", err)
	}
}

// writeWaterZones writes a WaterZone.xml with one lake in the fixture region
// under xmlDir/zones.
func writeWaterZones(t *testing.T, xmlDir string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(xmlDir, "zones"), 0o700); err != nil {
		t.Fatal(err)
	}
	x, y := geotest.CellWorldX(100), geotest.CellWorldY(100)
	body := fmt.Sprintf(`<list><zone name="lake" shape="Cuboid" minZ="-20" maxZ="20"><node x="%d" y="%d"/><node x="%d" y="%d"/></zone></list>`, x, y, x+160, y+160)
	if err := os.WriteFile(filepath.Join(xmlDir, "zones", "WaterZone.xml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNavbuildRejectsBadInput(t *testing.T) {
	doorsOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(doorsOnly, "doors.xml"), []byte(corridorDoorXML()), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no flags", nil, "-geodata, -geoengine and -datapack are required"},
		{"no workers", []string{"-geodata", "g", "-geoengine", "p", "-datapack", "x", "-workers", "0"}, "-workers must be at least 1"},
		{"no listed region", []string{"-geodata", t.TempDir(), "-geoengine", writeProps(t, "GeoDataType = L2OFF\n#20_18\n"), "-datapack", t.TempDir()}, "lists no regions"},
		{"no doors.xml", []string{"-geodata", t.TempDir(), "-geoengine", writeProps(t, "20_18\n"), "-datapack", t.TempDir()}, "doors.xml"},
		{"no WaterZone.xml", []string{"-geodata", t.TempDir(), "-geoengine", writeProps(t, "20_18\n"), "-datapack", doorsOnly}, "WaterZone.xml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), append([]string{"navbuild"}, tc.args...), &stdout, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1; stderr %q", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr %q, want it to contain %q", stderr.String(), tc.want)
			}
		})
	}
}
