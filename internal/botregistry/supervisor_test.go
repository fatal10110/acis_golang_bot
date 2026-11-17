package botregistry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// fakeClock is a botclient.Clock that moves only when a test advances it.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) botclient.Timer {
	t := &fakeTimer{clock: c, ch: make(chan time.Time, 1)}
	c.mu.Lock()
	c.timers = append(c.timers, t)
	c.mu.Unlock()
	t.Reset(d)
	return t
}

// Advance moves the time on by d and fires every timer then due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.fireLocked()
}

func (c *fakeClock) fireLocked() {
	for _, t := range c.timers {
		if t.active && !t.when.After(c.now) {
			t.active = false
			t.ch <- c.now
		}
	}
}

type fakeTimer struct {
	clock  *fakeClock
	ch     chan time.Time
	when   time.Time
	active bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Reset(d time.Duration) {
	c := t.clock
	c.mu.Lock()
	defer c.mu.Unlock()
	t.drainLocked()
	t.when, t.active = c.now.Add(d), true
	c.fireLocked()
}

func (t *fakeTimer) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	t.active = false
	t.drainLocked()
}

func (t *fakeTimer) drainLocked() {
	select {
	case <-t.ch:
	default:
	}
}

// fakeRun is one run the supervisor started: the test ends it.
type fakeRun struct {
	Run
	ctx context.Context
	end chan error
}

// harness is a running Supervisor over a fake runner and clock.
type harness struct {
	sup     *Supervisor
	reg     *Registry
	clock   *fakeClock
	gate    *botclient.LoginGate
	metrics *botmetrics.Registry
	runs    chan *fakeRun
	cancel  context.CancelFunc
	done    chan error
}

