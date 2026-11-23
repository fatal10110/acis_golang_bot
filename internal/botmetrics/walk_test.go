package botmetrics

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestMetricsWalk sums the walking-efficiency histograms of every bot: idle
// in whole milliseconds, efficiency in per mille, and a nil bot records
// nothing.
func TestMetricsWalk(t *testing.T) {
	tests := []struct {
		name       string
		idle       []time.Duration
		efficiency []int64
		wantIdle   HistogramSnapshot
		wantEff    HistogramSnapshot
	}{
		{name: "none"},
		{
			name:     "idle in milliseconds",
			idle:     []time.Duration{700 * time.Millisecond, 1500*time.Millisecond + 900*time.Microsecond},
			wantIdle: HistogramSnapshot{Count: 2, Sum: 2200, Max: 1500},
		},
		{
			name:       "efficiency in per mille",
			efficiency: []int64{612, 1000},
			wantEff:    HistogramSnapshot{Count: 2, Sum: 1612, Max: 1000},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			a, b := r.NewBot(), r.NewBot()
			var none *Bot
			for i, d := range tt.idle {
				[]*Bot{a, b}[i%2].NavWalkIdle(d)
				none.NavWalkIdle(d)
			}
			for i, p := range tt.efficiency {
				[]*Bot{a, b}[i%2].NavWalkEfficiency(p)
				none.NavWalkEfficiency(p)
			}
			s := r.Collect(t0)
			got := [2]HistogramSnapshot{s.NavWalkIdleMillis, s.NavWalkEfficiency}
			for i, want := range [2]HistogramSnapshot{tt.wantIdle, tt.wantEff} {
				if got[i].Count != want.Count || got[i].Sum != want.Sum || got[i].Max != want.Max {
					t.Errorf("histogram %d = count %d sum %d max %d, want %d %d %d", i, got[i].Count, got[i].Sum, got[i].Max, want.Count, want.Sum, want.Max)
				}
			}
			raw, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{`"nav_walk_idle_ms"`, `"nav_walk_efficiency_permille"`} {
				if !strings.Contains(string(raw), key) {
					t.Errorf("snapshot JSON lacks %s: %s", key, raw)
				}
			}
		})
	}
}
