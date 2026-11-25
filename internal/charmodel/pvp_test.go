package charmodel

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The golden table's skills (docs/pvp/model.md "Land rates") at a magicLvl
// equal to the target's level 20: Shield Stun 92 (power 80, lvlDepend 2),
// Stun Attack 100 (a PDAM whose stun effect has effectPower 50, lvlDepend
// 1), Sleep 1069 (power 70, magic) and Dryad Root 1201 (power 80, magic).
var (
	testShieldStun = datapack.Skill{
		Type: "STUN", Power: 80, MagicLevel: 20, LevelDepend: 2,
		HitTime: 1200 * time.Millisecond, ReuseDelay: 12 * time.Second,
		Effects: []datapack.Effect{{Name: "Stun", Time: 9, EffectPower: -1}},
	}
	testStunAttack = datapack.Skill{
		Type: "PDAM", Power: 100, MagicLevel: 20, LevelDepend: 1,
		HitTime: 1080 * time.Millisecond, ReuseDelay: 13 * time.Second,
		Effects: []datapack.Effect{{Name: "Stun", Time: 9, EffectPower: 50, EffectType: "STUN"}},
	}
	testSleep = datapack.Skill{Type: "SLEEP", Magic: true, Power: 70, MagicLevel: 20, LevelDepend: 2}
	testRoot  = datapack.Skill{Type: "ROOT", Magic: true, Power: 80, MagicLevel: 20, LevelDepend: 2}
)

// TestLandRateGolden checks floor(LandRate) against the golden table at
// equal level, matk 1 (M.Atk 1 against M.Def 11) and no resist gear.
func TestLandRateGolden(t *testing.T) {
	caster := Fighter{Profile: Profile{MAtk: 1}, Level: 20}
	for _, tt := range []struct {
		name                     string
		con, men                 int
		shield, stun, sleep, dry int
	}{
		{"Human Fighter", 43, 25, 60, 37, 61, 70},
		{"Elven Fighter", 36, 26, 70, 43, 60, 69},
		{"Dark Fighter", 32, 26, 75, 46, 60, 69},
		{"Orc Fighter", 47, 27, 54, 33, 60, 69},
		{"Dwarven Fighter", 45, 27, 57, 35, 60, 69},
		{"Human Mystic", 27, 39, 81, 50, 55, 63},
		{"Elven Mystic", 25, 40, 83, 52, 55, 62},
		{"Dark Mystic", 24, 37, 84, 52, 56, 64},
		{"Orc Mystic", 31, 42, 76, 47, 54, 61},
	} {
		target := Fighter{Profile: Profile{MDef: 11}, Level: 20, CON: tt.con, MEN: tt.men}
		for _, c := range []struct {
			skill string
			s     datapack.Skill
			want  int
		}{
			{"Shield Stun", testShieldStun, tt.shield},
			{"Stun Attack", testStunAttack, tt.stun},
			{"Sleep", testSleep, tt.sleep},
			{"Dryad Root", testRoot, tt.dry},
		} {
			if got := int(math.Floor(LandRate(c.s, caster, target))); got != c.want {
				t.Errorf("%s %s = %d %%, want %d", tt.name, c.skill, got, c.want)
			}
		}
	}
}

// TestLandRateServerFormula checks LandRate against the server's product
// with a sleep jewel (addMul sleepVuln 30) worn by the target, a blessed
// spiritshot on the caster and a level gap either way.
func TestLandRateServerFormula(t *testing.T) {
	class := datapack.Class{CON: 43, MEN: 25, MDef: 41, HPTable: []float64{100}, CPTable: []float64{50}}
	sleep := datapack.Skill{Type: "SLEEP", Magic: true, Power: 70, LevelDepend: 2}
	for _, tt := range []struct {
		name          string
		casterLevel   int
		targetLevel   int
		jewel         bool
		shot          Shot
		wantLevelTerm float64
	}{
		{"jewel, blessed, caster above", 30, 25, true, BlessedSpiritshot, 1 + 0.005*7},
		{"jewel, blessed, caster below", 20, 25, true, BlessedSpiritshot, 1 - 0.01*3},
		{"no jewel, unshot, caster below", 20, 25, false, NoShot, 1 - 0.01*3},
	} {
		b := Build{Class: class, Level: tt.targetLevel}
		vuln := 1.0
		if tt.jewel {
			b.Funcs = []datapack.StatFunc{{Op: "addMul", Stat: "sleepVuln", Value: 30}}
			vuln = 0.7
		}
		target := ModelFighter(b)
		caster := Fighter{Profile: Profile{MAtk: 100}, Level: tt.casterLevel, Shot: tt.shot}
		mAtk := 100.0
		if tt.shot == BlessedSpiritshot {
			mAtk = 400
		}
		want := 70 * math.Max(0, 2-math.Sqrt(MENBonus(25))) * vuln *
			(math.Sqrt(mAtk) / target.Profile.MDef * 11) * tt.wantLevelTerm
		want = min(max(want, 1), 99)
		if got := LandRate(sleep, caster, target); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: LandRate = %v, want %v", tt.name, got, want)
		}
	}
}

