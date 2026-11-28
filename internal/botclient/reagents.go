package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// reagent is the item one cast of a skill level consumes and how many of
// it (datapack.Skill.ItemConsumeID, ItemConsumeCount); the zero value for
// a cast that consumes none.
type reagent struct {
	item, count int32
}

// indexReagents indexes the reagent of every level of the skills one level
// of which consumes an item, by skill id then plain level, a level that
// consumes none included (so the top plain level is known). The server
// charges an item only when both its id and count are above 0
// (cast/controller.go CanCast): a level with an id and count 0 (2234, 2276)
// costs nothing.
func indexReagents(skills map[datapack.SkillRef]datapack.Skill) map[int32]map[int32]reagent {
	costly := make(map[int32]bool)
	for ref, sk := range skills {
		if sk.ItemConsumeID > 0 && sk.ItemConsumeCount > 0 {
			costly[ref.ID] = true
		}
	}
	out := make(map[int32]map[int32]reagent, len(costly))
	for ref, sk := range skills {
		if !costly[ref.ID] {
			continue
		}
		levels := out[ref.ID]
		if levels == nil {
			levels = make(map[int32]reagent)
			out[ref.ID] = levels
		}
		var r reagent
		if sk.ItemConsumeID > 0 && sk.ItemConsumeCount > 0 {
			r = reagent{item: sk.ItemConsumeID, count: sk.ItemConsumeCount}
		}
		levels[ref.Level] = r
	}
	return out
}

// reagentOf is the reagent of skill at level (indexReagents), an
// enchanted level's that of the top plain level (the reader skips the
// enchanted levels); the zero value for none or with no travel map.
func (m *TravelMap) reagentOf(skill, level int32) reagent {
	if m == nil {
		return reagent{}
	}
	levels := m.reagents[skill]
	if r, ok := levels[level]; ok || level <= enchantLevels {
		return r
	}
	top := int32(0)
	for l := range levels {
		if l <= enchantLevels {
			top = max(top, l)
		}
	}
	return levels[top]
}

// reagentOf is the reagent of a cast of skill at the level the bot knows
// it at; the zero value for a skill it does not know.
func (b *Brain) reagentOf(skill int32) reagent {
	s, ok := b.world.Skills[skill]
	if !ok {
		return reagent{}
	}
	return b.cfg.Travel.reagentOf(skill, s.Level)
}

// affords reports whether the bot carries the reagent a cast of skill
// consumes, every stack counted: the server refuses the cast of a caster
// carrying fewer with SystemMessage 113, the message of a failed Charges
// condition too (network/magic_skill.go, ErrNotEnoughItems), so the bot
// never asks for it and chargesRefused never reads an item refusal.
func (b *Brain) affords(skill int32) bool {
	r := b.reagentOf(skill)
	return r.count == 0 || b.carried(r.item) >= r.count
}

// castReady reports whether the bot may cast skill at now: the skill is
// ready (World.SkillReady) and the bot carries its reagent (affords).
// Every cast the brain decides on reads it.
func (b *Brain) castReady(skill int32, now time.Time) bool {
	return b.world.SkillReady(skill, now) && b.affords(skill)
}

// reagentNeed is the restock of one reagent: how many the bot keeps of it
// at least and at most (ShopPolicy.ReagentCastsMin, ReagentCastsMax), summed
// over the kit's skills that consume it.
type reagentNeed struct {
	item     int32
	min, max int32
}

// reagentNeeds are the reagents of the kit's skills the bot knows (its
// rotation, buffs, heals, toggles, Relax and flee skill), in kit order,
// each sized ReagentCastsMin and ReagentCastsMax casts per skill at the
// level the bot knows it, summed per item: a bishop's Major Heal (1 Spirit
// Ore) and Major Group Heal (4) make 500 at ReagentCastsMax 100. A
// summon skill of the kit's Summons counts one full life (summonLife: the
// cast and every upkeep checkpoint) as one cast (docs/servitors.md section
// 4); no spawned merchant sells its crystals, so reagentWants buys none and
// the count only keeps them from the junk sale (usesReagent). A skill
// listed twice counts once.
func (b *Brain) reagentNeeds() []reagentNeed {
	if b.cfg.Travel == nil {
		return nil
	}
	kit := b.build().Kit
	var skills []int32
	for _, st := range kit.Rotation {
		skills = append(skills, st.Skill)
	}
	for _, bf := range kit.Buffs {
		skills = append(skills, bf.Skill)
	}
	for _, h := range kit.Heals {
		skills = append(skills, h.Skill)
	}
	for _, t := range kit.Toggles {
		skills = append(skills, t.Skill)
	}
	skills = append(skills, kit.Relax, kit.Flee)
	p := &b.persona.Shop
	seen := make(map[int32]bool, len(skills))
	var out []reagentNeed
	add := func(r reagent) {
		i := -1
		for j := range out {
			if out[j].item == r.item {
				i = j
			}
		}
		if i < 0 {
			out = append(out, reagentNeed{item: r.item})
			i = len(out) - 1
		}
		out[i].min += p.ReagentCastsMin * r.count
		out[i].max += p.ReagentCastsMax * r.count
	}
	for _, id := range skills {
		if seen[id] {
			continue
		}
		seen[id] = true
		if r := b.reagentOf(id); r.count > 0 {
			add(r)
		}
	}
	for _, id := range kit.Summons {
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, r := range b.summonLife(id) {
			add(r)
		}
	}
	return out
}

// reagentWants are the reagents the shop errand buys (ShopPolicy
// ReagentCastsMin, ReagentCastsMax): each the list of a spawned merchant
// sells, up to its max once fewer than its min are carried. A reagent no
// spawned merchant sells (Crystal D, only the unspawned Sales Cat's) is not
// bought: the gate keeps its skills silent (affords).
func (b *Brain) reagentWants() []buyRow {
	var rows []buyRow
	for _, n := range b.reagentNeeds() {
		if have := b.carried(n.item); have < n.min && n.max > have && b.cfg.Travel.spawnedSold[n.item] {
			rows = append(rows, buyRow{Item: n.item, Count: n.max - have})
		}
	}
	return rows
}

// reagentShort reports whether the bot carries some, but fewer than the
// ReagentCastsMin sum, of a reagent of its kit a spawned merchant sells: a
// supply trip's cause (runsOut), as its shots running out are. One it
// carries none of does not count, as with shots: a bot that could not buy
// it farms without its skills.
func (b *Brain) reagentShort() bool {
	for _, n := range b.reagentNeeds() {
		if have := b.carried(n.item); have > 0 && have < n.min && b.cfg.Travel.spawnedSold[n.item] {
			return true
		}
	}
	return false
}

// usesReagent reports whether item is the reagent of a skill of the kit
// (reagentNeeds), which the bot never sells as junk.
func (b *Brain) usesReagent(item int32) bool {
	for _, n := range b.reagentNeeds() {
		if n.item == item {
			return true
		}
	}
	return false
}
