package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
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
			wantRule: "supply", wantKind: actSelect, wantPayload: botwire.EncodeAction(gkObj, Point{X: 2100}),
		},
		{name: "talk", at: time.Minute + 2*time.Second, events: []Event{MyTargetSelected{ObjectID: gkObj}}, wantRule: "supply", wantKind: actTalk},
		{name: "wait for the page", at: time.Minute + 3*time.Second, wantRule: "supply"},
		{
			name: "list link", at: time.Minute + 4*time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkPage(gkObj)}},
			wantRule: "supply", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(gkObj) + "_teleport_request"),
		},
		{name: "wait for the list", at: time.Minute + 5*time.Second, wantRule: "supply"},
		{
			name: "destination link", at: time.Minute + 6*time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkList(gkObj, 2)}},
			wantRule: "supply", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(gkObj) + "_teleport 2"),
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

// gateDialog is a gatekeeper visit's steps from the gatekeeper in sight to
// its destination link, from start on one second apart: the bot, standing
// at stand, selects gatekeeper obj (template npcID at npcAt), talks, opens
// the list and sends destination index.
func gateDialog(start time.Duration, obj, npcID int32, npcAt, stand Point, index int) []trainerStep {
	s := func(i int) time.Duration { return start + time.Duration(i)*time.Second }
	return []trainerStep{
		{
			name: "gatekeeper in sight: select", at: s(0),
			setup:    func(b *Brain) { b.world.Self.move = stopAt(stand) },
			events:   []Event{NPCInfo{ObjectID: obj, NPCID: npcID, Pos: npcAt}},
			wantRule: "supply", wantKind: actSelect, wantPayload: botwire.EncodeAction(obj, stand),
		},
		{name: "talk", at: s(1), events: []Event{MyTargetSelected{ObjectID: obj}}, wantRule: "supply", wantKind: actTalk},
		{name: "wait for the page", at: s(2), wantRule: "supply"},
		{
			name: "list link", at: s(3), events: []Event{NpcHtmlMessage{ObjectID: obj, HTML: gkPage(obj)}},
			wantRule: "supply", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(obj) + "_teleport_request"),
		},
		{name: "wait for the list", at: s(4), wantRule: "supply"},
		{
			name: "destination link", at: s(5), events: []Event{NpcHtmlMessage{ObjectID: obj, HTML: gkList(obj, index)}},
			wantRule: "supply", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(obj) + "_teleport " + itoa(int32(index))),
		},
	}
}

// TestSupplyTripGatekeeperUnreachable checks the supply trip whose town is
// in walking reach (one SCC, within MaxTravel) but the walker answers it
// unreachable: it shuns the town's own gatekeeper, which the walk would not
// reach either, and takes a gatekeeper trip from another, its first leg
// under the supply rule; with no trip to take it ends at once, with no
// arrival, and the bot farms on.
func TestSupplyTripGatekeeperUnreachable(t *testing.T) {
	tests := []struct {
		name     string
		adena    int32
		wantTrip bool
	}{
		{name: "gatekeeper trip", adena: 5000, wantTrip: true},
		{name: "no gatekeeper trip: ends", adena: 999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGateTripBrain(t, tt.adena)
			// Gatekeeper C, in walking reach west of the wall, a town of its
			// own further from the farmer than (2000, 0); its destination
			// lands in eastTown.
			b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers,
				gatekeeper{npcID: 30070, at: Point{X: 7500, Y: 5000}, dests: []destination{{index: 2, at: Point{X: travelDest}, price: 1000}}})
			arrivals := 0
			b.persona.Supply.InTown = func(*World) { arrivals++ }
			now := at(time.Minute)
			// The town (2000, 0) is in walking reach of (4500, 0), and the
			// walker's last walk there ended unreachable (a door, the watchdog).
			from, home := Point{X: 4500}, Point{X: 2000}
			b.world.Self.move = stopAt(from)
			if !b.onFoot(from, home) {
				t.Fatalf("onFoot(%v, %v) = false, want the town in walking reach", from, home)
			}
			b.walk.reset(home, townArrive, walkPlace)
			b.walk.status, b.walk.endedAt = walkUnreachable, now
			rule, a := b.decide(now)
			if tt.wantTrip {
				if rule != "supply" || a.kind != actMove || !b.supplyTrip.gate || !b.supplyTrip.active() || !b.trip.v.active() || b.trip.gk != 3 || b.trip.dest.index != 2 {
					t.Fatalf("decide = rule %q action %d, gatekeeper trip %v, supply trip %v, travel trip %v from %d to %d: want a supply leg to gatekeeper 3, destination 2",
						rule, a.kind, b.supplyTrip.gate, b.supplyTrip.active(), b.trip.v.active(), b.trip.gk, b.trip.dest.index)
				}
				if !now.Before(b.trip.shunned[0]) {
					t.Fatalf("the town's gatekeeper shunned until %v, want past %v", b.trip.shunned[0], now)
				}
				return
			}
			if rule == "supply" || b.supplyTrip.active() || b.supplyTrip.endedAt != now || b.supplyTrip.gate || b.trip.v.active() || arrivals != 0 {
				t.Fatalf("decide = rule %q, supply trip active %v ended %v gate %v, travel trip %v, in town hook calls %d: want the trip ended at %v, no arrival",
					rule, b.supplyTrip.active(), b.supplyTrip.endedAt, b.supplyTrip.gate, b.trip.v.active(), arrivals, now)
			}
		})
	}
}

