package botclient

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// gkObj is the gatekeeper Roxxy (NPC 30006) once in sight.
const gkObj = int32(0x20000020)

// gkPage is a gatekeeper's first page with %objectId% filled in
// (data/html/gatekeeper/30006.htm, reduced).
func gkPage(obj int32) string {
	return `<html><body>Gatekeeper Roxxy:<br><a action="bypass -h npc_` + itoa(obj) + `_teleport_request">Teleport</a><br>` +
		`<a action="bypass -h npc_` + itoa(obj) + `_Chat 1">Ask about the area</a></body></html>`
}

// gkList is the teleport list page as gatekeeper.Window writes it: one link
// per standard destination, indexed in the whole list.
func gkList(obj int32, indexes ...int) string {
	s := "<html><body>&$556;<br><br>"
	for _, i := range indexes {
		s += `<a action="bypass -h npc_` + itoa(obj) + `_teleport ` + itoa(int32(i)) + `" msg="811;Somewhere">Somewhere - 1000 &#57;</a><br1>`
	}
	return s + "</body></html>"
}

// testTravelMap has gatekeeper 30006 standing at (2000, 0), its destination
// index 2 at (95000, 0) for 1000 adena.
// travelDest is the X of the test gatekeeper's second destination, east of
// the split navigation's wall, and travelFar the X of the spot that fits
// the travel brain there: in straight line well within MaxTravel of the
// origin, but in the other SCC, more than spotSnap past the wall so its
// centre never snaps to the west side.
const (
	travelDest = 9000
	travelFar  = 8700
)

func testTravelMap() *TravelMap {
	return &TravelMap{gatekeepers: []gatekeeper{{
		npcID: 30006, at: Point{X: 2000},
		dests: []destination{{index: 0, at: Point{X: 6000, Y: 7000}, price: 18000}, {index: 2, at: Point{X: travelDest}, price: 1000}},
	}}}
}

// newTravelBrain is a level 20 bot at the origin carrying adena, holding no
// spot, with spot "low" in walking reach that does not fit it and spot
// "far" across the split navigation's wall that does, near the gatekeeper's
// second destination.
func newTravelBrain(t *testing.T, adena int32) *Brain {
	t.Helper()
	b := newSpotBrain(t, 20, testSpot("low", 1000, 0, 1, 5, 9), testSpot("far", travelFar, 0, 18, 21, 9))
	b.persona.Newbie.MaxLevel = 0
	b.cfg.Travel = testTravelMap()
	b.world.Inventory.Items = map[int32]Item{1: {ObjectID: 1, TemplateID: adenaID, Count: adena}}
	return b
}

