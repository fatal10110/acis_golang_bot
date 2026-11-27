package botclient

import (
	"testing"
	"time"
)

// TestTrainerVisitOwnObjectOnly has another row click or bypass another NPC
// while a trainer visit runs: the visit counts only clicks on its trainer and
// waits only on its trainer's packets, so the trainer is still talked to and
// is not shunned (D24, GM2).
func TestTrainerVisitOwnObjectOnly(t *testing.T) {
	tests := []struct {
		name  string
		other func(from Point) Action
	}{
		{name: "talk of another NPC", other: func(from Point) Action { return talkAction(npcID, from) }},
		{name: "select of another NPC", other: func(from Point) Action { return selectAction(npcID, from) }},
		{name: "bypass of another NPC", other: func(Point) Action { return bypassTo(npcID, "npc_1_Chat 0") }},
		{name: "bypass of no NPC", other: func(Point) Action { return bypassAction("menu_select?ask=-1") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTrainerBrain(t, 500)
			runTrainerSteps(t, b, []trainerStep{
				{name: "select the trainer", wantRule: "train", wantKind: actSelect},
			})
			// More clicks than a visit spends, each a second apart to pass
			// the frame pacer.
			for i := range visitClicks {
				now := at(time.Duration(i+1) * time.Second)
				if sent, err := b.send(tt.other(b.world.Self.Position(now)), now); err != nil || !sent {
					t.Fatalf("other %d: send = %v, %v", i, sent, err)
				}
			}
			runTrainerSteps(t, b, []trainerStep{
				{
					name: "talk to the trainer", at: time.Duration(visitClicks+1) * time.Second,
					events: []Event{MyTargetSelected{ObjectID: trainerObj}}, wantRule: "train", wantKind: actTalk,
				},
			})
			if b.train.npc != trainerObj || b.train.shunned.held(trainerObj, at(time.Minute)) {
				t.Errorf("visit at %d, shunned %v; want the visit at the trainer, not shunned", b.train.npc, b.train.shunned)
			}
		})
	}
}
