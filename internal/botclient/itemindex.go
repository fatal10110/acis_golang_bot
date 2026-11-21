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

// shotKinds are the player's shot kinds in the order indexShots lists them.
var shotKinds = []datapack.EtcKind{datapack.EtcSoulshot, datapack.EtcSpiritshot, datapack.EtcBlessedSpiritshot}

// indexShots lists the soulshots, spiritshots and blessed spiritshots of a
// datapack, in that order, each kind by grade, the beginners' unsellable
// ones before the others of their grade, then by id.
func indexShots(etc map[int32]datapack.EtcItem) []int32 {
	var ids []int32
	for id, it := range etc {
		if slices.Contains(shotKinds, it.Kind) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b int32) int {
		x, y := etc[a], etc[b]
		return cmp.Or(
			cmp.Compare(slices.Index(shotKinds, x.Kind), slices.Index(shotKinds, y.Kind)),
			cmp.Compare(x.Grade, y.Grade),
			cmp.Compare(boolRank(x.Sellable), boolRank(y.Sellable)),
			cmp.Compare(a, b),
		)
	})
	return ids
}

// boolRank orders false before true.
func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// WithItemIndex returns p with the healing potions and shots of m's
// datapack in place of the built-in lists DefaultPersona carries for a bot
// without a datapack (healingPotions, shotTemplates). A persona that sets
// lists of its own keeps them, and so does one whose datapack lists none.
func (m *TravelMap) WithItemIndex(p Persona) Persona {
	if len(m.potions) > 0 && slices.Equal(p.Potions, healingPotions) {
		p.Potions = slices.Clone(m.potions)
	}
	if len(m.shots) > 0 && slices.Equal(p.Shots, shotTemplates) {
		p.Shots = slices.Clone(m.shots)
	}
	return p
}
