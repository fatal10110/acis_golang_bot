package botclient

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// mysticPvPClasses are the classes of the mystic PvP rows (X8): the base
// rows HM EM DM OM (levels 1-19), then the first professions HW CL EW EO
// DW SO OS (20-39).
var mysticPvPClasses = []int32{10, 25, 38, 49, 11, 15, 26, 29, 39, 42, 50}

// checkMysticPvPRows checks the mystic PvP rows (docs/pvp/rows.md) against
// the shipped classes and skills, as checkRow does the M10 rows: for every
// role, each PvP step's skill is in the class tree from the row's "@N" and
// in the learn table; a Within gate is the skill's castRange (its radius
// for an aura, Stun Attack's padded reach); a weapon gate is the skill's
// weaponsAllowed; an Int step stuns, sleeps or deals damage; a Hold step
// is control; a Break step is control or a slow; a self step is no
// offensive skill; a poison is gated to casters and healers. Then the row
// table (pvpRowTable) at levels 10 and 40 against mysticRowTable.
func checkMysticPvPRows(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	for _, class := range mysticPvPClasses {
		c := classes[class]
		start, end := int32(1), int32(19)
		if c.Parent >= 0 {
			start, end = 20, 39
		}
		for _, role := range []Role{RoleDamage, RoleSupport, RoleCrafter} {
			kit := CompileBuild(c.Race, class, end, role, BranchChoice{}).Kit
			if len(kit.PvP) == 0 {
				t.Errorf("class %d %v: no PvP row", class, role)
			}
			for _, st := range kit.PvP {
				fail := func(format string, args ...any) {
					t.Errorf("class %d %v step %d: %s", class, role, st.Skill, fmt.Sprintf(format, args...))
				}
				lvl, ok := firstMinLevel(c, st.Skill)
				if !ok {
					fail("not in the class tree")
					continue
				}
				if want, got := max(start, lvl), max(start, st.From); got != want {
					fail("starts at %d, the tree grants it at %d", got, want)
				}
				if !slices.Contains(kit.Skills, st.Skill) {
					fail("not in the learn table %v", kit.Skills)
				}
				for _, g := range c.Skills {
					if g.ID != st.Skill || int32(g.MinLevel) > end {
						continue
					}
					checkPvPStep(st, skills[datapack.SkillRef{ID: g.ID, Level: int32(g.Level)}], fail)
				}
			}
		}
	}
	// ids are the PvP skills' ids by name: a RefStep keeps no id.
	ids := map[string]int32{}
	for _, class := range mysticPvPClasses {
		for _, st := range lineOrProfessionKit(class).PvP {
			ids[skills[datapack.SkillRef{ID: st.Skill, Level: 1}].Name] = st.Skill
		}
	}
	m := &TravelMap{classes: classes, skills: skills}
	for _, level := range []int{10, 40} {
		rows := pvpRowTable(m, shippedKits, level)
		for id, want := range mysticRowTable[level] {
			if got := rowSteps(rows[charmodel.Row{ID: id}], ids); got != want {
				t.Errorf("level %d row %s: steps\n%s\nwant\n%s", level, id, got, want)
			}
		}
	}
}

// checkPvPStep checks PvP step st against sk, one level of its skill.
func checkPvPStep(st Step, sk datapack.Skill, fail func(string, ...any)) {
	within := sk.CastRange
	switch {
	case st.Skill == skillStunAttack:
		within = stunAttackReach
	case within <= 0:
		within = sk.Radius
	}
	if st.Within != 0 && st.Within != within {
		fail("Within %d, the skill reaches %d", st.Within, within)
	}
	if len(sk.WeaponsAllowed) > 0 && !slices.Equal(slices.Sorted(slices.Values(st.Weapons)), slices.Sorted(slices.Values(sk.WeaponsAllowed))) {
		fail("weapon gate %v, the skill allows %v", st.Weapons, sk.WeaponsAllowed)
	}
	_, _, _, kites := spanOf(sk)
	switch {
	case st.Interrupt && !aborts(sk) && (!isDamageTypes[sk.Type] || sk.Power <= 0):
		fail("an Int step of type %s neither stuns, sleeps nor deals damage", sk.Type)
	case st.Hold && !isControl(sk):
		fail("a Hold step of type %s is no control", sk.Type)
	case st.Break && !isControl(sk) && !kites:
		fail("a Break step of type %s is neither control nor a slow", sk.Type)
	case st.Target == TargetSelf && sk.Offensive:
		fail("a self step of an offensive skill")
	case sk.Type == "POISON" && (len(st.Vs) == 0 || slices.ContainsFunc(st.Vs, func(a charmodel.Archetype) bool {
		return a != charmodel.Caster && a != charmodel.Healer
	})):
		fail("a poison gated vs %v, not to casters and healers", st.Vs)
	}
}

