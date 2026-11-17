package botclient

import (
	"cmp"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// expertiseSkill is the Expertise skill: its level is the highest crystal
// grade the character wears without the expertise penalty
// (model/skill/tree.go ExpertiseSkillID, network/inventory.go
// RefreshExpertisePenalty). The server equips gear of any grade.
const expertiseSkill int32 = 239

// gearTimeout is how long the bot waits for an item it used to show up
// equipped (an InventoryUpdate). The server defers the use of a weapon
// while the bot casts (network/use_item_intention.go), and answers a
// refusal with a system message or ActionFailed only.
const gearTimeout = 5 * time.Second

// GearPolicy is how the bot wears and buys better weapons and armor
// (rule 9 errand "gear", and the shop errand). Every value is a
// configurable default (D6).
type GearPolicy struct {
	// Equip wears a carried weapon or armor piece that beats the one worn
	// in its slot (gearSlot): of the worn piece's weapon type and hands or
	// armor type, of a grade the bot wears without penalty, of a higher
	// reference price, and not over an enchanted piece. The piece it
	// replaces is junk the shop errand sells.
	Equip bool
	// Buy adds, per slot, every such piece a merchant sells and the
	// adena past KeepAdena buys to the shop errand's wants, best first:
	// the visit buys the best one its merchant sells, one per slot. It
	// needs Equip.
	Buy bool
	// MaxGrade caps the crystal grade bought and worn: 0 for no grade,
	// then 1 for D up to 5 for S. The Expertise skill's level caps it too.
	MaxGrade int32
	// KeepAdena is the adena a gear purchase never spends; potions, shots
	// and teleports need it.
	KeepAdena int32
}

// defaultGearPolicy wears and buys gear up to the bot's expertise,
// keeping 10000 adena.
func defaultGearPolicy() GearPolicy {
	return GearPolicy{Equip: true, Buy: true, MaxGrade: 5, KeepAdena: 10000}
}

// gearSlot is a paperdoll slot the bot keeps gear in. Shields, jewelry
// and accessories are left alone.
type gearSlot uint8

const (
	slotNone gearSlot = iota
	slotWeapon
	// slotBody holds a chest piece or full armor; full armor covers the
	// legs too.
	slotBody
	slotLegs
	slotHead
	slotGloves
	slotFeet
	slotCount
)

// Body part masks of the gear slots: a template's slot, as ItemList and
// InventoryUpdate carry it (model/item/item.go Slot).
const (
	partHead      int32 = 0x0040
	partRHand     int32 = 0x0080
	partGloves    int32 = 0x0200
	partChest     int32 = 0x0400
	partLegs      int32 = 0x0800
	partFeet      int32 = 0x1000
	partLRHand    int32 = 0x4000
	partFullArmor int32 = 0x8000
)

// bodyParts maps the item files' bodypart to its mask.
var bodyParts = map[string]int32{
	"head": partHead, "rhand": partRHand, "gloves": partGloves, "chest": partChest,
	"legs": partLegs, "feet": partFeet, "lrhand": partLRHand, "fullarmor": partFullArmor,
}

// slotOf is the gear slot of body part mask part.
func slotOf(part int32) gearSlot {
	switch part {
	case partRHand, partLRHand:
		return slotWeapon
	case partChest, partFullArmor:
		return slotBody
	case partLegs:
		return slotLegs
	case partHead:
		return slotHead
	case partGloves:
		return slotGloves
	case partFeet:
		return slotFeet
	}
	return slotNone
}

// gearItem is a weapon or armor template the bot may wear.
type gearItem struct {
	part int32
	// kind is a weapon's type and a body or legs piece's armor type; ""
	// for a head, gloves or feet piece, which fits any.
	kind  string
	grade int32
	price int32
}

// indexGear indexes the templates the bot may wear: a weapon of a wielded
// type, or an armor piece of a gear slot, with a reference price, no use
// or equip condition and no mana to run out. shopGear are the ones in
// cheapest, the items merchants sell, by slot then price, best first.
func indexGear(items map[int32]datapack.Item, cheapest map[int32]int32) (map[int32]gearItem, []int32) {
	gear := make(map[int32]gearItem)
	for id, it := range items {
		part, ok := bodyParts[it.BodyPart]
		if !ok || it.Price <= 0 || it.Conditional || it.Duration > 0 {
			continue
		}
		g := gearItem{part: part, grade: int32(it.Grade), price: it.Price}
		switch slotOf(part) {
		case slotWeapon:
			if it.Type != "Weapon" || slices.Contains([]string{"", "NONE", "ETC", "PET", "FISHINGROD"}, it.Kind) {
				continue
			}
			g.kind = it.Kind
		case slotBody, slotLegs:
			if it.Type != "Armor" || !slices.Contains([]string{"LIGHT", "HEAVY", "MAGIC"}, it.Kind) {
				continue
			}
			g.kind = it.Kind
		default:
			if it.Type != "Armor" {
				continue
			}
		}
		gear[id] = g
	}
	var sold []int32
	for id := range cheapest {
		if _, ok := gear[id]; ok {
			sold = append(sold, id)
		}
	}
	slices.SortFunc(sold, func(a, b int32) int {
		ga, gb := gear[a], gear[b]
		return cmp.Or(cmp.Compare(slotOf(ga.part), slotOf(gb.part)), cmp.Compare(gb.price, ga.price), cmp.Compare(a, b))
	})
	return gear, sold
}

// gearState is the gear errand's state, owned by the brain goroutine.
type gearState struct {
	// obj is the item the last gear UseItem equips, waited for until
	// until.
	obj   int32
	until time.Time
	// refused are the templates the server left unequipped past
	// gearTimeout: never worn, bought or kept again.
	refused map[int32]bool
}

// sent records a gear UseItem that went out at now.
func (g *gearState) sent(a Action, now time.Time) {
	if a.kind == actUseItem && a.object != 0 {
		g.obj, g.until = a.object, now.Add(gearTimeout)
	}
}

// worn is what the bot wears in each gear slot; a zero Item is an empty
// slot.
type worn [slotCount]Item

// gearItem is the wearable template id, if the travel map knows it.
func (b *Brain) gearItem(id int32) (gearItem, bool) {
	if b.cfg.Travel == nil {
		return gearItem{}, false
	}
	g, ok := b.cfg.Travel.gear[id]
	return g, ok
}

// worn is what the bot wears now.
func (b *Brain) worn() worn {
	var w worn
	for _, it := range b.world.Inventory.Items {
		if it.Equipped {
			if s := slotOf(it.BodySlot); s != slotNone {
				w[s] = it
			}
		}
	}
	return w
}

// gearGrade is the highest grade the bot wears: its Expertise level,
// capped by GearPolicy.MaxGrade.
func (b *Brain) gearGrade() int32 {
	return min(b.world.Skills[expertiseSkill].Level, b.persona.Gear.MaxGrade)
}

// beats reports whether wearing template id beats what the bot wears, w
// (GearPolicy.Equip): an empty weapon slot, or empty body and legs slots,
// give no type to match and take nothing.
func (b *Brain) beats(id int32, w *worn) bool {
	g, ok := b.gearItem(id)
	if !ok || b.gear.refused[id] || g.grade > b.gearGrade() {
		return false
	}
	s := slotOf(g.part)
	if s == slotLegs && w[slotBody].BodySlot == partFullArmor {
		return false
	}
	cur := w[s]
	if cur.ObjectID == 0 {
		switch s {
		case slotWeapon:
			return false
		case slotBody, slotLegs:
			other := w[slotLegs]
			if s == slotLegs {
				other = w[slotBody]
			}
			o, ok := b.gearItem(other.TemplateID)
			return other.ObjectID != 0 && ok && o.kind == g.kind
		}
		return true
	}
	c, ok := b.gearItem(cur.TemplateID)
	if !ok || cur.Enchant > 0 || c.kind != g.kind || (s == slotWeapon && c.part != g.part) {
		return false
	}
	value := int64(c.price)
	if g.part == partFullArmor && c.part == partChest && w[slotLegs].ObjectID != 0 {
		// Full armor replaces the legs too.
		legs, ok := b.gearItem(w[slotLegs].TemplateID)
		if !ok || w[slotLegs].Enchant > 0 {
			return false
		}
		value += int64(legs.price)
	}
	return int64(g.price) > value
}

// upgrade reports whether carried item it is one the gear errand wears
// over w: it is never junk.
func (b *Brain) upgrade(it Item, w *worn) bool {
	p := &b.persona.Gear
	return p.Equip && !it.Equipped && it.Augmentation == 0 && b.beats(it.TemplateID, w)
}

// wearsNext reports whether the gear errand wears carried item it next.
func (b *Brain) wearsNext(it Item) bool {
	if _, ok := b.gearItem(it.TemplateID); !ok || it.Equipped {
		return false
	}
	w := b.worn()
	return b.upgrade(it, &w)
}

// ruleGear is a rule 9 errand: out of combat and trade, the bot uses the
// best carried piece that beats the one worn in its slot (UseItem equips
// it), the first slot first. One UseItem is waited for at a time, up to
// gearTimeout; a template the server leaves unequipped is refused for
// good, so each template is used at most once more than it is worn. The
// frame pacer bounds the rest.
func (b *Brain) ruleGear(now time.Time) (Action, bool) {
	g, self := &b.gear, &b.world.Self
	if !b.persona.Gear.Equip || b.cfg.Travel == nil || self.Dead || self.InCombat || b.world.Trade.Open() {
		return Action{}, false
	}
	if g.obj != 0 {
		it, ok := b.world.Inventory.Items[g.obj]
		switch {
		case !ok || it.Equipped:
			g.obj = 0
		case now.Before(g.until):
			return Action{}, false
		default:
			if g.refused == nil {
				g.refused = make(map[int32]bool)
			}
			g.refused[it.TemplateID] = true
			g.obj = 0
		}
	}
	w := b.worn()
	var best Item
	var bestG gearItem
	for _, it := range b.world.Inventory.Items {
		if !b.upgrade(it, &w) {
			continue
		}
		gi, _ := b.gearItem(it.TemplateID)
		if best.ObjectID == 0 || cmp.Or(cmp.Compare(slotOf(gi.part), slotOf(bestG.part)), cmp.Compare(bestG.price, gi.price), cmp.Compare(it.ObjectID, best.ObjectID)) < 0 {
			best, bestG = it, gi
		}
	}
	if best.ObjectID == 0 {
		return Action{}, false
	}
	a := useItemAction(best)
	a.object = best.ObjectID
	return a, true
}

// gearWants are the pieces the shop errand buys (GearPolicy.Buy), one of
// each: per slot without a carried upgrade, every piece merchants sell
// that beats the worn one and that the adena past GearPolicy.KeepAdena
// buys at its cheapest base price, best first.
func (b *Brain) gearWants() []buyRow {
	p := &b.persona.Gear
	if !p.Buy || !p.Equip || b.cfg.Travel == nil {
		return nil
	}
	budget := int64(b.carried(adenaID)) - int64(p.KeepAdena)
	if budget <= 0 {
		return nil
	}
	w := b.worn()
	var held [slotCount]bool
	for _, it := range b.world.Inventory.Items {
		if b.upgrade(it, &w) {
			g, _ := b.gearItem(it.TemplateID)
			held[slotOf(g.part)] = true
		}
	}
	var rows []buyRow
	for _, id := range b.cfg.Travel.shopGear {
		g := b.cfg.Travel.gear[id]
		price := b.cfg.Travel.cheapest[id]
		if held[slotOf(g.part)] || price <= 0 || int64(price) > budget || !b.beats(id, &w) {
			continue
		}
		rows = append(rows, buyRow{item: id, count: 1})
	}
	return rows
}

// gearReserve is how much more adena a gear purchase keeps than the shop
// errand's own purchases.
func (b *Brain) gearReserve() int64 {
	return max(0, int64(b.persona.Gear.KeepAdena)-int64(b.persona.Shop.KeepAdena))
}
