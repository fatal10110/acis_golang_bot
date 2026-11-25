package botclient

import (
	"testing"
	"time"
)

// TestLeaderLostGroundLoot pins the order the live leader-logout scenario
// depends on (#471): rule "loot" runs ahead of rule "leader", so a bot whose
// leader logged out where other players left ground items picks them up
// past its LeaderWait instead of whispering and casting /unstuck (live: 19
// pickups in a minute on the GM's open ground, rule "loot" to the end). A
// persona that loots only its own fights' drops (GroundNone) leaves them,
// and rule "leader" whispers, then casts /unstuck.
func TestLeaderLostGroundLoot(t *testing.T) {
	litter := []Event{
		SpawnItem{Item: adenaOnGround},
		SpawnItem{Item: GroundItem{ObjectID: adenaDrop + 100, TemplateID: 57, Pos: Point{X: 600, Y: 300}, Stackable: true, Count: 40}},
	}
	tests := []struct {
		name   string
		ground GroundLoot
		// want is the rule in charge after LeaderWait, then after chatGap,
		// and kinds what each sends.
		want  [2]string
		kinds [2]actionKind
	}{
		{
			name: "a fitting persona loots ahead of its lost leader", ground: GroundFit,
			want: [2]string{"loot", "loot"}, kinds: [2]actionKind{actPickup, actPickup},
		},
		{
			name: "GroundNone whispers, then casts /unstuck", ground: GroundNone,
			want: [2]string{"leader", "leader"}, kinds: [2]actionKind{actTell, actUserCommand},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeaderBrain(t)
			b.persona.Loot.Ground = tt.ground
			deathApply(t, b, t0, litter...)
			lost := at(time.Second)
			deathApply(t, b, lost, PartySmallWindowDeleteAll{}, DeleteObject{ObjectID: gmID})
			now := lost.Add(b.persona.Recover.LeaderWait)
			for i := range tt.want {
				if name, a := deathStep(t, b, now); name != tt.want[i] || a.kind != tt.kinds[i] {
					t.Fatalf("step %d: %q kind %v, want %q kind %v", i, name, a.kind, tt.want[i], tt.kinds[i])
				}
				now = now.Add(chatGap)
			}
		})
	}
}
