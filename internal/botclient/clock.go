package botclient

import "time"

// Clock is the time source of the brain. SystemClock is the real one; tests
// inject a fake so brain logic never waits on the wall clock.
type Clock interface {
	Now() time.Time
	// NewTimer returns a timer that fires once d from now.
	NewTimer(d time.Duration) Timer
}

// Timer is a resettable one-shot timer. After Reset or Stop returns, C
// yields no value from the earlier schedule (the Go 1.23 time.Timer
// contract).
type Timer interface {
	C() <-chan time.Time
	Reset(d time.Duration)
	Stop()
}

// SystemClock is the wall clock.
type SystemClock struct{}

// Now is time.Now.
func (SystemClock) Now() time.Time { return time.Now() }

// NewTimer wraps time.NewTimer.
func (SystemClock) NewTimer(d time.Duration) Timer { return systemTimer{time.NewTimer(d)} }

type systemTimer struct{ t *time.Timer }

func (s systemTimer) C() <-chan time.Time   { return s.t.C }
func (s systemTimer) Reset(d time.Duration) { s.t.Reset(d) }
func (s systemTimer) Stop()                 { s.t.Stop() }
