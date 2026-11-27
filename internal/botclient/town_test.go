package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// gkTownObj is gatekeeper 30006 once in sight.
const gkTownObj = gkObj

// townTravelMap has gatekeeper 30006 in the bot's town at (2000, 0) and
// 30059 in another town at (95500, 0). 30006's destinations: index 0 the
// far town at (-50000, 0) for 18000 adena (30080 stands there), index 2
// the town of 30059 for 1000, index 3 its own town for nothing and index 4
// a hunting ground with no gatekeeper for 100.
func townTravelMap() *TravelMap {
	return &TravelMap{gatekeepers: []gatekeeper{
		{npcID: 30006, at: Point{X: 2000}, dests: []destination{
			{index: 0, at: Point{X: -50000}, price: 18000},
			{index: 2, at: Point{X: 95000}, price: 1000},
			{index: 3, at: Point{X: 2500}},
			{index: 4, at: Point{X: 60000}, price: 100},
		}},
		{npcID: 30059, at: Point{X: 95500}, dests: []destination{{index: 0, at: Point{X: 2000}, price: 1000}}},
		{npcID: 30080, at: Point{X: -50500}, dests: []destination{{index: 0, at: Point{X: 2000}, price: 18000}}},
	}}
}

// newTownBrain is a town bot at the origin carrying adena, with no idle
// drift, no Newbie Guide or trainer errand, and a hop due one minute
// after its first decision.
func newTownBrain(t *testing.T, adena int32) *Brain {
	t.Helper()
	p := TownPersona()
	p.IdleChance = 0
	p.Newbie.MaxLevel = 0
	p.LearnSkills = map[int32][]int32{}
	p.Town.HopMin, p.Town.HopMax = time.Minute, time.Minute
	b, _ := newTestBrain(t, p)
	b.cfg.Travel = townTravelMap()
	b.world.Inventory.Items = map[int32]Item{1: {ObjectID: 1, TemplateID: adenaID, Count: adena}}
	return walkOn(t, b)
}

func TestTownPersona(t *testing.T) {
	p, d := TownPersona(), DefaultPersona()
	if p.Town.HopMax <= 0 || p.Town.HopMin > p.Town.HopMax || p.Town.MaxPrice <= 0 || p.Town.Reach <= 0 {
		t.Fatalf("Town = %+v, want hops between idle spells", p.Town)
	}
	if d.Town.HopMax != 0 {
		t.Fatalf("default Town = %+v, want no hops", d.Town)
	}
	if p.IdleChance <= d.IdleChance || len(p.Emotes) == 0 {
		t.Fatalf("idle chance %v emotes %v, want more drift than the default %v", p.IdleChance, p.Emotes, d.IdleChance)
	}
	// The server plays social actions 2-13 (movement.go
	// broadcastLiveSocialAction) and drops the others.
	for _, e := range p.Emotes {
		if e < 2 || e > 13 {
			t.Fatalf("emote %d outside 2-13", e)
		}
	}
	if p.Newbie.MaxLevel == 0 || p.Shop.Reach == 0 || p.TrainerReach == 0 {
		t.Fatalf("errands: newbie %+v shop reach %d trainer reach %d, want all on", p.Newbie, p.Shop.Reach, p.TrainerReach)
	}
}

