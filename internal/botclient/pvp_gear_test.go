package botclient

import (
	"maps"
	"math"
	"os"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPvPGearBlend checks PvP value in a gear score (docs/pvp/model.md
// "Blend"): with GearWeight 0 a piece scores its value gain alone and the
// model keeps no PvP side; with 0.5 it scores Blend of its value and PvP
// value gains over the mix, where an M. Def necklace raises PvP value
// against a mystic casting Wind Strike and leaves it against a fighter's
// plain attacks.
func TestPvPGearBlend(t *testing.T) {
	const neck int32 = 92101 // a necklace of M. Def 30
	wind := modelSkills[datapack.SkillRef{ID: skillWindStrike, Level: 1}]
	wind.Magic = true
	tests := []struct {
		name   string
		weight float64
		foe    int32
		pvp    int // the sign of the necklace's PvP value gain
	}{
		{name: "w 0: the value gain alone", weight: 0, foe: classHumanMystic},
		{name: "w 0.5 against a mystic: M. Def raises PvP value", weight: 0.5, foe: classHumanMystic, pvp: 1},
		{name: "w 0.5 against a fighter: M. Def adds no PvP value", weight: 0.5, foe: 0, pvp: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newModelBrain(t, 0, 20, RoleDamage, nil, nil)
			b.persona.PvP.GearWeight = tc.weight
			m := b.cfg.Travel
			items := maps.Clone(modelItems)
			items[neck] = datapack.Item{ID: neck, Type: "Armor", BodyPart: "neck", Price: 1000, Stats: datapack.Stats{MDef: 30}}
			m.gear, m.shopGear = indexGear(items, m.cheapest)
			b.pvp.rows, b.pvp.rowsLevel = charmodel.RowTable{{ID: "HM"}: {{Skill: wind}}}, 20
			b.handleOne(t, CharInfo{ObjectID: 0x1000, Name: "Foe", ClassID: tc.foe, Pos: Point{Y: 900}}, t0)
			v := b.valuer()
			if v == nil {
				t.Fatal("no gear model")
			}
			if (v.pvp != nil) != (tc.weight > 0) {
				t.Fatalf("PvP side %v with GearWeight %v", v.pvp != nil, tc.weight)
			}
			g := m.gear[neck]
			it := Item{ObjectID: 0x2000, TemplateID: neck, Count: 1, BodySlot: g.part}
			w := b.worn()
			base := w
			base[slotOf(g.part)] = Item{}
			with := base
			with[slotOf(g.part)] = it
			d := v.build.Dyes
			dv := v.value(m, &with, false, d) - v.value(m, &base, false, d)
			want := dv / v.norm
			if v.pvp != nil {
				if len(v.pvp.mix) != 1 || v.pvp.mix[0].Share != 1 {
					t.Fatalf("mix %+v, want the one player seen", v.pvp.mix)
				}
				dp := v.pvpValue(m, &with, false, d) - v.pvpValue(m, &base, false, d)
				if sign(dp) != tc.pvp {
					t.Fatalf("PvP value gain %v, want sign %d", dp, tc.pvp)
				}
				want = charmodel.Blend(v.norm, dv, v.pvp.norm, dp, tc.weight) / v.norm
			}
			class := b.gearClass()
			want += statTieBreak * class.weights().score(&g.tmpl, 0)
			if got := b.pieceScore(&w, &g, it); got != want {
				t.Errorf("pieceScore = %v, want %v", got, want)
			}
		})
	}
}

func sign(x float64) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

// TestPvPGearMithrilHeavy checks the shipped rows in PvP value
// (docs/pvp/model.md "Tests", shipped rows) against the shipped datapack
// when BOT_DATAPACK_XML names its data/xml directory, and skips otherwise:
// the Mithril Heavy pieces with their set skill 3502 (poisonVuln only,
// skills/3500-3599.xml:22-30) beat the same pieces without it on a Cleric
// against the poison rows, are equal on a Warrior, and equal on either
// against rows that never poison it.
func TestPvPGearMithrilHeavy(t *testing.T) {
	if os.Getenv("BOT_DATAPACK_XML") == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	shipped, err := shippedTravel()
	if err != nil {
		t.Fatal(err)
	}
	const (
		level   = 30
		cleric  = int32(15)
		warrior = int32(1)
	)
	poisoners := []int32{10, 38, 42, 39, 32, 35, 49} // HM, DM, SO, DW, PK, AS, OM
	others := []int32{0, 1, 4, 11, 15}               // HF, WA, KN, HW, CL
	var w worn
	w[slotBody] = Item{TemplateID: 58, BodySlot: partChest}
	w[slotLegs] = Item{TemplateID: 59, BodySlot: partLegs}
	w[slotHead] = Item{TemplateID: 47, BodySlot: partHead}
	if fs := shipped.setFuncs(&w); len(fs) == 0 {
		t.Fatal("the Mithril Heavy pieces complete no set")
	}
	table := pvpRowTable(shipped, shippedKits, level)
	value := func(class int32, foes []int32, sets bool) float64 {
		rows := make(map[mixRow]int)
		for _, id := range foes {
			row, _ := charmodel.ClassRow(id, false)
			rows[mixRow{id, row}] = 1
		}
		v := &gearModel{
			in:    gearInputs{key: buildKey{class: class, level: level}},
			build: charmodel.Build{Class: shipped.classes[class], Level: level, Mage: charmodel.MageClass(shipped.classes, class)},
			pvp:   &gearPvP{mix: mixShares(shipped, rows, level), table: table, values: make(map[gearValueKey]float64)},
		}
		return v.pvpValue(shipped, &w, sets, charmodel.Attrs{})
	}
	if with, without := value(cleric, poisoners, true), value(cleric, poisoners, false); with <= without {
		t.Errorf("Cleric against the poison rows: %v with set skill 3502, %v without; want it higher", with, without)
	}
	if with, without := value(warrior, poisoners, true), value(warrior, poisoners, false); math.Abs(with-without) > 1e-9 {
		t.Errorf("Warrior against the poison rows: %v with set skill 3502, %v without; want them equal", with, without)
	}
	for _, class := range []int32{cleric, warrior} {
		if with, without := value(class, others, true), value(class, others, false); math.Abs(with-without) > 1e-9 {
			t.Errorf("class %d against HF, WA, KN, HW, CL: %v with set skill 3502, %v without; want them equal", class, with, without)
		}
	}
}
