package botmetrics

import "testing"

// TestMetricsPanics: Panic counts per bot, sums in the snapshot apart from
// the aborts, and records nothing on a nil Bot.
func TestMetricsPanics(t *testing.T) {
	r := NewRegistry()
	a, b := r.NewBot(), r.NewBot()
	a.Panic()
	b.Panic()
	b.Panic()
	var none *Bot
	none.Panic()
	if s := r.Collect(t0); s.Panics != 3 || s.Aborts != 0 {
		t.Fatalf("panics %d, aborts %d; want 3, 0", s.Panics, s.Aborts)
	}
}