// lineOrProfessionKit is the kit table entry of class.
func lineOrProfessionKit(class int32) ClassKit {
	for _, table := range shippedKits {
		if k, ok := table[class]; ok {
			return k
		}
	}
	return ClassKit{}
}

// rowSteps is a row table row as "skill@level[vs]" words, each skill's id
// by its name in ids.
func rowSteps(steps []charmodel.RefStep, ids map[string]int32) string {
	w := make([]string, len(steps))
	for i, s := range steps {
		w[i] = fmt.Sprintf("%d@%d", ids[s.Skill.Name], s.Level)
		if len(s.Vs) > 0 {
			w[i] += fmt.Sprint(s.Vs)
		}
	}
	return strings.Join(w, " ")
}

// mysticRowTable is the mystic rows' row table at a level: each row's
// damage, DOT and control steps against a player (pvpSteps: the PvP row's,
// then the M10 steps of the skills it does not name) in that order, the
// level each starts at and its vs gate. Int, Break and side-cast steps,
// sustain buffs, Mental Shield, Chant of Fire and Aura Sink (MP damage)
// are left out.
var mysticRowTable = map[int]map[string]string{
	10: {
		"HM": "1184@7[M A] 1177@1[C H B] 1168@7[H] 1184@7[M] 1177@1",
		"EM": "1184@7 1184@7[M] 1177@1",
		"DM": "1168@7[C H] 1184@7[M] 1177@1",
		"OM": "1090@7 1100@7 1095@7[C H] 1100@7 1090@7",
		"CL": "1184@7[M] 1177@1",
	},
	40: {
		"HM": "1184@7[M A] 1177@1[C H B] 1164@14[M] 1168@7[H] 1184@7[M] 1177@1 1147@14",
		"EM": "1206@14[M] 1206@14[A] 1164@14[M] 1184@7 1184@7[M] 1177@1",
		"DM": "1206@14[M] 1206@14[A] 1168@7[C H] 1184@7[M] 1177@1 1147@14",
		"OM": "1090@7 1100@7 1095@7[C H] 1100@7 1090@7",
		"CL": "1201@25[M] 1184@7[M] 1177@1",
		"EO": "1206@14[M] 1201@25[M] 1184@7[M] 1177@1",
		"SO": "1147@14 1206@14[M] 1201@25[M] 1168@7[C H] 1184@7[M] 1177@1",
		"HW": "1184@7[M A] 1220@20[C H] 1160@35[M] 1222@35[M A] 1184@7[M] 1220@20 1274@20 1172@20 1147@14",
		"EW": "1206@14[M] 1206@14[A] 1264@25 1175@20 1184@7[M] 1264@25 1175@20 1274@20 1172@20",
		"DW": "1206@14[M] 1206@14[A] 1168@7[C H] 1266@25 1160@35[M] 1184@7[M] 1266@25 1178@20 1172@20 1147@14",
		"OS": "1090@7 1107@20 1107@20 1090@7 100@20",
	},
}

// The mystic PvP rows' opponents: the layer tests' Warrior (melee), Human
// Mystic (caster) and Cleric (healer), a Rogue with a bow (archer) and an
// Orc Mystic (battle caster).
const (
	rowRogue = int32(7)
	rowOrcMy = int32(49)
)

