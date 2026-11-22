package e2e

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// stepClock moves to each timer's end as the timer is made, so a wait on
// it ends at once, at the time it would have ended.
type stepClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *stepClock) NewTimer(d time.Duration) botclient.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	fired := make(chan time.Time, 1)
	fired <- c.now
	return firedTimer(fired)
}

type firedTimer chan time.Time

func (t firedTimer) C() <-chan time.Time { return t }
func (firedTimer) Reset(time.Duration)   {}
func (firedTimer) Stop()                 {}

func TestPoolExhaustedWithinItsBound(t *testing.T) {
	clock := &stepClock{now: time.Unix(1000, 0)}
	p := newGMPool(gmDir(t, "gm"), clock)
	held, err := p.take(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	start := clock.Now()
	// The scenario's ctx has no deadline: the pool's own bound ends the wait.
	_, err = p.take(t.Context())
	if err == nil || !strings.Contains(err.Error(), "GM pool exhausted") {
		t.Fatalf("take with every GM held = %v, want GM pool exhausted", err)
	}
	if waited := clock.Now().Sub(start); waited != gmWait {
		t.Errorf("waited %s for a GM, want the pool's bound %s", waited, gmWait)
	}
}

func TestGMShortfall(t *testing.T) {
	tests := []struct {
		gms, parallel int
		warn          bool
	}{
		{gms: 1, parallel: 1},
		{gms: 1, parallel: 2, warn: true},
		{gms: 1, parallel: 16, warn: true},
		{gms: MaxScenarios, parallel: 16},
		{gms: 0, parallel: 16}, // no GM: the scenarios that need one skip
	}
	for _, tt := range tests {
		got := gmShortfall(tt.gms, tt.parallel)
		if (got != "") != tt.warn || tt.warn && !strings.Contains(got, EnvGMFile) {
			t.Errorf("gmShortfall(%d, %d) = %q, want warning %v", tt.gms, tt.parallel, got, tt.warn)
		}
	}
}
