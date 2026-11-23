package botclient

import (
	"testing"
	"time"
)

// TestWorldAttackPosition places an attacker exactly where its Attack says
// it stands: the server stops an attacker before it swings (ai/player.go,
// ai/attackable.go: move.Stop before DoAttack) and the packet carries its
// position then (attack/controller.go snapshot). A dead-reckoned walk's end
// may miss it by a step (a pawn walk ends on the first step in range), and
// the walker's next segment goes out from there (#246).
func TestWorldAttackPosition(t *testing.T) {
	type attackStep struct {
		at time.Duration
		ev Event
	}
	tests := []struct {
		name  string
		steps []attackStep
		id    int32
		check time.Duration
		want  Point
	}{
		{
			name: "own attack a step off the walk's dead-reckoned end",
			steps: []attackStep{
				{0, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 100}}},
				{2 * time.Second, Attack{AttackerID: selfID, Pos: Point{X: 97, Y: 13}}},
			},
			id: selfID, check: 3 * time.Second, want: Point{X: 97, Y: 13},
		},
		{
			name: "own attack mid-walk stops the walk",
			steps: []attackStep{
				{0, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 1000}}},
				{500 * time.Millisecond, Attack{AttackerID: selfID, Pos: Point{X: 40}}},
			},
			id: selfID, check: 3 * time.Second, want: Point{X: 40},
		},
		{
			name: "a monster's attack",
			steps: []attackStep{
				{0, MoveToLocation{ObjectID: npcID, Origin: Point{X: 1000}, Dest: Point{X: 2000}}},
				{100 * time.Millisecond, Attack{AttackerID: npcID, Pos: Point{X: 1013, Y: 5}, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}},
			},
			id: npcID, check: 2 * time.Second, want: Point{X: 1013, Y: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, s := range tt.steps {
				w.Apply(s.ev, at(s.at))
			}
			m := w.motionOf(tt.id)
			now := at(tt.check)
			if got := m.at(now); got != tt.want {
				t.Errorf("position %+v, want %+v", got, tt.want)
			}
			if m.moving(now) {
				t.Errorf("still moving, want standing")
			}
		})
	}
}
