package botclient

import (
	"sync"
	"time"
)

// fakeClock is a Clock that moves only when a test advances it. Every timer
// created or reset signals armed, so a test can wait for the brain to park
// before it advances the time.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
	armed  chan struct{}
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now, armed: make(chan struct{}, 1)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) Timer {
	t := &fakeTimer{clock: c, ch: make(chan time.Time, 1)}
	c.mu.Lock()
	c.timers = append(c.timers, t)
	c.mu.Unlock()
	t.Reset(d)
	return t
}

// Advance moves the time on by d and fires every timer then due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.fireLocked()
}

func (c *fakeClock) fireLocked() {
	for _, t := range c.timers {
		if t.active && !t.when.After(c.now) {
			t.active = false
			t.ch <- c.now
		}
	}
}

type fakeTimer struct {
	clock  *fakeClock
	ch     chan time.Time
	when   time.Time
	active bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Reset(d time.Duration) {
	c := t.clock
	c.mu.Lock()
	t.drainLocked()
	t.when, t.active = c.now.Add(d), true
	c.fireLocked()
	c.mu.Unlock()
	signal(c.armed)
}

func (t *fakeTimer) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	t.active = false
	t.drainLocked()
}

func (t *fakeTimer) drainLocked() {
	select {
	case <-t.ch:
	default:
	}
}
