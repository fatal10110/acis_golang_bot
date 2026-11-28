package botclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// newPanicBot returns a bot over n sharing gate, timed by clock, with
// brain's config and a metrics Bot of reg.
func newPanicBot(t *testing.T, n sessionNet, gate *LoginGate, clock *fakeClock, reg *botmetrics.Registry, brain BrainConfig) *Bot {
	t.Helper()
	cfg := SessionConfig{Persistent: true, Clock: clock, Gate: gate}
	rng := rand.New(rand.NewPCG(1, 2))
	ch, err := NewCharacter(botnames.DefaultMarker, "bot1", rng)
	if err != nil {
		t.Fatal(err)
	}
	brain.Metrics = reg.NewBot()
	b, err := NewBot(cfg, "bot1", "not-a-secret", BotConfig{Character: ch, Brain: brain}, rng)
	if err != nil {
		t.Fatal(err)
	}
	b.net = n
	return b
}

// sessionBudget bounds the time the brain's timers may move botClock in one
// session of runPanicBot: well under stableSession.
const sessionBudget = stableSession / 4

// runPanicBot runs b, timed by botClock, and keeps the session time a
// panic is counted against below stableSession, which would reset the panic
// count: botClock moves freely only while b waits out its backoff, and by at
// most sessionBudget in all while it enters or plays (past that the brain's
// timers fire without the time moving), whichever other bot shares the
// gate. Until ready closes (nil: from the start) a playing b's timers do
// not fire at all, so a brain waiting for the test's cue does not spend its
// budget. gateClock, the clock of the gate and of every other bot, moves
// whenever a timer is armed.
func runPanicBot(ctx context.Context, b *Bot, botClock, gateClock *fakeClock, ready <-chan struct{}) error {
	errc := make(chan error, 1)
	go func() { errc <- b.Run(ctx) }()
	var spent time.Duration
	cued := ready == nil
	drive := func() {
		switch {
		case b.Status().State == StateReconnecting:
			spent = 0
			botClock.advanceToNext()
		case cued:
			spent += botClock.advanceWithin(sessionBudget - spent)
		}
	}
	for {
		select {
		case err := <-errc:
			return err
		case <-gateClock.armed:
			gateClock.advanceToNext()
		case <-botClock.armed:
			drive()
		case <-ready:
			ready, cued = nil, true
			drive()
		}
	}
}

// advanceWithin moves the time on to the earliest active timer when that is
// within limit, else leaves it; either way it fires that timer. It returns
// how far the time moved.
func (c *fakeClock) advanceWithin(limit time.Duration) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	var next *fakeTimer
	for _, t := range c.timers {
		if t.active && (next == nil || t.when.Before(next.when)) {
			next = t
		}
	}
	if next == nil {
		return 0
	}
	var moved time.Duration
	if d := next.when.Sub(c.now); d > 0 && d <= limit {
		c.now, moved = next.when, d
	}
	next.active = false
	next.ch <- c.now
	return moved
}

// checkTurnBack fails unless the gate's turn is free.
func checkTurnBack(t *testing.T, gate *LoginGate) {
	t.Helper()
	if len(gate.turn) != 1 {
		t.Error("the gate's turn was not given back")
	}
}

// checkPanics fails unless err stops the bot for ErrPanic after maxAborts
// panics, each logged in reg.
func checkPanics(t *testing.T, err error, reg *botmetrics.Registry, clock *fakeClock) {
	t.Helper()
	if !errors.Is(err, ErrPanic) || errors.Is(err, ErrRepeatedAborts) {
		t.Fatalf("Run = %v, want %v alone", err, ErrPanic)
	}
	if _, ok := errors.AsType[*panicError](err); !ok {
		t.Errorf("Run = %v, want the last panic wrapped", err)
	}
	if s := reg.Collect(clock.Now()); s.Panics != maxAborts || s.Aborts != 0 {
		t.Errorf("panics %d, aborts %d; want %d, 0", s.Panics, s.Aborts, maxAborts)
	}
}

