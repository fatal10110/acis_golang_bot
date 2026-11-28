package datapack

import (
	"os"
	"reflect"
	"testing"
)

func TestSpots(t *testing.T) {
	square := &Territory{Name: "sq", MinZ: -3500, MaxZ: -3000, Nodes: []Point{{0, 0}, {1000, 0}, {1000, 1000}, {0, 1000}}}
	npcs := map[int32]NPC{
		20001: {ID: 20001, Type: "Monster", Level: 3},
		20002: {ID: 20002, Type: "Monster", Level: 6, AggroRange: 300},
		20003: {ID: 20003, Type: "Monster", Level: 9},
		25001: {ID: 25001, Type: "RaidBoss", Level: 23},
		29001: {ID: 29001, Type: "GrandBoss", Level: 40},
		27001: {ID: 27001, Type: "Monster", Level: 4},
		30001: {ID: 30001, Type: "Folk", Level: 1},
		18001: {ID: 18001, Type: "Chest", Level: 5},
	}
	tests := []struct {
		name  string
		maker Maker
		want  Spot
		ok    bool
	}{
		{
			name:  "monsters",
			maker: Maker{Name: "m", Territories: []*Territory{square}, MaximumNPCs: 9, Spawns: []Spawn{{NPCID: 20001}, {NPCID: 20002}}},
			want:  Spot{Maker: "m", Territory: square, Center: Point{500, 500}, Z: -3250, MinLevel: 3, MaxLevel: 6, AggroLevel: 6, MaximumNPCs: 9, Area: 1e6, AggroRange: 300},
			ok:    true,
		},
		{name: "event", maker: Maker{Name: "m", Territories: []*Territory{square}, Event: "christmas", Spawns: []Spawn{{NPCID: 20001}}}},
		{
			name:  "seven signs dungeon event, tagged",
			maker: Maker{Name: "m", Territories: []*Territory{square}, Event: "ssq_event", MaximumNPCs: 2, Spawns: []Spawn{{NPCID: 20001}}},
			want:  Spot{Maker: "m", Event: "ssq_event", Territory: square, Center: Point{500, 500}, Z: -3250, MinLevel: 3, MaxLevel: 3, MaximumNPCs: 2, Area: 1e6},
			ok:    true,
		},
		{
			name:  "seven signs seal event, tagged",
			maker: Maker{Name: "m", Territories: []*Territory{square}, Event: "ssq_seal2_twilight", MaximumNPCs: 2, Spawns: []Spawn{{NPCID: 20001}}},
			want:  Spot{Maker: "m", Event: "ssq_seal2_twilight", Territory: square, Center: Point{500, 500}, Z: -3250, MinLevel: 3, MaxLevel: 3, MaximumNPCs: 2, Area: 1e6},
			ok:    true,
		},
		{name: "ban", maker: Maker{Name: "m", Territories: []*Territory{square}, Banned: []*Territory{square}, Spawns: []Spawn{{NPCID: 20001}}}},
		{name: "no territory", maker: Maker{Name: "m", Spawns: []Spawn{{NPCID: 20001}}}},
		{name: "raid boss", maker: Maker{Name: "m", Territories: []*Territory{square}, Spawns: []Spawn{{NPCID: 20001}, {NPCID: 25001}}}},
		{name: "grand boss", maker: Maker{Name: "m", Territories: []*Territory{square}, Spawns: []Spawn{{NPCID: 29001}, {NPCID: 20001}}}},
		{name: "non-monsters only", maker: Maker{Name: "m", Territories: []*Territory{square}, Spawns: []Spawn{{NPCID: 30001}, {NPCID: 18001}}}},
		{name: "unknown template", maker: Maker{Name: "m", Territories: []*Territory{square}, Spawns: []Spawn{{NPCID: 12345}}}},
		{name: "quest block", maker: Maker{Name: "m", Territories: []*Territory{square}, Spawns: []Spawn{{NPCID: 27001}}}},
		{name: "quest maker", maker: Maker{Name: "gludio32_qm1725_00m1", Territories: []*Territory{square}, Spawns: []Spawn{{NPCID: 20001}}}},
		{name: "never respawns", maker: Maker{Name: "m", Territories: []*Territory{square}, Spawns: []Spawn{{NPCID: 20001, Respawn: NoRespawn}}}},
		{
			name: "non-monsters and quest mobs leave the levels alone",
			maker: Maker{Name: "m", Territories: []*Territory{square}, MaximumNPCs: 3, Spawns: []Spawn{
				{NPCID: 30001}, {NPCID: 27001}, {NPCID: 20002, Respawn: NoRespawn}, {NPCID: 20003},
			}},
			want: Spot{Maker: "m", Territory: square, Center: Point{500, 500}, Z: -3250, MinLevel: 9, MaxLevel: 9, MaximumNPCs: 3, Area: 1e6},
			ok:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Datapack{NPCs: npcs, Spawnlist: &Spawnlist{Makers: []*Maker{&tt.maker}}}
			got := d.Spots()
			if !tt.ok {
				if len(got) != 0 {
					t.Fatalf("Spots = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || !reflect.DeepEqual(got[0], tt.want) {
				t.Fatalf("Spots = %+v, want [%+v]", got, tt.want)
			}
		})
	}
}

func TestTerritoryInner(t *testing.T) {
	tests := []struct {
		name  string
		nodes []Point
		want  Point
	}{
		{"square: the mean", []Point{{0, 0}, {1000, 0}, {1000, 1000}, {0, 1000}}, Point{500, 500}},
		{"triangle: the mean", []Point{{-100, -100}, {100, -100}, {0, 100}}, Point{0, -33}},
		// A U whose mean (500, 550) falls in the gap between its arms: the
		// nearest grid cell inside, on the right arm.
		{"concave: nearest inside", []Point{{0, 0}, {1000, 0}, {1000, 1000}, {800, 1000}, {800, 200}, {200, 200}, {200, 1000}, {0, 1000}}, Point{843, 531}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Territory{Nodes: tt.nodes}
			got := tr.inner()
			if got != tt.want {
				t.Fatalf("inner = %+v, want %+v", got, tt.want)
			}
			if !tr.Contains(got) {
				t.Fatalf("inner %+v is outside the polygon", got)
			}
		})
	}
}

