package botclient

import (
	"cmp"
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// statTieBreak weighs a piece's StatWeights score into its score where the
// model values gear (gearModel): the model's value gain decides, and the
// stats break its ties, such as jewelry's M. Def against a monster that
// hits only in melee, or anything a support's heal does not depend on.
const statTieBreak = 1e-6

// gearModel is the character model (internal/charmodel) the gear and dye
// errands value what the bot wears with (docs/strategy.md, Design section
// 5): the bot's class template, level, learned passives and dyes, its
// build's role, the reference monster of its level (indexRefMobs) as the
// target, and the calibration of the model against the stats UserInfo
// reports for what it wears now. It is valid while its inputs (in, skills)
// are the bot's; gen is the world's event count (World.gen) they were last
// found unchanged at, so no event since leaves them unchanged without a
// check. values holds the value of each arrangement asked for.
type gearModel struct {
	gen uint64
	in  gearInputs
	// skills are the bot's skills the model was made with: its passives
	// (passiveFuncs) and the heal or attack it values gear for (gearRole).
	skills map[int32]KnownSkill
	build  charmodel.Build
	role   charmodel.Role
	// tank values survival alone (gearRole).
	tank   bool
	target charmodel.Profile
	calib  charmodel.Calibration
	// norm is the value of what the bot wears now (1 when that is 0):
	// value gains are fractions of it, so they compare across roles.
	norm   float64
	values map[gearValueKey]float64
	// worn and funcs are profile's buffers for the gear and stat functions
	// of the arrangement it models, reused from one call to the next.
	worn  []datapack.Item
	funcs []datapack.StatFunc
	// pvp is the PvP side of its comparisons (pvp_gear.go), nil while
	// PvPPolicy.GearWeight is 0.
	pvp *gearPvP
}

// gearValueKey is an arrangement the model valued: the template and
// enchant level worn in each slot, whether a complete set's skills count,
// and the dyes.
type gearValueKey struct {
	worn wornKey
	sets bool
	dyes charmodel.Attrs
}

// wornKey is the template and enchant level worn in each slot, what the
// model sees of an arrangement (gearModel.profile).
type wornKey [slotCount]struct{ id, enchant int32 }

// keyOf is the wornKey of arrangement w.
func keyOf(w *worn) wornKey {
	var k wornKey
	for s, it := range w {
		if it.TemplateID != 0 {
			k[s].id, k[s].enchant = it.TemplateID, int32(it.Enchant)
		}
	}
	return k
}

// gearInputs are the inputs of the gear model but the skills
// (gearModel.skills): the bot's class, level and role, what it wears now,
// the stats UserInfo and StatusUpdate report (the calibration's), and its
// dyes' sum.
type gearInputs struct {
	key      buildKey
	worn     wornKey
	reported charmodel.Profile
	dyes     charmodel.Attrs
}

// valuer is the bot's gear model now, nil where the model cannot value
// gear: without a travel map, the bot's class template or a reference
// monster for its level. Then every score is the StatWeights score alone
// and sets take GearPolicy.SetBonus, as before the model.
//
// The model, and the values it holds, last while its inputs do: an event
// that changes none of them (a move, an HP or MP update, a ground item, a
// chat line) keeps it.
func (b *Brain) valuer() *gearModel {
	v, m, s := b.gearVal, b.cfg.Travel, &b.world.Self
	key := buildKey{s.ClassID, s.Level, b.persona.Role, b.persona.ServitorLines}
	if v != nil && v.gen == b.world.gen && v.in.key == key {
		return v
	}
	b.gearVal = nil
	if m == nil {
		return nil
	}
	now := b.rawWorn()
	in := gearInputs{key: key, worn: keyOf(&now), reported: b.reported(), dyes: b.wornDyes()}
	if v != nil && v.in == in && maps.Equal(v.skills, b.world.Skills) {
		v.gen, b.gearVal = b.world.gen, v
		b.gearPvP(m, v)
		return v
	}
	cl, ok := m.classes[s.ClassID]
	if !ok {
		return nil
	}
	target, ok := m.refMob(int(s.Level))
	if !ok {
		return nil
	}
	v = &gearModel{gen: b.world.gen, in: in, skills: maps.Clone(b.world.Skills), target: target, norm: 1, values: make(map[gearValueKey]float64)}
	v.role, v.tank = b.gearRole()
	v.build = charmodel.Build{Class: cl, Level: int(s.Level), Mage: charmodel.MageClass(m.classes, s.ClassID), Funcs: b.passiveFuncs(), Dyes: in.dyes}
	v.calib = charmodel.Calibrate(v.profile(m, &now, true, v.build.Dyes), in.reported)
	if n := v.value(m, &now, true, v.build.Dyes); n > 0 {
		v.norm = n
	}
	b.gearVal = v
	b.gearPvP(m, v)
	return v
}

// profile is the calibrated profile of arrangement w with dyes, a
// complete set's skills counted when sets is set.
func (v *gearModel) profile(m *TravelMap, w *worn, sets bool, dyes charmodel.Attrs) charmodel.Profile {
	return v.calib.Apply(charmodel.Model(v.buildOf(m, w, sets, dyes)))
}

// buildOf is the model's build wearing arrangement w with dyes, a complete
// set's skills counted when sets is set; its gear and stat functions are
// v's buffers, valid until the next call.
func (v *gearModel) buildOf(m *TravelMap, w *worn, sets bool, dyes charmodel.Attrs) charmodel.Build {
	bd := v.build
	bd.Dyes = dyes
	v.worn = v.worn[:0]
	for _, it := range w {
		if g, ok := m.gear[it.TemplateID]; ok && it.TemplateID != 0 {
			v.worn = append(v.worn, enchantedTmpl(g.tmpl, int(it.Enchant)))
		}
	}
	bd.Worn = v.worn
	if sets {
		if f := m.setFuncs(w); len(f) > 0 {
			v.funcs = append(append(v.funcs[:0], bd.Funcs...), f...)
			bd.Funcs = v.funcs
		}
	}
	return bd
}

// value is the model's value of arrangement w with dyes (charmodel.Value).
func (v *gearModel) value(m *TravelMap, w *worn, sets bool, dyes charmodel.Attrs) float64 {
	k := gearValueKey{worn: keyOf(w), sets: sets, dyes: dyes}
	if x, ok := v.values[k]; ok {
		return x
	}
	x := v.worth(v.profile(m, w, sets, dyes))
	v.values[k] = x
	return x
}

// worth is the value of profile p against the target: for a tank, the
// seconds it lasts (charmodel.TimeToDie); else charmodel.Value while it is
// positive, and below 0 how far the fight is from paying off, so a piece
// that brings it closer still gains: a damage role's time to kill over its
// time to die, a support's survival floor over its time to die, negated.
func (v *gearModel) worth(p charmodel.Profile) float64 {
	ttd := charmodel.TimeToDie(p, v.target).Seconds()
	if v.tank {
		return ttd
	}
	if x := charmodel.Value(p, v.role, v.target); x > 0 {
		return x
	}
	ttd = max(ttd, 1e-3)
	if v.role.Kind == charmodel.Support {
		return -v.role.SurvivalFloor.Seconds() / ttd
	}
	return -charmodel.TimeToKill(p, v.role.Attack, v.target).Seconds() / ttd
}

// gain is the value w2 adds over w1 (delta: PvP value blended in for a
// persona that weighs it), as a fraction of the value of what the bot
// wears now.
func (v *gearModel) gain(m *TravelMap, w2, w1 *worn, sets2, sets1 bool) float64 {
	d := v.build.Dyes
	return v.delta(m, w2, sets2, d, w1, sets1, d) / v.norm
}

// enchantedTmpl is template t at enchant level enchant: its P. Atk, M. Atk,
// P. Def and M. Def raised by its enchant gain (datapack.Item.EnchantGain),
// which the model does not know.
func enchantedTmpl(t datapack.Item, enchant int) datapack.Item {
	if enchant <= 0 {
		return t
	}
	for _, f := range [...]struct {
		name string
		v    *float64
	}{{"pAtk", &t.Stats.PAtk}, {"mAtk", &t.Stats.MAtk}, {"pDef", &t.Stats.PDef}, {"mDef", &t.Stats.MDef}} {
		*f.v += float64(t.EnchantGain(f.name, enchant))
	}
	return t
}

// pieceScore is the score of gear template g worn as item it in the
// arrangement w: with the model (valuer), the value it adds over its slot
// left empty (a two-handed weapon's with no shield, a full armor's with no
// legs), a complete set's skills left out (setBonus counts them), as a
// fraction of the value of what the bot wears now, its StatWeights score
// breaking ties (statTieBreak); without, the StatWeights score of the
// class entry (GearClass.Weights), enchant counted.
func (b *Brain) pieceScore(w *worn, g *gearItem, it Item) float64 {
	c := b.gearClass()
	stat := c.weights().score(&g.tmpl, int(it.Enchant))
	v := b.valuer()
	if v == nil {
		return stat
	}
	s := slotOf(g.part)
	base := *w
	base[s] = Item{}
	switch g.part {
	case partFullArmor:
		base[slotLegs] = Item{}
	case partLRHand:
		base[slotShield] = Item{}
	}
	with := base
	it.TemplateID, it.BodySlot = g.tmpl.ID, g.part
	with[s] = it
	return v.gain(b.cfg.Travel, &with, &base, false, false) + statTieBreak*stat
}

// statFactor is what a StatWeights score counts for in a score: 1 without
// the model, statTieBreak with it.
func (b *Brain) statFactor() float64 {
	if b.valuer() == nil {
		return 1
	}
	return statTieBreak
}

// setFuncs are the stat functions of the skills of the armor set w
// completes: the set skill, and the shield skill with the set's shield
// worn too (skill/persistence.go armorSetGrants); none without one.
func (m *TravelMap) setFuncs(w *worn) []datapack.StatFunc {
	s, ok := m.sets[w[slotBody].TemplateID]
	if !ok || !s.complete(w) {
		return nil
	}
	funcs := m.skills[datapack.SkillRef{ID: s.skill, Level: 1}].Funcs
	if id := s.pieces[slotShield]; id != 0 && w[slotShield].TemplateID == id && s.shieldSkill != 0 {
		funcs = append(slices.Clip(funcs), m.skills[datapack.SkillRef{ID: s.shieldSkill, Level: 1}].Funcs...)
	}
	return funcs
}

// rawWorn is what the bot wears now as the model sees it, without the
// scores worn uses to pick the earring and ring a new one replaces: the
// first equipped piece of each slot by object id.
func (b *Brain) rawWorn() worn {
	var w worn
	for _, it := range b.world.Inventory.Items {
		if !it.Equipped {
			continue
		}
		if s := slotOf(it.BodySlot); s != slotNone && (w[s].ObjectID == 0 || it.ObjectID < w[s].ObjectID) {
			w[s] = it
		}
	}
	return w
}

// reported is the bot's profile as its last UserInfo reports it; a stat
// not reported yet is 0 and calibrates nothing (charmodel.Calibrate).
func (b *Brain) reported() charmodel.Profile {
	s := &b.world.Self
	return charmodel.Profile{
		MaxHP: float64(s.MaxHP), MaxMP: float64(s.MaxMP), PAtk: float64(s.PAtk), MAtk: float64(s.MAtk),
		PAtkSpd: float64(s.PAtkSpd), MAtkSpd: float64(s.CastSpd), Critical: float64(s.Critical),
		Accuracy: float64(s.Accuracy), Evasion: float64(s.Evasion), PDef: float64(s.PDef), MDef: float64(s.MDef),
		RunSpeed: float64(s.RunSpeed), WeightLimit: float64(s.MaxLoad),
	}
}

// passiveFuncs are the stat functions of the passive skills the bot
// knows (masteries, Spellcraft), the set skills left out: the model counts
// a set's skills by what it wears (setFuncs).
func (b *Brain) passiveFuncs() []datapack.StatFunc {
	m := b.cfg.Travel
	ids := make([]int32, 0, len(b.world.Skills))
	for id, k := range b.world.Skills {
		if k.Passive && !k.Disabled && !m.setSkills[id] {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	var funcs []datapack.StatFunc
	for _, id := range ids {
		funcs = append(funcs, m.skills[datapack.SkillRef{ID: id, Level: b.world.Skills[id].Level}].Funcs...)
	}
	return funcs
}

// wornDyes are the base stat changes of the symbols the bot wears and its
// class uses (HennaInfo active), summed.
func (b *Brain) wornDyes() charmodel.Attrs {
	var a charmodel.Attrs
	for _, w := range b.world.Symbols.Worn {
		if h, ok := b.cfg.Travel.hennas[w.SymbolID]; ok && w.Active != 0 {
			a = addDye(a, h)
		}
	}
	return a
}

// addDye is a with symbol h's stat changes added.
func addDye(a charmodel.Attrs, h datapack.Henna) charmodel.Attrs {
	a.STR += h.STR
	a.CON += h.CON
	a.DEX += h.DEX
	a.INT += h.INT
	a.WIT += h.WIT
	a.MEN += h.MEN
	return a
}

// magicSkillTypes are the skill types whose damage is magic (M. Atk
// against M. Def, timed by cast speed; skill/formulas/damage.go).
var magicSkillTypes = []string{"MDAM", "DRAIN", "DEATHLINK"}

// gearRole is what the model values the bot's gear for (charmodel.Role):
// a Support build with an HP heal of type HEAL it knows and a caster's
// melee policy (not MeleeAlways), the HP a minute the first such heal
// restores (its power, M. Atk, its MP and cast time, and the rest): never
// Restore Life (HEAL_PERCENT), a HOT or an MP heal (Recharge), so the
// Bishop is valued by Greater Battle Heal; any other Support build is a
// tank, which values how long it lasts (tank): a fighter that heals, PL
// with Holy Blessing, still wears its plate and sword; any other build,
// the kills a minute of the first rotation step it knows that it kills
// with (killAttack, the spot fit's rule: not Self Heal's HEAL, and not a
// step gated on its own HP, so the sorcerer is valued by Prominence, not by
// Vampiric Touch; #676), else of its melee hits. A heal costs its
// mpInitialConsume and mpConsume as the server charges them
// (cast/controller.go CanCast); a heal that costs no MP is no heal.
func (b *Brain) gearRole() (role charmodel.Role, tank bool) {
	bd, m := b.build(), b.cfg.Travel
	known := func(id int32) (datapack.Skill, bool) {
		k, ok := b.world.Skills[id]
		if !ok || k.Disabled {
			return datapack.Skill{}, false
		}
		sk, ok := m.skills[datapack.SkillRef{ID: id, Level: k.Level}]
		return sk, ok && sk.Power > 0
	}
	if bd.Role == RoleSupport {
		for _, h := range bd.Kit.Heals {
			if sk, ok := known(h.Skill); ok && !h.MP && sk.Type == "HEAL" && sk.MPInitialConsume+sk.MPConsume > 0 && bd.Kit.Melee != MeleeAlways {
				return charmodel.Role{
					Kind: charmodel.Support, RestTime: charmodel.DefaultRestTime, SurvivalFloor: charmodel.DefaultSurvivalFloor,
					HealPower: sk.Power, HealMP: float64(sk.MPInitialConsume + sk.MPConsume), HealHitTime: sk.HitTime, HealReuse: sk.ReuseDelay, HealMAtk: true,
				}, false
			}
		}
		tank = true
	}
	role = charmodel.Role{Kind: charmodel.Damage, RestTime: charmodel.DefaultRestTime}
	for _, st := range bd.Kit.Rotation {
		if sk, ok := known(st.Skill); ok {
			if a, ok := killAttack(st, sk); ok {
				role.Attack = a
				break
			}
		}
	}
	return role, tank
}

// armorKinds are the body and legs armor kinds the bot may wear: its
// class's (GearClass.Armor) first, then each of its race's mystic grants
// (RaceProfile.MysticArmor) whose mastery it knows.
func (b *Brain) armorKinds() []string {
	bd := b.build()
	kinds := []string{bd.Kit.Gear.Armor}
	for _, g := range bd.Race.MysticArmor {
		if _, ok := b.world.Skills[g.Mastery]; ok && !slices.Contains(kinds, g.Kind) {
			kinds = append(kinds, g.Kind)
		}
	}
	return kinds
}

// pairKind is the armor kind of what w wears in the body and legs slots:
// a full armor's, or the chest's and legs' when they match; "" for a mix,
// a single piece or none.
func (b *Brain) pairKind(w *worn) string {
	body, ok := b.gearItem(w[slotBody].TemplateID)
	if !ok {
		return ""
	}
	if body.part == partFullArmor {
		return body.kind
	}
	if legs, ok := b.gearItem(w[slotLegs].TemplateID); ok && legs.kind == body.kind {
		return body.kind
	}
	return ""
}

// pairPlans are the plans that put on a chest and legs of an armor kind
// the bot may wear (armorKinds) other than the pair it wears (pairKind):
// for each such kind, the best scoring full armor, or chest and legs
// together, of what it wears and pool, each scored as worn with the other.
// The server counts a chest's or legs' kind only with the other of the
// same kind (itemcontainer/inventory.go setPaperdollItemLocked), so the
// bot changes kinds only by the pair.
func (b *Brain) pairPlans(w *worn, pool map[int32]Item) []setPlan {
	kinds := b.armorKinds()
	cur := b.pairKind(w)
	if len(kinds) == 1 && kinds[0] == cur {
		return nil
	}
	m := b.cfg.Travel
	have := func(s gearSlot, kind string) []Item {
		var out []Item
		if it := w[s]; it.ObjectID != 0 && m.gear[it.TemplateID].kind == kind {
			out = append(out, it)
		}
		for id, it := range pool {
			if g := m.gear[id]; slotOf(g.part) == s && g.kind == kind && id != w[s].TemplateID {
				out = append(out, it)
			}
		}
		slices.SortFunc(out, func(x, y Item) int { return cmp.Compare(x.TemplateID, y.TemplateID) })
		return out
	}
	var plans []setPlan
	for _, kind := range kinds {
		if kind == cur {
			continue
		}
		var best setPlan
		var bestScore float64
		found := false
		try := func(body, legs Item) {
			next := *w
			gb := m.gear[body.TemplateID]
			next[slotBody] = body
			next[slotBody].BodySlot = gb.part
			next[slotLegs] = legs
			score := b.pieceScore(&next, &gb, body)
			if legs.TemplateID != 0 {
				gl := m.gear[legs.TemplateID]
				score += b.pieceScore(&next, &gl, legs)
			}
			if found && score <= bestScore {
				return
			}
			var p setPlan
			p.covers[slotBody], p.covers[slotLegs] = true, true
			p.pieces[slotBody], p.missing[slotBody] = body.TemplateID, body.ObjectID == 0
			if legs.TemplateID != 0 {
				p.pieces[slotLegs], p.missing[slotLegs] = legs.TemplateID, legs.ObjectID == 0
			}
			best, bestScore, found = p, score, true
		}
		legs := have(slotLegs, kind)
		for _, body := range have(slotBody, kind) {
			if m.gear[body.TemplateID].part == partFullArmor {
				try(body, Item{})
				continue
			}
			for _, l := range legs {
				try(body, l)
			}
		}
		if found {
			plans = append(plans, best)
		}
	}
	return plans
}

// indexSetSkills are the skills armor sets grant: what the bot learns
// with a set worn (SkillList), which the model counts by what it wears
// instead (setFuncs).
func indexSetSkills(sets map[int32]datapack.ArmorSet) map[int32]bool {
	out := make(map[int32]bool)
	for _, s := range sets {
		for _, id := range [...]int32{s.SkillID, s.ShieldSkillID, s.Enchant6Skill} {
			if id != 0 {
				out[id] = true
			}
		}
	}
	return out
}

// indexRefMobs are the reference monsters gear is valued against
// (gearModel), by level: of the monster templates (type Monster) of each
// level with base combat stats, the median by modeled max HP, ties to the
// lower template id.
func indexRefMobs(npcs map[int32]datapack.NPC) map[int]charmodel.Profile {
	type mob struct {
		id int32
		p  charmodel.Profile
	}
	byLevel := make(map[int][]mob)
	for id, n := range npcs {
		if n.Type != "Monster" || n.Combat.HP <= 0 || n.Level <= 0 {
			continue
		}
		byLevel[n.Level] = append(byLevel[n.Level], mob{id, charmodel.Monster(n)})
	}
	out := make(map[int]charmodel.Profile, len(byLevel))
	for lvl, mobs := range byLevel {
		slices.SortFunc(mobs, func(x, y mob) int { return cmp.Or(cmp.Compare(x.p.MaxHP, y.p.MaxHP), cmp.Compare(x.id, y.id)) })
		out[lvl] = mobs[len(mobs)/2].p
	}
	return out
}

// refMob is the reference monster of level, else of the nearest level
// below it that has one, else of the nearest above.
func (m *TravelMap) refMob(level int) (charmodel.Profile, bool) {
	best, ok := 0, false
	for lvl := range m.refMobs {
		switch {
		case !ok:
			best, ok = lvl, true
		case lvl <= level && (best > level || lvl > best):
			best = lvl
		case lvl > level && best > level && lvl < best:
			best = lvl
		}
	}
	return m.refMobs[best], ok
}