// TestPanicBrainRule: a persona rule that panics ends each attempt, and the
// bot stops with ErrPanic after maxAborts of them. Each session's
// connection is closed and its reader joined before the next dial (fakeNet
// checks it), the turn is given back, and a second bot of the same gate,
// which took its turn while the first played, plays on.
func TestPanicBrainRule(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock, botClock := newFakeClock(t0), newFakeClock(t0)
	gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
	reg := botmetrics.NewRegistry(RuleNames()...)

	// The rule panics once the other bot is in the world: the other bot
	// cannot get its turn unless the first one gave it back before play.
	otherIn := make(chan struct{})
	var once sync.Once
	other := &fakeNet{t: t, games: []gameScript{{answerLogout: true}}}
	other.onEnter = func(int, Character, func(string, int32)) error {
		once.Do(func() { close(otherIn) })
		return nil
	}
	ob := newPanicBot(t, other, gate, clock, botmetrics.NewRegistry(RuleNames()...), BrainConfig{})
	errc := make(chan error, 1)

	n := &fakeNet{t: t}
	n.onEnter = func(i int, _ Character, _ func(string, int32)) error {
		if i == 0 {
			go func() { errc <- ob.Run(ctx) }()
		}
		return nil
	}
	goal := func(*World, time.Time) (Action, bool) {
		select {
		case <-otherIn:
			panic("rule bug")
		default:
			return Action{}, false
		}
	}
	b := newPanicBot(t, n, gate, botClock, reg, BrainConfig{Persona: Persona{Goal: goal}})
	err := runPanicBot(ctx, b, botClock, clock, otherIn)
	checkPanics(t, err, reg, botClock)
	if n.entered != maxAborts {
		t.Errorf("entered %d times, want %d", n.entered, maxAborts)
	}
	checkJoined(t, n)
	checkTurnBack(t, gate)

	select {
	case err := <-errc:
		t.Fatalf("other bot Run = %v while the first one panicked", err)
	default:
	}
	if other.nLogin != 1 || other.entered != 1 {
		t.Errorf("other bot logins %d, entries %d; want 1, 1", other.nLogin, other.entered)
	}
	cancel()
	for {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("other bot Run = %v, want nil", err)
			}
			checkJoined(t, other)
			return
		case <-clock.armed:
			clock.advanceToNext()
		}
	}
}

// TestPanicEnterGame: a panic in the game entry ends the attempt, gives
// the turn back and closes the connection it entered on, and the bot stops
// with ErrPanic after maxAborts of them.
func TestPanicEnterGame(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock, botClock := newFakeClock(t0), newFakeClock(t0)
	gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
	reg := botmetrics.NewRegistry(RuleNames()...)
	n := &fakeNet{t: t}
	n.onEnter = func(int, Character, func(string, int32)) error { panic("entry bug") }
	b := newPanicBot(t, n, gate, botClock, reg, BrainConfig{})
	err := runPanicBot(ctx, b, botClock, clock, nil)
	checkPanics(t, err, reg, botClock)
	if n.nEnter != maxAborts || len(n.played) != maxAborts {
		t.Errorf("entries %d, dials %d; want %d each", n.nEnter, len(n.played), maxAborts)
	}
	checkJoined(t, n)
	checkTurnBack(t, gate)
}

// panicNet is a fakeNet whose game connections' readers panic.
type panicNet struct{ *fakeNet }

// panicReader is a game connection whose reader panics.
type panicReader struct{ *fakeGame }

func (g panicReader) ReadEvents(*Events, <-chan struct{}) error {
	defer close(g.readerDone)
	panic("decoder bug")
}

func (n panicNet) dialGame(ctx context.Context, addr string) (gameSession, error) {
	g, err := n.fakeNet.dialGame(ctx, addr)
	if err != nil {
		return nil, err
	}
	return panicReader{g.(*fakeGame)}, nil
}

func (n panicNet) enterGame(ctx context.Context, g gameSession, account string, keys SessionKeys, ch Character, chosen func(string, int32)) (Self, error) {
	return n.fakeNet.enterGame(ctx, g.(panicReader).fakeGame, account, keys, ch, chosen)
}

