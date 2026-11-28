package nav

import "testing"

// TestHierarchyPlannerCovered pins Hierarchy.Planner: the planner it returns
// weighs covered ground as the nav cache was built, whatever base weighed,
// and keeps base's other fields.
func TestHierarchyPlannerCovered(t *testing.T) {
	e := hallEngine(t)
	for _, tc := range []struct {
		cfg, want int
	}{
		{0, DefaultCoveredCost},
		{-1, 0},
		{30, 30},
	} {
		h := hallHier(t, e, tc.cfg)
		base := Planner{Engine: e, Budget: 1234}.withCovered(7)
		p := h.Planner(base)
		if got := p.CoveredCost(); got != tc.want {
			t.Errorf("CoveredCost %d: planner weighs %d, want %d", tc.cfg, got, tc.want)
		}
		if p.Budget != base.Budget || p.Engine != base.Engine {
			t.Errorf("CoveredCost %d: planner lost base's fields: %+v", tc.cfg, p)
		}
	}
	if got := (&Planner{}).CoveredCost(); got != DefaultCoveredCost {
		t.Errorf("zero planner weighs %d, want %d", got, DefaultCoveredCost)
	}
}
