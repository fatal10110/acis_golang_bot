package datapack

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadSummonTemplates checks that a summon skill's level names its
// servitor's NPC template through npcId and its #npcIds table, and that the
// template loads with the stats the model reads: Summon Kai the Cat (1276,
// skills/1200-1299.xml) levels 1 and 14 and their cats 14207 and 14220
// (npcs/14000-14999.xml); a skill without npcId reads 0.
func TestLoadSummonTemplates(t *testing.T) {
	root := t.TempDir()
	skills := `<list>
<skill id="1276" levels="14" name="Summon Kai the Cat">
	<table name="#npcIds"> 14207 14208 14209 14210 14211 14212 14213 14214 14215 14216 14217 14218 14219 14220 </table>
	<set name="npcId" val="#npcIds" />
	<set name="itemConsumeId" val="1459"/>
	<set name="skillType" val="SUMMON"/>
</skill>
<skill id="1177" levels="1" name="Wind Strike"/>
</list>`
	npcs := `<list>
<npc id="14207" name="Kai the Cat">
	<set name="type" val="Servitor"/>
	<set name="level" val="42"/>
	<set name="hp" val="1050.81652"/>
	<set name="pAtk" val="198.34591"/>
	<set name="pDef" val="164.88513"/>
	<set name="mAtk" val="135.44454"/>
	<set name="mDef" val="120.65578"/>
	<set name="crit" val="4.0"/>
	<set name="atkSpd" val="253.0"/>
	<set name="str" val="40"/>
	<set name="dex" val="30"/>
	<set name="con" val="43"/>
</npc>
<npc id="14220" name="Kai the Cat">
	<set name="type" val="Servitor"/>
	<set name="level" val="76"/>
	<set name="hp" val="2719.97952"/>
	<set name="pAtk" val="815.37145"/>
	<set name="pDef" val="323.78455"/>
	<set name="mAtk" val="556.793"/>
	<set name="mDef" val="236.93149"/>
	<set name="crit" val="4.0"/>
	<set name="atkSpd" val="253.0"/>
	<set name="str" val="40"/>
	<set name="dex" val="30"/>
	<set name="con" val="43"/>
</npc>
</list>`
	for dir, body := range map[string]string{"skills": skills, "npcs": npcs} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "a.xml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sk, err := LoadSkills(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	templates, err := LoadNPCs(filepath.Join(root, "npcs"))
	if err != nil {
		t.Fatalf("LoadNPCs: %v", err)
	}
	for _, tt := range []struct {
		ref   SkillRef
		npc   int32
		level int
		pAtk  float64
	}{
		{SkillRef{1276, 1}, 14207, 42, 198.34591},
		{SkillRef{1276, 14}, 14220, 76, 815.37145},
	} {
		got := sk[tt.ref].NPCID
		if got != tt.npc {
			t.Fatalf("%v NPCID = %d, want %d", tt.ref, got, tt.npc)
		}
		n, ok := templates[got]
		if !ok {
			t.Fatalf("%v: template %d not loaded", tt.ref, got)
		}
		if n.Type != "Servitor" || n.Level != tt.level || n.Combat.PAtk != tt.pAtk || n.Combat.AtkSpd != 253 {
			t.Errorf("%v template = %+v, want a level %d Servitor of P.Atk %v, attack speed 253", tt.ref, n, tt.level, tt.pAtk)
		}
	}
	if got := sk[SkillRef{1177, 1}].NPCID; got != 0 {
		t.Errorf("Wind Strike NPCID = %d, want 0", got)
	}
}

// TestLoadSummonTemplatesShipped checks the same on the shipped datapack,
// when BOT_DATAPACK_XML names its data/xml directory: Summon Kai the Cat
// levels 1 and 14 name the Servitor templates 14207 and 14220.
func TestLoadSummonTemplatesShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	sk, err := LoadSkills(filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	templates, err := LoadNPCs(filepath.Join(dir, "npcs"))
	if err != nil {
		t.Fatalf("LoadNPCs: %v", err)
	}
	for ref, want := range map[SkillRef]int32{{1276, 1}: 14207, {1276, 14}: 14220} {
		id := sk[ref].NPCID
		if n := templates[id]; id != want || n.Type != "Servitor" {
			t.Errorf("%v names %d (%q), want the Servitor %d", ref, id, n.Type, want)
		}
	}
}
