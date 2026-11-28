package botclient

import (
	"cmp"
	"math"
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

// expertiseLevels are the levels at which a character gets each Expertise
// level, D (1) to S (5), for free and on its own: skill 239 at minLvl 20,
// 40, 52, 61 and 76 in every class file (aCis_datapack/data/xml/classes,
// e.g. humanFighter.xml; model/actor/player/template.go
// AutoGetSkillGrants).
var expertiseLevels = [...]int32{20, 40, 52, 61, 76}

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
	// of a grade the bot wears without penalty, and of a higher score, its
	// stats and enchant weighed by the class's build (GearClass.Weights),
	// the worn piece's enchant counted too. An empty slot takes what the
	// bot's class wears (Classes), and so does a worn weapon of a type the
	// class does not wield (GearClass.Weapons). The piece it replaces is
	// junk the shop errand sells once the new one is worn. A bow waits
	// until the bot carries arrows of its grade (ShopPolicy.ArrowsMin).
	Equip bool
	// Jewelry wears a necklace, two earrings and two rings the same way.
	// With both of a pair worn, the bot takes the lesser one off before it
	// wears the new one: the server would replace the left piece, and the
	// bot cannot tell the sides apart.
	Jewelry bool
	// Classes are what each class wears where no worn piece gives a type
	// to match: the first entry for the bot's class applies, and a class
	// with none wears nothing new in an empty weapon, body and legs slot,
	// and no shield. nil plays the build's entry (Persona, overrides).
	Classes []GearClass
	// Buy adds, per slot, every such piece a merchant sells and the
	// adena past KeepAdena buys to the shop errand's gear tier, best first:
	// the visit buys the best one its merchant sells, one per slot. It
	// needs Equip.
	Buy bool
	// MaxGrade caps the crystal grade bought and worn: 0 for no grade,
	// then 1 for D up to 5 for S. The Expertise skill's level caps it too.
	MaxGrade int32
	// KeepAdena is the reserve floor: the adena the gear and comfort tiers
	// of a shop visit's budget never spend, so the next trips' potions,
	// shots and teleports are paid.
	KeepAdena int32
	// SaveFrom is the percent of the gear goal's price (gearGoal) from
	// which the adena past KeepAdena counts as within reach: a visit that
	// does not buy the goal then saves for it and skips the comfort tier.
	// 0 never saves.
	SaveFrom int32
	// SaveBefore is the grade jump's window: within this many levels
	// below the level of the next Expertise grade the bot may wear
	// (expertiseLevels, MaxGrade), its shop visits save for that grade's
	// core pieces (jumpGoal), and buy a piece of the grade it wears only
	// when the piece is worth it (SaveWorth). 0 never saves for a jump.
	SaveBefore int32
	// SaveWorth is what a piece of the bot's grade must be worth to be
	// bought in the grade jump's window, the gear and set pieces it would
	// throw away at the jump: its score gain over the pieces it replaces,
	// as a fraction of their score, at least SaveWorth times its price as
	// a fraction of the jump goal's (worthBefore). An empty slot's piece is
	// always worth it. 0 buys every one.
	SaveWorth float64
	// ShotFactor multiplies a weapon's score where Equip and Buy compare it
	// with the wielded one, when the bot can keep it charged and not the
	// other: a shot source has shots of its grade for the class (a spawned
	// merchant), or the bot carries ShopPolicy's restock minimum of them,
	// any while it wields a weapon firing the same shots (shotsBacked). So a
	// no-grade weapon with shots beats a D-grade one without them until the
	// D one's score wins on its own. 1 or less compares scores alone.
	ShotFactor float64
	// SetBonus values a complete armor set (armorSets.xml) at its pieces'
	// score times one plus this fraction, the set shield's too while it is
	// worn: the bot reads no set skill's stats. Equip, Buy and the gear
	// goal compare armor by combination: a set the bot carries, or could
	// buy, whole beats the pieces it would replace when its value does,
	// the best mix of single pieces for its slots and a worn set's bonus
	// counted (setPlan); a piece that breaks a worn set must beat the
	// set's bonus too. 0 ignores sets.
	SetBonus float64
}

