package datapack

import (
	"encoding/xml"
	"fmt"
)

// Item is the part of a weapon or armor template (items/*.xml) bots pick
// gear with. Etc items are not read.
type Item struct {
	ID   int32
	Name string
	// Type is "Weapon" or "Armor".
	Type string
	// BodyPart is the paperdoll part as the file names it: rhand, lrhand,
	// chest, fullarmor, legs, head, gloves, feet and so on; "" for none.
	BodyPart string
	// Kind is a weapon's weapon_type (SWORD, BLUNT, BOW, ...) and an armor
	// piece's armor_type (LIGHT, HEAVY, MAGIC, ...); "" when the file sets
	// none.
	Kind string
	// Magical is a weapon's is_magical: a mystic's weapon (wands, magic
	// blunts and swords), whose attacks the server scales by M. Atk.
	Magical bool
	// Grade is the crystal grade: 0 for none, then 1 for D up to 5 for S,
	// the level of the Expertise skill that wears it without a penalty.
	Grade int
	// Price is the reference price in adena.
	Price int32
	// Duration is a shadow item's mana in minutes, 0 for an ordinary item.
	Duration int
	// Conditional is set for a template with use or equip conditions (a
	// <cond> block, use_condition or equip_condition): the server may
	// refuse to equip it.
	Conditional bool
}

// grades maps crystal_type to Item.Grade (item.ParseCrystalType on the
// server).
var grades = map[string]int{"NONE": 0, "D": 1, "C": 2, "B": 3, "A": 4, "S": 5}

type itemFile struct {
	Items []itemElement `xml:"item"`
}

// itemElement is one <item>: its attributes, its direct <set> children and
// whether it has a <cond> block. Stat blocks are not read.
type itemElement struct {
	Attrs []xml.Attr   `xml:",any,attr"`
	Sets  []setElement `xml:"set"`
	Conds []struct{}   `xml:"cond"`
}

// LoadItems reads every *.xml item template file directly under dir, in
// sorted file order, keeping the weapons and armor. A file that is not
// well-formed, or a weapon or armor template missing or mangling a field
// bots read, fails the whole load. An id declared twice keeps its last
// declaration.
func LoadItems(dir string) (map[int32]Item, error) {
	paths, err := xmlFiles(dir, "item")
	if err != nil {
		return nil, err
	}
	items := make(map[int32]Item)
	for _, path := range paths {
		var f itemFile
		if err := decodeFile(path, &f); err != nil {
			return nil, err
		}
		for _, el := range f.Items {
			it, ok, err := buildItem(el)
			if err != nil {
				return nil, fmt.Errorf("datapack: %s: %w", path, err)
			}
			if ok {
				items[it.ID] = it
			}
		}
	}
	return items, nil
}

// buildItem reads a weapon or armor template, reporting false for any
// other type: id, type and name are required, price and duration default
// to 0, crystal_type to none.
func buildItem(el itemElement) (Item, bool, error) {
	a := foldAttrs(el.Attrs)
	for _, s := range el.Sets {
		a.vals[s.Name] = s.Val
	}
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
	default:
		return Item{}, false, nil
	}
	it.BodyPart = a.strDefault("bodypart", "")
	it.Kind = a.strDefault(kindKey, "")
	it.Magical = a.boolDefault("is_magical", false)
	it.Price = a.int32Default("price", 0)
	it.Duration = a.intDefault("duration", 0)
	grade, ok := grades[a.strDefault("crystal_type", "NONE")]
	if !ok {
		return Item{}, false, fmt.Errorf("item %d: crystal_type %q", it.ID, a.vals["crystal_type"])
	}
	it.Grade = grade
	_, use := a.vals["use_condition"]
	_, equip := a.vals["equip_condition"]
	it.Conditional = len(el.Conds) > 0 || use || equip
	if a.err != nil {
		return Item{}, false, fmt.Errorf("item %d: %w", it.ID, a.err)
	}
	return it, true, nil
}