func TestNewTravelMap(t *testing.T) {
	pos := func(x int32) []datapack.Position { return []datapack.Position{{X: x, Y: 1, Z: 2}} }
	std := func(x, priceID int32) datapack.Teleport {
		return datapack.Teleport{Kind: datapack.TeleportStandard, PriceID: priceID, PriceCount: 100, X: x}
	}
	dp := &datapack.Datapack{
		NPCs: map[int32]datapack.NPC{
			30006: {ID: 30006, Type: npcTypeGatekeeper},
			30059: {ID: 30059, Type: npcTypeGatekeeper},
			31000: {ID: 31000, Type: "Folk"},
		},
		Spawnlist: &datapack.Spawnlist{Makers: []*datapack.Maker{
			{Name: "town", Spawns: []datapack.Spawn{
				{NPCID: 30006, Positions: pos(10)},
				{NPCID: 30598, Positions: pos(20)},
				{NPCID: 20002}, // placed at random: no fixed spawn
				{NPCID: 31000, Positions: pos(30)},
			}},
			{Name: "ssq", Event: "ssq_event", Spawns: []datapack.Spawn{{NPCID: 30006, Positions: pos(40)}}},
			{Name: "town2", Spawns: []datapack.Spawn{{NPCID: 30059, Positions: pos(50)}}},
		}},
		Teleports: map[int32][]datapack.Teleport{
			30006: {std(100, 57), {Kind: "CHF_LEVEL_1", PriceID: 57, X: 200}, std(300, 5575), std(400, 57)},
			30059: {{Kind: "NOBLE_HUNTING_ZONE_PASS", PriceID: 6651, X: 1}}, // nothing standard
			31000: {std(500, 57)},                                           // not a gatekeeper
		},
		Prices: map[int32]int32{1867: 10},
	}
	m := NewTravelMap(dp)
	if want := map[int32]int32{1867: 10}; !reflect.DeepEqual(m.prices, want) {
		t.Fatalf("prices = %v, want %v", m.prices, want)
	}
	want := []gatekeeper{{npcID: 30006, at: Point{X: 10, Y: 1, Z: 2}, dests: []destination{
		{index: 0, at: Point{X: 100}, price: 100},
		{index: 3, at: Point{X: 400}, price: 100},
	}}}
	if !reflect.DeepEqual(m.gatekeepers, want) {
		t.Fatalf("gatekeepers = %+v, want %+v", m.gatekeepers, want)
	}
	wantSpawns := map[int32][]Point{
		30006: {{X: 10, Y: 1, Z: 2}}, 30598: {{X: 20, Y: 1, Z: 2}}, 31000: {{X: 30, Y: 1, Z: 2}}, 30059: {{X: 50, Y: 1, Z: 2}},
	}
	if !reflect.DeepEqual(m.spawns, wantSpawns) {
		t.Fatalf("spawns = %+v, want %+v", m.spawns, wantSpawns)
	}
}

// TestTravelMapShipped builds the travel map of a real datapack when
// BOT_DATAPACK_XML names its data/xml directory, and skips otherwise.
func TestTravelMapShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewTravelMap(dp)
	t.Logf("gatekeepers=%d gear=%d sold gear=%d", m.Gatekeepers(), len(m.gear), len(m.shopGear))
	for _, g := range m.gatekeepers {
		if g.npcID != 30006 {
			continue
		}
		// Talking Island's Roxxy: Gludin is the first of her list.
		if g.at != (Point{X: -84108, Y: 244604, Z: -3729}) || g.dests[0] != (destination{index: 0, at: Point{X: -80749, Y: 149834, Z: -3043}, price: 18000}) {
			t.Fatalf("Roxxy = %+v", g)
		}
		if len(m.spawns[30598]) != 1 {
			t.Fatalf("Newbie Guide 30598 spawns = %v, want one", m.spawns[30598])
		}
		return
	}
	t.Fatal("no Roxxy (30006) on the map")
}

// TestBrainTravel walks a trip: the spot rule finds no fitting spot in
// reach and plans a trip; the bot walks to the gatekeeper, talks, opens the
// list, sends the destination's link, and on its own teleport sends
// Appearing and claims the spot near the destination.
func TestBrainTravel(t *testing.T) {
	b := newTravelBrain(t, 5000)
	runTrainerSteps(t, b, []trainerStep{
		{name: "no spot in reach plans a trip", at: time.Second},
		{name: "leg to the gatekeeper", at: 2 * time.Second, wantRule: "travel", wantKind: actMove},
		{
			name: "gatekeeper in sight: select", at: 3 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 1900}) },
			events:   []Event{NPCInfo{ObjectID: gkObj, NPCID: 30006, Pos: Point{X: 2000}}},
			wantRule: "travel", wantKind: actSelect, wantPayload: encodeAction(gkObj, Point{X: 1900}),
		},
		{name: "talk", at: 4 * time.Second, events: []Event{MyTargetSelected{ObjectID: gkObj}}, wantRule: "travel", wantKind: actTalk},
		{name: "wait for the page", at: 5 * time.Second, wantRule: "travel"},
		{name: "list link", at: 6 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkPage(gkObj)}}, wantRule: "travel", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(gkObj) + "_teleport_request")},
		{name: "wait for the list", at: 7 * time.Second, wantRule: "travel"},
		{name: "destination link", at: 8 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkList(gkObj, 0, 2)}}, wantRule: "travel", wantKind: actBypass, wantPayload: encodeRequestBypassToServer("npc_" + itoa(gkObj) + "_teleport 2")},
		{name: "wait for the teleport", at: 10 * time.Second, wantRule: "travel"},
	})
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: travelDest}}}, at(11*time.Second)); err != nil {
		t.Fatal(err)
	}
	if b.trip.v.active() || !b.appearing {
		t.Fatalf("after the teleport: trip %+v appearing %v, want the trip over and Appearing due", b.trip, b.appearing)
	}
	b.appearing = false // the Appearing went out
	rule, a := b.decide(at(12 * time.Second))
	if rule != "spot" || a.kind != actMove || !b.spot.held || b.spot.idx != 1 {
		t.Fatalf("after arrival = %q %d spot %+v, want a walk to spot far", rule, a.kind, b.spot)
	}
}

