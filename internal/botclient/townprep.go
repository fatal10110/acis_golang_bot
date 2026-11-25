package botclient

import (
	"slices"
	"time"
)

// townPrep is the town preparation gate (docs/items.md section 7): from a
// farmer's arrival in town on a supply trip, its respawn or its login
// until it is ready to leave, it claims no spot (ruleSpot) and the shop
// errand buys its supplies up to their trip targets (refills), not only
// once they fall under Supply.Refill percent. The brain goroutine owns it.
type townPrep struct {
	armed bool
	// rounds is the shop errand's rounds (shopVisit.rounds) when the gate
	// was armed: one more opens it.
	rounds int
}

// armPrep arms the preparation gate of a farmer (a spot registry and a
// travel map) and lets the shop errand start a visit at once: a miss far
// from town held the next one off shopRetry.
func (b *Brain) armPrep() {
	if b.cfg.Spots == nil || b.cfg.Travel == nil {
		return
	}
	b.prep = townPrep{armed: true, rounds: b.shop.rounds}
	b.shop.nextAt = time.Time{}
}

// townArrival is a supply trip's arrival in town: it arms the preparation
// gate and the half-speed stash (stashDue, queueStash), then calls the trip
// in town hook (SupplyPolicy.InTown), once per arrival.
func (b *Brain) townArrival() {
	b.armPrep()
	b.stashDue = true
	if f := b.persona.Supply.InTown; f != nil {
		f(b.world)
	}
}

// preparing reports whether the preparation gate holds the bot in town:
// it is armed and a supply is short of its trip target (prepShort). The
// gate opens, until armed again, once the targets are met, nothing short
// is sold within reach or paid by the budget (a broke bot still goes and
// farms), or the shop errand ended a round since it was armed: a visit
// over, so the budget spent what it may, or a search that found no
// merchant within reach.
func (b *Brain) preparing() bool {
	v := &b.prep
	if v.armed && (b.shop.rounds != v.rounds || !b.prepShort()) {
		v.armed = false
	}
	return v.armed
}

// prepShort reports whether a supply of the bot's own is short of its trip
// target (shotWants, supplyWants: the Extra ones count not) and a spawned
// merchant within reach sells it (soldNear) at a base price the adena
// past the supplies tier's floor pays.
func (b *Brain) prepShort() bool {
	adena := int64(b.carried(adenaID)) - b.floor(tierSupplies)
	for _, r := range slices.Concat(b.shotWants(), b.supplyWants()) {
		if price, ok := b.cfg.Travel.cheapest[r.item]; ok && int64(price) <= adena && b.soldNear(r.item) {
			return true
		}
	}
	return false
}
