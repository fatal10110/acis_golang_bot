package botclient

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSessionAfterClosedAtAccept checks a bot adds no delay of its own after
// a dial closed at accept: the gate's quiet period holds every bot.
func TestSessionAfterClosedAtAccept(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("login: read Init: %w", closedOnRead(io.EOF)),
		fmt.Errorf("read VersionCheck: %w", closedOnRead(io.EOF)),
		fmt.Errorf("send ProtocolVersion: %w", closedOnWrite(opErr("write", syscall.EPIPE))),
	} {
		b, _, _ := startBot(t, true, &fakeNet{})
		delay, stop := b.after(false, 0, err)
		if delay != 0 || stop != nil || b.retry.n != 0 {
			t.Errorf("after(%v) = %s, %v (backoff step %d), want no delay", err, delay, stop, b.retry.n)
		}
	}
}

// TestSessionClosedAtAcceptQuiet runs a bot whose dials are closed at accept
// on either port: each dial waits out banQuiet after the previous one, and
// each logs one line naming the port.
func TestSessionClosedAtAcceptQuiet(t *testing.T) {
	loginClosed := fmt.Errorf("read Init: %w", closedOnRead(io.EOF))
	gameClosed := fmt.Errorf("read VersionCheck: %w", closedOnRead(io.EOF))
	tests := []struct {
		name   string
		logins []error
		dials  []error
		ports  []string // the port of each line logged
	}{
		{"login port", []error{loginClosed, loginClosed}, nil, []string{"2106", "2106"}},
		{"game port", nil, []error{gameClosed}, []string{"7777"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNet{logins: tt.logins, dials: tt.dials, cancelAt: 1}
			b, clock, ctx := startBot(t, true, n)
			b.cfg.LoginAddr = "127.0.0.1:2106"
			var buf bytes.Buffer
			b.log = slog.New(slog.NewTextHandler(&buf, nil))
			var dials []time.Time
			n.onLogin = func() { dials = append(dials, clock.Now()) }
			if err := runBot(ctx, b, clock); err != nil {
				t.Fatalf("Run = %v", err)
			}
			if len(tt.dials) > 0 {
				// The game dial closed at accept follows the first login.
				if len(dials) != 2 || dials[1].Sub(dials[0]) < banQuiet {
					t.Errorf("logins at %v, want the second %s after the first", dials, banQuiet)
				}
			} else {
				for i := 1; i < len(dials); i++ {
					if gap := dials[i].Sub(dials[i-1]); gap != banQuiet {
						t.Errorf("logins %d and %d %s apart, want %s", i-1, i, gap, banQuiet)
					}
				}
			}
			var ports []string
			for line := range strings.Lines(buf.String()) {
				if !strings.Contains(line, "closed at accept: the address is banned or filtered, or the server failed") {
					continue
				}
				_, port, _ := strings.Cut(line, " port=")
				port, _, _ = strings.Cut(port, " ")
				ports = append(ports, port)
			}
			if fmt.Sprint(ports) != fmt.Sprint(tt.ports) {
				t.Errorf("logged ports %v, want %v", ports, tt.ports)
			}
		})
	}
}

// TestSessionEntryResetsGate checks a bot's entry into the world resets the
// fleet backoff its refused dials stepped.
func TestSessionEntryResetsGate(t *testing.T) {
	n := &fakeNet{logins: []error{errRefused, errRefused}, cancelAt: 1}
	b, clock, ctx := startBot(t, true, n)
	if err := runBot(ctx, b, clock); err != nil {
		t.Fatalf("Run = %v", err)
	}
	if got := b.cfg.Gate.refused.n; n.entered != 1 || got != 0 {
		t.Errorf("entered %d, backoff step %d; want 1, 0", n.entered, got)
	}
}

// TestSessionInUseAfterRefusedGameDial checks a refused game dial, which
// leaves the bot's session on the login server for its LoginTimeout, does
// not stop the bot: the AccountInUse that follows is retried.
func TestSessionInUseAfterRefusedGameDial(t *testing.T) {
	n := &fakeNet{logins: []error{nil, errInUse}, dials: []error{errRefused}, cancelAt: 1}
	b, clock, ctx := startBot(t, true, n)
	if err := runBot(ctx, b, clock); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if n.nLogin != 3 || n.entered != 1 {
		t.Errorf("logins, sessions = %d, %d; want 3, 1", n.nLogin, n.entered)
	}
}
