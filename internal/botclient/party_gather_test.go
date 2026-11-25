package botclient

import (
	"testing"
	"time"
)

// TestPartyGatherGate: at its spot a leader picks no new farm target while
// a living bot member was last reported farther than GatherRange (1200)
// from it, for at most GatherWait (60 s); one beyond FarAway (10000), dead,
// with no report yet, or a player holds nothing (docs/parties/spots.md,
// "Gather gate"). The wait is no idle stretch.
func TestPartyGatherGate(t *testing.T) {
	const mob = 0x5001
	type step struct {
		d    time.Duration
		farm bool
	}
	tests := []struct {
		name  string
		id    int32
		pos   Point
		dead  bool
		quiet bool
		steps []step
	}{
		{name: "a member at 2000 holds it 60 s", id: abelID, pos: Point{X: 4000}, steps: []step{{2 * time.Second, false}, {61 * time.Second, false}, {62 * time.Second, true}}},
		{name: "a member at 1000 holds nothing", id: abelID, pos: Point{X: 3000}, steps: []step{{2 * time.Second, true}}},
		{name: "a member at 12000 holds nothing", id: abelID, pos: Point{X: 14000}, steps: []step{{2 * time.Second, true}}},
		{name: "a dead member holds nothing", id: abelID, pos: Point{X: 4000}, dead: true, steps: []step{{2 * time.Second, true}}},
		{name: "a member with no report holds nothing", id: abelID, quiet: true, steps: []step{{2 * time.Second, true}}},
		{name: "a player at 2000 holds nothing", id: talbotID, pos: Point{X: 4000}, steps: []step{{2 * time.Second, true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mate(tt.id, 1)
			if tt.dead {
				m.HP = 0
			}
			b := newLeadSpotBrain(t, 1, []PartyMember{m}, testSpot("a", 2000, 0, 1, 3, 9))
			b.world.Objects = map[int32]*Object{}
			spotAt(b, 0, false, 2)
			b.spot.arrived = true
			b.world.Self.move = stopAt(Point{X: 2000})
			b.world.Apply(NPCInfo{ObjectID: mob, NPCID: 20001, Attackable: true, Pos: Point{X: 2300}}, at(time.Second))
			if !tt.quiet {
				feed(t, b, time.Second, PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: tt.id, Pos: tt.pos}}})
			}
			for _, s := range tt.steps {
				a, ok := b.ruleFarm(at(s.d))
				if farmed := ok && a.object == mob; farmed != s.farm {
					t.Fatalf("at %v: farmed %v (action %d on %#x), want %v", s.d, farmed, a.kind, a.object, s.farm)
				}
				if !s.farm && !b.spot.idleSince.IsZero() {
					t.Fatalf("at %v: idle since %v while gathering", s.d, b.spot.idleSince)
				}
			}
		})
	}
}

// TestPartyGatherPolicyDefaults: a zero LeadPolicy gathers as the design
// says.
func TestPartyGatherPolicyDefaults(t *testing.T) {
	p := LeadPolicy{}.withDefaults()
	if p.GatherRange != 1200 || p.GatherWait != time.Minute || p.FarAway != 10000 {
		t.Fatalf("defaults %+v: want 1200, 1m, 10000", p)
	}
}
