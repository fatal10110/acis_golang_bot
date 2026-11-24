package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// newTrainerWalkBrain is a level 15 human mystic at the origin with no
// trainer in sight and trainer 30022 spawned at (5000, 0) on its travel
// map.
func newTrainerWalkBrain(t *testing.T) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	p.Buffs = []Buff{}
	p.Newbie.MaxLevel = 0
	b, _ := newTestBrain(t, p)
	s := &b.world.Self
	s.ClassID, s.Level, s.SP = classHumanMystic, 15, 5000
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{30022: {{X: 5000}}}}
	return walkOn(t, b)
}

// TestBrainTrainerWalk walks a bot due a visit to its trainer's spawn and
// talks to the trainer once it is in sight.
func TestBrainTrainerWalk(t *testing.T) {
	b := newTrainerWalkBrain(t)
	runTrainerSteps(t, b, []trainerStep{
		{name: "a segment toward the trainer's spawn", at: time.Second, wantRule: "train", wantKind: actMove},
		{name: "walking", at: 2 * time.Second, setup: func(b *Brain) {
			b.world.Self.move = motion{from: Point{}, to: Point{X: 3000}, start: at(time.Second), speed: 100}
		}, wantRule: "train"},
		{
			name: "trainer in sight: select", at: 40 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) },
			events:   []Event{NPCInfo{ObjectID: trainerObj, NPCID: 30022, Pos: Point{X: 5000}}},
			wantRule: "train", wantKind: actSelect,
		},
		{name: "talk", at: 41 * time.Second, events: []Event{MyTargetSelected{ObjectID: trainerObj}}, wantRule: "train", wantKind: actTalk},
	})
	if b.train.npc != trainerObj || b.train.walk.active() {
		t.Fatalf("train = npc %d walk %+v, want the visit at the trainer and no walk", b.train.npc, b.train.walk)
	}
}

// TestTrainerWalkDue checks when a visit walks to a trainer's spawn.
func TestTrainerWalkDue(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(b *Brain)
		wantWalk bool
	}{
		{name: "spawn in reach", wantWalk: true},
		{name: "spawn beyond reach", setup: func(b *Brain) { b.persona.TrainerReach = 4999 }},
		{name: "persona walks to none", setup: func(b *Brain) { b.persona.TrainerReach = 0 }},
		{name: "no travel map", setup: func(b *Brain) { b.cfg.Travel = nil }},
		{name: "in a party", setup: func(b *Brain) { b.world.Party.LeaderID = selfID }},
		{name: "told to follow a commander", setup: func(b *Brain) { b.orders.follow = 7 }},
		{name: "visited at this level", setup: func(b *Brain) { b.train.doneLevel = 15 }},
		{name: "SP reached a wanted skill", setup: func(b *Brain) { b.train.doneLevel, b.train.needSP = 15, 5000 }, wantWalk: true},
		{name: "class learns nothing", setup: func(b *Brain) { b.world.Self.ClassID = 0 }},
		{name: "trainer template shunned", setup: func(b *Brain) {
			b.train.walkShunned = map[int32]time.Time{30022: at(time.Hour)}
		}},
		{name: "another class's trainer", setup: func(b *Brain) {
			b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{30031: {{X: 5000}}}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTrainerWalkBrain(t)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(at(time.Second))
			if got := rule == "train" && a.kind == actMove; got != tt.wantWalk {
				t.Fatalf("decide = %q %d, want walk %v", rule, a.kind, tt.wantWalk)
			}
			if b.train.walk.active() != tt.wantWalk {
				t.Fatalf("walk %+v, want active %v", b.train.walk, tt.wantWalk)
			}
		})
	}
}

// TestTrainerWalkGivesUp checks a walk that finds no trainer, or no route
// to its spawn, leaves the template alone, and one the bot was teleported
// away from is dropped.
func TestTrainerWalkGivesUp(t *testing.T) {
	tests := []struct {
		name      string
		steps     []trainerStep
		wantShun  bool
		wantAfter bool // a walk runs after the steps
	}{
		{
			name: "trainer not at its spawn",
			steps: []trainerStep{
				{name: "leg", at: time.Second, wantRule: "train", wantKind: actMove},
				{name: "nobody there", at: 40 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) }},
			},
			wantShun: true,
		},
		{
			// The spawn lies inside the walled pocket: no route there.
			name: "spawn walled off",
			steps: []trainerStep{
				{name: "unreachable", at: time.Second, setup: func(b *Brain) {
					b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{30022: {scenePoint(geotest.PocketInside, 0, 0)}}}
					b.persona.TrainerReach = 1 << 20
				}},
			},
			wantShun: true,
		},
		{
			name: "teleported out of reach",
			steps: []trainerStep{
				{name: "leg", at: time.Second, wantRule: "train", wantKind: actMove},
				{name: "far away", at: 2 * time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 50000}) }},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTrainerWalkBrain(t)
			runTrainerSteps(t, b, tt.steps)
			if b.train.walk.active() != tt.wantAfter || b.train.walkShunned[30022].IsZero() == tt.wantShun {
				t.Fatalf("walk %+v shunned %v, want walk %v shunned %v",
					b.train.walk, b.train.walkShunned, tt.wantAfter, tt.wantShun)
			}
		})
	}
}
