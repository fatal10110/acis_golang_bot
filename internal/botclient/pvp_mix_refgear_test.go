package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPvPMixRefGear checks the PvP value mix's reference opponent wears
// the engage estimate's grade-typical gear (#600): at level 20 a P. Def
// piece (a heavy chest) gains more PvP value against the geared opponent
// than against the bare class template the mix used before, and below
// level 20 (no grade) the opponent is the bare template either way.
func TestPvPMixRefGear(t *testing.T) {
	classes := map[int32]datapack.Class{0: findFighter}
	geared := refTravel()
	geared.classes = classes
	bare := &TravelMap{classes: classes}
	rows := map[mixRow]int{{0, charmodel.Row{}}: 1}
	var base []datapack.Item
	for _, id := range []int32{refSword2, refShield3, refNeck2, refEar, refEar, refRing1, refRing1} {
		base = append(base, refItems[id])
	}
	withChest := append(slices.Clone(base), refItems[refHeavy])
	// pdefGain is the PvP value the heavy chest adds to the bot over m's
	// one-row mix at level.
	pdefGain := func(m *TravelMap, level int) (gain, oppPDef float64) {
		mix := mixShares(m, rows, level)
		if len(mix) != 1 {
			t.Fatalf("mix of %d rows, want 1", len(mix))
		}
		value := func(worn []datapack.Item) float64 {
			bot := charmodel.ModelFighter(charmodel.Build{Class: findFighter, Level: level, Worn: worn})
			return charmodel.PvPValue(bot, mix, charmodel.RowTable{})
		}
		return value(withChest) - value(base), mix[0].Opponent.Profile.PDef
	}
	cases := []struct {
		name   string
		level  int
		higher bool
	}{
		{"level 20: the geared opponent weighs the P. Def piece more", 20, true},
		{"level 19: no grade, the bare template either way", 19, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gGain, gPDef := pdefGain(geared, tc.level)
			bGain, bPDef := pdefGain(bare, tc.level)
			if bGain <= 0 {
				t.Fatalf("P. Def piece gain against the bare template %v, want above 0", bGain)
			}
			if tc.higher {
				if gPDef <= bPDef {
					t.Errorf("geared opponent P. Def %v, want above the bare template's %v", gPDef, bPDef)
				}
				if gGain <= bGain {
					t.Errorf("P. Def piece gain %v against the geared opponent, want above %v against the bare template", gGain, bGain)
				}
				return
			}
			if gPDef != bPDef || gGain != bGain {
				t.Errorf("geared map: opponent P. Def %v, gain %v; want the bare template's %v, %v", gPDef, gGain, bPDef, bGain)
			}
		})
	}
	t.Run("each row's opponent is the reference of its class and bow flag", func(t *testing.T) {
		m := refTravel()
		m.classes = map[int32]datapack.Class{7: findFighter} // Rogue
		dagger, bow := charmodel.Row{ID: "dagger"}, charmodel.Row{ID: "bow", Bow: true}
		mix := mixShares(m, map[mixRow]int{{7, dagger}: 3, {7, bow}: 1}, 20)
		if len(mix) != 2 {
			t.Fatalf("mix of %d rows, want 2", len(mix))
		}
		for i, row := range []charmodel.Row{bow, dagger} { // ordered by row id
			want, _ := m.refFighter(7, row.Bow, 20)
			if got := mix[i].Opponent; got.Row != row || got.Profile != want.Profile {
				t.Errorf("row %d opponent row %+v, profile %+v; want %+v, %+v", i, got.Row, got.Profile, row, want.Profile)
			}
		}
		if mix[0].Opponent.Profile.PAtk == mix[1].Opponent.Profile.PAtk {
			t.Errorf("dagger and bow rows share P. Atk %v, want the bow reference's apart", mix[0].Opponent.Profile.PAtk)
		}
	})
}
