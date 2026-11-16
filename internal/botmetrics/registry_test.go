package botmetrics

import (
	"context"
	"maps"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestMetricsCollect(t *testing.T) {
	r := NewRegistry("dead", "idle", "idle", "")
	if s := r.Last(); s.Bots != 0 || s.Sent != 0 {
		t.Fatalf("Last before Collect = %+v, want empty", s)
	}
	a, b := r.NewBot(), r.NewBot()
	a.Drained(2)
	a.Drained(5)
	a.Sent()
	a.Sent()
	a.Paced()
	a.Rule("idle")
	a.Rule("")
	b.Rule("idle")
	b.Rule("dead")
	b.Rule("store")
	b.Reconnect()
	b.Abort()
	b.WakeLag(1500 * time.Microsecond)
	r.SetBreaker(true)

	s := r.Collect(t0)
	if r.Last() != s {
		t.Error("Last is not the latest collection")
	}
	type counters struct {
		bots                                    int
		events, sent, paced, reconnects, aborts uint64
		breaker                                 bool
	}
	got := counters{s.Bots, s.Events, s.Sent, s.Paced, s.Reconnects, s.Aborts, s.BreakerOpen}
	if want := (counters{2, 7, 2, 1, 1, 1, true}); got != want || !s.At.Equal(t0) {
		t.Errorf("Collect = %+v at %v, want %+v at %v", got, s.At, want, t0)
	}
	if wantRules := map[string]uint64{"dead": 1, "idle": 2, "other": 1}; !maps.Equal(s.Rules, wantRules) {
		t.Errorf("rules = %v, want %v", s.Rules, wantRules)
	}
	if d := s.QueueDepth; d.Count != 2 || d.Sum != 7 || d.Max != 5 {
		t.Errorf("queue depth = %+v, want 2 drains of 7 events, max 5", d)
	}
	if l := s.WakeLagMicros; l.Count != 1 || l.Max != 1500 {
		t.Errorf("wake lag = %+v, want one of 1500us", l)
	}
}

func TestMetricsRun(t *testing.T) {
	r := NewRegistry()
	r.NewBot().Sent()
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		r.Run(ctx, tick)
		close(done)
	}()
	tick <- t0
	tick <- t0.Add(time.Second)
	cancel()
	<-done
	// Run received both ticks, so it collected both before it returned.
	if s := r.Last(); s.Sent != 1 || !s.At.Equal(t0.Add(time.Second)) {
		t.Errorf("Last = %+v, want the collection of the second tick", s)
	}
}