// TestTravelPlan checks when the spot rule's failed pick plans a trip, and
// which.
func TestTravelPlan(t *testing.T) {
	tests := []struct {
		name  string
		adena int32
		setup func(b *Brain)
		// wantGK is the planned gatekeeper's npc id and wantIndex its
		// destination, 0 and -1 for no trip.
		wantGK    int32
		wantIndex int
	}{
		{name: "plans the affordable destination near a fitting spot", adena: 5000, wantGK: 30006, wantIndex: 2},
		{name: "not enough adena", adena: 999, wantIndex: -1},
		{name: "no adena", adena: 0, wantIndex: -1},
		{name: "price above the persona's", adena: 5000, setup: func(b *Brain) { b.persona.Travel.MaxPrice = 999 }, wantIndex: -1},
		{name: "travel off", adena: 5000, setup: func(b *Brain) { b.persona.Travel.MaxPrice = 0 }, wantIndex: -1},
		{name: "no travel map", adena: 5000, setup: func(b *Brain) { b.cfg.Travel = nil }, wantIndex: -1},
		{name: "gatekeeper out of walking reach", adena: 5000, setup: func(b *Brain) { b.cfg.Travel.gatekeepers[0].at = Point{X: 30000} }, wantIndex: -1},
		{name: "gatekeeper across the wall", adena: 5000, setup: func(b *Brain) { b.cfg.Travel.gatekeepers[0].at = Point{X: travelDest + 500} }, wantIndex: -1},
		{name: "destination off the navigation", adena: 5000, setup: func(b *Brain) { b.cfg.Travel.gatekeepers[0].dests[1].at = Point{X: 95000} }, wantIndex: -1},
		{name: "gatekeeper shunned", adena: 5000, setup: func(b *Brain) { b.trip.shunned = map[int]time.Time{0: at(time.Hour)} }, wantIndex: -1},
		{name: "within the gap after the last trip", adena: 5000, setup: func(b *Brain) { b.trip.endedAt = at(-time.Minute) }, wantIndex: -1},
		{name: "after the gap", adena: 5000, setup: func(b *Brain) { b.trip.endedAt = at(-5 * time.Minute) }, wantGK: 30006, wantIndex: 2},
		{name: "fitting spot full", adena: 5000, setup: func(b *Brain) { b.cfg.Spots.held[1] = b.cfg.Spots.caps[1] }, wantIndex: -1},
		{name: "fitting spot avoided", adena: 5000, setup: func(b *Brain) { b.spot.avoid = map[int]time.Time{1: at(time.Hour)} }, wantIndex: -1},
		{name: "no fitting spot near any destination", adena: 5000, setup: func(b *Brain) { b.world.Self.Level = 40 }, wantIndex: -1},
		{name: "fitting spot in reach: claimed, no trip", adena: 5000, setup: func(b *Brain) { b.world.Self.Level = 4 }, wantIndex: -1},
		{
			// A second gatekeeper nearer the bot, its destination right at
			// the spot: walks of 500 and 0 at 10 adena for 100 units
			// (SecondPrice 10, run speed 100) and 1100 adena, 1150, beat
			// walks of 2000 and 500 and 1000 adena, 1250.
			name: "cheapest path first", adena: 5000,
			setup: func(b *Brain) {
				b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{
					npcID: 30059, at: Point{X: 500}, dests: []destination{{index: 5, at: Point{X: travelFar}, price: 1100}},
				})
			},
			wantGK: 30059, wantIndex: 5,
		},
		{
			name: "a dear teleport loses to a longer walk", adena: 50000,
			setup: func(b *Brain) {
				b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{
					npcID: 30059, at: Point{X: 500}, dests: []destination{{index: 5, at: Point{X: travelFar}, price: 20000}},
				})
			},
			wantGK: 30006, wantIndex: 2,
		},
		{
			// Two teleports: the gatekeeper's first destination has a
			// gatekeeper in walking reach whose destination is the spot's;
			// the trip is the first teleport.
			name: "two teleports: the first", adena: 50000,
			setup: func(b *Brain) {
				// Walks of at most 5000: the second gatekeeper is out of
				// the bot's reach, in its first destination's.
				b.persona.Spots.MaxTravel = 5000
				b.cfg.Travel.gatekeepers[0].dests = b.cfg.Travel.gatekeepers[0].dests[:1]
				b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{
					npcID: 30059, at: Point{X: 6500, Y: 7000}, dests: []destination{{index: 5, at: Point{X: travelDest}, price: 1000}},
				})
			},
			wantGK: 30006, wantIndex: 0,
		},
		{
			// A nearer gatekeeper reaches the first destination cheaper in
			// cost (walk 0, 18050 adena) but dearer in adena: the only
			// path within 19000 adena is the costlier one by 30006.
			name: "two teleports: the path within the budget", adena: 19000,
			setup: func(b *Brain) {
				b.persona.Spots.MaxTravel = 5000
				b.cfg.Travel.gatekeepers[0].dests = b.cfg.Travel.gatekeepers[0].dests[:1]
				b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{
					npcID: 30059, at: Point{X: 6500, Y: 7000}, dests: []destination{{index: 5, at: Point{X: travelDest}, price: 1000}},
				}, gatekeeper{
					npcID: 30060, dests: []destination{{index: 1, at: Point{X: 6000, Y: 7000}, price: 18050}},
				})
			},
			wantGK: 30006, wantIndex: 0,
		},
		{
			// Landing with one teleport onward: the plan is due within the
			// gap, but a path of two is not taken.
			name: "onward: no more teleports than the rest", adena: 50000,
			setup: func(b *Brain) {
				b.persona.Spots.MaxTravel = 5000
				b.trip.endedAt, b.trip.onward = at(-time.Minute), 1
				b.cfg.Travel.gatekeepers[0].dests = b.cfg.Travel.gatekeepers[0].dests[:1]
				b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{
					npcID: 30059, at: Point{X: 6500, Y: 7000}, dests: []destination{{index: 5, at: Point{X: travelDest}, price: 1000}},
				})
			},
			wantIndex: -1,
		},
		{
			name: "onward: due within the gap", adena: 5000,
			setup:  func(b *Brain) { b.trip.endedAt, b.trip.onward = at(-time.Minute), 1 },
			wantGK: 30006, wantIndex: 2,
		},
		{
			name: "two teleports past the budget", adena: 18500,
			setup: func(b *Brain) {
				// Walks of at most 5000: the second gatekeeper is out of
				// the bot's reach, in its first destination's.
				b.persona.Spots.MaxTravel = 5000
				b.cfg.Travel.gatekeepers[0].dests = b.cfg.Travel.gatekeepers[0].dests[:1]
				b.cfg.Travel.gatekeepers = append(b.cfg.Travel.gatekeepers, gatekeeper{
					npcID: 30059, at: Point{X: 6500, Y: 7000}, dests: []destination{{index: 5, at: Point{X: travelDest}, price: 1000}},
				})
			},
			wantIndex: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTravelBrain(t, tt.adena)
			if tt.adena == 0 {
				b.world.Inventory.Items = nil
			}
			if tt.setup != nil {
				tt.setup(b)
			}
			b.decide(at(time.Second))
			if tt.wantIndex < 0 {
				if b.trip.v.active() {
					t.Fatalf("trip planned: %+v", b.trip)
				}
				return
			}
			if b.trip.v.npcID != tt.wantGK || b.trip.dest.index != tt.wantIndex {
				t.Fatalf("trip = %+v, want gatekeeper %d destination %d", b.trip, tt.wantGK, tt.wantIndex)
			}
		})
	}
}

