package botclient

import (
	"cmp"
	"slices"
	"time"
)

// The inventory slot limit (#528). The server refuses a pickup needing a
// slot past the character's inventory limit (UserInfo.InvLimit, every item
// held counting, worn ones included) with ActionFailed and SystemMessage 129
// (network/pickup.go), but only when it checks the picker's room at all:
// outside a party, or in a finders-keepers one (pickupNeedsRoom). It is an
// operational limit, not a persona default: no pickup the bot knows the
// server refuses goes out.

// Loot stall (#528): a bot whose clicks on ground items pick nothing up
// would hold the loot rule, and every rule after it, for the whole queue.
const (
	// lootMisses is how many ground items in a row the bot gives up after
	// clicking them for Loot.Timeout before it leaves the ground alone.
	lootMisses = 3
	// lootPause is how long it then leaves ground items alone, unless one
	// of its pickups succeeds first; its own kills' drops it still takes.
	lootPause = 2 * time.Minute
)

// roomChecked reports whether the server checks the bot's own inventory
// room for a ground item it picks up (pickup.go pickupNeedsRoom): outside a
// party, or in a finders-keepers one.
func (b *Brain) roomChecked() bool {
	pt := &b.world.Party
	return !pt.In() || pt.Loot == partyLootFindersKeepers
}

// slotsNeeded is how many inventory slots picking up ground item it takes,
// as the server counts them (itemcontainer.Inventory.SlotsNeededFor): none
// for a stackable item whose template the bot holds, nor for a herb (the
// datapack's, TravelMap.herbs), else one.
func (b *Brain) slotsNeeded(it GroundItem) int {
	if t := b.cfg.Travel; t != nil && t.herbs[it.TemplateID] {
		return 0
	}
	if _, held := b.world.Inventory.ByTemplate(it.TemplateID); it.Stackable && held {
		return 0
	}
	return 1
}

// slotsFit reports whether the server has room for ground item it in the
// bot's inventory: it needs no slot, or the room is not checked
// (roomChecked), or the bot holds fewer items than its inventory limit.
// After a pickup refused for want of a slot (lootRefused), no item needing
// one fits until the bot holds fewer items than it did then. With the limit
// unknown only that refusal counts.
func (b *Brain) slotsFit(it GroundItem) bool {
	if b.slotsNeeded(it) == 0 {
		return true
	}
	n, l := len(b.world.Inventory.Items), &b.loot
	if l.full > 0 && n < l.full {
		l.full = 0
	}
	if l.full > 0 {
		return false
	}
	lim := b.world.Self.InvLimit
	return lim <= 0 || !b.roomChecked() || n < int(lim)
}

// bagNear is how many free slots under its inventory limit make a bag full
// enough to sell its junk (startShop): room for a few kills' drops.
const bagNear = 5

// bagFull reports whether the bot holds its inventory limit of items, or is
// within bagNear of it; false with the limit unknown.
func (b *Brain) bagFull() bool {
	lim := int(b.world.Self.InvLimit)
	return lim > 0 && len(b.world.Inventory.Items) >= lim-bagNear
}

// lootRefused handles a SystemMessage id at now: a refused pickup for want
// of a slot (msgSlotsFull) gives the item clicked up at once, and pickups
// needing a slot wait until one frees (slotsFit).
func (b *Brain) lootRefused(id int32, now time.Time) {
	if id != msgSlotsFull {
		return
	}
	l := &b.loot
	l.full = max(len(b.world.Inventory.Items), 1)
	if !l.clicked.IsZero() {
		l.skip()
	}
}

// pickupLock is how long the server holds a picker after its pickup
// (pickupParalyzeLock, network/pickup.go lockPickupParalysis): a click it
// gets meanwhile it answers with ActionFailed and runs once the lock ends
// (deferOrFailPickup, network/targeting.go).
const pickupLock = 200 * time.Millisecond

