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
	// weights are the weights of the item templates that weigh anything:
	// the supplies' (SupplyPolicy.BuyLoad) and the junk's a town visit
	// sheds (SupplyPolicy.ShedMin).
	weights map[int32]int32
	// recipes are the craft recipes by id (datapack.Datapack.Recipes), and
	// craftItems every template one of them uses or makes, its scroll
	// included: what the crafter plans with and deposits (Persona.Crafter).
	recipes    map[int32]datapack.Recipe
	craftItems map[int32]bool
	// crystals are the gear templates the server crystallizes, with what
	// a +0 one gives (indexCrystals): what the crafter weighs to
	// crystallize.
	crystals map[int32]crystalYield
	// hennas are the dye symbols by id (datapack.Datapack.Hennas): what
	// the dye errand checks prices, dyes and class lists with.
	hennas map[int32]datapack.Henna
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
// and buff potions and shots, the items' weights, the aggressive NPCs' aggro
// ranges, the warehouse keepers, the craft recipes, the crystals gear
// breaks into and the dye symbols.
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
	m.weights = indexWeights(dp.Items, dp.EtcItems)
	m.recipes, m.craftItems = dp.Recipes, indexCraftItems(dp.Recipes)
	m.crystals = indexCrystals(dp.Items)
	m.hennas = dp.Hennas
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
	// SecondPrice is the adena one second of walking is worth when a trip
	// weighs its walks against its teleports (planTrip).
	SecondPrice int32
}

