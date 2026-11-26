package botclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// advanceToNext moves the clock to its earliest active timer, if any, and
// fires every timer then due.
func (c *fakeClock) advanceToNext() {
	c.mu.Lock()
	defer c.mu.Unlock()
	var next time.Time
	for _, t := range c.timers {
		if t.active && (next.IsZero() || t.when.Before(next)) {
			next = t.when
		}
	}
	if next.After(c.now) {
		c.now = next
	}
	c.fireLocked()
}

// fakeGame is a game connection whose server side is a script: the events
// it queues, then a hang-up or nothing.
type fakeGame struct {
	events     chan Event
	closed     chan struct{}
	closeOnce  sync.Once
	readerDone chan struct{}
	// answerLogout queues LeaveWorld when the brain sends Logout.
	answerLogout bool
	// entered is set once the bot entered the world over it.
	entered bool

	mu   sync.Mutex
	sent []byte // opcodes sent; guarded by mu
}

// gameScript is what a fakeGame's server does once the bot is in the world.
type gameScript struct {
	events []Event
	// hangUp closes the connection after the events.
	hangUp       bool
	answerLogout bool
}

func newFakeGame(s gameScript) *fakeGame {
	f := &fakeGame{
		events:       make(chan Event, len(s.events)+4),
		closed:       make(chan struct{}),
		readerDone:   make(chan struct{}),
		answerLogout: s.answerLogout,
	}
	for _, ev := range s.events {
		f.events <- ev
	}
	if s.hangUp {
		close(f.events)
	}
	return f
}

func (f *fakeGame) Send(p []byte) error {
	f.mu.Lock()
	f.sent = append(f.sent, p[0])
	f.mu.Unlock()
	if p[0] == opGameLogout && f.answerLogout {
		f.events <- LeaveWorld{}
	}
	return nil
}

func (f *fakeGame) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeGame) ReadEvents(q *Events, done <-chan struct{}) error {
	defer close(f.readerDone)
	for {
		select {
		case ev, ok := <-f.events:
			if !ok {
				select {
				case <-done:
					return nil
				default:
					return fmt.Errorf("read game frame: %w", io.EOF)
				}
			}
			if !q.push(ev, done) {
				return nil
			}
		case <-f.closed:
			select {
			case <-done:
				return nil
			default:
				return fmt.Errorf("read game frame: %w", net.ErrClosed)
			}
		}
	}
}

func (f *fakeGame) sentOpcode(op byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sent {
		if s == op {
			return true
		}
	}
	return false
}

// fakeNet scripts a bot's connections. The nth login returns logins[n] (nil
// past the end); the nth game dial fails with dials[n] when it is set; the
// nth game entry fails with enters[n] or plays games[n]
// (staying in the world past the end). Once the bot has entered cancelAt
// times, ctx is canceled. It is used by the bot's goroutine only; the test
// reads it after Run returns.
type fakeNet struct {
	t        *testing.T
	logins   []error
	dials    []error
	enters   []error
	games    []gameScript
	cancelAt int
	cancel   context.CancelFunc
	// onLogin, when set, runs at the start of every login.
	onLogin func()
	// onEnter, when set, runs at the start of every game entry with its
	// index, the character asked for and the entry's chosen callback; an
	// error it returns fails the entry.
	onEnter func(i int, ch Character, chosen func(string, int32)) error

	nLogin, nDial, nEnter int
	// entered counts the entries that reached the world.
	entered int
	played  []*fakeGame
}

func (n *fakeNet) login(ctx context.Context, account, password string) (LoginResult, error) {
	if n.onLogin != nil {
		n.onLogin()
	}
	i := n.nLogin
	n.nLogin++
	if i < len(n.logins) && n.logins[i] != nil {
		return LoginResult{}, n.logins[i]
	}
	return LoginResult{GameAddr: "127.0.0.1:7777"}, nil
}

func (n *fakeNet) dialGame(ctx context.Context, addr string) (gameSession, error) {
	n.nDial++
	if i := n.nDial - 1; i < len(n.dials) && n.dials[i] != nil {
		return nil, n.dials[i]
	}
	if k := len(n.played); k > 0 {
		prev := n.played[k-1]
		if prev.entered {
			select {
			case <-prev.readerDone:
			default:
				n.t.Error("dialed again before the previous session's reader was joined")
			}
		}
		select {
		case <-prev.closed:
		default:
			n.t.Error("dialed again before the previous connection was closed")
		}
	}
	i := len(n.played)
	var s gameScript
	if i < len(n.games) {
		s = n.games[i]
	}
	g := newFakeGame(s)
	n.played = append(n.played, g)
	return g, nil
}

