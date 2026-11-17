package botregistry

import (
	"errors"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// endIdle ends hz1's run so that it waits with no live run: cooling down
// after repeated aborts, or halted by the credential breaker.
func endIdle(t *testing.T, h *harness, halted bool) {
	t.Helper()
	_, r := h.create(t, "town", nil)
	if halted {
		h.gate.Halt(errors.New("refused credential"))
		r.end <- h.gate.Err()
	} else {
		r.end <- botclient.ErrRepeatedAborts
	}
	h.settle(t, "hz1", RunActive)
}

// wakeIdle passes the event that would restart an idle bot: the end of its
// cool-down, or a "resume logins".
func wakeIdle(t *testing.T, h *harness, halted bool) {
	t.Helper()
	if !halted {
		h.clock.Advance(CoolDown)
		return
	}
	if err := h.sup.ResumeLogins(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorIdleBotDespawn(t *testing.T) {
	for _, halted := range []bool{false, true} {
		t.Run(map[bool]string{false: "cooling down", true: "halted"}[halted], func(t *testing.T) {
			h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 3)
			endIdle(t, h, halted)
			if err := h.sup.Despawn(t.Context(), "hz1"); err != nil {
				t.Fatal(err)
			}
			if s := h.list(t)["hz1"]; s.Run != RunNone || s.Online || s.StopReason != reasonDespawned {
				t.Fatalf("after Despawn: run %s online %v reason %q", s.Run, s.Online, s.StopReason)
			}
			wakeIdle(t, h, halted)
			h.noRun(t)
			h.stop(t)
		})
	}
}

func TestSupervisorIdleBotRespawn(t *testing.T) {
	for _, halted := range []bool{false, true} {
		t.Run(map[bool]string{false: "cooling down", true: "halted"}[halted], func(t *testing.T) {
			h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 3)
			endIdle(t, h, halted)
			if err := h.sup.Respawn(t.Context(), "hz1"); err != nil {
				t.Fatal(err)
			}
			r := h.next(t)
			if r.Account != "hz1" {
				t.Fatalf("respawned %s, want hz1", r.Account)
			}
			if s := h.list(t)["hz1"]; s.Run != RunActive || !s.Online {
				t.Fatalf("after Respawn: run %s online %v", s.Run, s.Online)
			}
			// The old cool-down or halt is gone: no second run.
			wakeIdle(t, h, halted)
			h.noRun(t)
			h.stop(t, r)
		})
	}
}

// A bot whose restart cannot start is offline with no run state left, so
// Delete takes it.
func TestSupervisorDeleteAfterFailedRestart(t *testing.T) {
	h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 3)
	endIdle(t, h, true)
	// The supervisor is idle: the List in settle ordered this write after
	// its last registry access, and ResumeLogins orders it before its next.
	if err := h.reg.Update("hz1", func(b *Bot) { b.Class = ptr(999) }); err != nil {
		t.Fatal(err)
	}
	if err := h.sup.ResumeLogins(t.Context()); err != nil {
		t.Fatal(err)
	}
	h.noRun(t)
	s := h.list(t)["hz1"]
	if s.Run != RunNone || s.Online || !strings.Contains(s.StopReason, "not a starting class") {
		t.Fatalf("after the failed restart: run %s online %v reason %q", s.Run, s.Online, s.StopReason)
	}
	if err := h.sup.Delete(t.Context(), "hz1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	h.stop(t)
}
