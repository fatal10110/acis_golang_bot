package botmetrics

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestMetricsNavPlan sums the walker's route plans of every bot by outcome,
// every outcome present, and their durations in whole microseconds; a nil
// bot and an outcome out of range record nothing.
func TestMetricsNavPlan(t *testing.T) {
	type plan struct {
		o PlanOutcome
		d time.Duration
	}
	tests := []struct {
		name     string
		plans    []plan
		want     map[string]uint64
		wantTime HistogramSnapshot
	}{
		{name: "none", want: map[string]uint64{}},
		{
			name: "by outcome",
			plans: []plan{
				{PlanOK, 1500 * time.Microsecond},
				{PlanOK, 0},
				{PlanBudget, 80*time.Millisecond + 900*time.Nanosecond},
				{PlanDoor, time.Microsecond},
				{PlanBlocked, 0},
				{PlanNoRoute, 2 * time.Microsecond},
				{PlanIsolated, 0},
				{PlanNoNav, 0},
			},
			want:     map[string]uint64{"ok": 2, "budget": 1, "door": 1, "blocked": 1, "no_route": 1, "isolated": 1, "no_nav": 1},
			wantTime: HistogramSnapshot{Count: 8, Sum: 81503, Max: 80000},
		},
		{
			name:  "out of range",
			plans: []plan{{planOutcomes, time.Second}, {PlanNoRoute, 3 * time.Microsecond}},
			want:  map[string]uint64{"no_route": 1}, wantTime: HistogramSnapshot{Count: 1, Sum: 3, Max: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			a, b := r.NewBot(), r.NewBot()
			var none *Bot
			for i, p := range tt.plans {
				[]*Bot{a, b}[i%2].NavPlan(p.o, p.d)
				none.NavPlan(p.o, p.d)
			}
			s := r.Collect(t0)
			if len(s.NavPlans) != int(planOutcomes) {
				t.Errorf("outcomes %v, want all %d", s.NavPlans, planOutcomes)
			}
			for _, name := range planNames {
				if s.NavPlans[name] != tt.want[name] {
					t.Errorf("%s plans = %d, want %d", name, s.NavPlans[name], tt.want[name])
				}
			}
			if got := s.NavPlanMicros; got.Count != tt.wantTime.Count || got.Sum != tt.wantTime.Sum || got.Max != tt.wantTime.Max {
				t.Errorf("plan time = count %d sum %d max %d, want %d %d %d", got.Count, got.Sum, got.Max, tt.wantTime.Count, tt.wantTime.Sum, tt.wantTime.Max)
			}
			raw, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{`"nav_plans_total":{`, `"no_route":`, `"nav_plan_us"`} {
				if !strings.Contains(string(raw), key) {
					t.Errorf("snapshot JSON lacks %s: %s", key, raw)
				}
			}
		})
	}
	if got := len(NewRegistry().Last().NavPlans); got != int(planOutcomes) {
		t.Errorf("empty snapshot has %d outcomes, want %d", got, planOutcomes)
	}
	if got := PlanOutcome(200).String(); got != "unknown" {
		t.Errorf("String of an outcome out of range = %q", got)
	}
}