// TestSupplyTripGatekeeperTwoHops checks a gatekeeper trip of two
// teleports: gatekeeper A's destination lands by gatekeeper B, whose
// destination lands in the trip's town. The landing on the way keeps the
// supply trip going, its next leg to B within the travel gap; the last
// landing is the arrival, once.
func TestSupplyTripGatekeeperTwoHops(t *testing.T) {
	const objB = gkObj + 1
	mid, gkB := Point{X: 15000, Y: 2000}, Point{X: 15500, Y: 2000}
	b := newGateTripBrain(t, 5000)
	b.cfg.Travel.gatekeepers[0].dests = []destination{{index: 1, at: mid, price: 1000}}
	b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers,
		gatekeeper{npcID: 30070, at: gkB, dests: []destination{{index: 0, at: Point{X: travelDest}, price: 1000}}})
	arrivals := 0
	b.persona.Supply.InTown = func(*World) { arrivals++ }
	steps := append([]trainerStep{{name: "leg to gatekeeper A", at: time.Minute, wantRule: "supply", wantKind: actMove}},
		gateDialog(time.Minute+time.Second, gkObj, townGK, Point{X: 2000}, Point{X: 2100}, 1)...)
	runTrainerSteps(t, b, steps)
	if b.trip.onward != 1 || b.trip.gk != 0 {
		t.Fatalf("travel trip from gatekeeper %d, %d onward: want from gatekeeper 0, one onward", b.trip.gk, b.trip.onward)
	}

	land := at(time.Minute + 7*time.Second)
	b.world.Objects = map[int32]*Object{}
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: mid}}, land); err != nil {
		t.Fatal(err)
	}
	if !b.supplyTrip.active() || !b.supplyTrip.gate || arrivals != 0 {
		t.Fatalf("after the landing on the way: supply trip active %v gate %v, in town hook calls %d: want it going, no arrival",
			b.supplyTrip.active(), b.supplyTrip.gate, arrivals)
	}
	if next := 2 * time.Second; next >= b.persona.Travel.Gap {
		t.Fatalf("travel gap %v: the next leg at +%v would not show it waived", b.persona.Travel.Gap, next)
	}
	runTrainerSteps(t, b, append([]trainerStep{{
		name: "leg to gatekeeper B", at: time.Minute + 9*time.Second,
		setup: func(b *Brain) {
			if sent, err := b.send(Action{kind: actAppear, payload: botwire.EncodeAppearing()}, land); err != nil || !sent {
				t.Fatalf("Appearing: send = %v, %v", sent, err)
			}
		},
		wantRule: "supply", wantKind: actMove, wantTo: gkB, wantWithin: talkReach,
	}}, gateDialog(time.Minute+10*time.Second, objB, 30070, gkB, Point{X: 15400, Y: 2000}, 0)...))
	if b.trip.gk != 3 || b.trip.onward != 0 {
		t.Fatalf("travel trip from gatekeeper %d, %d onward: want from gatekeeper 3, none onward", b.trip.gk, b.trip.onward)
	}

	last := at(time.Minute + 16*time.Second)
	b.world.Objects = map[int32]*Object{}
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: travelDest}}}, last); err != nil {
		t.Fatal(err)
	}
	if b.supplyTrip.active() || b.supplyTrip.endedAt != last || b.trip.v.active() || arrivals != 1 {
		t.Fatalf("after the last landing: supply trip active %v ended %v, travel trip %v, in town hook calls %d: want both over at %v, one arrival",
			b.supplyTrip.active(), b.supplyTrip.endedAt, b.trip.v.active(), arrivals, last)
	}
}

// TestSupplyTripGatekeeperRefused checks a gatekeeper trip the server
// refuses: no teleport by tripDepartTimeout after the destination link ends
// the supply trip at that tick, with no arrival, and shuns the gatekeeper.
func TestSupplyTripGatekeeperRefused(t *testing.T) {
	b := newGateTripBrain(t, 5000)
	arrivals := 0
	b.persona.Supply.InTown = func(*World) { arrivals++ }
	link := time.Minute + 6*time.Second
	steps := append([]trainerStep{{name: "leg to the gatekeeper", at: time.Minute, wantRule: "supply", wantKind: actMove}},
		gateDialog(time.Minute+time.Second, gkObj, townGK, Point{X: 2000}, Point{X: 2100}, 2)...)
	runTrainerSteps(t, b, append(steps, trainerStep{name: "wait for the teleport", at: link + tripDepartTimeout - time.Millisecond, wantRule: "supply"}))
	now := at(link + tripDepartTimeout)
	if rule, _ := b.decide(now); rule == "supply" {
		t.Fatalf("decide at the timeout = rule %q, want the supply trip over", rule)
	}
	if b.supplyTrip.active() || b.supplyTrip.endedAt != now || b.trip.v.active() || arrivals != 0 {
		t.Fatalf("after the timeout: supply trip active %v ended %v, travel trip %v, in town hook calls %d: want both over at %v, no arrival",
			b.supplyTrip.active(), b.supplyTrip.endedAt, b.trip.v.active(), arrivals, now)
	}
	if !now.Before(b.trip.shunned[0]) {
		t.Fatalf("gatekeeper 0 shunned until %v, want past %v", b.trip.shunned[0], now)
	}
}
