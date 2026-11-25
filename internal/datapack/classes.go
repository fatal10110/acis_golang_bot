package datapack

import (
	"fmt"
	"slices"
)

// Class is the part of a profession's template (classes/*.xml) bots plan a
// character with: its line, its base stats, its HP, MP and CP by level and
// the skills it may learn.
type Class struct {
	ID int32
	// Parent is the profession ID upgrades from, -1 for a base class.
	Parent int32
	// Race is the client race ordinal of the class's line (0 Human, 1 Elf,
	// 2 Dark Elf, 3 Orc, 4 Dwarf), as UserInfo sends it.
	Race int32
	// STR to MEN are the six base stats, at the server's defaults (40, 21,
	// 30, 20, 43, 20) where the template leaves one out.
	STR, CON, DEX, INT, WIT, MEN int
	// PAtk, PDef, MAtk and MDef are the naked base combat stats.
	PAtk, PDef, MAtk, MDef float64
	// RunSpeed is runSpd, 1 when absent.
	RunSpeed float64
	// HPTable, MPTable and CPTable are the base max HP, MP and CP by
	// character level: entry i is level i+1's.
	HPTable, MPTable, CPTable []float64
	// Skills is the skill tree: the class's own grants in file order, then
	// its parent's merged tree, as the server's NewTemplateTable merges it
	// (a profession keeps what its line learned before it).
	Skills []SkillGrant
}

// SkillGrant is one <skill> of a class's skill tree: level Level of skill
// ID, learnable from character level MinLevel for Cost SP.
type SkillGrant struct {
	ID              int32
	Level, MinLevel int
	Cost            int
}

// classParents maps every profession ID to the one it upgrades from, -1 for
// a base class. The class files do not carry it: this is the server's
// table (model/actor/player/class.go classParent). Every parent ID is
// smaller than its children's, so one ascending pass merges whole lines.
var classParents = map[int32]int32{
	0: -1, 1: 0, 2: 1, 3: 1, 4: 0, 5: 4, 6: 4, 7: 0, 8: 7, 9: 7,
	10: -1, 11: 10, 12: 11, 13: 11, 14: 11, 15: 10, 16: 15, 17: 15,
	18: -1, 19: 18, 20: 19, 21: 19, 22: 18, 23: 22, 24: 22,
	25: -1, 26: 25, 27: 26, 28: 26, 29: 25, 30: 29,
	31: -1, 32: 31, 33: 32, 34: 32, 35: 31, 36: 35, 37: 35,
	38: -1, 39: 38, 40: 39, 41: 39, 42: 38, 43: 42,
	44: -1, 45: 44, 46: 45, 47: 44, 48: 47,
	49: -1, 50: 49, 51: 50, 52: 50,
	53: -1, 54: 53, 55: 54, 56: 53, 57: 56,

	88: 2, 89: 3, 90: 5, 91: 6, 92: 9, 93: 8, 94: 12, 95: 13, 96: 14, 97: 16, 98: 17,
	99: 20, 100: 21, 101: 23, 102: 24, 103: 27, 104: 28, 105: 30,
	106: 33, 107: 34, 108: 36, 109: 37, 110: 40, 111: 41, 112: 43,
	113: 46, 114: 48, 115: 51, 116: 52,
	117: 55, 118: 57,
}

// baseClassRaces is the race of each base class, which its whole line
// shares (the server's model/actor/player/race.go ClassRace).
var baseClassRaces = map[int32]int32{
	0: 0, 10: 0, 18: 1, 25: 1, 31: 2, 38: 2, 44: 3, 49: 3, 53: 4,
}

// ClassParent is the profession id upgrades from, -1 for a base class; ok
// is false for an id the profession table does not hold.
func ClassParent(id int32) (parent int32, ok bool) {
	parent, ok = classParents[id]
	return parent, ok
}

// BaseClass is the base class of profession id's line (id itself for a
// base class); ok is false for an id the profession table does not hold.
func BaseClass(id int32) (base int32, ok bool) {
	parent, ok := classParents[id]
	if !ok {
		return 0, false
	}
	for parent >= 0 {
		id, parent = parent, classParents[parent]
	}
	return id, true
}

// ParentClass is the profession id upgrades from, -1 for a base class; ok
// is false for an id the profession table does not hold.
func ParentClass(id int32) (parent int32, ok bool) {
	parent, ok = classParents[id]
	return parent, ok
}

