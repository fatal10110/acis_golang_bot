package datapack

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// Skill is the part of one level of a skill template (skills/*.xml) bots
// weigh potions, buffs and builds with, and cast it from: whether it is a
// potion's, how and on what it is cast, what it costs, the charges it needs,
// the effects it lands, with their stack types, and the stat functions it
// applies.
type Skill struct {
	Name string
	// Type is skillType (BUFF, CHARGEDAM, CONT, ...), OperateType is
	// operateType (ACTIVE, PASSIVE, TOGGLE, ...) and Target is target
	// (TARGET_ONE, TARGET_SELF, ...), each as written; "" when absent,
	// which the server refuses of a shipped skill.
	Type, OperateType, Target string
	// WeaponsAllowed are the weapon kinds of weaponsAllowed (DUALFIST,
	// SWORD, ...), split on "," and trimmed as the server's mask reads them;
	// nil when the skill casts with any weapon.
	WeaponsAllowed []string
	// HitTime and ReuseDelay are hitTime and reuseDelay (default 0): how
	// long the cast takes and how long until the skill may be cast again.
	HitTime, ReuseDelay time.Duration
	// MPConsume is mpConsume (default 0), the MP a cast costs.
	MPConsume int
	// Power is power (default 0), the skill's base damage or heal.
	Power float64
	// NumCharges is numCharges, the charges a cast spends (or a charging
	// skill adds), and MaxCharges maxCharges, the most a charging skill
	// holds; both default to 0.
	NumCharges, MaxCharges int
	// Charges is the skill's <cond><player Charges/></cond>, the zero value
	// for none.
	Charges ChargeCond
	// Potion is isPotion: the skill of a potion etc item.
	Potion bool
	// CastRange is castRange (default 0): how far from its target the
	// server casts the skill without walking the caster first, before
	// both collision radii are added (network/magic_skill.go
	// walkToCastTarget). A negative one takes no approach at all.
	CastRange int32
	// Effects are the effects of the skill's <for> blocks at this level,
	// in file order; nil for none. The effects only an enchanted skill
	// level lands (<enchant1for>, <enchant2for>) are not read.
	Effects []Effect
	// Funcs are the stat functions directly in the skill's <for> blocks at
	// this level (a passive's, a set skill's), in file order; nil for none.
	// The functions an effect applies are not read.
	Funcs []StatFunc
}

// ChargeCond is a skill's Charges condition: <player Charges="Min"/> lets
// the cast through with at least Min charges (skill/conditions Charges);
// under <not> (Not) it refuses the cast from Min charges up, as a charging
// skill at its maximum. Min 0 means the skill has no such condition.
type ChargeCond struct {
	Min int
	Not bool
}

// StatFunc is one stat function (<add>, <mul>, ...) of a skill level: Op
// applies Value to Stat.
type StatFunc struct {
	// Op is the element name, spelled as the server's funcOpTags (add,
	// addMul, sub, subDiv, mul, basemul, div, set, enchant, baseadd).
	Op    string
	Stat  string
	Value float64
	// Using are the item kinds (SWORD, LIGHT, MAGIC, ...) of the function's
	// <using kind> condition: it applies only while an equipped item is of
	// one of them, or with NotUsing (<not><using/></not>) only while none
	// is. nil for no such condition.
	Using    []string
	NotUsing bool
	// OtherCond is set when the function's condition is anything but a
	// plain or negated <using kind> (a <player hp>, a <game night>, an
	// <and>): the reader keeps no detail of it, so the function applies
	// only some of the time.
	OtherCond bool
}

// Effect is one <effect> of a skill level.
type Effect struct {
	// Name is the effect implementation: Buff, HealOverTime, ...
	Name string
	// StackType and StackOrder decide what lands over what: of two
	// effects of one stack type the server keeps the one of the higher
	// order. StackType is "none" for an effect that stacks with any.
	StackType  string
	StackOrder float64
	// Time is the effect's time attribute and Count its count: a buff
	// lasts Time seconds; an over-time effect ticks Count times, Time
	// seconds apart.
	Time, Count int
	// Value is the effect's val: what an over-time heal restores a tick.
	Value float64
}

type skillFile struct {
	Skills []skillElement `xml:"skill"`
}

// skillElement is one <skill>: its attributes, its value tables, its direct
// <set> children, its <cond> children and its <for> blocks. <enchant1>,
// <enchant2>, <enchant1for> and <enchant2for> are not read.
type skillElement struct {
	Attrs  []xml.Attr `xml:",any,attr"`
	Tables []struct {
		Name string `xml:"name,attr"`
		Text string `xml:",chardata"`
	} `xml:"table"`
	Sets  []setElement `xml:"set"`
	Conds []xmlNode    `xml:"cond"`
	For   []struct {
		Nodes []xmlNode `xml:",any"`
	} `xml:"for"`
}

