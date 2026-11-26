package botclient

import (
	"errors"
	"math/rand/v2"
	"testing"
	"time"
)

func TestLoginGateSetSpacing(t *testing.T) {
	tests := []struct {
		name        string
		allowlisted bool
		keep        bool // keep the gate's default spacing
		set         time.Duration
		wantErr     bool
		// gap bounds the time between two dial starts: [lo, hi).
		lo, hi time.Duration
	}{
		{name: "spaced default", keep: true, lo: dialSpacing, hi: dialSpacing + dialJitter},
		{name: "spaced at floor", set: dialSpacing, lo: dialSpacing, hi: dialSpacing + dialJitter},
		{name: "spaced raised", set: 5 * time.Second, lo: 5 * time.Second, hi: 5*time.Second + dialJitter},
		{name: "spaced at max", set: MaxLoginSpacing, lo: MaxLoginSpacing, hi: MaxLoginSpacing + dialJitter},
		{name: "spaced under floor", set: dialSpacing - time.Millisecond, wantErr: true, lo: dialSpacing, hi: dialSpacing + dialJitter},
		{name: "spaced zero", set: 0, wantErr: true, lo: dialSpacing, hi: dialSpacing + dialJitter},
		{name: "spaced over max", set: MaxLoginSpacing + time.Millisecond, wantErr: true, lo: dialSpacing, hi: dialSpacing + dialJitter},
		{name: "allowlisted default", allowlisted: true, keep: true, lo: 0, hi: 1},
		{name: "allowlisted zero", allowlisted: true, set: 0, lo: 0, hi: 1},
		{name: "allowlisted raised", allowlisted: true, set: 800 * time.Millisecond, lo: 800 * time.Millisecond, hi: 800*time.Millisecond + 1},
		{name: "allowlisted negative", allowlisted: true, set: -time.Millisecond, wantErr: true, lo: 0, hi: 1},
		{name: "allowlisted over max", allowlisted: true, set: MaxLoginSpacing + 1, wantErr: true, lo: 0, hi: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newFakeClock(t0)
			g := NewLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
			floor := dialSpacing
			if tt.allowlisted {
				g, floor = NewAllowlistedLoginGate(clock, rand.New(rand.NewPCG(1, 2))), 0
			}
			if got := g.MinSpacing(); got != floor {
				t.Fatalf("MinSpacing %s, want %s", got, floor)
			}
			if !tt.keep {
				err := g.SetSpacing(tt.set)
				if tt.wantErr != errors.Is(err, ErrSpacing) || tt.wantErr != (err != nil) {
					t.Fatalf("SetSpacing(%s) = %v, want error %t", tt.set, err, tt.wantErr)
				}
			}
			if got := g.Spacing(); got < tt.lo || got >= tt.hi {
				t.Fatalf("Spacing %s, want in [%s, %s)", got, tt.lo, tt.hi)
			}
			turn, err := g.Take(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer turn.Done()
			var starts []time.Time
			for range 3 {
				dialed := make(chan error, 1)
				go func() { dialed <- turn.Dial(t.Context()) }()
				for waiting := true; waiting; {
					select {
					case err := <-dialed:
						if err != nil {
							t.Fatal(err)
						}
						waiting = false
					case <-clock.armed:
						clock.advanceToNext()
					}
				}
				starts = append(starts, clock.Now())
				turn.Dialed(nil)
			}
			for i := 1; i < len(starts); i++ {
				if gap := starts[i].Sub(starts[i-1]); gap < tt.lo || gap >= tt.hi {
					t.Errorf("dial %d started %s after the previous one, want in [%s, %s)", i, gap, tt.lo, tt.hi)
				}
			}
		})
	}
}
