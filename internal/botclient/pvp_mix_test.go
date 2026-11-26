package botclient

import (
	"maps"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
)

// TestPvPMix checks the mix table (docs/pvp/model.md "Row table", "Tests"):
// a player seen 59 minutes ago counts, one seen 61 minutes ago does not, a
// bot-marked player never; nobody seen shares the rows of the bot's tier
// equally; a persona that gives PvP value no weight keeps no table.
func TestPvPMix(t *testing.T) {
	row := func(id int32) mixRow {
		r, _ := charmodel.ClassRow(id, false)
		return mixRow{id, r}
	}
	tier := func(t int) map[mixRow]int {
		out := make(map[mixRow]int)
		for _, id := range charmodel.RowClasses(t) {
			out[row(id)] = 1
		}
		return out
	}
	tests := []struct {
		name   string
		weight float64
		class  int32 // the bot's
		after  time.Duration
		want   map[mixRow]int
	}{
		{name: "Warrior seen 59 min ago, two Mystics 50: all count", weight: 0.5, after: 59 * time.Minute, want: map[mixRow]int{row(1): 1, row(10): 2}},
		{name: "Warrior seen 61 min ago: only the Mystics", weight: 0.5, after: 61 * time.Minute, want: map[mixRow]int{row(10): 2}},
		{name: "all seen over an hour ago, a base class: its tier's rows", weight: 0.5, after: 71 * time.Minute, want: tier(0)},
		{name: "all seen over an hour ago, a Knight: the first professions' rows", weight: 0.5, class: 4, after: 71 * time.Minute, want: tier(1)},
		{name: "GearWeight 0: no table", weight: 0, after: time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance = 0
			p.PvP.GearWeight = tc.weight
			b, _ := newTestBrain(t, p)
			b.world.Self.ClassID = tc.class
			b.handleOne(t, CharInfo{ObjectID: 0x1001, Name: "Wolf", ClassID: 1, Pos: Point{Y: 900}}, t0)
			b.handleOne(t, CharInfo{ObjectID: 0x1002, Name: "Owl", ClassID: 10, Pos: Point{Y: 900}}, t0.Add(9*time.Minute))
			b.handleOne(t, CharInfo{ObjectID: 0x1003, Name: "Lark", ClassID: 10, Pos: Point{Y: 900}}, t0.Add(9*time.Minute))
			b.handleOne(t, CharInfo{ObjectID: 0x1004, Name: "OwlBot", ClassID: 15, Pos: Point{Y: 900}}, t0.Add(9*time.Minute))
			// Any event ages the table.
			b.handleOne(t, DeleteObject{ObjectID: 0x7fff}, t0.Add(tc.after))
			if tc.weight == 0 {
				if len(b.pvp.mix.seen) != 0 {
					t.Fatalf("mix table %v, want none", b.pvp.mix.seen)
				}
				return
			}
			if got := b.mixRows(); !maps.Equal(got, tc.want) {
				t.Errorf("mixRows = %v, want %v", got, tc.want)
			}
		})
	}
}
