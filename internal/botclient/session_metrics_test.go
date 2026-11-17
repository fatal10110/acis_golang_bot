package botclient

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// startMetricsBot is a persistent startBot with a metrics registry: the bot counts into
// its own registry Bot and the gate reports the breaker.
func startMetricsBot(t *testing.T, n *fakeNet) (*Bot, *botmetrics.Registry, *fakeClock, func() error) {
	t.Helper()
	b, clock, ctx := startBot(t, true, n)
	reg := botmetrics.NewRegistry(RuleNames()...)
	b.brain.Metrics = reg.NewBot()
	b.cfg.Gate.ReportBreaker(reg)
	return b, reg, clock, func() error { return runBot(ctx, b, clock) }
}

func TestSessionMetrics(t *testing.T) {
	tests := []struct {
		name                     string
		logins                   []error
		games                    []gameScript
		sessions                 int
		wantReconnects, wantAbts uint64
	}{
		{"one session", nil, nil, 1, 0, 0},
		{"reconnect after an abort", nil, []gameScript{abortEnd}, 2, 1, 1},
		{"reconnect after a kick", nil, []gameScript{serverCloseEnd}, 2, 1, 0},
		{"two aborts", nil, []gameScript{abortEnd, abortEnd}, 3, 2, 2},
		// Every new attempt after the first counts, a login retry too.
		{"reconnects through refused dials", []error{errRefused, errRefused}, nil, 1, 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNet{logins: tt.logins, games: tt.games, cancelAt: tt.sessions}
			_, reg, clock, run := startMetricsBot(t, n)
			if err := run(); err != nil {
				t.Fatalf("Run = %v, want nil", err)
			}
			s := reg.Collect(clock.Now())
			if s.Reconnects != tt.wantReconnects || s.Aborts != tt.wantAbts || s.BreakerOpen {
				t.Errorf("reconnects, aborts, breaker = %d, %d, %v; want %d, %d, false",
					s.Reconnects, s.Aborts, s.BreakerOpen, tt.wantReconnects, tt.wantAbts)
			}
		})
	}
}

func TestSessionMetricsBreaker(t *testing.T) {
	n := &fakeNet{logins: []error{&LoginError{Packet: "LoginFail", Reason: loginFailPasswordWrong}}}
	_, reg, clock, run := startMetricsBot(t, n)
	if err := run(); err == nil {
		t.Fatal("Run = nil, want the refused credential")
	}
	s := reg.Collect(clock.Now())
	if !s.BreakerOpen || s.Reconnects != 0 || s.Aborts != 0 {
		t.Errorf("breaker, reconnects, aborts = %v, %d, %d; want true, 0, 0", s.BreakerOpen, s.Reconnects, s.Aborts)
	}
}

// TestGateReportBreaker checks the breaker is reported only when Halt trips
// it, and once.
func TestGateReportBreaker(t *testing.T) {
	reg := botmetrics.NewRegistry()
	g := NewLoginGate(newFakeClock(t0), rand.New(rand.NewPCG(1, 2)))
	g.ReportBreaker(reg)
	if reg.Collect(t0).BreakerOpen {
		t.Fatal("breaker open before Halt")
	}
	g.Halt(fmt.Errorf("account bot1: refused"))
	g.Halt(fmt.Errorf("account bot2: refused"))
	if !reg.Collect(t0).BreakerOpen {
		t.Fatal("breaker closed after Halt")
	}
}
