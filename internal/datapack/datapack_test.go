package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// combatDefaults are the combat stats of a template that sets none, the
// server's defaults (data/xml/npc.go buildNPCTemplate).
var combatDefaults = Combat{AtkSpd: 300, Crit: 4, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20}

// corpseDefault is the corpse time of a template that sets none.
const corpseDefault = 7 * time.Second

func TestLoadSamples(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantNPCs := map[int32]NPC{
		20001: {
			ID: 20001, Name: "Gremlin", Type: "Monster", Level: 1, AggroRange: 1000, RunSpeed: 108.9, ClanRange: 300,
			Combat:     Combat{HP: 39, PAtk: 7.56, PDef: 39, AtkSpd: 247.42, Crit: 4, STR: 40, CON: 43, DEX: 30, INT: 20, WIT: 43, MEN: 20},
			CorpseTime: corpseDefault,
			Drops:      []DropCategory{{Kind: DropKindDrop, Chance: 50, Drops: []Drop{{ItemID: 57, Min: 1, Max: 2, Chance: 70}}}},
		},
		20002: {ID: 20002, Name: "Rabbit", Type: "Monster", Level: 3, Combat: combatDefaults, CorpseTime: corpseDefault},
		20101: {ID: 20101, Name: "Set Name", Type: "Folk", Level: 1, Combat: combatDefaults, CorpseTime: corpseDefault},
		25001: {ID: 25001, Name: "Greyclaw Kutus", Type: "RaidBoss", Level: 23, AggroRange: 300, Combat: combatDefaults, CorpseTime: corpseDefault},
		30010: {
			ID: 30010, Name: "Auron", Type: "Trainer", Level: 70,
			Combat:     Combat{MAtk: 430, MDef: 350, AtkSpd: 300, Crit: 4, STR: 40, CON: 21, DEX: 30, INT: 21, WIT: 20, MEN: 10},
			CorpseTime: 10 * time.Second,
			TeachTo:    []int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		},
	}
	if !reflect.DeepEqual(dp.NPCs, wantNPCs) {
		t.Errorf("NPCs = %+v, want %+v", dp.NPCs, wantNPCs)
	}

	sl := dp.Spawnlist
	ssq := &Territory{
		Name: "ssqteleport1_1612_31", MinZ: -6786, MaxZ: -6586,
		Nodes: []Point{{-112364, -182829}, {-111569, -182829}, {-111569, -182040}, {-112364, -182040}},
	}
	gludio := &Territory{
		Name: "gludio02_1922_10", MinZ: -3500, MaxZ: -3000,
		Nodes: []Point{{-100, -100}, {100, -100}, {0, 100}},
	}
	ban := &Territory{
		Name: "gludio02_1922_05b", MinZ: -3500, MaxZ: -3000,
		Nodes: []Point{{-10, -10}, {10, -10}, {0, 10}},
	}
	dup := &Territory{
		Name: "GLUDIO02_1922_10", MinZ: 0, MaxZ: 10,
		Nodes: []Point{{0, 0}, {1, 0}, {0, 1}},
	}
	wantTerritories := []*Territory{ssq, gludio, ban, dup}
	if !reflect.DeepEqual(sl.Territories, wantTerritories) {
		t.Errorf("Territories = %+v, want %+v", sl.Territories, wantTerritories)
	}

	wantMakers := []*Maker{
		{
			Name: "dimension_gap_00", Territories: []*Territory{ssq}, MaximumNPCs: 9, AIType: "manage_teleport_dungeon",
			Spawns: []Spawn{{NPCID: 21659, Total: 2, Respawn: 90 * time.Second}},
		},
		{
			Name: "gludio02_1922_1001", Territories: []*Territory{gludio}, Banned: []*Territory{ban}, MaximumNPCs: 20,
			Spawns: []Spawn{
				{NPCID: 20001, Total: 5, Respawn: 30 * time.Second, RespawnRandom: 10 * time.Second},
				{NPCID: 20002, Total: 3, Respawn: time.Minute},
			},
		},
		{
			Name: "ssq_event_maker", Territories: []*Territory{ssq, gludio}, MaximumNPCs: 4, Event: "ssq_seal1_dawn",
			Spawns: []Spawn{{
				NPCID: 25001, Total: 1, Respawn: 48 * time.Hour, RespawnRandom: 20 * time.Hour,
				Positions: []Position{
					{X: 55024, Y: 17368, Z: -5412, Chance: 40},
					{X: 53504, Y: 21248, Z: -5496, Chance: 60},
				},
			}},
		},
		{
			Name: "fixed_npc", MaximumNPCs: 1,
			Spawns: []Spawn{{
				NPCID: 20002, Total: 1, Respawn: NoRespawn,
				Positions: []Position{{X: 214880, Y: -116144, Z: -1644}},
			}},
		},
		{Name: "partly_missing", MaximumNPCs: 2, Spawns: []Spawn{}},
	}
	if len(sl.Makers) != len(wantMakers) {
		t.Fatalf("got %d makers, want %d", len(sl.Makers), len(wantMakers))
	}
	for i, want := range wantMakers {
		if !reflect.DeepEqual(sl.Makers[i], want) {
			t.Errorf("Makers[%d] = %+v, want %+v", i, sl.Makers[i], want)
		}
	}

	file := filepath.Join("testdata", "xml", "spawnlist", "19_22.xml")
	wantWarnings := []string{
		file + `: skipping territory "two_nodes": needs at least 3 nodes`,
		file + `: skipping territory "inverted_z": maxZ must be >= minZ`,
		`maker "fixed_npc": territory "two_nodes" does not exist`,
		`maker "partly_missing": territory "nowhere" does not exist`,
		`maker "partly_missing": territory "nowhere_b" does not exist`,
	}
	if !reflect.DeepEqual(sl.Warnings, wantWarnings) {
		t.Errorf("Warnings = %q, want %q", sl.Warnings, wantWarnings)
	}
}

