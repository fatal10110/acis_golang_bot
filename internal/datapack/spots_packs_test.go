package datapack

import (
	"encoding/xml"
	"strings"
	"testing"
)

// TestSpotPackFacts checks the pack facts of a spot: the aggressive
// monsters' totals capped at MaximumNPCs, their largest aggro range, clan
// call range and run speed, and the territory's area.
func TestSpotPackFacts(t *testing.T) {
	// A 4000 x 3000 rectangle.
	rect := &Territory{Name: "r", MinZ: -3500, MaxZ: -3000, Nodes: []Point{{0, 0}, {4000, 0}, {4000, 3000}, {0, 3000}}}
	npcs := map[int32]NPC{
		20093: {ID: 20093, Type: "Monster", Level: 10, AggroRange: 1000, RunSpeed: 108.9, ClanRange: 300},
		20342: {ID: 20342, Type: "Monster", Level: 12, AggroRange: 800, RunSpeed: 118.8, ClanRange: 200},
		20001: {ID: 20001, Type: "Monster", Level: 13, RunSpeed: 200},
	}
	tests := []struct {
		name  string
		max   int
		spawn []Spawn
		want  Spot
	}{
		{
			name:  "aggressive totals, largest ranges and speed",
			max:   20,
			spawn: []Spawn{{NPCID: 20093, Total: 4}, {NPCID: 20342, Total: 3}, {NPCID: 20001, Total: 5}},
			want:  Spot{Aggressive: 7, AggroRange: 1000, ClanRange: 300, RunSpeed: 118.8},
		},
		{
			name:  "capped at the maker's maximum",
			max:   5,
			spawn: []Spawn{{NPCID: 20093, Total: 4}, {NPCID: 20342, Total: 3}},
			want:  Spot{Aggressive: 5, AggroRange: 1000, ClanRange: 300, RunSpeed: 118.8},
		},
		{
			name:  "a passive monster counts toward none",
			max:   9,
			spawn: []Spawn{{NPCID: 20001, Total: 9}},
			want:  Spot{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Maker{Name: "m", Territories: []*Territory{rect}, MaximumNPCs: tt.max, Spawns: tt.spawn}
			d := &Datapack{NPCs: npcs, Spawnlist: &Spawnlist{Makers: []*Maker{m}}}
			got := d.Spots()
			if len(got) != 1 {
				t.Fatalf("Spots = %+v, want one", got)
			}
			s := got[0]
			if s.Area != 12e6 {
				t.Errorf("Area = %v, want 12e6", s.Area)
			}
			if s.Aggressive != tt.want.Aggressive || s.AggroRange != tt.want.AggroRange || s.ClanRange != tt.want.ClanRange || s.RunSpeed != tt.want.RunSpeed {
				t.Errorf("pack facts = %d %d %d %v, want %d %d %d %v", s.Aggressive, s.AggroRange, s.ClanRange, s.RunSpeed,
					tt.want.Aggressive, tt.want.AggroRange, tt.want.ClanRange, tt.want.RunSpeed)
			}
		})
	}
}

// TestTerritoryArea checks the shoelace area on either winding.
func TestTerritoryArea(t *testing.T) {
	tests := []struct {
		name  string
		nodes []Point
		want  float64
	}{
		{"square counter-clockwise", []Point{{0, 0}, {1000, 0}, {1000, 1000}, {0, 1000}}, 1e6},
		{"square clockwise", []Point{{0, 1000}, {1000, 1000}, {1000, 0}, {0, 0}}, 1e6},
		{"triangle", []Point{{-100, -100}, {100, -100}, {0, 100}}, 2e4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&Territory{Nodes: tt.nodes}).area(); got != tt.want {
				t.Fatalf("area = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNPCClanAndSpeed checks the run speed and clan call range buildNPC
// reads: clanRange only with a clan, where the server requires it
// (data/xml/npc.go), and a mangled value failing the template.
func TestNPCClanAndSpeed(t *testing.T) {
	tests := []struct {
		name      string
		sets      string
		wantRun   float64
		wantRange int
		wantErr   string
	}{
		{name: "clan and speed", sets: `runSpd=108.9 clan=orc_clan clanRange=300`, wantRun: 108.9, wantRange: 300},
		{name: "no clan: its range is not read", sets: `runSpd=118.8 clanRange=300`, wantRun: 118.8},
		{name: "neither", sets: ``},
		{name: "clan without a range", sets: `clan=orc_clan`, wantErr: "clanRange"},
		{name: "mangled speed", sets: `runSpd=fast`, wantErr: "runSpd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			el := npcElement{Attrs: []xml.Attr{
				{Name: xml.Name{Local: "id"}, Value: "20093"},
				{Name: xml.Name{Local: "name"}, Value: "Orc"},
				{Name: xml.Name{Local: "type"}, Value: "Monster"},
			}}
			for _, kv := range strings.Fields(tt.sets) {
				k, v, _ := strings.Cut(kv, "=")
				el.Sets = append(el.Sets, setElement{Name: k, Val: v})
			}
			npc, err := buildNPC(el)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("buildNPC error = %v, want one naming %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildNPC: %v", err)
			}
			if npc.RunSpeed != tt.wantRun || npc.ClanRange != tt.wantRange {
				t.Fatalf("RunSpeed, ClanRange = %v, %d, want %v, %d", npc.RunSpeed, npc.ClanRange, tt.wantRun, tt.wantRange)
			}
		})
	}
}
