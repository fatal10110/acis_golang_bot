package datapack

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Item is the part of a weapon, armor or arrow template (items/*.xml) bots
// pick gear and ammunition with. The etc items bots use are EtcItem.
type Item struct {
	ID   int32
	Name string
	// Type is "Weapon", "Armor" or "EtcItem" (arrows only).
	Type string
	// BodyPart is the paperdoll part as the file names it: rhand, lrhand,
	// chest, fullarmor, legs, head, gloves, feet and so on; "" for none.
	BodyPart string
	// Kind is a weapon's weapon_type (SWORD, BLUNT, BOW, ...), an armor
	// piece's armor_type (LIGHT, HEAVY, MAGIC, ...) and an etc item's
	// etcitem_type (ARROW); "" when the file sets none.
	Kind string
	// Magical is a weapon's is_magical: a mystic's weapon (wands, magic
	// blunts and swords), whose attacks the server scales by M. Atk.
	Magical bool
	// Grade is the crystal grade: 0 for none, then 1 for D up to 5 for S,
	// the level of the Expertise skill that wears it without a penalty.
	Grade int
	// CrystalCount is how many crystals of its grade crystallizing a +0
	// one gives (crystal_count, model/item/crystal.go CrystalCountAt); 0
	// for none: the server crystallizes a template only with a grade and a
	// count.
	CrystalCount int32
	// Price is the reference price in adena.
	Price int32
	// Weight is the template's weight, what one carried adds to the load.
	Weight int32
	// Duration is a shadow item's mana in minutes, 0 for an ordinary item.
	Duration int
	// Conditional is set for a template with use or equip conditions (a
	// <cond> block, use_condition or equip_condition): the server may
	// refuse to equip it.
	Conditional bool
	// Stats are the base stats the template's <for> block gives.
	Stats Stats
	// Enchants are the stats its <enchant> entries name, in file order:
	// the ones an enchant level raises (EnchantGain).
	Enchants []string
	// Soulshots and Spiritshots are how many shots a weapon's attack or
	// cast spends; 0 for armor and arrows.
	Soulshots, Spiritshots int
	// Skills are the item_skill entries: the passive skills the item
	// gives while worn, whose Funcs carry a weapon's Special Ability
	// (pvpPhysDmg, pvpPhysSkillsDmg, pvpMagicalDmg) and a jewel's
	// vulnerabilities; nil for none.
	Skills []SkillRef
}

// Stats are the base values of the stats bots compare gear by: what a
// template's <for> block sets (<set>, weapons) or adds to the base
// (<baseadd>, armor), the phases the server runs before it scales a stat by
// level and attributes (skill/effect/mod.go Op.order). The flat <add>,
// <sub> and <mul> modifiers that run after that scaling are not read; in
// the shipped datapack only pet weapons <add> to these stats. SDef is a
// shield's defense, which counts on a hit the shield blocks.
type Stats struct {
	PAtk, MAtk, PDef, MDef, PAtkSpd, RCrit, SDef float64
}

// field returns the field of s that the stat named name (as the files spell
// it) reads into, nil for a stat bots do not read.
func (s *Stats) field(name string) *float64 {
	switch name {
	case "pAtk":
		return &s.PAtk
	case "mAtk":
		return &s.MAtk
	case "pDef":
		return &s.PDef
	case "mDef":
		return &s.MDef
	case "pAtkSpd":
		return &s.PAtkSpd
	case "rCrit":
		return &s.RCrit
	case "sDef":
		return &s.SDef
	}
	return nil
}

// EtcKind is the use bots put an etc item template to.
type EtcKind string

// The etc item kinds bots keep (buildEtcItem).
const (
	EtcSoulshot          EtcKind = "soulshot"
	EtcSpiritshot        EtcKind = "spiritshot"
	EtcBlessedSpiritshot EtcKind = "blessed spiritshot"
	EtcPotion            EtcKind = "potion"
	EtcScroll            EtcKind = "scroll"
	EtcEnchantScroll     EtcKind = "enchant scroll"
	// EtcMaterial is a crafting material (etcitem_type MATERIAL): what a
	// clan crafter keeps for its recipes.
	EtcMaterial EtcKind = "material"
	EtcDye      EtcKind = "dye"
)

// EtcItem is the part of an etc item template bots use, restock or keep:
// a player's shots, potions, scrolls, enchant scrolls, crafting materials
// and dyes. Arrows are an Item; other etc items are not read.
type EtcItem struct {
	ID   int32
	Name string
	Kind EtcKind
	// Type is the etcitem_type as the file names it (POTION, SCROLL,
	// SCRL_ENCHANT_WP, MATERIAL, DYE, ...), "" when it sets none, as for
	// shots. Potions and scrolls of escape share default_action
	// skill_reduce: only Type tells them apart.
	Type string
	// Grade is the crystal grade, as Item.Grade.
	Grade  int
	Weight int32
	// Skills are the item_skill entries: the skills using the item casts.
	Skills []SkillRef
	// Reuse is reuse_delay: how long after one use the server refuses the
	// next of the item; 0 for none.
	Reuse time.Duration
	// Tradable is is_tradable and Sellable is_sellable: the beginners'
	// shots are neither.
	Tradable, Sellable bool
}

// SkillRef names one level of a skill: an entry of an item_skill value, and
// the key of Datapack.Skills.
type SkillRef struct {
	ID, Level int32
}

// grades maps crystal_type to Item.Grade (item.ParseCrystalType on the
// server).
var grades = map[string]int{"NONE": 0, "D": 1, "C": 2, "B": 3, "A": 4, "S": 5}

type itemFile struct {
	Items []itemElement `xml:"item"`
}

// itemElement is one <item>: its attributes, its direct <set> children,
// whether it has a <cond> block, and its <for> stat block.
type itemElement struct {
	Attrs []xml.Attr   `xml:",any,attr"`
	Sets  []setElement `xml:"set"`
	Conds []struct{}   `xml:"cond"`
	For   []struct {
		Mods []statElement `xml:",any"`
	} `xml:"for"`
}

// statElement is one child of a <for> block: <set>, <baseadd>, <add>,
// <sub>, <mul> or <enchant>, naming its stat.
type statElement struct {
	XMLName xml.Name
	Stat    string `xml:"stat,attr"`
	Val     string `xml:"val,attr"`
}

// ItemTables are the item template tables LoadItems reads.
type ItemTables struct {
	// Items are the weapons, armor and arrows.
	Items map[int32]Item
	// Etc are the etc items bots use (buildEtcItem).
	Etc map[int32]EtcItem
	// Prices are the reference prices of every priced template (buildPrice).
	Prices map[int32]int32
	// Names are the names of the templates in Prices, whatever their type.
	Names map[int32]string
	// Stackable are the templates in Prices that stack (is_stackable): the
	// server stores one row of each, not one per unit.
	Stackable map[int32]bool
}

// LoadItems reads every *.xml item template file directly under dir, in
// sorted file order, keeping the weapons, armor and arrows, the etc items
// bots use, and the reference price and name of every priced template. A
// file that is not well-formed, or a template missing or mangling a field
// bots read, fails the whole load. An id declared twice keeps its last
// declaration.
func LoadItems(dir string) (ItemTables, error) {
	paths, err := xmlFiles(dir, "item")
	if err != nil {
		return ItemTables{}, err
	}
	t := ItemTables{
		Items: make(map[int32]Item), Etc: make(map[int32]EtcItem),
		Prices: make(map[int32]int32), Names: make(map[int32]string), Stackable: make(map[int32]bool),
	}
	for _, path := range paths {
		var f itemFile
		if err := decodeFile(path, &f); err != nil {
			return ItemTables{}, err
		}
		for _, el := range f.Items {
			if err := t.add(el); err != nil {
				return ItemTables{}, fmt.Errorf("datapack: %s: %w", path, err)
			}
		}
	}
	return t, nil
}

// add replaces whatever t holds for el's id with el's templates.
func (t ItemTables) add(el itemElement) error {
	id, name, price, priced, err := buildPrice(el)
	if err != nil {
		return err
	}
	delete(t.Prices, id)
	delete(t.Names, id)
	delete(t.Stackable, id)
	delete(t.Items, id)
	delete(t.Etc, id)
	if priced {
		t.Prices[id], t.Names[id] = price, name
		if itemAttrs(el).boolDefault("is_stackable", false) {
			t.Stackable[id] = true
		}
	}
	it, ok, err := buildItem(el)
	if err != nil {
		return err
	}
	if ok {
		t.Items[id] = it
		return nil
	}
	e, ok, err := buildEtcItem(el)
	if err != nil {
		return err
	}
	if ok {
		t.Etc[id] = e
	}
	return nil
}

// itemAttrs reads el's attributes and its <set> values into one reader.
func itemAttrs(el itemElement) *attrReader {
	a := foldAttrs(el.Attrs)
	for _, s := range el.Sets {
		a.vals[s.Name] = s.Val
	}
	return a
}

// buildPrice reads the id, name and reference price of any template (id and
// name are required, as on the server), reporting false for one a private
// store never lists: no price above 0, or is_tradable false
// (item.Instance.Tradable, privatestore ItemsToSell).
func buildPrice(el itemElement) (id int32, name string, price int32, priced bool, err error) {
	a := itemAttrs(el)
	id, name, price = a.int32("id"), a.str("name"), a.int32Default("price", 0)
	if a.err != nil {
		return 0, "", 0, false, fmt.Errorf("item %s: %w", a.strDefault("id", "?"), a.err)
	}
	return id, name, price, price > 0 && a.boolDefault("is_tradable", true), nil
}

// buildItem reads a weapon, armor or arrow template, reporting false for
// any other: id, type and name are required, price, duration and the shot
// counts and crystal_count default to 0, crystal_type to none.
func buildItem(el itemElement) (Item, bool, error) {
	a := itemAttrs(el)
	it := Item{ID: a.int32("id"), Type: a.str("type"), Name: a.str("name")}
	if a.err != nil {
		return Item{}, false, fmt.Errorf("item %s: %w", a.strDefault("id", "?"), a.err)
	}
	var kindKey string
	switch it.Type {
	case "Weapon":
		kindKey = "weapon_type"
	case "Armor":
		kindKey = "armor_type"
	case "EtcItem":
		// Arrows only: a bow shoots the ones of its grade
		// (itemcontainer/inventory.go arrowIDForCrystal).
		if a.strDefault("etcitem_type", "") != "ARROW" {
			return Item{}, false, nil
		}
		kindKey = "etcitem_type"
	default:
		return Item{}, false, nil
	}
	it.BodyPart = a.strDefault("bodypart", "")
	it.Kind = a.strDefault(kindKey, "")
	it.Magical = a.boolDefault("is_magical", false)
	it.Price = a.int32Default("price", 0)
	it.Weight = a.int32Default("weight", 0)
	it.Duration = a.intDefault("duration", 0)
	grade, ok := grades[a.strDefault("crystal_type", "NONE")]
	if !ok {
		return Item{}, false, fmt.Errorf("item %d: crystal_type %q", it.ID, a.vals["crystal_type"])
	}
	it.Grade = grade
	it.CrystalCount = a.int32Default("crystal_count", 0)
	_, use := a.vals["use_condition"]
	_, equip := a.vals["equip_condition"]
	it.Conditional = len(el.Conds) > 0 || use || equip
	it.Soulshots = a.intDefault("soulshots", 0)
	it.Spiritshots = a.intDefault("spiritshots", 0)
	if a.err != nil {
		return Item{}, false, fmt.Errorf("item %d: %w", it.ID, a.err)
	}
	skills, err := parseSkillRefs(a.strDefault("item_skill", ""))
	if err != nil {
		return Item{}, false, fmt.Errorf("item %d: item_skill: %w", it.ID, err)
	}
	it.Skills = skills
	if err := readStats(el, &it); err != nil {
		return Item{}, false, fmt.Errorf("item %d: %w", it.ID, err)
	}
	return it, true, nil
}

// readStats reads el's <for> block into it: the <set> values first, then
// the <baseadd> amounts added to them, the server's calculation order; and
// the stat of every <enchant> entry. A value bots read that is not a
// number fails the template.
func readStats(el itemElement, it *Item) error {
	for _, op := range []string{"set", "baseadd"} {
		for _, f := range el.For {
			for _, m := range f.Mods {
				dst := it.Stats.field(m.Stat)
				if m.XMLName.Local != op || dst == nil {
					continue
				}
				v, err := strconv.ParseFloat(m.Val, 64)
				if err != nil {
					return fmt.Errorf("<%s stat=%q>: %w", op, m.Stat, err)
				}
				if op == "set" {
					*dst = v
				} else {
					*dst += v
				}
			}
		}
	}
	for _, f := range el.For {
		for _, m := range f.Mods {
			if m.XMLName.Local == "enchant" {
				if m.Stat == "" {
					return fmt.Errorf("<enchant>: %w", errMissing)
				}
				it.Enchants = append(it.Enchants, m.Stat)
			}
		}
	}
	return nil
}

// buildEtcItem reads an etc item template of an EtcKind, reporting false
// for any other. A player's shots are told by their handler (the fish and
// pet shots have handlers of their own, and blessed spiritshots share
// default_action spiritshot), enchant scrolls by their handler or their
// etcitem_type (the crystal scrolls set no enchant type), the rest by
// etcitem_type. id and name are required; weight and reuse_delay default
// to 0, crystal_type to none, is_tradable and is_sellable to true.
func buildEtcItem(el itemElement) (EtcItem, bool, error) {
	a := itemAttrs(el)
	if a.strDefault("type", "") != "EtcItem" {
		return EtcItem{}, false, nil
	}
	e := EtcItem{Type: a.strDefault("etcitem_type", "")}
	switch handler := a.strDefault("handler", ""); {
	case handler == "SoulShots":
		e.Kind = EtcSoulshot
	case handler == "SpiritShots":
		e.Kind = EtcSpiritshot
	case handler == "BlessedSpiritShots":
		e.Kind = EtcBlessedSpiritshot
	case handler == "EnchantScrolls", strings.Contains(e.Type, "SCRL_ENCHANT_"):
		e.Kind = EtcEnchantScroll
	case e.Type == "POTION":
		e.Kind = EtcPotion
	case e.Type == "SCROLL":
		e.Kind = EtcScroll
	case e.Type == "MATERIAL":
		e.Kind = EtcMaterial
	case e.Type == "DYE":
		e.Kind = EtcDye
	default:
		return EtcItem{}, false, nil
	}
	e.ID, e.Name = a.int32("id"), a.str("name")
	e.Weight = a.int32Default("weight", 0)
	e.Reuse = time.Duration(a.int32Default("reuse_delay", 0)) * time.Millisecond
	e.Tradable = a.boolDefault("is_tradable", true)
	e.Sellable = a.boolDefault("is_sellable", true)
	if a.err != nil {
		return EtcItem{}, false, fmt.Errorf("item %s: %w", a.strDefault("id", "?"), a.err)
	}
	grade, ok := grades[a.strDefault("crystal_type", "NONE")]
	if !ok {
		return EtcItem{}, false, fmt.Errorf("item %d: crystal_type %q", e.ID, a.vals["crystal_type"])
	}
	e.Grade = grade
	skills, err := parseSkillRefs(a.strDefault("item_skill", ""))
	if err != nil {
		return EtcItem{}, false, fmt.Errorf("item %d: item_skill: %w", e.ID, err)
	}
	e.Skills = skills
	return e, true, nil
}

// parseSkillRefs parses an item_skill value the way the server does
// (model/item/skillref.go ParseSkillRefs): ";"-separated "id-level" pairs
// of base-10 numbers; "" is none.
func parseSkillRefs(s string) ([]SkillRef, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ";")
	refs := make([]SkillRef, len(parts))
	for i, p := range parts {
		id, level, ok := strings.Cut(p, "-")
		if !ok {
			return nil, fmt.Errorf("skill reference %q: want \"id-level\"", p)
		}
		idNum, err := strconv.ParseInt(id, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("skill reference %q: %w", p, err)
		}
		levelNum, err := strconv.ParseInt(level, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("skill reference %q: %w", p, err)
		}
		refs[i] = SkillRef{ID: int32(idNum), Level: int32(levelNum)}
	}
	return refs, nil
}
