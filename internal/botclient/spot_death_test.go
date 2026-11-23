package botclient

import (
	"testing"
	"time"
)

// TestSpotDeathAvoid checks a bot that dies holding a spot gives it back
// and avoids it for SpotPolicy.DeathAvoid (#258): back on its feet it
// claims another spot that fits, and the one it died at again once the
// time is up; with DeathAvoid 0 it walks straight back.
func TestSpotDeathAvoid(t *testing.T) {
	const avoid = 30 * time.Minute
	type step struct {
		name string
		// die is a death of the bot then its revive, at the step's time.
		die bool
		at  time.Duration
		// want is the spot held after the step's decision, -1 for none.
		want int
	}
	tests := []struct {
		name       string
		deathAvoid time.Duration
		steps      []step
	}{
		{
			name: "avoided until the time is up", deathAvoid: avoid,
			steps: []step{
				{name: "the near spot", at: time.Second, want: 0},
				{name: "died there: the far one", die: true, at: 2 * time.Second, want: 1},
				{name: "died at the far one too: none left", die: true, at: 10 * time.Minute, want: -1},
				{name: "the near one again past its time", at: 2*time.Second + avoid, want: 0},
			},
		},
		{
			name: "never avoided at 0",
			steps: []step{
				{name: "the near spot", at: time.Second, want: 0},
				{name: "died there: the near one again", die: true, at: 2 * time.Second, want: 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 3, testSpot("near", 1000, 0, 1, 3, 9), testSpot("far", 0, 3000, 1, 3, 9))
			b.persona.Spots.DeathAvoid = tt.deathAvoid
			for _, s := range tt.steps {
				now := at(s.at)
				if s.die {
					if err := b.handle([]Event{Die{ObjectID: selfID}}, now); err != nil {
						t.Fatal(err)
					}
					if b.spot.held || b.cfg.Spots.Held(0)+b.cfg.Spots.Held(1) != 0 {
						t.Fatalf("%s: dead, spot %d held %v, registry %d/%d", s.name, b.spot.idx, b.spot.held, b.cfg.Spots.Held(0), b.cfg.Spots.Held(1))
					}
					if err := b.handle([]Event{Revive{ObjectID: selfID}}, now); err != nil {
						t.Fatal(err)
					}
				}
				b.decide(now)
				got := -1
				if b.spot.held {
					got = b.spot.idx
				}
				if got != s.want {
					t.Fatalf("%s: spot %d, want %d", s.name, got, s.want)
				}
				for i := range 2 {
					if want := map[bool]int{true: 1}[i == got]; b.cfg.Spots.Held(i) != want {
						t.Fatalf("%s: registry holds %d at spot %d, want %d", s.name, b.cfg.Spots.Held(i), i, want)
					}
				}
			}
		})
	}
}
