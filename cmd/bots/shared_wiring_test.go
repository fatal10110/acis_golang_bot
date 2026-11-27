package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// lakeDatapack writes the spots datapack (TestLoadSpots) whose one farming
// spot lies in the lake writeWaterZones writes, with the corridor's
// doors.xml, next to the fixture region's geodata, packs it and builds its
// nav cache, and returns the datapack directory and the geodata flags over
// them.
func lakeDatapack(t *testing.T) (string, geoOptions) {
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
	if err := os.Mkdir(xml, 0o700); err != nil {
		t.Fatal(err)
	}
	writeWaterZones(t, xml)
	x, y := geotest.CellWorldX(100), geotest.CellWorldY(100)
	lakeSpawnlist := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="lake_01" minZ="-100" maxZ="100">
		<node x="%d" y="%d"/>
		<node x="%d" y="%d"/>
		<node x="%d" y="%d"/>
		<node x="%d" y="%d"/>
	</territory>
	<npcmaker name="lake_01m1" territory="lake_01" maximumNpcs="9">
		<npc id="20002" total="9" respawn="30sec"/>
	</npcmaker>
</list>
`, x+32, y+32, x+128, y+32, x+128, y+128, x+32, y+128)
	for name, body := range map[string]string{"doors.xml": corridorDoorXML(), "npcs/20000-20999.xml": spotsNPCs, "spawnlist/20_18.xml": lakeSpawnlist, "teleports.xml": spotsTeleports, "buyLists.xml": spotsBuyLists, "items/0000-0099.xml": spotsItems, "skills/2000-2099.xml": spotsSkills, "recipes.xml": spotsRecipes, "armorSets.xml": spotsArmorSets, "classes/humanFighter.xml": spotsClasses, "hennas.xml": "<list/>", "zones/ArenaZone.xml": "<list/>", "zones/PeaceZone.xml": "<list/>", "zones/TownZone.xml": "<list/>", "zones/DerbyTrackZone.xml": "<list/>", "zones/SiegeZone.xml": "<list/>", "zones/NoRestartZone.xml": "<list/>", "multisell/001.xml": "<list/>", "restartPointAreas.xml": "<list/>"} {
		path := filepath.Join(xml, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
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
	return xml, geoOptions{geodata: dir, geoengine: props, cache: cache, navCache: navCache, lruBytes: 1 << 20}
}

// TestBuildShared builds the set `bots swarm` and `bots ui` share (#656).
// With -datapack and the geodata flags the navigation opens before the
// spots: the spot registry drops the spot in the lake of the navigation it
// was given. Without them those pieces are nil; the store registry is built
// only when asked; the claims, boards and gate always are.
func TestBuildShared(t *testing.T) {
	xml, geo := lakeDatapack(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	outcomeLog := filepath.Join(t.TempDir(), "outcomes.log")
	tests := []struct {
		name string
		o    wiringOptions
		// spots is the spot registry's size, -1 for no registry.
		spots                         int
		wantNav, wantStores, wantOutc bool
	}{
		{"datapack and navigation: the lake's spot dropped", wiringOptions{datapack: xml, spawnMultiplier: 1, geo: geo, stores: true}, 0, true, true, false},
		{"datapack alone keeps the lake's spot", wiringOptions{datapack: xml, spawnMultiplier: 1, stores: true}, 1, false, true, false},
		{"no store registry unless asked", wiringOptions{datapack: xml, spawnMultiplier: 1, geo: geo}, 0, true, false, false},
		{"neither: the pieces are nil", wiringOptions{allowlisted: true, stores: true}, -1, false, false, false},
		{"the outcome log", wiringOptions{outcomeLog: outcomeLog}, -1, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, cleanup, err := buildShared(tt.o, botclient.SystemClock{}, log)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			sh := w.shared
			if w.gate == nil || sh.claims == nil || sh.ground == nil || sh.challenges == nil || sh.board == nil || sh.ads == nil {
				t.Fatalf("gate %v, shared %+v: want the gate, the claims and the boards", w.gate, sh)
			}
			if sh.auth != nil || sh.clanInviters != nil {
				t.Errorf("auth %v, clanInviters %v: want them left to the caller", sh.auth, sh.clanInviters)
			}
			if got := sh.nav != nil; got != tt.wantNav {
				t.Errorf("nav %v, want set %v", sh.nav, tt.wantNav)
			}
			if got := sh.stores != nil; got != tt.wantStores {
				t.Errorf("stores %v, want set %v", sh.stores, tt.wantStores)
			}
			if got := sh.outcomes != nil; got != tt.wantOutc {
				t.Errorf("outcomes %v, want set %v", sh.outcomes, tt.wantOutc)
			}
			if tt.spots < 0 {
				if sh.spots != nil || sh.travel != nil || w.dp != nil {
					t.Errorf("spots %v, travel %v, datapack %v: want none without -datapack", sh.spots, sh.travel, w.dp)
				}
				return
			}
			if sh.spots == nil || sh.travel == nil || w.dp == nil {
				t.Fatalf("spots %v, travel %v, datapack %v: want all with -datapack", sh.spots, sh.travel, w.dp)
			}
			if sh.spots.Len() != tt.spots {
				t.Errorf("spots %d, want %d", sh.spots.Len(), tt.spots)
			}
		})
	}
	t.Run("a navigation that fails to open", func(t *testing.T) {
		bad := geo
		bad.navCache = filepath.Join(t.TempDir(), "missing.cache")
		if _, cleanup, err := buildShared(wiringOptions{datapack: xml, geo: bad, outcomeLog: filepath.Join(t.TempDir(), "o.log")}, botclient.SystemClock{}, log); err == nil || cleanup != nil {
			t.Fatalf("err %v, cleanup set %v: want an error and no cleanup", err, cleanup != nil)
		}
	})
}
