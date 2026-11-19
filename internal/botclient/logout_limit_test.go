package botclient

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"
	"testing/synctest"
	"time"
)

// TestBrainLogoutLimit runs the loop against a server that refuses the first
// tc.refusals Logouts (the character is in combat) and leaves the rest
// unanswered. The refusal is queued while its Logout goes out, and the clock
// moves only once the loop is parked (synctest.Wait), so no timer fires ahead
// of the refusal the loop has yet to handle.
func TestBrainLogoutLimit(t *testing.T) {
	tests := []struct {
		name     string
		refusals int
		// want is how long after the logout request the loop gives up.
		want        time.Duration
		wantLogouts int
	}{
		// Refused at 0s, 11s and 22s; the next retry would be at 33s.
		{"refused until the limit", math.MaxInt, shutdownGrace, 3},
		// Refused at 0s, retried at 11s and left unanswered: its timeout
		// ends the loop before the limit.
		{"retry unanswered", 1, logoutRefusalGrace + logoutRetry + defaultLogoutTimeout, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clock := newFakeClock(t0)
				events := NewEvents(64)
				logouts := 0
				server := senderFunc(func(p []byte) error {
					if p[0] == opGameLogout {
						if logouts++; logouts <= tc.refusals {
							events.push(ActionFailed{}, nil)
						}
					}
					return nil
				})
				b := NewBrain(newTestWorld(t), events, server, clock, BrainConfig{LogoutLimit: shutdownGrace, Rand: rand.New(rand.NewPCG(5, 6))})
				done, logout := make(chan struct{}), make(chan struct{})
				errc := make(chan error, 1)
				go func() { errc <- b.Run(done, logout) }()
				synctest.Wait() // parked before the request: the limit starts at t0
				close(logout)
				for range 1000 {
					synctest.Wait()
					select {
					case err := <-errc:
						if !errors.Is(err, errLogoutTimeout) {
							t.Fatalf("Run = %v, want %v", err, errLogoutTimeout)
						}
						if took := clock.Now().Sub(t0); took != tc.want {
							t.Fatalf("gave up after %s, want %s", took, tc.want)
						}
						if logouts != tc.wantLogouts {
							t.Fatalf("%d Logout frames, want %d", logouts, tc.wantLogouts)
						}
						return
					default:
						clock.advanceToNext()
					}
				}
				close(done)
				t.Fatalf("Run still running at %s", clock.Now().Sub(t0))
			})
		})
	}
}