// defaultGearPolicy wears and buys gear and jewelry up to the bot's
// expertise, keeping 10000 adena and saving for the next upgrade from half
// its price, with a shot factor of 2: a soulshot
// doubles a physical hit's damage (skill/formulas/damage.go), a blessed
// spiritshot four times the M. Atk a spell's damage takes the root of. A
// complete armor set is worth 15 % more than its pieces. Two levels before
// a grade jump it saves for the next grade, buying a piece of its grade
// only when its score gain is at least the share of the jump goal's price
// it costs (docs/items.md section 6).
func defaultGearPolicy() GearPolicy {
	return GearPolicy{Equip: true, Jewelry: true, Buy: true, MaxGrade: 5, KeepAdena: 10000, SaveFrom: 50, SaveBefore: 2, SaveWorth: 1, ShotFactor: 2, SetBonus: 0.15}
}

// GearClass is a build: what classes wear where no worn piece gives a type
// to match, and what they weigh gear by (GearPolicy.Classes). The server
// equips any weapon or armor type on any class: this is a preference.
type GearClass struct {
	// Classes are the class ids of the entry; none for every class.
	Classes []int32
	// Weapons are the weapon types the classes wield (the item files'
	// weapon_type, with "/MAGIC" for a magical weapon). An unarmed bot
	// wears the best of them, and a one-handed type and its two-handed
	// one ("SWORD" and "BIGSWORD", "BLUNT" and "BIGBLUNT") replace each
	// other when both are listed: a two-handed weapon is worth more than
	// the one-handed weapon and the shield it takes off, a one-handed one
	// more than the two-handed weapon less the best shield the bot carries,
	// by score. A worn weapon of a type not listed (an archer's dagger from
	// its dagger days) gives way to any listed one, whatever the scores and
	// its enchant, the best the bot carries or buys first, unless a
	// two-handed one would take an unknown shield off; nothing unlisted
	// replaces it, and nothing unlisted replaces a listed one, so the bot
	// never swaps back. A class listing none keeps whatever it wields.
	Weapons []string
	// Armor is the armor type (LIGHT, HEAVY or MAGIC) of the body and
	// legs pieces a bot wearing neither wears.
	Armor string
	// Shield wears a shield with a one-handed weapon or none.
	Shield bool
	// Weights are what the classes score gear by; the zero value is
	// evenWeights.
	Weights StatWeights
	// BuffPotion is the buff potion template the classes drink at their
	// spot (rule "buffpotion") and buy in the budget's comfort tier; 0 for
	// none. It is in the keep set.
	BuffPotion int32
}

// StatWeights weigh a piece's stats into its score: the sum of each stat,
// its enchant gain included (datapack.Item.EnchantGain), times its weight.
// A weapon scores its attack, armor its P. Def, jewelry its M. Def and a
// shield its shield defense, so the weights put a weapon and a shield on
// one scale only where a two-handed weapon trades against a one-handed
// one and its shield. The datapack reads no MP stat: a healer weighs
// M. Atk as a caster does.
type StatWeights struct {
	PAtk, MAtk, PDef, MDef float64
	// PAtkSpd is how much a weapon's attack speed counts: its P. Atk is
	// multiplied by its pAtkSpd over a sword's (379) to this power, so 1
	// scores the damage a weapon deals in a given time, and 0 ignores speed.
	// Two weapons of a type share their speed (a bow 293, a two-handed
	// sword 325, a dagger 433); it tells a two-handed weapon from a
	// one-handed one, and the odd slow bow from the others.
	PAtkSpd float64
	// SDef weighs a shield's defense, which counts only on the hits the
	// shield blocks (rShld 20 for every shield) and then only cuts the hit
	// it is added to the P. Def of.
	SDef float64
}

