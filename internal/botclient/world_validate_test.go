package botclient

import (
	"testing"
	"time"
)

// TestSelfValidateLocationAdopted checks that the bot's own ValidateLocation
// is adopted whatever its distance from the estimate: the server sends one
// only past move-speed drift and never adopts the bot's report
// (network/movement.go validateLivePlayerPosition), so a correction inside
// snapDrift is where the server holds the character (#464).
func TestSelfValidateLocationAdopted(t *testing.T) {
	type check struct {
		at   time.Duration
		want Point
	}
	tests := []struct {
		name   string
		move   bool
		at     time.Duration
		report Point
		checks []check
	}{
		{
			name: "standing, 150 off", at: time.Second, report: Point{X: 150},
			checks: []check{{time.Second, Point{X: 150}}, {5 * time.Second, Point{X: 150}}},
		},
		{
			// The estimate at 2 s is 200: 150 behind, carrying on from it.
			name: "moving, 150 behind", move: true, at: 2 * time.Second, report: Point{X: 50},
			checks: []check{{2 * time.Second, Point{X: 50}}, {3 * time.Second, Point{X: 150}}},
		},
		{
			name: "moving, 150 ahead", move: true, at: 2 * time.Second, report: Point{X: 350},
			checks: []check{{2 * time.Second, Point{X: 350}}, {3 * time.Second, Point{X: 450}}},
		},
		{
			name: "moving, 150 aside", move: true, at: 2 * time.Second, report: Point{X: 200, Y: 150},
			checks: []check{{2 * time.Second, Point{X: 200, Y: 150}}, {30 * time.Second, Point{X: 1000}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			if tt.move {
				w.Apply(MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}, t0)
			}
			w.Apply(ValidateLocation{ObjectID: selfID, Pos: tt.report, Heading: 7}, at(tt.at))
			for _, c := range tt.checks {
				if got := w.Self.Position(at(c.at)); got != c.want {
					t.Errorf("position at %v = %v, want %v", c.at, got, c.want)
				}
			}
			if w.Self.Heading != 7 {
				t.Errorf("heading %d, want 7", w.Self.Heading)
			}
		})
	}
}

// TestSelfUserInfoKeepsEstimate checks that a UserInfo, a status packet and
// not a correction, keeps the estimate when its position is within
// snapDrift of it.
func TestSelfUserInfoKeepsEstimate(t *testing.T) {
	w := newTestWorld(t)
	w.Apply(MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}, t0)
	w.Apply(UserInfo{ObjectID: selfID, Name: "bot7", RunSpeed: 100, WalkSpeed: 50, Running: true, HP: 10, MaxHP: 20, Pos: Point{X: 350}}, at(2*time.Second))
	if got, want := w.Self.Position(at(3*time.Second)), (Point{X: 300}); got != want {
		t.Fatalf("position = %v, want %v", got, want)
	}
}

// TestWalkerSegmentFromCorrection checks that the walker's next segment
// starts where a self ValidateLocation inside snapDrift put the bot, not at
// the estimate the server does not hold (#464).
func TestWalkerSegmentFromCorrection(t *testing.T) {
	dest := Point{X: 1000}
	t.Run("standing", func(t *testing.T) {
		b, _, _ := newNavBrain(t, testNav(t), Persona{}, Point{})
		b.handleOne(t, ValidateLocation{ObjectID: selfID, Pos: Point{X: 150}}, at(100*time.Millisecond))
		a, st := walkStep(t, b, dest, at(200*time.Millisecond))
		if st != walkWalking || a.kind != actMove {
			t.Fatalf("status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
		}
		if o := moveOrigin(a.payload); o.X != 150 || o.Y != 0 {
			t.Fatalf("segment origin %v, want the correction at (150, 0)", o)
		}
	})
	t.Run("after a segment", func(t *testing.T) {
		b, seg := wakeWalk(t, dest, segAnswer)
		// Past the segment's modeled end, the server holds the bot 150 short
		// of it.
		due := b.world.Self.arrival().Add(positionTick)
		corr := seg.to
		corr.X -= 150
		b.handleOne(t, ValidateLocation{ObjectID: selfID, Pos: corr}, due)
		a, st := walkStep(t, b, dest, due.Add(100*time.Millisecond))
		if st != walkWalking || a.kind != actMove {
			t.Fatalf("status %v kind %v (%s), want a move", st, a.kind, b.walk.reason)
		}
		if o := moveOrigin(a.payload); o.X != corr.X || o.Y != corr.Y {
			t.Fatalf("segment origin %v, want the correction at (%d, %d)", o, corr.X, corr.Y)
		}
	})
}