// TestPanicReader: a panic in the session's reader is the read error it
// queues as connLost, so the brain ends the session as after a lost
// connection; the bot stops with ErrPanic after maxAborts of them.
func TestPanicReader(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock, botClock := newFakeClock(t0), newFakeClock(t0)
	gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
	reg := botmetrics.NewRegistry(RuleNames()...)
	n := &fakeNet{t: t}
	b := newPanicBot(t, panicNet{n}, gate, botClock, reg, BrainConfig{})
	err := runPanicBot(ctx, b, botClock, clock, nil)
	checkPanics(t, err, reg, botClock)
	if !errors.Is(err, errConnLost) {
		t.Errorf("Run = %v, want the session ended by %v", err, errConnLost)
	}
	if n.entered != maxAborts {
		t.Errorf("entered %d times, want %d", n.entered, maxAborts)
	}
	checkJoined(t, n)
	checkTurnBack(t, gate)
}

// TestPanicNotPersistent: a bot not persistent stops at its session's
// first panic, with ErrPanic.
func TestPanicNotPersistent(t *testing.T) {
	b := &Bot{rng: rand.New(rand.NewPCG(1, 2)), log: discardLog(), retry: backoff{base: retryBase, max: retryMax}}
	_, err := b.after(true, time.Minute, &panicError{where: "attempt", value: "bug"})
	if !errors.Is(err, ErrPanic) {
		t.Fatalf("after = %v, want %v", err, ErrPanic)
	}
}

// TestPanicStableResets: a stable session resets the panic count, and a
// panic in play after a stable stretch still reports the session entered
// and how long it played.
func TestPanicStableResets(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		b := &Bot{cfg: SessionConfig{Persistent: true}, rng: rand.New(rand.NewPCG(1, 2)), log: discardLog(), retry: backoff{base: retryBase, max: retryMax}}
		p := &panicError{where: "attempt", value: "bug"}
		for range maxAborts - 1 {
			if _, err := b.after(true, time.Minute, p); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := b.after(true, stableSession, p); err != nil {
			t.Fatalf("after a stable session: %v, want a retry", err)
		}
		if b.panics != 1 || b.aborts != 0 || b.retry.n != 1 {
			t.Fatalf("panics, aborts, backoff step = %d, %d, %d; want 1, 0, 1", b.panics, b.aborts, b.retry.n)
		}
		// A stable session that ends otherwise resets it too.
		if _, err := b.after(true, stableSession, fmt.Errorf("%w: EOF", errConnLost)); err != nil {
			t.Fatal(err)
		}
		if b.panics != 0 {
			t.Fatalf("panics %d after a stable abort, want 0", b.panics)
		}
	})
	t.Run("played", func(t *testing.T) {
		clock := newFakeClock(t0)
		gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
		reg := botmetrics.NewRegistry(RuleNames()...)
		goal := func(_ *World, now time.Time) (Action, bool) {
			// The entry's own pacing passes after t0.
			if now.Sub(t0) >= stableSession+time.Minute {
				panic("rule bug")
			}
			return Action{}, false
		}
		b := newPanicBot(t, &fakeNet{t: t}, gate, clock, reg, BrainConfig{Persona: Persona{Goal: goal}})
		b.panics = maxAborts - 1
		type result struct {
			entered bool
			played  time.Duration
			err     error
		}
		res := make(chan result, 1)
		go func() {
			entered, played, err := b.attempt(t.Context())
			res <- result{entered, played, err}
		}()
		var r result
	wait:
		for {
			select {
			case r = <-res:
				break wait
			case <-clock.armed:
				clock.advanceToNext()
			}
		}
		if _, ok := errors.AsType[*panicError](r.err); !ok || !r.entered || r.played < stableSession {
			t.Fatalf("attempt = %v, %v, %v; want entered, played %v or more, a panic", r.entered, r.played, r.err, stableSession)
		}
		if _, err := b.after(r.entered, r.played, r.err); err != nil {
			t.Fatalf("after a stable panicked session: %v, want a retry", err)
		}
		if b.panics != 1 {
			t.Fatalf("panics %d, want 1", b.panics)
		}
	})
}

