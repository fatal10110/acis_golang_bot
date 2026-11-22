package datapack

import "fmt"

// ArmorSet is one armor set (armorSets.xml <armorset>): the pieces whose
// wearing together grants the set skill. A piece of 0 is none: the set
// names no piece for that slot, and any piece there keeps it complete
// (model/armorset ContainsAll). A set with no legs is a full armor chest
// and the pieces it names.
type ArmorSet struct {
	Name string
	// Chest keys the set: the server finds a worn set by its chest.
	Chest, Legs, Head, Gloves, Feet int32
	// SkillID is the skill a complete set grants.
	SkillID int32
	// Shield is the set's shield and ShieldSkillID the skill a complete set
	// grants with it worn too; 0 for a set without one.
	Shield, ShieldSkillID int32
	// Enchant6Skill is the skill a complete set grants with every piece it
	// names at +6 or more; 0 for none.
	Enchant6Skill int32
}

type armorSetsFile struct {
	Sets []attrsElement `xml:"armorset"`
}

// LoadArmorSets reads armorSets.xml at path: every set by its chest
// template id. Every attribute is required, every one but name an int32,
// as on the server (data/xml/economy.go LoadArmorSets). A later set of the
// same chest replaces the earlier one, as in the server's table
// (model/armorset NewTable). A piece no item file holds is kept: the set
// simply never completes.
func LoadArmorSets(path string) (map[int32]ArmorSet, error) {
	var f armorSetsFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make(map[int32]ArmorSet, len(f.Sets))
	for _, e := range f.Sets {
		s, err := buildArmorSet(e)
		if err != nil {
			return nil, fmt.Errorf("datapack: %s: %w", path, err)
		}
		out[s.Chest] = s
	}
	return out, nil
}

// buildArmorSet builds one <armorset> from its attributes.
func buildArmorSet(e attrsElement) (ArmorSet, error) {
	a := foldAttrs(e.Attrs)
	name := a.str("name")
	if a.err != nil {
		return ArmorSet{}, fmt.Errorf("armorset: %w", a.err)
	}
	s := ArmorSet{
		Name:          name,
		Chest:         a.int32("chest"),
		Legs:          a.int32("legs"),
		Head:          a.int32("head"),
		Gloves:        a.int32("gloves"),
		Feet:          a.int32("feet"),
		SkillID:       a.int32("skillId"),
		Shield:        a.int32("shield"),
		ShieldSkillID: a.int32("shieldSkillId"),
		Enchant6Skill: a.int32("enchant6Skill"),
	}
	if a.err != nil {
		return ArmorSet{}, fmt.Errorf("armorset %q: %w", name, a.err)
	}
	return s, nil
}