// Builds of the default class table: fighters weigh their damage, P. Atk
// and attack speed, casters (and healers) M. Atk, and both defense alike.
// evenWeights scores a class entry without weights, every attack and
// defense stat alike.
var (
	fighterWeights = StatWeights{PAtk: 1, PAtkSpd: 1, PDef: 1, MDef: 1, SDef: 0.05}
	casterWeights  = StatWeights{MAtk: 1, PDef: 1, MDef: 1, SDef: 0.05}
	evenWeights    = StatWeights{PAtk: 1, MAtk: 1, PDef: 1, MDef: 1, SDef: 0.05}
)

// swordAtkSpd is a sword's attack speed, the one PAtkSpd scales from.
const swordAtkSpd = 379

// weights are the class's stat weights: evenWeights when it sets none.
func (c *GearClass) weights() StatWeights {
	if c.Weights == (StatWeights{}) {
		return evenWeights
	}
	return c.Weights
}

// score is the score of gear template it at enchant level enchant.
func (w StatWeights) score(it *datapack.Item, enchant int) float64 {
	stat := func(name string, base float64) float64 { return base + float64(it.EnchantGain(name, enchant)) }
	atk := stat("pAtk", it.Stats.PAtk)
	if spd := it.Stats.PAtkSpd; w.PAtkSpd != 0 && spd > 0 {
		atk *= math.Pow(spd/swordAtkSpd, w.PAtkSpd)
	}
	return w.PAtk*atk + w.MAtk*stat("mAtk", it.Stats.MAtk) + w.PDef*stat("pDef", it.Stats.PDef) +
		w.MDef*stat("mDef", it.Stats.MDef) + w.SDef*stat("sDef", it.Stats.SDef)
}

// defaultGearClasses is the class table builds compile (CompileBuild): mystics
// wield magical one-handed weapons and staves in robes, the warcryer (52)
// magical blunts alone (Hammer Crush and Stun Attack take BLUNT); dagger users
// daggers and archers bows (the shop errand restocks their arrows,
// ShopPolicy.ArrowsMin) in light armor; monks fists in light armor; gladiators and bladedancers
// duals, warlords polearms and destroyers two-handed blunts, then swords
// (Hammer Crush takes only the blunt), in heavy armor; the warrior its
// lines' two-handed blunts and polearms in heavy armor without a shield
// (CompileBuild keeps its line's one, lineWeapons); the first fighter
// classes swords or blunts in light armor (their starting set) with a
// shield; the paladin (5) swords or blunts, the temple knight (20), sword
// singer (21) and shillien knight (33) swords, the bounty hunter (55)
// blunts, each with a shield in heavy armor (docs/strategy.md, "Gear and
// range", column 1); every other fighter swords, blunts or their
// two-handed kinds in heavy armor with a shield (classes/*.xml,
// model/actor/player/race.go). Mystics weigh gear as casters, every other
// class as fighters; mystics drink the Magic Haste Potion, every other
// class the Potion of Alacrity. Where two pieces score alike the type
// listed first wins (GearClass.weaponRank).
func defaultGearClasses() []GearClass {
	return []GearClass{
		{Classes: []int32{52}, Weapons: []string{"BLUNT/MAGIC"}, Armor: "MAGIC", Weights: casterWeights, BuffPotion: magicHastePotion},
		{Classes: slices.DeleteFunc(slices.Clone(mysticClasses), func(c int32) bool { return c == 52 }), Weapons: []string{"BLUNT/MAGIC", "SWORD/MAGIC", "BIGBLUNT/MAGIC"}, Armor: "MAGIC", Weights: casterWeights, BuffPotion: magicHastePotion},
		{Classes: []int32{7, 8, 22, 23, 35, 36, 93, 101, 108}, Weapons: []string{"DAGGER"}, Armor: "LIGHT", Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{9, 24, 37, 92, 102, 109}, Weapons: []string{"BOW"}, Armor: "LIGHT", Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{47, 48, 114}, Weapons: []string{"DUALFIST"}, Armor: "LIGHT", Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{2, 34, 88, 107}, Weapons: []string{"DUAL"}, Armor: "HEAVY", Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{3, 89}, Weapons: []string{"POLE"}, Armor: "HEAVY", Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{46, 113}, Weapons: []string{"BIGBLUNT", "BIGSWORD"}, Armor: "HEAVY", Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{1}, Weapons: []string{"BIGBLUNT", "POLE"}, Armor: "HEAVY", Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{0, 18, 31, 44, 53}, Weapons: []string{"SWORD", "BLUNT"}, Armor: "LIGHT", Shield: true, Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{5, 6}, Weapons: []string{"SWORD", "BLUNT"}, Armor: "HEAVY", Shield: true, Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{20, 21, 33}, Weapons: []string{"SWORD"}, Armor: "HEAVY", Shield: true, Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Classes: []int32{55}, Weapons: []string{"BLUNT"}, Armor: "HEAVY", Shield: true, Weights: fighterWeights, BuffPotion: alacrityPotion},
		{Weapons: []string{"SWORD", "BLUNT", "BIGSWORD", "BIGBLUNT"}, Armor: "HEAVY", Shield: true, Weights: fighterWeights, BuffPotion: alacrityPotion},
	}
}

