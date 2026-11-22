package botclient

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// skillCrystallize is Crystallize, level 1 for the Artisan at 20 and 2 for
// the Warsmith at 40 (network/dispatch.go crystallizeSkillID): the grade it
// reaches (model/item/crystal.go CanCrystallize: level 1 D, 2 C, 3 B, 4 A,
// 5 S).
const skillCrystallize int32 = 248

// The crystallize answers (serverpackets/systemmessage.go): the item named
// broke into crystals (SystemMessageItemName, then the crystals'
// ItemObtained), or the skill is below its grade or not known.
const (
	msgCrystallizeLevelTooLow = 562
	msgItemCrystallized       = 1258
)

// Crystallizing limits.
const (
	// crystallizeTimeout is how long a RequestCrystallizeItem waits for
	// its answer: the server answers at once, or never for an item it
	// cannot crystallize (network/inventory.go crystallizeLiveItem, a
	// missing, hero or shadow item, no slot for the crystals).
	crystallizeTimeout = 5 * time.Second
	// crystallizePerCycle bounds the gear one cycle withdraws to
	// crystallize, under the weight guard's room besides.
	crystallizePerCycle = 20
)

// heroItem reports whether the server never crystallizes template id
// (model/item/item.go Template.HeroItem).
func heroItem(id int32) bool { return id >= 6611 && id <= 6621 || id == 6842 }

// crystalYield is what crystallizing a +0 item of a template gives: count
// crystals of grade, the crystal template's.
type crystalYield struct {
	grade   int
	crystal int32
	count   int32
}

// crystalItem is the crystal template of grade g, 1 D to 5 S
// (model/item/crystal.go crystalTypeTable), 0 for none.
func crystalItem(g int) int32 {
	if g < 1 || g > 5 {
		return 0
	}
	return 1457 + int32(g)
}

// indexCrystals is every weapon and armor template the server crystallizes
// (model/item/crystal.go Template.Crystallizable: a grade and a crystal
// count) with its +0 yield, less the hero and shadow items it refuses.
func indexCrystals(items map[int32]datapack.Item) map[int32]crystalYield {
	out := make(map[int32]crystalYield)
	for id, it := range items {
		if it.Type == "EtcItem" || it.CrystalCount <= 0 || crystalItem(it.Grade) == 0 || it.Duration > 0 || heroItem(id) {
			continue
		}
		out[id] = crystalYield{grade: it.Grade, crystal: crystalItem(it.Grade), count: it.CrystalCount}
	}
	return out
}

// worthCrystallizing reports whether the crafter crystallizes a +0 item of
// template t with Crystallize at level (docs/crafting.md section 6): t's
// grade within the skill's, and its crystals worth more than the item by
// reference prices; an unpriced item or crystal never is.
func (m *TravelMap) worthCrystallizing(t, level int32) (crystalYield, bool) {
	y, ok := m.crystals[t]
	if !ok || int32(y.grade) > level {
		return crystalYield{}, false
	}
	price, crystal := m.prices[t], m.prices[y.crystal]
	return y, price > 0 && crystal > 0 && int64(y.count)*int64(crystal) > int64(price)
}