// mysticPvPSkills are the mystic PvP rows' skills at level 1 as
// data/xml/skills holds them (type, target, hit time, power, castRange,
// radius, weapons, effects): the row tests cast them on real ids.
var mysticPvPSkills = map[int32]datapack.Skill{
	skillStunAttack:     {Type: "PDAM", Target: "ONE", Offensive: true, HitTime: 1080 * time.Millisecond, Power: 30, CastRange: 40, WeaponsAllowed: []string{"BLUNT", "BIGBLUNT"}, Effects: []datapack.Effect{{Name: "Stun", Time: 9, Count: 1, EffectType: "STUN", EffectPower: 50}}},
	skillChantFire:      {Type: "BUFF", Target: "PARTY", Magic: true, HitTime: 2500 * time.Millisecond, Radius: 1000, Effects: []datapack.Effect{{Name: "Buff", StackType: "md_up", Time: 1200, Count: 1, EffectPower: -1}}},
	skillMentalShield:   {Type: "BUFF", Target: "ONE", Magic: true, HitTime: 4 * time.Second, CastRange: 400, Effects: []datapack.Effect{{Name: "Buff", StackType: "resist_derangement", Time: 1200, Count: 1, EffectPower: -1}}},
	skillLifeDrain:      {Type: "DRAIN", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 15, CastRange: 600},
	skillFear:           {Type: "FEAR", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 40, CastRange: 600, Effects: []datapack.Effect{{Name: "Fear", Time: 2, Count: 10, EffectPower: -1}}},
	skillVenom:          {Type: "POISON", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 70, CastRange: 600, Effects: []datapack.Effect{{Name: "DamOverTime", StackType: "poison", Time: 3, Count: 10, Value: 24, EffectPower: -1}}},
	skillDreamingSpirit: {Type: "SLEEP", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 80, CastRange: 600, Effects: []datapack.Effect{{Name: "Sleep", Time: 30, Count: 1, EffectPower: -1}}},
	skillChillFlame:     {Type: "DOT", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 70, CastRange: 600, Effects: []datapack.Effect{{Name: "DamOverTime", StackType: "dot_attr", Time: 1, Count: 15, Value: 20, EffectPower: -1}}},
	skillAuraSink:       {Type: "MANADAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 16, CastRange: 600, Effects: []datapack.Effect{{Name: "ManaDamOverTime", StackType: "dot_mp", Time: 3, Count: 10, Value: 12, EffectType: "MDOT", EffectPower: 80}}},
	skillFrostFlame:     {Type: "DOT", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 70, CastRange: 750, Effects: []datapack.Effect{{Name: "DamOverTime", StackType: "dot_attr", Time: 1, Count: 15, Value: 44, EffectPower: -1}}},
	skillVampiricTouch:  {Type: "DRAIN", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 18, CastRange: 600},
	skillSlow:           {Type: "DEBUFF", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 80, CastRange: 600, Effects: []datapack.Effect{{Name: "Debuff", StackType: "speed_down", Time: 60, Count: 1, EffectPower: -1, Funcs: []datapack.StatFunc{{Op: "mul", Stat: "runSpd", Value: 0.7}}}}},
	skillCurseChaos:     {Type: "DEBUFF", Target: "ONE", Magic: true, Offensive: true, HitTime: 1500 * time.Millisecond, Power: 80, CastRange: 600, Effects: []datapack.Effect{{Name: "Debuff", StackType: "hit_down", Time: 15, Count: 1, EffectPower: -1, Funcs: []datapack.StatFunc{{Op: "sub", Stat: "accCombat", Value: 12}}}}},
	skillCurseWeakness:  {Type: "DEBUFF", Target: "ONE", Magic: true, Offensive: true, HitTime: 1500 * time.Millisecond, Power: 80, CastRange: 600, Effects: []datapack.Effect{{Name: "Debuff", StackType: "pa_down", Time: 15, Count: 1, EffectPower: -1, Funcs: []datapack.StatFunc{{Op: "mul", Stat: "pAtk", Value: 0.83}}}}},
	skillCursePoison:    {Type: "POISON", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 70, CastRange: 600, Effects: []datapack.Effect{{Name: "DamOverTime", StackType: "poison", Time: 3, Count: 10, Value: 24, EffectPower: -1}}},
	skillAuraBurn:       {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 1500 * time.Millisecond, Power: 19, CastRange: 150},
	skillAquaSwirl:      {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 23, CastRange: 750},
	skillWindStrike:     {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 12, CastRange: 600},
	skillTwister:        {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 23, CastRange: 750},
	skillIceBolt:        {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 3100 * time.Millisecond, Power: 8, CastRange: 600, Effects: []datapack.Effect{{Name: "Debuff", StackType: "speed_down", Time: 120, Count: 1, EffectType: "DEBUFF", EffectPower: 60, Funcs: []datapack.StatFunc{{Op: "mul", Stat: "runSpd", Value: 0.7}}}}},
	skillDryadRoot:      {Type: "ROOT", Target: "ONE", Magic: true, Offensive: true, HitTime: 2500 * time.Millisecond, Power: 80, CastRange: 600, Effects: []datapack.Effect{{Name: "Root", StackType: "root_magically", Time: 30, Count: 1, EffectPower: -1}}},
	skillTrance:         {Type: "SLEEP", Target: "ONE", Magic: true, Offensive: true, Debuff: true, HitTime: 3 * time.Second, Power: 80, CastRange: 600, Effects: []datapack.Effect{{Name: "Sleep", StackType: "sleep", Time: 15, Count: 1, EffectPower: -1}}},
	skillWindShackle:    {Type: "DEBUFF", Target: "ONE", Magic: true, Offensive: true, HitTime: 1500 * time.Millisecond, Power: 80, CastRange: 600, Effects: []datapack.Effect{{Name: "Debuff", StackType: "attack_time_up", Time: 15, Count: 1, EffectPower: -1, Funcs: []datapack.StatFunc{{Op: "mul", Stat: "pAtkSpd", Value: 0.83}}}}},
	skillSealBinding:    {Type: "ROOT", Target: "AURA", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 40, Radius: 200, Effects: []datapack.Effect{{Name: "Root", StackType: "root_magically", Time: 30, Count: 1, EffectPower: -1}}},
	skillSelfHeal:       {Type: "HEAL", Target: "SELF", Magic: true, HitTime: 5 * time.Second, Power: 42},
	skillBlaze:          {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 4 * time.Second, Power: 23, CastRange: 750},
	skillSolarSpark:     {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 5 * time.Second, Power: 39, CastRange: 750},
	skillShadowSpark:    {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 5 * time.Second, Power: 39, CastRange: 750},
	skillEnergyBolt:     {Type: "MDAM", Target: "ONE", Magic: true, Offensive: true, HitTime: 2 * time.Second, Power: 13, CastRange: 400},
	skillBodyToMind:     {Type: "MANAHEAL", Target: "SELF", Magic: true, HitTime: 4 * time.Second, Power: 22},
}

// mysticRowBrain is layerBrain for a bot of class at level in the Damage
// role with its real compiled build, knowing the skills of its PvP and
// rotation steps at level 1 with their datapack entries (mysticPvPSkills)
// and nothing else (no buff, toggle or heal goes first), against pcID of
// class opp (a bow in its right hand when bow) 300 away.
func mysticRowBrain(t *testing.T, class, level, opp int32, bow bool) *Brain {
	t.Helper()
	b := layerBrain(t, opp)
	m := b.cfg.Travel
	m.classes[rowRogue], m.classes[rowOrcMy] = layerClass(rowRogue, 0), layerClass(rowOrcMy, -1)
	if m.castRanges == nil {
		m.castRanges = make(map[datapack.SkillRef]int32)
	}
	for id, s := range mysticPvPSkills {
		ref := datapack.SkillRef{ID: id, Level: 1}
		m.skills[ref] = s
		if s.CastRange > 0 {
			m.castRanges[ref] = s.CastRange
		}
	}
	s := &b.world.Self
	s.ClassID, s.Level = class, level
	b.persona.Role = RoleDamage
	b.builtOK = false
	k := b.build().Kit
	var known []KnownSkill
	for _, st := range slices.Concat(k.PvP, k.Rotation) {
		known = append(known, KnownSkill{ID: st.Skill, Level: 1})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	b.handleOne(t, rowFoe(opp, bow, Point{Y: 300}, 0), t0)
	return b
}

// rowFoe is pcID's CharInfo as a player of class (with a bow when bow) at
// p showing the abnormal mask.
func rowFoe(class int32, bow bool, p Point, abnormal int32) CharInfo {
	e := CharInfo{ObjectID: pcID, Name: foeName, ClassID: class, Pos: p, RunSpeed: 124, Running: true, PvPFlag: 1, Abnormal: abnormal, CollisionRadius: 9}
	if bow {
		e.RHand = pvpBow
	}
	return e
}