// ClassRace is the race of profession id's line (0 Human, 1 Elf, 2 Dark
// Elf, 3 Orc, 4 Dwarf); ok is false for an id the profession table does not
// hold.
func ClassRace(id int32) (race int32, ok bool) {
	base, ok := BaseClass(id)
	if !ok {
		return 0, false
	}
	return baseClassRaces[base], true
}

type classFile struct {
	Classes []classElement `xml:"class"`
}

// classElement is one <class>: its <set> children, merged later-wins, and
// its skill tree. <items> and <spawns> are not read.
type classElement struct {
	Sets   []attrsElement `xml:"set"`
	Skills []attrsElement `xml:"skills>skill"`
}

// LoadClasses reads every *.xml class template file directly under dir, in
// sorted file order, keyed by class ID, and merges each profession's skill
// tree with its parent's (see Class.Skills). A file that is not
// well-formed, a class missing or mangling a value bots read, a class ID
// the profession table does not hold, or a profession whose parent is not
// loaded fails the whole load. An ID declared twice keeps its last
// declaration, as on the server.
func LoadClasses(dir string) (map[int32]Class, error) {
	paths, err := xmlFiles(dir, "class")
	if err != nil {
		return nil, err
	}
	classes := make(map[int32]Class)
	for _, path := range paths {
		var f classFile
		if err := decodeFile(path, &f); err != nil {
			return nil, err
		}
		for _, el := range f.Classes {
			c, err := buildClass(el)
			if err != nil {
				return nil, fmt.Errorf("datapack: %s: %w", path, err)
			}
			classes[c.ID] = c
		}
	}
	ids := make([]int32, 0, len(classes))
	for id := range classes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		c := classes[id]
		if c.Parent < 0 {
			continue
		}
		parent, ok := classes[c.Parent]
		if !ok {
			return nil, fmt.Errorf("datapack: class %d: parent class %d not loaded", id, c.Parent)
		}
		c.Skills = append(slices.Clip(c.Skills), parent.Skills...)
		classes[id] = c
	}
	return classes, nil
}

// buildClass mirrors the server's buildTemplate (data/xml/classtemplate.go)
// for the fields bots read: id, pAtk, pDef, mAtk, mDef, hpTable, mpTable
// and cpTable are required, the six base stats default to 40, 21, 30, 20, 43 and 20 and
// runSpd to 1; a skill grant's id, lvl, minLvl and cost are all required.
func buildClass(el classElement) (Class, error) {
	a := &attrReader{vals: make(map[string]string)}
	for _, s := range el.Sets {
		for _, at := range s.Attrs {
			a.vals[at.Name.Local] = at.Value
		}
	}
	id := a.int32("id")
	if a.err != nil {
		return Class{}, fmt.Errorf("class: %w", a.err)
	}
	parent, ok := classParents[id]
	if !ok {
		return Class{}, fmt.Errorf("class %d: unknown profession id", id)
	}
	root, _ := BaseClass(id)
	c := Class{
		ID: id, Parent: parent, Race: baseClassRaces[root],
		STR: a.intDefault("str", 40), CON: a.intDefault("con", 21), DEX: a.intDefault("dex", 30),
		INT: a.intDefault("int", 20), WIT: a.intDefault("wit", 43), MEN: a.intDefault("men", 20),
		PAtk: a.float("pAtk"), PDef: a.float("pDef"), MAtk: a.float("mAtk"), MDef: a.float("mDef"),
		RunSpeed: a.floatDefault("runSpd", 1),
		HPTable:  a.floats("hpTable"), MPTable: a.floats("mpTable"), CPTable: a.floats("cpTable"),
	}
	if a.err != nil {
		return Class{}, fmt.Errorf("class %d: %w", id, a.err)
	}
	for _, s := range el.Skills {
		g := foldAttrs(s.Attrs)
		grant := SkillGrant{ID: g.int32("id"), Level: g.int("lvl"), MinLevel: g.int("minLvl"), Cost: g.int("cost")}
		if g.err != nil {
			return Class{}, fmt.Errorf("class %d: skill grant: %w", id, g.err)
		}
		c.Skills = append(c.Skills, grant)
	}
	return c, nil
}
