package botclient

import (
	"cmp"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// expertiseGrade is the Expertise grade a character of level has, 0 (no
// grade) to 5 (S): one per expertiseLevels entry it has reached.
func expertiseGrade(level int) int32 {
	var g int32
	for _, at := range expertiseLevels {
		if int32(level) >= at {
			g++
		}
	}
	return g
}

// refGear is the grade-typical loadout of a player of build gc at grade
// (docs/pvp/model.md "Engage estimate"): the middle-priced complete armor
// set of gc's armor type, the middle-priced weapon of gc's first weapon
// type, with a one-handed weapon the set's shield where gc wears a shield
// (else the grade's middle-priced shield), and the grade's middle-priced
// necklace, two earrings and two rings. "Middle-priced" is the median by
// reference price, the lower one of an even count, the template id
// breaking ties: the grade's cheapest pieces (common items) and its top
// ones (raid jewelry) are not what a player of the grade typically wears.
// items are the templates, funcs the complete set's skills' stat functions
// (setFuncs); none below grade D or for a slot the grade has no piece of.
func (m *TravelMap) refGear(gc GearClass, grade int32) (items []datapack.Item, funcs []datapack.StatFunc) {
	if grade <= 0 {
		return nil, nil
	}
	var weapons, shields, neck, ear, finger []int32
	for id, g := range m.gear {
		if g.grade != grade {
			continue
		}
		switch slotOf(g.part) {
		case slotWeapon:
			if len(gc.Weapons) > 0 && g.kind == gc.Weapons[0] {
				weapons = append(weapons, id)
			}
		case slotShield:
			shields = append(shields, id)
		case slotNeck:
			neck = append(neck, id)
		case slotEar:
			ear = append(ear, id)
		case slotFinger:
			finger = append(finger, id)
		}
	}
	var sets []int32
	for chest := range m.sets {
		if g := m.gear[chest]; g.grade == grade && g.kind == gc.Armor {
			sets = append(sets, chest)
		}
	}
	var w worn
	set := m.sets[median(sets, m.setPrice)]
	for _, s := range setSlots {
		w[s].TemplateID = set.pieces[s]
	}
	w[slotWeapon].TemplateID = median(weapons, m.piecePrice)
	if gc.Shield && m.gear[w[slotWeapon].TemplateID].part == partRHand {
		w[slotShield].TemplateID = cmp.Or(set.pieces[slotShield], median(shields, m.piecePrice))
	}
	w[slotNeck].TemplateID = median(neck, m.piecePrice)
	w[slotEar].TemplateID = median(ear, m.piecePrice)
	w[slotFinger].TemplateID = median(finger, m.piecePrice)
	for s, it := range w {
		if it.TemplateID == 0 {
			continue
		}
		items = append(items, m.gear[it.TemplateID].tmpl)
		if gearSlot(s) == slotEar || gearSlot(s) == slotFinger {
			items = append(items, m.gear[it.TemplateID].tmpl) // the pair's second
		}
	}
	return items, m.setFuncs(&w)
}

// piecePrice is gear template id's reference price.
func (m *TravelMap) piecePrice(id int32) int64 { return int64(m.gear[id].tmpl.Price) }

// setPrice is the summed reference price of the pieces of the armor set
// whose chest is chest.
func (m *TravelMap) setPrice(chest int32) int64 {
	var p int64
	for _, id := range m.sets[chest].pieces {
		if id != 0 {
			p += m.piecePrice(id)
		}
	}
	return p
}

// median is the median of ids by price, the lower one of an even count,
// the id breaking ties; 0 for none. It sorts ids.
func median(ids []int32, price func(int32) int64) int32 {
	if len(ids) == 0 {
		return 0
	}
	slices.SortFunc(ids, func(a, b int32) int {
		return cmp.Or(cmp.Compare(price(a), price(b)), cmp.Compare(a, b))
	})
	return ids[(len(ids)-1)/2]
}

// refGearClass is the build the reference opponent of class wears gear
// for: its default build (defaultGearClasses, not the bot persona's
// table), with a bow and no shield where its row shoots (bow, a scout
// line seen wielding a bow, charmodel.ClassRow).
func refGearClass(class int32, bow bool) GearClass {
	gc := gearClassOf(defaultGearClasses(), class)
	if bow {
		gc.Weapons, gc.Shield = []string{"BOW"}, false
	}
	return gc
}

// refKey keys the reference opponents (TravelMap.refFighter) by class and
// bow.
type refKey struct {
	class int32
	bow   bool
}

// refFighter is the reference opponent of class at level
// (docs/pvp/model.md "Engage estimate", "PvP value"): its class template at level
// wearing the grade-typical gear (refGear) of its build (refGearClass, a
// bow one where bow) at the Expertise grade of level, no dyes, no passive
// skills; below level 20 (no grade) the bare template. The caller sets
// Row; ok is false without class's template. The engage estimate
// (Brain.pvpRef) and the PvP value mix (mixShares) both use it.
func (m *TravelMap) refFighter(class int32, bow bool, level int) (charmodel.Fighter, bool) {
	cl, ok := m.classes[class]
	if !ok {
		return charmodel.Fighter{}, false
	}
	items, funcs := m.refGear(refGearClass(class, bow), expertiseGrade(level))
	return charmodel.ModelFighter(charmodel.Build{Class: cl, Level: level, Mage: charmodel.MageClass(m.classes, class), Worn: items, Funcs: funcs}), true
}

// pvpRef is the engage estimate's reference opponent of class at level
// (TravelMap.refFighter). The caller sets Row. Cached per class and bow
// while level holds, as engages runs per candidate each tick; ok is false
// without class's template.
func (b *Brain) pvpRef(m *TravelMap, class int32, bow bool, level int) (charmodel.Fighter, bool) {
	p, key := &b.pvp, refKey{class, bow}
	if p.refs == nil || p.refsLevel != level {
		p.refs, p.refsLevel = make(map[refKey]charmodel.Fighter), level
	}
	if f, ok := p.refs[key]; ok {
		return f, true
	}
	f, ok := m.refFighter(class, bow, level)
	if !ok {
		return charmodel.Fighter{}, false
	}
	p.refs[key] = f
	return f, true
}