func startSupervisor(t *testing.T, reg *Registry, clock *fakeClock, maxOnline int) *harness {
	t.Helper()
	h := &harness{
		reg:     reg,
		clock:   clock,
		gate:    botclient.NewLoginGate(clock, rand.New(rand.NewPCG(3, 4))),
		metrics: botmetrics.NewRegistry(),
		runs:    make(chan *fakeRun),
		done:    make(chan error, 1),
	}
	sup, err := New(Config{
		Registry:  reg,
		Run:       h.run,
		Gate:      h.gate,
		Clock:     clock,
		MaxOnline: maxOnline,
		Metrics:   h.metrics,
		CheckPreset: func(p string) error {
			if !slices.Contains([]string{"support", "town", "merchant"}, p) {
				return fmt.Errorf("preset %q unknown", p)
			}
			return nil
		},
		Rand: rand.New(rand.NewPCG(1, 2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.sup = sup
	ctx, cancel := context.WithCancel(t.Context())
	h.cancel = cancel
	go func() { h.done <- sup.Run(ctx) }()
	return h
}

func (h *harness) run(ctx context.Context, r Run) error {
	fr := &fakeRun{Run: r, ctx: ctx, end: make(chan error)}
	h.runs <- fr
	return <-fr.end
}

// next is the next run the supervisor starts.
func (h *harness) next(t *testing.T) *fakeRun {
	t.Helper()
	select {
	case r := <-h.runs:
		return r
	case err := <-h.done:
		t.Fatalf("supervisor returned %v while a run was awaited", err)
	}
	return nil
}

// noRun fails when a run is starting; a run starts only on the supervisor
// goroutine, so a List round trip first lets one under way begin.
func (h *harness) noRun(t *testing.T) {
	t.Helper()
	h.list(t)
	runtime.Gosched()
	select {
	case r := <-h.runs:
		t.Fatalf("unexpected run of %s", r.Account)
	default:
	}
}

// stop cancels the supervisor, logs out each live run once it is canceled,
// and waits for Run to return.
func (h *harness) stop(t *testing.T, live ...*fakeRun) {
	t.Helper()
	h.cancel()
	for _, r := range live {
		<-r.ctx.Done()
		r.end <- nil
	}
	if err := <-h.done; err != nil {
		t.Fatalf("Run = %v", err)
	}
}

func (h *harness) list(t *testing.T) map[string]Status {
	t.Helper()
	l, err := h.sup.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]Status, len(l))
	for _, s := range l {
		m[s.Account] = s
	}
	return m
}

// settle waits until account's run left state from: its report was handled.
func (h *harness) settle(t *testing.T, account string, from RunState) Status {
	t.Helper()
	for range 10000 {
		if s := h.list(t)[account]; s.Run != from {
			return s
		}
		runtime.Gosched()
	}
	t.Fatalf("bot %s stays %s", account, from)
	return Status{}
}

func (h *harness) create(t *testing.T, preset string, class *int32) (Bot, *fakeRun) {
	t.Helper()
	b, err := h.sup.Create(t.Context(), preset, class)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	r := h.next(t)
	if r.Account != b.Account {
		t.Fatalf("run of %s started for new bot %s", r.Account, b.Account)
	}
	return b, r
}

func derivedNames(t *testing.T, m botnames.Marker, account string) []string {
	t.Helper()
	gen, err := botnames.New(m, botnames.AccountSource(account))
	if err != nil {
		t.Fatal(err)
	}
	return gen.Names(botclient.MaxNameCandidates)
}

func newClock() *fakeClock { return &fakeClock{now: t0} }

func ptr(v int32) *int32 { return &v }

func TestSupervisorCreate(t *testing.T) {
	dir := t.TempDir()
	h := startSupervisor(t, openPanel(t, dir), newClock(), 5)

	b1, r1 := h.create(t, "town", nil)
	if b1.Account != "hz1" || b1.Class == nil {
		t.Fatalf("new bot %+v: want hz1 with a class", b1)
	}
	ch := r1.Character
	if ch.Name != "" || ch.NeverCreate || ch.Class.ID != *b1.Class || !slices.Equal(ch.Names, derivedNames(t, botnames.DefaultMarker, "hz1")) {
		t.Fatalf("new bot's character %+v", ch)
	}
	if err := ch.Validate(); err != nil {
		t.Fatalf("new bot's character: %v", err)
	}
	if r1.Preset != "town" || r1.Metrics == nil || !slices.Equal(r1.Markers, botnames.Markers{botnames.DefaultMarker}) {
		t.Fatalf("run %+v", r1.Run)
	}

	// The learned name is in the file as soon as the run reports it.
	r1.Learned("AldenBot", 0)
	if s := h.list(t)["hz1"]; s.Name != "AldenBot" || *s.Class != 0 {
		t.Fatalf("after learned: %+v", s.Bot)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, FileName)); err != nil || !strings.Contains(string(raw), `"name": "AldenBot"`) {
		t.Fatalf("registry file after learned: %v\n%s", err, raw)
	}

	// An account holding characters of no bot is dropped; the bot moves on.
	_, r2 := h.create(t, "support", ptr(botclient.OrcFighter.ID))
	r2.end <- botclient.ErrAccountHasCharacters
	r3 := h.next(t)
	if r3.Account != "hz3" || r3.Character.Class != botclient.OrcFighter || r3.Preset != "support" {
		t.Fatalf("replacement run %s %+v %s", r3.Account, r3.Character, r3.Preset)
	}
	l := h.list(t)
	if _, ok := l["hz2"]; ok || !l["hz3"].Online {
		t.Fatalf("after account has characters: %v", l)
	}

	// A deleted bot's account is never taken again.
	if err := h.sup.Delete(t.Context(), "hz1"); !errors.Is(err, ErrOnline) {
		t.Fatalf("Delete of an online bot: %v", err)
	}
	if err := h.sup.Despawn(t.Context(), "hz1"); err != nil {
		t.Fatal(err)
	}
	<-r1.ctx.Done()
	r1.end <- nil
	if s := h.settle(t, "hz1", RunStopping); s.Online || s.StopReason != reasonDespawned || !s.StoppedAt.Equal(t0) {
		t.Fatalf("despawned bot %+v", s)
	}
	if err := h.sup.Delete(t.Context(), "hz1"); err != nil {
		t.Fatal(err)
	}
	b4, r4 := h.create(t, "town", nil)
	if b4.Account != "hz4" {
		t.Fatalf("new bot after a delete took %s, want hz4", b4.Account)
	}
	if _, err := h.sup.Create(t.Context(), "bogus", nil); err == nil {
		t.Fatal("unknown preset accepted")
	}
	if _, err := h.sup.Create(t.Context(), "town", ptr(1000)); err == nil {
		t.Fatal("unknown class accepted")
	}
	h.stop(t, r3, r4)
	if ts := h.reg.Tombstones(); !slices.Equal([]string{ts[0].Account, ts[1].Account}, []string{"hz2", "hz1"}) {
		t.Fatalf("tombstones %+v", ts)
	}
}

func TestSupervisorImportedBotNeverCreatesAcrossRestart(t *testing.T) {
	reg := openPanel(t, t.TempDir())
	clock := newClock()
	h := startSupervisor(t, reg, clock, 5)
	added, err := h.sup.Import(t.Context(), "hz", botnames.DefaultMarker, 2)
	if err != nil || !slices.Equal(accounts(added), []string{"hz1", "hz2"}) {
		t.Fatalf("Import = %v, %v", accounts(added), err)
	}
	b3, r3 := h.create(t, "town", nil)
	if b3.Account != "hz3" {
		t.Fatalf("new bot after import took %s, want hz3", b3.Account)
	}
	if err := h.sup.Spawn(t.Context(), "hz1"); err != nil {
		t.Fatal(err)
	}
	r1 := h.next(t)
	want := botclient.Character{Names: derivedNames(t, botnames.DefaultMarker, "hz1"), NeverCreate: true}
	if !slices.Equal(r1.Character.Names, want.Names) || !r1.Character.NeverCreate || r1.Preset != ImportPreset {
		t.Fatalf("imported run %+v %s", r1.Character, r1.Preset)
	}
	h.stop(t, r1, r3)

	// Between runs the test owns the registry: an older swarm's prefix
	// joins it, and every run's bot filter knows its marker.
	if _, err := reg.Import("old", npcMarker, 1, ImportPreset); err != nil {
		t.Fatal(err)
	}
	h = startSupervisor(t, reg, clock, 5)
	restored := map[string]*fakeRun{}
	for range 2 {
		r := h.next(t)
		restored[r.Account] = r
		if !slices.Contains(r.Markers, npcMarker) || !slices.Contains(r.Markers, botnames.DefaultMarker) {
			t.Fatalf("restored run of %s has markers %v", r.Account, r.Markers)
		}
	}
	r1, r3 = restored["hz1"], restored["hz3"]
	if r1 == nil || r3 == nil {
		t.Fatalf("restored %v, want hz1 and hz3", restored)
	}
	if !r1.Character.NeverCreate || r1.Character.Name != "" || r1.Preset != ImportPreset {
		t.Fatalf("restored imported run %+v", r1.Character)
	}
	if r3.Character.NeverCreate || r3.Character.Class.ID != *b3.Class || r3.Preset != "town" {
		t.Fatalf("restored new run %+v", r3.Character)
	}

	// Adopting learns the name and class; the next run is a known-name
	// login and creates nothing.
	r1.Learned("OldBot", botclient.ElvenMystic.ID)
	if err := h.sup.Respawn(t.Context(), "hz1"); err != nil {
		t.Fatal(err)
	}
	<-r1.ctx.Done()
	r1.end <- nil
	r1b := h.next(t)
	if !reflect.DeepEqual(r1b.Character, botclient.Character{Name: "OldBot"}) {
		t.Fatalf("known-name run %+v", r1b.Character)
	}
	if s := h.list(t)["hz1"]; *s.Class != botclient.ElvenMystic.ID {
		t.Fatalf("learned class %d", *s.Class)
	}
	h.stop(t, r1b, r3)
}

func TestSupervisorMaxOnline(t *testing.T) {
	reg := openPanel(t, t.TempDir())
	clock := newClock()
	h := startSupervisor(t, reg, clock, 2)
	_, r1 := h.create(t, "town", nil)
	_, r2 := h.create(t, "support", nil)
	if _, err := h.sup.Create(t.Context(), "town", nil); !errors.Is(err, ErrMaxOnline) {
		t.Fatalf("Create past the cap: %v", err)
	}
	if _, err := h.sup.Import(t.Context(), "hz", botnames.DefaultMarker, 3); err != nil {
		t.Fatal(err)
	}
	if err := h.sup.Spawn(t.Context(), "hz3"); !errors.Is(err, ErrMaxOnline) {
		t.Fatalf("Spawn past the cap: %v", err)
	}
	// A despawned bot holds its place until its run reported.
	if err := h.sup.Despawn(t.Context(), "hz2"); err != nil {
		t.Fatal(err)
	}
	if err := h.sup.Spawn(t.Context(), "hz3"); !errors.Is(err, ErrMaxOnline) {
		t.Fatalf("Spawn while a despawn is under way: %v", err)
	}
	<-r2.ctx.Done()
	r2.end <- nil
	h.settle(t, "hz2", RunStopping)
	if err := h.sup.Spawn(t.Context(), "hz3"); err != nil {
		t.Fatal(err)
	}
	r3 := h.next(t)
	r1.Learned("AldenBot", botclient.HumanFighter.ID)
	r3.Learned("OldBot", botclient.DarkMystic.ID)
	if err := h.sup.Spawn(t.Context(), "hz2"); !errors.Is(err, ErrMaxOnline) {
		t.Fatalf("Spawn past the cap: %v", err)
	}
	h.stop(t, r1, r3)

	// Mark hz2 online too: a restart with a cap of 2 restores the first
	// two, each as a known-name login with its preset, and drops the third.
	if err := reg.Update("hz2", func(b *Bot) { b.Online = true }); err != nil {
		t.Fatal(err)
	}
	h = startSupervisor(t, reg, clock, 2)
	got := map[string]*fakeRun{}
	for range 2 {
		r := h.next(t)
		got[r.Account] = r
	}
	if r := got["hz1"]; r == nil || !reflect.DeepEqual(r.Character, botclient.Character{Name: "AldenBot"}) || r.Preset != "town" {
		t.Fatalf("restored hz1: %+v", r)
	}
	if r := got["hz2"]; r == nil || r.Character.Name != "" || r.Character.NeverCreate || r.Preset != "support" {
		t.Fatalf("restored hz2: %+v", r)
	}
	if s := h.list(t)["hz3"]; s.Online || s.Run != RunNone || s.StopReason != reasonOverCap {
		t.Fatalf("hz3 past the cap at restore: %+v", s)
	}
	h.noRun(t)
	h.stop(t, got["hz1"], got["hz2"])
}

func TestSupervisorEndStates(t *testing.T) {
	tests := []struct {
		name string
		// halt trips the breaker first; the run then ends with its error.
		halt       bool
		err        error
		wantRun    RunState
		wantOnline bool
		wantReason string
	}{
		{name: "commanded logout", err: botclient.ErrCommandedLogout, wantRun: RunNone, wantReason: botclient.ErrCommandedLogout.Error()},
		{name: "halted", halt: true, wantRun: RunHalted, wantOnline: true, wantReason: botclient.ErrLoginsHalted.Error()},
		{name: "repeated aborts", err: botclient.ErrRepeatedAborts, wantRun: RunCoolingDown, wantOnline: true, wantReason: botclient.ErrRepeatedAborts.Error()},
		{name: "account taken", err: botclient.ErrAccountTaken, wantRun: RunNone, wantReason: botclient.ErrAccountTaken.Error()},
		{name: "character missing", err: botclient.ErrCharacterMissing, wantRun: RunNone, wantReason: botclient.ErrCharacterMissing.Error()},
		{name: "other error", err: errors.New("login refused: banned"), wantRun: RunNone, wantReason: "banned"},
		{name: "nil end", err: nil, wantRun: RunNone, wantReason: "run ended"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 3)
			_, r := h.create(t, "town", nil)
			err := tt.err
			if tt.halt {
				h.gate.Halt(errors.New("refused credential"))
				err = h.gate.Err()
			}
			r.end <- err
			s := h.settle(t, "hz1", RunActive)
			if s.Run != tt.wantRun || s.Online != tt.wantOnline || !strings.Contains(s.StopReason, tt.wantReason) || !s.StoppedAt.Equal(t0) {
				t.Fatalf("after %v: run %s online %v reason %q at %v", err, s.Run, s.Online, s.StopReason, s.StoppedAt)
			}
			if tt.wantRun == RunCoolingDown && !s.RestartAt.Equal(t0.Add(CoolDown)) {
				t.Fatalf("restart at %v", s.RestartAt)
			}
			h.noRun(t)
			h.stop(t)
		})
	}
}

