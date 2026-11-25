package charmodel

import (
	"math"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Fighter is one side of a player fight (docs/pvp/model.md): the bot, or
// the reference opponent of a row (its class template at the bot's level,
// grade-typical gear, no dyes).
type Fighter struct {
	Profile Profile
	// MaxCP is the CP bar a player loses before its HP.
	MaxCP float64
	Level int
	// CON and MEN are the final base stats landing rolls and cast breaks
	// read.
	CON, MEN int
	// Row is the behavior row the fighter plays; its archetype is what the
	// other side's Vs gates see.
	Row Row
	// Shot is what its attacks and casts are charged with.
	Shot Shot
	// Mul are the multiplier stats by the server's stat name: the landing
	// vulnerabilities (stunVuln, sleepVuln, ...) and the PvP damage bonuses
	// (pvpPhysDmg, pvpPhysSkillsDmg, pvpMagicalDmg). A stat it lacks is 1.
	Mul map[string]float64
}

// mulStats are the stats a Fighter's Mul holds (skill/stat/stat.go names).
var mulStats = []string{
	"stunVuln", "sleepVuln", "rootVuln", "paralyzeVuln", "poisonVuln", "bleedVuln",
	"debuffVuln", "derangementVuln", "cancelVuln",
	"pvpPhysDmg", "pvpPhysSkillsDmg", "pvpMagicalDmg",
}

// mul is the fighter's multiplier stat name, 1 when it has none.
func (f Fighter) mul(name string) float64 {
	if v, ok := f.Mul[name]; ok {
		return v
	}
	return 1
}

// ModelFighter derives a fighter from b through the stat pipeline Model
// runs: the profile, max CP (the class's CP at its level times the CON
// bonus, skill/funcs/vitals.go MaxCpMul), the final CON and MEN and the
// multiplier stats of its gear, sets and buffs in b.Funcs (a jewel's
// addMul v leaves 1 - v/100). The caller sets Row and Shot.
func ModelFighter(b Build) Fighter {
	p, a, c := derive(b)
	level := max(b.Level, 1)
	f := Fighter{
		Profile: p,
		MaxCP:   math.Trunc(c.stat("maxCp", tableAt(b.Class.CPTable, level), mul(CONBonus(a.CON)))),
		Level:   level,
		CON:     a.CON,
		MEN:     a.MEN,
	}
	for _, name := range mulStats {
		// The vulnerabilities start from the elemental term, 1 for the
		// element-less skills the rows cast (formula.go
		// ElementalSkillModifier).
		if v := c.stat(name, 1, nil); v != 1 {
			if f.Mul == nil {
				f.Mul = make(map[string]float64)
			}
			f.Mul[name] = v
		}
	}
	return f
}

// LandRate is the percent chance skill s cast by caster lands on target
// (skill/formulas/skillsuccess.go SkillSuccessRate, model/actor/creature/
// formula.go resolveLandingInput): base x stat x vuln x matk x level, held
// to 1..99; the server lands it when an integer roll in [0, 100) is under
// it. A perfect shield block and the skills that ignore resists are out of
// the model.
func LandRate(s datapack.Skill, caster, target Fighter) float64 {
	return landRate(s, landingPower(s), landingType(s), caster, target)
}

// landRate is LandRate from base resisted as typ.
func landRate(s datapack.Skill, base float64, typ string, caster, target Fighter) float64 {
	rate := base * statModifier(target, typ, s.Magic) * vulnerability(target, typ) *
		matkModifier(s, caster, target) * levelModifier(s, caster.Level, target.Level)
	return min(max(rate, 1), 99)
}

// landingPower is the base chance a skill's own landing roll starts from
// (model/skill/landing.go LandingPower): the first positive effect power
// of its effects, else its positive effectPower, else 20 for PDAM and
// MDAM, else its power in (0, 100], else 20.
func landingPower(s datapack.Skill) float64 {
	for _, e := range s.Effects {
		if e.EffectPower > 0 {
			return e.EffectPower
		}
	}
	if s.EffectPower > 0 {
		return float64(s.EffectPower)
	}
	switch typeKey(s.Type) {
	case "PDAM", "MDAM":
		return 20
	}
	if s.Power <= 0 || s.Power > 100 {
		return 20
	}
	return s.Power
}

// landingType is the type a skill's own landing roll resists as
// (LandingEffectType): the first effect type of its effects, else its
// effectType, else STUN for PDAM and PARALYZE for MDAM, else its skill
// type.
func landingType(s datapack.Skill) string {
	for _, e := range s.Effects {
		if e.EffectType != "" {
			return e.EffectType
		}
	}
	if s.EffectType != "" {
		return s.EffectType
	}
	switch typeKey(s.Type) {
	case "PDAM":
		return "STUN"
	case "MDAM":
		return "PARALYZE"
	}
	return s.Type
}

// effectLand is the percent chance effect e of skill s lands
// (ResolveEffectSuccessInput): an effect without its own effect power
// lands with the skill's roll, one without a type rolls its bare power, a
// CANCEL always lands, any other resists as its type.
func effectLand(s datapack.Skill, e datapack.Effect, caster, target Fighter) float64 {
	switch {
	case e.EffectPower < 0:
		return LandRate(s, caster, target)
	case e.EffectType == "":
		return e.EffectPower
	case typeKey(e.EffectType) == "CANCEL":
		return 100
	}
	return landRate(s, e.EffectPower, e.EffectType, caster, target)
}

// typeKey is a skill or effect type as the server compares it
// (formula.go SkillTypeKey).
func typeKey(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// statModifier is the target's base stat term (formula.go
// SkillStatModifier): 2 - sqrt(CON bonus) for a stun, bleed or poison,
// 2 - sqrt(MEN bonus) for a magic sleep, debuff and the like, at least 0;
// else 1.
func statModifier(target Fighter, typ string, magic bool) float64 {
	switch typeKey(typ) {
	case "STUN", "BLEED", "POISON":
		return max(0, 2-math.Sqrt(CONBonus(target.CON)))
	case "SLEEP", "DEBUFF", "WEAKNESS", "ERASE", "ROOT", "MUTE", "FEAR", "BETRAY", "CONFUSION", "AGGREDUCE_CHAR", "PARALYZE":
		if magic {
			return max(0, 2-math.Sqrt(MENBonus(target.MEN)))
		}
	}
	return 1
}

// vulnStats are the vulnerability stat each landing type resists through
// (formula.go SkillVulnerability).
var vulnStats = map[string]string{
	"BLEED": "bleedVuln", "POISON": "poisonVuln", "STUN": "stunVuln",
	"PARALYZE": "paralyzeVuln", "ROOT": "rootVuln", "SLEEP": "sleepVuln",
	"MUTE": "derangementVuln", "FEAR": "derangementVuln", "BETRAY": "derangementVuln",
	"AGGDEBUFF": "derangementVuln", "AGGREDUCE_CHAR": "derangementVuln",
	"ERASE": "derangementVuln", "CONFUSION": "derangementVuln",
	"DEBUFF": "debuffVuln", "WEAKNESS": "debuffVuln", "CANCEL": "cancelVuln",
}

// vulnerability is the target's vulnerability to typ: its matching stat
// over the elemental term (1 here), 1 for a type none matches.
func vulnerability(target Fighter, typ string) float64 {
	if name, ok := vulnStats[typeKey(typ)]; ok {
		return target.mul(name)
	}
	return 1
}

// matkModifier is the caster's M.Atk against the target's M.Def for a magic
// skill (formula.go SkillMAtkModifier): 11 x sqrt(M.Atk) / M.Def, M.Atk
// x4 with a blessed spiritshot; 1 for any other skill.
func matkModifier(s datapack.Skill, caster, target Fighter) float64 {
	if !s.Magic {
		return 1
	}
	mAtk := caster.Profile.MAtk
	if caster.Shot == BlessedSpiritshot {
		mAtk *= 4
	}
	return math.Sqrt(mAtk) / max(target.Profile.MDef, 1) * 11
}

// levelModifier is the level term (formula.go SkillLevelModifier): with a
// lvlDepend, 1 + 0.005 x delta (0.01 below 0), delta the skill's magicLvl
// (or the caster's level) plus lvlDepend minus the target's level; 1
// without.
func levelModifier(s datapack.Skill, casterLevel, targetLevel int) float64 {
	if s.LevelDepend == 0 {
		return 1
	}
	level := casterLevel
	if s.MagicLevel > 0 {
		level = s.MagicLevel
	}
	delta := level + s.LevelDepend - targetLevel
	scale := 0.005
	if delta < 0 {
		scale = 0.01
	}
	return 1 + scale*float64(delta)
}

// CastBreakChance is the percent chance a hit of damage breaks the magic
// cast of a caster of MEN men inside its interrupt window
// (skill/formulas/cast.go CastBreakRate, CastBreaks): 15 + sqrt(13 x
// damage) - (MEN bonus x 100 - 100), held to 1..99.
func CastBreakChance(damage float64, men int) float64 {
	rate := 15 + math.Sqrt(13*max(damage, 0)) - (MENBonus(men)*100 - 100)
	return min(max(rate, 1), 99)
}
