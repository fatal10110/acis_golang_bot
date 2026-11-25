package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestLoadZonesContains checks each zone kind Load reads from the fixture
// zones directory with a position in and one out: NPoly prisms with their
// minZ/maxZ band, a concave ring's notch, and Rune's reversed-corner Cuboid
// (TownZone.xml: x 47150 then 32531, y -44815 then -52045) holding a point
// between its corners and its own edges.
func TestLoadZonesContains(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	zs := dp.Zones
	tests := []struct {
		name    string
		zones   []Zone
		x, y, h int32
		want    bool
	}{
		{"arena in", zs.Arenas, 12400, 183500, -3500, true},
		{"arena above maxZ", zs.Arenas, 12400, 183500, -3395, false},
		{"arena beside", zs.Arenas, 11000, 183500, -3500, false},
		{"peace in", zs.Peace, 500, 1500, 0, true},
		{"peace concave notch", zs.Peace, 1500, 1500, 0, false},
		{"peace below minZ", zs.Peace, 500, 500, -101, false},
		{"town Giran", zs.Towns, 80000, 145000, -3500, true},
		{"town beside Giran", zs.Towns, 76000, 145000, -3500, false},
		{"town Rune between corners", zs.Towns, 40000, -48000, 0, true},
		{"town Rune low corner", zs.Towns, 32531, -52045, -1200, true},
		{"town Rune high corner", zs.Towns, 47150, -44815, 3000, true},
		{"town Rune west of it", zs.Towns, 32530, -48000, 0, false},
		{"town Rune above it", zs.Towns, 40000, -48000, 3001, false},
		{"derby track in", zs.DerbyTracks, 12500, 181500, -3600, true},
		{"derby track above", zs.DerbyTracks, 12500, 181500, -3300, false},
		{"castle battlefield in", zs.Sieges, -18000, 110000, 0, true},
		{"castle battlefield beside", zs.Sieges, -25000, 110000, 0, false},
		{"clan hall battlefield in", zs.Sieges, 44000, 108500, -2000, true},
		{"clan hall battlefield beside its apex", zs.Sieges, 43100, 109900, -2000, false},
		{"no-restart in", zs.NoRestart, 0, 0, 0, true},
		{"no-restart beside", zs.NoRestart, 1001, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := FindZone(tt.zones, tt.x, tt.y, tt.h); got != tt.want {
				t.Errorf("FindZone(%d, %d, %d) = %v, want %v", tt.x, tt.y, tt.h, got, tt.want)
			}
		})
	}
}

// TestLoadZonesSettings checks the settings each kind reads: a town's ids
// and isPeaceZone (true by default), a battlefield's castle or clan hall;
// and the Cuboid normalized to its low and high corner.
func TestLoadZonesSettings(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	type town struct {
		TownID, CastleID int
		Peaceful, Cuboid bool
	}
	var towns []town
	for _, z := range dp.Zones.Towns {
		towns = append(towns, town{z.TownID, z.CastleID, z.Peaceful, z.Cuboid})
	}
	if want := []town{{9, 3, true, false}, {14, 8, true, true}, {20, 0, false, false}}; !reflect.DeepEqual(towns, want) {
		t.Errorf("towns = %+v, want %+v", towns, want)
	}
	rune := dp.Zones.Towns[1]
	if want := (Zone{Cuboid: true, X: []int32{32531, 47150}, Y: []int32{-52045, -44815}, MinZ: -1200, MaxZ: 3000, TownID: 14, CastleID: 8, Peaceful: true}); !reflect.DeepEqual(rune, want) {
		t.Errorf("Rune = %+v, want %+v", rune, want)
	}
	var sieges [][2]int
	for _, z := range dp.Zones.Sieges {
		sieges = append(sieges, [2]int{z.ResidenceCastle, z.ResidenceClanHall})
	}
	if want := [][2]int{{1, 0}, {0, 21}}; !reflect.DeepEqual(sieges, want) {
		t.Errorf("siege residences = %v, want %v", sieges, want)
	}
	for _, kind := range [][]Zone{dp.Zones.Arenas, dp.Zones.Peace, dp.Zones.DerbyTracks, dp.Zones.NoRestart} {
		if len(kind) != 1 || kind[0].Cuboid || kind[0].TownID != 0 || kind[0].Peaceful {
			t.Errorf("zones = %+v, want one plain NPoly", kind)
		}
	}
}

