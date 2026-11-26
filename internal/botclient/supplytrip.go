package botclient

import "time"

const (
	// supplyCheck is how often a farmer at its spot reads the supply trip's
	// triggers (supplyDue).
	supplyCheck = 5 * time.Second
	// supplyTripGap is how long after a trip ended no other starts: a bot
	// back at its spot no better off (the town had nothing for it) farms
	// for a while instead of turning round at once.
	supplyTripGap = 10 * time.Minute
	// townArrive is how close to the town's gatekeeper a walk home has
	// arrived.
	townArrive = 600
)

// tripCause is what started a supply trip (supplyCause).
type tripCause uint8

const (
	causeNone tripCause = iota
	// causeRunOut: a consumable runs out within Supply.ReturnMinutes of
	// fighting.
	causeRunOut
	// causePotions: the HP potions the bot carried at its spot (a join
	// member: by its leader) are gone.
	causePotions
	// causeLoad: the load is past Supply.ReturnLoad and a town visit
	// sheds at least Supply.ShedMin of the limit.
	causeLoad
	// causeGear: the adena past the gear floor pays the gear goal.
	causeGear
	// causeLeader: the bot leader of a join member teleported away (a
	// restart, its own supply trip), the member carrying a Scroll of Escape
	// with nothing attacking it (docs/parties/members.md, "Events").
	causeLeader
	// causePenalty: the bot has a death penalty and no Scroll: Recovery of
	// its band, and the town sells one its adena pays (penaltyTrip, #491).
	causePenalty
	// causeStash: the load is in the half-speed band past
	// Supply.ReturnLoad, a town visit sheds nothing, and the bot carries
	// kept items it does not use to deposit in its own warehouse
	// (queueStash, #449).
	causeStash
	// causeRotation: a town bot's farming spell is over (townRota, #618).
	causeRotation
)

// supplyTrip is a farmer's trip home to restock (rule "supply", D16,
// docs/items.md section 7): from its trigger until it is in town. The brain
// goroutine owns it.
type supplyTrip struct {
	// checkAt is when the triggers are read next (supplyCheck).
	checkAt time.Time
	// potions is set once the bot carried its HP potions at its spot since
	// the last trip: only then does their running out send it home, so a
	// bot that could not buy any farms all the same.
	potions bool
	// since is when the trip started, zero for none; endedAt is when the
	// last one ended (supplyTripGap).
	since, endedAt time.Time
	cause          tripCause
	// walk is set once the trip walks home instead (no Scroll of Escape,
	// no safe moment within Supply.SafeWait, a recall that took it
	// nowhere), and town is the town it walks to.
	walk bool
	town Point
	// gate is set once the trip takes a gatekeeper instead of the walk
	// (planTown): the travel errand's trip (Brain.trip) carries it, and its
	// last teleport ends the trip in town.
	gate bool
	// escapeAt is when the Scroll of Escape went out, zero before it: the
	// escaping hold runs from it (escaping).
	escapeAt time.Time
}

// active reports whether a trip is under way.
func (t *supplyTrip) active() bool { return !t.since.IsZero() }

// end ends the trip at now.
func (t *supplyTrip) end(now time.Time) {
	*t = supplyTrip{endedAt: now, checkAt: t.checkAt}
}

// ruleEscape is the escaping hold: from the Scroll of Escape's UseItem
// until the teleport or the recall's end, every later rule that moves,
// attacks, loots, casts or clicks is held, since the server queues a move
// or a cast sent mid-cast and runs it at the cast's end, after the teleport
// (network/movement.go tryLiveMoveTo). Damage does not break the recall
// (cast/controller.go: InterruptOnDamage stops magic casts only). A recall
// that ended with no teleport sends the trip walking.
func (b *Brain) ruleEscape(now time.Time) (Action, bool) {
	return Action{}, b.escaping(now)
}

