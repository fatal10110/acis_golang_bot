package botclient

import (
	"errors"
	"io"
	"testing"
	"time"
)

// closeConn is a stubConn that counts its closes and may panic on Write.
type closeConn struct {
	stubConn
	panicWrite bool
	closes     *int
}

func (c closeConn) Write(p []byte) (int, error) {
	if c.panicWrite {
		panic("handshake write")
	}
	return c.stubConn.Write(p)
}

func (c closeConn) Close() error { *c.closes++; return nil }

// TestNewGameConnClosesOnFailure checks that a handshake that fails or
// panics closes the connection exactly once, so a panic recovered by the
// session never leaves the socket open.
func TestNewGameConnClosesOnFailure(t *testing.T) {
	tests := []struct {
		name       string
		panicWrite bool
	}{
		{"error at read VersionCheck", false},
		{"panic on send ProtocolVersion", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var closes int
			conn := closeConn{stubConn: stubConn{readErr: io.EOF}, panicWrite: tt.panicWrite, closes: &closes}
			var err error
			panicked := func() (p bool) {
				defer func() { p = recover() != nil }()
				_, err = newGameConn(t.Context(), conn, ProtocolRevision, time.Second)
				return false
			}()
			if panicked != tt.panicWrite {
				t.Fatalf("panicked = %v, want %v", panicked, tt.panicWrite)
			}
			if !tt.panicWrite && !errors.Is(err, io.EOF) {
				t.Fatalf("err = %v, want EOF", err)
			}
			if closes != 1 {
				t.Fatalf("closes = %d, want 1", closes)
			}
		})
	}
}
