package botclient

import (
	"maps"
	"math"
	"slices"
	"time"
)

// scrollOfEscape is the Scroll of Escape, an etc item of type SCROLL that
// grocers sell (aCis_datapack/data/xml/items/0700-0799.xml, buyLists.xml).
const scrollOfEscape int32 = 736

// The buff potions of the default build table (GearClass.BuffPotion), which
// grocers sell: Potion of Alacrity (skill 2012, attack_time_down) for
// fighters and Magic Haste Potion (skill 2169, casting_time_down) for
// mystics (aCis_datapack/data/xml/items/0700-0799.xml, 6000-6099.xml).
const (
	alacrityPotion   int32 = 735
	magicHastePotion int32 = 6035
)

// The use meter's decay and first estimate.
const (
	// meterHalfLife is how many minutes of fighting halve the weight of
	// what the meter saw before them.
	meterHalfLife = 30.0
	// meterPrior is how many minutes of fighting the first estimate weighs
	// before any fight; it decays with them like any other.
	meterPrior = 5.0
	// attackSpeedBase is the attack speed at which the server runs a cast
	// unscaled (formulas.AttackSpeedBase, skill/formulas/speed.go); a
	// weapon template with no attack speed is taken at it.
	attackSpeedBase = 333
	// castsPerMinute is the first estimate of a mystic's casts per minute
	// of fighting: one 4 s nuke at the base casting speed (formulas.AtkSpd)
	// after another.
	castsPerMinute = 15.0
)

// SupplyPolicy sizes the consumables the bot carries for a trip from what
// it spends (docs/items.md section 4). Every value is a configurable
// default (D6).
type SupplyPolicy struct {
	// TripMinutes is how many minutes of fighting one trip's supplies last:
	// a consumable's trip target is its use rate times TripMinutes, plus
	// Margin percent. 0 sizes nothing.
	TripMinutes int
	Margin      int
	// BuyLoad is the percent of the weight limit the supplies never fill
	// the bot past, below the server's first weight-penalty band at 50 %
	// (character_weight_penalty.go). 0 sets no weight cap.
	BuyLoad int
	// Refill is the percent of a trip target under which the shop errand
	// buys the consumable up to its target.
	Refill int
	// PotionsPerHour is the first estimate of the HP potions drunk per hour
	// of fighting, before the bot drank any.
	PotionsPerHour float64
	// Escapes is how many Scrolls of Escape the bot carries.
	Escapes int32
	// The supply trip home (rule "supply", docs/items.md section 7) starts
	// when a consumable runs out within ReturnMinutes of fighting (0 never),
	// or the load passes ReturnLoad percent of the weight limit (0 never)
	// with a town visit shedding at least ShedMin percent of it. It reads
	// its Scroll of Escape only once nothing attacks it; past SafeWait
	// without such a moment, it walks.
	ReturnMinutes int
	ReturnLoad    int
	ShedMin       int
	SafeWait      time.Duration
	// Extra adds supply targets (template id to count) after the use
	// meter's, clamped by weight like the rest; nil adds none. The clan
	// crafter plan (M9) asks for ore with it. They count toward neither the
	// preparation gate nor the run-out trigger.
	Extra func(w *World) map[int32]int32
	// InTown is the "trip in town" hook, called once per supply trip
	// arrival in town, and HandOff the weight it will hand off there,
	// which a town visit sheds toward ReturnLoad (shed); nil for none.
	// The clan crafter plan (M9) fills them to deposit materials and
	// collect products. The brain goroutine calls both.
	InTown  func(w *World)
	HandOff func(w *World) int64
}

// defaultSupplyPolicy carries an hour of fighting's shots and HP potions,
// a fifth more, and two Scrolls of Escape, within 45 % of the weight limit.
// It goes home with ten minutes of fighting left, or past 70 % of the
// weight limit (well above the 45 %, so a bot that bought to the cap does
// not turn round at its first loot) with a tenth of it to sell, waiting up
// to two minutes for a safe moment to read its scroll.
func defaultSupplyPolicy() SupplyPolicy {
	return SupplyPolicy{
		TripMinutes: 60, Margin: 20, BuyLoad: 45, Refill: 25, PotionsPerHour: 10, Escapes: 2,
		ReturnMinutes: 10, ReturnLoad: 70, ShedMin: 10, SafeWait: 2 * time.Minute,
	}
}

// useMeter is what the bot spends of each consumable per minute of
// fighting: a decayed average over the minutes it spent in combat. The brain
// goroutine owns it; time comes from the events' injected clock.
type useMeter struct {
	// at is the last event seen, fighting whether the bot was in combat
	// since then.
	at       time.Time
	fighting bool
	// fought are the minutes of fighting so far, minutes the same decayed,
	// and used the decayed count spent of each template.
	fought, minutes float64
	used            map[int32]float64
}

