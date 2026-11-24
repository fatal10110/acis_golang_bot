package charmodel

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestMonster checks the NPC finalize step against values worked out from
// the server's functions (npc/hostile_stats.go defaultBuiltin) on three
// templates of npcs/20000-20999.xml, all at the base stats 40 21 30 20 43
// 10: CON 43 1.58, STR 40 1.2, DEX 30 1.1, INT 21 0.81, WIT 20 1.0,
// MEN 10 1.11.
func TestMonster(t *testing.T) {
	tests := []struct {
		name string
		npc  datapack.NPC
		want Profile
	}{
		{
			// hp 39*1.58 = 61.62; pAtk 7.56*1.2*0.9 = 8.16; mAtk
			// 6.48*0.81*0.81*0.81 = 3.44; crit 8*1.1*10 = 88; accuracy
			// sqrt(30)*6+1 = 33.86.
			name: "Gremlin 20001",
			npc:  npc(1, 49.5, datapack.Combat{HP: 39, PAtk: 7.56, PDef: 39, MAtk: 6.48, MDef: 29.44, Crit: 8, AtkSpd: 247.42}),
			want: Profile{MaxHP: 61, PAtk: 8, PDef: 35, MAtk: 3, MDef: 29, PAtkSpd: 272, MAtkSpd: 333, Critical: 88, Accuracy: 33, Evasion: 33, RunSpeed: 54},
		},
		{
			name: "Ol Mahum Patrolman 20065",
			npc:  npc(25, 108.9, datapack.Combat{HP: 461, PAtk: 61.6, PDef: 120.65, MAtk: 51.06, MDef: 81.78, Crit: 4, AtkSpd: 247.42}),
			want: Profile{MaxHP: 728, PAtk: 84, PDef: 137, MAtk: 43, MDef: 103, PAtkSpd: 272, MAtkSpd: 333, Critical: 44, Accuracy: 57, Evasion: 57, RunSpeed: 119},
		},
		{
			name: "Tarlk Basilisk 20573",
			npc:  npc(50, 171.9, datapack.Combat{HP: 1421, PAtk: 277.8, PDef: 217.26, MAtk: 208.56, MDef: 146.9, Crit: 4, AtkSpd: 247.42}),
			want: Profile{MaxHP: 2245, PAtk: 463, PDef: 301, MAtk: 264, MDef: 226, PAtkSpd: 272, MAtkSpd: 333, Critical: 44, Accuracy: 82, Evasion: 82, RunSpeed: 189},
		},
	}
	for _, tt := range tests {
		if got := Monster(tt.npc); got != tt.want {
			t.Errorf("%s: Monster = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// npc is a monster template at level with the shipped monsters' base stats.
func npc(level int, runSpd float64, c datapack.Combat) datapack.NPC {
	c.STR, c.INT, c.DEX, c.WIT, c.CON, c.MEN = 40, 21, 30, 20, 43, 10
	return datapack.NPC{Type: "Monster", Level: level, RunSpeed: runSpd, Combat: c}
}
