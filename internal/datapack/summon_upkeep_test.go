package datapack

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// summonUpkeep is the part of a Skill the servitor upkeep reads (#759).
type summonUpkeep struct {
	item, count, itemOT, countOT, steps int32
	life, idle, active                  time.Duration
	penalty                             float64
}

func upkeepOf(sk Skill) summonUpkeep {
	return summonUpkeep{
		item: sk.ItemConsumeID, count: sk.ItemConsumeCount, itemOT: sk.ItemConsumeIDOT, countOT: sk.ItemConsumeCountOT,
		steps: sk.ItemConsumeSteps, life: sk.SummonLifetime, idle: sk.SummonLostIdle, active: sk.SummonLostActive,
		penalty: sk.ExpPenalty,
	}
}

// TestLoadSummonUpkeep checks that the reader takes a summon skill's
// lifetime and upkeep per level, tables included, as the server's skill
// reader does (data/xml/skill.go): Summon Kai the Cat (1276) and Summon Kat
// the Cat (1111, whose itemConsumeSteps is a table: 0 at levels 1 and 2)
// as written in skills/1200-1299.xml and 1100-1199.xml; summonTotalLifeTime
// defaults to 1200000 ms on a SUMMON skill, and a skill of another type
// reads none of it.
func TestLoadSummonUpkeep(t *testing.T) {
	root := t.TempDir()
	skills := `<list>
<skill id="1276" levels="14" name="Summon Kai the Cat">
	<table name="#itemConsumeCount"> 1 1 1 2 2 2 2 2 3 3 3 3 4 4 </table>
	<table name="#itemConsumeCountOT"> 1 1 1 1 1 1 2 2 2 2 3 3 3 3 </table>
	<set name="itemConsumeId" val="1459"/>
	<set name="itemConsumeCount" val="#itemConsumeCount" />
	<set name="itemConsumeIdOT" val="1459"/>
	<set name="itemConsumeCountOT" val="#itemConsumeCountOT"/>
	<set name="itemConsumeSteps" val="14"/>
	<set name="summonTotalLifeTime" val="1200000"/>
	<set name="summonTimeLostIdle" val="500"/>
	<set name="summonTimeLostActive" val="1000"/>
	<set name="skillType" val="SUMMON"/>
	<set name="expPenalty" val="0.1" />
</skill>
<skill id="1111" levels="3" name="Summon Kat the Cat">
	<table name="#itemConsumeCount"> 3 4 1 </table>
	<table name="#itemConsumeCountOT"> 0 0 1 </table>
	<table name="#itemConsumeSteps"> 0 0 4 </table>
	<set name="itemConsumeId" val="1458"/>
	<set name="itemConsumeCount" val="#itemConsumeCount"/>
	<set name="itemConsumeIdOT" val="1458"/>
	<set name="itemConsumeCountOT" val="#itemConsumeCountOT"/>
	<set name="itemConsumeSteps" val="#itemConsumeSteps" />
	<set name="summonTimeLostIdle" val="500"/>
	<set name="summonTimeLostActive" val="1000"/>
	<set name="skillType" val="SUMMON"/>
	<set name="expPenalty" val="0.3" />
</skill>
<skill id="1177" levels="1" name="Wind Strike">
	<set name="skillType" val="MDAM"/>
</skill>
</list>`
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "a.xml"), []byte(skills), 0o600); err != nil {
		t.Fatal(err)
	}
	sk, err := LoadSkills(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	const life, idle, active = 1200 * time.Second, 500 * time.Millisecond, time.Second
	for _, tt := range []struct {
		ref  SkillRef
		want summonUpkeep
	}{
		{SkillRef{1276, 1}, summonUpkeep{1459, 1, 1459, 1, 14, life, idle, active, 0.1}},
		{SkillRef{1276, 14}, summonUpkeep{1459, 4, 1459, 3, 14, life, idle, active, 0.1}},
		{SkillRef{1111, 1}, summonUpkeep{1458, 3, 1458, 0, 0, life, idle, active, 0.3}},
		{SkillRef{1111, 3}, summonUpkeep{1458, 1, 1458, 1, 4, life, idle, active, 0.3}},
		{SkillRef{1177, 1}, summonUpkeep{}},
	} {
		if got := upkeepOf(sk[tt.ref]); got != tt.want {
			t.Errorf("%v upkeep = %+v, want %+v", tt.ref, got, tt.want)
		}
	}
}

// TestLoadSummonUpkeepShipped checks the same on the shipped datapack, when
// BOT_DATAPACK_XML names its data/xml directory: the summons of
// docs/servitors.md section 4, and the XP penalties the summon gate refuses
// (1154 and 1225 at 0.9).
func TestLoadSummonUpkeepShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	sk, err := LoadSkills(filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	const life, idle, active = 1200 * time.Second, 500 * time.Millisecond, time.Second
	for _, tt := range []struct {
		ref  SkillRef
		want summonUpkeep
	}{
		{SkillRef{1276, 1}, summonUpkeep{1459, 1, 1459, 1, 14, life, idle, active, 0.1}},
		{SkillRef{1276, 14}, summonUpkeep{1459, 4, 1459, 3, 14, life, idle, active, 0.1}},
		{SkillRef{1111, 5}, summonUpkeep{1458, 5, 1458, 1, 14, life, idle, active, 0.3}},
	} {
		if got := upkeepOf(sk[tt.ref]); got != tt.want {
			t.Errorf("%v upkeep = %+v, want %+v", tt.ref, got, tt.want)
		}
	}
	for _, ref := range []SkillRef{{1154, 1}, {1225, 1}} {
		if p := sk[ref].ExpPenalty; p != 0.9 {
			t.Errorf("%v ExpPenalty = %v, want 0.9", ref, p)
		}
	}
}
