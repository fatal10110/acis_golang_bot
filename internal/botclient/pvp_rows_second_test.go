package botclient

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// secondRowIDs are the 24 second rows' ids (docs/pvp/rows-second.md,
// charmodel.RowClasses tier 2).
func secondRowIDs() map[string]bool {
	ids := map[string]bool{}
	for _, class := range charmodel.RowClasses(2) {
		if r, ok := charmodel.OwnRow(class); ok {
			ids[r.ID] = true
		}
	}
	return ids
}

// checkSecondPvPRows checks the 24 second rows of the row table
// (pvpRowTable, docs/pvp/rows-second.md "Tests") at each level of
// secondRowTable against it: each row's damage, DOT and control steps,
// "skill" or "skill/vs", in the order the row plays them; the bow-paired
// rows (TH-HK, PW-SR, AW-PR) each with its own side's steps.
func checkSecondPvPRows(t *testing.T, classes map[int32]datapack.Class, skills map[datapack.SkillRef]datapack.Skill) {
	t.Helper()
	second := secondRowIDs()
	if len(second) != 24 {
		t.Fatalf("second rows %v, want 24", slices.Sorted(maps.Keys(second)))
	}
	m := &TravelMap{classes: classes, skills: skills}
	for _, level := range slices.Sorted(maps.Keys(secondRowTable)) {
		got := map[string]string{}
		for r, refs := range pvpRowTable(m, shippedKits, level) {
			if second[r.ID] {
				if r.Bow {
					t.Errorf("level %d: second row %+v with the bow flag", level, r)
				}
				got[r.ID] = refWords(t, skills, refs)
			}
		}
		if want := secondRowTable[level]; !maps.Equal(got, want) {
			for _, id := range slices.Sorted(maps.Keys(second)) {
				if got[id] != want[id] {
					t.Errorf("level %d row %s: steps\n%q\nwant\n%q", level, id, got[id], want[id])
				}
			}
		}
	}
}

// refWords is a row table row as "skill" or "skill/vs" words.
func refWords(t *testing.T, skills map[datapack.SkillRef]datapack.Skill, refs []charmodel.RefStep) string {
	t.Helper()
	ids := make([]string, len(refs))
	for i, ref := range refs {
		ids[i] = strconv.Itoa(int(skillOf(t, skills, ref.Skill)))
		if len(ref.Vs) > 0 {
			var vs strings.Builder
			for _, a := range ref.Vs {
				vs.WriteString(a.String())
			}
			ids[i] += "/" + vs.String()
		}
	}
	return strings.Join(ids, " ")
}