// xmlNode is any element with its attributes and child elements.
type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []xmlNode  `xml:",any"`
}

// statFuncOps are the stat function element names, spelled as the
// server's funcOpTags (model/skill/template.go); the server matches them
// case-insensitively and skips any other element of a <for> block.
var statFuncOps = []string{"add", "addMul", "sub", "subDiv", "mul", "basemul", "div", "set", "enchant", "baseadd"}

// LoadSkills reads every *.xml skill template file directly under dir, in
// sorted file order, into one Skill per regular level (1 to levels; the
// enchanted levels 101 and up are not read). A value a level reads may
// name a <table> ("#name"), whose n-th entry is level n's value, as on the
// server (data/xml/tables.go resolveTableValue); stackType is read as
// written. A file that is not well-formed, or a skill missing or mangling a
// value bots read, an undefined table or one without a row for a level,
// fails the whole load. A level declared twice keeps its last declaration.
func LoadSkills(dir string) (map[SkillRef]Skill, error) {
	paths, err := xmlFiles(dir, "skill")
	if err != nil {
		return nil, err
	}
	skills := make(map[SkillRef]Skill)
	for _, path := range paths {
		var f skillFile
		if err := decodeFile(path, &f); err != nil {
			return nil, err
		}
		for _, el := range f.Skills {
			if err := buildSkill(el, skills); err != nil {
				return nil, fmt.Errorf("datapack: %s: %w", path, err)
			}
		}
	}
	return skills, nil
}

// buildSkill reads every regular level of el into skills: id, levels and
// name are required, isPotion defaults to false, castRange, hitTime,
// reuseDelay, mpConsume, power, numCharges and maxCharges to 0 and
// weaponsAllowed to none (data/xml/skill.go buildSkillDefinitionAttrs);
// skillType, operateType and target, which the server requires, read as ""
// when absent; an effect's name is required, its stackType defaults to
// "none", stackOrder to 0, time and count to 1 (data/xml/effectgrammar.go
// readEffectTemplate); a stat function's stat and val are required
// (data/xml/skill.go funcTemplate).
func buildSkill(el skillElement, skills map[SkillRef]Skill) error {
	a := foldAttrs(el.Attrs)
	id, levels, name := a.int32("id"), a.int("levels"), a.str("name")
	if a.err == nil && levels < 0 {
		a.fail("levels", fmt.Errorf("%d: negative level count", levels))
	}
	if a.err != nil {
		return fmt.Errorf("skill %s: %w", a.strDefault("id", "?"), a.err)
	}
	tables := make(map[string][]string, len(el.Tables))
	for _, t := range el.Tables {
		if !strings.HasPrefix(t.Name, "#") {
			return fmt.Errorf("skill %d: table name %q must start with '#'", id, t.Name)
		}
		tables[t.Name] = strings.FieldsFunc(t.Text, isTableSpace)
	}
	sets := make(map[string]string, len(el.Sets))
	for _, s := range el.Sets {
		sets[s.Name] = s.Val
	}
	for level := 1; level <= levels; level++ {
		res := resolver{tables: tables, row: level}
		r := res.reader(sets, "isPotion", "castRange", "skillType", "operateType", "target", "weaponsAllowed",
			"hitTime", "reuseDelay", "mpConsume", "power", "numCharges", "maxCharges")
		sk := Skill{
			Name: name, Potion: r.boolDefault("isPotion", false), CastRange: r.int32Default("castRange", 0),
			Type: r.strDefault("skillType", ""), OperateType: r.strDefault("operateType", ""), Target: r.strDefault("target", ""),
			WeaponsAllowed: splitKinds(r.strDefault("weaponsAllowed", "")),
			HitTime:        time.Duration(r.intDefault("hitTime", 0)) * time.Millisecond,
			ReuseDelay:     time.Duration(r.intDefault("reuseDelay", 0)) * time.Millisecond,
			MPConsume:      r.intDefault("mpConsume", 0), Power: r.floatDefault("power", 0),
			NumCharges: r.intDefault("numCharges", 0), MaxCharges: r.intDefault("maxCharges", 0),
			Charges: chargeCond(el.Conds, &res),
		}
		if res.err != nil {
			return fmt.Errorf("skill %d level %d: %w", id, level, res.err)
		}
		if r.err != nil {
			return fmt.Errorf("skill %d level %d: %w", id, level, r.err)
		}
		for _, f := range el.For {
			for _, n := range f.Nodes {
				if !strings.EqualFold(n.XMLName.Local, "effect") {
					fn, ok := statFunc(n, &res)
					if res.err != nil {
						return fmt.Errorf("skill %d level %d: %s: %w", id, level, n.XMLName.Local, res.err)
					}
					if ok {
						sk.Funcs = append(sk.Funcs, fn)
					}
					continue
				}
				vals := foldAttrs(n.Attrs).vals
				r := res.reader(vals, "stackOrder", "time", "count", "val")
				eff := Effect{
					Name:       r.str("name"),
					StackType:  r.strDefault("stackType", "none"),
					StackOrder: r.floatDefault("stackOrder", 0),
					Time:       int(r.literalDefault("time", 1)),
					Count:      int(r.literalDefault("count", 1)),
					Value:      r.floatDefault("val", 0),
				}
				if eff.Name == "" {
					r.fail("name", errMissing)
				}
				if res.err != nil {
					return fmt.Errorf("skill %d level %d: effect: %w", id, level, res.err)
				}
				if r.err != nil {
					return fmt.Errorf("skill %d level %d: effect: %w", id, level, r.err)
				}
				sk.Effects = append(sk.Effects, eff)
			}
		}
		skills[SkillRef{ID: id, Level: int32(level)}] = sk
	}
	return nil
}

