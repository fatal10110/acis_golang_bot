package botclient

import (
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// adenaID is the adena item template (the price item of a gatekeeper's
// adena-priced destinations).
const adenaID int32 = 57

// npcTypeGatekeeper is the NPC template type of a gatekeeper (npcs/*.xml).
const npcTypeGatekeeper = "Gatekeeper"

// TravelMap is what the datapack says about getting around: where the
// gatekeepers stand and where they take a bot for how much adena, and where
// the NPCs with a fixed spawn stand. It is read-only after NewTravelMap, so
// every bot of a process shares one.
type TravelMap struct {
	gatekeepers []gatekeeper
	// spawns are the fixed spawn points of each NPC template.
	spawns map[int32][]Point
}

// gatekeeper is one spawned gatekeeper and its adena-priced standard
// destinations.
type gatekeeper struct {
	npcID int32
	at    Point
	dests []destination
}

// destination is a standard destination of a gatekeeper's list.
type destination struct {
	// index is its position in the NPC's whole list, other kinds counted:
	// the list page links it as "teleport <index>" (gatekeeper.Window).
	index int
	at    Point
	// price is its base price in adena: the server only ever lowers it
	// (weekend core time) or waives it (free teleports).
	price int32
}

// NewTravelMap builds the travel map of dp: every gatekeeper template with
// a fixed spawn outside an event, with its standard adena-priced
// destinations (the ones its list page shows), and the fixed spawn points
// of every NPC template.
func NewTravelMap(dp *datapack.Datapack) *TravelMap {
	m := &TravelMap{spawns: make(map[int32][]Point)}
	for _, mk := range dp.Spawnlist.Makers {
		if mk.Event != "" {
			continue
		}
		for _, sp := range mk.Spawns {
			if len(sp.Positions) != 1 {
				continue
			}
			p := sp.Positions[0]
			m.spawns[sp.NPCID] = append(m.spawns[sp.NPCID], Point{X: p.X, Y: p.Y, Z: p.Z})
		}
	}
	ids := make([]int32, 0, len(dp.Teleports))
	for id := range dp.Teleports {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if dp.NPCs[id].Type != npcTypeGatekeeper {
			continue
		}
		var dests []destination
		for i, t := range dp.Teleports[id] {
			if t.Kind != datapack.TeleportStandard || t.PriceID != adenaID {
				continue
			}
			dests = append(dests, destination{index: i, at: Point{X: t.X, Y: t.Y, Z: t.Z}, price: int32(t.PriceCount)})
		}
		if len(dests) == 0 {
			continue
		}
		for _, at := range m.spawns[id] {
			m.gatekeepers = append(m.gatekeepers, gatekeeper{npcID: id, at: at, dests: dests})
		}
	}
	return m
}

// Gatekeepers is the number of spawned gatekeepers on the map.
func (m *TravelMap) Gatekeepers() int { return len(m.gatekeepers) }

// TravelPolicy is how a bot with a spot registry and a travel map
// (BrainConfig.Spots and Travel) travels by gatekeeper when no spot that
// fits its level is in walking reach (rule "travel"). Every value is a
// configurable default (D6).
type TravelPolicy struct {
	// MaxPrice is the most adena one trip may cost; 0 never travels.
	MaxPrice int32
	// Gap is the least time from the end of one trip to the start of the
	// next.
	Gap time.Duration
}

// defaultTravelPolicy travels for up to the price of a trip between two
// villages.
func defaultTravelPolicy() TravelPolicy {
	return TravelPolicy{MaxPrice: 50000, Gap: 5 * time.Minute}
}

// tripDepartTimeout is how long the teleport link waits for the bot's own
// TeleportToLocation. The server refuses a trip in silence but for a
// system message (adena short, a besieged castle).
const tripDepartTimeout = 10 * time.Second

// tripNoLinkShunFor is how long a gatekeeper whose page has neither the
// teleport list link nor the destination's link is left alone. Its page
// does not change while the bot plays on: the scripted gatekeepers 31376
// Kurfa and 31383 Teranu answer a bot outside the Ketra or Varka alliance
// with their "-no" page (server script ketraorcsupport.go and
// varkasilenossupport.go onFirstTalk), so a short shun would walk the bot
// back to them over and over.
const tripNoLinkShunFor = 24 * time.Hour

// tripStep is where a trip stands once the gatekeeper's page is open.
type tripStep uint8

const (
	// tripTalk walks to the gatekeeper and talks to it until a page opens.
	tripTalk tripStep = iota
	// tripList sends the page's teleport list link.
	tripList
	// tripGo sends the destination's link of the list.
	tripGo
	// tripGone waits for the teleport the destination's link asked.
	tripGone
)

// travelTrip is the bot's trip by gatekeeper to a spot that fits its level
// (rule "travel"). It is owned by the brain goroutine.
type travelTrip struct {
	v    npcVisit
	step tripStep
	// gk is the gatekeeper's index in the travel map and dest the
	// destination the trip heads for.
	gk   int
	dest destination
	// endedAt is when the last trip ended, zero before one.
	endedAt time.Time
	// shunned are the gatekeepers that took the bot nowhere, by index in
	// the travel map, until a time.
	shunned map[int]time.Time
}

// end ends the trip at now; a positive shunFor leaves its gatekeeper alone
// for that long.
func (tr *travelTrip) end(now time.Time, shunFor time.Duration) {
	if shunFor > 0 {
		if tr.shunned == nil {
			tr.shunned = make(map[int]time.Time)
		}
		tr.shunned[tr.gk] = now.Add(shunFor)
	}
	tr.v, tr.step, tr.dest, tr.endedAt = npcVisit{}, tripTalk, destination{}, now
}

// sent records a packet of the trip that went out at now.
func (tr *travelTrip) sent(a Action, now time.Time) {
	if !tr.v.active() {
		return
	}
	tr.v.sent(a, now)
	if a.kind == actBypass && a.object == tr.v.obj && tr.step == tripGo {
		tr.step, tr.v.waitUntil = tripGone, now.Add(tripDepartTimeout)
	}
}

// teleportListBypass is the gatekeeper page's link to its teleport list
// (folk_bypass.go BypassTeleportList).
func teleportListBypass(obj int32) string {
	return "npc_" + strconv.Itoa(int(obj)) + "_teleport_request"
}

// teleportBypass is the list page's link to destination index
// (gatekeeper.Window: npc_<object id>_teleport <index>).
func teleportBypass(obj int32, index int) string {
	return "npc_" + strconv.Itoa(int(obj)) + "_teleport " + strconv.Itoa(index)
}

// ruleTravel is a rule 9 errand ahead of the spot rule: on a trip the spot
// rule planned (planTrip), the bot walks to the gatekeeper, talks to it,
// opens its teleport list and sends the destination's link; its own
// teleport ends the trip (the brain sends Appearing) and the spot rule
// picks again where it lands. A gatekeeper that takes it nowhere is
// shunned. Bypasses pass the bypass gate (gateBypass); legs and clicks the
// frame pacer, a trip spending at most visitClicks clicks.
func (b *Brain) ruleTravel(now time.Time) (Action, bool) {
	tr := &b.trip
	v := &tr.v
	if !v.active() || b.world.Self.InCombat {
		return Action{}, false
	}
	if tr.step == tripGone {
		if now.Before(v.waitUntil) {
			return Action{}, true
		}
		// No teleport: the server refused the trip.
		tr.end(now, visitShunFor)
		return Action{}, false
	}
	if tr.step != tripTalk {
		if !v.waitUntil.IsZero() && now.Before(v.waitUntil) {
			return Action{}, true
		}
		cmd := teleportListBypass(v.obj)
		if tr.step == tripGo {
			cmd = teleportBypass(v.obj, tr.dest.index)
		}
		if v.waitUntil.IsZero() && b.world.Dialog.Has(cmd) {
			return bypassTo(v.obj, cmd), true
		}
		// Unanswered, or another page replaced the gatekeeper's: the server
		// would refuse the link, so talk again.
		tr.step, v.waitUntil = tripTalk, time.Time{}
	}
	a, ok := b.approach(v, now)
	if !ok {
		tr.end(now, visitShunFor)
		return Action{}, false
	}
	return a, true
}

// tripPage moves the trip on with the gatekeeper's page: its destination's
// link, else its list link; a page with neither ends the trip and shuns the
// gatekeeper for tripNoLinkShunFor.
func (b *Brain) tripPage(ev NpcHtmlMessage, now time.Time) {
	tr := &b.trip
	if !tr.v.active() || tr.step == tripGone || !tr.v.page(ev) {
		return
	}
	switch obj := tr.v.obj; {
	case b.world.Dialog.Has(teleportBypass(obj, tr.dest.index)):
		tr.step = tripGo
	case b.world.Dialog.Has(teleportListBypass(obj)):
		tr.step = tripList
	default:
		tr.end(now, tripNoLinkShunFor)
	}
}

// tripTeleported ends the trip on the bot's own loading-screen teleport:
// its arrival, or a teleport of another cause that left the gatekeeper
// behind. The spot rule picks again at once.
func (b *Brain) tripTeleported(now time.Time) {
	if !b.trip.v.active() {
		return
	}
	b.trip.end(now, 0)
	b.spot.retryAt = time.Time{}
}

// planTrip plans a trip when the spot rule found no spot to claim in
// walking reach: from a gatekeeper in walking reach to a destination with a
// spot that fits the bot's level and has room in walking reach of it, the
// pair with the least walking first, at a price the bot carries and the
// persona allows.
func (b *Brain) planTrip(now time.Time) {
	m, tr, p := b.cfg.Travel, &b.trip, &b.persona.Travel
	if m == nil || tr.v.active() || p.MaxPrice <= 0 || (!tr.endedAt.IsZero() && now.Sub(tr.endedAt) < p.Gap) {
		return
	}
	var budget int32
	if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
		budget = min(p.MaxPrice, it.Count)
	}
	reach := int64(b.persona.Spots.MaxTravel) * int64(b.persona.Spots.MaxTravel)
	from := b.world.Self.Position(now)
	best, bestScore := -1, math.Inf(1)
	var bestDest destination
	for gi := range m.gatekeepers {
		g := &m.gatekeepers[gi]
		dg := dist2(from, g.at)
		if dg > reach || now.Before(tr.shunned[gi]) {
			continue
		}
		for _, d := range g.dests {
			if d.price > budget {
				continue
			}
			ds, ok := b.spotNear(d.at, reach, now)
			if !ok {
				continue
			}
			if score := math.Sqrt(float64(dg)) + math.Sqrt(float64(ds)); score < bestScore {
				best, bestScore, bestDest = gi, score, d
			}
		}
	}
	if best < 0 {
		return
	}
	g := &m.gatekeepers[best]
	tr.v = npcVisit{npcID: g.npcID, at: g.at}
	tr.step, tr.gk, tr.dest = tripTalk, best, bestDest
}

// spotNear is the squared distance from at to the nearest spot within
// reach (squared) of it that fits the bot's level, is not avoided and has
// room; false when there is none.
func (b *Brain) spotNear(at Point, reach int64, now time.Time) (int64, bool) {
	r, v, p := b.cfg.Spots, &b.spot, &b.persona.Spots
	level := int(b.world.Self.Level)
	best, bestDist := -1, int64(0)
	for i := range r.spots {
		sp := &r.spots[i]
		if now.Before(v.avoid[i]) || !p.fits(sp, level) {
			continue
		}
		d := dist2(at, Point{X: sp.Center.X, Y: sp.Center.Y})
		if d > reach || (best >= 0 && d >= bestDist) || !r.hasRoom(i) {
			continue
		}
		best, bestDist = i, d
	}
	return bestDist, best >= 0
}