func TestSupervisorResumeLogins(t *testing.T) {
	h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 3)
	_, r1 := h.create(t, "town", nil)
	_, r2 := h.create(t, "town", nil)
	_, r3 := h.create(t, "town", nil)
	h.gate.Halt(errors.New("refused credential"))
	halted := h.gate.Err()
	r1.end <- halted
	r2.end <- halted
	h.settle(t, "hz1", RunActive)
	h.settle(t, "hz2", RunActive)
	h.noRun(t)

	if err := h.sup.ResumeLogins(t.Context()); err != nil {
		t.Fatal(err)
	}
	if h.gate.Err() != nil {
		t.Fatal("breaker still tripped after ResumeLogins")
	}
	back := map[string]*fakeRun{}
	for range 2 {
		r := h.next(t)
		back[r.Account] = r
	}
	if back["hz1"] == nil || back["hz2"] == nil {
		t.Fatalf("resumed %v, want hz1 and hz2", back)
	}
	// A halted report that arrives after the reset restarts its bot at once.
	r3.end <- halted
	r3b := h.next(t)
	if r3b.Account != "hz3" {
		t.Fatalf("restarted %s, want hz3", r3b.Account)
	}
	h.stop(t, back["hz1"], back["hz2"], r3b)
}

func TestSupervisorCoolDown(t *testing.T) {
	clock := newClock()
	h := startSupervisor(t, openPanel(t, t.TempDir()), clock, 3)
	_, r := h.create(t, "town", nil)
	r.end <- botclient.ErrRepeatedAborts
	h.settle(t, "hz1", RunActive)
	clock.Advance(CoolDown - time.Second)
	h.noRun(t)
	if s := h.list(t)["hz1"]; s.Run != RunCoolingDown || !s.Online {
		t.Fatalf("before the cool-down ended: %+v", s)
	}
	clock.Advance(time.Second)
	r2 := h.next(t)
	if r2.Account != "hz1" || r2.Metrics != r.Metrics {
		t.Fatalf("restart of %s with slot %p, want hz1 with %p", r2.Account, r2.Metrics, r.Metrics)
	}
	h.stop(t, r2)
}

