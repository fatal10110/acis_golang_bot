package datapack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadSkillReagents checks the item a skill level consumes per cast:
// itemConsumeId as a plain value, itemConsumeCount from a #table by level,
// as Mana Regeneration's Spirit Ore (skills/1000-1099.xml); a skill with
// neither reads 0 for both.
func TestLoadSkillReagents(t *testing.T) {
	dir := t.TempDir()
	body := `<list>
<skill id="1047" levels="4" name="Mana Regeneration">
	<table name="#itemConsumeCount"> 7 9 12 15 </table>
	<set name="itemConsumeId" val="3031"/>
	<set name="itemConsumeCount" val="#itemConsumeCount"/>
</skill>
<skill id="1177" levels="1" name="Wind Strike"/>
</list>`
	if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	skills, err := LoadSkills(dir)
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	for _, tt := range []struct {
		ref   SkillRef
		id    int32
		count int
	}{
		{SkillRef{1047, 1}, 3031, 7},
		{SkillRef{1047, 2}, 3031, 9},
		{SkillRef{1047, 3}, 3031, 12},
		{SkillRef{1047, 4}, 3031, 15},
		{SkillRef{1177, 1}, 0, 0},
	} {
		sk := skills[tt.ref]
		if sk.ItemConsumeID != tt.id || sk.ItemConsumeCount != tt.count {
			t.Errorf("%v consumes %d x%d, want %d x%d", tt.ref, sk.ItemConsumeID, sk.ItemConsumeCount, tt.id, tt.count)
		}
	}
}

// TestLoadSkillReagentsRejects checks a mangled itemConsumeId or
// itemConsumeCount fails the load with the field named.
func TestLoadSkillReagentsRejects(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
	}{
		{"id", `<skill id="1" levels="1" name="x"><set name="itemConsumeId" val="ore"/></skill>`, `"itemConsumeId"`},
		{"count", `<skill id="1" levels="1" name="x"><set name="itemConsumeCount" val="1.5"/></skill>`, `"itemConsumeCount"`},
		{"count table", `<skill id="1" levels="2" name="x"><table name="#c"> 1 </table><set name="itemConsumeCount" val="#c"/></skill>`, "no row 2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSkills(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadSkills err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}