func TestLoadDirErrors(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name:    "no npc files",
			files:   map[string]string{"spawnlist/a.xml": "<list/>"},
			wantErr: "no npc files",
		},
		{
			name:    "no spawnlist files",
			files:   map[string]string{"npcs/a.xml": "<list/>"},
			wantErr: "no spawnlist files",
		},
		{
			name:    "malformed npc file",
			files:   map[string]string{"npcs/a.xml": "<list><npc>", "spawnlist/a.xml": "<list/>"},
			wantErr: "decode",
		},
		{
			name:    "malformed spawnlist file",
			files:   map[string]string{"npcs/a.xml": "<list/>", "spawnlist/a.xml": "<list><territory"},
			wantErr: "decode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, sub := range []string{"npcs", "spawnlist"} {
				if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for name, body := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestLoadNPCsLastDeclarationWins checks that a duplicated id keeps its last
// declaration, within a file and across sorted files, as the server's
// template table does.
func TestLoadNPCsLastDeclarationWins(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  map[int32]NPC
	}{
		{
			name: "within one file",
			files: map[string]string{
				"a.xml": `<list><npc id="1" name="first" type="Monster"/><npc id="1" name="second" type="Folk"/></list>`,
			},
			want: map[int32]NPC{1: {ID: 1, Name: "second", Type: "Folk", Level: 1, Combat: combatDefaults, CorpseTime: corpseDefault}},
		},
		{
			name: "across files",
			files: map[string]string{
				"b.xml": `<list><npc id="1" name="later" type="Folk"/><npc id="2" name="other" type="Monster"/></list>`,
				"a.xml": `<list><npc id="1" name="earlier" type="Monster"/></list>`,
			},
			want: map[int32]NPC{
				1: {ID: 1, Name: "later", Type: "Folk", Level: 1, Combat: combatDefaults, CorpseTime: corpseDefault},
				2: {ID: 2, Name: "other", Type: "Monster", Level: 1, Combat: combatDefaults, CorpseTime: corpseDefault},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := LoadNPCs(dir)
			if err != nil {
				t.Fatalf("LoadNPCs: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadNPCs = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestLoadShippedDatapack loads a real datapack when BOT_DATAPACK_XML names
// its data/xml directory (the server checkout's aCis_datapack/data/xml), and
// skips otherwise: the datapack is not part of this repository.
func TestLoadShippedDatapack(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	start := time.Now()
	dp, err := Load(dir)
	if err != nil {
		t.Fatalf("Load(%s): %v", dir, err)
	}
	t.Logf("loaded in %v: items=%d etcitems=%d skills=%d", time.Since(start), len(dp.Items), len(dp.EtcItems), len(dp.Skills))
	if len(dp.EtcItems) == 0 || len(dp.Skills) == 0 {
		t.Fatal("shipped datapack loaded no etc items or skills")
	}
	spawns := 0
	for _, m := range dp.Spawnlist.Makers {
		spawns += len(m.Spawns)
	}
	t.Logf("npcs=%d territories=%d makers=%d spawns=%d warnings=%d",
		len(dp.NPCs), len(dp.Spawnlist.Territories), len(dp.Spawnlist.Makers), spawns, len(dp.Spawnlist.Warnings))
	for _, w := range dp.Spawnlist.Warnings {
		t.Logf("warning: %s", w)
	}
	if len(dp.NPCs) == 0 || len(dp.Spawnlist.Territories) == 0 || len(dp.Spawnlist.Makers) == 0 || spawns == 0 {
		t.Fatal("shipped datapack loaded empty")
	}
}
