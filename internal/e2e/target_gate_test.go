package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// errAcceptClosed is a login the server closed at accept.
var errAcceptClosed = fmt.Errorf("read Init: %w: %w", botclient.ErrClosedAtAccept, io.EOF)

// TestTargetClosedAtAccept checks a login or a credential check closed at
// accept quiets the run's gate, allowlisted or not: the next login waits,
// and an allowlisted gate spaces its dials from then on.
func TestTargetClosedAtAccept(t *testing.T) {
	tests := []struct {
		name  string
		first func(*Target) error
		n     *fakeNet
	}{
		{"login", func(tg *Target) error {
			_, err := tg.Enter(context.Background(), "e2ea1", "pw", botclient.Character{Name: "e2eBot"}, nil)
			return err
		}, &fakeNet{loginErr: errAcceptClosed}},
		{"check", func(tg *Target) error { return tg.CheckCredentials(context.Background(), "e2ea1", "pw") }, &fakeNet{checkErr: errAcceptClosed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg, clock := fakeTarget(t, Config{Allowlisted: true}, tt.n)
			if err := tt.first(tg); !errors.Is(err, botclient.ErrClosedAtAccept) {
				t.Fatalf("first login = %v, want closed at accept", err)
			}
			if got := tg.gate.MinSpacing(); got < time.Second {
				t.Errorf("allowlisted gate spacing floor %s after a close at accept, want it spaced", got)
			}
			waitsForNextLogin(t, tg, clock, "e2ea2")
		})
	}
}

// TestTargetNotReady checks the run's logins carry their account to the
// gate: PlayFail of one account alone does not step the fleet backoff, of a
// second account does.
func TestTargetNotReady(t *testing.T) {
	n := &fakeNet{loginErr: &botclient.LoginError{Packet: "PlayFail", Reason: 0x0f}}
	tg, clock := fakeTarget(t, Config{Allowlisted: true}, n)
	ch := botclient.Character{Name: "e2eBot"}
	for range 3 {
		if _, err := tg.Enter(t.Context(), "e2ea1", "pw", ch, nil); !errors.As(err, new(*botclient.LoginError)) {
			t.Fatalf("Enter = %v, want PlayFail", err)
		}
	}
	if len(clock.parked) != 0 {
		t.Fatal("one account's PlayFail made the gate wait")
	}
	if _, err := tg.Enter(t.Context(), "e2ea2", "pw", ch, nil); !errors.As(err, new(*botclient.LoginError)) {
		t.Fatalf("Enter = %v, want PlayFail", err)
	}
	waitsForNextLogin(t, tg, clock, "e2ea3")
}

// waitsForNextLogin fails unless a credential check of account parks on
// the gate.
func waitsForNextLogin(t *testing.T, tg *Target, clock *parkClock, account string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- tg.CheckCredentials(ctx, account, "pw") }()
	select {
	case <-clock.parked:
	case err := <-done:
		cancel()
		t.Fatalf("next login went out at once (%v), want it to wait", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting login = %v, want canceled", err)
	}
}