// TestBrainTownHop idles a town bot until its hop is due, takes the
// gatekeeper to the only other town it can afford and idles again from
// its arrival.
func TestBrainTownHop(t *testing.T) {
	b := newTownBrain(t, 5000)
	runTrainerSteps(t, b, []trainerStep{
		{name: "first decision starts the idle spell", at: time.Second},
		{name: "idle", at: 60 * time.Second},
		{name: "hop due: leg to the gatekeeper", at: 61 * time.Second, wantRule: "travel", wantKind: actMove},
		{
			name: "gatekeeper in sight: select", at: 62 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 1900}) },
			events:   []Event{NPCInfo{ObjectID: gkTownObj, NPCID: 30006, Pos: Point{X: 2000}}},
			wantRule: "travel", wantKind: actSelect,
		},
		{name: "talk", at: 63 * time.Second, events: []Event{MyTargetSelected{ObjectID: gkTownObj}}, wantRule: "travel", wantKind: actTalk},
		{name: "list link", at: 64 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkTownObj, HTML: gkPage(gkTownObj)}}, wantRule: "travel", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(gkTownObj) + "_teleport_request")},
		{name: "the other town's link", at: 65 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: gkTownObj, HTML: gkList(gkTownObj, 0, 2, 3, 4)}}, wantRule: "travel", wantKind: actBypass, wantPayload: botwire.EncodeRequestBypassToServer("npc_" + itoa(gkTownObj) + "_teleport 2")},
		{name: "wait for the teleport", at: 66 * time.Second, wantRule: "travel"},
	})
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 95000}}}, at(67*time.Second)); err != nil {
		t.Fatal(err)
	}
	b.appearing = false // the Appearing went out
	if rule, a := b.decide(at(68 * time.Second)); rule != "" || a.kind != actHold {
		t.Fatalf("after arrival = %q %d, want idle", rule, a.kind)
	}
	if b.trip.v.active() || !b.trip.hopAt.Equal(at(128*time.Second)) {
		t.Fatalf("after arrival: trip %+v, want the next hop due a spell after arrival", b.trip)
	}
}

// TestPlanHop checks when a due hop plans a trip, and which.
func TestPlanHop(t *testing.T) {
	due := at(2 * time.Minute)
	tests := []struct {
		name  string
		adena int32
		setup func(b *Brain)
		// wantGK is the gatekeeper's index (-1 for no hop); wantDests
		// the destination indexes the hop may pick.
		wantGK    int
		wantDests []int
	}{
		{name: "the town it can afford", adena: 5000, wantGK: 0, wantDests: []int{2}},
		{name: "every town it can afford", adena: 50000, wantGK: 0, wantDests: []int{0, 2}},
		{name: "persona price cap", adena: 50000, setup: func(b *Brain) { b.persona.Town.MaxPrice = 1000 }, wantGK: 0, wantDests: []int{2}},
		{name: "no adena", wantGK: -1},
		{name: "not due", adena: 5000, setup: func(b *Brain) { b.trip.hopAt = due.Add(time.Second) }, wantGK: -1},
		{name: "spell not started", adena: 5000, setup: func(b *Brain) { b.trip.hopAt = time.Time{} }, wantGK: -1},
		{name: "persona never hops", adena: 5000, setup: func(b *Brain) { b.persona.Town.HopMax = 0 }, wantGK: -1},
		{name: "no travel map", adena: 5000, setup: func(b *Brain) { b.cfg.Travel = nil }, wantGK: -1},
		{name: "gatekeeper beyond reach", adena: 5000, setup: func(b *Brain) { b.persona.Town.Reach = 1999 }, wantGK: -1},
		{name: "gatekeeper shunned", adena: 5000, setup: func(b *Brain) { b.trip.shunned = map[int]time.Time{0: due.Add(time.Hour)} }, wantGK: -1},
		{name: "nearest gatekeeper", adena: 50000, setup: func(b *Brain) {
			b.persona.Town.Reach = 100000
			b.world.Self.move = stopAt(Point{X: 95000})
		}, wantGK: 1, wantDests: []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTownBrain(t, tt.adena)
			b.trip.hopAt = due
			if tt.setup != nil {
				tt.setup(b)
			}
			planned := b.planHop(due)
			if !planned {
				if tt.wantGK >= 0 {
					t.Fatalf("no hop, want one from gatekeeper %d", tt.wantGK)
				}
				if b.trip.v.active() {
					t.Fatalf("trip %+v without a hop", b.trip)
				}
				return
			}
			if tt.wantGK < 0 || b.trip.gk != tt.wantGK || !slices.Contains(tt.wantDests, b.trip.dest.index) {
				t.Fatalf("hop from %d to %d, want from %d to one of %v", b.trip.gk, b.trip.dest.index, tt.wantGK, tt.wantDests)
			}
			if !b.trip.hopAt.IsZero() || b.trip.step != tripTalk {
				t.Fatalf("trip %+v, want a talk step and the spell restarting after it", b.trip)
			}
		})
	}
}