func TestSupervisorRespawnWaitsForTheReport(t *testing.T) {
	h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 3)
	_, r1 := h.create(t, "town", nil)
	_, other := h.create(t, "town", nil)
	if err := h.sup.SetPreset(t.Context(), "hz1", "bogus"); err == nil {
		t.Fatal("unknown preset accepted")
	}
	if err := h.sup.SetPreset(t.Context(), "hz1", "merchant"); err != nil {
		t.Fatal(err)
	}
	<-r1.ctx.Done()
	if s := h.list(t)["hz1"]; s.Run != RunStopping || !s.Online {
		t.Fatalf("during the persona change: %+v", s)
	}
	h.noRun(t)
	r1.end <- nil
	r2 := h.next(t)
	if r2.Account != "hz1" || r2.Preset != "merchant" || r2.Metrics != r1.Metrics || r2.Metrics == other.Metrics {
		t.Fatalf("new run %s %s slot %p (old %p)", r2.Account, r2.Preset, r2.Metrics, r1.Metrics)
	}
	if err := h.sup.Respawn(t.Context(), "hz1"); err != nil {
		t.Fatal(err)
	}
	<-r2.ctx.Done()
	h.noRun(t)
	r2.end <- nil
	r3 := h.next(t)
	if err := h.sup.Respawn(t.Context(), "hz9"); !errors.Is(err, ErrNoBot) {
		t.Fatalf("Respawn of an unknown bot: %v", err)
	}
	h.stop(t, r3, other)
}

