package botclient

import (
	"testing"
	"time"
)

// The gatekeeper supply trip tests' towns: eastTown across the split
// navigation's wall, the nearest town to gateFrom, where the farmer stands,
// and farTown further east. The gatekeeper the farmer walks to stands at
// (2000, 0) on its side, its destination index 2 lands in eastTown.
var (
	gateFrom = Point{X: 6000}
	eastTown = Point{X: 9500}
	farTown  = Point{X: 16000, Y: 8000}
)

// newGateTripBrain is newTripBrain's farmer with no Scroll of Escape and
// its HP potions gone, standing at gateFrom with adena, the town nearest it
// across the wall: only a gatekeeper takes it there.
func newGateTripBrain(t *testing.T, adena int32) *Brain {
	t.Helper()
	b := newTripBrain(t, Item{ObjectID: 1, TemplateID: adenaID, Count: adena})
	b.supplyTrip.potions = true
	b.world.Self.move = stopAt(gateFrom)
	b.cfg.Travel.gatekeepers = []gatekeeper{
		{npcID: townGK, at: Point{X: 2000}, dests: []destination{{index: 2, at: Point{X: travelDest}, price: 1000}}},
		{npcID: 30059, at: eastTown},
		{npcID: 30060, at: farTown},
	}
	return b
}

// TestSupplyTripGatekeeper checks the trip home of a farmer with no Scroll
// of Escape whose nearest town it cannot walk to: it walks to a gatekeeper
// in reach, talks, opens the list and sends the town's destination link;
// the teleport ends the trip as an arrival (townArrival, once) and asks
// Appearing.
func TestSupplyTripGatekeeper(t *testing.T) {
	b := newGateTripBrain(t, 5000)
	arrivals := 0
	b.persona.Supply.InTown = func(*World) { arrivals++ }
	runTrainerSteps(t, b, []trainerStep{
		{name: "town across the wall: leg to the gatekeeper", at: time.Minute, wantRule: "supply", wantKind: actMove},
		{
			name: "gatekeeper in sight: select", at: time.Minute + time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 2100}) },
			events:   []Event{NPCInfo{ObjectID: gkObj, NPCID: townGK, Pos: Point{X: 2000}}},
			wantRule: "supply", wantKind: actSelect, wantPayload: encodeAction(gkObj, Point{X: 2100}),
		},
		{name: "talk", at: time.Minute + 2*time.Second, events: []Event{MyTargetSelected{ObjectID: gkObj}}, wantRule: "supply", wantKind: actTalk},
		{name: "wait for the page", at: time.Minute + 3*time.Second, wantRule: "supply"},
		{
			name: "list link", at: time.Minute + 4*time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkPage(gkObj)}},
			wantRule: "supply", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(gkObj) + "_teleport_request"),
		},
		{name: "wait for the list", at: time.Minute + 5*time.Second, wantRule: "supply"},
		{
			name: "destination link", at: time.Minute + 6*time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkList(gkObj, 2)}},
			wantRule: "supply", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(gkObj) + "_teleport 2"),
		},
		{name: "wait for the teleport", at: time.Minute + 8*time.Second, wantRule: "supply"},
	})
	if b.spot.held || !b.supplyTrip.gate {
		t.Fatalf("spot held %v, gatekeeper trip %v: want the spot released, a gatekeeper trip", b.spot.held, b.supplyTrip.gate)
	}
	land := at(time.Minute + 9*time.Second)
	b.world.Objects = map[int32]*Object{}
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: travelDest}}}, land); err != nil {
		t.Fatal(err)
	}
	if b.supplyTrip.active() || b.supplyTrip.endedAt != land || b.trip.v.active() {
		t.Fatalf("after the teleport: supply trip active %v ended %v, travel trip %v: want both over at %v",
			b.supplyTrip.active(), b.supplyTrip.endedAt, b.trip.v.active(), land)
	}
	if !b.prep.armed || arrivals != 1 || !b.appearing {
		t.Fatalf("after the teleport: preparation armed %v, in town hook calls %d, Appearing due %v: want armed, one, due",
			b.prep.armed, arrivals, b.appearing)
	}
}

