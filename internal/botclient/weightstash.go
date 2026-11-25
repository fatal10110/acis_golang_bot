package botclient

import (
	"cmp"
	"slices"
	"time"
)

// stashRetry is how long a half-speed stash that moved nothing (no keeper,
// a full warehouse, the fee short, a refusal) holds the next one off: no
// stash trip starts and no arrival stashes before it, so a bot that cannot
// stash does not walk home every supplyTripGap for it.
const stashRetry = time.Hour

// The half-speed stash (docs/items.md section 7, "Going home", #449): a
// farmer in the half-speed band (slowed) whose town visit sheds nothing
// (shed) but which carries kept items it does not use (stashes) goes home
// (causeStash) and deposits them in its own warehouse. Any supply trip's
// arrival in town stashes the same way while the bot is still slowed once
// its shop visit is over. The outcome is Status.Stash.

// stashes reports whether the stash deposits carried item it: kept rather
// than sold or not sellable (Shop.Keep, an item the shop refused, gear
// from a trade or a grant) with no use of the bot's own, of a known weight
// above zero. Never
// a worn item, adena, a quest item, a valuable (the valuables ledger's,
// D33), what the keep hook keeps (the clan member's deposit, D17/D19), a
// supply, shot, potion or keep-set item (uses), nor gear the bot wears
// next, falls back to or keeps for a set.
func (b *Brain) stashes(it Item, w *worn) bool {
	t := b.cfg.Travel
	if t == nil || it.Equipped || it.TemplateID == adenaID || it.SubCategory == subCategoryQuest || t.weights[it.TemplateID] <= 0 {
		return false
	}
	return !b.uses(it.TemplateID) && !b.valuable(it) && !b.hold(it).Keep &&
		!b.upgrade(it, w) && !b.fallsBack(it, w) && !b.keepsForSet(it, w)
}

// stashRows are the rows of the items the stash deposits (stashes), the
// heaviest stack first, then by object id. With selling on (Shop.SellAt)
// it leaves what the shop visit sells (junk): the stash never deposits for
// 30 adena a row what a merchant would pay for.
func (b *Brain) stashRows() []WarehouseRow {
	w := b.worn()
	t := b.cfg.Travel
	var junk map[int32]Item
	if b.persona.Shop.SellAt > 0 {
		junk = b.junk()
	}
	var rows []WarehouseRow
	for id, it := range b.world.Inventory.Items {
		if _, sold := junk[id]; !sold && b.stashes(it, &w) {
			rows = append(rows, WarehouseRow{TemplateID: it.TemplateID, Count: it.Count, Object: it.ObjectID})
		}
	}
	weight := func(r WarehouseRow) int64 { return int64(t.weights[r.TemplateID]) * int64(max(r.Count, 1)) }
	slices.SortFunc(rows, func(a, c WarehouseRow) int {
		return cmp.Or(cmp.Compare(weight(c), weight(a)), cmp.Compare(a.Object, c.Object))
	})
	return rows
}

// stashHeld reports whether the last stash moved nothing less than
// stashRetry before now.
func (b *Brain) stashHeld(now time.Time) bool {
	r := b.wh.stashed
	return r.Outcome != WarehouseNone && r.Outcome != WarehouseDone && now.Sub(r.At) < stashRetry
}

// stashOpen reports whether a stash trip may start at now: no stash is held
// (stashHeld) and the travel map has warehouse keepers. With none it
// records the stash as WarehouseUnreachable for Status, which holds it.
func (b *Brain) stashOpen(now time.Time) bool {
	if b.stashHeld(now) {
		return false
	}
	if len(b.cfg.Travel.keepers) == 0 {
		b.wh.stashed = WarehouseResult{Outcome: WarehouseUnreachable, At: now}
		return false
	}
	return true
}

// queueStash queues and starts the stash after a supply trip's arrival
// (stashDue), under rule "warehouse": once the shop visit and the
// preparation gate are over, the bot still slowed, no request running and
// an open due, it deposits stashRows in its own warehouse at the nearest
// unmarked keeper within Warehouse.Reach of where it stands, as many rows
// as the adena past the valuables' fees (valuableFees) pays warehouseFee
// for, at most warehouseMaxRows. Not slowed, with nothing to stash, held
// (stashHeld) or back at a spot, no stash runs. One that cannot start
// records its outcome (stashed).
//
// Pacing: the errand's own (rule "warehouse"): opens MinOpenGap apart, one
// deposit per open, links through the bypass gate.
func (b *Brain) queueStash(now time.Time) {
	e := &b.wh
	if !b.stashDue || e.running || !e.privateWant.empty() || b.shop.v.active() || b.preparing() ||
		!e.openDue(now) || now.Before(e.retryAt) {
		return
	}
	b.stashDue = false
	if b.spot.farming() || !b.slowed() || b.stashHeld(now) {
		return
	}
	rows := b.stashRows()
	if len(rows) == 0 {
		return
	}
	if len(b.cfg.Travel.keepers) == 0 {
		e.stashed = WarehouseResult{Outcome: WarehouseUnreachable, At: now}
		return
	}
	n := max((int64(b.carried(adenaID))-b.valuableFees())/warehouseFee, 0)
	rows = rows[:min(int64(len(rows)), n, warehouseMaxRows)]
	if len(rows) == 0 {
		e.stashed = WarehouseResult{Outcome: WarehouseShort, At: now}
		return
	}
	e.privateWant = WarehouseRequest{Deposit: rows, Private: true, Anchor: b.world.Self.Position(now), Reach: b.persona.Warehouse.Reach, stash: true}
	if !b.startWarehouse(now) {
		e.privateWant = WarehouseRequest{}
	}
}