// TestTravelGivesUp checks the trips that end without a teleport shun
// their gatekeeper (one whose page lacks the links for
// tripNoLinkShunFor), and that another teleport ends a trip unshunned.
func TestTravelGivesUp(t *testing.T) {
	inSight := func(b *Brain) {
		b.world.Self.move = stopAt(Point{X: 1900})
		b.world.Apply(NPCInfo{ObjectID: gkObj, NPCID: 30006, Pos: Point{X: 2000}}, t0)
		b.world.Target = gkObj
	}
	tests := []struct {
		name    string
		steps   []trainerStep
		shunFor time.Duration
	}{
		{
			name: "teleport link unanswered",
			steps: []trainerStep{
				{name: "plan", at: time.Second, setup: inSight},
				{name: "talk", at: 2 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "list", at: 3 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkList(gkObj, 2)}}, wantRule: "travel", wantKind: actBypass},
				{name: "wait", at: 3*time.Second + tripDepartTimeout - time.Millisecond, wantRule: "travel"},
				{name: "refused", at: 3*time.Second + tripDepartTimeout},
			},
			shunFor: visitShunFor,
		},
		{
			name: "page without the links",
			steps: []trainerStep{
				{name: "plan", at: time.Second, setup: inSight},
				{name: "talk", at: 2 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "no link", at: 3 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkObj, HTML: gkList(gkObj, 0)}}},
			},
			shunFor: tripNoLinkShunFor,
		},
		{
			name: "gatekeeper not at its spawn",
			steps: []trainerStep{
				{name: "plan", at: time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 1800}) }},
				{name: "nobody there", at: 2 * time.Second},
			},
			shunFor: visitShunFor,
		},
		{
			name: "talks unanswered",
			steps: []trainerStep{
				{name: "plan", at: time.Second, setup: inSight},
				{name: "talk 1", at: 2 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "talk 2", at: 23 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "talk 3", at: 44 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "talk 4", at: 65 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "talk 5", at: 86 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "talk 6", at: 107 * time.Second, wantRule: "travel", wantKind: actTalk},
				// The trip given up, no spot of its level is left: it
				// falls back to the one under it (#446).
				{name: "clicks spent", at: 128 * time.Second, wantRule: "spot", wantKind: actMove},
			},
			shunFor: visitShunFor,
		},
		{
			name: "teleported elsewhere",
			steps: []trainerStep{
				{name: "plan", at: time.Second, setup: inSight},
				{name: "talk", at: 2 * time.Second, wantRule: "travel", wantKind: actTalk},
				{name: "recalled", at: 3 * time.Second, events: []Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: -40000}}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTravelBrain(t, 5000)
			b.spot.retryAt = time.Time{}
			runTrainerSteps(t, b, tt.steps)
			if b.trip.v.active() || b.trip.endedAt.IsZero() {
				t.Fatalf("trip %+v, want it ended", b.trip)
			}
			var shunFor time.Duration
			if until, ok := b.trip.shunned[0]; ok {
				shunFor = until.Sub(b.trip.endedAt)
			}
			if shunFor != tt.shunFor {
				t.Fatalf("gatekeeper shunned for %v, want %v", shunFor, tt.shunFor)
			}
		})
	}
}
