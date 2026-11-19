package botclient

import (
	"fmt"
	"testing"
	"time"
)

// TestTrainerWalkStuck has a bot whose legs toward its trainer's spawn
// stall where it stands, with a second trainer's spawn beside the first
// (#134): once its sidesteps and its /unstuck are spent, refused or landing
// it back where its legs stall, it walks to no trainer before the next
// level, the second spawn's included, and walks again once it levels up.
func TestTrainerWalkStuck(t *testing.T) {
	echo := []Event{MoveToLocation{ObjectID: selfID}}
	var stuck []trainerStep
	for i := range 3 {
		st := trainerStep{name: fmt.Sprintf("leg %d", i+1), at: time.Duration(1+i) * time.Second, wantRule: "train", wantKind: actMove}
		if i > 0 {
			st.events = echo
		}
		stuck = append(stuck, st)
	}
	for i := range len(sidestepTurns) {
		stuck = append(stuck, trainerStep{
			name: fmt.Sprintf("sidestep %d", i+1), at: time.Duration(4+i) * time.Second, events: echo,
			wantRule: "train", wantKind: actMove,
		})
	}
	escapeAt := 11 * time.Second
	stuck = append(stuck, trainerStep{
		name: "/unstuck", at: escapeAt, events: echo, wantRule: "train", wantKind: actUserCommand,
		wantPayload: encodeRequestUserCommand(userCommandEscape),
	})
	recall := MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: 2099, Level: 1, HitTime: 5 * time.Minute}
	// The recall lands the bot in its restart area, beside where it stood.
	landed := Point{X: 40}
	echoLanded := []Event{MoveToLocation{ObjectID: selfID, Origin: landed, Dest: landed}}
	land := 6 * time.Minute
	tests := []struct {
		name  string
		steps []trainerStep
		// end is when the walk was given up.
		end time.Duration
	}{
		{
			name: "/unstuck refused",
			steps: append(stuck[:len(stuck):len(stuck)],
				trainerStep{name: "no cast yet", at: escapeAt + time.Second, wantRule: "train"},
				trainerStep{name: "refused: given up", at: escapeAt + escapeRetry},
			),
			end: escapeAt + escapeRetry,
		},
		{
			name: "recall back where the legs stall",
			steps: append(stuck[:len(stuck):len(stuck)],
				trainerStep{name: "casting", at: escapeAt + time.Second, events: []Event{recall}, wantRule: "train"},
				trainerStep{
					name: "landed: leg", at: land, setup: func(b *Brain) { b.world.Self.move = stopAt(landed) },
					wantRule: "train", wantKind: actMove,
				},
				trainerStep{name: "leg 2", at: land + time.Second, events: echoLanded, wantRule: "train", wantKind: actMove},
				trainerStep{name: "leg 3", at: land + 2*time.Second, events: echoLanded, wantRule: "train", wantKind: actMove},
				trainerStep{name: "stalled again: given up", at: land + 3*time.Second, events: echoLanded},
			),
			end: land + 3*time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTrainerWalkBrain(t)
			b.cfg.Travel.spawns[30030] = []Point{{X: 5000, Y: 500}}
			runTrainerSteps(t, b, tt.steps)
			runTrainerSteps(t, b, []trainerStep{
				{name: "no walk to the other spawn", at: tt.end + time.Second},
				{name: "nor after the shuns", at: tt.end + visitShunFor + time.Minute},
			})
			if b.train.walk.active() || b.train.stuckLevel != 15 {
				t.Fatalf("walk %+v stuck at level %d, want none and 15", b.train.walk, b.train.stuckLevel)
			}
			runTrainerSteps(t, b, []trainerStep{{
				name: "level up: walks again", at: tt.end + visitShunFor + 2*time.Minute,
				setup:    func(b *Brain) { b.world.Self.Level = 16 },
				wantRule: "train", wantKind: actMove,
			}})
		})
	}
}
