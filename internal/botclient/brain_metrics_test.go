package botclient

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

func TestBrainMetricsRun(t *testing.T) {
	reg := botmetrics.NewRegistry(RuleNames()...)
	goal := func(*World, time.Time) (Action, bool) { return emoteAction(7), true }
	clock := newFakeClock(t0)
	out := make(chanSender, 16)
	b := NewBrain(newTestWorld(t), NewEvents(64), out, clock, BrainConfig{
		Persona: Persona{Goal: goal}, Metrics: reg.NewBot(), Rand: rand.New(rand.NewPCG(5, 6)),
	})
	done, errc := make(chan struct{}), make(chan error, 1)
	go func() { errc <- b.Run(done, nil) }()
	<-clock.armed
	b.events.push(StatusUpdate{ObjectID: selfID + 1, Set: StatusHP, HP: 1}, nil)
	<-clock.armed // the loop drained the event and parked again
	const late = 50 * time.Millisecond
	clock.Advance(defaultTick + defaultTickJitter + late)
	if f := <-out; f[0] != opGameRequestSocialAction {
		t.Fatalf("sent opcode %#x, want RequestSocialAction", f[0])
	}
	close(done)
	if err := <-errc; err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}

	s := reg.Collect(clock.Now())
	if s.Events != 1 || s.QueueDepth.Count != 1 || s.QueueDepth.Max != 1 {
		t.Errorf("events %d, queue depth %+v: want one drain of one event", s.Events, s.QueueDepth)
	}
	if s.Sent != 1 || s.Rules["goal"] != 1 {
		t.Errorf("sent %d, rules %v: want one goal decision sent", s.Sent, s.Rules)
	}
	if s.WakeLagMicros.Count == 0 || s.WakeLagMicros.Max < late.Microseconds() {
		t.Errorf("wake lag %+v, want a wakeup at least %v late", s.WakeLagMicros, late)
	}
}

func TestBrainMetricsPaced(t *testing.T) {
	reg := botmetrics.NewRegistry(RuleNames()...)
	b, out := newTestBrain(t, Persona{})
	b.metrics = reg.NewBot()
	for range 2 {
		if _, err := b.send(emoteAction(2), t0); err != nil {
			t.Fatal(err)
		}
	}
	if s := reg.Collect(t0); s.Sent != 1 || s.Paced != 1 || len(out.frames) != 1 {
		t.Errorf("sent %d, paced %d, frames %d: want the second emote held by the social gate", s.Sent, s.Paced, len(out.frames))
	}
}

func TestRuleNamesMetrics(t *testing.T) {
	names := RuleNames()
	for _, want := range []string{"dead", "prompt", "goal", "idle", "store"} {
		if !slices.Contains(names, want) {
			t.Errorf("RuleNames() = %v, missing %q", names, want)
		}
	}
	sorted := slices.Sorted(slices.Values(names))
	if len(slices.Compact(sorted)) != len(names) {
		t.Errorf("RuleNames() = %v has duplicates", names)
	}
}
