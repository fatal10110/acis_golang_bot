package botclient

import (
	"math"
	"testing"
	"time"
)

// TestUnstickVisit has a bot whose legs toward an NPC's spawn stall where it
// stands, the server echoing its own position (#122): it sidesteps left,
// right and back, then asks /unstuck once and stands still through the
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
	stuck := append(legs[:3:3],
		trainerStep{
			name: "left", at: 4 * time.Second, events: echo, wantRule: "newbie", wantKind: actMove,
			wantPayload: encodeMoveBackwardToLocation(Point{Y: unstickStep}, Point{}),
		},
		trainerStep{
			name: "right", at: 5 * time.Second, events: echo, wantRule: "newbie", wantKind: actMove,
			wantPayload: encodeMoveBackwardToLocation(Point{Y: -unstickStep}, Point{}),
		},
		trainerStep{
			name: "back", at: 6 * time.Second, events: echo, wantRule: "newbie", wantKind: actMove,
			wantPayload: encodeMoveBackwardToLocation(Point{X: -unstickStep}, Point{}),
		},
		trainerStep{
			name: "/unstuck", at: 7 * time.Second, events: echo, wantRule: "newbie", wantKind: actUserCommand,
			wantPayload: encodeRequestUserCommand(userCommandEscape),
		},
	)
	recall := MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: 2099, Level: 1, HitTime: 5 * time.Minute}
	tests := []struct {
		name    string
		steps   []trainerStep
		wantEnd bool
	}{
		{
			name: "stuck: sidesteps, /unstuck, then the visit is over",
			steps: append(stuck[:len(stuck):len(stuck)],
				trainerStep{name: "no cast yet", at: 8 * time.Second, wantRule: "newbie"},
				trainerStep{name: "refused: given up", at: 7*time.Second + escapeRetry},
			),
			wantEnd: true,
		},
		{
			name: "recall cast: stands still, then legs from town",
			steps: append(stuck[:len(stuck):len(stuck)],
				trainerStep{name: "casting", at: 8 * time.Second, events: []Event{recall}, wantRule: "newbie"},
				trainerStep{name: "still casting", at: 4 * time.Minute, wantRule: "newbie"},
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
		n    int
		want Point
	}{
		{1, Point{X: -140, Y: 280, Z: -3717}},
		{2, Point{X: 340, Y: -80, Z: -3717}},
		{3, Point{X: -80, Y: -140, Z: -3717}},
	}
	for _, tt := range tests {
		if got := sidestep(from, to, 500, tt.n); got != tt.want {
			t.Errorf("sidestep %d = %+v, want %+v", tt.n, got, tt.want)
		}
	}
}