// secondRowTable is the 24 second rows' row table at a level with today's
// lists (X20, #718): each second profession's kit plays its first
// profession's PvP list (ClassKit.PvP) joined to its own M10 rotation
// (pvpSteps), or its own list once shipped (PL, TK, SK: X21; SS, BD:
// X22; GL, WL, DS, BH: X23; PW, AW: X24; SR, PR: X25; BI, EE: X28, whose
// Trance Int and Break steps stay outside the table). X21-X28 change a
// row's line when they ship its list, and add the level where a new step
// starts (55: Shackle and Arrest, O1; checkHeavyPvPRows checks GL, WL, DS,
// BH at 60 and 66; checkArcherPvPRows HK, SR, PR at 46 and 60).
var secondRowTable = map[int]map[string]string{
	40: {
		"AW": "129/CH 115/M 70 70 263 96 223 16",
		"BD": "129/CH 115/MA 105/M 70 70 223",
		"BH": "260 100",
		"BI": "1201/M 1184/M 1177",
		"DS": "260 100 190 255",
		"EE": "1206/M 1201/M 1184/M 1177",
		"GL": "1",
		"HK": "101 19 56",
		"PL": "70 70/CA 92 3",
		"PP": "1201/M 1184/M 1177",
		"PR": "105/M 129/CH 101 70 19 56",
		"PW": "102/CA 263 96 16",
		"SE": "1147 1206/M 1201/M 1168/CH 1184/M 1177",
		"SG": "1206/M 1206/A 1264 1175 1184/M 1264 1175 1274 1172 1235 1265 1275 1231",
		"SH": "1206/M 1206/A 1168/CH 1266 1160/M 1184/M 1266 1178 1172 1234 1147 1239 1267",
		"SK": "129/CH 115/MA 105/M 289 70 70 289 223 3",
		"SN": "1184/MA 1220/CH 1160/M 1222/MA 1184/M 1220 1274 1172 1147 1230 1275 1231",
		"SR": "102/M 101 19 56",
		"SS": "102/CA 3",
		"TH": "263 96 16",
		"TK": "102/CA 3",
		"TY": "54/CA 95/CA 281 120 284 280 54 29",
		"WC": "1090 1107 1107 1090 100 1245 1245 260",
		"WL": "48 36 245",
	},
	55: {
		"AW": "129/CH 115/M 70 70 263 96 223 16",
		"BD": "129/CH 115/MA 105/M 402/CHA 70 70 84 408 223",
		"BH": "260 100",
		"BI": "1201/M 1184/M 1177",
		"DS": "260 100 190 255",
		"EE": "1206/M 1201/M 1184/M 1177",
		"GL": "6 261 5 9 7 1",
		"HK": "101 19 24 56",
		"PL": "70 70/CA 92 403/CHA 400 3",
		"PP": "1201/M 1184/M 1177",
		"PR": "105/M 129/CH 101 70 19 56",
		"PW": "102/CA 263 96 16",
		"SE": "1147 1206/M 1201/M 1168/CH 1184/M 1177",
		"SG": "1206/M 1206/A 1264 1175 1184/M 1264 1175 1274 1172 1235 1237 1265 1275 1231",
		"SH": "1206/M 1206/A 1168/CH 1266 1160/M 1184/M 1266 1178 1172 1234 1147 1239 1148 1267 1176",
		"SK": "129/CH 115/MA 105/M 402/CHA 289 70 70 289 401 223 3",
		"SN": "1184/MA 1220/CH 1160/M 1222/MA 1184/M 1220 1274 1172 1147 1230 1275 1231",
		"SR": "102/M 101 19 24 56",
		"SS": "102/CA 402/CHA 3",
		"TH": "409 263 96 16",
		"TK": "102/CA 402/CHA 400 3",
		"TY": "54/CA 95/CA 281 120 35 17 284 280 54 29",
		"WC": "1090 1107 1107 1090 100 1245 1245 260",
		"WL": "48 36 245",
	},
	74: {
		"AW": "129/CH 115/M 70 70 321 263 96 223 16",
		"BD": "129/CH 115/MA 105/M 402/CHA 70 70 84 408 223",
		"BH": "260 100",
		"BI": "1201/M 1184/M 1177",
		"DS": "260 100 315 190 255",
		"EE": "1206/M 1201/M 1184/M 1177",
		"GL": "6 261 5 9 7 1",
		"HK": "101 19 24 56",
		"PL": "70 70/CA 92 403/CHA 400 3",
		"PP": "1201/M 1184/M 1177",
		"PR": "105/M 129/CH 101 70 314 19 56",
		"PW": "102/CA 321 263 96 16",
		"SE": "1147 1206/M 1201/M 1168/CH 1184/M 1177",
		"SG": "1206/M 1206/A 1264 1175 1184/M 1264 1175 1274 1172 1235 1237 1265 1295 1275 1231",
		"SH": "1206/M 1206/A 1168/CH 1266 1160/M 1184/M 1266 1178 1172 1234 1147 1239 1148 1267 1176",
		"SK": "129/CH 115/MA 105/M 402/CHA 289 70 70 289 401 279 223 3",
		"SN": "1184/MA 1220/CH 1160/M 1222/MA 1184/M 1220 1274 1172 1147 1230 1296 1275 1231",
		"SR": "102/M 101 19 24 56",
		"SS": "102/CA 402/CHA 3",
		"TH": "409 263 96 16",
		"TK": "102/CA 402/CHA 400 3",
		"TY": "54/CA 95/CA 281 120 35 17 284 280 54 29",
		"WC": "1090 1107 1107 1090 100 1245 1245 260",
		"WL": "320 48 36 245",
	},
}

