package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// spotsNPCs, spotsSpawnlist, spotsTeleports, spotsItems and spotsBuyLists
// are a datapack with one farming spot, one gatekeeper and one merchant.
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
	spotsItems = `<?xml version="1.0" encoding="UTF-8"?>
<list>
	<item id="1" type="Weapon" name="Short Sword">
		<set name="weapon_type" val="SWORD" />
		<set name="bodypart" val="rhand" />
		<set name="price" val="768" />
	</item>
</list>
`
	spotsSkills = `<?xml version="1.0" encoding="UTF-8"?>
<list>
	<skill id="2031" levels="1" name="Lesser Healing Potion">
		<set name="isPotion" val="true" />
		<for>
			<effect count="7" name="HealOverTime" time="2" val="16" stackOrder="1" stackType="hp_recover" />
		</for>
	</skill>
</list>
`
	spotsBuyLists = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<buyList id="14" npcId="30004">
		<product id="1060" price="103"/>
	</buyList>
</list>
`
	spotsRecipes = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<recipe alias="mk_wooden_arrow" id="1" material="1864-4;1869-2" product="17-500" itemId="1666" level="1" mpConsume="30" successRate="100" isDwarven="true"/>
</list>
`
)

func TestLoadSpots(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"npcs/20000-20999.xml": spotsNPCs, "spawnlist/19_22.xml": spotsSpawnlist, "teleports.xml": spotsTeleports, "buyLists.xml": spotsBuyLists, "items/0000-0099.xml": spotsItems, "skills/2000-2099.xml": spotsSkills, "recipes.xml": spotsRecipes} {
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
