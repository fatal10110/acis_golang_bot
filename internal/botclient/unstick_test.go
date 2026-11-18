package botclient

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// TestUnstickVisit has a bot whose legs toward an NPC's spawn stall where it
// stands, the server echoing its own position (#122): it sidesteps in seven
// directions, then asks /unstuck once and stands still through the
// recall cast, before it gives the visit up. A sidestep or a recall that
// moves it sends the legs again; legs that move it no closer give up at once.
func TestUnstickVisit(t *testing.T) {
	echo := []Event{MoveToLocation{ObjectID: selfID}}
	leg := encodeMoveBackwardToLocation(Point{X: 3000}, Point{})
	legs := []trainerStep{
		{name: "leg 1", at: time.Second, wantRule: "newbie", wantKind: actMove, wantPayload: leg},
		{name: "leg 2", at: 2 * time.Second, events: echo, wantRule: "newbie", wantKind: actMove, wantPayload: leg},
		{name: "leg 3", at: 3 * time.Second, events: echo, wantRule: "newbie", wantKind: actMove, wantPayload: leg},
	}
	stuck := legs[:3:3]
	for i, to := range []Point{
		{Y: 300}, {Y: -300}, {X: -300}, {X: -212, Y: 212}, {X: -212, Y: -212}, {X: 212, Y: 212}, {X: 212, Y: -212},
	} {
		stuck = append(stuck, trainerStep{
			name: fmt.Sprintf("sidestep %d", i+1), at: time.Duration(4+i) * time.Second, events: echo,
			wantRule: "newbie", wantKind: actMove, wantPayload: encodeMoveBackwardToLocation(to, Point{}),
		})
	}
	escapeAt := 11 * time.Second
	stuck = append(stuck, trainerStep{
		name: "/unstuck", at: escapeAt, events: echo, wantRule: "newbie", wantKind: actUserCommand,
		wantPayload: encodeRequestUserCommand(userCommandEscape),
	})
	recall := MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: 2099, Level: 1, HitTime: 5 * time.Minute}
	tests := []struct {
		name    string
		steps   []trainerStep
		wantEnd bool
	}{
		{
			name: "stuck: sidesteps, /unstuck, then the visit is over",
			steps: append(stuck[:len(stuck):len(stuck)],
				trainerStep{name: "no cast yet", at: escapeAt + time.Second, wantRule: "newbie"},
				trainerStep{name: "refused: given up", at: escapeAt + escapeRetry},
			),
			wantEnd: true,
		},
		{
			name: "recall cast: stands still, then legs from town",
			steps: append(stuck[:len(stuck):len(stuck)],
				trainerStep{name: "casting", at: escapeAt + time.Second, events: []Event{recall}, wantRule: "newbie"},
				trainerStep{name: "still casting", at: 4 * time.Minute, wantRule: "newbie"},
				trainerStep{name: "cast over, recall not landed yet", at: escapeAt + time.Second + 5*time.Minute + time.Second, wantRule: "newbie"},
				trainerStep{
					name: "landed: leg", at: 6 * time.Minute, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 1000}) },
					wantRule: "newbie", wantKind: actMove, wantPayload: encodeMoveBackwardToLocation(Point{X: 4000}, Point{X: 1000}),
				},
			),
		},
		{
			name: "a sidestep moves the bot: legs again",
			steps: append(stuck[:4:4],
				trainerStep{
					name: "freed: leg", at: 10 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 100, Y: 300}) },
					wantRule: "newbie", wantKind: actMove,
					wantPayload: encodeMoveBackwardToLocation(legToward(Point{X: 100, Y: 300}, Point{X: 5000}, math.Hypot(4900, 300)), Point{X: 100, Y: 300}),
				},
			),
		},
		{
			name: "legs move the bot no closer: given up",
			steps: append(legs[:3:3],
				trainerStep{name: "stalled", at: 4 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{Y: 200}) }},
			),
			wantEnd: true,
		},
		{
			name: "sitting while stuck: stands up first",
			steps: append(legs[:3:3],
				trainerStep{name: "stand", at: 4 * time.Second, setup: func(b *Brain) { b.world.Self.Sitting = true }, wantRule: "newbie", wantKind: actSitStand},
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newNewbieBrain(t, 10)
			runTrainerSteps(t, b, tt.steps)
			if ended := !b.newbie.v.active(); ended != tt.wantEnd {
				t.Fatalf("visit ended = %v, want %v", ended, tt.wantEnd)
			}
		})
	}
}

// TestSidestep checks the sidesteps' ends against a target off the axes.
func TestSidestep(t *testing.T) {
	from, to := Point{X: 100, Y: 100, Z: -3717}, Point{X: 400, Y: 500, Z: -3000}
	tests := []struct {
		turn float64
		want Point
	}{
		{90, Point{X: -140, Y: 280, Z: -3717}},
		{-90, Point{X: 340, Y: -80, Z: -3717}},
		{180, Point{X: -80, Y: -140, Z: -3717}},
		{45, Point{X: 58, Y: 397, Z: -3717}},
	}
	for _, tt := range tests {
		if got := sidestep(from, to, 500, tt.turn); got != tt.want {
			t.Errorf("sidestep %v = %+v, want %+v", tt.turn, got, tt.want)
		}
	}
}