// crystallizeWant is the gear of the clan warehouse's contents the crafter
// withdraws to crystallize, by template: +0 unaugmented items worth it at
// level (worthCrystallizing), never the ledger's reserved counts (every
// member's items and the other crafters' reservations), which it takes
// from the +0 unaugmented items first since the ledger names no object, at
// most
// crystallizePerCycle of them and, when room is positive, no more weight
// than room.
func (m *TravelMap) crystallizeWant(contents []WarehouseItem, reserved map[int32]int64, level int32, room int64) map[int32]int32 {
	if level <= 0 {
		return nil
	}
	plain := make(map[int32]int64)
	for _, it := range contents {
		if it.Enchant == 0 && it.Augmentation == 0 {
			plain[it.TemplateID] += int64(it.Count)
		}
	}
	out := make(map[int32]int32)
	left, weight := int64(crystallizePerCycle), int64(0)
	for _, t := range slices.Sorted(maps.Keys(plain)) {
		if _, ok := m.worthCrystallizing(t, level); !ok {
			continue
		}
		n := min(plain[t]-reserved[t], left)
		if w := int64(m.weights[t]); room > 0 && w > 0 {
			n = min(n, (room-weight)/w)
		}
		if n <= 0 {
			continue
		}
		out[t] = int32(n)
		left -= n
		weight += n * int64(m.weights[t])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// encodeRequestCrystallizeItem crystallizes count of item object
// (clientpackets/item_ops.go: two int32, object id then count).
func encodeRequestCrystallizeItem(object, count int32) []byte {
	w := wire.NewPacketWriter(opGameRequestCrystallizeItem)
	w.WriteInt32(object)
	w.WriteInt32(count)
	return w.Bytes()
}

// crystallizeAction crystallizes the whole of item object. It passes the
// craft gate (gateCraft, craftGap).
func crystallizeAction(object int32) Action {
	return Action{kind: actCrystallize, payload: encodeRequestCrystallizeItem(object, 1), object: object}
}

// crystalStep is the crystallizing of a crafter cycle: the gear it
// withdraws to crystallize, then one RequestCrystallizeItem per item, each
// waiting for its answer or crystallizeTimeout. It is owned by the brain
// goroutine.
type crystalStep struct {
	// want is the gear the cycle withdraws to crystallize, by template;
	// own the object ids of what the bot carried before, which it never
	// crystallizes; tried the objects asked once, never asked again.
	want  map[int32]int32
	own   map[int32]bool
	tried map[int32]bool
	// pending is set from a request's send to its answer, sentAt when it
	// went out.
	pending bool
	sentAt  time.Time
	// stopped is set once the skill answered below an item's grade: the
	// cycle crystallizes no more.
	stopped bool
}

// planCrystallize picks the gear of the fresh warehouse list the cycle
// withdraws to crystallize (crystallizeWant), with the weight guard's room
// left, and notes what the bot carries now as its own.
func (b *Brain) planCrystallize() {
	cz, s := &b.craft.crystal, &b.world.Self
	*cz = crystalStep{}
	var room int64
	if s.MaxLoad > 0 {
		if room = int64(s.MaxLoad)*crafterWeightPercent/100 - int64(s.Load); room <= 0 {
			return
		}
	}
	reserved := b.cfg.CraftBoard.Reserved(b.world.Clan.ID, b.world.Self.ObjectID)
	cz.want = b.cfg.Travel.crystallizeWant(b.wh.status.Contents, reserved, b.world.Skills[skillCrystallize].Level, room)
	if cz.want == nil {
		return
	}
	cz.own = make(map[int32]bool, len(b.world.Inventory.Items))
	for _, it := range b.world.Inventory.Items {
		cz.own[it.ObjectID] = true
	}
}

// rows are the withdrawal rows of the gear to crystallize.
func (cz *crystalStep) rows() []WarehouseRow {
	rows := make([]WarehouseRow, 0, len(cz.want))
	for _, t := range slices.Sorted(maps.Keys(cz.want)) {
		rows = append(rows, WarehouseRow{TemplateID: t, Count: cz.want[t]})
	}
	return rows
}

// withdrawnGear are the carried items the cycle withdrew to crystallize:
// unequipped, of a wanted template, not the bot's own.
func (b *Brain) withdrawnGear() []Item {
	cz := &b.craft.crystal
	var out []Item
	for _, it := range b.world.Inventory.Items {
		if _, ok := cz.want[it.TemplateID]; ok && !it.Equipped && !cz.own[it.ObjectID] {
			out = append(out, it)
		}
	}
	slices.SortFunc(out, func(a, b Item) int { return cmp.Compare(a.ObjectID, b.ObjectID) })
	return out
}

// crystalTargets are the withdrawn items still to crystallize: +0,
// unaugmented, worth it at the skill's level and not asked yet; none once
// the skill answered too low.
func (b *Brain) crystalTargets() []Item {
	cz := &b.craft.crystal
	if cz.stopped || cz.want == nil {
		return nil
	}
	level := b.world.Skills[skillCrystallize].Level
	var out []Item
	for _, it := range b.withdrawnGear() {
		if _, ok := b.cfg.Travel.worthCrystallizing(it.TemplateID, level); ok && it.Enchant == 0 && it.Augmentation == 0 && !cz.tried[it.ObjectID] {
			out = append(out, it)
		}
	}
	return out
}

// crystalsDue moves the crystals the cycle's crystallizing will give into
// the planner's inventory inv: from the wanted gear, out of warehouse, while
// the look plans (fromWarehouse), else from the withdrawn items still to
// crystallize, out of inv.
func (b *Brain) crystalsDue(inv, warehouse map[int32]int64, fromWarehouse bool) {
	m, cz := b.cfg.Travel, &b.craft.crystal
	if fromWarehouse && b.craft.phase == craftLook {
		for t, n := range cz.want {
			y := m.crystals[t]
			inv[y.crystal] += int64(n) * int64(y.count)
			warehouse[t] = max(warehouse[t]-int64(n), 0)
		}
		return
	}
	for _, it := range b.crystalTargets() {
		y := m.crystals[it.TemplateID]
		inv[y.crystal] += int64(y.count)
		if inv[it.TemplateID] = max(inv[it.TemplateID]-int64(it.Count), 0); inv[it.TemplateID] == 0 {
			delete(inv, it.TemplateID)
		}
	}
}

// crystalLeftovers are the deposit rows of the withdrawn gear not
// crystallized (enchanted, refused or unanswered), by template, less the
// crafting stock's own rows (craftDepositRows).
func (b *Brain) crystalLeftovers() []WarehouseRow {
	counts := make(map[int32]int64)
	for _, it := range b.withdrawnGear() {
		if !b.cfg.Travel.craftItems[it.TemplateID] {
			counts[it.TemplateID] += int64(it.Count)
		}
	}
	rows := make([]WarehouseRow, 0, len(counts))
	for _, t := range slices.Sorted(maps.Keys(counts)) {
		rows = append(rows, WarehouseRow{TemplateID: t, Count: int32(min(counts[t], int64(1<<31-1)))})
	}
	return rows
}

// crystallizeNext is the cycle's next crystallizing, ahead of its crafts:
// after the answer to the last or its timeout, at least craftGap plus
// jitter after the last request. It reports false once nothing is left to
// crystallize. An unanswered request counts toward the timeout breaker
// (TimeoutStreak) and its item is never asked again; a re-plan follows the
// crystallizing when a request failed.
func (b *Brain) crystallizeNext(now time.Time) (Action, bool) {
	c := &b.craft
	cz := &c.crystal
	if cz.pending {
		if now.Before(cz.sentAt.Add(crystallizeTimeout)) {
			return Action{}, true
		}
		cz.pending, c.replan = false, true
		if c.timeouts++; c.timeouts >= b.persona.Crafter.TimeoutStreak {
			b.tripCrafter(now)
			return Action{}, true
		}
	}
	targets := b.crystalTargets()
	if len(targets) == 0 {
		return Action{}, false
	}
	if now.Before(c.nextAt) {
		return Action{}, true
	}
	return crystallizeAction(targets[0].ObjectID), true
}

// crystallizeSent records a RequestCrystallizeItem that went out at now.
func (b *Brain) crystallizeSent(a Action, now time.Time) {
	c := &b.craft
	cz := &c.crystal
	if cz.tried == nil {
		cz.tried = make(map[int32]bool)
	}
	cz.tried[a.object] = true
	cz.pending, cz.sentAt = true, now
	c.nextAt = now.Add(craftGap + time.Duration(b.rng.Int64N(int64(craftJitter)+1)))
}

// crystallizeEvent moves the crystallizing under way on with its answer,
// system message id: ItemCrystallized, or CrystallizeLevelTooLow, which
// stops the cycle's crystallizing and counts toward the failure breaker
// (FailStreak).
func (b *Brain) crystallizeEvent(id int32, now time.Time) {
	c := &b.craft
	cz := &c.crystal
	if !cz.pending {
		return
	}
	switch id {
	case msgItemCrystallized:
		cz.pending, c.timeouts, c.fails = false, 0, 0
	case msgCrystallizeLevelTooLow:
		cz.pending, cz.stopped, c.timeouts, c.replan = false, true, 0, true
		b.craftFailed(now)
	}
}
