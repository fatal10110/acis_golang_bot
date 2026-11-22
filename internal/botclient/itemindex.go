package botclient

import (
	"cmp"
	"slices"
	"time"

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

// indexWeights indexes the weight of every item and etc item template that
// weighs anything.
func indexWeights(items map[int32]datapack.Item, etc map[int32]datapack.EtcItem) map[int32]int32 {
	out := make(map[int32]int32)
	for id, it := range items {
		if it.Weight > 0 {
			out[id] = it.Weight
		}
	}
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

// stackEffect is the stack type and order of a skill level's effect: of two
// effects of one type the server keeps the one of the higher order.
type stackEffect struct {
	kind  string
	order float64
}

// buffPotion is a buff potion: the skill level it lands, that effect's
// stack, and how long the effect lasts.
type buffPotion struct {
	skill datapack.SkillRef
	stack stackEffect
	lasts time.Duration
}

// stackless are the stack types that stack with any effect, or that
// indexPotions covers.
var stackless = []string{"", "none", hpRecover}

// indexBuffPotions indexes the buff potions of a datapack, the potions
// whose first skill lands a timed Buff effect of a stack type, and the
// stack effect of every skill level with an effect of one of those types,
// by skill id then level: a support's Haste or Acumen and the Newbie
// Guide's beginner buffs, each at the order its level's #stackOrder gives.
func indexBuffPotions(etc map[int32]datapack.EtcItem, skills map[datapack.SkillRef]datapack.Skill) (map[int32]buffPotion, map[int32]map[int32]stackEffect) {
	potions := make(map[int32]buffPotion)
	kinds := make(map[string]bool)
	for id, it := range etc {
		if it.Kind != datapack.EtcPotion || len(it.Skills) == 0 {
			continue
		}
		ref := it.Skills[0]
		for _, e := range skills[ref].Effects {
			if e.Name == "Buff" && e.Time > 0 && !slices.Contains(stackless, e.StackType) {
				potions[id] = buffPotion{skill: ref, stack: stackEffect{e.StackType, e.StackOrder}, lasts: time.Duration(e.Time) * time.Second}
				kinds[e.StackType] = true
				break
			}
		}
	}
	stacks := make(map[int32]map[int32]stackEffect)
	for ref, sk := range skills {
		for _, e := range sk.Effects {
			if !kinds[e.StackType] {
				continue
			}
			if stacks[ref.ID] == nil {
				stacks[ref.ID] = make(map[int32]stackEffect)
			}
			stacks[ref.ID][ref.Level] = stackEffect{e.StackType, e.StackOrder}
			break
		}
	}
	return potions, stacks
}

// stackOf is the stack effect of skill at level (indexBuffPotions); false
// for a skill of no buff potion's stack type. An enchanted level (101 and
// up), which the skill reader does not read, stacks as the skill's top
// level.
func (m *TravelMap) stackOf(skill, level int32) (stackEffect, bool) {
	levels := m.stacks[skill]
	if st, ok := levels[level]; ok {
		return st, true
	}
	if level <= enchantLevels {
		return stackEffect{}, false
	}
	top := int32(0)
	for l := range levels {
		if l <= enchantLevels {
			top = max(top, l)
		}
	}
	st, ok := levels[top]
	return st, ok
}

// enchantLevels is the last plain skill level: an enchanted skill level is
// 101 and up.
const enchantLevels = 100
