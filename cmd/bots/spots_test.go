package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// spotsNPCs, spotsSpawnlist and spotsTeleports are a datapack with one
// farming spot and one gatekeeper.
const (
	spotsNPCs = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<npc id="20002" name="Rabbit" title="" alias="rabbit">
		<set name="type" val="Monster"/>
		<set name="level" val="3"/>
	</npc>
	<npc id="30006" name="Roxxy" title="Gatekeeper" alias="rapunzel">
		<set name="type" val="Gatekeeper"/>
		<set name="level" val="70"/>
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
	<npcmaker name="town_01" territory="field_01" maximumNpcs="1">
		<npc id="30006" pos="-84108;244604;-3729;40960" total="1" respawn="1min"/>
	</npcmaker>
</list>
`
	spotsTeleports = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<telPosList npcId="30006">
		<loc desc="The Village of Gludin" type="STANDARD" priceId="57" priceCount="18000" x="-80749" y="149834" z="-3043" />
	</telPosList>
</list>
`
)

func TestLoadSpots(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"npcs/20000-20999.xml": spotsNPCs, "spawnlist/19_22.xml": spotsSpawnlist, "teleports.xml": spotsTeleports} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg, travel, err := loadSpots(dir, 1, log)
	if err != nil {
		t.Fatalf("loadSpots: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("spots = %d, want 1", reg.Len())
	}
	if travel.Gatekeepers() != 1 {
		t.Fatalf("gatekeepers = %d, want 1", travel.Gatekeepers())
	}
	if _, _, err := loadSpots(t.TempDir(), 1, log); err == nil {
		t.Fatal("loadSpots of an empty directory: no error")
	}
}
