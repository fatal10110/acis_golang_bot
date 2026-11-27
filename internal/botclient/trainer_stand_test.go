package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestTrainerClosesIn has a trainer in sight farther than visitArrive: the
// click would walk the bot straight at it, never routed, so the visit walks
// to a stand point with an open line first, then clicks; with no stand
// point the bot reaches, the clicks are tried, bounded by visitClicks.
func TestTrainerClosesIn(t *testing.T) {
	trainer := Point{X: 5000}
	tests := []struct {
		name  string
		at    Point
		steps []trainerStep
	}{
		{
			name: "far off: a walk to a stand point, then clicks once close",
			at:   trainer,
			steps: []trainerStep{
				{name: "segment", at: time.Second, wantRule: "train", wantKind: actMove, wantTo: trainer, wantWithin: visitTalkRange},
				{
					name: "close: select", at: 70 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4880}) },
					wantRule: "train", wantKind: actSelect,
				},
				{name: "talk", at: 71 * time.Second, events: []Event{MyTargetSelected{ObjectID: trainerObj}}, wantRule: "train", wantKind: actTalk},
			},
		},
		{
			// The trainer stands inside the walled pocket.
			name: "no stand point holds: clicks",
			at:   scenePoint(geotest.PocketInside, 0, 0),
			steps: []trainerStep{
				{name: "select", at: time.Second, wantRule: "train", wantKind: actSelect},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTrainerWalkBrain(t)
			b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{30022: {tt.at}}, trainers: testTrainers}
			b.world.Apply(NPCInfo{ObjectID: trainerObj, NPCID: 30022, Pos: tt.at}, t0)
			runTrainerSteps(t, b, tt.steps)
		})
	}
}

// TestTrainerOutOfSightWalksOn has the trainer the visit closes in on leave
// sight (seen from far, as from the hill above Talking Island): the same
// decision walks on to the trainer's spawn, with no decision off the
// errand.
func TestTrainerOutOfSightWalksOn(t *testing.T) {
	trainer := Point{X: 5000}
	b := newTrainerWalkBrain(t)
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{30022: {trainer}}, trainers: testTrainers}
	b.world.Apply(NPCInfo{ObjectID: trainerObj, NPCID: 30022, Pos: trainer}, t0)
	runTrainerSteps(t, b, []trainerStep{
		{name: "segment to the stand point", at: time.Second, wantRule: "train", wantKind: actMove, wantTo: trainer, wantWithin: visitTalkRange},
		{
			name: "out of sight: the walk to the spawn", at: 70 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 1000}) },
			events:   []Event{DeleteObject{ObjectID: trainerObj}},
			wantRule: "train", wantKind: actMove, wantTo: trainer, wantWithin: visitArrive,
		},
	})
}