// TestSupplyTripGatekeeperPlan checks which gatekeeper trip the supply trip
// takes when its town is out of walking reach: one to the trip's town
// first, else the cheapest to any town, within the persona's price, the
// adena carried and the travel gap; with none it ends at once, the bot farms
// on and tries again after supplyTripGap. A town in walking reach is
// walked to.
func TestSupplyTripGatekeeperPlan(t *testing.T) {
	tests := []struct {
		name  string
		adena int32
		setup func(b *Brain)
		// wantIndex is the planned destination's index, -1 for no trip.
		wantIndex int
		wantWalk  bool
	}{
		{name: "the town's destination", adena: 5000, wantIndex: 2},
		{
			name: "the trip's town before a cheaper other town", adena: 5000, wantIndex: 2,
			setup: func(b *Brain) {
				g := &b.cfg.Travel.gatekeepers[0]
				g.dests = append(g.dests, destination{index: 0, at: Point{X: 15000, Y: 8000}, price: 500})
			},
		},
		{
			name: "no way to the trip's town: another town", adena: 5000, wantIndex: 0,
			setup: func(b *Brain) {
				b.cfg.Travel.gatekeepers[0].dests = []destination{{index: 0, at: Point{X: 15000, Y: 8000}, price: 1000}}
			},
		},
		{
			name: "no destination in a town", adena: 5000, wantIndex: -1,
			setup: func(b *Brain) {
				b.cfg.Travel.gatekeepers[0].dests = []destination{{index: 0, at: Point{X: 13000, Y: 2000}, price: 1000}}
			},
		},
		{name: "not enough adena", adena: 999, wantIndex: -1},
		{name: "price above the persona's", adena: 5000, wantIndex: -1, setup: func(b *Brain) { b.persona.Travel.MaxPrice = 999 }},
		{name: "within the travel gap", adena: 5000, wantIndex: -1, setup: func(b *Brain) { b.trip.endedAt = at(30 * time.Second) }},
		{name: "gatekeeper shunned", adena: 5000, wantIndex: -1, setup: func(b *Brain) { b.trip.shunned = map[int]time.Time{0: at(time.Hour)} }},
		{
			name: "town in walking reach: walks", adena: 5000, wantIndex: -1, wantWalk: true,
			setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4500}) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGateTripBrain(t, tt.adena)
			if tt.setup != nil {
				tt.setup(b)
			}
			now := at(time.Minute)
			rule, a := b.decide(now)
			switch {
			case tt.wantIndex >= 0:
				if rule != "supply" || a.kind != actMove || !b.supplyTrip.gate || !b.trip.v.active() || b.trip.dest.index != tt.wantIndex {
					t.Fatalf("decide = rule %q action %d, gatekeeper trip %v, travel trip %v to %d: want a supply leg to destination %d",
						rule, a.kind, b.supplyTrip.gate, b.trip.v.active(), b.trip.dest.index, tt.wantIndex)
				}
			case tt.wantWalk:
				if rule != "supply" || a.kind != actMove || b.supplyTrip.gate || b.trip.v.active() || b.supplyTrip.town != (Point{X: 2000}) {
					t.Fatalf("decide = rule %q action %d, gatekeeper trip %v, walking to %v: want a supply walk to (2000, 0)",
						rule, a.kind, b.supplyTrip.gate, b.supplyTrip.town)
				}
			default:
				if rule == "supply" || b.supplyTrip.active() || b.supplyTrip.endedAt != now || b.trip.v.active() {
					t.Fatalf("decide = rule %q, supply trip active %v ended %v, travel trip %v: want the trip ended at %v, no travel",
						rule, b.supplyTrip.active(), b.supplyTrip.endedAt, b.trip.v.active(), now)
				}
			}
		})
	}
}

// TestSupplyTripTeleported checks which of the bot's own teleports end the
// supply trip and which are an arrival in town: the recall's landing and a
// gatekeeper trip's last are; a gatekeeper trip's landing on the way ends
// nothing; a teleport of another cause ends the trip with no arrival.
func TestSupplyTripTeleported(t *testing.T) {
	tests := []struct {
		name        string
		trip        supplyTrip
		gk          bool
		onward      int
		wantArrival bool
		wantActive  bool
	}{
		{name: "no trip", trip: supplyTrip{}, gk: true},
		{name: "recall landing", trip: supplyTrip{since: t0, escapeAt: t0}, wantArrival: true},
		{name: "gatekeeper trip, last landing", trip: supplyTrip{since: t0, walk: true, gate: true}, gk: true, wantArrival: true},
		{name: "gatekeeper trip, landing on the way", trip: supplyTrip{since: t0, walk: true, gate: true}, gk: true, onward: 1, wantActive: true},
		{name: "gatekeeper trip, teleport of another cause", trip: supplyTrip{since: t0, walk: true, gate: true}},
		{name: "walk, a travel trip's teleport", trip: supplyTrip{since: t0, walk: true}, gk: true},
		{name: "walk, teleport of another cause", trip: supplyTrip{since: t0, walk: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.trip
			now := t0.Add(time.Minute)
			if got := v.teleported(now, tt.gk, tt.onward); got != tt.wantArrival {
				t.Fatalf("teleported = %v, want %v", got, tt.wantArrival)
			}
			if v.active() != tt.wantActive {
				t.Fatalf("trip active %v after the teleport, want %v", v.active(), tt.wantActive)
			}
		})
	}
}
