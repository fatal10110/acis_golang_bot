package charmodel

import (
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Profile is a character's or a monster's final combat stats, in the units
// UserInfo reports them: critical rate in per mille, speeds as the client
// shows them.
type Profile struct {
	MaxHP, MaxMP float64
	PAtk, MAtk   float64
	// PAtkSpd is the attack speed, MAtkSpd the cast speed.
	PAtkSpd, MAtkSpd float64
	Critical         float64
	Accuracy         float64
	Evasion          float64
	PDef, MDef       float64
	RunSpeed         float64
	WeightLimit      float64
}

// The creature defaults the server uses where neither the template nor a
// weapon sets the stat (player/character_attack.go): attack speed, cast
// speed and critical rate.
const (
	defaultPAtkSpd  = 300
	defaultMAtkSpd  = 333
	defaultCritical = 4
	// baseWeightLimit is a player's weight limit before CON
	// (character_weight_penalty.go), at the server's default multiplier of 1
	// (players.properties WeightLimit).
	baseWeightLimit = 69000
	// maxCritical caps the critical rate (CriticalRate: min(int(stat), 500)).
	maxCritical = 500
	// maxDyeStat caps what dyes add to one base stat (henna/list.go).
	maxDyeStat = 5
)

// Build is everything the model derives a character's profile from.
type Build struct {
	Class datapack.Class
	Level int
	// Mage selects the mystic's smaller armor penalties on P.Def (the
	// server's ClassMage; see MageClass).
	Mage bool
	// Worn are the equipped items: the weapon (rhand or lrhand), armor and
	// jewelry. An empty right hand fights with the creature defaults; pass
	// the class's fists to model them exactly.
	Worn []datapack.Item
	// Funcs are the stat functions always in force: the learned passive
	// skills' (masteries, Spellcraft) and the complete sets' skills'.
	// Functions of a condition the model cannot judge (OtherCond) are left
	// out; <using kind> conditions are judged against Worn.
	Funcs []datapack.StatFunc
	// Dyes are the base stat changes of the drawn hennas, summed; each stat
	// counts at most +5, as on the server.
	Dyes Attrs
}

// MageClass reports whether class id is of one of the four mystic lines
// (player/race.go ClassMage): its root, followed through Parent in
// classes, is 10, 25, 38 or 49.
func MageClass(classes map[int32]datapack.Class, id int32) bool {
	for range len(classes) + 1 {
		c, ok := classes[id]
		if !ok {
			return false
		}
		if c.Parent < 0 {
			return id == 10 || id == 25 || id == 38 || id == 49
		}
		id = c.Parent
	}
	return false
}

// Model derives b's profile through the server's stat pipeline: the class
// base or the weapon's value, the armor and jewelry added to it, the
// finalize step by base stats and level, then the passive and set skills.
// Enchantment and grade penalties are not modeled; calibration against the
// reported stats absorbs them for the current gear.
func Model(b Build) Profile {
	kinds := wornKinds(b.Worn)
	funcs := append(itemFuncs(b.Worn), b.Funcs...)
	c := calc{funcs: funcs, kinds: kinds}
	cl := b.Class
	a := Attrs{
		STR: c.attr("STR", cl.STR, b.Dyes.STR),
		CON: c.attr("CON", cl.CON, b.Dyes.CON),
		DEX: c.attr("DEX", cl.DEX, b.Dyes.DEX),
		INT: c.attr("INT", cl.INT, b.Dyes.INT),
		WIT: c.attr("WIT", cl.WIT, b.Dyes.WIT),
		MEN: c.attr("MEN", cl.MEN, b.Dyes.MEN),
	}
	level := max(b.Level, 1)
	lm := LevelMod(level)
	pDefPenalty, mDefPenalty := armorPenalties(b.Worn, b.Mage)

	var p Profile
	p.MaxHP = math.Trunc(c.stat("maxHp", tableAt(cl.HPTable, level), mul(CONBonus(a.CON))))
	p.MaxMP = math.Trunc(c.stat("maxMp", tableAt(cl.MPTable, level), mul(MENBonus(a.MEN))))
	p.PAtk = math.Trunc(c.stat("pAtk", positive(cl.PAtk), func(v float64) float64 { return v * STRBonus(a.STR) * lm }))
	p.MAtk = math.Trunc(c.stat("mAtk", positive(cl.MAtk), mul((lm*lm)*(INTBonus(a.INT)*INTBonus(a.INT)))))
	p.PAtkSpd = math.Trunc(c.stat("pAtkSpd", defaultPAtkSpd, mul(DEXBonus(a.DEX))))
	p.MAtkSpd = math.Trunc(c.stat("mAtkSpd", defaultMAtkSpd, mul(WITBonus(a.WIT))))
	p.Critical = min(math.Trunc(c.stat("rCrit", defaultCritical, func(v float64) float64 { return v * DEXBonus(a.DEX) * 10 })), maxCritical)
	evasionAccuracy := func(v float64) float64 { return v + EvasionAccuracy(a.DEX) + float64(level) }
	p.Accuracy = math.Trunc(c.stat("accCombat", 0, evasionAccuracy))
	p.Evasion = math.Trunc(c.stat("rEvas", 0, evasionAccuracy))
	p.PDef = math.Trunc(c.stat("pDef", positive(cl.PDef), func(v float64) float64 { return (v - pDefPenalty) * lm }))
	p.MDef = math.Trunc(c.stat("mDef", positive(cl.MDef), func(v float64) float64 { return (v - mDefPenalty) * MENBonus(a.MEN) * lm }))
	p.RunSpeed = c.stat("runSpd", cl.RunSpeed, mul(DEXBonus(a.DEX)))
	p.WeightLimit = math.Trunc(c.stat("weightLimit", baseWeightLimit*CONBonus(a.CON), nil))
	return p
}

// Monster derives an NPC's final stats from its template's bases, base
// stats and level through the server's NPC finalize step
// (npc/hostile_stats.go defaultBuiltin): an NPC wears no equipment
// penalties and learns no passives the model knows. MaxMP and WeightLimit
// are 0: the template's MP is not read and a monster carries nothing.
func Monster(n datapack.NPC) Profile {
	s := n.Combat
	lm := LevelMod(n.Level)
	level := float64(max(n.Level, 1))
	return Profile{
		MaxHP:    math.Trunc(s.HP * CONBonus(s.CON)),
		PAtk:     math.Trunc(s.PAtk * STRBonus(s.STR) * lm),
		MAtk:     math.Trunc(positive(s.MAtk) * ((lm * lm) * (INTBonus(s.INT) * INTBonus(s.INT)))),
		PAtkSpd:  math.Trunc(s.AtkSpd * DEXBonus(s.DEX)),
		MAtkSpd:  math.Trunc(defaultMAtkSpd * WITBonus(s.WIT)),
		Critical: min(math.Trunc(s.Crit*DEXBonus(s.DEX)*10), maxCritical),
		Accuracy: math.Trunc(EvasionAccuracy(s.DEX) + level),
		Evasion:  math.Trunc(EvasionAccuracy(s.DEX) + level),
		PDef:     math.Trunc(s.PDef * lm),
		MDef:     math.Trunc(positive(s.MDef) * MENBonus(s.MEN) * lm),
		RunSpeed: math.Trunc(n.RunSpeed * DEXBonus(s.DEX)),
	}
}

// The calculation phases of the server's stat pipeline (skill/effect/mod.go
// Op.order), lowest first: set, basemul, baseadd, enchant, the builtin
// finalize step, mul and div, add and sub, addMul and subDiv.
const (
	finalizePhase = 4
	lastPhase     = 7
)

// phaseOf is the phase of op, -1 for an op the pipeline does not know.
func phaseOf(op string) int {
	switch op {
	case "set":
		return 0
	case "basemul":
		return 1
	case "baseadd":
		return 2
	case "enchant":
		return 3
	case "mul", "div":
		return 5
	case "add", "sub":
		return 6
	case "addMul", "subDiv":
		return 7
	}
	return -1
}

// calc runs the stat pipeline of one character.
type calc struct {
	funcs []datapack.StatFunc
	kinds map[string]bool
}

// stat runs the pipeline for stat name from base, with finalize (nil for
// none) as the server's builtin step.
func (c calc) stat(name string, base float64, finalize func(float64) float64) float64 {
	v := base
	for i := range lastPhase + 1 {
		if i == finalizePhase {
			if finalize != nil {
				v = finalize(v)
			}
			continue
		}
		for _, f := range c.funcs {
			if f.Stat != name || phaseOf(f.Op) != i || !c.applies(f) {
				continue
			}
			v = apply(f, base, v)
		}
	}
	return v
}

// attr is a final base stat: the class's, the dyes' at finalize, then the
// skills'.
func (c calc) attr(name string, base, dye int) int {
	d := float64(min(dye, maxDyeStat))
	return int(c.stat(name, float64(base), add(d)))
}

// applies judges f's condition against the worn kinds: a plain <using
// kind> needs one of them worn, a negated one none; any other condition is
// out of the model.
func (c calc) applies(f datapack.StatFunc) bool {
	if f.OtherCond {
		return false
	}
	if len(f.Using) == 0 {
		return true
	}
	wearing := false
	for _, k := range f.Using {
		if c.kinds[k] {
			wearing = true
			break
		}
	}
	return wearing != f.NotUsing
}

// apply is one function's arithmetic (skill/effect/mod.go apply). An
// enchant function adds by the item's enchant level, which the model does
// not know: it adds nothing.
func apply(f datapack.StatFunc, base, v float64) float64 {
	switch f.Op {
	case "set":
		return f.Value
	case "basemul":
		return v + base*f.Value
	case "baseadd", "add":
		return v + f.Value
	case "mul":
		return v * f.Value
	case "div":
		return v / f.Value
	case "sub":
		return v - f.Value
	case "addMul":
		return v * (1 - f.Value/100)
	case "subDiv":
		return v / (1 - f.Value/100)
	}
	return v
}

// wornKinds is the set of kinds a <using kind> condition sees
// (itemcontainer/inventory.go setPaperdollItemLocked): the weapon's type, a
// full-body armor's or any other piece's armor type, and a chest's or
// legs' armor type only while the other one is of the same type.
func wornKinds(worn []datapack.Item) map[string]bool {
	kinds := make(map[string]bool)
	var chest, legs string
	for _, it := range worn {
		switch {
		case it.Kind == "":
		case it.BodyPart == "chest":
			chest = it.Kind
		case it.BodyPart == "legs":
			legs = it.Kind
		default:
			kinds[it.Kind] = true
		}
	}
	if chest != "" && chest == legs {
		kinds[chest] = true
	}
	return kinds
}

// itemFuncs are the worn items' stats as the pipeline functions their
// templates are: a weapon sets its values, armor and jewelry add theirs to
// the base.
func itemFuncs(worn []datapack.Item) []datapack.StatFunc {
	var out []datapack.StatFunc
	for _, it := range worn {
		op := "baseadd"
		if isWeapon(it) {
			op = "set"
		}
		s := it.Stats
		for _, f := range []struct {
			stat string
			v    float64
		}{
			{"pAtk", s.PAtk},
			{"mAtk", s.MAtk},
			{"pDef", s.PDef},
			{"mDef", s.MDef},
			{"pAtkSpd", s.PAtkSpd},
			{"rCrit", s.RCrit},
		} {
			if f.v != 0 {
				out = append(out, datapack.StatFunc{Op: op, Stat: f.stat, Value: f.v})
			}
		}
	}
	return out
}

func isWeapon(it datapack.Item) bool {
	return it.Type == "Weapon" && (it.BodyPart == "rhand" || it.BodyPart == "lrhand")
}

// armorPenalties are what the worn pieces take off a player's P.Def and
// M.Def before the level factor (skill/funcs/attack.go PDefMod, MDefMod):
// head 12, chest 31 (a mage's 15), legs or a full-body armor 18 (8),
// gloves 8, feet 7; each earring 9, each ring 5, the necklace 13.
func armorPenalties(worn []datapack.Item, mage bool) (pDef, mDef float64) {
	chest, legs := 31.0, 18.0
	if mage {
		chest, legs = 15, 8
	}
	var ears, fingers int
	for _, it := range worn {
		switch it.BodyPart {
		case "head":
			pDef += 12
		case "chest":
			pDef += chest
		case "fullarmor":
			pDef += chest + legs
		case "legs":
			pDef += legs
		case "gloves":
			pDef += 8
		case "feet":
			pDef += 7
		case "rear;lear":
			ears++
		case "rfinger;lfinger":
			fingers++
		case "neck":
			mDef += 13
		}
	}
	mDef += 9*float64(min(ears, 2)) + 5*float64(min(fingers, 2))
	return pDef, mDef
}

// tableAt is a class's per-level table at level, 0 past its end.
func tableAt(t []float64, level int) float64 {
	if level < 1 || level > len(t) {
		return 0
	}
	return t[level-1]
}

// positive is the server's guard on a template base: 1 where the template
// has none.
func positive(v float64) float64 {
	if v > 0 {
		return v
	}
	return 1
}

func mul(m float64) func(float64) float64 {
	return func(v float64) float64 { return v * m }
}

func add(a float64) func(float64) float64 {
	return func(v float64) float64 { return v + a }
}
