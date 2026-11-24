package datapack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadNPCsRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"mAtk", `<npc id="1" name="x" type="Monster"><set name="mAtk" val="high"/></npc>`, `"mAtk"`},
		{"men", `<npc id="1" name="x" type="Monster"><set name="men" val="1.5"/></npc>`, `"men"`},
		{"corpse time", `<npc id="1" name="x" type="Monster"><set name="corpseTime" val="7s"/></npc>`, `"corpseTime"`},
		{"teachTo classes", `<npc id="1" name="x" type="Trainer"><teachTo/></npc>`, `"classes"`},
		{"teachTo class", `<npc id="1" name="x" type="Trainer"><teachTo classes="0;x"/></npc>`, `"classes"`},
		{"teachTo empty part", `<npc id="1" name="x" type="Trainer"><teachTo classes="0;"/></npc>`, `"classes"`},
		{"teachTo range", `<npc id="1" name="x" type="Trainer"><teachTo classes="0;119"/></npc>`, "class id 119 out of range"},
		{"teachTo negative", `<npc id="1" name="x" type="Trainer"><teachTo classes="-1"/></npc>`, "class id -1 out of range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadNPCs(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadNPCs err = %v, want one naming %s", err, tt.want)
			}
		})
	}
}
