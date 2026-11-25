package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// TestWalkPlanOutcome checks the outcome each plan records: ok with a
// route, else the walk's unreachable reason, and isolated for an /unstuck
// ask that ends no walk (#497).
func TestWalkPlanOutcome(t *testing.T) {
	tests := []struct {
		ok     bool
		status walkStatus
		reason string
		want   botmetrics.PlanOutcome
	}{
		{ok: true, status: walkWalking, want: botmetrics.PlanOK},
		{status: walkWalking, want: botmetrics.PlanIsolated},
		{status: walkUnreachable, reason: reasonBudget, want: botmetrics.PlanBudget},
		{status: walkUnreachable, reason: reasonDoor, want: botmetrics.PlanDoor},
		{status: walkUnreachable, reason: reasonBlocked, want: botmetrics.PlanBlocked},
		{status: walkUnreachable, reason: reasonNoRoute, want: botmetrics.PlanNoRoute},
		{status: walkUnreachable, reason: reasonIsolated, want: botmetrics.PlanIsolated},
		{status: walkUnreachable, reason: reasonNoNav, want: botmetrics.PlanNoNav},
	}
	for _, tt := range tests {
		w := walker{status: tt.status, reason: tt.reason}
		if got := w.planOutcome(tt.ok); got != tt.want {
			t.Errorf("ok %v status %v reason %q: %v, want %v", tt.ok, tt.status, tt.reason, got, tt.want)
		}
	}
}

// TestWalkPlanCounted checks that a walk's plan is counted once, as ok,
// with its duration (aheadWalk plans the route around the U).
func TestWalkPlanCounted(t *testing.T) {
	_, reg, _, at := aheadWalk(t)
	s := reg.Collect(at)
	if s.NavPlans[botmetrics.PlanOK.String()] != 1 || s.NavPlanMicros.Count != 1 {
		t.Fatalf("plans %v timed %d, want one ok plan timed", s.NavPlans, s.NavPlanMicros.Count)
	}
}
