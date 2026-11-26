package botclient

import (
	"math/rand/v2"
	"testing"
	"time"
)

// TestLoginGateFleetBackoff runs turns of several bots through one gate and
// checks the fleet backoff after each: refused dials on either port and
// not-ready endings of two different bots in a row step it, only an entry
// resets it, and AccountInUse does neither.
func TestLoginGateFleetBackoff(t *testing.T) {
	playFail := &LoginError{Packet: "PlayFail", Reason: 0x0f}
	accessFailed := &LoginError{Packet: "LoginFail", Reason: loginFailAccessFailed}
	type turnEnd struct {
		who string
		// errs are the outcomes of the turn's dials, in order.
		errs    []error
		entered bool
		steps   int // the backoff's steps after the turn
	}
	tests := []struct {
		name  string
		turns []turnEnd
	}{
		{"PlayFail of a 3-bot fleet doubles to the cap, an entry resets", []turnEnd{
			{who: "a", errs: []error{playFail}, steps: 0},
			{who: "b", errs: []error{playFail}, steps: 1},
			{who: "c", errs: []error{playFail}, steps: 2},
			{who: "a", errs: []error{playFail}, steps: 3},
			{who: "b", errs: []error{playFail}, steps: 4},
			{who: "c", errs: []error{playFail}, steps: 5},
			{who: "a", errs: []error{playFail}, steps: 6},
			{who: "b", errs: []error{playFail}, steps: 7},
			{who: "c", errs: []error{nil, nil}, entered: true, steps: 0},
			{who: "a", errs: []error{playFail}, steps: 0},
			{who: "b", errs: []error{playFail}, steps: 1},
		}},
		{"login accepted, game dial refused", []turnEnd{
			{who: "a", errs: []error{nil, errRefused}, steps: 1},
			{who: "b", errs: []error{nil, errRefused}, steps: 2},
			{who: "c", errs: []error{nil, errRefused}, steps: 3},
			{who: "a", errs: []error{nil, nil}, entered: true, steps: 0},
		}},
		{"one always-failing bot beside two that enter never steps", []turnEnd{
			{who: "x", errs: []error{accessFailed}, steps: 0},
			{who: "a", errs: []error{nil, nil}, entered: true, steps: 0},
			{who: "x", errs: []error{accessFailed}, steps: 0},
			{who: "b", errs: []error{nil, nil}, entered: true, steps: 0},
			{who: "x", errs: []error{accessFailed}, steps: 0},
			{who: "x", errs: []error{accessFailed}, steps: 0},
		}},
		{"two always-failing bots step", []turnEnd{
			{who: "x", errs: []error{accessFailed}, steps: 0},
			{who: "y", errs: []error{accessFailed}, steps: 1},
			{who: "x", errs: []error{accessFailed}, steps: 2},
			{who: "y", errs: []error{accessFailed}, steps: 3},
		}},
		{"every system LoginFail is not ready", []turnEnd{
			{who: "a", errs: []error{&LoginError{Packet: "LoginFail", Reason: loginFailSystemError}}, steps: 0},
			{who: "b", errs: []error{&LoginError{Packet: "LoginFail", Reason: loginFailServerOverloaded}}, steps: 1},
			{who: "c", errs: []error{&LoginError{Packet: "LoginFail", Reason: loginFailMaintenance}}, steps: 2},
			{who: "d", errs: []error{accessFailed}, steps: 3},
			{who: "e", errs: []error{playFail}, steps: 4},
		}},
		{"AccountInUse neither steps nor resets", []turnEnd{
			{who: "a", errs: []error{nil, errRefused}, steps: 1},
			{who: "a", errs: []error{errInUse}, steps: 1},
			{who: "b", errs: []error{errInUse}, steps: 1},
		}},
		{"a refused credential is no not-ready ending", []turnEnd{
			{who: "a", errs: []error{&LoginError{Packet: "LoginFail", Reason: loginFailUserOrPassWrong}}, steps: 0},
			{who: "b", errs: []error{&LoginError{Packet: "LoginFail", Reason: loginFailPasswordWrong}}, steps: 0},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unspaced, so a turn's second dial needs no clock move.
			clock := newFakeClock(t0)
			g := NewAllowlistedLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
			for i, te := range tt.turns {
				turn, err := g.Take(t.Context(), te.who)
				if err != nil {
					t.Fatal(err)
				}
				before := g.refused.n
				for _, err := range te.errs {
					if err := turn.Dial(t.Context()); err != nil {
						t.Fatal(err)
					}
					turn.Dialed(err)
				}
				if te.entered {
					turn.Entered()
				}
				turn.Done()
				if g.refused.n != te.steps {
					t.Fatalf("turn %d (%s): %d steps, want %d", i, te.who, g.refused.n, te.steps)
				}
				if te.steps > before {
					// A step pushes the next dial of every bot out by its
					// window: base doubled per earlier step, capped.
					d := min(refusedBase<<(te.steps-1), refusedMax)
					if wait := g.next.Sub(clock.Now()); wait < d/2 || wait >= d {
						t.Fatalf("turn %d (%s): next dial in %s, want in [%s, %s)", i, te.who, wait, d/2, d)
					}
				}
				clock.Advance(max(g.next.Sub(clock.Now()), 0))
			}
		})
	}
}

// TestLoginGateFleetBackoffCap checks the not-ready backoff tops out at
// refusedMax, the most a healthy bot waits behind failing ones.
func TestLoginGateFleetBackoffCap(t *testing.T) {
	clock := newFakeClock(t0)
	g := NewLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	playFail := &LoginError{Packet: "PlayFail", Reason: 0x0f}
	var most time.Duration
	for i := range 20 {
		turn, err := g.Take(t.Context(), string(rune('a'+i%3)))
		if err != nil {
			t.Fatal(err)
		}
		if err := turn.Dial(t.Context()); err != nil {
			t.Fatal(err)
		}
		turn.Dialed(playFail)
		turn.Done()
		most = max(most, g.next.Sub(clock.Now()))
		clock.Advance(g.next.Sub(clock.Now()))
	}
	if most < refusedMax/2 || most >= refusedMax {
		t.Errorf("longest wait %s, want in [%s, %s)", most, refusedMax/2, refusedMax)
	}
}