// TestPvPRowTableSecond checks the row table's second rows on a test kit
// (docs/pvp/rows-second.md, X20): a bow-paired second row keeps its own
// side's steps (TH its dagger and unweaponed steps, HK its bow and
// unweaponed steps, each from its own kit), and a class without a row of
// its own (Dark Avenger, a third profession) adds none.
func TestPvPRowTableSecond(t *testing.T) {
	const (
		shot, blow, stun = int32(9311), int32(9312), int32(9313)
		th, hk, da, duel = int32(8), int32(9), int32(6), int32(88)
	)
	grants := []datapack.SkillGrant{{ID: shot, Level: 1, MinLevel: 1}, {ID: blow, Level: 1, MinLevel: 1}, {ID: stun, Level: 1, MinLevel: 1}}
	m := &TravelMap{
		skills: map[datapack.SkillRef]datapack.Skill{
			{ID: shot, Level: 1}: {Name: "shot", Type: "PDAM", Power: 10},
			{ID: blow, Level: 1}: {Name: "blow", Type: "BLOW", Power: 20},
			{ID: stun, Level: 1}: {Name: "stun", Type: "STUN", Effects: []datapack.Effect{{Name: "Stun", Time: 9}}},
		},
		classes: map[int32]datapack.Class{
			th: {ID: th, Parent: 7, Skills: grants}, hk: {ID: hk, Parent: 7, Skills: grants},
			da: {ID: da, Parent: 4, Skills: grants}, duel: {ID: duel, Parent: 2, Skills: grants},
		},
	}
	pvp := []Step{{Skill: shot, Weapons: []string{"BOW"}}, {Skill: blow, Weapons: []string{"DAGGER"}}, {Skill: stun}}
	kits := []map[int32]ClassKit{{th: {PvP: pvp}, hk: {PvP: pvp}, da: {PvP: pvp}, duel: {PvP: pvp}}}
	got := map[charmodel.Row]string{}
	for r, steps := range pvpRowTable(m, kits, 40) {
		names := make([]string, len(steps))
		for i, s := range steps {
			names[i] = s.Skill.Name
		}
		got[r] = strings.Join(names, " ")
	}
	want := map[charmodel.Row]string{{ID: "TH"}: "blow stun", {ID: "HK"}: "shot stun"}
	if !maps.Equal(got, want) {
		t.Errorf("row table %v, want %v", got, want)
	}
}

// TestPvPMixArcherRowGear checks that a mix row that shoots without the
// bow flag (an archer second row: a Treasure Hunter seen with a bow plays
// HK) gets the bow reference opponent (docs/pvp/model.md "Engage
// estimate", a bow where the row shoots), and its dagger row the dagger
// one.
func TestPvPMixArcherRowGear(t *testing.T) {
	const th = int32(8)
	c := findFighter
	c.ID = th
	m := refTravel()
	m.classes = map[int32]datapack.Class{th: c}
	const level = 20
	bow, _ := m.refFighter(th, true, level)
	dagger, _ := m.refFighter(th, false, level)
	if bow.Profile.PAtk == dagger.Profile.PAtk {
		t.Fatalf("bow and dagger references both P. Atk %v", bow.Profile.PAtk)
	}
	for _, tc := range []struct {
		row  string
		want float64
	}{{"HK", bow.Profile.PAtk}, {"TH", dagger.Profile.PAtk}} {
		mix := mixShares(m, map[mixRow]int{{th, charmodel.Row{ID: tc.row}}: 1}, level)
		if len(mix) != 1 || mix[0].Opponent.Profile.PAtk != tc.want || mix[0].Opponent.Row.ID != tc.row {
			t.Errorf("row %s: mix %+v, want one opponent of P. Atk %v", tc.row, mix, tc.want)
		}
	}
}
