package datapack

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Skill is the part of one level of a skill template (skills/*.xml) bots
// weigh potions and buffs with: whether it is a potion's and the effects it
// lands, with their stack types.
type Skill struct {
	Name string
	// Potion is isPotion: the skill of a potion etc item.
	Potion bool
	// Effects are the effects of the skill's <for> blocks at this level,
	// in file order; nil for none. The effects only an enchanted skill
	// level lands (<enchant1for>, <enchant2for>) are not read.
	Effects []Effect
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
}

type skillFile struct {
	Skills []skillElement `xml:"skill"`
}

// skillElement is one <skill>: its attributes, its value tables, its direct
// <set> children and its <for> blocks. <enchant1>, <enchant2>,
// <enchant1for>, <enchant2for> and <cond> are not read.
type skillElement struct {
	Attrs  []xml.Attr `xml:",any,attr"`
	Tables []struct {
		Name string `xml:"name,attr"`
		Text string `xml:",chardata"`
	} `xml:"table"`
	Sets []setElement `xml:"set"`
	For  []struct {
		Effects []struct {
			Attrs []xml.Attr `xml:",any,attr"`
		} `xml:"effect"`
	} `xml:"for"`
}

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
// name are required, isPotion defaults to false; an effect's name is
// required, its stackType defaults to "none", stackOrder to 0, time and
// count to 1 (data/xml/effectgrammar.go readEffectTemplate).
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
		sk := Skill{Name: name, Potion: res.reader(sets, "isPotion").boolDefault("isPotion", false)}
		if res.err != nil {
			return fmt.Errorf("skill %d level %d: %w", id, level, res.err)
		}
		for _, f := range el.For {
			for _, e := range f.Effects {
				vals := foldAttrs(e.Attrs).vals
				r := res.reader(vals, "stackOrder", "time", "count")
				eff := Effect{
					Name:       r.str("name"),
					StackType:  r.strDefault("stackType", "none"),
					StackOrder: r.floatDefault("stackOrder", 0),
					Time:       int(r.literalDefault("time", 1)),
					Count:      int(r.literalDefault("count", 1)),
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
