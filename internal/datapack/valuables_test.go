package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// writeXMLFile writes body to a fresh temp dir as name and returns the dir.
func writeXMLFile(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadNPCDropsAndPrivates(t *testing.T) {
	dir := writeXMLFile(t, "a.xml", `<list>
	<npc id="21544" name="Judge of Splendor" type="Monster">
		<drops>
			<category type="DROP" chance="70.5">
				<drop itemid="57" min="10" max="20" chance="60"/>
				<drop itemid="0x10" min="1" max="1" chance="40.25"/>
			</category>
			<category type="SPOIL">
				<drop itemid="1864" min="1" max="3" chance="100"/>
			</category>
		</drops>
		<drops>
			<category type="HERB" chance="1"><drop itemid="8600" min="1" max="1" chance="100"/></category>
			<category type="CURRENCY" chance="2"/>
		</drops>
		<privates>
			<private id="21546" weight="2" respawn="100sec"/>
			<private id="21547" weight="1" respawn="no"/>
		</privates>
	</npc>
	<npc id="1" name="Plain" type="Monster"/>
</list>`)
	got, err := LoadNPCs(dir)
	if err != nil {
		t.Fatalf("LoadNPCs: %v", err)
	}
	wantDrops := []DropCategory{
		{Kind: DropKindDrop, Chance: 70.5, Drops: []Drop{
			{ItemID: 57, Min: 10, Max: 20, Chance: 60},
			{ItemID: 16, Min: 1, Max: 1, Chance: 40.25},
		}},
		{Kind: DropKindSpoil, Chance: 100, Drops: []Drop{{ItemID: 1864, Min: 1, Max: 3, Chance: 100}}},
		{Kind: DropKindHerb, Chance: 1, Drops: []Drop{{ItemID: 8600, Min: 1, Max: 1, Chance: 100}}},
		{Kind: DropKindCurrency, Chance: 2, Drops: []Drop{}},
	}
	wantPrivates := []Private{
		{NPCID: 21546, Weight: 2, Respawn: 100 * time.Second},
		{NPCID: 21547, Weight: 1, Respawn: NoRespawn},
	}
	if npc := got[21544]; !reflect.DeepEqual(npc.Drops, wantDrops) || !reflect.DeepEqual(npc.Privates, wantPrivates) {
		t.Errorf("drops = %+v, privates = %+v; want %+v, %+v", npc.Drops, npc.Privates, wantDrops, wantPrivates)
	}
	if npc := got[1]; npc.Drops != nil || npc.Privates != nil {
		t.Errorf("plain npc drops = %+v, privates = %+v; want nil", npc.Drops, npc.Privates)
	}
}

func TestLoadNPCDropsRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"category type missing", `<drops><category><drop itemid="1" min="1" max="1" chance="1"/></category></drops>`, `"type"`},
		{"category type unknown", `<drops><category type="QUEST"/></drops>`, `unknown drop kind "QUEST"`},
		{"category chance", `<drops><category type="DROP" chance="x"/></drops>`, `"chance"`},
		{"drop itemid", `<drops><category type="DROP"><drop min="1" max="1" chance="1"/></category></drops>`, `"itemid"`},
		{"drop min", `<drops><category type="DROP"><drop itemid="1" min="a" max="1" chance="1"/></category></drops>`, `"min"`},
		{"drop max", `<drops><category type="DROP"><drop itemid="1" min="1" max="1.5" chance="1"/></category></drops>`, `"max"`},
		{"drop chance missing", `<drops><category type="DROP"><drop itemid="1" min="1" max="1"/></category></drops>`, `"chance"`},
		{"drop chance mangled", `<drops><category type="DROP"><drop itemid="1" min="1" max="1" chance="5%"/></category></drops>`, `"chance"`},
		{"private id", `<privates><private weight="1" respawn="1sec"/></privates>`, `"id"`},
		{"private weight", `<privates><private id="2" weight="x" respawn="1sec"/></privates>`, `"weight"`},
		{"private respawn missing", `<privates><private id="2" weight="1"/></privates>`, `"respawn"`},
		{"private respawn mangled", `<privates><private id="2" weight="1" respawn="xsec"/></privates>`, `"respawn"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeXMLFile(t, "a.xml", `<list><npc id="7" name="x" type="Monster">`+tt.body+`</npc></list>`)
			_, err := LoadNPCs(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "npc 7") {
				t.Fatalf("LoadNPCs err = %v, want one naming npc 7 and %s", err, tt.want)
			}
		})
	}
}

func TestLoadSpawnlistMakerAIAndPrivates(t *testing.T) {
	dir := writeXMLFile(t, "a.xml", `<list>
	<npcmaker name="event_maker" maximumNpcs="2" spawnTime="siege_warfare_start(8)">
		<ai type="first_type"><set name="EventName" val="old"/></ai>
		<ai type="event_maker">
			<set name="EventName" val="extra_mob"/>
			<set name="IsNight" val="1"/>
			<set name="on_start_spawn" val="0"/>
			<set name="maker_name1" val="@linked_a"/>
			<set name="maker_name2" val="linked_b"/>
			<set name="maker_name3" val="linked_c"/>
			<set name="maker_cnt" val="3"/>
			<set name="Seal" val="@SEAL_REVELATION"/>
		</ai>
		<npc id="21544" total="2" respawn="100sec">
			<privates><private id="21546" weight="2" respawn="0sec"/></privates>
			<privates><private id="21547" weight="1"/></privates>
			<ai><set name="EventName" val="ignored"/></ai>
		</npc>
		<npc id="20001" total="1" respawn="no"/>
	</npcmaker>
	<npcmaker name="attr_maker" maximumNpcs="1" maker="default_use_db_maker">
		<ai><set name="Seal" val="x"/></ai>
	</npcmaker>
	<npcmaker name="plain" maximumNpcs="1"/>
</list>`)
	sl, err := LoadSpawnlist(dir)
	if err != nil {
		t.Fatalf("LoadSpawnlist: %v", err)
	}
	want := []*Maker{
		{
			Name: "event_maker", MaximumNPCs: 2, SpawnTime: "siege_warfare_start(8)", AIType: "event_maker",
			Params: map[string]string{
				"EventName": "extra_mob", "IsNight": "1", "on_start_spawn": "0",
				"maker_name1": "linked_a", "maker_name2": "linked_b", "maker_name3": "linked_c", "maker_cnt": "3",
			},
			Spawns: []Spawn{
				{NPCID: 21544, Total: 2, Respawn: 100 * time.Second, Privates: []Private{
					{NPCID: 21546, Weight: 2}, {NPCID: 21547, Weight: 1},
				}},
				{NPCID: 20001, Total: 1, Respawn: NoRespawn},
			},
		},
		{Name: "attr_maker", MaximumNPCs: 1, AIType: "default_use_db_maker", Spawns: []Spawn{}},
		{Name: "plain", MaximumNPCs: 1, Spawns: []Spawn{}},
	}
	if len(sl.Makers) != len(want) {
		t.Fatalf("got %d makers, want %d", len(sl.Makers), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(sl.Makers[i], want[i]) {
			t.Errorf("Makers[%d] = %+v, want %+v", i, sl.Makers[i], want[i])
		}
	}
}

func TestLoadSpawnlistPrivateRejects(t *testing.T) {
	tests := []struct {
		name, private, want string
	}{
		{"id", `<private weight="1"/>`, `"id"`},
		{"weight", `<private id="1"/>`, `"weight"`},
		{"respawn", `<private id="1" weight="1" respawn="5xsec"/>`, `"respawn"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeXMLFile(t, "a.xml", `<list><npcmaker name="m" maximumNpcs="1"><npc id="3" total="1"><privates>`+
				tt.private+`</privates></npc></npcmaker></list>`)
			_, err := LoadSpawnlist(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadSpawnlist err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}

func TestLoadMultisells(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []Multisell{{
		Name: "002", NPCs: []int32{30001, 30002},
		Rows: []MultisellRow{
			{
				Ingredients: []MultisellItem{{ItemID: 57, Count: 2500}, {ItemID: 1, Count: 1}},
				Products:    []MultisellItem{{ItemID: 10, Count: 1}},
			},
			{
				Ingredients: []MultisellItem{{ItemID: 5575, Count: 588000}},
				Products:    []MultisellItem{{ItemID: 71, Count: 1}, {ItemID: 17, Count: 20}},
			},
		},
	}}
	if !reflect.DeepEqual(dp.Multisells, want) {
		t.Errorf("Multisells = %+v, want %+v", dp.Multisells, want)
	}
}

func TestLoadMultisellsRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"npc text", `<npcs><npc> 30001</npc></npcs>`, "npc: strconv.ParseInt"},
		{"npc empty", `<npcs><npc></npc></npcs>`, "npc: strconv.ParseInt"},
		{"ingredient id", `<item><ingredient count="1"/><production id="1" count="1"/></item>`, `ingredient: attribute "id"`},
		{"ingredient count", `<item><ingredient id="1" count="x"/><production id="1" count="1"/></item>`, `ingredient: attribute "count"`},
		{"production id", `<item><ingredient id="1" count="1"/><production id="0x1" count="1"/></item>`, `production: attribute "id"`},
		{"production count", `<item><ingredient id="1" count="1"/><production id="1"/></item>`, `production: attribute "count"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeXMLFile(t, "001.xml", `<list>`+tt.body+`</list>`)
			_, err := LoadMultisells(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadMultisells err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}

func TestLoadRestartPoints(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []RestartPoint{
		{Name: "talking_island_town", Tiles: []MapTile{{15, 23}, {16, 23}}},
		{Name: "no_tiles"},
	}
	if !reflect.DeepEqual(dp.RestartPoints, want) {
		t.Errorf("RestartPoints = %+v, want %+v", dp.RestartPoints, want)
	}
}

func TestLoadRestartPointsRejects(t *testing.T) {
	tests := []struct {
		name, sets, want string
	}{
		{"name", `<set name="map" val="1;2"/>`, `"name"`},
		{"map separator", `<set name="name" val="p"/><set name="map" val="1,2"/>`, "must be formatted x;y"},
		{"map x", `<set name="name" val="p"/><set name="map" val="a;2"/>`, `map "a;2"`},
		{"map y", `<set name="name" val="p"/><set name="map" val="1;"/>`, `map "1;"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeXMLFile(t, "restartPointAreas.xml", `<list><point>`+tt.sets+`</point></list>`)
			_, err := LoadRestartPoints(filepath.Join(dir, "restartPointAreas.xml"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadRestartPoints err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}

func TestLoadItemNames(t *testing.T) {
	dir := writeXMLFile(t, "a.xml", `<list>
	<item id="5458" type="EtcItem" name="Recipe: Halberd(60%)">
		<set name="etcitem_type" val="RECIPE"/>
		<set name="price" val="366000"/>
	</item>
	<item id="1" type="Weapon" name="Short Sword"><set name="price" val="768"/></item>
	<item id="2" type="EtcItem" name="Unpriced"><set name="etcitem_type" val="RECIPE"/></item>
	<item id="3" type="EtcItem" name="Untradable">
		<set name="price" val="10"/><set name="is_tradable" val="false"/>
	</item>
	<item id="4" type="EtcItem" name="Old Name"><set name="price" val="5"/></item>
	<item id="4" type="EtcItem" name="New Name"><set name="price" val="6"/></item>
</list>`)
	got, err := LoadItems(dir)
	if err != nil {
		t.Fatalf("LoadItems: %v", err)
	}
	wantNames := map[int32]string{5458: "Recipe: Halberd(60%)", 1: "Short Sword", 4: "New Name"}
	wantPrices := map[int32]int32{5458: 366000, 1: 768, 4: 6}
	if !reflect.DeepEqual(got.Names, wantNames) || !reflect.DeepEqual(got.Prices, wantPrices) {
		t.Errorf("Names = %v, Prices = %v; want %v, %v", got.Names, got.Prices, wantNames, wantPrices)
	}
	if _, ok := got.Etc[5458]; ok {
		t.Error("recipe scroll 5458 kept as an etc item")
	}
}

// TestLoadShippedValuables loads the valuables tables of a real datapack
// when BOT_DATAPACK_XML names its data/xml directory, and skips otherwise.
func TestLoadShippedValuables(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := Load(dir)
	if err != nil {
		t.Fatalf("Load(%s): %v", dir, err)
	}

	kinds := map[DropKind]int{}
	withPrivates := 0
	for _, npc := range dp.NPCs {
		for _, c := range npc.Drops {
			kinds[c.Kind]++
		}
		if len(npc.Privates) > 0 {
			withPrivates++
		}
	}
	wantKinds := map[DropKind]int{DropKindDrop: 4561, DropKindSpoil: 1645, DropKindCurrency: 1905, DropKindHerb: 7362}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Errorf("drop categories = %v, want %v", kinds, wantKinds)
	}
	if withPrivates != 205 {
		t.Errorf("templates with privates = %d, want 205", withPrivates)
	}

	entryPrivates, aiTypes, params, spawnTimes := 0, 0, 0, 0
	for _, m := range dp.Spawnlist.Makers {
		if m.AIType != "" {
			aiTypes++
		}
		if m.SpawnTime != "" {
			spawnTimes++
		}
		params += len(m.Params)
		for _, s := range m.Spawns {
			if len(s.Privates) > 0 {
				entryPrivates++
			}
		}
	}
	t.Logf("makers=%d with ai type=%d with spawnTime=%d params=%d entries with privates=%d",
		len(dp.Spawnlist.Makers), aiTypes, spawnTimes, params, entryPrivates)
	if aiTypes == 0 || spawnTimes == 0 || params == 0 || entryPrivates == 0 {
		t.Error("shipped spawnlist read no maker ai, spawnTime, params or entry privates")
	}

	if len(dp.Names) != len(dp.Prices) {
		t.Errorf("names = %d, prices = %d; want one name per price", len(dp.Names), len(dp.Prices))
	}
	if got := dp.Names[5458]; got != "Recipe: Halberd(60%)" {
		t.Errorf("Names[5458] = %q, want the recipe scroll's name", got)
	}

	rows := 0
	for _, m := range dp.Multisells {
		rows += len(m.Rows)
	}
	// 779 of the 939 RECIPE scrolls are priced and tradable, 774 of them
	// named "Recipe: ..."; buildEtcItem keeps none of them.
	recipes := 0
	for id, name := range dp.Names {
		_, etc := dp.EtcItems[id]
		if strings.HasPrefix(name, "Recipe:") && !etc {
			recipes++
		}
	}
	t.Logf("names=%d recipe names=%d multisells=%d rows=%d restart points=%d",
		len(dp.Names), recipes, len(dp.Multisells), rows, len(dp.RestartPoints))
	if recipes != 774 {
		t.Errorf("recipe scroll names = %d, want 774", recipes)
	}
	if len(dp.Multisells) == 0 || rows == 0 {
		t.Error("shipped datapack loaded no multisell rows")
	}

	wantTI := []MapTile{{15, 23}, {15, 24}, {15, 25}, {16, 23}, {16, 24}, {16, 25}, {17, 24}, {17, 25}, {18, 25}}
	found := false
	for _, p := range dp.RestartPoints {
		if p.Name == "talking_island_town" {
			found = true
			if !reflect.DeepEqual(p.Tiles, wantTI) {
				t.Errorf("talking_island_town tiles = %v, want %v", p.Tiles, wantTI)
			}
		}
	}
	if !found {
		t.Error("no talking_island_town restart point")
	}
}
