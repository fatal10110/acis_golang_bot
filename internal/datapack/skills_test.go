package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadSkills(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	haste := func(order, speed float64) []Effect {
		return []Effect{{
			Name: "Buff", StackType: "attack_time_down", StackOrder: order, Time: 1200, Count: 1,
			Funcs: []StatFunc{{Op: "mul", Stat: "pAtkSpd", Value: speed}},
		}}
	}
	hot := func(order, val float64) []Effect {
		return []Effect{{Name: "HealOverTime", StackType: "hp_recover", StackOrder: order, Time: 2, Count: 7, Value: val}}
	}
	magicHaste := func(order, speed float64) []Effect {
		return []Effect{{
			Name: "Buff", StackType: "casting_time_down", StackOrder: order, Time: 1200, Count: 1,
			Funcs: []StatFunc{{Op: "mul", Stat: "mAtkSpd", Value: speed}},
		}}
	}
	// The stack order of 1086 and 2169 and the cast range of 4110 differ
	// by level (#stackOrder, #castRange);
	// 1086's enchanted levels are not read; 4327 keeps its last
	// declaration.
	want := map[SkillRef]Skill{
		{1086, 1}: {Name: "Haste", Target: "TARGET_ONE", CastRange: 400, Effects: haste(1, 1.15)},
		{1086, 2}: {Name: "Haste", Target: "TARGET_ONE", CastRange: 400, Effects: haste(2, 1.33)},
		{1177, 1}: {Name: "Wind Strike", CastRange: 600},
		{4110, 1}: {Name: "Breath Attack", CastRange: 500},
		{4110, 2}: {Name: "Breath Attack", CastRange: 1000},
		{2002, 1}: {Name: "Healing Drug", Potion: true, Effects: []Effect{{Name: "HealOverTime", StackType: "hp_recover", StackOrder: 1.5, Time: 5, Count: 4, Value: 1.5}}},
		{2012, 1}: {Name: "Potion of Alacrity", Potion: true, Effects: haste(1, 1.15)},
		{2013, 1}: {Name: "Scroll of Escape", Type: "TELEPORT"},
		{2031, 1}: {Name: "Lesser Healing Potion", Potion: true, Effects: hot(1, 16)},
		{2032, 1}: {Name: "Healing potion", Potion: true, Effects: hot(2, 48)},
		{2037, 1}: {Name: "Greater Healing Potion", Potion: true, Effects: hot(3, 100)},
		{2169, 1}: {Name: "Magic Haste Potion", Potion: true, Effects: magicHaste(2, 1.23)},
		{2169, 2}: {Name: "Magic Haste Potion", Potion: true, Effects: magicHaste(3, 1.3)},
		{4327, 1}: {Name: "Haste for Beginners", Effects: []Effect{{
			Name: "Buff", StackType: "attack_time_down", StackOrder: 2, Time: 3600, Count: 1,
			Funcs: []StatFunc{{Op: "mul", Stat: "pAtkSpd", Value: 1.15}},
		}}},
	}
	// Combat fields, charges and stat functions: maxCharges, the
	// <not><player Charges> of 50 and the val of 142 differ by level
	// (#maxCharges, #rEvas); 163's unknown <for> element is skipped; 256's
	// effect count is hex and its <add> inside the effect is the effect's.
	focused := func(max int) Skill {
		return Skill{
			Name: "Focused Force", Type: "BUFF", OperateType: "ACTIVE", Target: "SELF", WeaponsAllowed: []string{"DUALFIST"},
			HitTime: 900 * time.Millisecond, ReuseDelay: time.Second, MPConsume: 5,
			NumCharges: 1, MaxCharges: max, Charges: ChargeCond{Min: max, Not: true},
		}
	}
	hurricane := func(mp, initMP int, power float64) Skill {
		return Skill{
			Name: "Hurricane Assault", Type: "CHARGEDAM", OperateType: "ACTIVE", Target: "ONE", WeaponsAllowed: []string{"DUALFIST"},
			CastRange: 40, HitTime: 1360 * time.Millisecond, ReuseDelay: 17 * time.Second, MPConsume: mp, MPInitialConsume: initMP, Power: power,
			NumCharges: 2, Charges: ChargeCond{Min: 2},
		}
	}
	passive := func(name string, funcs ...StatFunc) Skill {
		return Skill{Name: name, Type: "BUFF", OperateType: "PASSIVE", Target: "SELF", Funcs: funcs}
	}
	pDef := func(v float64) StatFunc { return StatFunc{Op: "add", Stat: "pDef", Value: v} }
	rEvas := func(v float64) StatFunc {
		return StatFunc{Op: "add", Stat: "rEvas", Value: v, Using: []string{"LIGHT", "HEAVY"}}
	}
	want[SkillRef{50, 1}] = focused(1)
	want[SkillRef{50, 2}] = focused(2)
	want[SkillRef{142, 1}] = passive("Armor Mastery", pDef(9), rEvas(0))
	want[SkillRef{142, 2}] = passive("Armor Mastery", pDef(11), rEvas(3))
	want[SkillRef{163, 1}] = passive("Spellcraft", StatFunc{Op: "mul", Stat: "mAtkSpd", Value: 0.5, Using: []string{"MAGIC"}, NotUsing: true})
	want[SkillRef{256, 1}] = Skill{
		Name: "Accuracy", Type: "CONT", OperateType: "TOGGLE", Target: "SELF", MPConsume: 1,
		Effects: []Effect{{
			Name: "ManaDamOverTime", StackType: "none", Time: 5, Count: 0x7fffffff, Value: 4.8,
			Funcs: []StatFunc{{Op: "add", Stat: "accCombat", Value: 3}},
		}},
	}
	want[SkillRef{284, 1}] = hurricane(49, 12, 501)
	want[SkillRef{284, 2}] = hurricane(50, 13, 535)
	want[SkillRef{290, 1}] = passive("Final Frenzy", StatFunc{Op: "add", Stat: "pAtk", Value: 2, OtherCond: true})
	want[SkillRef{3502, 1}] = passive("Mithril Heavy Armor Set", StatFunc{Op: "addMul", Stat: "poisonVuln", Value: 20})
	// None of these sets a landing or cure field: each reads its default,
	// and only Hurricane Assault's CHARGEDAM is offensive.
	for ref, sk := range want {
		sk.Radius, sk.AbnormalLevel, sk.EffectAbnormalLevel, sk.NegateLevel = 80, -1, -1, -1
		sk.Offensive = sk.Type == "CHARGEDAM"
		for i := range sk.Effects {
			sk.Effects[i].EffectPower = -1
		}
		want[ref] = sk
	}
	if !reflect.DeepEqual(dp.Skills, want) {
		t.Errorf("Skills = %+v, want %+v", dp.Skills, want)
	}
}

func TestLoadSkillsRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"no id", `<skill levels="1" name="x"/>`, `"id"`},
		{"no levels", `<skill id="1" name="x"/>`, `"levels"`},
		{"negative levels", `<skill id="1" levels="-1" name="x"/>`, `"levels"`},
		{"no name", `<skill id="1" levels="1"/>`, `"name"`},
		{"table name", `<skill id="1" levels="1" name="x"><table name="order"> 1 </table></skill>`, "table name"},
		{"stack order", `<skill id="1" levels="1" name="x"><for><effect name="Buff" stackOrder="high"/></for></skill>`, `"stackOrder"`},
		{"time", `<skill id="1" levels="1" name="x"><for><effect name="Buff" time="1.5"/></for></skill>`, `"time"`},
		{"count", `<skill id="1" levels="1" name="x"><for><effect name="Buff" count="many"/></for></skill>`, `"count"`},
		{"effect name", `<skill id="1" levels="1" name="x"><for><effect name="" time="1"/></for></skill>`, `"name"`},
		{"undefined table", `<skill id="1" levels="1" name="x"><for><effect name="Buff" stackOrder="#order"/></for></skill>`, "undefined table"},
		{"short table", `<skill id="1" levels="2" name="x"><table name="#order"> 1 </table><for><effect name="Buff" stackOrder="#order"/></for></skill>`, "no row 2"},
		{"potion table", `<skill id="1" levels="1" name="x"><set name="isPotion" val="#potion"/></skill>`, "undefined table"},
		{"cast range", `<skill id="1" levels="1" name="x"><set name="castRange" val="far"/></skill>`, `"castRange"`},
		{"cast range table", `<skill id="1" levels="2" name="x"><table name="#range"> 400 </table><set name="castRange" val="#range"/></skill>`, "no row 2"},
		{"hit time", `<skill id="1" levels="1" name="x"><set name="hitTime" val="0.9"/></skill>`, `"hitTime"`},
		{"reuse delay", `<skill id="1" levels="1" name="x"><set name="reuseDelay" val="soon"/></skill>`, `"reuseDelay"`},
		{"mp consume", `<skill id="1" levels="1" name="x"><set name="mpConsume" val="1.5"/></skill>`, `"mpConsume"`},
		{"power", `<skill id="1" levels="1" name="x"><set name="power" val="strong"/></skill>`, `"power"`},
		{"power table", `<skill id="1" levels="2" name="x"><table name="#power"> 10 </table><set name="power" val="#power"/></skill>`, "no row 2"},
		{"max charges", `<skill id="1" levels="1" name="x"><set name="maxCharges" val="#max"/></skill>`, "undefined table"},
		{"num charges", `<skill id="1" levels="1" name="x"><set name="numCharges" val="two"/></skill>`, `"numCharges"`},
		{"charges", `<skill id="1" levels="1" name="x"><cond><player Charges="many"/></cond></skill>`, `"Charges"`},
		{"charges table", `<skill id="1" levels="2" name="x"><table name="#c"> 1 </table><cond><not><player Charges="#c"/></not></cond></skill>`, "no row 2"},
		{"func stat", `<skill id="1" levels="1" name="x"><for><add val="1"/></for></skill>`, `"stat"`},
		{"func val", `<skill id="1" levels="1" name="x"><for><mul stat="pAtk"/></for></skill>`, `"val"`},
		{"func val number", `<skill id="1" levels="1" name="x"><for><mul stat="pAtk" val="x"/></for></skill>`, `"val"`},
		{"func val table", `<skill id="1" levels="2" name="x"><table name="#v"> 1 </table><for><add stat="pAtk" val="#v"/></for></skill>`, "no row 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSkills(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadSkills err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}
