package botclient

import (
	"os"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The reference gear tests' templates: grade D swords at four prices, a C
// sword, a D two-handed sword, magic blunt, dagger and bow, three shields, three light
// sets (the middle one with a shield and a skill), a heavy and a robe set,
// and jewelry.
const (
	refSword1, refSword2, refSword3, refSword4 int32 = 9101, 9102, 9103, 9104
	refSwordC, refBigSword, refWand            int32 = 9105, 9106, 9107
	refDagger, refBow                          int32 = 9108, 9109
	refShield1, refShield2, refShield3         int32 = 9201, 9202, 9203
	refLight1, refLight1Legs                   int32 = 9301, 9302
	refLight2, refLight2Legs                   int32 = 9303, 9304
	refLight3, refLight3Legs                   int32 = 9305, 9306
	refHeavy, refHeavyLegs                     int32 = 9311, 9312
	refRobe, refRobeLegs                       int32 = 9321, 9322
	refNeck1, refNeck2, refNeck3               int32 = 9401, 9402, 9403
	refEar, refRing1, refRing2                 int32 = 9411, 9421, 9422
	refSetSkill                                int32 = 4001
)

var refItems = func() map[int32]datapack.Item {
	m := make(map[int32]datapack.Item)
	weapon := func(id int32, part, kind string, magical bool, grade int, price int32, pAtk float64) {
		m[id] = datapack.Item{
			ID: id, Type: "Weapon", BodyPart: part, Kind: kind, Magical: magical, Grade: grade, Price: price,
			Stats: datapack.Stats{PAtk: pAtk, MAtk: pAtk / 2, PAtkSpd: 379, RCrit: 8},
		}
	}
	armor := func(id int32, part, kind string, price int32, s datapack.Stats) {
		m[id] = datapack.Item{ID: id, Type: "Armor", BodyPart: part, Kind: kind, Grade: 1, Price: price, Stats: s}
	}
	weapon(refSword1, "rhand", "SWORD", false, 1, 100, 60)
	weapon(refSword2, "rhand", "SWORD", false, 1, 200, 80)
	weapon(refSword3, "rhand", "SWORD", false, 1, 300, 100)
	weapon(refSword4, "rhand", "SWORD", false, 1, 400, 120)
	weapon(refSwordC, "rhand", "SWORD", false, 2, 1000, 150)
	weapon(refBigSword, "lrhand", "BIGSWORD", false, 1, 500, 110)
	weapon(refWand, "rhand", "BLUNT", true, 1, 150, 50)
	weapon(refDagger, "rhand", "DAGGER", false, 1, 250, 70)
	weapon(refBow, "lrhand", "BOW", false, 1, 350, 150)
	armor(refShield1, "lhand", "", 50, datapack.Stats{SDef: 100})
	armor(refShield2, "lhand", "", 70, datapack.Stats{SDef: 120})
	armor(refShield3, "lhand", "", 90, datapack.Stats{SDef: 140})
	armor(refLight1, "chest", "LIGHT", 100, datapack.Stats{PDef: 40})
	armor(refLight1Legs, "legs", "LIGHT", 100, datapack.Stats{PDef: 25})
	armor(refLight2, "chest", "LIGHT", 200, datapack.Stats{PDef: 50})
	armor(refLight2Legs, "legs", "LIGHT", 200, datapack.Stats{PDef: 30})
	armor(refLight3, "chest", "LIGHT", 300, datapack.Stats{PDef: 60})
	armor(refLight3Legs, "legs", "LIGHT", 300, datapack.Stats{PDef: 35})
	armor(refHeavy, "chest", "HEAVY", 250, datapack.Stats{PDef: 70})
	armor(refHeavyLegs, "legs", "HEAVY", 250, datapack.Stats{PDef: 40})
	armor(refRobe, "chest", "MAGIC", 250, datapack.Stats{PDef: 30})
	armor(refRobeLegs, "legs", "MAGIC", 250, datapack.Stats{PDef: 15})
	armor(refNeck1, "neck", "", 100, datapack.Stats{MDef: 30})
	armor(refNeck2, "neck", "", 200, datapack.Stats{MDef: 40})
	armor(refNeck3, "neck", "", 300, datapack.Stats{MDef: 50})
	armor(refEar, "rear;lear", "", 150, datapack.Stats{MDef: 25})
	armor(refRing1, "rfinger;lfinger", "", 100, datapack.Stats{MDef: 15})
	armor(refRing2, "rfinger;lfinger", "", 200, datapack.Stats{MDef: 20})
	return m
}()

var refSets = map[int32]datapack.ArmorSet{
	refLight1: {Chest: refLight1, Legs: refLight1Legs},
	refLight2: {Chest: refLight2, Legs: refLight2Legs, Shield: refShield3, SkillID: refSetSkill},
	refLight3: {Chest: refLight3, Legs: refLight3Legs},
	refHeavy:  {Chest: refHeavy, Legs: refHeavyLegs},
	refRobe:   {Chest: refRobe, Legs: refRobeLegs},
}

var refSkillFuncs = []datapack.StatFunc{{Op: "add", Stat: "pDef", Value: 20}}

// refTravel is a travel map of the reference gear templates and sets.
func refTravel() *TravelMap {
	gear, _ := indexGear(refItems, nil)
	sets, pieces := indexSets(refSets, gear)
	return &TravelMap{
		gear: gear, sets: sets, setPieces: pieces,
		skills: map[datapack.SkillRef]datapack.Skill{{ID: refSetSkill, Level: 1}: {OperateType: "PASSIVE", Funcs: refSkillFuncs}},
	}
}

func TestRefGear(t *testing.T) {
	jewels := []int32{refNeck2, refEar, refEar, refRing1, refRing1}
	cases := []struct {
		name  string
		gc    GearClass
		grade int32
		want  []int32
		funcs []datapack.StatFunc
	}{
		{"no grade", gearClassOf(defaultGearClasses(), 0), 0, nil, nil},
		{
			"light fighter: middle sword, middle set with its shield and skill",
			gearClassOf(defaultGearClasses(), 0), 1,
			append([]int32{refSword2, refShield3, refLight2, refLight2Legs}, jewels...), refSkillFuncs,
		},
		{
			"heavy fighter: a set without a shield takes the middle shield",
			gearClassOf(defaultGearClasses(), 4), 1,
			append([]int32{refSword2, refShield2, refHeavy, refHeavyLegs}, jewels...), nil,
		},
		{
			"mystic: magic blunt and robe set, no shield",
			gearClassOf(defaultGearClasses(), 10), 1,
			append([]int32{refWand, refRobe, refRobeLegs}, jewels...), nil,
		},
		{
			"a two-handed weapon takes no shield",
			GearClass{Weapons: []string{"BIGSWORD"}, Armor: "HEAVY", Shield: true},
			1,
			append([]int32{refBigSword, refHeavy, refHeavyLegs}, jewels...), nil,
		},
		{
			"rogue: dagger and middle light set, its skill without the shield",
			refGearClass(7, false), 1,
			append([]int32{refDagger, refLight2, refLight2Legs}, jewels...), refSkillFuncs,
		},
		{
			"rogue with a bow row: the bow",
			refGearClass(7, true), 1,
			append([]int32{refBow, refLight2, refLight2Legs}, jewels...), refSkillFuncs,
		},
		{
			"a bow row of a shield build: the bow, no shield",
			refGearClass(0, true), 1,
			append([]int32{refBow, refLight2, refLight2Legs}, jewels...), refSkillFuncs,
		},
		{"a grade with only a weapon", gearClassOf(defaultGearClasses(), 0), 2, []int32{refSwordC}, nil},
	}
	m := refTravel()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, funcs := m.refGear(tc.gc, tc.grade)
			var got []int32
			for _, it := range items {
				got = append(got, it.ID)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("worn %v, want %v", got, tc.want)
			}
			if !slices.EqualFunc(funcs, tc.funcs, func(a, b datapack.StatFunc) bool { return a.Op == b.Op && a.Stat == b.Stat && a.Value == b.Value }) {
				t.Errorf("funcs %v, want %v", funcs, tc.funcs)
			}
		})
	}
}

func TestExpertiseGrade(t *testing.T) {
	for _, tc := range []struct {
		level int
		want  int32
	}{{1, 0}, {19, 0}, {20, 1}, {39, 1}, {40, 2}, {52, 3}, {61, 4}, {75, 4}, {76, 5}, {80, 5}} {
		if got := expertiseGrade(tc.level); got != tc.want {
			t.Errorf("expertiseGrade(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
}

// TestPvPEngageRefGear is the engage estimate against a Human Fighter of
// the bot's class and level (20, grade D) while the bot wears the
// grade-typical gear (#553): the reference opponent wears it too, an even
// fight the bot does not take at margin 2.0; without gear on the travel
// map the reference opponent is naked, and the bot takes it.
func TestPvPEngageRefGear(t *testing.T) {
	var worn []datapack.Item
	for _, id := range []int32{refSword2, refShield3, refLight2, refLight2Legs, refNeck2, refEar, refEar, refRing1, refRing1} {
		worn = append(worn, refItems[id])
	}
	geared := charmodel.ModelFighter(charmodel.Build{Class: findFighter, Level: 20, Worn: worn, Funcs: refSkillFuncs})
	cases := []struct {
		name  string
		naked bool
		want  bool
	}{
		{"grade-typical opponent: even fight", false, false},
		{"naked opponent", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := findBrain(t, nil)
			if m := b.build().Kit.SpotMargin; m != 0 && m != marginNormal {
				t.Fatalf("spot margin %v, want the normal %v", m, marginNormal)
			}
			ref := refTravel()
			if !tc.naked {
				b.cfg.Travel.gear, b.cfg.Travel.sets, b.cfg.Travel.skills = ref.gear, ref.sets, ref.skills
			}
			b.pvp.rows, b.pvp.rowsLevel = charmodel.RowTable{}, 20 // plain attacks
			s, p := &b.world.Self, geared.Profile
			s.MaxHP, s.HP, s.MaxMP, s.MP = int32(p.MaxHP), int32(p.MaxHP), int32(p.MaxMP), int32(p.MaxMP)
			s.PAtk, s.PDef, s.MAtk, s.MDef = int32(p.PAtk), int32(p.PDef), int32(p.MAtk), int32(p.MDef)
			s.PAtkSpd, s.CastSpd, s.Accuracy, s.Evasion, s.Critical = int32(p.PAtkSpd), int32(p.MAtkSpd), int32(p.Accuracy), int32(p.Evasion), int32(p.Critical)
			s.MaxCP, s.CP, s.CON, s.MEN = int32(geared.MaxCP), int32(geared.MaxCP), int32(geared.CON), int32(geared.MEN)
			if o := b.world.object(pcID); o == nil || o.ClassID != s.ClassID {
				t.Fatalf("opponent %+v, want class %d", o, s.ClassID)
			}
			if got := b.engages(pcID); got != tc.want {
				t.Errorf("engages = %v, want %v", got, tc.want)
			}
		})
	}
	t.Run("a bow row's reference wields the bow", func(t *testing.T) {
		b := findBrain(t, nil)
		ref := refTravel()
		b.cfg.Travel.gear, b.cfg.Travel.sets, b.cfg.Travel.skills = ref.gear, ref.sets, ref.skills
		b.cfg.Travel.classes[7] = findFighter // Rogue
		b.world.object(pcID).ClassID = 7
		b.pvp.opponents = map[int32]*Opponent{pcID: {Row: charmodel.Row{Bow: true}}}
		_, opp, ok := b.pvpFighters(pcID)
		if !ok {
			t.Fatal("no fighters")
		}
		dagger, _ := b.pvpRef(b.cfg.Travel, 7, false, int(b.world.Self.Level))
		bow, _ := b.pvpRef(b.cfg.Travel, 7, true, int(b.world.Self.Level))
		if !opp.Row.Bow || opp.Profile.PAtk != bow.Profile.PAtk {
			t.Errorf("opponent row bow %v, P. Atk %v, want the bow reference's %v", opp.Row.Bow, opp.Profile.PAtk, bow.Profile.PAtk)
		}
		if bow.Profile.PAtk <= dagger.Profile.PAtk {
			t.Errorf("bow reference P. Atk %v, want above the dagger reference's %v", bow.Profile.PAtk, dagger.Profile.PAtk)
		}
	})
	t.Run("a new level rebuilds the reference", func(t *testing.T) {
		b := findBrain(t, nil)
		ref := refTravel()
		b.cfg.Travel.gear, b.cfg.Travel.sets, b.cfg.Travel.skills = ref.gear, ref.sets, ref.skills
		naked, _ := b.pvpRef(b.cfg.Travel, 0, false, 19)
		at20, _ := b.pvpRef(b.cfg.Travel, 0, false, 20)
		if at20.Profile.PAtk <= naked.Profile.PAtk || at20.Profile.PDef <= naked.Profile.PDef {
			t.Errorf("level 20 reference P. Atk %v, P. Def %v, want above level 19's naked %v, %v", at20.Profile.PAtk, at20.Profile.PDef, naked.Profile.PAtk, naked.Profile.PDef)
		}
	})
}

// TestRefGearShipped checks the shipped datapack (BOT_DATAPACK_XML names
// its data/xml directory) gives every default build a weapon, a body piece
// and a necklace at every grade, D to S.
func TestRefGearShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewTravelMap(dp, SupplyConfig{})
	for _, gc := range defaultGearClasses() {
		for grade := int32(1); grade <= 5; grade++ {
			items, _ := m.refGear(gc, grade)
			var weapon, body, neck bool
			for _, it := range items {
				switch slotOf(bodyParts[it.BodyPart]) {
				case slotWeapon:
					weapon = true
				case slotBody:
					body = true
				case slotNeck:
					neck = true
				}
			}
			if !weapon || !body || !neck {
				t.Errorf("%v %s grade %d: weapon %v, body %v, necklace %v", gc.Weapons, gc.Armor, grade, weapon, body, neck)
			}
		}
	}
}
