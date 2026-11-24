package botclient

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// gearRow is the gear part of a row of "Behavior per class"
// (docs/strategy.md, columns 2 and 11): the armor sets it prefers, by their
// chest, lowest grade first, and the dye symbols of its direction. The gear
// and dye errands reach them through the model (gearModel); the rows are
// what TestBehaviorRowsShipped holds the datapack and the build to.
type gearRow struct {
	sets, dyes []int32
}

// The sets of the rows: Wooden, Devotion, Brigandine, Manticore Skin,
// Reinforced Leather, Knowledge and Mithril Robe (armorSets.xml).
const (
	setWooden     int32 = 23
	setDevotion   int32 = 1101
	setBrigandine int32 = 352
	setManticore  int32 = 395
	setReinforced int32 = 394
	setKnowledge  int32 = 436
	setMithril    int32 = 437
)

// gearRows are the rows' sets and dyes by class.
var gearRows = map[int32]gearRow{
	0:  {sets: []int32{setWooden}},
	18: {sets: []int32{setWooden}},
	31: {sets: []int32{setWooden}},
	44: {sets: []int32{setWooden}},
	53: {sets: []int32{setWooden}},
	10: {sets: []int32{setDevotion}},
	25: {sets: []int32{setDevotion}},
	38: {sets: []int32{setDevotion}},
	49: {sets: []int32{setDevotion, setWooden}},
	4:  {sets: []int32{setBrigandine}, dyes: []int32{4, 16, 28}},
	1:  {sets: []int32{setBrigandine}, dyes: []int32{1, 13, 25}},
	7:  {sets: []int32{setManticore, setReinforced}, dyes: []int32{6, 18, 30, 1, 13, 25}},
	19: {sets: []int32{setBrigandine}, dyes: []int32{4, 16, 28}},
	22: {sets: []int32{setManticore, setReinforced}, dyes: []int32{2, 14, 26}},
	32: {sets: []int32{setBrigandine}, dyes: []int32{4, 16, 28}},
	35: {sets: []int32{setManticore, setReinforced}, dyes: []int32{2, 14, 26}},
	45: {sets: []int32{setBrigandine}, dyes: []int32{1, 13, 25}},
	47: {sets: []int32{setManticore, setReinforced}, dyes: []int32{1, 13, 25}},
	54: {sets: []int32{setBrigandine}, dyes: []int32{1, 13, 25}},
	15: {sets: []int32{setMithril}, dyes: []int32{12, 24, 36}},
	11: {sets: []int32{setKnowledge}, dyes: []int32{8, 20, 32, 7, 19, 31}},
	29: {sets: []int32{setMithril}, dyes: []int32{12, 24, 36}},
	26: {sets: []int32{setKnowledge}, dyes: []int32{7, 19, 31}},
	42: {sets: []int32{setMithril}, dyes: []int32{10, 22, 34}},
	39: {sets: []int32{setKnowledge}, dyes: []int32{8, 20, 32}},
	50: {sets: []int32{setBrigandine, setKnowledge}, dyes: []int32{10, 22, 34}},
}

// checkGearRow checks class's gear row against the datapack and the
// builds of every role at its row's end level: each set exists, its chest
// an armor kind one of the builds wears (GearClass.Armor or a race grant);
// each dye symbol exists, lists the class and raises one of the kit's dye
// stats; each race grant's mastery is in the class's tree from the grant's
// level.
func checkGearRow(class int32, r gearRow, classes map[int32]datapack.Class, items map[int32]datapack.Item, sets map[int32]datapack.ArmorSet, hennas map[int32]datapack.Henna) []string {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf("class %d: ", class)+fmt.Sprintf(format, args...))
	}
	c, ok := classes[class]
	if !ok {
		return []string{fmt.Sprintf("class %d not in the datapack", class)}
	}
	start, end := int32(1), int32(19)
	if c.Parent >= 0 {
		start, end = 20, 39
	}
	var kinds, stats []string
	for _, role := range []Role{RoleDamage, RoleSupport} {
		bd := CompileBuild(c.Race, class, end, role)
		kinds = append(kinds, bd.Kit.Gear.Armor)
		stats = append(stats, bd.Kit.DyeStats...)
		for _, g := range bd.Race.MysticArmor {
			kinds = append(kinds, g.Kind)
			if lvl, ok := firstMinLevel(c, g.Mastery); !ok || max(start, lvl) != max(start, g.From) {
				fail("mastery %d for %s armor from %d, tree from %d (%v)", g.Mastery, g.Kind, g.From, lvl, ok)
			}
		}
	}
	for _, chest := range r.sets {
		s, ok := sets[chest]
		if !ok {
			fail("no set of chest %d", chest)
			continue
		}
		if kind := items[s.Chest].Kind; !slices.Contains(kinds, kind) {
			fail("set %s is %s armor, the builds wear %v", s.Name, kind, kinds)
		}
	}
	for _, id := range r.dyes {
		h, ok := hennas[id]
		if !ok || !slices.Contains(h.Classes, class) || !slices.ContainsFunc(stats, func(s string) bool { return dyeStat(h, s) > 0 }) {
			fail("symbol %d = %+v, want it listing the class and raising one of %v", id, h, stats)
		}
	}
	return errs
}

// TestGearRowsCheck checks checkGearRow on fixtures: a right row passes; a
// set of another armor kind, a symbol leaving the class out and one
// raising another stat each fail.
func TestGearRowsCheck(t *testing.T) {
	classes := map[int32]datapack.Class{4: {ID: 4, Parent: 0, Race: 0}}
	items := map[int32]datapack.Item{
		setBrigandine: {ID: setBrigandine, Kind: "HEAVY"},
		setMithril:    {ID: setMithril, Kind: "MAGIC"},
	}
	sets := map[int32]datapack.ArmorSet{
		setBrigandine: {Name: "Brigandine", Chest: setBrigandine},
		setMithril:    {Name: "Mithril Robe", Chest: setMithril},
	}
	hennas := map[int32]datapack.Henna{
		4:  {SymbolID: 4, CON: 1, DEX: -3, Classes: []int32{4}},
		10: {SymbolID: 10, MEN: 1, WIT: -3, Classes: []int32{4}},
		16: {SymbolID: 16, CON: 1, DEX: -2, Classes: []int32{1}},
	}
	tests := []struct {
		name string
		row  gearRow
		fail bool
	}{
		{"right", gearRow{sets: []int32{setBrigandine}, dyes: []int32{4}}, false},
		{"robe set", gearRow{sets: []int32{setMithril}}, true},
		{"other class's symbol", gearRow{dyes: []int32{16}}, true},
		{"other stat", gearRow{dyes: []int32{10}}, true},
		{"no such set", gearRow{sets: []int32{99}}, true},
	}
	for _, tt := range tests {
		if errs := checkGearRow(4, tt.row, classes, items, sets, hennas); (len(errs) > 0) != tt.fail {
			t.Errorf("%s: %v, want failing %v", tt.name, errs, tt.fail)
		}
	}
}
