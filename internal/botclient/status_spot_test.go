package botclient

import (
	"testing"
	"time"
)

// TestBrainStatusSpot checks that a snapshot names the spot the brain
// holds: none before it picks, the new one once it outlevels the first,
// none once it gives it back.
func TestBrainStatusSpot(t *testing.T) {
	b := newSpotBrain(t, 5, testSpot("newbie", 1000, 0, 2, 5, 9), testSpot("next", 0, 3000, 8, 11, 9))
	linkBrain(b, standing{})
	steps := []struct {
		name  string
		level int32
		leave bool
		want  string
	}{
		{"level 5: the newbie spot", 5, false, "newbie"},
		{"level 11: outlevelled, the next one", 11, false, "next"},
		{"given back", 11, true, ""},
	}
	b.publishStatus(t0, true)
	if got := b.Status().Spot; got != "" {
		t.Fatalf("before any pick: spot %q, want none", got)
	}
	for i, s := range steps {
		now := at(time.Duration(i+1) * time.Second)
		b.world.Self.Level = s.level
		if s.leave {
			b.leaveSpot()
		} else {
			b.decide(now)
		}
		b.publishStatus(now, true)
		if got := b.Status().Spot; got != s.want {
			t.Fatalf("%s: spot %q, want %q", s.name, got, s.want)
		}
	}
}