// escaping reports whether the escaping hold runs at now: the recall cast
// runs, or less than escapeRetry has passed since its end (escapeWait), the
// TeleportToLocation landing just after it, or since the UseItem when no
// cast started (a refusal). Past that, the trip walks.
func (b *Brain) escaping(now time.Time) bool {
	v, s := &b.supplyTrip, &b.world.Self
	if v.escapeAt.IsZero() {
		return false
	}
	if s.Casting(now) || now.Sub(escapeWait(v.escapeAt, s.castEnd)) < escapeRetry {
		return true
	}
	v.escapeAt, v.walk = time.Time{}, true
	return false
}

// ruleSupply is the supply trip, a rule 9 errand ahead of following and
// farming (D16): a farmer at its spot whose triggers hold (supplyDue)
// releases the spot to the registry and goes home. With a Scroll of Escape
// it waits for a safe moment, no attacker (safeToEscape), stands up and
// reads it; the escaping hold
// (ruleEscape) takes over until the teleport, which ends the trip. With no
// scroll, with no safe moment within Supply.SafeWait, or after a recall
// that took it nowhere, it walks to the nearest town (walkHome), or takes a
// gatekeeper when it cannot walk there (planTown). Back in town the spot
// rule picks again, as at a level-up.
//
// The UseItem has no server gate but the item's own (network/item_use_gate.go):
// it goes out once, and no other before the recall ends or escapeRetry
// (escaping). Walks are the walker's (goTo, the frame pacer); a gatekeeper
// trip is the travel errand's (ruleTravel: the bypass gate, Travel.Gap).
func (b *Brain) ruleSupply(now time.Time) (Action, bool) {
	v, s := &b.supplyTrip, &b.world.Self
	if !v.active() {
		cause := b.supplyDue(now)
		if cause == causeNone {
			return Action{}, false
		}
		v.since, v.cause = now, cause
		b.leaveSpot()
	}
	if v.walk {
		return b.walkHome(now)
	}
	if !v.escapeAt.IsZero() {
		return Action{}, true
	}
	it, ok := b.world.Inventory.ByTemplate(scrollOfEscape)
	if !ok {
		v.walk = true
		return b.walkHome(now)
	}
	if !b.safeToEscape(now) {
		if now.Sub(v.since) >= b.persona.Supply.SafeWait {
			v.walk = true
			return b.walkHome(now)
		}
		return Action{}, true
	}
	switch {
	case s.Sitting:
		return b.toggle(now), true
	case s.Casting(now) || now.Sub(b.store.stoodAt) < storeStandSettle:
		// The server queues an item cast behind a cast or a stand-up
		// (item_skill_cast.go itemAICastBusy): wait them out.
		return Action{}, true
	}
	return useItemAction(it), true
}

// sent records the supply trip's Scroll of Escape going out at now.
func (t *supplyTrip) sent(a Action, now time.Time) {
	if t.active() && a.kind == actUseItem && a.item == scrollOfEscape {
		t.escapeAt = now
	}
}

// teleported ends the trip on the bot's own loading-screen teleport: the
// recall's landing in town, the gatekeeper trip's last landing, or a
// teleport of another cause (a restart, a GM) that took it elsewhere. gk
// reports that the travel errand's trip was waiting for its teleport, and
// onward how many its path takes after it: a gatekeeper trip's landing on
// the way does not end the trip, which plans its next teleport from there.
// It reports whether the teleport was an arrival (townArrival): the Scroll
// of Escape went out (escapeAt), or the gatekeeper trip landed in town. A
// restart arms the gate through its Revive instead.
func (t *supplyTrip) teleported(now time.Time, gk bool, onward int) bool {
	if !t.active() {
		return false
	}
	if t.gate && gk && onward > 0 {
		return false
	}
	landed := !t.escapeAt.IsZero() || t.gate && gk
	t.end(now)
	return landed
}