// gearClass is the build's gear table entry; the zero GearClass when the
// table has none for the bot's class.
func (b *Brain) gearClass() GearClass { return b.build().Kit.Gear }

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
	// tmpl is the template, whose stats and enchant gain score it
	// (Brain.pieceScore).
	tmpl datapack.Item
}

// indexGear indexes the templates the bot may wear: a weapon of a wielded
// type, or an armor piece (a shield or jewelry too) of a gear slot, with a
// reference price, no use or equip condition and no mana to run out.
// shopGear are the ones in cheapest, the items merchants sell, by slot
// then price; gearCandidates ranks them by the bot's scores.
func indexGear(items map[int32]datapack.Item, cheapest map[int32]int32) (map[int32]gearItem, []int32) {
	gear := make(map[int32]gearItem)
	for id, it := range items {
		part, ok := bodyParts[it.BodyPart]
		if !ok || it.Price <= 0 || it.Conditional || it.Duration > 0 {
			continue
		}
		g := gearItem{part: part, grade: int32(it.Grade), tmpl: it}
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
		return cmp.Or(cmp.Compare(slotOf(ga.part), slotOf(gb.part)), cmp.Compare(gb.tmpl.Price, ga.tmpl.Price), cmp.Compare(a, b))
	})
	return gear, sold
}

// gearState is the gear errand's state, owned by the brain goroutine.
type gearState struct {
	// obj is the item the last gear UseItem puts on, or takes off when
	// room is set, waited for until until.
	obj   int32
	until time.Time
	// room is the earring or ring template a take-off frees a side for,
	// or the template taken off for the grade penalty (overGrade).
	room int32
	// refused are the templates the server left unequipped past
	// gearTimeout: never worn, bought or kept again.
	refused map[int32]bool
	// ahead plans one grade above the bot's (gearGrade) while jumpGoal
	// runs, and is false otherwise.
	ahead bool
	// plans counts the gear plans computed (planGear), the expensive part
	// of a decision; the tests read it to keep plans off the hot paths
	// that do not need one (#493).
	plans int
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
	var raw worn
	rawOK := false
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
				if !rawOK {
					raw, rawOK = b.rawWorn(), true
				}
				w[s] = b.paired(first[s], it, &raw)
			}
		default:
			w[s] = it
		}
	}
	return w
}

