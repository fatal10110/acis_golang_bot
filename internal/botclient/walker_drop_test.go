package botclient

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// TestWalkerDropPrecheck searches for the drop only when a cell of the box
// around the bot is in an abstract SCC its member reaches. A planner with
// no budget tells the two apart: its search answers ErrBudget at once, so
// ErrUnreachable is the precheck's answer, given before any cell is
// expanded.
func TestWalkerDropPrecheck(t *testing.T) {
	n := testNav(t)
	for _, tc := range []struct {
		name       string
		at, member geotest.Point
		want       error
	}{
		{"a reached SCC in the box searches", geotest.BridgeWestDeck, geotest.BridgeWestDeck, nav.ErrBudget},
		{"no reached SCC in the box answers without a search", geotest.PitOutside, geotest.PitInside, nav.ErrUnreachable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from := scenePoint(tc.at, 0, 0)
			b, _, _ := newNavBrain(t, n, Persona{}, from)
			c, ok := b.walk.plan.Locate(toLoc(scenePoint(tc.member, 0, 0)))
			if !ok {
				t.Fatal("member point off the geodata")
			}
			cur, err := n.Hier.Locate(c, nil)
			if err != nil {
				t.Fatal(err)
			}
			b.walk.plan.Budget = 0
			if _, err := b.walk.drop(toLoc(from), cur); !errors.Is(err, tc.want) {
				t.Fatalf("drop: %v, want %v", err, tc.want)
			}
		})
	}
}
