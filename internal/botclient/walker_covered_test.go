package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// TestWalkerPlanCoveredCost pins the covered cost of the walker's own
// planner, the one its swim and drop searches run on (#698): the cost the
// nav cache was built with, as the hierarchy's legs weigh it, and the
// default without a hierarchy.
func TestWalkerPlanCoveredCost(t *testing.T) {
	for _, tc := range []struct {
		name    string
		covered int // nav.BuildConfig.CoveredCost
		hier    bool
		want    int
	}{
		{"default build", 0, true, nav.DefaultCoveredCost},
		{"no covered weight", -1, true, 0},
		{"heavier build", 30, true, 30},
		{"no hierarchy", 0, false, nav.DefaultCoveredCost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.hier {
				w := newWalker(&Nav{})
				if got := w.plan.CoveredCost(); got != tc.want {
					t.Errorf("walker plan weighs covered ground at %d, want %d", got, tc.want)
				}
				return
			}
			n, err := buildTestNavCovered(geotest.Scenes(), navWindow(0, 0, 0, 0), nil, tc.covered)
			if err != nil {
				t.Fatal(err)
			}
			if got := n.Hier.Graph().CoveredCost(); got != tc.want {
				t.Fatalf("graph built at %d records %d, want %d", tc.covered, got, tc.want)
			}
			b, _, _ := newNavBrain(t, n, Persona{}, Point{X: 3500, Y: 500})
			if got := b.walker().plan.CoveredCost(); got != tc.want {
				t.Errorf("walker plan weighs covered ground at %d, want %d", got, tc.want)
			}
		})
	}
}
