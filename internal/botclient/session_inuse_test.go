package botclient

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"
)

// inUseRun is n AccountInUse replies in a row.
func inUseRun(n int) []error { return slices.Repeat([]error{errInUse}, n) }

// TestSessionInUseWindow runs a bot through AccountInUse replies until it
// stops: it retries at most inUseRetryMax apart and stops at the first
// AccountInUse inUseWindow or more after the first of the window, however
// late that first one came.
func TestSessionInUseWindow(t *testing.T) {
	entryFail := &GameStartError{Packet: "AuthLoginFail", Reason: 1}
	tests := []struct {
		name   string
		logins []error
		enters []error
		games  []gameScript
		// from is the login (from 0) whose AccountInUse starts the window.
		from int
	}{
		{name: "at the first login", logins: inUseRun(100)},
		{name: "late after refused dials", logins: append([]error{errRefused, errRefused, errRefused}, inUseRun(100)...), from: 3},
		{name: "after an abort", logins: append([]error{nil}, inUseRun(100)...), games: []gameScript{abortEnd}, from: 1},
		{
			// The good login ends the first window; the failed entry is a
			// non-kick end, so the next AccountInUse starts a new one.
			name:   "anew after a failed entry",
			logins: append(append(inUseRun(6), nil), inUseRun(100)...),
			enters: []error{entryFail},
			from:   7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNet{logins: tt.logins, enters: tt.enters, games: tt.games}
			b, clock, ctx := startBot(t, true, n)
			var at []time.Time
			n.onLogin = func() { at = append(at, clock.Now()) }
			if err := runBot(ctx, b, clock); !errors.Is(err, ErrAccountTaken) {
				t.Fatalf("Run = %v, want %v", err, ErrAccountTaken)
			}
			last := len(at) - 1
			if last >= len(tt.logins) || tt.logins[last] != errInUse {
				t.Fatalf("stopped after login %d, want an AccountInUse", last)
			}
			if held := at[last].Sub(at[tt.from]); held < inUseWindow {
				t.Errorf("stopped after %v of AccountInUse, want at least %v", held, inUseWindow)
			}
			if held := at[last-1].Sub(at[tt.from]); held >= inUseWindow {
				t.Errorf("retried after %v of AccountInUse, want a stop past %v", held, inUseWindow)
			}
			for i := tt.from + 1; i <= last; i++ {
				if gap := at[i].Sub(at[i-1]); gap > inUseRetryMax {
					t.Errorf("logins %d and %d %v apart, want at most %v", i-1, i, gap, inUseRetryMax)
				}
			}
			checkJoined(t, n)
		})
	}
}

// TestSessionInUseDelay checks each AccountInUse retry delay is capped, even
// once the bot's backoff has grown past the cap.
func TestSessionInUseDelay(t *testing.T) {
	clock := newFakeClock(t0)
	b := &Bot{cfg: SessionConfig{Persistent: true, Clock: clock}, rng: rand.New(rand.NewPCG(1, 2)), log: discardLog(), retry: backoff{base: retryBase, max: retryMax, n: 20}}
	// Five retries of at most inUseRetryMax stay inside the window.
	for i := range 5 {
		d, err := b.after(false, 0, errInUse)
		if err != nil {
			t.Fatalf("AccountInUse %d: after = %v, want a retry", i, err)
		}
		if d <= 0 || d > inUseRetryMax {
			t.Fatalf("AccountInUse %d: delay %v, want in (0, %v]", i, d, inUseRetryMax)
		}
		clock.Advance(d)
	}
}

func TestSessionDeadLineNoAbort(t *testing.T) {
	readErr := func(errno syscall.Errno) error {
		return fmt.Errorf("%w: %w", errConnLost, fmt.Errorf("read game frame: %w",
			&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", errno)}))
	}
	tests := []struct {
		name string
		err  error
		// wantAborts is the abort count after maxAborts such sessions.
		wantAborts int
		wantStop   bool
	}{
		{name: "dead line (keepalive or user timeout)", err: readErr(syscall.ETIMEDOUT)},
		{name: "dead line on a send", err: fmt.Errorf("send game packet: %w", &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.ETIMEDOUT)})},
		{name: "dead line after an ICMP host unreachable", err: readErr(syscall.EHOSTUNREACH)},
		{name: "dead line after an ICMP net unreachable", err: readErr(syscall.ENETUNREACH)},
		{name: "reset", err: readErr(syscall.ECONNRESET), wantAborts: maxAborts, wantStop: true},
		{name: "EOF", err: fmt.Errorf("%w: %w", errConnLost, fmt.Errorf("read game frame: %w", io.EOF)), wantAborts: maxAborts, wantStop: true},
		{name: "kick", err: errServerClose},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Bot{cfg: SessionConfig{Persistent: true}, rng: rand.New(rand.NewPCG(1, 2)), log: discardLog(), retry: backoff{base: retryBase, max: retryMax}}
			var err error
			for range maxAborts {
				_, err = b.after(true, time.Minute, tt.err)
			}
			if b.aborts != tt.wantAborts {
				t.Errorf("aborts = %d, want %d", b.aborts, tt.wantAborts)
			}
			if stopped := errors.Is(err, ErrRepeatedAborts); stopped != tt.wantStop {
				t.Errorf("after = %v, want stop %v", err, tt.wantStop)
			}
		})
	}
}