// panicSender is a connection whose sends panic: a bug in the brain's
// send path.
type panicSender struct{}

func (panicSender) Send([]byte) error { panic("send bug") }

// TestPanicReleasesHolds: a brain that panics gives back what the bot
// holds, as when its session ends: its spot, its store place, its party
// claim and its craft board lease (the rules' stop hooks Brain.Run defers).
func TestPanicReleasesHolds(t *testing.T) {
	emote := func(*World, time.Time) (Action, bool) { return emoteAction(7), true }
	tests := []struct {
		name string
		// hold returns a brain holding something, and whether it is still
		// held.
		hold func(t *testing.T) (*Brain, func() bool)
	}{
		{"spot", func(t *testing.T) (*Brain, func() bool) {
			b := newSpotBrain(t, 3, testSpot("near", 4000, 0, 1, 3, 9))
			b.decide(at(time.Second))
			return b, func() bool { return b.cfg.Spots.Held(0) != 0 }
		}},
		{"store", func(t *testing.T) (*Brain, func() bool) {
			b := newStoreBrain(t, 1000)
			id, _ := b.cfg.Stores.claim(Point{})
			b.store.claim, b.store.claimed, b.store.step = id, true, storeOpen
			return b, func() bool { return b.cfg.Stores.Open() != 0 }
		}},
		{"party claim", func(t *testing.T) (*Brain, func() bool) {
			claims := NewPartyClaims()
			b, _ := newClaimBrain(t, claimBot0, claims)
			if !claims.claim("Leader", claimBot0, t0) {
				t.Fatal("claim refused on an empty table")
			}
			return b, func() bool { return claims.Held() != 0 }
		}},
		{"craft board", func(t *testing.T) (*Brain, func() bool) {
			b, _ := newTestBrain(t, Persona{})
			board := NewCraftBoard()
			b.cfg.CraftBoard = board
			board.TakeLease(boardClan, selfID, t0)
			return b, func() bool { return !board.TakeLease(boardClan, boardBotB, t0) }
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, held := tt.hold(t)
			if !held() {
				t.Fatal("nothing held before Run")
			}
			b.persona.Goal = emote
			b.out = panicSender{}
			clock := b.clock.(*fakeClock)
			panicked := make(chan any, 1)
			go func() {
				defer func() { panicked <- recover() }()
				_ = b.Run(make(chan struct{}), nil)
			}()
			for {
				select {
				case v := <-panicked:
					if v == nil {
						t.Fatal("Run returned without panicking")
					}
					if held() {
						t.Error("still held after the brain panicked")
					}
					return
				case <-clock.armed:
					clock.advanceToNext()
				}
			}
		})
	}
}

// TestEventsConcurrentPushDrain drives push and Drain from concurrent
// callers: every event pushed is drained once, in each pusher's order.
func TestEventsConcurrentPushDrain(t *testing.T) {
	const pushers, each = 4, 500
	q := NewEvents(8)
	done := make(chan struct{})
	defer close(done)
	var wg sync.WaitGroup
	for p := range pushers {
		wg.Go(func() {
			for i := range each {
				if !q.push(StatusUpdate{ObjectID: int32(p), HP: int32(i)}, done) {
					t.Error("push refused before done")
					return
				}
			}
		})
	}
	next := make([]int32, pushers)
	var batch []Event
	for got := 0; got < pushers*each; {
		<-q.Ready()
		batch = q.Drain(batch[:0])
		for _, ev := range batch {
			s := ev.(StatusUpdate)
			if s.HP != next[s.ObjectID] {
				t.Fatalf("pusher %d: event %d, want %d", s.ObjectID, s.HP, next[s.ObjectID])
			}
			next[s.ObjectID]++
		}
		got += len(batch)
	}
	wg.Wait()
}
