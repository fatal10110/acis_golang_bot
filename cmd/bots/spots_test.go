package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// spotsNPCs and spotsSpawnlist are a datapack with one farming spot.
const (
	spotsNPCs = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<npc id="20002" name="Rabbit" title="" alias="rabbit">
		<set name="type" val="Monster"/>
		<set name="level" val="3"/>
	</npc>
</list>
`
	spotsSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="field_01" minZ="-3500" maxZ="-3000">
		<node x="-100" y="-100"/>
		<node x="100" y="-100"/>
		<node x="0" y="100"/>
	</territory>
	<npcmaker name="field_01m1" territory="field_01" maximumNpcs="9">
		<npc id="20002" total="9" respawn="30sec"/>
	</npcmaker>
</list>
`
)

func TestLoadSpots(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"npcs/20000-20999.xml": spotsNPCs, "spawnlist/19_22.xml": spotsSpawnlist} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg, err := loadSpots(dir, 1, log)
	if err != nil {
		t.Fatalf("loadSpots: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("spots = %d, want 1", reg.Len())
	}
	if _, err := loadSpots(t.TempDir(), 1, log); err == nil {
		t.Fatal("loadSpots of an empty directory: no error")
	}
}
