package botclient

import (
	"testing"
	"time"
)

// TestBrainCastPosition stands the bot exactly where its own MagicSkillUse
// says it casts from, for a cast it asked for whose hit time makes the
// server stop it first (cast/request.go stopForCast, before
// network/magic_skill.go broadcastCastStart reads the caster's position). A
// dead-reckoned walk's end may miss that spot by a step (a cast approach's
// pawn walk ends on the first step in range), and the walker's next segment
// goes out from there (#270, as #246 for Attack). A cast the bot did not ask
// for (a cubic's, a class change's) or one too short to stop it leaves the
// walk alone; so does another caster's.
func TestBrainCastPosition(t *testing.T) {
	const skill = int32(1177)
	type castStep struct {
		at time.Duration
		ev Event
	}
	tests := []struct {
		name string
		// asked is the skill of the cast the bot asked for at 0, none for 0.
		asked      int32
		steps      []castStep
		id         int32
		check      time.Duration
		want       Point
		wantMoving bool
	}{
		{
			name:  "own cast a step off the cast approach's dead-reckoned end",
			asked: skill,
			steps: []castStep{
				{0, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 100}}},
				{2 * time.Second, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skill, HitTime: time.Second, CasterPos: Point{X: 97, Y: 13}}},
			},
			id: selfID, check: 3 * time.Second, want: Point{X: 97, Y: 13},
		},
		{
			name:  "own cast mid-walk stops the walk",
			asked: skill,
			steps: []castStep{
				{0, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 1000}}},
				{500 * time.Millisecond, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skill, HitTime: castStopsFrom, CasterPos: Point{X: 40}}},
			},
			id: selfID, check: 3 * time.Second, want: Point{X: 40},
		},
		{
			name:  "own instant cast keeps walking",
			asked: skill,
			steps: []castStep{
				{0, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 1000}}},
				{500 * time.Millisecond, MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: skill, HitTime: castStopsFrom - time.Millisecond, CasterPos: Point{X: 40}}},
			},
			id: selfID, check: 3 * time.Second, want: Point{X: 300}, wantMoving: true,
		},
		{
			name: "own cast the bot did not ask for keeps walking",
			steps: []castStep{
				{0, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 1000}}},
				{500 * time.Millisecond, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skill, HitTime: 2 * time.Second, CasterPos: Point{X: 40}}},
			},
			id: selfID, check: 3 * time.Second, want: Point{X: 300}, wantMoving: true,
		},
		{
			name:  "own cast of another skill than asked keeps walking",
			asked: skill + 1,
			steps: []castStep{
				{0, MoveToLocation{ObjectID: selfID, Origin: Point{}, Dest: Point{X: 1000}}},
				{500 * time.Millisecond, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skill, HitTime: 2 * time.Second, CasterPos: Point{X: 40}}},
			},
			id: selfID, check: 3 * time.Second, want: Point{X: 300}, wantMoving: true,
		},
		{
			name:  "a monster's cast",
			asked: skill,
			steps: []castStep{
				{0, MoveToLocation{ObjectID: npcID, Origin: Point{X: 1000}, Dest: Point{X: 2000}}},
				{time.Second, MagicSkillUse{CasterID: npcID, TargetID: selfID, SkillID: skill, HitTime: 2 * time.Second, CasterPos: Point{X: 1013, Y: 5}}},
			},
			id: npcID, check: 3 * time.Second, want: Point{X: 1600}, wantMoving: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBrain(t, Persona{})
			if tt.asked != 0 {
				b.castAt, b.castSkill = t0, tt.asked
			}
			var cast MagicSkillUse
			for _, s := range tt.steps {
				if e, ok := s.ev.(MagicSkillUse); ok {
					cast = e
				}
				if err := b.handle([]Event{s.ev}, at(s.at)); err != nil {
					t.Fatal(err)
				}
			}
			m := b.world.motionOf(tt.id)
			now := at(tt.check)
			if got := m.at(now); got != tt.want {
				t.Errorf("position %+v, want %+v", got, tt.want)
			}
			if got := m.moving(now); got != tt.wantMoving {
				t.Errorf("moving %v, want %v", got, tt.wantMoving)
			}
			if cast.CasterID != selfID {
				return
			}
			// The cast and its reuse start as before.
			castAt := at(tt.steps[len(tt.steps)-1].at)
			if want := castAt.Add(cast.HitTime); !b.world.Self.castEnd.Equal(want) {
				t.Errorf("cast ends %v, want %v", b.world.Self.castEnd, want)
			}
			if b.world.SkillReady(skill, castAt.Add(cast.HitTime-time.Millisecond)) {
				t.Errorf("skill %d ready before its cast hits, want held", skill)
			}
			if !b.castAt.IsZero() {
				t.Errorf("cast still awaits its answer, want answered")
			}
		})
	}
}