// accrue adds the fighting between the last event and now.
func (m *useMeter) accrue(now time.Time) {
	if m.fighting && !m.at.IsZero() && now.After(m.at) {
		dt := now.Sub(m.at).Minutes()
		f := math.Exp2(-dt / meterHalfLife)
		m.fought += dt
		m.minutes = m.minutes*f + dt
		for id, n := range m.used {
			m.used[id] = n * f
		}
	}
	m.at = now
}

// spend counts n of template id spent.
func (m *useMeter) spend(id, n int32) {
	if m.used == nil {
		m.used = make(map[int32]float64)
	}
	m.used[id] += float64(n)
}

// rate is what the bot spends per minute of fighting of templates ids
// together, est per minute standing in for the fighting not yet seen.
func (m *useMeter) rate(ids []int32, est float64) float64 {
	prior := meterPrior * math.Exp2(-m.fought/meterHalfLife)
	var used float64
	for _, id := range ids {
		used += m.used[id]
	}
	return (used + est*prior) / (m.minutes + prior)
}

// meterEvent feeds the use meter with ev at now, before the world applies
// it: the fighting since the last event, and the shots and potions an
// InventoryUpdate in combat takes from the stacks the bot carries. Once the
// world applied ev, the brain sets fighting to the combat state that holds
// until the next event.
func (b *Brain) meterEvent(ev Event, now time.Time) {
	m := &b.meter
	m.accrue(now)
	u, ok := ev.(InventoryUpdate)
	if !ok || !m.fighting {
		return
	}
	for _, c := range u.Changes {
		prev, had := b.world.Inventory.Items[c.Item.ObjectID]
		if !had || !b.metered(prev.TemplateID) {
			continue
		}
		switch c.State {
		case ItemModified:
			if c.Item.Count < prev.Count {
				m.spend(prev.TemplateID, prev.Count-c.Item.Count)
			}
		case ItemRemoved:
			m.spend(prev.TemplateID, prev.Count)
		}
	}
}

// metered reports whether the use meter counts template id: a shot or a
// persona potion.
func (b *Brain) metered(id int32) bool {
	_, shot := b.shotTable()[id]
	return shot || slices.Contains(b.persona.Potions, id)
}

// firstShotRate is the first estimate of the shots weapon w spends per
// minute of fighting: its soulshots per attack at the attacks per minute
// its attack speed gives (formulas.TimeBetweenAttacks), or a mystic's
// spiritshots per cast at castsPerMinute.
func (b *Brain) firstShotRate(w weapon) float64 {
	if slices.Contains(mysticClasses, b.world.Self.ClassID) {
		return float64(w.spiritshots) * castsPerMinute
	}
	spd := w.atkSpd
	if spd <= 0 {
		spd = attackSpeedBase
	}
	return float64(w.soulshots) * 60000 / float64(max(500000/spd, 100))
}

// tripCount is a trip's worth of rate per minute of fighting, margin
// included.
func (b *Brain) tripCount(rate float64) int32 {
	s := &b.persona.Supply
	return int32(math.Ceil(rate * float64(s.TripMinutes) * float64(100+s.Margin) / 100))
}

// shotNeed is a trip's worth of the shots weapon w charges.
func (b *Brain) shotNeed(w weapon) int32 {
	return b.tripCount(b.meter.rate(b.shotsFor(w), b.firstShotRate(w)))
}

// supplyPotion is the HP potion the bot carries for a trip: the best of
// Potions a merchant the shop errand can visit sells (soldNear), else the
// best one a spawned merchant sells; 0 for none. A newbie village grocer
// sells only the Lesser Healing Potion while town merchants sell the
// Healing Potion too.
func (b *Brain) supplyPotion() int32 {
	if b.cfg.Travel == nil {
		return 0
	}
	for _, id := range b.persona.Potions {
		if b.soldNear(id) {
			return id
		}
	}
	for _, id := range b.persona.Potions {
		if b.cfg.Travel.spawnedSold[id] {
			return id
		}
	}
	return 0
}

// soldNear reports whether a merchant the shop errand can visit sells
// item (startShop): one in sight, or one spawned within Shop.Reach of
// where the bot stands or heads.
func (b *Brain) soldNear(item int32) bool {
	shops := b.cfg.Travel.shops
	sells := func(npcID int32) bool {
		return slices.ContainsFunc(shops[npcID], func(l shopList) bool { _, ok := l.sells(item); return ok })
	}
	for _, o := range b.world.Objects {
		if o.Kind == KindNPC && !o.Dead && sells(o.NPCID) {
			return true
		}
	}
	from, reach := b.world.Self.move.to, int64(b.persona.Shop.Reach)*int64(b.persona.Shop.Reach)
	for npcID := range shops {
		if slices.ContainsFunc(b.cfg.Travel.spawns[npcID], func(at Point) bool { return dist2(from, at) <= reach }) && sells(npcID) {
			return true
		}
	}
	return false
}

