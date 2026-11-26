package botclient

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// stubConn is a connection whose reads and writes fail with the errors set;
// a nil writeErr accepts every write.
type stubConn struct{ readErr, writeErr error }

func (c stubConn) Read([]byte) (int, error) { return 0, c.readErr }
func (c stubConn) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return len(p), nil
}
func (stubConn) Close() error                     { return nil }
func (stubConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (stubConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (stubConn) SetDeadline(time.Time) error      { return nil }
func (stubConn) SetReadDeadline(time.Time) error  { return nil }
func (stubConn) SetWriteDeadline(time.Time) error { return nil }

func opErr(op string, errno syscall.Errno) error {
	return &net.OpError{Op: op, Net: "tcp", Err: os.NewSyscallError(op, errno)}
}

// TestClosedAtAccept checks which first-frame failures of a login and a game
// connection classify as closed at accept: EOF or a reset at "read Init",
// EOF or a reset at "read VersionCheck", a broken pipe or a reset on "send
// ProtocolVersion". A timeout or a partial frame is not one.
func TestClosedAtAccept(t *testing.T) {
	timeout := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
	tests := []struct {
		name   string
		game   bool
		conn   stubConn
		closed bool
	}{
		{"login EOF at read Init", false, stubConn{readErr: io.EOF}, true},
		{"login reset at read Init", false, stubConn{readErr: opErr("read", syscall.ECONNRESET)}, true},
		{"login timeout at read Init", false, stubConn{readErr: timeout}, false},
		{"login partial Init header", false, stubConn{readErr: io.ErrUnexpectedEOF}, false},
		{"game EOF at read VersionCheck", true, stubConn{readErr: io.EOF}, true},
		{"game reset at read VersionCheck", true, stubConn{readErr: opErr("read", syscall.ECONNRESET)}, true},
		{"game broken pipe on send ProtocolVersion", true, stubConn{writeErr: opErr("write", syscall.EPIPE)}, true},
		{"game reset on send ProtocolVersion", true, stubConn{writeErr: opErr("write", syscall.ECONNRESET)}, true},
		{"game timeout at read VersionCheck", true, stubConn{readErr: timeout}, false},
		{"game timeout on send ProtocolVersion", true, stubConn{writeErr: &net.OpError{Op: "write", Net: "tcp", Err: os.ErrDeadlineExceeded}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.game {
				_, err = newGameConn(t.Context(), tt.conn, ProtocolRevision, time.Second)
			} else {
				_, _, _, err = authenticate(t.Context(), tt.conn, "bot1", "not-a-secret", time.Second)
			}
			if err == nil {
				t.Fatal("no error")
			}
			if got := errors.Is(err, ErrClosedAtAccept); got != tt.closed {
				t.Fatalf("%v: closed at accept = %v, want %v", err, got, tt.closed)
			}
		})
	}
	if errors.Is(errRefused, ErrClosedAtAccept) {
		t.Error("a refused dial classifies as closed at accept")
	}
}

// errAcceptClosed is a login closed at accept, as Login reports it.
var errAcceptClosed = fmt.Errorf("login: read Init: %w", closedOnRead(io.EOF))

// TestLoginGateClosedAtAccept checks the quiet period on a spaced and an
// allowlisted gate: no dial for banQuiet after a dial closed at accept, then
// exactly one probe; a probe closed at accept starts another quiet period,
// and each is counted.
func TestLoginGateClosedAtAccept(t *testing.T) {
	tests := []struct {
		name string
		gate func(Clock, *rand.Rand) *LoginGate
	}{
		{"spaced", NewLoginGate},
		{"allowlisted", NewAllowlistedLoginGate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newFakeClock(t0)
			g := tt.gate(clock, rand.New(rand.NewPCG(1, 2)))
			reg := botmetrics.NewRegistry()
			g.ReportBreaker(reg)
			turn, err := g.Take(t.Context(), "bot1")
			if err != nil {
				t.Fatal(err)
			}
			if err := turn.Dial(t.Context()); err != nil {
				t.Fatal(err)
			}
			for probe := range 2 {
				start := clock.Now()
				turn.Dialed(errAcceptClosed)
				done := make(chan error, 1)
				go func() { done <- turn.Dial(t.Context()) }()
				select {
				case <-clock.armed:
				case err := <-done:
					t.Fatalf("quiet %d: dial at once (%v), want none before %s", probe, err, banQuiet)
				}
				clock.Advance(banQuiet - time.Millisecond)
				select {
				case err := <-done:
					t.Fatalf("quiet %d: dial at +%s (%v), want none before %s", probe, clock.Now().Sub(start), err, banQuiet)
				default:
				}
				clock.Advance(time.Millisecond)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if got := clock.Now().Sub(start); got != banQuiet {
					t.Fatalf("quiet %d: probe at +%s, want +%s", probe, got, banQuiet)
				}
			}
			if got := reg.Collect(clock.Now()).AcceptClosed; got != 2 {
				t.Errorf("login_accept_closed_total = %d, want 2", got)
			}
			// A probe that connects ends the quiet: the next dial waits
			// only the spacing.
			turn.Dialed(nil)
			if wait := g.next.Sub(clock.Now()); wait >= banQuiet || wait > dialSpacing+dialJitter {
				t.Errorf("after a probe that connected the next dial is in %s, want the spacing", wait)
			}
			turn.Done()
		})
	}
}

// TestLoginGateSpacedAfterClosedAtAccept checks that an allowlisted gate
// spaces its dials for good once it saw a dial closed at accept, and that a
// spacing set above the floor is kept.
func TestLoginGateSpacedAfterClosedAtAccept(t *testing.T) {
	clock := newFakeClock(t0)
	g := NewAllowlistedLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	if err := g.SetSpacing(0); err != nil {
		t.Fatalf("SetSpacing(0) on an allowlisted gate = %v", err)
	}
	turn, err := g.Take(t.Context(), "bot1")
	if err != nil {
		t.Fatal(err)
	}
	defer turn.Done()
	turn.Dialed(errAcceptClosed)
	if got := g.MinSpacing(); got != dialSpacing {
		t.Errorf("MinSpacing = %s, want %s", got, dialSpacing)
	}
	if got := g.Spacing(); got != dialSpacing {
		t.Errorf("Spacing = %s, want %s", got, dialSpacing)
	}
	if err := g.SetSpacing(dialSpacing - time.Millisecond); !errors.Is(err, ErrSpacing) {
		t.Errorf("SetSpacing under the floor = %v, want %v", err, ErrSpacing)
	}
	clock.Advance(banQuiet)
	var last time.Time
	for i := range 4 {
		if err := turn.Dial(t.Context()); err != nil {
			t.Fatal(err)
		}
		now := clock.Now()
		if i > 0 {
			if gap := now.Sub(last); gap < dialSpacing || gap >= dialSpacing+dialJitter {
				t.Errorf("dials %d and %d %s apart, want in [%s, %s)", i-1, i, gap, dialSpacing, dialSpacing+dialJitter)
			}
		}
		last = now
		turn.Dialed(nil)
		clock.Advance(g.next.Sub(now))
	}

	// A higher spacing stays.
	g2 := NewAllowlistedLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	if err := g2.SetSpacing(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	turn2, err := g2.Take(t.Context(), "bot1")
	if err != nil {
		t.Fatal(err)
	}
	defer turn2.Done()
	turn2.Dialed(errAcceptClosed)
	if got := g2.Spacing(); got != 5*time.Second {
		t.Errorf("Spacing after a close at accept = %s, want the 5s set", got)
	}
}
