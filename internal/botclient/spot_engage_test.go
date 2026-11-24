package botclient

import (
	"testing"
	"time"
)

// TestSpotFarmWalkPastLeash checks a farmer whose farm target lies within
// the leash but behind a wall walks the whole way around it and attacks
// (#269). The walk's first segment, to the wall's end, ends beyond the
// leash: walking back to the spot from there and out again from the spot
// sent ground clicks back and forth for 20 minutes with no attack. Once the
// target is gone the leash holds again: the bot walks back from beyond it.
// Each segment is answered and run to its end, as the server does.
func TestSpotFarmWalkPastLeash(t *testing.T) {
	// The spot lies 460 west of the split navigation's second wall (world X
	// 12256-12319, Y up to 7215), the monster 380 east of it: 900 apart, the
	// straight line closed, the walk around the wall's south end.
	const mob = int32(0x20000031)
	centre, mobAt := Point{X: 11800, Y: 5000}, Point{X: 12700, Y: 5000}
	tests := []struct {
		name string
		// goneBeyond has the monster leave sight once the bot stands beyond
		// the leash.
		goneBeyond bool
		// wantRule and wantKind are the decision that ends the walk.
		wantRule string
		wantKind actionKind
	}{
		{name: "walks around and attacks", wantRule: "farm", wantKind: actAttack},
		{name: "target gone on the way: back to the spot", goneBeyond: true, wantRule: "spot", wantKind: actMove},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 3, testSpot("wall", centre.X, centre.Y, 1, 3, 9))
			b.world.Self.move = stopAt(centre)
			b.world.Objects = map[int32]*Object{}
			b.world.Apply(NPCInfo{ObjectID: mob, NPCID: 20003, Attackable: true, Pos: mobAt}, t0)
			if b.walk.lineOpen(centre, mobAt) {
				t.Fatal("the wall should close the straight line to the monster")
			}
			leash := int64(b.persona.Spots.Leash)
			now, past := at(time.Second), false
			for i := range 60 {
				if past && tt.goneBeyond {
					b.world.Apply(DeleteObject{ObjectID: mob}, now)
				}
				rule, a := b.decide(now)
				switch {
				case a.kind == actSelect:
					// The server's MyTargetSelected.
					b.world.Target = a.object
					now = now.Add(time.Second)
					continue
				case a.kind == actMove && rule == "farm":
					if ok, err := b.send(a, now); !ok || err != nil {
						t.Fatalf("decision %d: send: %v, %v", i, ok, err)
					}
					from, to := moveOrigin(a.payload), moveTarget(a.payload)
					if err := b.handle([]Event{segAnswer(from, to)}, now); err != nil {
						t.Fatal(err)
					}
					now = now.Add(segRun(b, now))
					past = past || horizontal2(to, centre) > leash*leash
					continue
				}
				if rule != tt.wantRule || a.kind != tt.wantKind {
					t.Fatalf("decision %d at %+v: rule %q action %d, want rule %q action %d", i, b.world.Self.Position(now), rule, a.kind, tt.wantRule, tt.wantKind)
				}
				if !past {
					t.Fatal("the walk never left the leash: the test no longer covers #269")
				}
				if a.kind == actAttack && a.object != mob {
					t.Fatalf("attacks %d, want the monster %d", a.object, mob)
				}
				if a.kind == actMove && b.walk.dest != centre {
					t.Fatalf("walks to %+v, want back to the spot %+v", b.walk.dest, centre)
				}
				return
			}
			t.Fatalf("no end of the walk after 60 decisions; at %+v", b.world.Self.Position(now))
		})
	}
}