// TestZoneContainsEdges pins the server's edge rules: an NPoly ring is
// half-open (its low X and low Y edges in, its high ones out) and an NPoly
// band with minZ over maxZ is kept as written, holding no point; a Cuboid
// holds all four edges and swaps a reversed band.
func TestZoneContainsEdges(t *testing.T) {
	dir := writeZoneFiles(t, map[string]string{
		"PeaceZone": `<zone shape="NPoly" minZ="0" maxZ="10"><node x="-10" y="-10"/><node x="10" y="-10"/><node x="10" y="10"/><node x="-10" y="10"/></zone>
			<zone shape="NPoly" minZ="10" maxZ="0"><node x="-10" y="-10"/><node x="10" y="-10"/><node x="10" y="10"/></zone>`,
		"ArenaZone": `<zone shape="Cuboid" minZ="10" maxZ="0"><node x="10" y="10"/><node x="-10" y="-10"/></zone>`,
	})
	zs, err := LoadZones(dir)
	if err != nil {
		t.Fatal(err)
	}
	poly, inverted, box := zs.Peace[0], zs.Peace[1], zs.Arenas[0]
	tests := []struct {
		name    string
		z       Zone
		x, y, h int32
		want    bool
	}{
		{"ring low X edge", poly, -10, 0, 5, true},
		{"ring high X edge", poly, 10, 0, 5, false},
		{"ring low Y edge", poly, 0, -10, 5, true},
		{"ring high Y edge", poly, 0, 10, 5, false},
		{"ring Z band edges", poly, 0, 0, 10, true},
		{"inverted band", inverted, 5, -5, 5, false},
		{"box high corner", box, 10, 10, 10, true},
		{"box low corner", box, -10, -10, 0, true},
		{"box past X", box, 11, 0, 5, false},
	}
	for _, tt := range tests {
		if got := tt.z.Contains(tt.x, tt.y, tt.h); got != tt.want {
			t.Errorf("%s: Contains(%d, %d, %d) = %v, want %v", tt.name, tt.x, tt.y, tt.h, got, tt.want)
		}
	}
}

