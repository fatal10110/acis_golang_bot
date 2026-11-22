package botclient

import (
	"cmp"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// hpRecover is the stack type of the potions that heal HP over time.
const hpRecover = "hp_recover"

// indexPotions lists the healing potions of a datapack: the potions whose
// first skill heals HP over time (a HealOverTime effect of stack type
// hp_recover), best first: the highest stack order, the one the server keeps
// when two land, then the lowest id.
func indexPotions(etc map[int32]datapack.EtcItem, skills map[datapack.SkillRef]datapack.Skill) []int32 {
	order := make(map[int32]float64)
	for id, it := range etc {
		if it.Kind != datapack.EtcPotion || len(it.Skills) == 0 {
			continue
		}
		ref := it.Skills[0]
		for _, e := range skills[ref].Effects {
			if e.Name == "HealOverTime" && e.StackType == hpRecover {
				order[id] = e.StackOrder
				break
			}
		}
	}
	ids := make([]int32, 0, len(order))
	for id := range order {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b int32) int {
		return cmp.Or(cmp.Compare(order[b], order[a]), cmp.Compare(a, b))
	})
	return ids
}

// shotKinds are the player's shot kinds.
var shotKinds = []datapack.EtcKind{datapack.EtcSoulshot, datapack.EtcSpiritshot, datapack.EtcBlessedSpiritshot}

// indexShots indexes the soulshots, spiritshots and blessed spiritshots of
// a datapack by kind, grade and whether they are a beginner's: the
// beginners' shots are not sellable.
func indexShots(etc map[int32]datapack.EtcItem) map[int32]shotItem {
	out := make(map[int32]shotItem)
	for id, it := range etc {
		if slices.Contains(shotKinds, it.Kind) {
			out[id] = shotItem{kind: it.Kind, grade: int32(it.Grade), beginner: !it.Sellable}
		}
	}
	return out
}

// indexWeights indexes the weight of every etc item template that weighs
// anything.
func indexWeights(etc map[int32]datapack.EtcItem) map[int32]int32 {
	out := make(map[int32]int32)
	for id, it := range etc {
		if it.Weight > 0 {
			out[id] = it.Weight
		}
	}
	return out
}

// WithItemIndex returns p with the healing potions of m's datapack in
// place of the built-in list DefaultPersona carries for a bot without a
// datapack (healingPotions). A persona that sets a list of its own keeps
// it, and so does one whose datapack lists none. The shots a bot uses come
// from the datapack's shot table by themselves (shotsFor).
func (m *TravelMap) WithItemIndex(p Persona) Persona {
	if len(m.potions) > 0 && slices.Equal(p.Potions, healingPotions) {
		p.Potions = slices.Clone(m.potions)
	}
	return p
}