// paired is the piece of the worn pair x, y a new one replaces: the
// lower scoring of the ones the gear errand may replace (known), enchant
// counted, each scored worn in ctx.
func (b *Brain) paired(x, y Item, ctx *worn) Item {
	gx, okx := b.gearItem(x.TemplateID)
	gy, oky := b.gearItem(y.TemplateID)
	var sx, sy float64
	if okx && oky {
		sx, sy = b.pieceScore(ctx, &gx, x), b.pieceScore(ctx, &gy, y)
	}
	if c := cmp.Or(-cmpBool(okx, oky), cmp.Compare(sx, sy), cmp.Compare(x.ObjectID, y.ObjectID)); c > 0 {
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
// capped by GearPolicy.MaxGrade; one more while jumpGoal plans ahead.
func (b *Brain) gearGrade() int32 {
	g := b.world.Skills[expertiseSkill].Level
	if b.gear.ahead {
		g++
	}
	return min(g, b.persona.Gear.MaxGrade)
}

// beats reports whether wearing template id beats what the bot wears, w
// (GearPolicy.Equip): an empty weapon slot, or empty body and legs slots,
// give no type to match and take what the bot's class wears
// (GearPolicy.Classes). Pieces compare by score (pieceScore: the model's
// value gain, else StatWeights), the worn ones' enchant counted, a
// weapon's with the shot factor (shotValues).
func (b *Brain) beats(id int32, w *worn) bool {
	return b.beatsBy(Item{TemplateID: id}, w, true)
}

// beatsBy is beats for carried item it, its enchant counted, counting the
// shots the bot carries towards the shot factor only when carried is set.
// In a slot the armor set the bot wears next decides (wearSet), only that
// set's piece beats what the bot wears; elsewhere pieces compare one by
// one (beatsPiece).
func (b *Brain) beatsBy(it Item, w *worn, carried bool) bool {
	if g, ok := b.gearItem(it.TemplateID); ok && b.cfg.Travel.setPieces != nil {
		if s := slotOf(g.part); s >= slotShield && s <= slotFeet {
			if p, ok := b.wearSet(w); ok && p.decides(s) {
				return p.pieces[s] == it.TemplateID && w[s].TemplateID != it.TemplateID
			}
		}
	}
	return b.beatsPiece(it, w, carried)
}

// beatsPiece is beatsBy for it alone: the scores of it and the worn piece,
// and for armor the bonus of the set the bot wears with it against the
// worn set's (setBonus), so a piece that breaks a set must beat its bonus
// too.
func (b *Brain) beatsPiece(it Item, w *worn, carried bool) bool {
	id := it.TemplateID
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
	if !ok {
		return false
	}
	if s == slotWeapon && len(class.Weapons) > 0 && !slices.Contains(class.Weapons, c.kind) {
		return b.leavesUnlisted(g, w, &class)
	}
	gs, cs := b.pieceScore(w, &g, it), b.pieceScore(w, &c, cur)
	gain, value := gs, cs
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
			var shield float64
			if sh := w[slotShield]; sh.ObjectID != 0 {
				worn, ok := b.gearItem(sh.TemplateID)
				if !ok {
					return false
				}
				shield = b.pieceScore(w, &worn, sh)
			}
			if class.Shield {
				shield = max(shield, b.carriedShield(w))
			}
			value += shield
		} else if class.Shield {
			gain += b.carriedShield(w)
		}
	case c.kind != g.kind:
		return false
	case g.part == partFullArmor && c.part == partChest && w[slotLegs].ObjectID != 0:
		// Full armor replaces the legs too.
		legs, ok := b.gearItem(w[slotLegs].TemplateID)
		if !ok {
			return false
		}
		value += b.pieceScore(w, &legs, w[slotLegs])
	}
	if s == slotWeapon {
		gain, value = b.shotValues(id, cur.TemplateID, gs, cs, gain, value, carried)
	} else if s >= slotShield && s <= slotFeet {
		next := wearing(w, it, &g)
		gain += b.setBonus(&next)
		value += b.setBonus(w)
	}
	return gain > value
}

// leavesUnlisted reports whether weapon g replaces a worn weapon of a type
// class does not wield (GearClass.Weapons): g is of a type it does, and
// takes no unknown shield off.
func (b *Brain) leavesUnlisted(g gearItem, w *worn, class *GearClass) bool {
	if !slices.Contains(class.Weapons, g.kind) {
		return false
	}
	if sh := w[slotShield]; g.part == partLRHand && sh.ObjectID != 0 {
		_, ok := b.gearItem(sh.TemplateID)
		return ok
	}
	return true
}

