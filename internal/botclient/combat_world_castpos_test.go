package botclient

import (
	"testing"
	"time"
)

// TestWorldCasterPosition stands another caster where its MagicSkillUse says
// it casts from when the hit time proves the server stopped it first: a
// monster (ai/attackable.go thinkCast), a folk NPC (npc/folk_cast.go castOn),
// a summon or pet (ai/summon.go thinkCastLocked) and a player
// (cast/request.go stopForCast) all stop before the broadcast reads their
// position (#291, as #246 for Attack). A shorter cast leaves the walk alone,
// and the bot's own cast is the brain's to stand (Brain.standCaster).
func TestWorldCasterPosition(t *testing.T) {
	const summonID = int32(0x20000003)
	walking := []struct {
		id         int32
		origin, to Point
	}{
		{npcID, Point{X: 1000}, Point{X: 2000}},
		{pcID, Point{Y: 1000}, Point{Y: 2000}},
		{summonID, Point{X: -1000}, Point{X: -2000}},
		{selfID, Point{}, Point{X: -1000, Y: -1000}},
	}
	tests := []struct {
		name       string
		cast       MagicSkillUse
		id         int32
		want       Point
		wantMoving bool
	}{
		{
			name: "monster",
			cast: MagicSkillUse{CasterID: npcID, TargetID: selfID, SkillID: 1177, HitTime: 2 * time.Second, CasterPos: Point{X: 1213, Y: 5}},
			id:   npcID, want: Point{X: 1213, Y: 5},
		},
		{
			name: "monster casting at another",
			cast: MagicSkillUse{CasterID: npcID, TargetID: pcID, SkillID: 1177, HitTime: castStopsFrom, CasterPos: Point{X: 1190}},
			id:   npcID, want: Point{X: 1190},
		},
		{
			name: "summon",
			cast: MagicSkillUse{CasterID: summonID, TargetID: npcID, SkillID: 1177, HitTime: time.Second, CasterPos: Point{X: -1150}},
			id:   summonID, want: Point{X: -1150},
		},
		{
			name: "player",
			cast: MagicSkillUse{CasterID: pcID, TargetID: pcID, SkillID: 1040, HitTime: 4 * time.Second, CasterPos: Point{X: 3, Y: 1090}},
			id:   pcID, want: Point{X: 3, Y: 1090},
		},
		{
			name: "short cast keeps walking",
			cast: MagicSkillUse{CasterID: npcID, TargetID: selfID, SkillID: 1177, HitTime: castStopsFrom - time.Millisecond, CasterPos: Point{X: 1190}},
			id:   npcID, want: Point{X: 1400}, wantMoving: true,
		},
		{
			name: "own cast is not the world's to stand",
			cast: MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: 1177, HitTime: 2 * time.Second, CasterPos: Point{X: -50}},
			id:   selfID, want: Point{X: -141, Y: -141}, wantMoving: true,
		},
		{
			name: "caster out of sight",
			cast: MagicSkillUse{CasterID: 0x20000099, TargetID: selfID, SkillID: 1177, HitTime: 2 * time.Second, CasterPos: Point{X: 7}},
			id:   npcID, want: Point{X: 1400}, wantMoving: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			w.Apply(NPCInfo{ObjectID: summonID, NPCID: 12077, Summon: true, Pos: Point{X: -1000}, RunSpeed: 200, Running: true}, t0)
			for _, m := range walking {
				w.Apply(MoveToLocation{ObjectID: m.id, Origin: m.origin, Dest: m.to}, t0)
			}
			cast := at(time.Second)
			w.Apply(tt.cast, cast)
			m := w.motionOf(tt.id)
			now := at(2 * time.Second)
			if got := m.at(now); got != tt.want {
				t.Errorf("position %+v, want %+v", got, tt.want)
			}
			if got := m.moving(now); got != tt.wantMoving {
				t.Errorf("moving %v, want %v", got, tt.wantMoving)
			}
		})
	}
}
