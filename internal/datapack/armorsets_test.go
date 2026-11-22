package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadArmorSetsSample(t *testing.T) {
	want := map[int32]ArmorSet{
		23: {Name: "Wooden Set", Chest: 23, Legs: 2386, Head: 43, SkillID: 3500},
		58: {Name: "Mithril Heavy Set", Chest: 58, Legs: 59, Head: 47, SkillID: 3502, Shield: 628, ShieldSkillID: 3543, Enchant6Skill: 3611},
		// No legs: a full armor chest.
		60: {Name: "Composite Set", Chest: 60, Head: 517, SkillID: 3512, Shield: 107, ShieldSkillID: 3546, Enchant6Skill: 3614},
	}
	got, err := LoadArmorSets(filepath.Join("testdata", "xml", "armorSets.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadArmorSets = %+v, want %+v", got, want)
	}
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dp.ArmorSets, want) {
		t.Fatalf("Load ArmorSets = %+v, want %+v", dp.ArmorSets, want)
	}
}

// armorSetXML is one <armorset> of name and chest.
func armorSetXML(name, chest string) string {
	return `<armorset name="` + name + `" chest="` + chest + `" legs="2" head="3" gloves="0" feet="0" skillId="4" shield="0" shieldSkillId="0" enchant6Skill="0"/>`
}

// TestLoadArmorSetsLaterWins checks that a later set of a chest replaces
// the earlier one, as in the server's table.
func TestLoadArmorSetsLaterWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "armorSets.xml")
	body := "<list>" + armorSetXML("First", "1") + armorSetXML("Second", "1") + "</list>"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadArmorSets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[1].Name != "Second" {
		t.Fatalf("LoadArmorSets = %+v, want the second set only", got)
	}
}

func TestLoadArmorSetsErrors(t *testing.T) {
	const all = `name="S" chest="1" legs="2" head="3" gloves="4" feet="5" skillId="6" shield="7" shieldSkillId="8" enchant6Skill="9"`
	drop := func(attr string) string {
		i := strings.Index(all, attr+`="`)
		j := strings.Index(all[i+len(attr)+2:], `"`)
		return "<list><armorset " + all[:i] + all[i+len(attr)+2+j+1:] + "/></list>"
	}
	tests := []struct {
		name, body, wantErr string
	}{
		{"malformed", "<list><armorset", "decode"},
		{"chest past int32", "<list>" + armorSetXML("S", "2147483648") + "</list>", `"chest"`},
		{"bad chest", "<list>" + armorSetXML("S", "x") + "</list>", `"chest"`},
	}
	for _, attr := range []string{"name", "chest", "legs", "head", "gloves", "feet", "skillId", "shield", "shieldSkillId", "enchant6Skill"} {
		tests = append(tests, struct{ name, body, wantErr string }{"no " + attr, drop(attr), `"` + attr + `"`})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "armorSets.xml")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadArmorSets(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), path) {
				t.Fatalf("LoadArmorSets = %v, want an error naming %s and %s", err, path, tt.wantErr)
			}
		})
	}
}

// TestLoadShippedArmorSets checks the reader on the shipped armorSets.xml
// when BOT_DATAPACK_XML names a real data/xml directory, and skips
// otherwise: the 51 sets, 25 of them full armor (no legs), every one
// complete with a chest.
func TestLoadShippedArmorSets(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	sets, err := LoadArmorSets(filepath.Join(dir, "armorSets.xml"))
	if err != nil {
		t.Fatal(err)
	}
	full := 0
	for chest, s := range sets {
		if chest == 0 || s.SkillID == 0 {
			t.Errorf("set %q: chest %d skill %d", s.Name, chest, s.SkillID)
		}
		if s.Legs == 0 {
			full++
		}
	}
	if len(sets) != 51 || full != 25 {
		t.Fatalf("shipped sets = %d, %d of them full armor; want 51, 25", len(sets), full)
	}
}