func TestTerritoryContains(t *testing.T) {
	tri := &Territory{Nodes: []Point{{-100, -100}, {100, -100}, {0, 100}}}
	tests := []struct {
		p    Point
		want bool
	}{
		{Point{0, 0}, true},
		{Point{0, 99}, true},
		{Point{90, 90}, false},
		{Point{-200, 0}, false},
		{Point{0, -101}, false},
	}
	for _, tt := range tests {
		if got := tri.Contains(tt.p); got != tt.want {
			t.Errorf("Contains(%+v) = %v, want %v", tt.p, got, tt.want)
		}
	}
}

// TestShippedSpots checks the spots of a real datapack when
// BOT_DATAPACK_XML names its data/xml directory, and skips otherwise: every
// centre lies in its territory, and near each race's start point
// (classes/*.xml <spawns>) lies a spot a level 3 bot can farm: no monster
// above level 3 (the shipped templates give nearly every monster an aggro
// range, so the aggressive rule bounds them all).
func TestShippedSpots(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := Load(dir)
	if err != nil {
		t.Fatalf("Load(%s): %v", dir, err)
	}
	spots := dp.Spots()
	for _, s := range spots {
		if !s.Territory.Contains(s.Center) {
			t.Errorf("spot %s: centre %+v outside its territory", s.Maker, s.Center)
		}
	}
	t.Logf("makers=%d spots=%d", len(dp.Spawnlist.Makers), len(spots))
	starts := []struct {
		race string
		at   Point
	}{
		{"human fighter", Point{-71338, 258271}},
		{"human mystic", Point{-90875, 248162}},
		{"elf", Point{46045, 41251}},
		{"dark elf", Point{28295, 11063}},
		{"orc", Point{-56733, -113459}},
		{"dwarf", Point{108644, -173947}},
	}
	// near is a newbie zone's reach, well inside SpotPolicy.MaxTravel.
	const near = 10000
	for _, st := range starts {
		best, bestD := "", int64(-1)
		for _, s := range spots {
			if s.MaxLevel > 3 || s.AggroLevel > 3 {
				continue
			}
			dx, dy := int64(s.Center.X-st.at.X), int64(s.Center.Y-st.at.Y)
			if d := dx*dx + dy*dy; bestD < 0 || d < bestD {
				best, bestD = s.Maker, d
			}
		}
		t.Logf("%s: nearest level 3 spot %s, squared distance %d", st.race, best, bestD)
		if bestD < 0 || bestD > near*near {
			t.Errorf("%s: no level 3 spot within %d of %+v", st.race, near, st.at)
		}
	}
}
