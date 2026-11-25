package charmodel

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// testPoison is Poison 129 level 1 (skills/0100-0199.xml): a magic POISON
// of power 70 whose DamOverTime ticks 54 ten times, 3 s apart.
var testPoison = datapack.Skill{
	Type: "POISON", Magic: true, Power: 70, MagicLevel: 20, LevelDepend: 1,
	HitTime: 3 * time.Second, ReuseDelay: 9 * time.Second,
	Effects: []datapack.Effect{{Name: "DamOverTime", Count: 10, Time: 3, Value: 54, EffectPower: -1}},
}

// testFighter is a level 20 fighter of row id.
func testFighter(id string) Fighter {
	return Fighter{
		Profile: Profile{
			MaxHP: 1000, PAtk: 200, PDef: 200, MAtk: 100, MDef: 150,
			PAtkSpd: 300, MAtkSpd: 333, Accuracy: 50, Evasion: 50, Critical: 40,
		},
		MaxCP: 500, Level: 20, CON: 40, MEN: 25, Row: Row{ID: id},
	}
}

// testCaster is a level 20 caster of row id: little P.Atk, more M.Atk.
func testCaster(id string) Fighter {
	f := testFighter(id)
	f.Profile.PAtk, f.Profile.MAtk = 50, 150
	return f
}

// TestPvPValuePoisonGate checks a row with only Poison: gated vs C, H it
// lowers a CL's PvP value and leaves a WA's; ungated it lowers both.
func TestPvPValuePoisonGate(t *testing.T) {
	dm := Row{ID: "DM"}
	mix := []Share{{Opponent: testCaster("DM"), Share: 1}}
	gated := RowTable{dm: {{Skill: testPoison, Level: 20, Vs: []Archetype{Caster, Healer}}}}
	ungated := RowTable{dm: {{Skill: testPoison, Level: 20}}}
	for _, tt := range []struct {
		bot          string
		rows         RowTable
		lower, equal bool
	}{
		{"CL", gated, true, false},
		{"WA", gated, false, true},
		{"CL", ungated, true, false},
		{"WA", ungated, true, false},
	} {
		bot := testFighter(tt.bot)
		plain, got := PvPValue(bot, mix, RowTable{}), PvPValue(bot, mix, tt.rows)
		if tt.lower && !(got < plain) {
			t.Errorf("%s: PvPValue %v, want below %v without poison", tt.bot, got, plain)
		}
		if tt.equal && math.Abs(got-plain) > 1e-9 {
			t.Errorf("%s: PvPValue %v, want %v as without poison", tt.bot, got, plain)
		}
	}
}

// TestPvPValueMissingRow checks an opponent whose row the table lacks, or
// whose steps are all gated out or past its level, plays plain attacks.
func TestPvPValueMissingRow(t *testing.T) {
	opp, bot := testFighter("KN"), testFighter("WA")
	want := hitDamage(opp.Profile, Attack{}, bot.Profile) / AttackInterval(opp.Profile.PAtkSpd).Seconds()
	kn := Row{ID: "KN"}
	for _, tt := range []struct {
		name string
		rows RowTable
	}{
		{"no table", nil},
		{"row missing", RowTable{Row{ID: "HF"}: {{Skill: testShieldStun, Level: 20}}}},
		{"gated out", RowTable{kn: {{Skill: testShieldStun, Level: 20, Vs: []Archetype{Caster, Healer}}}}},
		{"past its level", RowTable{kn: {{Skill: testShieldStun, Level: 21}}}},
	} {
		if got := play(opp, bot, tt.rows); math.Abs(got.dps-want) > 1e-9 || got.hold != 0 {
			t.Errorf("%s: play = %+v, want dps %v and no hold", tt.name, got, want)
		}
	}
}

// TestPvPValueWinEven checks win_r is 0.5 against an equal, control steps
// included, and against an opponent neither side can hurt.
func TestPvPValueWinEven(t *testing.T) {
	wa := testFighter("WA")
	rows := RowTable{Row{ID: "WA"}: {{Skill: testStunAttack, Level: 20}}}
	if got := PvPValue(wa, []Share{{Opponent: wa, Share: 1}}, rows); got != 0.5 {
		t.Errorf("PvPValue against an equal = %v, want 0.5", got)
	}
	harmless := testFighter("WA")
	harmless.Profile.PAtk = 0
	if got := winShare(harmless, harmless, nil); got != 0.5 {
		t.Errorf("win_r of two harmless fighters = %v, want 0.5", got)
	}
}

