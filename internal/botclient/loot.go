package botclient

import (
	"maps"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// Loot timing. lootRetry and dropRetry bound how often the bot repeats a
// pickup click or a drop request the server left unanswered; the frame
// bucket and gateDrop (pacing.go) bound them as well.
const (
	// lootRetry is how long after a pickup click the bot, no longer on its
	// way, clicks the item again. Every click is answered by ActionFailed
	// whatever its outcome (the server's targeting.go and pickup.go), so
	// only the item leaving the ground tells a pickup worked.
	lootRetry = 3 * time.Second
	// lootFoughtFor is how long after its last blow exchanged with the bot a
	// creature's drops count as the bot's loot.
	lootFoughtFor = time.Minute
	// dropRetry is how long the bot waits before asking again to drop a
	// stack it already asked to drop: a refused drop answers a message or
	// nothing (the server's config_gates.go and inventory.go).
	dropRetry = 30 * time.Second
)

// partyLootFindersKeepers is the party loot rule under which the picker
// keeps what it picks up (party.go on the server).
const partyLootFindersKeepers = 0

// LootPolicy is what the bot picks up after a fight and what it drops when
// it carries too much (rule 8).
type LootPolicy struct {
	// Pickup queues the items dropped by the creatures the bot fought that
	// land within Radius of it, and picks them up one at a time, oldest
	// first, once no attacker is left. It gives up on an item Timeout after
	// its first click: another player's loot stays locked to them for a
	// while (the server's killrewards.go). False picks up nothing.
	Pickup  bool
	Radius  int32
	Timeout time.Duration
	// LeaveFindersKeepers leaves the loot alone while the bot is a member,
	// not the leader, of a finders keepers party.
	LeaveFindersKeepers bool
	// DropAbove is the weight percent (SelfState.Load of MaxLoad) at or
	// over which the bot drops one stack of Junk at a time where it
	// stands; 0 drops nothing. Equipped items are never dropped.
	DropAbove int
	Junk      []int32
}

func defaultLootPolicy() LootPolicy {
	return LootPolicy{Pickup: true, Radius: 1000, Timeout: 15 * time.Second, LeaveFindersKeepers: true, DropAbove: 80}
}

// GroundItem is an item lying on the ground.
type GroundItem struct {
	ObjectID, TemplateID int32
	Pos                  Point
	Stackable            bool
	Count                int32
}

// SpawnItem reports a ground item coming into sight.
type SpawnItem struct{ Item GroundItem }

// DropItem reports an item dropped to the ground in sight: by DropperID, a
// killed creature or a player.
type DropItem struct {
	DropperID int32
	Item      GroundItem
}

// GetItem reports PickerID picking up ground item ObjectID at Pos.
type GetItem struct {
	PickerID, ObjectID int32
	Pos                Point
}

func (SpawnItem) event() {}
func (DropItem) event()  {}
func (GetItem) event()   {}

// readGroundItem reads the ground item rows of SpawnItem and DropItem:
// object id, template id, position, stackable flag and count.
func readGroundItem(r *wire.Reader) GroundItem {
	return GroundItem{ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(), Pos: readPoint(r), Stackable: r.ReadInt32() != 0, Count: r.ReadInt32()}
}

// decodeSpawnItem: the ground item, then a constant 0
// (serverpackets/grounditems.go FrameSpawnItem).
func decodeSpawnItem(r *wire.Reader) Event {
	e := SpawnItem{Item: readGroundItem(r)}
	r.ReadInt32()
	return e
}

// decodeDropItem: the dropper, the ground item, then a constant 1
// (serverpackets/grounditems.go FrameDropItem).
func decodeDropItem(r *wire.Reader) Event {
	e := DropItem{DropperID: r.ReadInt32()}
	e.Item = readGroundItem(r)
	r.ReadInt32()
	return e
}

// decodeGetItem: picker, ground item, position
// (serverpackets/grounditems.go FrameGetItem).
func decodeGetItem(r *wire.Reader) Event {
	return GetItem{PickerID: r.ReadInt32(), ObjectID: r.ReadInt32(), Pos: readPoint(r)}
}

// encodeRequestDropItem drops count of carried item objectID at p, which
// the server accepts within 150 of the character (the server's
// clientpackets/item_ops.go, inventory.go dropInRange).
func encodeRequestDropItem(objectID, count int32, p Point) []byte {
	w := wire.NewPacketWriter(opGameRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	writePoint(w, p)
	return w.Bytes()
}

// applyGround keeps the items on the ground in sight. A picked up item
// leaves with GetItem, and with the DeleteObject after it.
func (w *World) applyGround(ev Event) {
	if w.Ground == nil {
		w.Ground = make(map[int32]GroundItem)
	}
	switch e := ev.(type) {
	case SpawnItem:
		w.Ground[e.Item.ObjectID] = e.Item
	case DropItem:
		w.Ground[e.Item.ObjectID] = e.Item
	case GetItem:
		delete(w.Ground, e.ObjectID)
	}
}

// pickupAction clicks ground item id from origin, which picks it up, the
// server walking the character there first when it is out of reach.
func pickupAction(id int32, from Point) Action {
	return Action{kind: actPickup, payload: encodeAction(id, from), object: id}
}

// dropAction drops the whole stack it at p.
func dropAction(it Item, p Point) Action {
	return Action{kind: actDrop, payload: encodeRequestDropItem(it.ObjectID, it.Count, p), object: it.ObjectID}
}

// lootQueue is the bot's loot: the ground items it means to pick up and
// the creatures whose drops it takes. The brain goroutine owns it.
type lootQueue struct {
	// items are the queued ground item ids, oldest first.
	items []int32
	// started and clicked are the first and the last click on items[0],
	// zero before the first.
	started, clicked time.Time
	// fought is when the bot last exchanged a blow with each creature.
	fought map[int32]time.Time
	// dropped is when the bot last asked to drop each carried stack.
	dropped map[int32]time.Time
	// stand is the stand point the bot walks to before it picks up item
	// standFor (0 for none).
	stand    Point
	standFor int32
}

// next drops the head items that left the ground or ran out of time and
// returns the first one left, 0 for none.
func (l *lootQueue) next(ground map[int32]GroundItem, timeout time.Duration, now time.Time) int32 {
	for len(l.items) > 0 {
		id := l.items[0]
		if _, ok := ground[id]; ok && (l.started.IsZero() || now.Sub(l.started) < timeout) {
			return id
		}
		l.items = l.items[1:]
		l.started, l.clicked = time.Time{}, time.Time{}
	}
	return 0
}

// skip gives up the item at the head of the queue: no walk reaches it.
func (l *lootQueue) skip() {
	if len(l.items) > 0 {
		l.items = l.items[1:]
	}
	l.started, l.clicked, l.standFor = time.Time{}, time.Time{}, 0
}

// sent records a pickup or a drop that went out at now.
func (l *lootQueue) sent(a Action, now time.Time) {
	switch a.kind {
	case actPickup:
		if len(l.items) > 0 && l.items[0] == a.object {
			if l.started.IsZero() {
				l.started = now
			}
			l.clicked = now
		}
	case actDrop:
		if l.dropped == nil {
			l.dropped = make(map[int32]time.Time)
		}
		maps.DeleteFunc(l.dropped, func(_ int32, t time.Time) bool { return now.Sub(t) >= dropRetry })
		l.dropped[a.object] = now
	}
}

// lootEvent keeps the loot queue: the creatures the bot exchanges blows
// with, and the drops of those it fought. A queued drop arms the reaction
// delay so the pickup follows the kill.
func (b *Brain) lootEvent(ev Event, now time.Time) {
	self := b.world.Self.ObjectID
	switch e := ev.(type) {
	case Attack:
		for _, h := range e.Hits {
			switch self {
			case e.AttackerID:
				b.fought(h.TargetID, now)
			case h.TargetID:
				b.fought(e.AttackerID, now)
			}
		}
	case MagicSkillUse:
		switch self {
		case e.CasterID:
			b.fought(e.TargetID, now)
		case e.TargetID:
			b.fought(e.CasterID, now)
		}
	case DropItem:
		if !b.lootable(e, now) {
			return
		}
		// Items gone from the ground leave the queue, which bounds it by
		// what lies in sight; the head stays for next to retire with its
		// clicks.
		l := &b.loot
		if len(l.items) > 0 {
			head := l.items[0]
			l.items = slices.DeleteFunc(l.items, func(id int32) bool {
				_, ok := b.world.Ground[id]
				return !ok && id != head
			})
		}
		if !slices.Contains(l.items, e.Item.ObjectID) {
			l.items = append(l.items, e.Item.ObjectID)
		}
		if b.reactAt.IsZero() {
			b.reactAt = now.Add(b.reactDelay())
		}
	}
}

// fought records a blow between the bot and creature id at now. Only a
// creature the bot may fight counts: a heal or buff exchanged with a party
// member or a friendly player is no fight, so that player's dropped junk is
// never queued.
func (b *Brain) fought(id int32, now time.Time) {
	if id == 0 || id == b.world.Self.ObjectID {
		return
	}
	if _, mate := b.world.Party.Members[id]; mate {
		return
	}
	if o := b.world.object(id); o == nil || !b.persona.PvP.MayFight(o) {
		return
	}
	l := &b.loot
	if l.fought == nil {
		l.fought = make(map[int32]time.Time)
	}
	maps.DeleteFunc(l.fought, func(_ int32, t time.Time) bool { return now.Sub(t) > lootFoughtFor })
	l.fought[id] = now
}

// lootable reports whether the bot queues drop e: dropped by a creature it
// fought, within its loot radius, outside a finders keepers party it does
// not lead when the policy leaves those alone.
func (b *Brain) lootable(e DropItem, now time.Time) bool {
	p, w := &b.persona.Loot, b.world
	if !p.Pickup || e.DropperID == w.Self.ObjectID {
		return false
	}
	if t, ok := b.loot.fought[e.DropperID]; !ok || now.Sub(t) > lootFoughtFor {
		return false
	}
	if pt := &w.Party; p.LeaveFindersKeepers && pt.In() && pt.LeaderID != w.Self.ObjectID && pt.Loot == partyLootFindersKeepers {
		return false
	}
	r := int64(p.Radius)
	return dist2(w.Self.Position(now), e.Item.Pos) <= r*r
}

// lootQueued reports whether a queued item is still waiting at now.
func (b *Brain) lootQueued(now time.Time) bool {
	return b.loot.next(b.world.Ground, b.persona.Loot.Timeout, now) != 0
}

// ruleLoot is rule 8: with no attacker left, drop a stack of junk when
// overweight, else pick up the oldest queued item, standing up first. A
// click waits lootRetry, and the walk to the item the server starts, before
// the next; Timeout after the first, the bot gives the item up.
//
// It sends Action (frame bucket, lootRetry) and RequestDropItem (gateDrop,
// dropRetry per stack).
func (b *Brain) ruleLoot(now time.Time) (Action, bool) {
	if len(b.attackers(now)) > 0 || b.unfought(now) != 0 {
		return Action{}, false
	}
	if a, ok := b.dropJunk(now); ok {
		return a, true
	}
	id := b.loot.next(b.world.Ground, b.persona.Loot.Timeout, now)
	if id == 0 {
		return Action{}, false
	}
	s, l := &b.world.Self, &b.loot
	if s.Sitting {
		return b.toggle(now), true
	}
	if !l.clicked.IsZero() && (s.Moving(now) || now.Sub(l.clicked) < lootRetry) {
		return Action{}, true
	}
	from, at := s.Position(now), b.world.Ground[id].Pos
	w := b.walker()
	if w.pickUpOpen(from, at) {
		return pickupAction(id, from), true
	}
	// Closed from here: walk to a stand point the pick-up holds from first;
	// an item with none the bot reaches is given up.
	if l.standFor != id {
		st, ok := w.standPoint(from, at, pickupStand, nav.StandPickUp, 0)
		if !ok {
			l.skip()
			return Action{}, false
		}
		l.stand, l.standFor = st, id
	}
	a, st := b.goTo(l.stand, walkSlop, walkPickup, now)
	if st != walkWalking {
		// Unreachable, or there and the pick-up still closed (the item
		// lies off the stand point's floor): give it up.
		l.skip()
		return Action{}, false
	}
	return a, true
}

// pickupStand is how far from an item a pick-up stand point may lie:
// inside the server's 36-unit pick-up range (network/dispatch.go), so the
// pick-up from there needs no walk.
const pickupStand = 30

// dropJunk drops the junk stack with the lowest object id the bot carries,
// unequipped and not asked to drop within dropRetry, when its weight is at
// or over DropAbove and it stands still.
func (b *Brain) dropJunk(now time.Time) (Action, bool) {
	s, p := &b.world.Self, &b.persona.Loot
	if p.DropAbove <= 0 || s.MaxLoad <= 0 || below(s.Load, s.MaxLoad, p.DropAbove) || s.Moving(now) {
		return Action{}, false
	}
	var junk Item
	for _, it := range b.world.Inventory.Items {
		if it.Equipped || !slices.Contains(p.Junk, it.TemplateID) || (junk.ObjectID != 0 && it.ObjectID > junk.ObjectID) {
			continue
		}
		if t, ok := b.loot.dropped[it.ObjectID]; ok && now.Sub(t) < dropRetry {
			continue
		}
		junk = it
	}
	if junk.ObjectID == 0 {
		return Action{}, false
	}
	return dropAction(junk, s.Position(now)), true
}
