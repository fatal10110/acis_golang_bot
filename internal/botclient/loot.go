package botclient

import (
	"cmp"
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
	// lootGrace is how long after the bot first sees a ground item it did
	// not fight for it leaves the item alone (LootPolicy.Ground). The server
	// locks a kill's drops to the killer and their party for 15 s
	// (regularLootProtection, data/manager/killrewards.go) and announces
	// neither the owner nor the lift, so an item the bot did not see fall
	// may still be locked; a player who dropped an item has as long to take
	// it back. A raid boss's drops stay locked for 300 s: a click on one
	// costs the item's Timeout before the bot gives it up.
	lootGrace = 15 * time.Second
)

// GroundLoot is which ground items the bot did not fight for it picks up
// (LootPolicy.Ground).
type GroundLoot uint8

const (
	// GroundNone takes only the drops of the creatures the bot fought.
	GroundNone GroundLoot = iota
	// GroundFit takes the items the bot would keep rather than sell
	// (Brain.keepsItem: adena, its potions, shots and arrows, its supplies
	// and keep set, what the persona's keep hook holds, such as the clan
	// crafter's materials) and the gear that beats what it wears
	// (Brain.beats).
	GroundFit
	// GroundAny takes any item: the farmer's loot.
	GroundAny
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
	// StopAbove is the weight percent (SelfState.Load of MaxLoad) a pickup
	// never brings the bot to: a queued item that would is given up, so a
	// bot whose load a town visit cannot shed (SupplyPolicy.ShedMin) stops
	// looting instead of walking at half speed. An item that weighs nothing
	// (adena) is always picked up; with no item weights known, nothing is
	// at or over it. 0 sets no cap: the server's pickup never checks weight
	// (network/pickup.go), and past 100 % the bot cannot move.
	StopAbove int
	// DropAbove is the weight percent at or over which the bot drops one
	// stack of Junk at a time where it stands; 0 drops nothing. Equipped
	// items are never dropped.
	DropAbove int
	Junk      []int32
	// Ground queues, besides the drops of the creatures the bot fought, the
	// other ground items in Radius the bot picks up (#435), lootGrace after
	// it first sees them: none, those that fit the bot, or any. Its own
	// drops and Junk are never taken.
	Ground GroundLoot
}

