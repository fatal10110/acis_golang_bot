package botclient

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestWalkerIdleMetric answers the first segment of a walk out of the U,
// lets the bot stand past the modeled arrival, and sends the next segment:
// exactly one idle observation, from the arrival to that send. The first
// segment has none, nor has a segment after one that did not arrive.
func TestWalkerIdleMetric(t *testing.T) {
	tests := []struct {
		name      string
		answerLag time.Duration // from the send to the server's answer
		delay     time.Duration // from the modeled arrival to the next send
		blocked   bool          // the server stops the first segment at once
		wantCount uint64
	}{
		{name: "next send one tick after the arrival", delay: positionTick, wantCount: 1},
		{name: "late answer, brain tick later", answerLag: 60 * time.Millisecond, delay: 750 * time.Millisecond, wantCount: 1},
		{name: "jittered tick", answerLag: 25 * time.Millisecond, delay: 1400 * time.Millisecond, wantCount: 1},
		{name: "blocked first segment", answerLag: 30 * time.Millisecond, delay: time.Second, blocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := testNav(t)
			from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
			b, _, reg := newNavBrain(t, n, Persona{}, from)
			a, st := walkStep(t, b, dest, t0)
			if st != walkWalking || a.kind != actMove {
				t.Fatalf("first decision: status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
			}
			if s := reg.Collect(t0); s.NavWalkIdleMillis.Count != 0 {
				t.Fatalf("first segment: %d idle observations, want none", s.NavWalkIdleMillis.Count)
			}
			origin, to := moveOrigin(a.payload), moveTarget(a.payload)
			answer := segAnswer(origin, to)
			if tt.blocked {
				answer.Dest = answer.Origin
			}
			answeredAt := t0.Add(tt.answerLag)
			b.handle([]Event{answer}, answeredAt)
			arrival := answeredAt
			if !tt.blocked {
				arrival = b.world.Self.arrival()
				if !arrival.After(answeredAt) {
					t.Fatalf("modeled arrival %v, want after the answer at %v", arrival.Sub(t0), tt.answerLag)
				}
			}
			now := arrival.Add(tt.delay)
			if a, st := walkStep(t, b, dest, now); st != walkWalking || a.kind != actMove {
				t.Fatalf("next decision: status %v kind %v (%s), want the next segment", st, a.kind, b.walk.reason)
			}
			s := reg.Collect(now)
			if got := s.NavWalkIdleMillis; got.Count != tt.wantCount {
				t.Fatalf("%d idle observations, want %d", got.Count, tt.wantCount)
			}
			if want := uint64(tt.delay.Milliseconds()); tt.wantCount == 1 && s.NavWalkIdleMillis.Sum != want {
				t.Errorf("idle %d ms, want %d ms (the arrival to the send)", s.NavWalkIdleMillis.Sum, want)
			}
		})
	}
}

// TestWalkerEfficiencyMetric walks the whole route out of the U, standing a
// while after each modeled arrival: the arrival records one efficiency, the
// route's length over the elapsed time times the run speed, and a later
// decision at the destination records no second one.
func TestWalkerEfficiencyMetric(t *testing.T) {
	const stand = 300 * time.Millisecond
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	b, _, reg := newNavBrain(t, n, Persona{}, from)
	now := t0
	var length float64
	var segs int
	for i := 0; ; i++ {
		if i > 60 {
			t.Fatal("no arrival after 60 decisions")
		}
		a, st := walkStep(t, b, dest, now)
		if st == walkArrived {
			break
		}
		if st != walkWalking || a.kind != actMove {
			t.Fatalf("decision %d: status %v kind %v (%s), want a move", i, st, a.kind, b.walk.reason)
		}
		origin, to := moveOrigin(a.payload), moveTarget(a.payload)
		length += math.Sqrt(float64(horizontal2(origin, to)))
		segs++
		b.handle([]Event{segAnswer(origin, to)}, now)
		now = b.world.Self.arrival().Add(positionTick + stand)
	}
	if segs < 2 {
		t.Fatalf("%d segments, want the route around the U", segs)
	}
	run := float64(b.world.Self.RunSpeed)
	want := int64(math.Round(1000 * length / (now.Sub(t0).Seconds() * run)))
	if want <= 0 || want >= 1000 {
		t.Fatalf("expected efficiency %d per mille, want inside (0, 1000) for a walk with stops", want)
	}
	if _, st := walkStep(t, b, dest, now.Add(time.Second)); st != walkArrived {
		t.Fatalf("status %v at the destination, want arrived", st)
	}
	s := reg.Collect(now)
	if got := s.NavWalkEfficiency; got.Count != 1 || got.Sum != uint64(want) {
		t.Errorf("efficiency count %d sum %d, want one of %d per mille", got.Count, got.Sum, want)
	}
	if got := s.NavWalkIdleMillis; got.Count != uint64(segs-1) || got.Sum != uint64(segs-1)*uint64((positionTick+stand).Milliseconds()) {
		t.Errorf("idle count %d sum %d ms, want %d of %v", got.Count, got.Sum, segs-1, positionTick+stand)
	}
}
