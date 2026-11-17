package botclient

import (
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// TestLoginGateResetStaleHalt checks that a Take or Dial woken by a halt that
// a reset already replaced waits on the new generation instead of failing.
func TestLoginGateResetStaleHalt(t *testing.T) {
	tests := []struct {
		name string
		// wait starts the waiter, parks it on the first generation, and
		// returns how to let it through once the gate is reset.
		wait func(t *testing.T, g *LoginGate, clock *fakeClock, errc chan<- error) (release func())
	}{
		{
			name: "take",
			wait: func(t *testing.T, g *LoginGate, _ *fakeClock, errc chan<- error) func() {
				first, err := g.Take(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				go func() {
					turn, err := g.Take(t.Context())
					if err == nil && turn == nil {
						err = errors.New("Take = nil turn, nil error")
					}
					errc <- err
				}()
				return first.Done
			},
		},
		{
			name: "dial",
			wait: func(t *testing.T, g *LoginGate, clock *fakeClock, errc chan<- error) func() {
				turn, err := g.Take(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := turn.Dial(t.Context()); err != nil {
					t.Fatal(err)
				}
				go func() { errc <- turn.Dial(t.Context()) }()
				return func() { clock.Advance(dialSpacing + dialJitter) }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clock := newFakeClock(t0)
				g := NewLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
				errc := make(chan error, 1)
				release := tt.wait(t, g, clock, errc)
				synctest.Wait() // the waiter is parked on the first generation's halt

				// Trip and reset in one critical section: the waiter wakes on a
				// closed halt that is no longer current.
				g.mu.Lock()
				g.haltLocked(errors.New("account bot1: refused"))
				g.resetLocked()
				g.mu.Unlock()
				synctest.Wait()
				select {
				case err := <-errc:
					t.Fatalf("waiter ended on a stale halt: %v", err)
				default:
				}

				release()
				if err := <-errc; err != nil {
					t.Fatalf("waiter = %v, want it through on the new generation", err)
				}
			})
		})
	}
}

// TestLoginGateReset checks that a reset reopens a tripped gate and reports
// the closed breaker, that the next Halt trips it with its own cause, and that
// a reset of an open gate does nothing.
func TestLoginGateReset(t *testing.T) {
	first, second := errors.New("account bot1: refused"), errors.New("account bot2: refused")
	reg := botmetrics.NewRegistry()
	g := NewLoginGate(newFakeClock(t0), rand.New(rand.NewPCG(1, 2)))
	g.ReportBreaker(reg)

	g.Reset()
	if err := g.Err(); err != nil {
		t.Fatalf("Err after Reset of an open gate = %v, want nil", err)
	}
	if reg.Collect(t0).BreakerOpen {
		t.Fatal("breaker open after Reset of an open gate")
	}

	g.Halt(first)
	if _, err := g.Take(t.Context()); !errors.Is(err, first) {
		t.Fatalf("Take after Halt = %v, want %v", err, first)
	}
	g.Reset()
	if err := g.Err(); err != nil {
		t.Fatalf("Err after Reset = %v, want nil", err)
	}
	if reg.Collect(t0).BreakerOpen {
		t.Fatal("breaker open after Reset")
	}
	turn, err := g.Take(t.Context())
	if err != nil {
		t.Fatalf("Take after Reset = %v, want a turn", err)
	}
	if err := turn.Dial(t.Context()); err != nil {
		t.Fatalf("Dial after Reset = %v", err)
	}
	turn.Done()

	g.Halt(second)
	g.Halt(first)
	if _, err := g.Take(t.Context()); !errors.Is(err, ErrLoginsHalted) || !errors.Is(err, second) || errors.Is(err, first) {
		t.Fatalf("Take after the second Halt = %v, want %v of %v", err, ErrLoginsHalted, second)
	}
	if !reg.Collect(t0).BreakerOpen {
		t.Fatal("breaker closed after the second Halt")
	}
}

// TestLoginGateResetRace races Halt and Reset against bots taking turns and
// dialing: every Take and Dial either goes through or fails with
// ErrLoginsHalted, never with a nil turn and a nil error. Run it with -race.
func TestLoginGateResetRace(t *testing.T) {
	const bots, rounds = 8, 200
	g := NewAllowlistedLoginGate(newFakeClock(t0), rand.New(rand.NewPCG(1, 2)))
	g.ReportBreaker(botmetrics.NewRegistry())
	cause := errors.New("account bot1: refused")
	var wg sync.WaitGroup
	wg.Go(func() {
		for range rounds {
			g.Halt(cause)
			_ = g.Err()
			g.Reset()
		}
	})
	for range bots {
		wg.Go(func() {
			for range rounds {
				turn, err := g.Take(t.Context())
				switch {
				case err != nil && turn != nil:
					t.Errorf("Take = a turn and %v", err)
				case err != nil && !errors.Is(err, ErrLoginsHalted):
					t.Errorf("Take = %v, want nil or %v", err, ErrLoginsHalted)
				case err == nil && turn == nil:
					t.Error("Take = nil turn, nil error")
				}
				if err != nil {
					continue
				}
				if err := turn.Dial(t.Context()); err != nil && !errors.Is(err, ErrLoginsHalted) {
					t.Errorf("Dial = %v, want nil or %v", err, ErrLoginsHalted)
				}
				turn.Done()
			}
		})
	}
	wg.Wait()
	g.Reset()
	if _, err := g.Take(t.Context()); err != nil {
		t.Fatalf("Take after the last Reset = %v", err)
	}
}