// TestPvPValueShareWeighted checks PvPValue is the share-weighted sum of
// win_r.
func TestPvPValueShareWeighted(t *testing.T) {
	bot := testFighter("WA")
	weak, strong := testFighter("HF"), testFighter("OR")
	weak.Profile.PAtk, strong.Profile.PAtk = 100, 400
	rows := RowTable{Row{ID: "OR"}: {{Skill: testStunAttack, Level: 20}}}
	ww, ws := winShare(bot, weak, rows), winShare(bot, strong, rows)
	if !(ww > 0.5 && ws < 0.5) {
		t.Fatalf("win_r weak %v, strong %v, want above and below 0.5", ww, ws)
	}
	mix := []Share{{Opponent: weak, Share: 0.3}, {Opponent: strong, Share: 0.7}}
	if got, want := PvPValue(bot, mix, rows), 0.3*ww+0.7*ws; math.Abs(got-want) > 1e-12 {
		t.Errorf("PvPValue = %v, want %v", got, want)
	}
}

// TestPvPValueGear checks a 1.05 pvpPhysDmg weapon scores above the same
// without, and Earring of Antharas raises u_r against a stun row.
func TestPvPValueGear(t *testing.T) {
	class := datapack.Class{
		CON: 43, MEN: 25, STR: 40, DEX: 30, PAtk: 4, PDef: 80, MDef: 41,
		HPTable: make([]float64, 20), CPTable: make([]float64, 20),
	}
	class.HPTable[19], class.CPTable[19] = 400, 200
	sword := datapack.Item{Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Stats: datapack.Stats{PAtk: 100}}
	fighter := func(funcs ...datapack.StatFunc) Fighter {
		f := ModelFighter(Build{Class: class, Level: 20, Worn: []datapack.Item{sword}, Funcs: funcs})
		f.Row = Row{ID: "WA"}
		return f
	}
	mix := []Share{{Opponent: fighter(), Share: 1}}
	stun := RowTable{Row{ID: "WA"}: {{Skill: testShieldStun, Level: 20}}}

	pvpWeapon := fighter(datapack.StatFunc{Op: "mul", Stat: "pvpPhysDmg", Value: 1.05})
	if with, without := PvPValue(pvpWeapon, mix, nil), PvPValue(fighter(), mix, nil); !(with > without) {
		t.Errorf("PvPValue with pvpPhysDmg 1.05 = %v, want above %v", with, without)
	}

	antharas := fighter(
		datapack.StatFunc{Op: "addMul", Stat: "earthRes", Value: 15},
		datapack.StatFunc{Op: "addMul", Stat: "bleedVuln", Value: 80},
		datapack.StatFunc{Op: "addMul", Stat: "stunVuln", Value: 60},
		datapack.StatFunc{Op: "addMul", Stat: "derangementVuln", Value: 60},
	)
	opp := mix[0].Opponent
	with, without := 1-play(opp, antharas, stun).hold, 1-play(opp, fighter(), stun).hold
	if !(without < 1 && with > without) {
		t.Errorf("u_r with Earring of Antharas = %v, want above %v (below 1)", with, without)
	}
}

// TestBlend checks w 0 is exactly the Value delta and w 0.5 scores +10 %
// Value as +10 % PvP value.
func TestBlend(t *testing.T) {
	for _, tt := range []struct{ value, dv, pvp, dp float64 }{
		{3.7, 0.37, 0.6, 0.05},
		{1.1, -0.2, 0, 0.1},
		{0, 0.3, 0.5, -0.1},
	} {
		if got := Blend(tt.value, tt.dv, tt.pvp, tt.dp, 0); got != tt.dv {
			t.Errorf("Blend(%+v, w 0) = %v, want exactly %v", tt, got, tt.dv)
		}
	}
	value, pvp := 3.7, 0.6
	byValue := Blend(value, 0.1*value, pvp, 0, PvPWeight)
	byPvP := Blend(value, 0, pvp, 0.1*pvp, PvPWeight)
	if math.Abs(byValue-byPvP) > 1e-9 || byValue <= 0 {
		t.Errorf("w 0.5: +10%% Value scores %v, +10%% PvP value %v, want equal and positive", byValue, byPvP)
	}
}

// TestEngage checks the engage estimate against the spot margin.
func TestEngage(t *testing.T) {
	bot := testFighter("WA")
	weak := testFighter("HF")
	weak.Profile.PAtk = 50
	harmless := testFighter("HF")
	harmless.Profile.PDef = math.Inf(1)
	for _, tt := range []struct {
		name   string
		opp    Fighter
		margin float64
		want   bool
	}{
		{"weak opponent, glass cannon", weak, 3, true},
		{"equal, tank margin", bot, 1.5, false},
		{"equal, margin 1", bot, 1, true},
		{"cannot hurt it", harmless, 1.5, false},
	} {
		if got := Engage(bot, tt.opp, nil, tt.margin); got != tt.want {
			kill, die := FightTimes(bot, tt.opp, nil)
			t.Errorf("%s: Engage = %v, want %v (kill %v, die %v)", tt.name, got, tt.want, kill, die)
		}
	}
	kill, die := FightTimes(bot, weak, nil)
	k, d := fight(bot, weak, nil)
	if math.Abs(kill.Seconds()-k) > 1e-6 || math.Abs(die.Seconds()-d) > 1e-6 {
		t.Errorf("FightTimes = %v, %v, want %v s, %v s", kill, die, k, d)
	}
}