// swaps reports whether weapon types a and b are a one-handed type and its
// two-handed one, both of which the class wields.
func (c *GearClass) swaps(a, b string) bool {
	return a != b && strings.TrimPrefix(a, "BIG") == strings.TrimPrefix(b, "BIG") &&
		slices.Contains(c.Weapons, a) && slices.Contains(c.Weapons, b)
}

// weaponRank is where piece kind kind stands in the class's Weapons, the
// first listed first: it breaks a tie between two pieces of one score
// (ruleGear, planGear). A type the class does not list, and armor, rank
// last.
func (c *GearClass) weaponRank(kind string) int {
	if i := slices.Index(c.Weapons, kind); i >= 0 {
		return i
	}
	return len(c.Weapons)
}

// carriedShield is the score worn in w of the best shield the bot carries
// and would wear with a one-handed weapon; 0 for none.
func (b *Brain) carriedShield(w *worn) float64 {
	var best float64
	for _, it := range b.world.Inventory.Items {
		g, ok := b.gearItem(it.TemplateID)
		if ok && g.part == partLHand && !it.Equipped && it.Augmentation == 0 && !b.gear.refused[it.TemplateID] && g.grade <= b.gearGrade() {
			best = max(best, b.pieceScore(w, &g, it))
		}
	}
	return best
}

// upgrade reports whether carried item it is one the gear errand wears
// over w: it is never junk.
func (b *Brain) upgrade(it Item, w *worn) bool {
	p := &b.persona.Gear
	return p.Equip && !it.Equipped && it.Augmentation == 0 && !b.hold(it).NoWear && b.beatsBy(it, w, true)
}

// wearsNext reports whether the gear errand wears carried item it next.
func (b *Brain) wearsNext(it Item) bool {
	if _, ok := b.gearItem(it.TemplateID); !ok || it.Equipped {
		return false
	}
	w := b.worn()
	return b.upgrade(it, &w)
}

