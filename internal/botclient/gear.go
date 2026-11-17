package botclient

import (
	"cmp"
	"slices"
	"strings"
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

// GearPolicy is how the bot wears and buys better weapons, shields, armor
// and jewelry (rule 9 errand "gear", and the shop errand). Every value is a
// configurable default (D6).
type GearPolicy struct {
	// Equip wears a carried piece that beats the one worn in its slot
	// (gearSlot): of the worn piece's weapon type and hands or armor type,
	// of a grade the bot wears without penalty, of a higher reference
	// price, and not over an enchanted piece. An empty slot takes what the
	// bot's class wears (Classes). The piece it replaces is junk the shop
	// errand sells.
	Equip bool
	// Jewelry wears a necklace, two earrings and two rings the same way.
	// With both of a pair worn, the bot takes the lesser one off before it
	// wears the new one: the server would replace the left piece, and the
	// bot cannot tell the sides apart.
	Jewelry bool
	// Classes are what each class wears where no worn piece gives a type
	// to match: the first entry for the bot's class applies, and a class
	// with none wears nothing new in an empty weapon, body and legs slot,
	// and no shield.
	Classes []GearClass
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

// defaultGearPolicy wears and buys gear and jewelry up to the bot's
// expertise, keeping 10000 adena.
func defaultGearPolicy() GearPolicy {
	return GearPolicy{Equip: true, Jewelry: true, Classes: defaultGearClasses(), Buy: true, MaxGrade: 5, KeepAdena: 10000}
}

// GearClass is what classes wear where no worn piece gives a type to
// match (GearPolicy.Classes). The server equips any weapon or armor type
// on any class: this is a preference.
type GearClass struct {
	// Classes are the class ids of the entry; none for every class.
	Classes []int32
	// Weapons are the weapon types the classes wield (the item files'
	// weapon_type, with "/MAGIC" for a magical weapon). An unarmed bot
	// wears the best of them, and a one-handed type and its two-handed
	// one ("SWORD" and "BIGSWORD", "BLUNT" and "BIGBLUNT") replace each
	// other when both are listed: a two-handed weapon is worth more than
	// the one-handed weapon and the shield it takes off, a one-handed one
	// more than the two-handed weapon less the best shield the bot carries.
	Weapons []string
	// Armor is the armor type (LIGHT, HEAVY or MAGIC) of the body and
	// legs pieces a bot wearing neither wears.
	Armor string
	// Shield wears a shield with a one-handed weapon or none.
	Shield bool
}

// defaultGearClasses is the class table of the default persona: mystics
// wield magical one-handed weapons and staves in robes; dagger users
// daggers and archers bows (the shop errand restocks their arrows,
// ShopPolicy.ArrowsMin) in light armor; monks fists in light armor; gladiators and bladedancers
// duals, warlords polearms and destroyers two-handed weapons, in heavy
// armor; the first fighter classes swords or blunts in light armor (their
// starting set) with a shield; every other fighter swords, blunts or
// their two-handed kinds in heavy armor with a shield (classes/*.xml,
// model/actor/player/race.go).
func defaultGearClasses() []GearClass {
	return []GearClass{
		{Classes: slices.Clone(mysticClasses), Weapons: []string{"BLUNT/MAGIC", "SWORD/MAGIC", "BIGBLUNT/MAGIC"}, Armor: "MAGIC"},
		{Classes: []int32{7, 8, 22, 23, 35, 36, 93, 101, 108}, Weapons: []string{"DAGGER"}, Armor: "LIGHT"},
		{Classes: []int32{9, 24, 37, 92, 102, 109}, Weapons: []string{"BOW"}, Armor: "LIGHT"},
		{Classes: []int32{47, 48, 114}, Weapons: []string{"DUALFIST"}, Armor: "LIGHT"},
		{Classes: []int32{2, 34, 88, 107}, Weapons: []string{"DUAL"}, Armor: "HEAVY"},
		{Classes: []int32{3, 89}, Weapons: []string{"POLE"}, Armor: "HEAVY"},
		{Classes: []int32{46, 113}, Weapons: []string{"BIGSWORD", "BIGBLUNT"}, Armor: "HEAVY"},
		{Classes: []int32{0, 18, 31, 44, 53}, Weapons: []string{"SWORD", "BLUNT"}, Armor: "LIGHT", Shield: true},
		{Weapons: []string{"SWORD", "BLUNT", "BIGSWORD", "BIGBLUNT"}, Armor: "HEAVY", Shield: true},
	}
}

// gearClass is the bot's GearPolicy.Classes entry; the zero GearClass
// when none is.
func (b *Brain) gearClass() GearClass {
	for _, c := range b.persona.Gear.Classes {
		if len(c.Classes) == 0 || slices.Contains(c.Classes, b.world.Self.ClassID) {
			return c
		}
	}
	return GearClass{}
}

// gearSlot is a paperdoll slot the bot keeps gear in. Accessories (hair,
// face, cloak, underwear) are left alone.
type gearSlot uint8

const (
	slotNone gearSlot = iota
	slotWeapon
	slotShield
	// slotBody holds a chest piece or full armor; full armor covers the
	// legs too.
	slotBody
	slotLegs
	slotHead
	slotGloves
	slotFeet
	slotNeck
	// slotEar and slotFinger hold a pair: two earrings, two rings.
	slotEar
	slotFinger
	slotCount
)

// Body part masks of the gear slots: a template's slot, as ItemList and
// InventoryUpdate carry it (model/item/item.go Slot).
const (
	partREar      int32 = 0x0002
	partLEar      int32 = 0x0004
	partLREar     int32 = 0x0006
	partNeck      int32 = 0x0008
	partRFinger   int32 = 0x0010
	partLFinger   int32 = 0x0020
	partLRFinger  int32 = 0x0030
	partHead      int32 = 0x0040
	partRHand     int32 = 0x0080
	partLHand     int32 = 0x0100
	partGloves    int32 = 0x0200
	partChest     int32 = 0x0400
	partLegs      int32 = 0x0800
	partFeet      int32 = 0x1000
	partLRHand    int32 = 0x4000
	partFullArmor int32 = 0x8000
)

// bodyParts maps the item files' bodypart to its mask: an earring or ring
// is of either side, and ItemList and InventoryUpdate report that mask for
// a worn one too, not its side.
var bodyParts = map[string]int32{
	"head": partHead, "rhand": partRHand, "gloves": partGloves, "chest": partChest,
	"legs": partLegs, "feet": partFeet, "lrhand": partLRHand, "fullarmor": partFullArmor,
	"lhand": partLHand, "neck": partNeck, "rear;lear": partLREar, "rfinger;lfinger": partLRFinger,
}

// slotOf is the gear slot of body part mask part.
func slotOf(part int32) gearSlot {
	switch part {
	case partRHand, partLRHand:
		return slotWeapon
	case partLHand:
		return slotShield
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
	case partNeck:
		return slotNeck
	case partREar, partLEar, partLREar:
		return slotEar
	case partRFinger, partLFinger, partLRFinger:
		return slotFinger
	}
	return slotNone
}

// gearItem is a weapon or armor template the bot may wear.
type gearItem struct {
	part int32
	// kind is a weapon's type, with "/MAGIC" for a magical one (a
	// mystic's weapon never replaces a fighter's, nor the other way), and a
	// body or legs piece's armor type; "" for a shield, a head, gloves or
	// feet piece, or jewelry, which fits any.
	kind  string
	grade int32
	price int32
}

// indexGear indexes the templates the bot may wear: a weapon of a wielded
// type, or an armor piece (a shield or jewelry too) of a gear slot, with a reference price, no use
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
			if it.Magical {
				g.kind += "/MAGIC"
			}
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
	// obj is the item the last gear UseItem puts on, or takes off when
	// room is set, waited for until until.
	obj   int32
	until time.Time
	// room is the earring or ring template a take-off frees a side for.
	room int32
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
// slot. Of the earrings and rings it is the piece of a full pair a new one
// replaces (paired), and zero while a side is free.
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
	var w, first worn
	for _, it := range b.world.Inventory.Items {
		if !it.Equipped {
			continue
		}
		switch s := slotOf(it.BodySlot); s {
		case slotNone:
		case slotEar, slotFinger:
			if first[s].ObjectID == 0 {
				first[s] = it
			} else {
				w[s] = b.paired(first[s], it)
			}
		default:
			w[s] = it
		}
	}
	return w
}

// paired is the piece of the worn pair x, y a new one replaces: the
// cheaper of the ones the gear errand may replace (known, not enchanted).
func (b *Brain) paired(x, y Item) Item {
	gx, okx := b.gearItem(x.TemplateID)
	gy, oky := b.gearItem(y.TemplateID)
	okx, oky = okx && x.Enchant == 0, oky && y.Enchant == 0
	if c := cmp.Or(-cmpBool(okx, oky), cmp.Compare(gx.price, gy.price), cmp.Compare(x.ObjectID, y.ObjectID)); c > 0 {
		return y
	}
	return x
}

// cmpBool orders false before true.
func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// gearGrade is the highest grade the bot wears: its Expertise level,
// capped by GearPolicy.MaxGrade.
func (b *Brain) gearGrade() int32 {
	return min(b.world.Skills[expertiseSkill].Level, b.persona.Gear.MaxGrade)
}

// beats reports whether wearing template id beats what the bot wears, w
// (GearPolicy.Equip): an empty weapon slot, or empty body and legs slots,
// give no type to match and take what the bot's class wears
// (GearPolicy.Classes).
func (b *Brain) beats(id int32, w *worn) bool {
	g, ok := b.gearItem(id)
	if !ok || b.gear.refused[id] || g.grade > b.gearGrade() {
		return false
	}
	class := b.gearClass()
	s := slotOf(g.part)
	switch s {
	case slotLegs:
		if w[slotBody].BodySlot == partFullArmor {
			return false
		}
	case slotShield:
		// A shield takes a two-handed weapon off
		// (itemcontainer/inventory.go equipItemLocked).
		if !class.Shield || w[slotWeapon].BodySlot == partLRHand {
			return false
		}
	case slotNeck, slotEar, slotFinger:
		if !b.persona.Gear.Jewelry {
			return false
		}
	}
	cur := w[s]
	if cur.ObjectID == 0 {
		switch s {
		case slotWeapon:
			return slices.Contains(class.Weapons, g.kind)
		case slotBody, slotLegs:
			other := w[slotLegs]
			if s == slotLegs {
				other = w[slotBody]
			}
			if other.ObjectID == 0 {
				return g.kind == class.Armor
			}
			o, ok := b.gearItem(other.TemplateID)
			return ok && o.kind == g.kind
		}
		return true
	}
	c, ok := b.gearItem(cur.TemplateID)
	if !ok || cur.Enchant > 0 {
		return false
	}
	gain, value := int64(g.price), int64(c.price)
	switch {
	case s == slotWeapon && c.part != g.part:
		// A two-handed weapon for a one-handed one and its shield, or the
		// other way round.
		if !class.swaps(c.kind, g.kind) {
			return false
		}
		// Both directions value the one-handed side with the best shield
		// it would hold, worn or carried, so the trade-off never flips.
		if g.part == partLRHand {
			var shield int64
			if sh := w[slotShield]; sh.ObjectID != 0 {
				worn, ok := b.gearItem(sh.TemplateID)
				if !ok || sh.Enchant > 0 {
					return false
				}
				shield = int64(worn.price)
			}
			if class.Shield {
				shield = max(shield, b.carriedShield())
			}
			value += shield
		} else if class.Shield {
			gain += b.carriedShield()
		}
	case c.kind != g.kind:
		return false
	case g.part == partFullArmor && c.part == partChest && w[slotLegs].ObjectID != 0:
		// Full armor replaces the legs too.
		legs, ok := b.gearItem(w[slotLegs].TemplateID)
		if !ok || w[slotLegs].Enchant > 0 {
			return false
		}
		value += int64(legs.price)
	}
	return gain > value
}

// swaps reports whether weapon types a and b are a one-handed type and its
// two-handed one, both of which the class wields.
func (c *GearClass) swaps(a, b string) bool {
	return a != b && strings.TrimPrefix(a, "BIG") == strings.TrimPrefix(b, "BIG") &&
		slices.Contains(c.Weapons, a) && slices.Contains(c.Weapons, b)
}

// carriedShield is the reference price of the best shield the bot carries
// and would wear with a one-handed weapon; 0 for none.
func (b *Brain) carriedShield() int64 {
	var best int64
	for _, it := range b.world.Inventory.Items {
		g, ok := b.gearItem(it.TemplateID)
		if ok && g.part == partLHand && !it.Equipped && it.Augmentation == 0 && !b.gear.refused[it.TemplateID] && g.grade <= b.gearGrade() {
			best = max(best, int64(g.price))
		}
	}
	return best
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

// ruleGear is a rule 9 errand: standing, out of combat and trade, the bot uses the
// best carried piece that beats the one worn in its slot (UseItem equips
// it), the first slot first. For an earring or ring with both sides worn
// it first uses the piece it replaces (UseItem takes it off,
// network/inventory.go toggleEquipItem), and wears the new one in the
// freed side next. One UseItem is waited for at a time, up to
// gearTimeout; a template the server leaves unequipped, or does not make
// room for, is refused for good, so each template is used at most once
// more than it is worn. The frame pacer bounds the rest.
func (b *Brain) ruleGear(now time.Time) (Action, bool) {
	g, self := &b.gear, &b.world.Self
	// Never while moving: a weapon used during a cast's approach is
	// toggled again on arrival (use_item_intention.go), leaving the bot
	// unarmed.
	if !b.persona.Gear.Equip || b.cfg.Travel == nil || self.Dead || self.InCombat || self.Moving(now) || b.world.Trade.Open() {
		return Action{}, false
	}
	if g.obj != 0 {
		it, ok := b.world.Inventory.Items[g.obj]
		switch {
		case !ok || it.Equipped == (g.room == 0):
			g.obj = 0
		case now.Before(g.until):
			return Action{}, false
		default:
			if g.refused == nil {
				g.refused = make(map[int32]bool)
			}
			g.refused[cmp.Or(g.room, it.TemplateID)] = true
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
	g.room = 0
	if s := slotOf(bestG.part); (s == slotEar || s == slotFinger) && w[s].ObjectID != 0 {
		// The server would replace the left piece of the pair
		// (itemcontainer/inventory.go equipPaired): take off the lesser.
		g.room, best = best.TemplateID, w[s]
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
