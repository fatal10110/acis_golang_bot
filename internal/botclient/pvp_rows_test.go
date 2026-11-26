package botclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPvPRowsShipped checks the PvP rows (docs/pvp/rows.md "Tests") and the
// skills the PvP layer names (docs/pvp/layer.md) against the shipped
// datapack when BOT_DATAPACK_XML names its data/xml directory, and skips
// otherwise. X6 ships no row: the row table is empty at every level; X7
// and X8 add their rows' cases.
func TestPvPRowsShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	classes, err := datapack.LoadClasses(filepath.Join(dir, "classes"))
	if err != nil {
		t.Fatal(err)
	}
	skills, err := datapack.LoadSkills(filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	m := &TravelMap{classes: classes, skills: skills}
	for _, level := range []int{1, 20, 40, 80} {
		if rows := pvpRowTable(m, shippedKits, level); len(rows) != 0 {
			t.Errorf("level %d: row table %v, want none before X7 and X8", level, rows)
		}
	}
	skill := func(id int32) datapack.Skill {
		s, ok := skills[datapack.SkillRef{ID: id, Level: 1}]
		if !ok {
			t.Fatalf("skill %d level 1 not in the datapack", id)
		}
		return s
	}
	// The long-cast rule's heals: Self Heal, Elemental Heal, Divine Heal,
	// Heal, Chant of Life.
	for _, id := range []int32{1216, 58, 45, 1011, 1229} {
		if s := skill(id); !healTypes[s.Type] || s.HitTime < longCast {
			t.Errorf("skill %d (%s %v) is no long heal", id, s.Type, s.HitTime)
		}
	}
	if s := skill(1011); !worthyHeals[s.Type] {
		t.Errorf("Heal (%s) is not worth an interrupt", s.Type)
	}
	// Stuns and sleeps abort a cast: Shield Stun, Stun Attack, Sleep,
	// Dreaming Spirit.
	for _, id := range []int32{92, 100, 1069, 1097} {
		if s := skill(id); !aborts(s) || !isControl(s) {
			t.Errorf("skill %d (%s) does not abort a cast as control", id, s.Type)
		}
	}
	tests := []struct {
		id         int32
		lasts      time.Duration
		bits       int32
		dot, kites bool
	}{
		{id: 1092, lasts: 20 * time.Second, bits: 0x20},                      // Fear: 10 ticks of 2 s
		{id: 1095, lasts: 30 * time.Second, dot: true},                       // Venom: 3 s × 10
		{id: 1100, lasts: 15 * time.Second, dot: true},                       // Chill Flame: 15 ticks
		{id: 1107, lasts: 15 * time.Second, dot: true},                       // Frost Flame: 15 ticks
		{id: 1201, lasts: 30 * time.Second, bits: rootAbnormal, kites: true}, // Dryad Root
		{id: 1208, lasts: 30 * time.Second, bits: rootAbnormal, kites: true}, // Seal of Binding
		{id: 1184, lasts: 120 * time.Second, kites: true},                    // Ice Bolt
		{id: 1160, lasts: 60 * time.Second, kites: true},                     // Slow
		{id: 102, lasts: 60 * time.Second, kites: true},                      // Entangle
	}
	for _, tc := range tests {
		lasts, bits, dot, kites := spanOf(skill(tc.id))
		if lasts != tc.lasts || bits != tc.bits || dot != tc.dot || kites != tc.kites {
			t.Errorf("skill %d: lasts %v bits %#x dot %v kites %v, want %v %#x %v %v", tc.id, lasts, bits, dot, kites, tc.lasts, tc.bits, tc.dot, tc.kites)
		}
	}
}