func (n *fakeNet) enterGame(ctx context.Context, g gameSession, account string, keys SessionKeys, ch Character, chosen func(string, int32)) (Self, error) {
	i := n.nEnter
	n.nEnter++
	if n.onEnter != nil {
		if err := n.onEnter(i, ch, chosen); err != nil {
			return Self{}, err
		}
	}
	if i < len(n.enters) && n.enters[i] != nil {
		return Self{}, n.enters[i]
	}
	name := ch.Name
	if name == "" {
		name = ch.Names[0]
	}
	chosen(name, 0)
	g.(*fakeGame).entered = true
	n.entered++
	if n.entered == n.cancelAt {
		n.cancel()
	}
	return Self{ObjectID: selfID, Name: name}, nil
}

// newTestBot returns a bot over net sharing gate, timed by clock.
func newTestBot(t *testing.T, persistent bool, n *fakeNet, gate *LoginGate, clock *fakeClock) *Bot {
	t.Helper()
	cfg := SessionConfig{Persistent: persistent, Clock: clock, Gate: gate}
	rng := rand.New(rand.NewPCG(1, 2))
	ch, err := NewCharacter(botnames.DefaultMarker, "bot1", rng)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBot(cfg, "bot1", "not-a-secret", BotConfig{Character: ch}, rng)
	if err != nil {
		t.Fatal(err)
	}
	b.net = n
	return b
}

// runBot runs b until it returns, moving clock to the next timer whenever
// one is armed.
func runBot(ctx context.Context, b *Bot, clock *fakeClock) error {
	errc := make(chan error, 1)
	go func() { errc <- b.Run(ctx) }()
	for {
		select {
		case err := <-errc:
			return err
		case <-clock.armed:
			clock.advanceToNext()
		}
	}
}

// startBot builds a bot over a fresh fakeNet and gate.
func startBot(t *testing.T, persistent bool, n *fakeNet) (*Bot, *fakeClock, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	n.t, n.cancel = t, cancel
	clock := newFakeClock(t0)
	gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
	return newTestBot(t, persistent, n, gate, clock), clock, ctx
}

// checkJoined fails unless every session's connection was closed and its
// reader joined.
func checkJoined(t *testing.T, n *fakeNet) {
	t.Helper()
	for i, g := range n.played {
		if g.entered {
			select {
			case <-g.readerDone:
			default:
				t.Errorf("session %d: reader not joined", i)
			}
		}
		select {
		case <-g.closed:
		default:
			t.Errorf("session %d: connection not closed", i)
		}
	}
}

var (
	errRefused     = fmt.Errorf("dial login server: %w", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)})
	errInUse       = &LoginError{Packet: "LoginFail", Reason: loginFailAccountInUse}
	serverCloseEnd = gameScript{events: []Event{ServerClose{}}, hangUp: true}
	abortEnd       = gameScript{hangUp: true}
)

func TestSessionReconnects(t *testing.T) {
	tests := []struct {
		name   string
		logins []error
		games  []gameScript
		// sessions is how many sessions reach the world before the test
		// cancels.
		sessions, wantLogins int
	}{
		{"after a kick, once the old character left", nil, []gameScript{serverCloseEnd}, 2, 2},
		{"after an abort", nil, []gameScript{abortEnd}, 2, 2},
		{"after an unasked LeaveWorld", nil, []gameScript{{events: []Event{LeaveWorld{}}, hangUp: true}}, 2, 2},
		{"through AccountInUse after a drop", []error{nil, errInUse}, []gameScript{abortEnd}, 2, 3},
		{"through AccountInUse at the first login", []error{errInUse}, nil, 1, 2},
		{"through a refused dial", []error{errRefused, errRefused}, nil, 1, 3},
		{"through a retryable LoginFail", []error{
			&LoginError{Packet: "LoginFail", Reason: loginFailSystemError},
			&LoginError{Packet: "LoginFail", Reason: loginFailAccessFailed},
			&LoginError{Packet: "LoginFail", Reason: loginFailServerOverloaded},
			&LoginError{Packet: "LoginFail", Reason: loginFailMaintenance},
		}, nil, 1, 5},
		{"through PlayFail", []error{&LoginError{Packet: "PlayFail", Reason: 0x0f}}, nil, 1, 2},
		{"after a login timeout", []error{os.ErrDeadlineExceeded}, nil, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNet{logins: tt.logins, games: tt.games, cancelAt: tt.sessions}
			b, clock, ctx := startBot(t, true, n)
			if err := runBot(ctx, b, clock); err != nil {
				t.Fatalf("Run = %v, want nil", err)
			}
			if n.nLogin != tt.wantLogins || n.entered != tt.sessions {
				t.Errorf("logins, sessions = %d, %d; want %d, %d", n.nLogin, n.entered, tt.wantLogins, tt.sessions)
			}
			checkJoined(t, n)
		})
	}
}

