package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// rowSpotMargins are the spot margins of the rows (docs/strategy.md
// column 10) by class: the base rows, first professions and second
// professions with a row of their own, the artisan playing the dwarven
// fighter's.
var rowSpotMargins = map[int32]float64{
	HumanFighter.ID: marginNormal, ElvenFighter.ID: marginNormal, DarkFighter.ID: marginGlassCannon,
	OrcFighter.ID: marginTank, DwarvenFighter.ID: marginNormal,
	4: marginTank, 1: marginNormal, 7: marginNormal, 19: marginNormal, 22: marginNormal,
	32: marginNormal, 35: marginGlassCannon, 45: marginTank, 47: marginNormal, 54: marginTank, 56: marginNormal,
	10: marginGlassCannon, 25: marginGlassCannon, 38: marginGlassCannon, 49: marginNormal,
	15: marginGlassCannon, 11: marginGlassCannon, 29: marginGlassCannon, 26: marginGlassCannon,
	42: marginGlassCannon, 39: marginGlassCannon, 50: marginNormal,
	21: marginNormal, 34: marginNormal,
}

// TestSpotMarginRows checks every role of every class of rowSpotMargins
// compiles its row's spot margin at its row's start (1, 20 or 40), and a
// persona's own replaces it.
func TestSpotMarginRows(t *testing.T) {
	for _, class := range slices.Sorted(maps.Keys(rowSpotMargins)) {
		race, _ := datapack.ClassRace(class)
		start, _, ok := rowSpan(class)
		if !ok {
			t.Errorf("class %d has a spot margin and no row", class)
			continue
		}
		for _, role := range []Role{RoleDamage, RoleSupport, RoleCrafter} {
			if got, want := CompileBuild(race, class, start, role, BranchChoice{}).Kit.SpotMargin, rowSpotMargins[class]; got != want {
				t.Errorf("class %d %v: SpotMargin %v, want %v", class, role, got, want)
			}
		}
	}
	p := DefaultPersona()
	p.Spots.Margin = 2.5
	if got := p.override(CompileBuild(0, OrcFighter.ID, 20, RoleDamage, BranchChoice{}), OrcFighter.ID).Kit.SpotMargin; got != 2.5 {
		t.Errorf("persona Margin 2.5 compiles %v", got)
	}
}

// TestSpotMarginTankGlass checks a tank row takes a spot a glass cannon row
// of the same level and stats passes over (D24): to the level 6 squire's
// stats the toads last it 2.27 kills, within the orc fighter's 1.5 and
// short of the dark fighter's 3.
func TestSpotMarginTankGlass(t *testing.T) {
	for _, tt := range []struct {
		name  string
		class int32
		fit   bool
	}{
		{"tank: orc fighter", OrcFighter.ID, true},
		{"normal: human fighter", HumanFighter.ID, true},
		{"glass cannon: dark fighter", DarkFighter.ID, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := tiToads27
			s.Maker, s.MaximumNPCs = "toads", 5
			b := newSpotBrain(t, 6, s)
			b.world.Self.ClassID = tt.class
			setSquire(b, squire6, 0)
			b.decide(at(time.Second))
			if b.spot.held != tt.fit {
				t.Fatalf("holds the toads %v, want %v", b.spot.held, tt.fit)
			}
		})
	}
}
