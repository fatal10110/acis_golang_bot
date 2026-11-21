package botregistry

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// hold blocks the supervisor goroutine in a request until release is called.
func (h *harness) hold(t *testing.T) (release func()) {
	t.Helper()
	entered, done := make(chan struct{}), make(chan struct{})
	go func() { _ = h.sup.do(t.Context(), func() { close(entered); <-done }) }()
	<-entered
	return func() { close(done) }
}

// writes is the number of registry files written, read on the supervisor
// goroutine.
func (h *harness) writes(t *testing.T) int {
	t.Helper()
	var n int
	if err := h.sup.do(t.Context(), func() { n = h.reg.writes }); err != nil {
		t.Fatal(err)
	}
	return n
}

// await waits until cond, read on the supervisor goroutine, holds.
func (h *harness) await(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for range 10000 {
		var ok bool
		if err := h.sup.do(t.Context(), func() { ok = cond() }); err != nil {
			t.Fatal(err)
		}
		if ok {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("never %s", what)
}

// TestSupervisorLearnedNeverWaits: a run reports its character while it
// holds the login gate turn, and the supervisor is busy: the report returns
// at once, the next bot takes the turn, and the name is saved once the
// supervisor is free.
func TestSupervisorLearnedNeverWaits(t *testing.T) {
	dir := t.TempDir()
	h := startSupervisor(t, openPanel(t, dir), newClock(), 3)
	b, r := h.create(t, "town", nil)

	release := h.hold(t)
	turn, err := h.gate.Take(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	r.Learned("Pinned", 0) // a blocking report deadlocks here
	turn.Done()
	next, err := h.gate.Take(t.Context())
	if err != nil {
		t.Fatalf("second bot's turn: %v", err)
	}
	next.Done()
	release()

	h.await(t, "saved the learned name", func() bool {
		got, _ := h.reg.Bot(b.Account)
		return got.Name == "Pinned" && !h.reg.Dirty()
	})
	if d := diskBots(t, dir)[b.Account]; d.Name != "Pinned" {
		t.Fatalf("bot on disk %+v: want the learned name", d)
	}
	h.stop(t, r)
}

// TestSupervisorLearnedSavedBeforeTheEnd: a run that ends right after it
// reported its character has the name saved before its end is applied: an
// account holding characters is then the bot's own, not one to drop.
func TestSupervisorLearnedSavedBeforeTheEnd(t *testing.T) {
	dir := t.TempDir()
	h := startSupervisor(t, openPanel(t, dir), newClock(), 3)
	b, r := h.create(t, "town", nil)

	release := h.hold(t)
	r.Learned("Pinned", 0)
	r.end <- botclient.ErrAccountHasCharacters
	release()

	s := h.settle(t, b.Account, RunActive)
	if s.Name != "Pinned" || s.Online || s.StopReason != botclient.ErrAccountHasCharacters.Error() {
		t.Fatalf("bot %+v: want Pinned stopped on its end, not dropped", s.Bot)
	}
	h.noRun(t)
	if d := diskBots(t, dir)[b.Account]; d.Name != "Pinned" || d.Class == nil || *d.Class != 0 {
		t.Fatalf("bot on disk %+v: want the learned name and class", d)
	}
	h.stop(t)
}

// TestSupervisorStopAppliesUndeliveredReports: reports still in the mailbox
// when the supervisor stops are applied, every run is joined, and the file
// holds them.
func TestSupervisorStopAppliesUndeliveredReports(t *testing.T) {
	dir := t.TempDir()
	h := startSupervisor(t, openPanel(t, dir), newClock(), 3)
	_, r1 := h.create(t, "town", nil)
	_, r2 := h.create(t, "town", nil)

	release := h.hold(t)
	r1.Learned("PinnedOne", 0)
	r2.Learned("PinnedTwo", 0)
	h.cancel()
	release()
	h.stop(t, r1, r2)

	bots := diskBots(t, dir)
	for account, name := range map[string]string{"hz1": "PinnedOne", "hz2": "PinnedTwo"} {
		if b := bots[account]; b.Name != name || !b.Online {
			t.Fatalf("bot %s on disk %+v: want %s, online", account, b, name)
		}
	}
}

// TestSupervisorRunEndsWriteOnce: run ends apply in memory; the flush timer
// writes them all in one save.
func TestSupervisorRunEndsWriteOnce(t *testing.T) {
	dir := t.TempDir()
	clock := newClock()
	h := startSupervisor(t, openPanel(t, dir), clock, 5)
	var runs []*fakeRun
	for range 3 {
		_, r := h.create(t, "town", nil)
		runs = append(runs, r)
	}
	w := h.writes(t)
	boom := errors.New("boom")
	for _, r := range runs {
		r.end <- boom
		h.settle(t, r.Account, RunActive)
	}
	if n := h.writes(t); n != w {
		t.Fatalf("%d writes for 3 run ends before the flush, want none", n-w)
	}
	for _, r := range runs {
		if b := diskBots(t, dir)[r.Account]; !b.Online || b.StopReason != "" {
			t.Fatalf("bot %s on disk %+v before the flush", r.Account, b)
		}
	}
	clock.Advance(FlushDelay - time.Nanosecond)
	if n := h.writes(t); n != w {
		t.Fatalf("%d writes before the flush delay passed, want none", n-w)
	}
	clock.Advance(time.Nanosecond)
	h.await(t, "flushed", func() bool { return h.reg.writes == w+1 })
	for _, r := range runs {
		if b := diskBots(t, dir)[r.Account]; b.Online || b.StopReason != "boom" || !b.StoppedAt.Equal(t0) {
			t.Fatalf("bot %s on disk %+v after the flush: want stopped on boom", r.Account, b)
		}
	}
	h.stop(t)
	if h.reg.writes != w+1 {
		t.Fatalf("%d writes in all, want 1: the stop has nothing left to write", h.reg.writes-w)
	}
}

// TestSupervisorSaveWritesDeferredEnds: a change saved at once (a new bot)
// writes the deferred run ends with it.
func TestSupervisorSaveWritesDeferredEnds(t *testing.T) {
	dir := t.TempDir()
	h := startSupervisor(t, openPanel(t, dir), newClock(), 5)
	b, r := h.create(t, "town", nil)
	r.end <- errors.New("boom")
	h.settle(t, b.Account, RunActive)
	w := h.writes(t)
	_, r2 := h.create(t, "town", nil)
	if n := h.writes(t); n != w+1 {
		t.Fatalf("%d writes for a new bot, want 1", n-w)
	}
	if d := diskBots(t, dir)[b.Account]; d.Online || d.StopReason != "boom" {
		t.Fatalf("bot on disk %+v: want its end written with the new bot", d)
	}
	h.stop(t, r2)
}

// TestSupervisorShutdownWritesOnce: a restore over the cap, a run end and the
// stop of every live run cost one write, after the last report, and the file
// holds each bot's online flag, stop record, level and position.
func TestSupervisorShutdownWritesOnce(t *testing.T) {
	dir := t.TempDir()
	reg := openPanel(t, dir)
	for range 4 {
		if _, err := reg.NewBot(Bot{Class: ptr(0), Preset: "town", Online: true, Level: 20, Position: &Position{X: 1, Y: 2, Z: 3}}); err != nil {
			t.Fatal(err)
		}
	}
	w := reg.writes
	h := startSupervisor(t, reg, newClock(), 3)
	runs := map[string]*fakeRun{}
	for range 3 {
		r := h.next(t)
		runs[r.Account] = r
		liveBot(t, h, r)
	}
	runs["hz1"].end <- errors.New("boom")
	h.settle(t, "hz1", RunActive)
	h.stop(t, runs["hz2"], runs["hz3"])
	if reg.writes != w+1 {
		t.Fatalf("%d writes, want 1 at the stop", reg.writes-w)
	}
	want := map[string]struct {
		online bool
		reason string
	}{
		"hz1": {false, "boom"},
		"hz2": {true, ""},
		"hz3": {true, ""},
		"hz4": {false, reasonOverCap},
	}
	for account, b := range diskBots(t, dir) {
		ww := want[account]
		if b.Online != ww.online || b.StopReason != ww.reason || b.Level != 20 || b.Position == nil || *b.Position != (Position{X: 1, Y: 2, Z: 3}) {
			t.Fatalf("bot %s on disk %+v: want online %v reason %q, level and position kept", account, b, ww.online, ww.reason)
		}
	}
}