// supply is a consumable the bot carries for a trip and its trip target.
// An extra one comes from SupplyPolicy.Extra.
type supply struct {
	item, count int32
	extra       bool
}

// supplies are the trip targets in priority order: the shots of each
// weapon of shotWeapons (buyShot), the HP potion (supplyPotion), the
// Scrolls of Escape, then the Extra ones by template id. A Restock row
// sets its item's to its Max, and ShotStock its Max for shots. Each is
// clamped by the weight left under BuyLoad once the ones before it are
// carried (weightCap).
func (b *Brain) supplies() []supply {
	var out []supply
	add := func(id, n int32, extra bool) {
		if id == 0 || slices.ContainsFunc(out, func(s supply) bool { return s.item == id }) {
			return
		}
		if r, ok := b.restockOf(id); ok {
			n = r.Max
		}
		out = append(out, supply{item: id, count: max(n, 0), extra: extra})
	}
	st := b.shotStock()
	for _, w := range b.shotWeapons() {
		n := st.Max
		if n == 0 {
			n = b.shotNeed(w)
		}
		add(b.buyShot(w), n, false)
	}
	if id := b.supplyPotion(); id != 0 {
		add(id, b.tripCount(b.meter.rate(b.persona.Potions, b.persona.Supply.PotionsPerHour/60)), false)
	}
	if b.persona.Supply.Escapes > 0 {
		add(scrollOfEscape, b.persona.Supply.Escapes, false)
	}
	if b.persona.Supply.Extra != nil {
		extra := b.persona.Supply.Extra(b.world)
		for _, id := range slices.Sorted(maps.Keys(extra)) {
			add(id, extra[id], true)
		}
	}
	room, capped := b.weightCap(out)
	if !capped {
		return out
	}
	for i := range out {
		wt := b.cfg.Travel.weights[out[i].item]
		if wt <= 0 {
			continue
		}
		out[i].count = int32(min(int64(out[i].count), max(room, 0)/int64(wt)))
		room -= int64(out[i].count) * int64(wt)
	}
	return out
}

// weightCap is the weight the supplies may take: BuyLoad percent of the
// weight limit, less the load of everything else carried. capped is false
// while the weight limit or the item weights are unknown.
func (b *Brain) weightCap(supplies []supply) (room int64, capped bool) {
	s, self := &b.persona.Supply, &b.world.Self
	if s.BuyLoad <= 0 || self.MaxLoad <= 0 || b.cfg.Travel == nil || len(b.cfg.Travel.weights) == 0 {
		return 0, false
	}
	room = int64(self.MaxLoad)*int64(s.BuyLoad)/100 - int64(self.Load)
	for _, sp := range supplies {
		room += int64(b.carried(sp.item)) * int64(b.cfg.Travel.weights[sp.item])
	}
	return room, true
}

// tripTarget is how many of template id the bot carries for a trip
// (supplies), 0 for one it carries none of. The clan crafter plan (M9)
// sizes its orders with it.
func (b *Brain) tripTarget(id int32) int32 {
	for _, s := range b.supplies() {
		if s.item == id {
			return s.count
		}
	}
	return 0
}

// restockOf is the Restock row of the bot's class for template id, the
// explicit override of its trip target.
func (b *Brain) restockOf(id int32) (Restock, bool) {
	for _, r := range b.persona.Shop.Restock {
		if r.Item == id && r.For.fits(b.world.Self.ClassID) {
			return r, true
		}
	}
	return Restock{}, false
}

// supplyWants are the HP potion and Scrolls of Escape the shop errand
// buys (refillRow). Shots are shotWants', and the Extra supplies
// extraWants', bought after the bot's own.
func (b *Brain) supplyWants() []buyRow {
	var rows []buyRow
	potion := b.supplyPotion()
	for _, s := range b.supplies() {
		if s.extra || s.item != scrollOfEscape && s.item != potion {
			continue
		}
		if r, ok := b.refillRow(s); ok {
			rows = append(rows, r)
		}
	}
	return rows
}

// refillRow is the purchase of supply s from a spawned merchant: up to its
// trip target once fewer than Refill percent of it are carried. ok is false
// for one a Restock row buys instead.
func (b *Brain) refillRow(s supply) (buyRow, bool) {
	if _, ok := b.restockOf(s.item); ok {
		return buyRow{}, false
	}
	if b.cfg.Travel != nil && !b.cfg.Travel.spawnedSold[s.item] {
		return buyRow{}, false
	}
	if have := b.carried(s.item); b.refills(have, s.count) {
		return buyRow{item: s.item, count: s.count - have}, true
	}
	return buyRow{}, false
}

// refills reports whether have carried of a trip target is low enough to
// buy up to it: under Refill percent of it, or under it at all while the
// town preparation gate holds (townPrep).
func (b *Brain) refills(have, target int32) bool {
	return target > have && (b.prep.armed || int64(have)*100 < int64(target)*int64(b.persona.Supply.Refill))
}
