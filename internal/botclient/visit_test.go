package botclient

import (
	"testing"
	"time"
)

// TestVisitClosesIn walks a bot toward an NPC in sight but far off with
// ground moves, which the server routes around walls, before it clicks: the
// approach of a click goes straight and ends at the first wall.
func TestVisitClosesIn(t *testing.T) {
	inSight := func(b *Brain) {
		b.world.Apply(NPCInfo{ObjectID: guideObj, NPCID: 30598, Pos: Point{X: 5000}}, t0)
	}
	leg := encodeMoveBackwardToLocation(Point{X: 3000}, Point{})
	tests := []struct {
		name  string
		steps []trainerStep
	}{
		{
			name: "far off: legs, then clicks once close",
			steps: []trainerStep{
				{name: "leg", at: time.Second, setup: inSight, wantRule: "newbie", wantKind: actMove, wantPayload: leg},
				{name: "walking", at: 2 * time.Second, setup: func(b *Brain) {
					b.world.Self.move = motion{from: Point{}, to: Point{X: 3000}, start: at(time.Second), speed: 100}
				}, wantRule: "newbie"},
				{
					name: "last leg", at: 40 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 3000}) },
					wantRule: "newbie", wantKind: actMove, wantPayload: encodeMoveBackwardToLocation(Point{X: 5000}, Point{X: 3000}),
				},
				{
					name: "close: select", at: 70 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4750}) },
					wantRule: "newbie", wantKind: actSelect,
				},
			},
		},
		{
			name: "legs get nowhere: clicks",
			steps: []trainerStep{
				{name: "leg 1", at: time.Second, setup: inSight, wantRule: "newbie", wantKind: actMove, wantPayload: leg},
				{name: "leg 2", at: 2 * time.Second, wantRule: "newbie", wantKind: actMove, wantPayload: leg},
				{name: "leg 3", at: 3 * time.Second, wantRule: "newbie", wantKind: actMove, wantPayload: leg},
				{name: "select", at: 4 * time.Second, wantRule: "newbie", wantKind: actSelect},
			},
		},
		{
			name: "sitting: stands up first",
			steps: []trainerStep{
				{name: "stand", at: time.Second, setup: func(b *Brain) { inSight(b); b.world.Self.Sitting = true }, wantRule: "newbie", wantKind: actSitStand},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runTrainerSteps(t, newNewbieBrain(t, 8), tt.steps)
		})
	}
}

// TestVisitLooksAfterLanding has a bot land at an NPC's spawn with nothing
// in sight yet: a loading-screen teleport empties the world, and the server
// shows the objects around the landing point only after the Appearing. The
// visit waits visitLook for the NPC before it calls the NPC absent.
func TestVisitLooksAfterLanding(t *testing.T) {
	land := func(b *Brain) {
		now := at(time.Second)
		if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5000}}}, now); err != nil {
			t.Fatal(err)
		}
		if sent, err := b.send(Action{kind: actAppear, payload: encodeAppearing()}, now); err != nil || !sent {
			t.Fatalf("appear: %v, %v", sent, err)
		}
	}
	tests := []struct {
		name  string
		steps []trainerStep
	}{
		{
			name: "the NPC shows: select",
			steps: []trainerStep{
				{name: "landed, nothing in sight", at: time.Second, setup: land, wantRule: "newbie"},
				{name: "still looking", at: 3 * time.Second, wantRule: "newbie"},
				{
					name: "in sight", at: 3 * time.Second, events: []Event{NPCInfo{ObjectID: guideObj, NPCID: 30598, Pos: Point{X: 5100}}},
					wantRule: "newbie", wantKind: actSelect,
				},
			},
		},
		{
			name: "the NPC never shows: absent",
			steps: []trainerStep{
				{name: "landed, nothing in sight", at: time.Second, setup: land, wantRule: "newbie"},
				{name: "gone", at: time.Second + visitLook, wantRule: ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runTrainerSteps(t, newNewbieBrain(t, 8), tt.steps)
		})
	}
}
