package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestVisitClosesIn walks a bot toward an NPC in sight but far off: a click
// on the NPC walks the bot straight at it, never routed, and a closed line
// ends that walk at the first wall (creature.go, MoveToPawn), so the bot
// first walks through the walker to a stand point within talking range with
// an open line, and clicks only from there.
func TestVisitClosesIn(t *testing.T) {
	npc := Point{X: 5000}
	inSight := func(b *Brain) {
		b.world.Apply(NPCInfo{ObjectID: guideObj, NPCID: 30598, Pos: npc}, t0)
	}
	tests := []struct {
		name  string
		steps []trainerStep
	}{
		{
			name: "far off: a walk to a stand point, then clicks once close",
			steps: []trainerStep{
				{name: "segment", at: time.Second, setup: inSight, wantRule: "newbie", wantKind: actMove},
				{
					name: "last segment", at: 40 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4000}) },
					wantRule: "newbie", wantKind: actMove, wantTo: npc, wantWithin: visitTalkRange,
				},
				{
					name: "close: select", at: 70 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4880}) },
					wantRule: "newbie", wantKind: actSelect,
				},
			},
		},
		{
			// The NPC stands inside the walled pocket: no stand point the
			// bot reaches, so the clicks are tried, bounded by visitClicks.
			name: "no stand point holds: clicks",
			steps: []trainerStep{
				{name: "select", at: time.Second, setup: func(b *Brain) {
					b.world.Apply(NPCInfo{ObjectID: guideObj, NPCID: 30598, Pos: scenePoint(geotest.PocketInside, 0, 0)}, t0)
					b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{30598: {scenePoint(geotest.PocketInside, 0, 0)}}}
				}, wantRule: "newbie", wantKind: actSelect},
			},
		},
		{
			name: "out of sight on the way: a walk to the spawn",
			steps: []trainerStep{
				{name: "segment", at: time.Second, setup: inSight, wantRule: "newbie", wantKind: actMove},
				{
					name: "gone: a segment toward the spawn", at: 4 * time.Second, events: []Event{DeleteObject{ObjectID: guideObj}},
					wantRule: "newbie", wantKind: actMove,
				},
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
		if sent, err := b.send(Action{kind: actAppear, payload: botwire.EncodeAppearing()}, now); err != nil || !sent {
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