// TestLandRateBase checks the land base and type of a PDAM, an MDAM, an
// effect-power and a level effectPower skill.
func TestLandRateBase(t *testing.T) {
	for _, tt := range []struct {
		name     string
		s        datapack.Skill
		want     float64
		wantType string
	}{
		{"PDAM", datapack.Skill{Type: "PDAM", Power: 300}, 20, "STUN"},
		{"MDAM", datapack.Skill{Type: "MDAM", Power: 40}, 20, "PARALYZE"},
		{"effect power", testStunAttack, 50, "STUN"},
		{"level effectPower", datapack.Skill{
			Type: "DEBUFF", Power: 90, EffectPower: 60, EffectType: "ROOT",
			Effects: []datapack.Effect{{Name: "Debuff", EffectPower: -1}},
		}, 60, "ROOT"},
		{"power in (0, 100]", testShieldStun, 80, "STUN"},
		{"power past 100", datapack.Skill{Type: "SLEEP", Power: 150}, 20, "SLEEP"},
		{"no power", datapack.Skill{Type: "ROOT"}, 20, "ROOT"},
	} {
		if got := landingPower(tt.s); got != tt.want {
			t.Errorf("%s: landing power = %v, want %v", tt.name, got, tt.want)
		}
		if got := landingType(tt.s); got != tt.wantType {
			t.Errorf("%s: landing type = %q, want %q", tt.name, got, tt.wantType)
		}
	}
}

// TestCastBreakChance checks the cast-break chance and the golden table's
// offsets, -(MEN bonus x 100 - 100) rounded.
func TestCastBreakChance(t *testing.T) {
	for _, tt := range []struct {
		damage float64
		men    int
		want   float64
	}{
		{0, 25, 1},
		{100, 25, 15 + math.Sqrt(1300) - (MENBonus(25)*100 - 100)},
		{400, 39, 15 + math.Sqrt(5200) - (MENBonus(39)*100 - 100)},
		{100000, 25, 99},
	} {
		if got := CastBreakChance(tt.damage, tt.men); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("CastBreakChance(%v, %d) = %v, want %v", tt.damage, tt.men, got, tt.want)
		}
	}
	for _, tt := range []struct{ men, offset int }{
		{25, -28}, {26, -30}, {27, -31}, {39, -48}, {40, -49}, {37, -45}, {42, -52},
	} {
		if got := int(math.Round(-(MENBonus(tt.men)*100 - 100))); got != tt.offset {
			t.Errorf("MEN %d cast-break offset = %d, want %d", tt.men, got, tt.offset)
		}
	}
}

// TestModelFighter checks the CP, base stats and multiplier stats a build
// gives: Earring of Antharas's vulnerabilities and a PvP damage weapon's
// bonus.
func TestModelFighter(t *testing.T) {
	class := datapack.Class{CON: 43, MEN: 25, PDef: 80, MDef: 41, HPTable: []float64{100, 120}, CPTable: []float64{50, 60}}
	f := ModelFighter(Build{Class: class, Level: 2, Funcs: []datapack.StatFunc{
		{Op: "addMul", Stat: "earthRes", Value: 15},
		{Op: "addMul", Stat: "bleedVuln", Value: 80},
		{Op: "addMul", Stat: "stunVuln", Value: 60},
		{Op: "mul", Stat: "pvpPhysDmg", Value: 1.05},
	}})
	if want := math.Trunc(60 * CONBonus(43)); f.MaxCP != want {
		t.Errorf("MaxCP = %v, want %v", f.MaxCP, want)
	}
	if f.Level != 2 || f.CON != 43 || f.MEN != 25 {
		t.Errorf("level, CON, MEN = %d, %d, %d, want 2, 43, 25", f.Level, f.CON, f.MEN)
	}
	for name, want := range map[string]float64{"bleedVuln": 0.2, "stunVuln": 0.4, "pvpPhysDmg": 1.05, "sleepVuln": 1, "pvpMagicalDmg": 1} {
		if got := f.mul(name); math.Abs(got-want) > 1e-12 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}
