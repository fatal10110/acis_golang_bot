package botclient

import (
	"cmp"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// armorSet is an armor set (datapack.ArmorSet) of pieces the bot may wear:
// the template of each slot it names, the body slot its chest and the
// shield slot its shield; 0 where it names none.
type armorSet struct{ pieces [slotCount]int32 }

// setSlots are the slots of a set's pieces but the shield, which the
// server checks to grant the set skill (model/armorset ContainsAll).
var setSlots = [...]gearSlot{slotBody, slotLegs, slotHead, slotGloves, slotFeet}

// indexSets indexes the armor sets whose every named piece is gear
// (indexGear) of the slot it names, by chest: a set with a piece the bot
// never wears never completes. A full armor chest with legs named would
// never complete either (the server takes the legs off). A shield that is
// not gear is dropped from its set, which still completes without it.
// pieces holds every template the indexed sets name.
func indexSets(sets map[int32]datapack.ArmorSet, gear map[int32]gearItem) (map[int32]armorSet, map[int32]bool) {
	out := make(map[int32]armorSet)
	pieces := make(map[int32]bool)
next:
	for chest, s := range sets {
		var a armorSet
		for i, id := range [...]int32{s.Chest, s.Legs, s.Head, s.Gloves, s.Feet} {
			if id == 0 {
				continue
			}
			if g, ok := gear[id]; !ok || slotOf(g.part) != setSlots[i] {
				continue next
			}
			a.pieces[setSlots[i]] = id
		}
		if a.pieces[slotBody] == 0 || a.pieces[slotLegs] != 0 && gear[s.Chest].part == partFullArmor {
			continue
		}
		if g, ok := gear[s.Shield]; ok && g.part == partLHand {
			a.pieces[slotShield] = s.Shield
		}
		out[chest] = a
		for _, id := range a.pieces {
			if id != 0 {
				pieces[id] = true
			}
		}
	}
	return out, pieces
}

// complete reports whether w wears every piece of s but the shield: a
// piece s does not name accepts anything (model/armorset ContainsAll).
func (s *armorSet) complete(w *worn) bool {
	for _, slot := range setSlots {
		if id := s.pieces[slot]; id != 0 && w[slot].TemplateID != id {
			return false
		}
	}
	return true
}

// setBonus is the bonus of the armor set w completes (GearPolicy.SetBonus):
// the fraction of its worn pieces' score by wt, the set shield's when worn
// (the server grants the shield skill only with the set complete,
// skill/persistence.go armorSetGrants); 0 without a complete set.
func (b *Brain) setBonus(w *worn, wt StatWeights) float64 {
	f := b.persona.Gear.SetBonus
	if f <= 0 || b.cfg.Travel == nil {
		return 0
	}
	s, ok := b.cfg.Travel.sets[w[slotBody].TemplateID]
	if !ok || !s.complete(w) {
		return 0
	}
	var sum float64
	for slot, id := range s.pieces {
		if id != 0 && w[slot].TemplateID == id {
			g := b.cfg.Travel.gear[id]
			sum += g.score(wt, w[slot])
		}
	}
	return f * sum
}

// wearing is w with gear template g worn as item it: full armor takes the
// legs off.
func wearing(w *worn, it Item, g *gearItem) worn {
	out := *w
	it.BodySlot = g.part
	out[slotOf(g.part)] = it
	if g.part == partFullArmor {
		out[slotLegs] = Item{}
	}
	return out
}

// setPlan is the armor set the bot wears next, or buys and wears
// (bestSet): the template it wears in each slot the set decides, the set's
// shield where it counts, and which of them the bot neither wears nor
// carries.
type setPlan struct {
	pieces [slotCount]int32
	// covers are the slots the set decides: its pieces', and the legs of a
	// full armor chest.
	covers [slotCount]bool
	// missing are the pieces to buy.
	missing [slotCount]bool
}

// decides reports whether p decides what the bot wears in slot s.
func (p *setPlan) decides(s gearSlot) bool { return s < slotCount && p.covers[s] }

// bestSet is the armor set the bot wears over w from what it wears and
// pool, the eligible templates it could wear (wearPool, shopPool), each
// with the instance it would: the set gaining the most, by value (its
// pieces' score, its shield's where that beats the best other shield, plus
// GearPolicy.SetBonus of both) over the slots it decides, against the more
// of what the bot wears there, a worn set's bonus counted, and the best
// piece per slot of what it wears and the pool's pieces that beat it
// (beatsPiece). The chest fits the armor type the bot wears, or its class's
// with none (GearClass.Armor), as in beatsPiece; the shield counts only for
// a class wearing one and no two-handed weapon. A set it wears whole is
// none: nothing is left to wear. Ties go to the lower chest id.
func (b *Brain) bestSet(w *worn, pool map[int32]Item) (setPlan, bool) {
	m := b.cfg.Travel
	f := b.persona.Gear.SetBonus
	if f <= 0 || m == nil || len(m.sets) == 0 {
		return setPlan{}, false
	}
	var chests []int32
	for id := range pool {
		if _, ok := m.sets[id]; ok {
			chests = append(chests, id)
		}
	}
	if id := w[slotBody].TemplateID; id != 0 && pool[id].TemplateID == 0 {
		if _, ok := m.sets[id]; ok {
			chests = append(chests, id)
		}
	}
	if len(chests) == 0 {
		return setPlan{}, false
	}
	class := b.gearClass()
	wt := class.weights()
	var cur, best [slotCount]float64
	for s := slotShield; s <= slotFeet; s++ {
		if it := w[s]; it.ObjectID != 0 {
			g, ok := m.gear[it.TemplateID]
			if !ok {
				return setPlan{}, false
			}
			cur[s] = g.score(wt, it)
		}
	}
	best = cur
	for id, it := range pool {
		g := m.gear[id]
		if s := slotOf(g.part); s >= slotShield && s <= slotFeet && b.beatsPiece(it, w, true) {
			best[s] = max(best[s], g.score(wt, it))
		}
	}
	wornBonus := b.setBonus(w, wt)
	have := func(s gearSlot, id int32) (Item, bool) {
		if w[s].TemplateID == id {
			return w[s], true
		}
		it, ok := pool[id]
		return it, ok
	}
	var plan setPlan
	var planGain float64
	var planChest int32
	found := false
	for _, chest := range chests {
		set := m.sets[chest]
		if !b.fitsArmor(m.gear[chest].kind, w, &class) {
			continue
		}
		var p setPlan
		var sum, mix, now float64
		ok := true
		for _, s := range setSlots {
			id := set.pieces[s]
			if id == 0 {
				if s == slotLegs && m.gear[chest].part == partFullArmor {
					p.covers[s] = true
					mix, now = mix+best[s], now+cur[s]
				}
				continue
			}
			it, has := have(s, id)
			if !has {
				ok = false
				break
			}
			g := m.gear[id]
			sum += g.score(wt, it)
			p.pieces[s], p.covers[s], p.missing[s] = id, true, it.ObjectID == 0
			mix, now = mix+best[s], now+cur[s]
		}
		if !ok {
			continue
		}
		value := (1 + f) * sum
		if id := set.pieces[slotShield]; id != 0 && class.Shield && w[slotWeapon].BodySlot != partLRHand {
			if it, has := have(slotShield, id); has {
				g := m.gear[id]
				if v := (1 + f) * g.score(wt, it); v > best[slotShield] {
					value += v
					p.pieces[slotShield], p.covers[slotShield], p.missing[slotShield] = id, true, it.ObjectID == 0
					mix, now = mix+best[slotShield], now+cur[slotShield]
				}
			}
		}
		worn := true
		for s, id := range p.pieces {
			if id != 0 && w[s].TemplateID != id {
				worn = false
			}
		}
		gain := value - max(mix, now+wornBonus)
		if worn || gain <= 0 {
			continue
		}
		if !found || cmp.Or(cmp.Compare(planGain, gain), cmp.Compare(chest, planChest)) < 0 {
			plan, planGain, planChest, found = p, gain, chest, true
		}
	}
	return plan, found
}

// fitsArmor reports whether a body or legs piece of armor type kind fits
// w: of the type of the body piece the bot wears, else of its legs piece,
// else of the class's (GearClass.Armor), as beatsPiece matches them. An
// unknown worn piece fits nothing.
func (b *Brain) fitsArmor(kind string, w *worn, class *GearClass) bool {
	for _, s := range [...]gearSlot{slotBody, slotLegs} {
		if it := w[s]; it.ObjectID != 0 {
			g, ok := b.gearItem(it.TemplateID)
			return ok && g.kind == kind
		}
	}
	return kind == class.Armor
}

// wearPool are the armor pieces (shield to feet) the bot carries and would
// wear, by template, each the best scoring instance: Equip on, not worn,
// augmented, held back from wearing (Hold.NoWear) or refused, and of a
// grade it wears.
func (b *Brain) wearPool() map[int32]Item {
	if !b.persona.Gear.Equip || b.cfg.Travel == nil {
		return nil
	}
	wt := b.gearWeights()
	pool := make(map[int32]Item)
	for _, it := range b.world.Inventory.Items {
		g, ok := b.gearItem(it.TemplateID)
		if s := slotOf(g.part); !ok || s < slotShield || s > slotFeet || it.Equipped || it.Augmentation != 0 ||
			b.gear.refused[it.TemplateID] || g.grade > b.gearGrade() || b.hold(it).NoWear {
			continue
		}
		if p, ok := pool[it.TemplateID]; !ok || cmp.Or(cmp.Compare(g.score(wt, p), g.score(wt, it)), cmp.Compare(it.ObjectID, p.ObjectID)) < 0 {
			pool[it.TemplateID] = it
		}
	}
	return pool
}

// shopPool is wearPool with every armor piece merchants sell that the bot
// does not carry, a zero object id marking it to buy (GearPolicy.Buy).
func (b *Brain) shopPool() map[int32]Item {
	pool := b.wearPool()
	if pool == nil || !b.persona.Gear.Buy {
		return pool
	}
	m := b.cfg.Travel
	for _, id := range m.shopGear {
		g := m.gear[id]
		if s := slotOf(g.part); s < slotShield || s > slotFeet || m.cheapest[id] <= 0 || b.gear.refused[id] || g.grade > b.gearGrade() {
			continue
		}
		if _, ok := pool[id]; !ok {
			pool[id] = Item{TemplateID: id}
		}
	}
	return pool
}

// wearSet is the set the gear errand wears over w from what the bot
// carries (bestSet on wearPool).
func (b *Brain) wearSet(w *worn) (setPlan, bool) {
	return b.bestSet(w, b.wearPool())
}

// keepsForSet reports whether carried item it is a piece of the set the
// bot buys towards (bestSet on shopPool), whatever adena it has now
// (gearPlan saves for the set only within reach): it is not junk while the
// rest is bought, though it beats nothing alone.
func (b *Brain) keepsForSet(it Item, w *worn) bool {
	if b.cfg.Travel == nil || !b.cfg.Travel.setPieces[it.TemplateID] || it.Equipped || !b.persona.Gear.Buy {
		return false
	}
	p, ok := b.bestSet(w, b.shopPool())
	if !ok {
		return false
	}
	g, _ := b.gearItem(it.TemplateID)
	s := slotOf(g.part)
	return p.decides(s) && p.pieces[s] == it.TemplateID
}