// walkHome walks the trip to the town nearest where it started walking (the
// travel map's towns, by their gatekeeper) and ends it on arrival
// (townArrival). A town out of walking reach (onFoot), or one the walker
// answers unreachable (its own gatekeepers then shunned, shunTown), sends
// it to a gatekeeper (planTown, gateHome); with
// no trip to take either, a town out of reach is walked to all the same
// (the walker decides), and one it cannot walk to ends the trip there: the
// bot farms on and tries again after supplyTripGap.
func (b *Brain) walkHome(now time.Time) (Action, bool) {
	v := &b.supplyTrip
	if v.gate {
		return b.gateHome(now)
	}
	if v.town == (Point{}) {
		at := b.world.Self.Position(now)
		t := b.nearestTown(at)
		if t == nil {
			v.end(now)
			return Action{}, false
		}
		v.town = *t
		if !b.onFoot(at, v.town) && b.planTown(now) {
			return b.gateHome(now)
		}
	}
	a, st := b.goTo(v.town, townArrive, walkPlace, now)
	switch st {
	case walkWalking:
		return a, true
	case walkUnreachable:
		b.shunTown(now)
		if b.planTown(now) {
			return b.gateHome(now)
		}
	}
	v.end(now)
	if st == walkArrived {
		b.townArrival()
	}
	return Action{}, false
}

// shunTown shuns, for visitShunFor, the travel map's gatekeepers within
// townArrive of the supply trip's town, which the walker answered
// unreachable: the town is a gatekeeper's spawn (TravelMap.Towns), so a
// walk to one of them ends where the walk home did, and a gatekeeper trip
// planned from it would fail at its first leg.
func (b *Brain) shunTown(now time.Time) {
	tr, town := &b.trip, b.supplyTrip.town
	for gi := range b.cfg.Travel.gatekeepers {
		if horizontal2(b.cfg.Travel.gatekeepers[gi].at, town) > townArrive*townArrive {
			continue
		}
		tr.shunned.set(gi, now.Add(visitShunFor))
	}
}

// onFoot reports whether the bot at at is in walking reach of town: both on
// the navigation, in one SCC (navDistance) and within Spots.MaxTravel in
// straight line, as the trip search walks (planTrip). Off the navigation
// the walker decides.
func (b *Brain) onFoot(at, town Point) bool {
	from, ok := b.locate(at)
	if !ok {
		return true
	}
	to, ok := b.locatePlace(town)
	if !ok {
		return true
	}
	reach := int64(b.persona.Spots.MaxTravel)
	if horizontal2(at, town) > reach*reach {
		return false
	}
	_, ok = b.navDistance(from, to)
	return ok
}

// planTown plans the supply trip's gatekeeper trip from where the bot
// stands (planPath): the cheapest path whose last teleport lands in the
// trip's town, else in any town of the travel map, within Travel.MaxPrice
// and the adena carried, and after Travel.Gap since the last trip unless a
// landing on the way plans the next teleport. It reports whether it planned
// one, and marks the trip a gatekeeper's.
func (b *Brain) planTown(now time.Time) bool {
	v, tr := &b.supplyTrip, &b.trip
	here, ok := b.locate(b.world.Self.Position(now))
	if !ok {
		return false
	}
	onward := tr.onward
	if !b.planPath(here, now, []Point{v.town}, storeTownRadius) {
		tr.onward = onward
		if !b.planPath(here, now, b.towns, storeTownRadius) {
			return false
		}
	}
	v.gate = true
	return true
}

// gateHome runs the supply trip's gatekeeper trip through the travel
// errand (ruleTravel), and plans the next teleport after a landing on the
// way. A trip that ends with no teleport (refused, a gatekeeper no route
// reaches) and no next one to plan ends the supply trip: the bot farms on
// and tries again after supplyTripGap. In a fight the errand yields.
func (b *Brain) gateHome(now time.Time) (Action, bool) {
	v, tr := &b.supplyTrip, &b.trip
	if !tr.v.active() && !b.planTown(now) {
		v.end(now)
		return Action{}, false
	}
	a, ok := b.ruleTravel(now)
	if !ok && !tr.v.active() {
		v.end(now)
	}
	return a, ok
}

