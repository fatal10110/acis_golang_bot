package botclient

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPvPRowsShipped checks the PvP rows (docs/pvp/rows.md "Tests") and the
// skills the PvP layer names (docs/pvp/layer.md) against the shipped
// datapack when BOT_DATAPACK_XML names its data/xml directory, and skips
// otherwise: the fighter rows' steps in the row table (X7), the mystic
// rows (X8, checkMysticPvPRows), every PvP step's skill learned at a
// trainer, and the rows' skill fixture (rowSkills) as the datapack has it.
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
	// The row table: each row's damage, DOT and control steps, "skill" or
	// "skill/vs", in the order the row plays them.
	second := secondRowIDs()
	row := func(id string) charmodel.Row { return charmodel.Row{ID: id} }
	bow := func(id string) charmodel.Row { return charmodel.Row{ID: id, Bow: true} }
	for _, tc := range []struct {
		level int
		want  map[charmodel.Row]string
	}{
		{level: 1, want: map[charmodel.Row]string{}},
		{level: 20, want: map[charmodel.Row]string{
			row("HF"): "56 16 3", row("EF"): "56 16 3", row("DF"): "70 70 56 16 3", row("OF"): "29 3",
			row("KN"): "70 70/CA 92 3", row("WA"): "100 255 245",
			row("RO"): "16", bow("RO"): "56", row("EK"): "3", row("ES"): "16", bow("ES"): "56",
			row("PK"): "129/CH 70 70 3", row("AS"): "129/CH 70 70 16", bow("AS"): "70 56",
			row("OR"): "100 255", row("MO"): "95/CA 120 29", row("SC"): "100",
		}},
		{level: 40, want: map[charmodel.Row]string{
			row("HF"): "56 16 3", row("EF"): "56 16 3", row("DF"): "70 70 56 16 3", row("OF"): "29 3",
			row("KN"): "70 70/CA 92 3", row("WA"): "100 255 245",
			row("RO"): "96 16", bow("RO"): "101 56", row("EK"): "102/CA 3", row("ES"): "102/CA 96 16", bow("ES"): "102/M 101 56",
			row("PK"): "129/CH 115/MA 105/M 70 70 223 3", row("AS"): "129/CH 115/M 70 70 96 16 223", bow("AS"): "101 70 56",
			row("OR"): "100 255", row("MO"): "54/CA 95/CA 120 284 54 29", row("SC"): "100",
		}},
	} {
		got := map[charmodel.Row]string{}
		for r, refs := range pvpRowTable(m, shippedKits, tc.level) {
			if _, mystic := mysticRowTable[40][r.ID]; mystic || second[r.ID] {
				continue // checkMysticPvPRows, checkSecondPvPRows
			}
			got[r] = refWords(t, skills, refs)
		}
		if !maps.Equal(got, tc.want) {
			t.Errorf("level %d: row table\n%v\nwant\n%v", tc.level, got, tc.want)
		}
	}
	// Every PvP step's skill is one the row learns at a trainer.
	for _, kits := range shippedKits {
		for class, kit := range kits {
			for _, st := range kit.PvP {
				if !slices.Contains(kit.Skills, st.Skill) && !slices.Contains(kit.Base, st.Skill) {
					t.Errorf("class %d: PvP step skill %d is not learned (Skills %v)", class, st.Skill, kit.Skills)
				}
			}
		}
	}
	for ref, want := range rowSkills {
		got, ok := skills[ref]
		if !ok {
			t.Errorf("skill %d level %d not in the datapack", ref.ID, ref.Level)
			continue
		}
		if !reflect.DeepEqual(rowView(got), want) {
			t.Errorf("skill %d level %d: fixture\n%+v\ndatapack\n%+v", ref.ID, ref.Level, want, rowView(got))
		}
	}
	t.Run("mystic rows", func(t *testing.T) { checkMysticPvPRows(t, classes, skills) })
	t.Run("second rows", func(t *testing.T) { checkSecondPvPRows(t, classes, skills) })
	t.Run("heavy rows", func(t *testing.T) { checkHeavyPvPRows(t, classes, skills) })
	t.Run("dagger rows at 66", func(t *testing.T) { checkDaggerPvPRows(t, classes, skills) })
	t.Run("archer rows at 46 and 60", func(t *testing.T) { checkArcherPvPRows(t, classes, skills) })
	t.Run("tyrant row at 58", func(t *testing.T) { checkTyrantPvPRow(t, classes, skills) })
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

// skillOf is the id of skill s, a level entry of skills.
func skillOf(t *testing.T, skills map[datapack.SkillRef]datapack.Skill, s datapack.Skill) int32 {
	t.Helper()
	for ref, sk := range skills {
		if sk.Name == s.Name && reflect.DeepEqual(sk, s) {
			return ref.ID
		}
	}
	t.Fatalf("skill %q not in the datapack", s.Name)
	return 0
}