// chargeCond reads the Charges condition of a skill level's <cond>
// elements: the first whose predicate is <player Charges>, or <not> of
// one. Charges may name a table, resolved against res's row.
func chargeCond(conds []xmlNode, res *resolver) ChargeCond {
	for _, c := range conds {
		if len(c.Children) == 0 {
			continue
		}
		pred, not := c.Children[0], false
		if strings.EqualFold(pred.XMLName.Local, "not") && len(pred.Children) > 0 {
			pred, not = pred.Children[0], true
		}
		if !strings.EqualFold(pred.XMLName.Local, "player") {
			continue
		}
		vals := foldAttrs(pred.Attrs).vals
		if _, ok := vals["Charges"]; !ok {
			continue
		}
		r := res.reader(vals, "Charges")
		n := r.int("Charges")
		if r.err != nil {
			res.fail(fmt.Errorf("cond: %w", r.err))
		}
		return ChargeCond{Min: n, Not: not}
	}
	return ChargeCond{}
}

// statFunc reads n as a stat function of a <for> block, its val resolved
// against res's row; ok is false for an element that is not one, which
// the server skips. Its first child is its condition (data/xml/skill.go
// funcTemplate).
func statFunc(n xmlNode, res *resolver) (fn StatFunc, ok bool) {
	for _, op := range statFuncOps {
		if strings.EqualFold(n.XMLName.Local, op) {
			fn.Op, ok = op, true
		}
	}
	if !ok {
		return StatFunc{}, false
	}
	r := res.reader(foldAttrs(n.Attrs).vals, "val")
	fn.Stat, fn.Value = r.str("stat"), r.float("val")
	if r.err != nil {
		res.fail(r.err)
		return StatFunc{}, false
	}
	if len(n.Children) == 0 {
		return fn, true
	}
	cond := n.Children[0]
	if strings.EqualFold(cond.XMLName.Local, "not") && len(cond.Children) == 1 {
		cond, fn.NotUsing = cond.Children[0], true
	}
	kind, hasKind := foldAttrs(cond.Attrs).vals["kind"]
	if !strings.EqualFold(cond.XMLName.Local, "using") || !hasKind || len(cond.Attrs) != 1 || len(cond.Children) != 0 {
		fn.NotUsing, fn.OtherCond = false, true
		return fn, true
	}
	fn.Using = splitKinds(kind)
	return fn, true
}

// splitKinds splits a "," separated item kind list, trimming each kind
// as the server's ParseWornKindMask does; nil for "".
func splitKinds(s string) []string {
	if s == "" {
		return nil
	}
	kinds := strings.Split(s, ",")
	for i, k := range kinds {
		kinds[i] = strings.TrimSpace(k)
	}
	return kinds
}

// isTableSpace reports whether c separates two values of a <table>: a
// space, tab, line feed, carriage return or form feed, as on the server
// (data/xml/tables.go); other white space is part of a value.
func isTableSpace(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// resolver resolves "#name" table references against one row of a skill's
// tables, keeping the first failure.
type resolver struct {
	tables map[string][]string
	row    int
	err    error
}

// reader returns an attrReader over vals with the values at keys resolved
// against r's row; the other values are kept as written.
func (r *resolver) reader(vals map[string]string, keys ...string) *attrReader {
	out := make(map[string]string, len(vals))
	for k, v := range vals {
		out[k] = v
	}
	for _, k := range keys {
		v, ok := vals[k]
		if !ok || !strings.HasPrefix(v, "#") {
			continue
		}
		row, ok := r.tables[v]
		switch {
		case !ok:
			r.fail(fmt.Errorf("attribute %q references undefined table %q", k, v))
		case r.row > len(row):
			r.fail(fmt.Errorf("attribute %q: table %q has no row %d (has %d)", k, v, r.row, len(row)))
		default:
			out[k] = row[r.row-1]
		}
	}
	return &attrReader{vals: out}
}

func (r *resolver) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}