// TestPlanHopSpell checks the idle spell: it starts at the first check, and
// a due hop with no gatekeeper to take it waits another.
func TestPlanHopSpell(t *testing.T) {
	b := newTownBrain(t, 0)
	b.persona.Town.HopMin, b.persona.Town.HopMax = time.Minute, 2*time.Minute
	if b.planHop(at(time.Second)) {
		t.Fatal("first check hops, want the spell to start")
	}
	first := b.trip.hopAt
	if first.Before(at(61*time.Second)) || !first.Before(at(121*time.Second)) {
		t.Fatalf("hop due at %v, want 1-2 minutes after the first check", first.Sub(t0))
	}
	if b.planHop(first) {
		t.Fatal("hop with no adena")
	}
	if !b.trip.hopAt.After(first) {
		t.Fatalf("next hop at %v, want a spell after %v", b.trip.hopAt.Sub(t0), first.Sub(t0))
	}
}

// TestIdleCenterTown checks where idle drift goes (#567): a town bot with
// no farming spots drifts around the travel map's town nearest it, from
// the field as from inside the town; a town bot with farming spots, a
// bot of another persona and a town bot without a travel map drift
// around their anchor.
func TestIdleCenterTown(t *testing.T) {
	town := Point{X: 3000, Y: 2000}
	field := Point{X: 7000, Y: 2000}
	travel := &TravelMap{gatekeepers: []gatekeeper{
		{npcID: 30006, at: town, dests: []destination{{index: 0, at: Point{X: 95000}, price: 1000}}},
		{npcID: 30059, at: Point{X: 95500}, dests: []destination{{index: 0, at: town, price: 1000}}},
	}}
	tests := []struct {
		name    string
		persona Persona
		from    Point
		noMap   bool
		spots   bool
		farm    bool
		want    Point
	}{
		{name: "town bot in the field drifts to its town", persona: TownPersona(), from: field, want: town},
		{name: "town bot in town drifts around it", persona: TownPersona(), from: Point{X: 3300, Y: 2100}, want: town},
		{name: "town bot with farming spots in its town spell drifts to its town", persona: TownPersona(), from: field, spots: true, want: town},
		{name: "town bot in its farming spell keeps its anchor", persona: TownPersona(), from: field, spots: true, farm: true, want: field},
		{name: "support bot keeps its anchor", persona: DefaultPersona(), from: field, want: field},
		{name: "town bot without a travel map keeps its anchor", persona: TownPersona(), from: field, noMap: true, want: field},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.persona
			p.IdleChance = 1
			b, _ := newTestBrain(t, p)
			walkOn(t, b)
			if !tc.noMap {
				b.cfg.Travel = travel
			}
			if tc.spots {
				b.cfg.Spots = NewSpotRegistry(nil, 1)
			}
			if tc.farm {
				b.rota = townRota{farm: true, until: at(time.Hour)}
			}
			b.world.Self.move = stopAt(tc.from)
			b.anchor = tc.from
			for _, p := range []Point{tc.from, tc.want} {
				if _, ok := b.walk.plan.Locate(toLoc(p)); !ok {
					t.Fatalf("%+v off the test ground", p)
				}
			}
			var drifts int
			for i := range 60 {
				b.walk = newWalker(b.walk.nav)
				a, ok := b.ruleIdle(at(time.Duration(i) * time.Second))
				if !ok || b.walk.purpose != walkIdle {
					continue
				}
				drifts++
				if d2 := horizontal2(b.walk.dest, tc.want); d2 > int64(p.IdleRadius+1)*int64(p.IdleRadius+1) {
					t.Fatalf("drift %d to %+v, want within %d of %+v", i, b.walk.dest, p.IdleRadius, tc.want)
				}
				if a.kind == actMove && !a.walk {
					t.Fatalf("drift %d: a move outside the walker", i)
				}
			}
			if drifts < 10 {
				t.Errorf("%d drifts in 60 draws, want about half", drifts)
			}
		})
	}
}

// TestIdleCenterAfterHop checks a town bot that hopped drifts around the
// town it landed in, not the one it left (#567).
func TestIdleCenterAfterHop(t *testing.T) {
	b := newTownBrain(t, 5000)
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 95000}}}, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, want := b.idleCenter(Point{X: 95000}), (Point{X: 95500}); got != want {
		t.Fatalf("idle center after the hop = %+v, want the landing town %+v", got, want)
	}
}
