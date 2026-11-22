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
// the NPCs with a fixed spawn stand, and what the merchants sell. It is read-only after NewTravelMap, so
// every bot of a process shares one.
type TravelMap struct {
	gatekeepers []gatekeeper
	// spawns are the fixed spawn points of each NPC template.
	spawns map[int32][]Point
	// shops are the buylists of each merchant NPC template, and cheapest
	// the lowest base price of each item they sell.
	shops    map[int32][]shopList
	cheapest map[int32]int32
	// gear are the weapon and armor templates bots may wear, and shopGear
	// the ones merchants sell, by slot then price (gearCandidates ranks
	// them by score).
	gear     map[int32]gearItem
	shopGear []int32
	// sets are the armor sets of gear pieces, by chest (indexSets), and
	// setPieces every template one of them names.
	sets      map[int32]armorSet
	setPieces map[int32]bool
	// arrows maps each bow template to the arrow template it shoots
	// (indexArrows).
	arrows map[int32]int32
	// prices are the reference prices of the item templates a store can
	// list (datapack.Datapack.Prices): the price a store's manage list shows, which
	// prices the loot a bot sells by trade (Brain.tradePrice).
	prices map[int32]int32
	// potions are the datapack's healing potions, best first
	// (indexPotions): what WithItemIndex puts in a persona.
	potions []int32
	// buffPotions are the datapack's buff potions (indexBuffPotions), and
	// stacks the stack effect of every skill level that shares a stack type
	// with one of them, by skill id then level: what stackOf reads the
	// bot's effects with.
	buffPotions map[int32]buffPotion
	stacks      map[int32]map[int32]stackEffect
	// shots are the datapack's player shots (indexShots), weapons what
	// each weapon template charges (indexWeapons), and spawnedSold the
	// items spawned merchants sell (spawnedSold).
	shots       map[int32]shotItem
	weapons     map[int32]weapon
	spawnedSold map[int32]bool
	// keepers are the warehouse keeper templates with a fixed spawn.
	keepers map[int32]bool
	// weights are the weights of the etc item templates that weigh
	// anything: the supplies' (SupplyPolicy.BuyLoad).
	weights map[int32]int32
	// recipes are the craft recipes by id (datapack.Datapack.Recipes), and
	// craftItems every template one of them uses or makes, its scroll
	// included: what the crafter plans with and deposits (Persona.Crafter).
	recipes    map[int32]datapack.Recipe
	craftItems map[int32]bool
	// products maps the lower-case name of each recipe product to its
	// template, and itemNames each such product to its name: what a craft
	// order names (indexProductNames).
	products  map[string]int32
	itemNames map[int32]string
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
// of every NPC template, the merchants' buylists, the gear bots wear and
// its armor sets, the arrows each bow shoots, the items' reference prices, the healing
// and buff potions and shots, the etc items' weights, the warehouse keepers
// and the craft recipes.
func NewTravelMap(dp *datapack.Datapack) *TravelMap {
	m := &TravelMap{spawns: make(map[int32][]Point)}
	m.shops, m.cheapest = indexShops(dp.BuyLists)
	m.gear, m.shopGear = indexGear(dp.Items, m.cheapest)
	m.sets, m.setPieces = indexSets(dp.ArmorSets, m.gear)
	m.arrows = indexArrows(dp.Items)
	m.prices = dp.Prices
	m.potions = indexPotions(dp.EtcItems, dp.Skills)
	m.buffPotions, m.stacks = indexBuffPotions(dp.EtcItems, dp.Skills)
	m.shots = indexShots(dp.EtcItems)
	m.weapons = indexWeapons(dp.Items)
	m.weights = indexWeights(dp.EtcItems)
	m.recipes, m.craftItems = dp.Recipes, indexCraftItems(dp.Recipes)
	m.products, m.itemNames = indexProductNames(dp.Recipes, dp.Items, dp.EtcItems)
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
	m.spawnedSold = spawnedSold(m.shops, m.spawns)
	m.keepers = make(map[int32]bool)
	for id := range m.spawns {
		if dp.NPCs[id].Type == npcTypeWarehouseKeeper {
			m.keepers[id] = true
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
			dests = append(dests, destination{index: i, at: Point{X: t.X, Y: t.Y, Z: t.Z}, price: t.PriceCount})
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
	// hopAt is when the next town hop is due (planHop), zero before the
	// idle spell to it started.
	hopAt time.Time
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
// rule planned (planTrip), or a town hop it plans itself when one is due
// (planHop), the bot walks to the gatekeeper, talks to it,
// opens its teleport list and sends the destination's link; its own
// teleport ends the trip (the brain sends Appearing) and the spot rule
// picks again where it lands. A gatekeeper that takes it nowhere is
// shunned, and so is one no route reaches. Bypasses
// pass the bypass gate (gateBypass); legs and clicks the frame pacer, a
// trip spending at most visitClicks clicks.
func (b *Brain) ruleTravel(now time.Time) (Action, bool) {
	tr := &b.trip
	v := &tr.v
	if b.world.Self.InCombat || (!v.active() && !b.planHop(now)) {
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