// ruleGear is a rule 9 errand: standing, out of combat and trade, the bot
// first takes off a piece worn above its Expertise grade while the server
// holds the grade penalty (overGrade), then uses the
// best (highest scoring) carried piece that beats the one worn in its slot (UseItem equips
// it), the first slot first. For an earring or ring with both sides worn
// it first uses the piece it replaces (UseItem takes it off,
// network/inventory.go toggleEquipItem), and wears the new one in the
// freed side next. A bow waits for arrows of its grade (bowWaits). One
// UseItem is waited for at a time, up to
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
			if ok && it.Equipped {
				b.armedBow(it)
			}
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
	if off := b.overGrade(&w); off.ObjectID != 0 {
		// Taken off for the grade penalty (#436): waited for unequipped.
		g.room = off.TemplateID
		a := useItemAction(off)
		a.object = off.ObjectID
		return a, true
	}
	var best Item
	var bestG gearItem
	class := b.gearClass()
	for _, it := range b.world.Inventory.Items {
		if !b.upgrade(it, &w) || b.bowWaits(it) {
			continue
		}
		gi, _ := b.gearItem(it.TemplateID)
		if best.ObjectID == 0 || cmp.Or(cmp.Compare(slotOf(gi.part), slotOf(bestG.part)), cmp.Compare(b.pieceScore(&w, &bestG, best), b.pieceScore(&w, &gi, it)), cmp.Compare(class.weaponRank(gi.kind), class.weaponRank(bestG.kind)), cmp.Compare(it.ObjectID, best.ObjectID)) < 0 {
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

// gearWants are the pieces the shop errand buys in the gear tier, one of
// each: the gearCandidates the adena past the reserve floor buys at their
// cheapest base price, best first; the planned set's pieces (gearPlan)
// only when it buys all of them, so the bot saves for a set rather than
// buy part of it. None takes the load into the half-speed band (gearFits):
// the set fits as a whole or is left out, and the other pieces fit on top
// of it, best first.
func (b *Brain) gearWants() []buyRow {
	budget := int64(b.carried(adenaID)) - b.floor(tierGear)
	if budget <= 0 {
		return nil
	}
	ids, set := b.gearPlan()
	var chosen []buyRow
	if setRows := oneEach(set); b.price(set) <= budget && b.loadFits(setRows...) {
		chosen = setRows
	}
	for _, id := range ids {
		r := buyRow{Item: id, Count: 1}
		if !slices.Contains(set, id) && int64(b.cfg.Travel.cheapest[id]) <= budget && b.gearFits(chosen, r) {
			chosen = append(chosen, r)
		}
	}
	var rows []buyRow
	for _, id := range ids {
		if r := (buyRow{Item: id, Count: 1}); slices.Contains(chosen, r) {
			rows = append(rows, r)
		}
	}
	return rows
}

// price is the cost of one of each of ids at their cheapest base price.
func (b *Brain) price(ids []int32) int64 {
	var sum int64
	for _, id := range ids {
		sum += int64(b.cfg.Travel.cheapest[id])
	}
	return sum
}

// gearCandidates are the pieces the gear errand would buy (gearPlan).
func (b *Brain) gearCandidates() []int32 {
	ids, _ := b.gearPlan()
	return ids
}

// gearPlan is what the gear errand would buy (GearPolicy.Buy): per slot
// without a carried upgrade, every piece merchants sell that beats the worn
// one, by slot then score, best first. In a slot the armor set the bot
// buys towards decides (bestSet on shopPool), the set's piece is the only
// one, a carried upgrade there notwithstanding, and set are those pieces:
// the ones the bot neither wears nor carries. The bot buys towards the set
// only when the adena past the gear tier's floor pays for them or has
// them within reach (reaches), so it saves for the set; out of reach, the
// set's slots take single pieces like the others.
//
// In the grade jump's window (GearPolicy.SaveBefore) every piece and set
// must also be worth its price against the jump goal's (worthBefore).
func (b *Brain) gearPlan() (ids, set []int32) {
	return b.planGear(b.price(b.jumpGoal()))
}

// planGear is gearPlan with the grade jump's goal priced at jump; 0 for
// none, which buys every piece that beats the worn one.
func (b *Brain) planGear(jump int64) (ids, set []int32) {
	b.gear.plans++
	p := &b.persona.Gear
	if !p.Buy || !p.Equip || b.cfg.Travel == nil {
		return nil, nil
	}
	w := b.worn()
	var held [slotCount]bool
	for _, it := range b.world.Inventory.Items {
		if b.upgrade(it, &w) {
			g, _ := b.gearItem(it.TemplateID)
			held[slotOf(g.part)] = true
		}
	}
	plan, planned := b.bestSet(&w, b.shopPool())
	if planned {
		var missing []int32
		for s, id := range plan.pieces {
			if plan.missing[s] {
				missing = append(missing, id)
			}
		}
		budget := int64(b.carried(adenaID)) - b.floor(tierGear)
		price := b.price(missing)
		planned = (price <= budget || b.reaches(price, budget)) && b.worthBefore(missing, &w, jump)
	}
	for _, id := range b.cfg.Travel.shopGear {
		g := b.cfg.Travel.gear[id]
		s := slotOf(g.part)
		if planned && plan.decides(s) {
			if plan.pieces[s] == id && plan.missing[s] {
				ids = append(ids, id)
				set = append(set, id)
			}
			continue
		}
		if held[s] || b.cfg.Travel.cheapest[id] <= 0 {
			continue
		}
		if b.unarmedBow(id) || !b.beats(id, &w) || !b.worthBefore([]int32{id}, &w, jump) {
			continue
		}
		ids = append(ids, id)
	}
	class := b.gearClass()
	slices.SortStableFunc(ids, func(x, y int32) int {
		gx, gy := b.cfg.Travel.gear[x], b.cfg.Travel.gear[y]
		return cmp.Or(cmp.Compare(slotOf(gx.part), slotOf(gy.part)), cmp.Compare(b.pieceScore(&w, &gy, Item{}), b.pieceScore(&w, &gx, Item{})), cmp.Compare(class.weaponRank(gx.kind), class.weaponRank(gy.kind)))
	})
	return ids, set
}

// gearGoal is the next upgrade, the one the shop errand buys or saves for
// (docs/items.md section 5): the first of gearCandidates, the best piece
// of the first slot with one, or every piece of the planned set to buy
// when that is one of them (gearPlan, docs/items.md section 6); set is that
// set's pieces to buy. In the grade jump's window it holds only the pieces
// worth buying before the jump, and a visit saves for the jump goal
// instead (jumpGoal, buyRows).
func (b *Brain) gearGoal() (goal, set []int32) {
	ids, set := b.gearPlan()
	switch {
	case len(ids) == 0:
		return nil, set
	case slices.Contains(set, ids[0]):
		return set, set
	}
	return ids[:1], set
}

// nextGrade is the grade of the next grade jump: the grade above the one
// the bot wears (gearGrade), when GearPolicy.MaxGrade allows it and the
// bot's level is within GearPolicy.SaveBefore levels below the level that
// grade's Expertise comes at (expertiseLevels).
func (b *Brain) nextGrade() (int32, bool) {
	p := &b.persona.Gear
	next := b.gearGrade() + 1
	if next > p.MaxGrade || int(next) > len(expertiseLevels) {
		return 0, false
	}
	lvl, at := b.world.Self.Level, expertiseLevels[next-1]
	return next, lvl < at && at-lvl <= p.SaveBefore
}

// jumpGoal is what a shop visit saves for in the grade jump's window
// (nextGrade, docs/items.md section 6): the next grade's core pieces the
// gear errand would buy once it wears that grade (gearPlan one grade
// ahead), weapon first: the best such weapon; with none, or one carried,
// the planned set of that grade, else its best body piece and, unless that
// is full armor, its best legs piece. Nil outside the window. They are
// saved for, never bought before the jump.
func (b *Brain) jumpGoal() []int32 {
	next, ok := b.nextGrade()
	if !ok || !b.persona.Gear.Buy || !b.persona.Gear.Equip || b.cfg.Travel == nil {
		return nil
	}
	b.gear.ahead = true
	ids, set := b.planGear(0)
	b.gear.ahead = false
	best := func(s gearSlot) (gearItem, int32, bool) {
		for _, id := range ids {
			if g := b.cfg.Travel.gear[id]; slotOf(g.part) == s && g.grade == next {
				return g, id, true
			}
		}
		return gearItem{}, 0, false
	}
	if _, id, ok := best(slotWeapon); ok {
		return []int32{id}
	}
	if len(set) > 0 && b.cfg.Travel.gear[set[0]].grade == next {
		return set
	}
	var goal []int32
	body, id, ok := best(slotBody)
	if ok {
		goal = append(goal, id)
	}
	if !ok || body.part != partFullArmor {
		if _, id, ok := best(slotLegs); ok {
			goal = append(goal, id)
		}
	}
	return goal
}

// worthBefore reports whether buying ids before the grade jump is worth
// it against a jump goal costing jump (GearPolicy.SaveWorth): their score
// gain over what the bot wears in the slots they fill, full armor filling
// the legs too, is at least SaveWorth times the share of jump their price
// is, both as fractions, and the shot factor and set bonus left out. True
// with no goal (jump 0), and for pieces that fill empty slots.
func (b *Brain) worthBefore(ids []int32, w *worn, jump int64) bool {
	f := b.persona.Gear.SaveWorth
	if jump <= 0 || f <= 0 || len(ids) == 0 {
		return true
	}
	var slots [slotCount]bool
	var gain, value float64
	for _, id := range ids {
		g := b.cfg.Travel.gear[id]
		gain += b.pieceScore(w, &g, Item{})
		slots[slotOf(g.part)] = true
		if g.part == partFullArmor {
			slots[slotLegs] = true
		}
	}
	for s, on := range slots {
		if c, ok := b.gearItem(w[s].TemplateID); on && ok {
			value += b.pieceScore(w, &c, w[s])
		}
	}
	if value <= 0 {
		return true
	}
	return (gain-value)*float64(jump) >= f*float64(b.price(ids))*value
}