// lootFailed handles an ActionFailed at now. The server answers every
// pickup click with one (network/pickup.go), but only a click within its
// pick-up range is answered after the pickup ran: a click from farther
// starts a walk, answered first, and the pickup runs at the item, and a
// click the pickup lock holds runs when the lock ends (pickupLock). So
// with a click on the head item in flight, the bot within pickupStand of
// the item and the click not sent within pickupLock of its last pickup,
// the item leaves the queue at once (#686): taken, a GetItem follows;
// refused (the item locked to another, claimed, an open trade, no slot),
// the bot no longer waits Loot.Timeout for it. It counts no miss
// (lootMisses): a refusal holds the loot rule no longer, and the item,
// out of the queue and the ground record, is clicked again only once it
// is seen anew.
func (b *Brain) lootFailed(now time.Time) {
	l := &b.loot
	if l.clicked.IsZero() || len(l.items) == 0 {
		return
	}
	if d := l.clicked.Sub(l.picked); d >= 0 && d < pickupLock {
		return
	}
	if it, ok := b.world.Ground[l.items[0]]; ok && !within3(b.world.Self.Position(now), it.Pos, pickupStand) {
		return
	}
	l.skip()
}

// groundPaused reports whether the bot leaves queued item id alone at now:
// a ground item, not its own kill's drop, while a loot stall pauses ground
// pickups (lootMisses).
func (l *lootQueue) groundPaused(id int32, now time.Time) bool {
	return !l.own[id] && now.Before(l.pauseUntil)
}

// missed records the head item, clicked and given up at now without a
// pickup: lootMisses ground items in a row pause ground pickups.
func (l *lootQueue) missed(id int32, now time.Time) {
	if l.own[id] {
		return
	}
	if l.misses++; l.misses >= lootMisses {
		l.misses, l.pauseUntil = 0, now.Add(lootPause)
	}
}

// jewelryCap is how many pieces jewelry slot s takes: one necklace, two
// earrings, two rings; 0 for a slot that is not jewelry.
func jewelryCap(s gearSlot) int {
	switch s {
	case slotNeck:
		return 1
	case slotEar, slotFinger:
		return 2
	}
	return 0
}

// bagFills reports whether the bot carries, unworn, as many pieces of the
// jewelry slot of template id that the gear errand would wear (upgrade) as
// the slot takes (jewelryCap): one more from the ground is surplus.
func (b *Brain) bagFills(id int32, w *worn) bool {
	g, ok := b.gearItem(id)
	if !ok {
		return false
	}
	s := slotOf(g.part)
	c := jewelryCap(s)
	if c == 0 {
		return false
	}
	n := 0
	for _, it := range b.world.Inventory.Items {
		if gi, ok := b.gearItem(it.TemplateID); ok && slotOf(gi.part) == s && b.upgrade(it, w) {
			n++
		}
	}
	return n >= c
}

// spareJewelry are the object ids of the carried jewelry pieces past what
// their slot takes: of the unworn pieces of a slot the gear errand would
// wear (upgrade), the best scoring jewelryCap are worn next and the rest
// are surplus, junk to sell.
func (b *Brain) spareJewelry(w *worn) map[int32]bool {
	type piece struct {
		id    int32
		score float64
	}
	var bySlot [slotCount][]piece
	for id, it := range b.world.Inventory.Items {
		g, ok := b.gearItem(it.TemplateID)
		if !ok || jewelryCap(slotOf(g.part)) == 0 || !b.upgrade(it, w) {
			continue
		}
		s := slotOf(g.part)
		bySlot[s] = append(bySlot[s], piece{id, b.pieceScore(w, &g, it)})
	}
	var out map[int32]bool
	for s, ps := range bySlot {
		c := jewelryCap(gearSlot(s))
		if len(ps) <= c {
			continue
		}
		slices.SortFunc(ps, func(x, y piece) int {
			return cmp.Or(cmp.Compare(y.score, x.score), cmp.Compare(x.id, y.id))
		})
		if out == nil {
			out = make(map[int32]bool)
		}
		for _, p := range ps[c:] {
			out[p.id] = true
		}
	}
	return out
}