// safeToEscape reports whether the bot may read its Scroll of Escape at
// now: nothing attacks it. Only an attacker can spoil the read: the recall
// (item skill 2013) is not a magic cast, so a hit cannot break it
// (cast/controller.go InterruptOnDamage), and what aborts it is the bot's
// death or a stun, sleep or paralysis a fight lands (skill/effect/hooks_cc.go).
// A monster that would attack on sight within its aggro range but has not
// does not block it: at a starter spot nearly every monster has one (#251),
// and one that closes in during the read only deals damage.
func (b *Brain) safeToEscape(now time.Time) bool {
	for id := range b.world.Attackers {
		if b.world.attacking(id, now) {
			return false
		}
	}
	return true
}

// supplyDue is the cause of a supply trip due at now, causeNone for none,
// never within supplyTripGap of the last trip: the bot leader of a join
// member with no spot teleported away (leaderTeleported); else the triggers
// (supplyCause), read every supplyCheck at the bot's spot, or for a join
// member of its bot party within GatherRange of its leader (nearSlotLeader),
// and last a town bot's farming spell over (causeRotation).
func (b *Brain) supplyDue(now time.Time) tripCause {
	v := &b.supplyTrip
	if b.cfg.Travel == nil || !v.endedAt.IsZero() && now.Sub(v.endedAt) < supplyTripGap {
		return causeNone
	}
	if b.leaderTeleported(now) {
		return causeLeader
	}
	if !(b.spot.farming() && b.spot.arrived) && !b.nearSlotLeader(now) || now.Before(v.checkAt) {
		return causeNone
	}
	v.checkAt = now.Add(supplyCheck)
	c := b.supplyCause()
	if c == causeStash && !b.stashOpen(now) {
		c = causeNone
	}
	if c == causeNone && b.farmOver(now) {
		c = causeRotation
	}
	return c
}

// slotMember reports whether the bot is a join member of its bot party
// with no spot farmed, out of town at now: the party a member's supply trip
// answers for (docs/parties/members.md, "Events"). In town a recall would
// land it in a town again, so it takes no trip there.
func (b *Brain) slotMember(now time.Time) bool {
	l := b.cfg.link
	return l != nil && l.party.Kind == CommandJoin && b.inBotParty() && !b.leading() && !b.spot.farming() && !b.inTown(now)
}

// leaderTeleported reports whether a join member's bot leader teleported
// away (leaderWatch.teleported, not seen since, the bot not yet landed and
// not on the leader's gatekeeper trip, partyTrip) and
// the member can recall at now: it carries a Scroll of Escape and nothing
// attacks it (safeToEscape). Without either it follows the leader's last
// reported position instead (rule "follow").
func (b *Brain) leaderTeleported(now time.Time) bool {
	v := &b.lead
	if !b.slotMember(now) || b.partyTrip() || v.lost.IsZero() || !v.teleported || v.seen || v.home || v.name != slotLeader(b.cfg.link.party) {
		return false
	}
	if _, ok := b.world.Inventory.ByTemplate(scrollOfEscape); !ok {
		return false
	}
	return b.safeToEscape(now)
}

// nearSlotLeader reports whether a join member of its bot party is within
// LeadPolicy.GatherRange of its leader, in sight or where last reported
// (PartyMemberPosition): where its triggers are read in place of a spot.
func (b *Brain) nearSlotLeader(now time.Time) bool {
	if !b.slotMember(now) {
		return false
	}
	lead, ok := b.leader(b.world.Party.LeaderID, now)
	if !ok {
		return false
	}
	r := int64(b.persona.Lead.withDefaults().GatherRange)
	return dist2(b.world.Self.Position(now), lead.pos) <= r*r
}

// supplyCause is the first trigger of a supply trip that holds
// (docs/items.md section 7), causeNone for none: a death penalty it has
// no Scroll: Recovery for and can buy one (penaltyTrip), a consumable it uses
// fighting runs out within ReturnMinutes, its HP potions are gone, its load
// is past ReturnLoad with ShedMin to shed, or in the half-speed band
// (slowed) with anything to shed, or it carries the price of the gear
// goal, or, last, in the half-speed band with nothing to shed, it carries
// kept items it does not use to stash (causeStash; supplyDue holds it while
// stashOpen does not). The Extra supplies and the Scrolls of Escape count
// toward none.
func (b *Brain) supplyCause() tripCause {
	v := &b.supplyTrip
	potion := b.supplyPotion() != 0
	var potions int32
	for _, id := range b.persona.Potions {
		potions += b.carried(id)
	}
	if potion && potions > 0 {
		v.potions = true
	}
	switch {
	case b.penaltyTrip():
		return causePenalty
	case b.runsOut():
		return causeRunOut
	case potion && v.potions && potions == 0:
		return causePotions
	case b.loadSendsHome():
		return causeLoad
	case b.gearPaid():
		return causeGear
	case b.overloaded() && b.slowed() && len(b.stashRows()) > 0:
		return causeStash
	}
	return causeNone
}

// runsOut reports whether a consumable the bot carries runs out within
// Supply.ReturnMinutes of fighting at the use meter's rate: the shots of a
// weapon it uses or will (shotWeapons), or its HP potions when a merchant
// sells one (supplyPotion), or a reagent of its kit falls under its
// ReagentCastsMin sum (reagentShort). One it carries none of does not
// count: a bot that could not buy it farms without it.
func (b *Brain) runsOut() bool {
	mins := float64(b.persona.Supply.ReturnMinutes)
	if mins <= 0 {
		return false
	}
	low := func(ids []int32, rate float64) bool {
		var n int32
		for _, id := range ids {
			n += b.carried(id)
		}
		return n > 0 && float64(n) < rate*mins
	}
	for _, w := range b.shotWeapons() {
		if ids := b.shotsFor(w); len(ids) > 0 && low(ids, b.meter.rate(ids, b.firstShotRate(w))) {
			return true
		}
	}
	p := &b.persona
	return b.supplyPotion() != 0 && low(p.Potions, b.meter.rate(p.Potions, p.Supply.PotionsPerHour/60)) || b.reagentShort()
}

// loadSendsHome reports whether the load sends the bot to town to shed it
// (causeLoad): past Supply.ReturnLoad (overloaded) with Supply.ShedMin
// percent of the limit to shed, or in the half-speed band (slowed) with
// anything to shed. The shop visit sells junk under Shop.SellAt stacks
// then (startShop), so the trip sheds it.
func (b *Brain) loadSendsHome() bool {
	if !b.overloaded() {
		return false
	}
	shed := b.shed()
	return shed*100 >= int64(b.world.Self.MaxLoad)*int64(b.persona.Supply.ShedMin) || b.slowed() && shed > 0
}

// overloaded reports whether the load is past Supply.ReturnLoad percent of
// the weight limit.
func (b *Brain) overloaded() bool {
	s, pct := &b.world.Self, b.persona.Supply.ReturnLoad
	return pct > 0 && s.MaxLoad > 0 && int64(s.Load)*100 > int64(s.MaxLoad)*int64(pct)
}

// shed is the weight a town visit takes off the bot: the junk it sells
// (ShopPolicy.SellAt) and what the trip in town hook hands off
// (SupplyPolicy.HandOff). Items it keeps with nowhere to go shed nothing.
func (b *Brain) shed() int64 {
	var w int64
	if f := b.persona.Supply.HandOff; f != nil {
		w = max(f(b.world), 0)
	}
	if b.persona.Shop.SellAt <= 0 {
		return w
	}
	for _, it := range b.junk() {
		w += int64(it.Count) * int64(b.cfg.Travel.weights[it.TemplateID])
	}
	return w
}

// gearPaid reports whether the adena the bot carries past the gear tier's
// floor pays its gear goal at the cheapest base prices (gearGoal), and the
// goal fits under the half-speed band on top of the load it carries now
// (loadFits), as the shop errand's gearWants requires: a goal it would not
// buy never sends it to town.
func (b *Brain) gearPaid() bool {
	if !b.persona.Gear.Buy {
		return false
	}
	goal, _ := b.gearGoal()
	if len(goal) == 0 || !b.loadFits(oneEach(goal)...) {
		return false
	}
	price := b.price(goal)
	return price > 0 && int64(b.carried(adenaID))-b.floor(tierGear) >= price
}
