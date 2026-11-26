package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// backoffSpots are six spots west of the split navigation's wall, all in
// the SCC of backoffSelf and nearer than Wander, none within spotArrive of
// it, and each farther from it than a route's straight segment reaches
// (nav's navSegment, 2500), so a walk to one always runs the search (#565).
var backoffSpots = []datapack.Spot{
	testSpot("e", 7000, 4000, 3, 6, 9), testSpot("s", 4000, 7000, 3, 6, 9), testSpot("w", 1000, 4000, 3, 6, 9),
	testSpot("n", 4000, 1000, 3, 6, 9), testSpot("se", 7000, 7000, 3, 6, 9), testSpot("nw", 1000, 1000, 3, 6, 9),
}

var backoffSelf = Point{X: 4000, Y: 4000}

// newBackoffBrain is a spot brain standing at backoffSelf among spots whose
// every walk ends unreachable: the walker's searches get a budget of one
// cell, and every walk searches. It records its metrics in the registry
// returned.
func newBackoffBrain(t *testing.T, spots ...datapack.Spot) (*Brain, *botmetrics.Registry) {
	t.Helper()
	b := newSpotBrain(t, 5, spots...)
	b.world.Self.move = stopAt(backoffSelf)
	b.walk.plan.Budget = 1
	reg := botmetrics.NewRegistry()
	b.metrics = reg.NewBot()
	return b, reg
}

// plansMade is how many route plans the registry's bots ran, and how many
// of them found a route.
func plansMade(reg *botmetrics.Registry, at time.Time) (all, ok uint64) {
	s := reg.Collect(at)
	for _, n := range s.NavPlans {
		all += n
	}
	return all, s.NavPlans[botmetrics.PlanOK.String()]
}

// TestSpotUnreachableBackOff checks that a bot whose walk to every
// candidate spot ends unreachable runs at most spotFailMax route plans in
// any spotRetry window, yet tries every candidate in turn (#497).
func TestSpotUnreachableBackOff(t *testing.T) {
	b, reg := newBackoffBrain(t, backoffSpots...)
	const step = 250 * time.Millisecond
	var times []time.Time
	tried := map[Point]bool{}
	for now := at(time.Second); now.Before(at(7 * time.Minute)); now = now.Add(step) {
		before, _ := plansMade(reg, now)
		b.ruleSpot(now)
		after, ok := plansMade(reg, now)
		if ok != 0 {
			t.Fatalf("%v: a plan found a route; the fixture wants every walk unreachable", now.Sub(t0))
		}
		for range after - before {
			times = append(times, now)
			tried[b.walk.dest] = true
		}
	}
	for i := range times {
		n := 0
		for _, u := range times[i:] {
			if u.Sub(times[i]) < spotRetry {
				n++
			}
		}
		if n > spotFailMax {
			t.Fatalf("%d plans in the spotRetry window from %v, want at most %d (plans at %v)", n, times[i].Sub(t0), spotFailMax, times)
		}
	}
	if len(tried) != len(backoffSpots) {
		t.Errorf("walked to %d spots, want every one of %d tried in turn", len(tried), len(backoffSpots))
	}
	t.Logf("%d plans in 7 minutes to %d spots", len(times), len(tried))
}

// TestSpotFailedRun checks how a run of unreachable spot walks grows and
// ends: the spotFailMax-th failure waits spotRetry with the run's spots
// avoided past it, a spot failing again is not added twice and waits too,
// an arrival ends the run, and so does a search with no candidate left.
func TestSpotFailedRun(t *testing.T) {
	now := at(time.Minute)
	tests := []struct {
		name   string
		self   Point
		failed []int
		// avoidAll avoids every spot for walkRetry.
		avoidAll bool
		// expired lets the run's avoids run out, so a run spot is picked again.
		expired bool
		// budget is the walker's search budget, 0 for the default.
		budget     int
		wantFailed int
		wantRetry  time.Time
	}{
		{name: "first failure: look again at once", self: backoffSelf, budget: 1, wantFailed: 1},
		{name: "spotFailMax-th failure: wait spotRetry", self: backoffSelf, failed: []int{1, 2}, budget: 1, wantFailed: 3, wantRetry: now.Add(spotRetry)},
		{name: "arrival ends the run", self: Point{X: 7000, Y: 4000}, failed: []int{1, 2}},
		{name: "repeat failure: wait spotRetry, run not grown", self: backoffSelf, failed: []int{0, 1, 2, 3, 4}, expired: true, budget: 1, wantFailed: 5, wantRetry: now.Add(spotRetry)},
		{name: "no candidate left ends the run", self: backoffSelf, failed: []int{0}, avoidAll: true, wantRetry: now.Add(spotRetry)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newBackoffBrain(t, backoffSpots...)
			b.world.Self.move = stopAt(tt.self)
			if tt.budget == 0 {
				b.walk.plan.Budget = walkBudget
			}
			v := &b.spot
			v.failed, v.avoid = append([]int(nil), tt.failed...), map[int]time.Time{}
			for _, i := range tt.failed {
				if !tt.expired {
					v.avoid[i] = now.Add(walkRetry - time.Second)
				}
			}
			if tt.avoidAll {
				for i := range backoffSpots {
					v.avoid[i] = now.Add(walkRetry)
				}
			}
			b.ruleSpot(now)
			if len(v.failed) != tt.wantFailed || !v.retryAt.Equal(tt.wantRetry) {
				t.Fatalf("run %v retry %v, want %d spots and retry %v", v.failed, v.retryAt, tt.wantFailed, tt.wantRetry)
			}
			if !tt.wantRetry.IsZero() && tt.wantFailed > 0 {
				for _, i := range v.failed {
					if v.avoid[i].Before(tt.wantRetry.Add(walkRetry)) {
						t.Errorf("spot %d avoided until %v, want past the wait and walkRetry", i, v.avoid[i])
					}
				}
			}
		})
	}
}