// TestLoadZonesRejects checks that a shape other than NPoly and Cuboid, a
// wrong node count, a mangled field or a missing file fails the load.
func TestLoadZonesRejects(t *testing.T) {
	tests := []struct {
		name, file, body, want string
	}{
		{"cylinder", "ArenaZone", `<zone shape="Cylinder" minZ="0" maxZ="10" rad="50"><node x="0" y="0"/></zone>`, `shape "Cylinder" is not modeled`},
		{"unknown shape", "TownZone", `<zone shape="Box" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/></zone>`, `shape "Box" is not modeled`},
		{"polygon of two nodes", "PeaceZone", `<zone shape="NPoly" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/></zone>`, "more than 2 nodes, has 2"},
		{"cuboid of three nodes", "SiegeZone", `<zone shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/><node x="9" y="9"/></zone>`, "wants 2 nodes, has 3"},
		{"no nodes", "DerbyTrackZone", `<zone shape="NPoly" minZ="0" maxZ="10"/>`, "no nodes"},
		{"no shape", "NoRestartZone", `<zone minZ="0" maxZ="10"><node x="0" y="0"/></zone>`, `"shape"`},
		{"no maxZ", "ArenaZone", `<zone shape="NPoly" minZ="0"><node x="0" y="0"/><node x="5" y="0"/><node x="5" y="5"/></zone>`, `"maxZ"`},
		{"node without y", "ArenaZone", `<zone shape="NPoly" minZ="0" maxZ="1"><node x="0"/><node x="5" y="0"/><node x="5" y="5"/></zone>`, `"y"`},
		{"town id", "TownZone", `<zone shape="NPoly" minZ="0" maxZ="1"><stat name="townId" val="x"/><node x="0" y="0"/><node x="5" y="0"/><node x="5" y="5"/></zone>`, `"townId"`},
		{"clan hall id", "SiegeZone", `<zone shape="NPoly" minZ="0" maxZ="1"><stat name="clanHallId" val="1.5"/><node x="0" y="0"/><node x="5" y="0"/><node x="5" y="5"/></zone>`, `"clanHallId"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadZones(writeZoneFiles(t, map[string]string{tt.file: tt.body}))
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), tt.file) {
				t.Fatalf("LoadZones err = %v, want one naming %s and %s", err, tt.file, tt.want)
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		dir := writeZoneFiles(t, nil)
		if err := os.Remove(filepath.Join(dir, "NoRestartZone.xml")); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadZones(dir); err == nil || !strings.Contains(err.Error(), "NoRestartZone.xml") {
			t.Fatalf("LoadZones err = %v, want one naming NoRestartZone.xml", err)
		}
	})
}

// writeZoneFiles writes the six zone files LoadZones reads to a new
// directory, each holding bodies[its base name] inside <list>.
func writeZoneFiles(t *testing.T, bodies map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"ArenaZone", "PeaceZone", "TownZone", "DerbyTrackZone", "SiegeZone", "NoRestartZone"} {
		if err := os.WriteFile(filepath.Join(dir, name+".xml"), []byte("<list>"+bodies[name]+"</list>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestLoadShippedZones checks the reader on the shipped zone files when
// BOT_DATAPACK_XML names a real data/xml directory, and skips otherwise:
// 4 arenas, 117 peace zones, 18 towns with Rune's Cuboid, 8 Derby track
// zones, 15 battlefields (9 castles, 6 clan halls) and 6 no-restart zones.
func TestLoadShippedZones(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	zs, err := LoadZones(filepath.Join(dir, "zones"))
	if err != nil {
		t.Fatal(err)
	}
	got := [6]int{len(zs.Arenas), len(zs.Peace), len(zs.Towns), len(zs.DerbyTracks), len(zs.Sieges), len(zs.NoRestart)}
	if want := [6]int{4, 117, 18, 8, 15, 6}; got != want {
		t.Errorf("arena, peace, town, derby, siege, no-restart zones = %v, want %v", got, want)
	}
	rune, ok := FindZone(zs.Towns, 40000, -48000, 0)
	if !ok || !rune.Cuboid || rune.TownID != 14 {
		t.Errorf("town at Rune = %+v, %v; want the Cuboid of town 14", rune, ok)
	}
	castles, halls := 0, 0
	for _, z := range zs.Sieges {
		switch {
		case z.ResidenceCastle != 0 && z.ResidenceClanHall == 0:
			castles++
		case z.ResidenceClanHall != 0 && z.ResidenceCastle == 0:
			halls++
		}
	}
	if castles != 9 || halls != 6 {
		t.Errorf("battlefields: %d castles, %d clan halls; want 9, 6", castles, halls)
	}
}

// TestLoadSkillsPvP checks the landing, cure and PvP fields: Sting's
// effectAbnormalLvl from its table (3 at level 1, 8 at 45) with abnormalLvl
// at its -1 default, its effect's own power and type; offensive defaulted
// from the skill type, isDebuff or a CORPSE_MOB target, or set; the cures'
// negateStats and negateLvl; the vulnerabilities of Lionheart's and Mental
// Shield's Buff effects and of a PvP jewel's skill; and the Special
// Ability's pvp*Dmg.
func TestLoadSkillsPvP(t *testing.T) {
	skills, err := LoadSkills(filepath.Join("testdata", "pvpskills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		level, abnormal, magicLvl int
		bleed                     float64
	}{{1, 3, 22, 39}, {45, 8, 70, 102}} {
		sk := skills[SkillRef{223, int32(tt.level)}]
		if sk.EffectAbnormalLevel != tt.abnormal || sk.AbnormalLevel != -1 || sk.MagicLevel != tt.magicLvl || sk.LevelDepend != 1 {
			t.Errorf("Sting %d: effectAbnormalLvl %d abnormalLvl %d magicLvl %d lvlDepend %d, want %d, -1, %d, 1",
				tt.level, sk.EffectAbnormalLevel, sk.AbnormalLevel, sk.MagicLevel, sk.LevelDepend, tt.abnormal, tt.magicLvl)
		}
		if !sk.Debuff || !sk.Offensive || !sk.NextActionAttack || sk.Magic || sk.EffectType != "BLEED" || sk.EffectPower != 0 ||
			sk.Target != "ONE" || sk.Radius != 80 || sk.NegateStats != nil || sk.NegateLevel != -1 {
			t.Errorf("Sting %d = %+v", tt.level, sk)
		}
		want := []Effect{{
			Name: "DamOverTime", StackType: "bleeding", StackOrder: float64(tt.abnormal), Time: 3, Count: 7, Value: tt.bleed,
			EffectType: "BLEED", EffectPower: 50,
		}}
		if !reflect.DeepEqual(sk.Effects, want) {
			t.Errorf("Sting %d effects = %+v, want %+v", tt.level, sk.Effects, want)
		}
	}

	corpse, harmless := skills[SkillRef{9001, 1}], skills[SkillRef{9002, 1}]
	if !corpse.Offensive || corpse.Radius != 150 || corpse.AbnormalLevel != 2 || corpse.EffectPower != 80 {
		t.Errorf("Corpse Buff = %+v, want offensive, radius 150, abnormalLvl 2, effectPower 80", corpse)
	}
	if harmless.Offensive || harmless.Target != "AREA" {
		t.Errorf("Harmless Strike = %+v, want offensive set false", harmless)
	}
	if ms := skills[SkillRef{1035, 1}]; !ms.Magic || ms.Offensive {
		t.Errorf("Mental Shield = %+v, want a magic, harmless buff", ms)
	}

	for _, tt := range []struct {
		id    int32
		stats []string
		level int
	}{{2042, []string{"POISON"}, 3}, {2045, []string{"BLEED"}, 7}} {
		sk := skills[SkillRef{tt.id, 1}]
		if !reflect.DeepEqual(sk.NegateStats, tt.stats) || sk.NegateLevel != tt.level {
			t.Errorf("cure %d: negateStats %v negateLvl %d, want %v, %d", tt.id, sk.NegateStats, sk.NegateLevel, tt.stats, tt.level)
		}
	}

	vuln := func(v float64, stats ...string) []StatFunc {
		out := make([]StatFunc, len(stats))
		for i, s := range stats {
			out[i] = StatFunc{Op: "addMul", Stat: s, Value: v}
		}
		return out
	}
	lionheart3 := append(vuln(40, "cancelVuln"), vuln(80, "stunVuln", "rootVuln", "sleepVuln", "paralyzeVuln")...)
	pvpDmg := []StatFunc{
		{Op: "add", Stat: "atkCountMax", Value: 4},
		{Op: "mul", Stat: "pvpPhysDmg", Value: 1.05},
		{Op: "mul", Stat: "pvpPhysSkillsDmg", Value: 1.05},
		{Op: "mul", Stat: "pvpMagicalDmg", Value: 1.05},
	}
	for _, tt := range []struct {
		name string
		got  []StatFunc
		want []StatFunc
	}{
		{"Lionheart 3 effect", effectFuncs(skills[SkillRef{287, 3}]), lionheart3},
		{"Mental Shield 2 effect", effectFuncs(skills[SkillRef{1035, 2}]), vuln(30, "rootVuln", "sleepVuln", "derangementVuln")},
		{"Earring of Antharas", skills[SkillRef{3558, 1}].Funcs, append([]StatFunc{{Op: "addMul", Stat: "earthRes", Value: 15}},
			append(vuln(80, "bleedVuln"), vuln(60, "stunVuln", "derangementVuln")...)...)},
		{"Special Ability: PvP Damage 2", skills[SkillRef{3552, 2}].Funcs, pvpDmg},
	} {
		if !reflect.DeepEqual(tt.got, tt.want) {
			t.Errorf("%s funcs = %+v, want %+v", tt.name, tt.got, tt.want)
		}
	}
	if f := skills[SkillRef{287, 1}].Funcs; f != nil {
		t.Errorf("Lionheart's own funcs = %+v, want none: its vulnerabilities are its effect's", f)
	}
}

// effectFuncs returns the stat functions of sk's only effect.
func effectFuncs(sk Skill) []StatFunc {
	if len(sk.Effects) != 1 {
		return nil
	}
	return sk.Effects[0].Funcs
}

// TestLoadItemSkills checks that a weapon's and an armor piece's item_skill
// are read: a PvP weapon's Special Ability, a PvP jewel's skill.
func TestLoadItemSkills(t *testing.T) {
	dir := t.TempDir()
	body := `<list>
		<item id="4809" type="Weapon" name="Blood Tornado - Haste"><set name="weapon_type" val="DUALFIST"/><set name="item_skill" val="3068-2"/></item>
		<item id="6656" type="Armor" name="Earring of Antharas"><set name="bodypart" val="rear;lear"/><set name="item_skill" val="3558-1"/></item>
		<item id="49" type="Armor" name="Gloves"><set name="bodypart" val="gloves"/></item>
		<item id="9" type="Weapon" name="x"><set name="item_skill" val="3068"/></item>
	</list>`
	if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadItems(dir); err == nil || !strings.Contains(err.Error(), "item 9: item_skill") {
		t.Fatalf("LoadItems err = %v, want item 9's item_skill refused", err)
	}
	body = strings.Replace(body, `<item id="9" type="Weapon" name="x"><set name="item_skill" val="3068"/></item>`, "", 1)
	if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := LoadItems(dir)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int32][]SkillRef{4809: {{3068, 2}}, 6656: {{3558, 1}}, 49: nil} {
		if got := items.Items[id].Skills; !reflect.DeepEqual(got, want) {
			t.Errorf("item %d skills = %v, want %v", id, got, want)
		}
	}
}

// TestLoadShippedPvPFields checks the PvP fields on the shipped datapack
// when BOT_DATAPACK_XML names a real data/xml directory, and skips
// otherwise: Sting's effectAbnormalLvl, the cures, Lionheart's effect
// vulnerabilities, a PvP jewel's and Mithril Heavy's through their skills,
// and the 143 weapons (A 99, S 44) whose Special Ability raises
// pvpPhysDmg: the item skill level of 57 more sets it to 1.
func TestLoadShippedPvPFields(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for level, want := range map[int32]int{1: 3, 45: 8} {
		if sk := dp.Skills[SkillRef{223, level}]; sk.EffectAbnormalLevel != want || sk.AbnormalLevel != -1 || !sk.Offensive {
			t.Errorf("Sting %d: effectAbnormalLvl %d abnormalLvl %d offensive %v, want %d, -1, true", level, sk.EffectAbnormalLevel, sk.AbnormalLevel, sk.Offensive, want)
		}
	}
	for id, want := range map[int32]int{2042: 3, 2043: 7, 2044: 3, 2045: 7} {
		if sk := dp.Skills[SkillRef{id, 1}]; len(sk.NegateStats) != 1 || sk.NegateLevel != want {
			t.Errorf("cure %d: negateStats %v negateLvl %d, want one type up to %d", id, sk.NegateStats, sk.NegateLevel, want)
		}
	}
	has := func(funcs []StatFunc, stat string, v float64) bool {
		for _, f := range funcs {
			if f.Stat == stat && f.Value == v {
				return true
			}
		}
		return false
	}
	if !has(effectFuncs(dp.Skills[SkillRef{287, 3}]), "stunVuln", 80) || !has(effectFuncs(dp.Skills[SkillRef{1035, 4}]), "derangementVuln", 50) {
		t.Error("Lionheart 3 or Mental Shield 4 lacks its effect's vulnerability")
	}
	itemFuncs := func(id int32) []StatFunc {
		var out []StatFunc
		for _, ref := range dp.Items[id].Skills {
			out = append(out, dp.Skills[ref].Funcs...)
		}
		return out
	}
	if !has(itemFuncs(8191), "sleepVuln", 30) || !has(itemFuncs(6656), "bleedVuln", 80) {
		t.Error("Frintezza's Necklace or Earring of Antharas lacks its vulnerability")
	}
	if set := dp.ArmorSets[58]; !has(dp.Skills[SkillRef{set.SkillID, 1}].Funcs, "poisonVuln", 20) {
		t.Errorf("Mithril Heavy set skill %d lacks poisonVuln 20", set.SkillID)
	}
	pvp := 0
	for id, it := range dp.Items {
		if it.Type == "Weapon" && has(itemFuncs(id), "pvpPhysDmg", 1.05) {
			pvp++
		}
	}
	if pvp != 143 {
		t.Errorf("%d weapons raise pvpPhysDmg by 1.05, want 143", pvp)
	}
}
