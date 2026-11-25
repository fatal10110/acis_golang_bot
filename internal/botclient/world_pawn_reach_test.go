package botclient

import (
	"testing"
	"time"
)

// TestWorldPawnInReachStandsAtServer stands the bot where the server holds
// it once a MoveToPawn of its own reports it in reach of the target: the
// server faces the target from the player's current location
// (targeting.go thinkInteract), which the walker's next segment starts from
// (#434). A pawn approach from out of reach keeps the estimate of its end.
func TestWorldPawnInReachStandsAtServer(t *testing.T) {
	type step struct {
		at time.Duration
		ev Event
	}
	tests := []struct {
		name  string
		steps []step
		check time.Duration
		want  Point
	}{
		{
			name: "the face after an approach stands at the server's position",
			steps: []step{
				{0, MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 100}},
				{8 * time.Second, MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 150, Origin: Point{X: 896, Y: 11}}},
			},
			check: 10 * time.Second,
			want:  Point{X: 896, Y: 11},
		},
		{
			name: "a face from a standstill a cell off the estimate stands at the server's position",
			steps: []step{
				{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 880}}},
				{10 * time.Second, MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 150, Origin: Point{X: 876, Y: -11}}},
			},
			check: 11 * time.Second,
			want:  Point{X: 876, Y: -11},
		},
		{
			name: "an approach out of reach keeps the estimate of its end",
			steps: []step{
				{0, MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 100, Origin: Point{X: 4, Y: 11}}},
			},
			check: 20 * time.Second,
			want:  Point{X: 900},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, s := range tt.steps {
				w.Apply(s.ev, at(s.at))
			}
			now := at(tt.check)
			if got := w.Self.Position(now); got != tt.want {
				t.Errorf("Position = %+v, want %+v", got, tt.want)
			}
			if w.Self.Moving(now) {
				t.Error("still moving, want standing")
			}
		})
	}
}