// defaultTravelPolicy travels for up to the price of a trip between two
// villages, a minute of walking worth 600 adena.
func defaultTravelPolicy() TravelPolicy {
	return TravelPolicy{MaxPrice: 50000, Gap: 5 * time.Minute, SecondPrice: 10}
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
	// onward is how many teleports the planned path takes after this
	// trip's: on landing the next plan, which may take at most that many,
	// is due at once rather than after TravelPolicy.Gap (planTrip).
	onward int
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
	tr.v, tr.step, tr.dest, tr.endedAt, tr.onward = npcVisit{}, tripTalk, destination{}, now, 0
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
// behind. The spot rule picks again at once, and plans the path's next
// teleport, if any, without waiting out the gap.
func (b *Brain) tripTeleported(now time.Time) {
	tr := &b.trip
	if !tr.v.active() {
		return
	}
	onward := tr.onward
	tr.end(now, 0)
	tr.onward = onward
	b.spot.retryAt = time.Time{}
}

// Trip search bounds (docs/navigation.md, "Cost bounds").
const (
	// tripExpand caps the nodes one trip search expands: running out of
	// it is no trip for now.
	tripExpand = 256
	// tripSpeed is the speed a trip's walks are timed at before UserInfo
	// reports the bot's.
	tripSpeed = 100
)

// tripKey names a node of the trip search: where the bot stands, a
// gatekeeper by its index in the travel map, a destination by its point
// (many gatekeepers share one), a spot by its index in the registry.
type tripKey struct {
	kind tripKind
	i    int
	at   Point
}

type tripKind uint8

const (
	tripHere tripKind = iota
	tripGatekeeper
	tripDestination
	tripSpot
)

// tripLabel is the cheapest way the trip search found to a node so far.
type tripLabel struct {
	key   tripKey
	p     navPoint
	cost  float64
	price int32
	// hops counts the path's teleports.
	hops int
	// gk and dest are the path's first teleport, gk -1 before one.
	gk   int
	dest destination
}

// planTrip plans a trip when the spot rule found no spot to claim in
// walking reach of here, where the bot stands: Dijkstra over here, the
// gatekeepers, their destinations and the spots that fit the bot's level
// and have room (docs/navigation.md, "Consumers"). A walk costs its
// abstract distance over the bot's speed, priced at TravelPolicy.SecondPrice
// a second, and only joins two points of one SCC (navDistance) whose
// straight line is within SpotPolicy.MaxTravel; a teleport costs its adena,
// the path's teleports together no more than the bot carries and the
// persona allows. The trip is the cheapest path's first teleport: once it
// lands the spot rule picks again, and plans the next teleport, if any,
// from there at once (travelTrip.onward), on a path of no more teleports
// than the rest of the last, so a chain of trips ends. Labels are kept per
// node while no other there is as cheap, as dear and as short in teleports,
// so the budget never prunes the only path within it.
func (b *Brain) planTrip(here navPoint, now time.Time) {
	m, tr, p, s := b.cfg.Travel, &b.trip, &b.persona.Travel, &b.world.Self
	onward := tr.onward
	if m == nil || tr.v.active() || p.MaxPrice <= 0 || (onward == 0 && !tr.endedAt.IsZero() && now.Sub(tr.endedAt) < p.Gap) {
		return
	}
	tr.onward = 0
	maxHops := math.MaxInt
	if onward > 0 {
		maxHops = onward
	}
	var budget int32
	if it, ok := b.world.Inventory.ByTemplate(adenaID); ok {
		budget = min(p.MaxPrice, it.Count)
	}
	speed := float64(max(s.RunSpeed, s.WalkSpeed))
	if speed <= 0 {
		speed = tripSpeed
	}
	walk := func(d float64) float64 { return d / speed * float64(p.SecondPrice) }
	reach := int64(b.persona.Spots.MaxTravel) * int64(b.persona.Spots.MaxTravel)
	type located struct {
		p  navPoint
		ok bool
	}
	gks := make(map[int]located)
	labels := make(map[tripKey][]tripLabel)
	open := []tripLabel{{key: tripKey{kind: tripHere}, p: here, gk: -1}}
	push := func(l tripLabel) {
		for _, o := range labels[l.key] {
			if o.cost <= l.cost && o.price <= l.price && o.hops <= l.hops {
				return
			}
		}
		labels[l.key] = append(labels[l.key], l)
		open = append(open, l)
	}
	for expanded := 0; len(open) > 0 && expanded < tripExpand; {
		k := 0
		for j := range open {
			if open[j].cost < open[k].cost {
				k = j
			}
		}
		l := open[k]
		open[k] = open[len(open)-1]
		open = open[:len(open)-1]
		expanded++
		switch l.key.kind {
		case tripSpot:
			g := &m.gatekeepers[l.gk]
			tr.v = npcVisit{npcID: g.npcID, at: g.at}
			tr.step, tr.gk, tr.dest, tr.onward = tripTalk, l.gk, l.dest, l.hops-1
			return
		case tripGatekeeper:
			if l.hops >= maxHops {
				continue
			}
			for _, d := range m.gatekeepers[l.key.i].dests {
				price := l.price + d.price
				if price > budget {
					continue
				}
				dp, ok := b.locate(d.at)
				if !ok {
					continue
				}
				gk, first := l.gk, l.dest
				if gk < 0 {
					gk, first = l.key.i, d
				}
				push(tripLabel{key: tripKey{kind: tripDestination, at: d.at}, p: dp, cost: l.cost + float64(d.price), price: price, hops: l.hops + 1, gk: gk, dest: first})
			}
			continue
		}
		// Where the bot stands or a destination: walks to the gatekeepers in
		// reach, and from a destination to the spots.
		for gi := range m.gatekeepers {
			g := &m.gatekeepers[gi]
			if now.Before(tr.shunned[gi]) || horizontal2(l.p.at, g.at) > reach {
				continue
			}
			at, seen := gks[gi]
			if !seen {
				at.p, at.ok = b.locate(g.at)
				gks[gi] = at
			}
			if !at.ok {
				continue
			}
			if d, ok := b.navDistance(l.p, at.p); ok {
				push(tripLabel{key: tripKey{kind: tripGatekeeper, i: gi}, p: at.p, cost: l.cost + walk(d), price: l.price, hops: l.hops, gk: l.gk, dest: l.dest})
			}
		}
		if l.key.kind == tripDestination {
			b.tripSpots(l, reach, walk, now, push)
		}
	}
}

// tripSpots pushes the walks from destination l to the spots within reach
// (squared, straight line) of it that fit the bot's level, are not avoided,
// have room and lie in its SCC.
func (b *Brain) tripSpots(l tripLabel, reach int64, walk func(float64) float64, now time.Time, push func(tripLabel)) {
	r, v, p := b.cfg.Spots, &b.spot, &b.persona.Spots
	level := int(b.world.Self.Level)
	for i := range r.spots {
		sp := &r.spots[i]
		if now.Before(v.avoid[i]) || !p.fits(sp, level) || !r.hasRoom(i) ||
			horizontal2(l.p.at, Point{X: sp.Center.X, Y: sp.Center.Y}) > reach {
			continue
		}
		at, ok := b.spotPoint(sp, l.p)
		if !ok {
			continue
		}
		if d, ok := b.navDistance(l.p, at); ok {
			push(tripLabel{key: tripKey{kind: tripSpot, i: i}, p: at, cost: l.cost + walk(d), price: l.price, hops: l.hops, gk: l.gk, dest: l.dest})
		}
	}
}