// defaultLootPolicy keeps the load under the server's half-speed band at
// 66.6 % (character_weight_penalty.go): no pickup takes it to 65 %, and
// Junk is dropped from there.
func defaultLootPolicy() LootPolicy {
	return LootPolicy{
		Pickup: true, Radius: 1000, Timeout: 15 * time.Second, LeaveFindersKeepers: true,
		StopAbove: 65, DropAbove: 65, Ground: GroundFit,
	}
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
	// ground are the ground items in sight the bot did not fight for, not
	// queued yet, and when each may be (lootGrace).
	ground map[int32]time.Time
	// claimed is the item the bot holds a GroundClaims claim on, 0 for none.
	claimed int32
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
	case SpawnItem:
		b.groundSeen(e.Item.ObjectID, now)
	case GetItem:
		delete(b.loot.ground, e.ObjectID)
		b.cfg.Ground.drop(e.ObjectID)
	case DeleteObject:
		// The record forgets what World.Ground forgets (World.Apply), so it
		// stays bounded by what lies in sight, also for a member that
		// leaves the loot alone and never scans it (groundScan).
		delete(b.loot.ground, e.ObjectID)
	case TeleportToLocation:
		if e.ObjectID == self && !e.Fast {
			clear(b.loot.ground)
		}
	case DropItem:
		if !b.lootable(e, now) {
			if e.DropperID != b.world.Self.ObjectID {
				b.groundSeen(e.Item.ObjectID, now)
			}
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
	if o := b.world.object(id); o == nil || b.fightKind(id, o, now) == fightNone {
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
	if b.leavesLoot() {
		return false
	}
	r := int64(p.Radius)
	return dist2(w.Self.Position(now), e.Item.Pos) <= r*r
}

// leavesLoot reports whether the policy leaves the loot alone: the bot is
// a member, not the leader, of a finders keepers party.
func (b *Brain) leavesLoot() bool {
	pt := &b.world.Party
	return b.persona.Loot.LeaveFindersKeepers && pt.In() && pt.LeaderID != b.world.Self.ObjectID && pt.Loot == partyLootFindersKeepers
}

// groundSeen records ground item id, which the bot did not fight for, in
// sight at now: lootGrace on, groundScan may queue it. Items leave the
// record as they leave World.Ground (lootEvent), which bounds it by what
// lies in sight at a constant cost per sighting (#495).
func (b *Brain) groundSeen(id int32, now time.Time) {
	if p := &b.persona.Loot; !p.Pickup || p.Ground == GroundNone {
		return
	}
	l := &b.loot
	if l.ground == nil {
		l.ground = make(map[int32]time.Time)
	}
	if _, ok := l.ground[id]; !ok {
		l.ground[id] = now.Add(lootGrace)
	}
}

// groundScan queues the recorded ground items (groundSeen) whose grace is
// over and that lie within the loot radius, those the policy takes
// (groundTakes) in the order their grace ended; the others leave the
// record. One out of the radius stays recorded while it is in sight.
func (b *Brain) groundScan(now time.Time) {
	p, w, l := &b.persona.Loot, b.world, &b.loot
	if len(l.ground) == 0 || !p.Pickup || p.Ground == GroundNone || b.leavesLoot() {
		return
	}
	var due []int32
	from, r := w.Self.Position(now), int64(p.Radius)
	for id, at := range l.ground {
		it, ok := w.Ground[id]
		switch {
		case !ok:
			delete(l.ground, id)
		case now.Before(at) || dist2(from, it.Pos) > r*r:
		default:
			due = append(due, id)
		}
	}
	slices.SortFunc(due, func(x, y int32) int {
		if c := l.ground[x].Compare(l.ground[y]); c != 0 {
			return c
		}
		return cmp.Compare(x, y)
	})
	for _, id := range due {
		delete(l.ground, id)
		if !slices.Contains(l.items, id) && b.groundTakes(w.Ground[id]) {
			l.items = append(l.items, id)
		}
	}
}

// groundTakes reports whether the policy takes ground item it, which the
// bot did not fight for (LootPolicy.Ground): never its Junk.
func (b *Brain) groundTakes(it GroundItem) bool {
	p := &b.persona.Loot
	if slices.Contains(p.Junk, it.TemplateID) {
		return false
	}
	switch p.Ground {
	case GroundAny:
		return true
	case GroundFit:
		if b.keepsItem(Item{TemplateID: it.TemplateID, Count: it.Count}) {
			return true
		}
		w := b.worn()
		return b.beats(it.TemplateID, &w)
	}
	return false
}

// lootQueued reports whether a queued item is still waiting at now.
func (b *Brain) lootQueued(now time.Time) bool {
	return b.nextLoot(now) != 0
}

// nextLoot is the queued item to pick up at now, 0 for none: the head of
// the queue, after the ground items due (groundScan), giving up the ones
// too heavy to carry (lootFits) and those another bot of the process is
// after (GroundClaims). The bot claims the item it returns and gives back
// the one it held before.
func (b *Brain) nextLoot(now time.Time) int32 {
	b.groundScan(now)
	l, self := &b.loot, b.world.Self.ObjectID
	for {
		id := l.next(b.world.Ground, b.persona.Loot.Timeout, now)
		if id != 0 && (!b.lootFits(b.world.Ground[id]) || !b.cfg.Ground.claim(id, self, now)) {
			l.skip()
			continue
		}
		if l.claimed != id {
			b.cfg.Ground.release(l.claimed, self)
			l.claimed = id
		}
		return id
	}
}

// lootFits reports whether picking it up keeps the load under
// Loot.StopAbove percent of the weight limit and, whatever the persona
// sets, under the limit itself: the server's pickup never checks weight
// (network/pickup.go) and from the limit the bot cannot move (#488). Its
// weight is its count times its template's (TravelMap.weights); with the
// weights known, adena and an item that weighs nothing always fit, and one
// the table lacks weighs an unknown amount and never does. With no weights known,
// it counts as nothing and the load alone must be under the caps.
func (b *Brain) lootFits(it GroundItem) bool {
	s, pct := &b.world.Self, b.persona.Loot.StopAbove
	if s.MaxLoad <= 0 {
		return true
	}
	var w int64
	if t := b.cfg.Travel; t != nil && len(t.weights) > 0 {
		unit, ok := t.weights[it.TemplateID]
		if !ok && it.TemplateID != adenaID {
			return false
		}
		if w = int64(unit) * int64(max(it.Count, 1)); w == 0 {
			return true
		}
	}
	load := int64(s.Load) + w
	return load < int64(s.MaxLoad) && (pct <= 0 || load*100 < int64(s.MaxLoad)*int64(pct))
}

// ruleLoot is rule 8: with no attacker left, drop a stack of junk when
// overweight, else pick up the oldest queued item that fits under
// Loot.StopAbove, standing up first. A
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
	id := b.nextLoot(now)
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
