package botclient

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	tests := []struct {
		base, max time.Duration
		want      []time.Duration // the cap d of each step: it waits [d/2, d)
	}{
		{retryBase, retryMax, []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 64 * time.Second, 128 * time.Second, 256 * time.Second, 5 * time.Minute, 5 * time.Minute}},
		{refusedBase, refusedMax, []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 64 * time.Second, 2 * time.Minute, 2 * time.Minute}},
	}
	for _, tt := range tests {
		rng := rand.New(rand.NewPCG(7, 8))
		b := backoff{base: tt.base, max: tt.max}
		for round := range 2 {
			seen := map[time.Duration]bool{}
			for i, d := range tt.want {
				got := b.next(rng)
				if got < d/2 || got >= d {
					t.Errorf("base %s round %d step %d: %s, want in [%s, %s)", tt.base, round, i, got, d/2, d)
				}
				seen[got] = true
			}
			if len(seen) < len(tt.want)-1 {
				t.Errorf("base %s: %d distinct delays in %d steps, want them re-jittered", tt.base, len(seen), len(tt.want))
			}
			b.reset()
		}
	}
}

// dialStart is one dial a test bot started.
type dialStart struct {
	bot int
	at  time.Time
}

func TestLoginGateSpacesDials(t *testing.T) {
	const bots, dialsPerTurn = 3, 2
	clock := newFakeClock(t0)
	g := NewLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	var (
		mu     sync.Mutex
		starts []dialStart
		wg     sync.WaitGroup
	)
	for bot := range bots {
		wg.Go(func() {
			turn, err := g.Take(t.Context())
			if err != nil {
				t.Error(err)
				return
			}
			defer turn.Done()
			for range dialsPerTurn {
				if err := turn.Dial(t.Context()); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				starts = append(starts, dialStart{bot, clock.Now()})
				mu.Unlock()
				turn.Dialed(nil)
			}
		})
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	for done := false; !done; {
		select {
		case <-finished:
			done = true
		case <-clock.armed:
			clock.advanceToNext()
		}
	}
	if len(starts) != bots*dialsPerTurn {
		t.Fatalf("%d dials, want %d", len(starts), bots*dialsPerTurn)
	}
	for i := 1; i < len(starts); i++ {
		gap := starts[i].at.Sub(starts[i-1].at)
		if gap < dialSpacing || gap >= dialSpacing+dialJitter {
			t.Errorf("dial %d started %s after the previous one, want in [%s, %s)", i, gap, dialSpacing, dialSpacing+dialJitter)
		}
		if i%dialsPerTurn != 0 && starts[i].bot != starts[i-1].bot {
			t.Errorf("dial %d by bot %d inside the turn of bot %d", i, starts[i].bot, starts[i-1].bot)
		}
	}
}

func TestAllowlistedLoginGate(t *testing.T) {
	clock := newFakeClock(t0)
	g := NewAllowlistedLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	turn, err := g.Take(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if err := turn.Dial(t.Context()); err != nil {
			t.Fatal(err)
		}
		if now := clock.Now(); !now.Equal(t0) {
			t.Fatalf("dial %d started at +%s, want no wait", i, now.Sub(t0))
		}
		turn.Dialed(nil)
	}
	// Turns stay exclusive, so the breaker sees one login at a time.
	if len(g.turn) != 0 {
		t.Fatal("the turn token is free during a turn")
	}
	// A refused dial still backs every bot off.
	turn.Dialed(errRefused)
	if wait := g.next.Sub(t0); wait < refusedBase/2 || wait >= refusedBase {
		t.Errorf("after a refusal the next dial is in %s, want in [%s, %s)", wait, refusedBase/2, refusedBase)
	}
	turn.Done()
}

func TestLoginGateRefusedBackoff(t *testing.T) {
	clock := newFakeClock(t0)
	g := NewLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	turn, err := g.Take(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := turn.Dial(t.Context()); err != nil {
		t.Fatal(err)
	}
	for i, d := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 64 * time.Second, 2 * time.Minute, 2 * time.Minute} {
		turn.Dialed(errRefused)
		if wait := g.next.Sub(t0); wait < d/2 || wait >= d {
			t.Errorf("refusal %d: next dial in %s, want in [%s, %s)", i+1, wait, d/2, d)
		}
	}
	turn.Dialed(&LoginError{Packet: "LoginFail", Reason: loginFailAccountInUse})
	if g.refused.n != 0 {
		t.Errorf("backoff step %d after a connection, want 0", g.refused.n)
	}
	turn.Done()
	turn.Done() // a second Done does nothing
	if _, err := g.Take(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestLoginGateHalt(t *testing.T) {
	cause := errors.New("account bot1: refused")
	t.Run("wakes a bot waiting for a turn", func(t *testing.T) {
		g := NewLoginGate(newFakeClock(t0), nil)
		if _, err := g.Take(t.Context()); err != nil {
			t.Fatal(err)
		}
		errc := make(chan error, 1)
		go func() {
			_, err := g.Take(t.Context())
			errc <- err
		}()
		g.Halt(cause)
		g.Halt(errors.New("later"))
		if err := <-errc; !errors.Is(err, ErrLoginsHalted) || !errors.Is(err, cause) {
			t.Fatalf("Take = %v, want %v of %v", err, ErrLoginsHalted, cause)
		}
		if _, err := g.Take(t.Context()); !errors.Is(err, cause) {
			t.Fatalf("Take after Halt = %v, want %v", err, cause)
		}
	})
	t.Run("wakes a bot waiting to dial", func(t *testing.T) {
		clock := newFakeClock(t0)
		g := NewLoginGate(clock, nil)
		turn, err := g.Take(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := turn.Dial(t.Context()); err != nil {
			t.Fatal(err)
		}
		errc := make(chan error, 1)
		go func() { errc <- turn.Dial(t.Context()) }()
		<-clock.armed
		g.Halt(cause)
		if err := <-errc; !errors.Is(err, ErrLoginsHalted) {
			t.Fatalf("Dial = %v, want %v", err, ErrLoginsHalted)
		}
	})
	t.Run("ctx ends a wait", func(t *testing.T) {
		clock := newFakeClock(t0)
		g := NewLoginGate(clock, nil)
		turn, err := g.Take(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		errc := make(chan error, 2)
		go func() {
			_, err := g.Take(ctx)
			errc <- err
		}()
		_ = turn.Dial(ctx)
		go func() { errc <- turn.Dial(ctx) }()
		<-clock.armed
		cancel()
		for range 2 {
			if err := <-errc; !errors.Is(err, context.Canceled) {
				t.Fatalf("wait = %v, want %v", err, context.Canceled)
			}
		}
	})
}