func TestSessionRetriesAfterEntryFailure(t *testing.T) {
	n := &fakeNet{enters: []error{&GameStartError{Packet: "AuthLoginFail", Reason: 1}}, cancelAt: 1}
	b, clock, ctx := startBot(t, true, n)
	if err := runBot(ctx, b, clock); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if n.nLogin != 2 || n.nDial != 2 {
		t.Errorf("logins, dials = %d, %d; want 2, 2", n.nLogin, n.nDial)
	}
}

func TestSessionStops(t *testing.T) {
	charCreateFail := &GameStartError{Packet: "CharCreateFail", Reason: 1}
	tests := []struct {
		name       string
		persistent bool
		logins     []error
		enters     []error
		games      []gameScript
		want       error
		wantLogins int
	}{
		{"kick by another login is not fought", true, []error{nil, errInUse}, nil, []gameScript{serverCloseEnd}, ErrAccountTaken, 2},
		{"repeated aborts", true, nil, nil, []gameScript{abortEnd, abortEnd, abortEnd}, ErrRepeatedAborts, 3},
		{"not persistent, after a kick", false, nil, nil, []gameScript{serverCloseEnd}, errServerClose, 1},
		{"not persistent, after an abort", false, nil, nil, []gameScript{abortEnd}, errConnLost, 1},
		{"banned account", true, []error{&LoginError{Packet: "AccountKicked", Reason: 0x20}}, nil, nil, &LoginError{}, 1},
		{"unknown LoginFail", true, []error{&LoginError{Packet: "LoginFail", Reason: 0x11}}, nil, nil, &LoginError{}, 1},
		{"character creation refused", true, nil, []error{charCreateFail}, nil, charCreateFail, 1},
		{"every name refused", true, nil, []error{fmt.Errorf("all 8 candidate names refused: %w", &GameStartError{Packet: "CharCreateFail", Reason: 2})}, nil, &GameStartError{}, 1},
		{"character banned", true, nil, []error{fmt.Errorf("character %q (delete timer -1): %w", "x", errCharacterUnplayable)}, nil, errCharacterUnplayable, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNet{logins: tt.logins, enters: tt.enters, games: tt.games}
			b, clock, ctx := startBot(t, tt.persistent, n)
			err := runBot(ctx, b, clock)
			switch want := tt.want.(type) {
			case *LoginError:
				if _, ok := errors.AsType[*LoginError](err); !ok {
					t.Fatalf("Run = %v, want a *LoginError", err)
				}
			case *GameStartError:
				if _, ok := errors.AsType[*GameStartError](err); !ok {
					t.Fatalf("Run = %v, want a *GameStartError", err)
				}
			default:
				if !errors.Is(err, want) {
					t.Fatalf("Run = %v, want %v", err, want)
				}
			}
			if n.nLogin != tt.wantLogins {
				t.Errorf("logins = %d, want %d", n.nLogin, tt.wantLogins)
			}
			if b.cfg.Gate.Err() != nil {
				t.Errorf("breaker tripped: %v", b.cfg.Gate.Err())
			}
			checkJoined(t, n)
		})
	}
}

func TestSessionCredentialBreaker(t *testing.T) {
	for _, reason := range []int32{loginFailPasswordWrong, loginFailUserOrPassWrong} {
		t.Run(fmt.Sprintf("reason %#x", reason), func(t *testing.T) {
			n := &fakeNet{logins: []error{&LoginError{Packet: "LoginFail", Reason: reason}}}
			b, clock, ctx := startBot(t, true, n)
			if err := runBot(ctx, b, clock); err == nil {
				t.Fatal("Run = nil, want the refused credential")
			}
			if !errors.Is(b.cfg.Gate.Err(), ErrLoginsHalted) {
				t.Fatalf("gate = %v, want halted", b.cfg.Gate.Err())
			}
			// Every other bot of the process stops before it logs in.
			other := &fakeNet{t: t}
			ob := newTestBot(t, true, other, b.cfg.Gate, clock)
			if err := runBot(ctx, ob, clock); !errors.Is(err, ErrLoginsHalted) {
				t.Fatalf("other bot Run = %v, want %v", err, ErrLoginsHalted)
			}
			if n.nLogin != 1 || other.nLogin != 0 {
				t.Errorf("logins = %d, %d; want 1, 0", n.nLogin, other.nLogin)
			}
		})
	}
}