func TestSupervisorStopJoinsEveryRun(t *testing.T) {
	reg := openPanel(t, t.TempDir())
	h := startSupervisor(t, reg, newClock(), 3)
	_, r1 := h.create(t, "town", nil)
	_, r2 := h.create(t, "town", nil)
	h.cancel()
	<-r1.ctx.Done()
	<-r2.ctx.Done()
	r1.end <- nil
	// Run returns only once every run reported.
	h.noRunReturn(t)
	r2.Learned("LateBot", 0) // a report still under way is handled
	r2.end <- nil
	if err := <-h.done; err != nil {
		t.Fatal(err)
	}
	if _, err := h.sup.List(t.Context()); !errors.Is(err, ErrStopped) {
		t.Fatalf("List after stop: %v", err)
	}
	for _, b := range reg.Bots() {
		if !b.Online {
			t.Fatalf("bot %s marked offline by the panel's stop", b.Account)
		}
	}
	if b, _ := reg.Bot("hz2"); b.Name != "LateBot" {
		t.Fatalf("name learned during the stop lost: %+v", b)
	}
}

func (h *harness) noRunReturn(t *testing.T) {
	t.Helper()
	runtime.Gosched()
	select {
	case err := <-h.done:
		t.Fatalf("Run returned %v before every run reported", err)
	default:
	}
}
