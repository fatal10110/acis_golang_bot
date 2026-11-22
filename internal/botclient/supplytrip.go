package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

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
	// causePotions: the HP potions the bot carried at its spot are gone.
	causePotions
	// causeLoad: the load is past Supply.ReturnLoad and a town visit
	// sheds at least Supply.ShedMin of the limit.
	causeLoad
	// causeGear: the adena past the gear floor pays the gear goal.
	causeGear
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
// it waits for a safe moment, no attacker and nothing aggressive within its
// aggro range (safeToEscape), stands up and reads it; the escaping hold
// (ruleEscape) takes over until the teleport, which ends the trip. With no
// scroll, with no safe moment within Supply.SafeWait, or after a recall
// that took it nowhere, it walks to the nearest town (walkHome). Back in
// town the spot rule picks again, as at a level-up.
//
// The UseItem has no server gate but the item's own (network/item_use_gate.go):
// it goes out once, and no other before the recall ends or escapeRetry
// (escaping). Walks are the walker's (goTo, the frame pacer).
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
// recall's landing in town, or a teleport of another cause (a restart, a
// GM) that took it elsewhere. It reports whether it ended one: an arrival
// (townArrival).
func (t *supplyTrip) teleported(now time.Time) bool {
	if !t.active() {
		return false
	}
	t.end(now)
	return true
}

// walkHome walks the trip to the town nearest where it started walking (the
// travel map's towns, by their gatekeeper) and ends it on arrival
// (townArrival). A town it cannot walk to ends the trip there: the bot
// farms on and tries again after supplyTripGap.
func (b *Brain) walkHome(now time.Time) (Action, bool) {
	v := &b.supplyTrip
	if v.town == (Point{}) {
		t := b.nearestTown(b.world.Self.Position(now))
		if t == nil {
			v.end(now)
			return Action{}, false
		}
		v.town = *t
	}
	a, st := b.goTo(v.town, townArrive, walkPlace, now)
	if st == walkWalking {
		return a, true
	}
	v.end(now)
	if st == walkArrived {
		b.townArrival()
	}
	return Action{}, false
}

// safeToEscape reports whether the bot may read its Scroll of Escape at
// now: nothing attacks it, and no live monster with an aggro range stands
// within it of the bot (the server's aggressive monsters attack on sight
// within aggroRange, npc.go).
func (b *Brain) safeToEscape(now time.Time) bool {
	for id := range b.world.Attackers {
		if b.world.attacking(id, now) {
			return false
		}
	}
	if b.cfg.Travel == nil {
		return true
	}
	at := b.world.Self.Position(now)
	for _, o := range b.world.Objects {
		if o.Kind != KindNPC || o.Dead {
			continue
		}
		if r := b.cfg.Travel.aggro[o.NPCID]; r > 0 && dist2(o.Position(now), at) <= r*r {
			return false
		}
	}
	return true
}

// supplyDue is the cause of a supply trip due at now, causeNone for none:
// read every supplyCheck at the bot's spot, never within supplyTripGap of
// the last trip.
func (b *Brain) supplyDue(now time.Time) tripCause {
	v := &b.supplyTrip
	if b.cfg.Travel == nil || !b.spot.held || !b.spot.arrived || now.Before(v.checkAt) ||
		!v.endedAt.IsZero() && now.Sub(v.endedAt) < supplyTripGap {
		return causeNone
	}
	v.checkAt = now.Add(supplyCheck)
	return b.supplyCause()
}

// supplyCause is the first trigger of a supply trip that holds
// (docs/items.md section 7), causeNone for none: a consumable it uses
// fighting runs out within ReturnMinutes, its HP potions are gone, its load
// is past ReturnLoad with ShedMin to shed, or it carries the price of the
// gear goal. The Extra supplies and the Scrolls of Escape count toward
// none.
func (b *Brain) supplyCause() tripCause {
	p, v := &b.persona.Supply, &b.supplyTrip
	potion := b.supplyPotion() != 0
	var potions int32
	for _, id := range b.persona.Potions {
		potions += b.carried(id)
	}
	if potion && potions > 0 {
		v.potions = true
	}
	switch {
	case b.runsOut():
		return causeRunOut
	case potion && v.potions && potions == 0:
		return causePotions
	case b.overloaded() && b.shed()*100 >= int64(b.world.Self.MaxLoad)*int64(p.ShedMin):
		return causeLoad
	case b.gearPaid():
		return causeGear
	}
	return causeNone
}

// runsOut reports whether a consumable the bot carries runs out within
// Supply.ReturnMinutes of fighting at the use meter's rate: the shots of a
// weapon it uses or will (shotWeapons), or its HP potions when a merchant
// sells one (supplyPotion). One it carries none of does not count: a bot
// that could not buy it farms without it.
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
	return b.supplyPotion() != 0 && low(p.Potions, b.meter.rate(p.Potions, p.Supply.PotionsPerHour/60))
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
// floor pays its gear goal at the cheapest base prices (gearGoal).
func (b *Brain) gearPaid() bool {
	if !b.persona.Gear.Buy {
		return false
	}
	goal, _ := b.gearGoal()
	if len(goal) == 0 {
		return false
	}
	price := b.price(goal)
	return price > 0 && int64(b.carried(adenaID))-b.floor(tierGear) >= price
}

// indexAggro maps each NPC template with an aggro range to it: the
// aggressive monsters (datapack.NPC.AggroRange).
func indexAggro(npcs map[int32]datapack.NPC) map[int32]int64 {
	out := make(map[int32]int64)
	for id, n := range npcs {
		if n.AggroRange > 0 {
			out[id] = int64(n.AggroRange)
		}
	}
	return out
}