// TestSessionCredentialBreakerHoldsTurn runs a second bot that waits for the
// gate's turn while the first one's credential is refused: the breaker trips
// before the turn passes on, so the second bot never logs in.
func TestSessionCredentialBreakerHoldsTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock := newFakeClock(t0)
	gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
	other := &fakeNet{t: t, cancelAt: 1, cancel: cancel}
	ob := newTestBot(t, true, other, gate, clock)
	errc := make(chan error, 1)
	n := &fakeNet{t: t, cancel: cancel, logins: []error{&LoginError{Packet: "LoginFail", Reason: loginFailPasswordWrong}}}
	n.onLogin = func() {
		started := make(chan struct{})
		go func() {
			close(started)
			errc <- ob.Run(ctx)
		}()
		<-started
		// A slow login: the spacing has passed when the turn passes on.
		clock.Advance(10 * time.Second)
	}
	b := newTestBot(t, true, n, gate, clock)
	if err := runBot(ctx, b, clock); !credentialRefused(err) {
		t.Fatalf("Run = %v, want the refused credential", err)
	}
	for {
		select {
		case err := <-errc:
			if !errors.Is(err, ErrLoginsHalted) {
				t.Fatalf("other bot Run = %v, want %v", err, ErrLoginsHalted)
			}
			if other.nLogin != 0 {
				t.Fatalf("other bot logged in %d times after the refused credential", other.nLogin)
			}
			return
		case <-clock.armed:
			clock.advanceToNext()
		}
	}
}

// TestSessionHaltsInTurn checks that a refused credential trips the breaker
// before enter returns, while the bot still holds the gate's turn.
func TestSessionHaltsInTurn(t *testing.T) {
	clock := newFakeClock(t0)
	gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
	n := &fakeNet{t: t, logins: []error{&LoginError{Packet: "LoginFail", Reason: loginFailUserOrPassWrong}}}
	b := newTestBot(t, true, n, gate, clock)
	turn, err := gate.Take(t.Context(), "bot1")
	if err != nil {
		t.Fatal(err)
	}
	defer turn.Done()
	if _, _, err := b.enter(t.Context(), turn); !credentialRefused(err) {
		t.Fatalf("enter = %v, want the refused credential", err)
	}
	if !errors.Is(gate.Err(), ErrLoginsHalted) {
		t.Fatalf("gate = %v with the turn held, want halted", gate.Err())
	}
}

func TestSessionDelayAfterSession(t *testing.T) {
	lost := fmt.Errorf("%w: EOF", errConnLost)
	tests := []struct {
		name     string
		err      error
		min, max time.Duration
	}{
		{"after a ServerClose", errServerClose, kickWait, kickWait + kickJitter},
		{"after an unasked LeaveWorld", errLeftWorld, kickWait, kickWait + kickJitter},
		{"after an abort", lost, retryBase / 2, retryBase},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for seed := range uint64(50) {
				b := &Bot{cfg: SessionConfig{Persistent: true}, rng: rand.New(rand.NewPCG(seed, 2)), log: discardLog(), retry: backoff{base: retryBase, max: retryMax}}
				d, err := b.after(true, time.Minute, tt.err)
				if err != nil {
					t.Fatalf("seed %d: after = %v, want a retry", seed, err)
				}
				if d < tt.min || d >= tt.max {
					t.Fatalf("seed %d: delay %v, want in [%v, %v)", seed, d, tt.min, tt.max)
				}
			}
		})
	}
}

func TestSessionShutdown(t *testing.T) {
	t.Run("logs out in the world", func(t *testing.T) {
		n := &fakeNet{games: []gameScript{{answerLogout: true}}, cancelAt: 1}
		b, clock, ctx := startBot(t, true, n)
		if err := runBot(ctx, b, clock); err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
		if !n.played[0].sentOpcode(opGameLogout) {
			t.Error("no Logout sent")
		}
		checkJoined(t, n)
	})
	t.Run("closes an unanswered logout", func(t *testing.T) {
		n := &fakeNet{cancelAt: 1}
		b, clock, ctx := startBot(t, true, n)
		if err := runBot(ctx, b, clock); err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
		if !n.played[0].sentOpcode(opGameLogout) {
			t.Error("no Logout sent")
		}
		checkJoined(t, n)
	})
	t.Run("during a backoff", func(t *testing.T) {
		n := &fakeNet{logins: []error{errInUse}}
		b, clock, ctx := startBot(t, true, n)
		errc := make(chan error, 1)
		go func() { errc <- b.Run(ctx) }()
		<-clock.armed // the gate's spacing after the login, or the backoff
		n.cancel()
		if err := <-errc; err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	})
}

func TestSessionStableResetsAborts(t *testing.T) {
	b := &Bot{cfg: SessionConfig{Persistent: true}, rng: rand.New(rand.NewPCG(1, 2)), log: discardLog(), retry: backoff{base: retryBase, max: retryMax}}
	lost := fmt.Errorf("%w: EOF", errConnLost)
	for range maxAborts - 1 {
		if _, err := b.after(true, time.Minute, lost); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.after(true, stableSession, lost); err != nil {
		t.Fatalf("after a stable session: %v, want a retry", err)
	}
	if b.aborts != 1 || b.retry.n != 1 {
		t.Fatalf("aborts, backoff step = %d, %d; want 1, 1", b.aborts, b.retry.n)
	}
}

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }
