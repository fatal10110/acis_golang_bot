package botclient

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Made-up armor of the model tests: a robe pair, a light pair, a heavy
// chest, and the Demon and Doom Robe sets' chest and legs (their real ids,
// the same stats each).
const (
	robeTop     int32 = 92001 // MAGIC, P. Def 20
	robeBottom  int32 = 92002 // MAGIC, P. Def 10
	lightTop    int32 = 92003 // LIGHT, P. Def 40
	lightBottom int32 = 92004 // LIGHT, P. Def 25
	demonChest  int32 = 441
	demonLegs   int32 = 472
	doomChest   int32 = 2399
	doomLegs    int32 = 2404
)

// Skills of the model tests (data/xml/skills): Spellcraft, Light Armor
// Mastery's first level, Wind Strike's, a heal, and the Demon and Doom
// Robe set skills, with their stat functions.
const (
	skillSpellcraft int32 = 163
	skillDemonSet   int32 = 3513
	skillDoomSet    int32 = 3529
)

var modelItems = func() map[int32]datapack.Item {
	m := make(map[int32]datapack.Item)
	add := func(id int32, part, kind string, price int32, pDef float64) {
		m[id] = datapack.Item{ID: id, Type: "Armor", BodyPart: part, Kind: kind, Price: price, Stats: datapack.Stats{PDef: pDef}}
	}
	add(robeTop, "chest", "MAGIC", 1000, 20)
	add(robeBottom, "legs", "MAGIC", 800, 10)
	add(lightTop, "chest", "LIGHT", 2000, 40)
	add(lightBottom, "legs", "LIGHT", 1500, 25)
	add(demonChest, "chest", "MAGIC", 5000, 50)
	add(demonLegs, "legs", "MAGIC", 4000, 30)
	add(doomChest, "chest", "MAGIC", 5000, 50)
	add(doomLegs, "legs", "MAGIC", 4000, 30)
	return m
}()

var modelSets = map[int32]datapack.ArmorSet{
	demonChest: {Name: "Demon Set", Chest: demonChest, Legs: demonLegs, SkillID: skillDemonSet},
	doomChest:  {Name: "Doom Robe Set", Chest: doomChest, Legs: doomLegs, SkillID: skillDoomSet},
}

var modelSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: skillSpellcraft, Level: 1}: {OperateType: "PASSIVE", Funcs: []datapack.StatFunc{{Op: "mul", Stat: "mAtkSpd", Value: 0.5, Using: []string{"MAGIC"}, NotUsing: true}}},
	{ID: skillLightArmorMastery, Level: 1}: {OperateType: "PASSIVE", Funcs: []datapack.StatFunc{
		{Op: "add", Stat: "pDef", Value: 12.6, Using: []string{"LIGHT"}},
		{Op: "mul", Stat: "mAtkSpd", Value: 1.9, Using: []string{"LIGHT"}},
		{Op: "mul", Stat: "pAtkSpd", Value: 1.25, Using: []string{"LIGHT"}},
	}},
	{ID: skillWindStrike, Level: 1}: {Type: "MDAM", OperateType: "ACTIVE", Power: 12, HitTime: 4 * time.Second, ReuseDelay: 6 * time.Second, MPConsume: 10},
	{ID: skillHeal, Level: 1}:       {Type: "HEAL", OperateType: "ACTIVE", Power: 49, HitTime: 5 * time.Second, ReuseDelay: 10 * time.Second, MPConsume: 10},
	{ID: skillDemonSet, Level: 1}: {OperateType: "PASSIVE", Funcs: []datapack.StatFunc{
		{Op: "sub", Stat: "maxHp", Value: 270}, {Op: "add", Stat: "INT", Value: 4}, {Op: "sub", Stat: "WIT", Value: 1},
	}},
	{ID: skillDoomSet, Level: 1}: {OperateType: "PASSIVE", Funcs: []datapack.StatFunc{
		{Op: "add", Stat: "runSpd", Value: 7}, {Op: "add", Stat: "INT", Value: 2}, {Op: "add", Stat: "MEN", Value: 1}, {Op: "sub", Stat: "WIT", Value: 3},
	}},
}

// modelClass is a class template of the shipped base stats and naked
// combat stats (data/xml/classes), HP hp and MP 40 plus 8 a level at every
// level.
func modelClass(id, parent, race int32, str, con, dex, intel, wit, men int, pAtk, pDef float64, hp float64) datapack.Class {
	c := datapack.Class{
		ID: id, Parent: parent, Race: race, STR: str, CON: con, DEX: dex, INT: intel, WIT: wit, MEN: men,
		PAtk: pAtk, PDef: pDef, MAtk: 6, MDef: 41, RunSpeed: 120,
	}
	for lvl := range 80 {
		c.HPTable = append(c.HPTable, hp)
		c.MPTable = append(c.MPTable, 40+8*float64(lvl))
	}
	return c
}

var modelClasses = map[int32]datapack.Class{
	classHumanMystic: modelClass(classHumanMystic, -1, 0, 22, 27, 21, 41, 20, 39, 3, 54, 500),
	11:               modelClass(11, classHumanMystic, 0, 22, 27, 21, 41, 20, 39, 3, 54, 500),
	classDarkMystic:  modelClass(classDarkMystic, -1, 2, 23, 24, 23, 44, 19, 37, 3, 54, 600),
	classOrcMystic:   modelClass(classOrcMystic, -1, 3, 27, 31, 24, 31, 15, 42, 3, 54, 400),
	0:                modelClass(0, -1, 0, 40, 43, 30, 21, 11, 25, 4, 80, 500),
	4:                modelClass(4, 0, 0, 40, 43, 30, 21, 11, 25, 4, 80, 500),
}

// modelTarget is the reference monster of the model tests: a level 20
// monster's final stats (indexRefMobs on the shipped datapack).
var modelTarget = charmodel.Profile{MaxHP: 540, PAtk: 57, MAtk: 24, PAtkSpd: 272, MAtkSpd: 333, Critical: 88, Accuracy: 52, Evasion: 52, PDef: 97, MDef: 79, RunSpeed: 92}

// newModelBrain is newGearBrain for a bot of class at level in role that
// knows skills, its gear valued by the model: the travel map holds the
// model tests' armor, sets, skills and class templates, modelTarget as
// every level's reference monster, and merchant 30001 selling sold. The
// build's own class table applies (no persona gear classes).
func newModelBrain(t *testing.T, class, level int32, role Role, skills []KnownSkill, sold []int32, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, 0, items...)
	b.persona.Gear.Classes = nil
	b.persona.Role = role
	b.world.Self.ClassID, b.world.Self.Level = class, level
	b.world.Apply(SkillList{Skills: skills}, t0)
	var products []datapack.Product
	for _, id := range sold {
		products = append(products, datapack.Product{ItemID: id, Price: modelItems[id].Price})
	}
	shops, cheapest := indexShops([]datapack.BuyList{{ID: 1, NPCID: gearMerchant, Products: products}})
	gear, shopGear := indexGear(modelItems, cheapest)
	sets, pieces := indexSets(modelSets, gear)
	b.cfg.Travel = &TravelMap{
		spawns: map[int32][]Point{gearMerchant: {{X: 5000}}}, shops: shops, cheapest: cheapest, gear: gear, shopGear: shopGear,
		sets: sets, setPieces: pieces, classes: modelClasses, skills: modelSkills, setSkills: indexSetSkills(modelSets),
		refMobs: map[int]charmodel.Profile{20: modelTarget},
	}
	return b
}

// carriedPiece is template id carried, object obj; worn is it equipped.
func carriedPiece(obj, id int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: 1, BodySlot: bodyParts[modelItems[id].BodyPart]}
}

func wornItem(obj, id int32) Item {
	it := carriedPiece(obj, id)
	it.Equipped = true
	return it
}

// TestRowOMArmorPair checks the armor kinds by race (rows OM and HM): an
// orc mystic in its Support role that knows Light Armor Mastery wears a
// light pair over its robes when the pair scores higher, both pieces; it
// keeps its robes without the mastery, in its Damage role, or when the
// light pair scores lower; a human mystic never leaves its robes.
func TestRowOMArmorPair(t *testing.T) {
	mastery := []KnownSkill{{ID: skillSpellcraft, Level: 1, Passive: true}, {ID: skillLightArmorMastery, Level: 1, Passive: true}}
	robes := []Item{wornItem(301, robeTop), wornItem(302, robeBottom), carriedPiece(311, lightTop), carriedPiece(312, lightBottom)}
	tests := []struct {
		name   string
		class  int32
		level  int32
		role   Role
		skills []KnownSkill
		weak   bool
		want   bool
	}{
		{"orc mystic support with the mastery", classOrcMystic, 7, RoleSupport, mastery, false, true},
		{"orc mystic support, lighter pair", classOrcMystic, 7, RoleSupport, mastery, true, false},
		{"orc mystic support below 7", classOrcMystic, 6, RoleSupport, mastery, false, false},
		{"orc mystic support without the mastery", classOrcMystic, 7, RoleSupport, mastery[:1], false, false},
		{"orc mystic damage", classOrcMystic, 7, RoleDamage, mastery, false, false},
		{"human mystic support", classHumanMystic, 7, RoleSupport, mastery, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newModelBrain(t, tt.class, tt.level, tt.role, tt.skills, nil, append(slices.Clone(robes), adenaStack(1000))...)
			if tt.weak {
				m := b.cfg.Travel
				for _, id := range []int32{lightTop, lightBottom} {
					g := m.gear[id]
					g.tmpl.Stats.PDef = 1
					m.gear[id] = g
				}
			}
			for _, it := range robes[2:] {
				if got := b.wearsNext(it); got != tt.want {
					t.Errorf("wearsNext(%d) = %v, want %v", it.TemplateID, got, tt.want)
				}
			}
			junk := b.junk()
			for _, it := range robes[2:] {
				if _, got := junk[it.ObjectID]; got == tt.want {
					t.Errorf("carried %d junk = %v, want %v", it.TemplateID, got, !tt.want)
				}
			}
		})
	}
}

// TestGearNoKindDroppingMix checks that a chest or legs of another kind
// than the other piece worn is never worn or bought alone: the server
// counts a chest's kind only with legs of the same kind (a robe top over
// light legs is no MAGIC and halves a mystic's cast speed); an orc mystic
// that may wear light armor buys a light pair whole.
func TestGearNoKindDroppingMix(t *testing.T) {
	mastery := []KnownSkill{{ID: skillSpellcraft, Level: 1, Passive: true}, {ID: skillLightArmorMastery, Level: 1, Passive: true}}
	tests := []struct {
		name     string
		class    int32
		role     Role
		items    []Item
		sold     []int32
		wantWear []int32
		wantBuy  []int32
	}{
		{"robe top over light legs", classHumanMystic, RoleSupport, []Item{wornItem(301, lightBottom), carriedPiece(311, robeTop)}, nil, nil, nil},
		{"robe top over an orc's light pair", classOrcMystic, RoleSupport, []Item{wornItem(301, lightTop), wornItem(302, lightBottom), carriedPiece(311, robeTop)}, nil, nil, nil},
		{"light top alone over robes", classOrcMystic, RoleSupport, []Item{wornItem(301, robeTop), wornItem(302, robeBottom), carriedPiece(311, lightTop)}, nil, nil, nil},
		{"light top alone for sale", classOrcMystic, RoleSupport, []Item{wornItem(301, robeTop), wornItem(302, robeBottom), adenaStack(100000)}, []int32{lightTop}, nil, nil},
		{"light pair for sale", classOrcMystic, RoleSupport, []Item{wornItem(301, robeTop), wornItem(302, robeBottom), adenaStack(100000)}, []int32{lightTop, lightBottom}, nil, []int32{lightTop, lightBottom}},
		{"light pair for sale, short of adena", classOrcMystic, RoleSupport, []Item{wornItem(301, robeTop), wornItem(302, robeBottom), adenaStack(12000)}, []int32{lightTop, lightBottom}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newModelBrain(t, tt.class, 7, tt.role, mastery, tt.sold, tt.items...)
			var wear []int32
			for _, it := range tt.items {
				if b.wearsNext(it) {
					wear = append(wear, it.TemplateID)
				}
			}
			if !slices.Equal(wear, tt.wantWear) {
				t.Errorf("wears %v, want %v", wear, tt.wantWear)
			}
			var buy []int32
			for _, r := range b.gearWants() {
				buy = append(buy, r.Item)
			}
			slices.Sort(buy)
			if !slices.Equal(buy, tt.wantBuy) {
				t.Errorf("buys %v, want %v", buy, tt.wantBuy)
			}
		})
	}
}

// TestRowDMSetSkill checks that sets are scored by their set skills
// (setBonus): of a Demon set (+4 INT, -1 WIT, -270 max HP) and a Doom Robe
// set (+2 INT, +1 MEN, -3 WIT, +7 run), of the same pieces' stats, the
// Doom Robe is worth more to a low-HP dark mystic; against a monster that
// deals no damage, where HP buys nothing, the Demon's INT is. A flat bonus
// (GearPolicy.SetBonus) values them alike.
func TestRowDMSetSkill(t *testing.T) {
	skills := []KnownSkill{{ID: skillSpellcraft, Level: 1, Passive: true}, {ID: skillWindStrike, Level: 1}}
	wearing := func(chest, legs int32) worn {
		var w worn
		w[slotBody], w[slotLegs] = wornItem(311, chest), wornItem(312, legs)
		return w
	}
	demon, doom := wearing(demonChest, demonLegs), wearing(doomChest, doomLegs)
	for _, tt := range []struct {
		name     string
		harmless bool
		flat     bool
		want     int // the sign of Doom Robe's bonus less Demon's
	}{
		{"low HP", false, false, 1},
		{"harmless target", true, false, -1},
		{"flat bonus", false, true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newModelBrain(t, classDarkMystic, 20, RoleDamage, skills, nil, wornItem(301, robeTop), wornItem(302, robeBottom))
			if tt.harmless {
				target := modelTarget
				target.PAtk = 0
				b.cfg.Travel.refMobs = map[int]charmodel.Profile{20: target}
			}
			if tt.flat {
				b.cfg.Travel.classes = nil
			}
			d := b.setBonus(&doom) - b.setBonus(&demon)
			if got := cmp.Compare(d, 0); got != tt.want {
				t.Errorf("Doom Robe's bonus less Demon's = %v, want sign %d", d, tt.want)
			}
		})
	}
}

// TestBuildMysticArmor checks the race profile's armor grants: an orc
// mystic's Support build may wear light armor from 7 and heavy from 14,
// its Damage build and other races' mystics none.
func TestBuildMysticArmor(t *testing.T) {
	kinds := func(bd Build) []string {
		var out []string
		for _, g := range bd.Race.MysticArmor {
			out = append(out, g.Kind)
		}
		return out
	}
	tests := []struct {
		race, class, level int32
		role               Role
		want               []string
	}{
		{3, classOrcMystic, 6, RoleSupport, nil},
		{3, classOrcMystic, 7, RoleSupport, []string{"LIGHT"}},
		{3, classOrcShaman, 20, RoleSupport, []string{"LIGHT", "HEAVY"}},
		{3, classOrcMystic, 14, RoleDamage, nil},
		{3, 44, 14, RoleSupport, nil},
		{0, classHumanMystic, 14, RoleSupport, nil},
	}
	for _, tt := range tests {
		if got := kinds(CompileBuild(tt.race, tt.class, tt.level, tt.role, BranchChoice{})); !slices.Equal(got, tt.want) {
			t.Errorf("CompileBuild(%d, %d, %d, %v) armor = %v, want %v", tt.race, tt.class, tt.level, tt.role, got, tt.want)
		}
	}
}

// TestGearModelFallback checks that the scores are the StatWeights scores
// where the model cannot value gear: no class template for the bot's
// class.
func TestGearModelFallback(t *testing.T) {
	b := newModelBrain(t, 99, 20, RoleDamage, nil, nil, wornItem(301, lightTop))
	if b.valuer() != nil {
		t.Fatal("valuer without a class template")
	}
	w := b.worn()
	g := b.cfg.Travel.gear[lightTop]
	c := b.gearClass()
	if got, want := b.pieceScore(&w, &g, w[slotBody]), c.weights().score(&g.tmpl, 0); got != want {
		t.Errorf("pieceScore = %v, want the StatWeights score %v", got, want)
	}
}
