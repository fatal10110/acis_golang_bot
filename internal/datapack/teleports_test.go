package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadTeleportsSample(t *testing.T) {
	got, err := LoadTeleports(filepath.Join("testdata", "xml", "teleports.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[int32][]Teleport{
		30006: {
			{Desc: "The Village of Gludin", Kind: TeleportStandard, PriceID: 57, PriceCount: 18000, X: -80749, Y: 149834, Z: -3043},
			{Desc: "Clan Hall", Kind: "CHF_LEVEL_1", PriceID: 57, PriceCount: 500, X: -84000, Y: 244000, Z: -3700},
			// type defaults to STANDARD, as on the server.
			{Desc: "Elven Ruins", Kind: TeleportStandard, PriceID: 57, PriceCount: 830, X: -112367, Y: 234703, Z: -3688},
		},
		30598: {{Desc: "Dark Elf Village", Kind: "NEWBIE_TOKEN", PriceID: 8542, PriceCount: 1, X: 9716, Y: 15502, Z: -4500}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadTeleports = %+v, want %+v", got, want)
	}
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dp.Teleports, want) {
		t.Fatalf("Load Teleports = %+v, want %+v", dp.Teleports, want)
	}
}

func TestLoadTeleportsErrors(t *testing.T) {
	loc := func(attrs string) string {
		return `<list><telPosList npcId="1"><loc ` + attrs + `/></telPosList></list>`
	}
	full := `desc="d" priceId="57" priceCount="1" x="1" y="2" z="3"`
	tests := []struct {
		name, body, wantErr string
	}{
		{"malformed", "<list><telPosList", "decode"},
		{"no npcId", `<list><telPosList><loc ` + full + `/></telPosList></list>`, `"npcId"`},
		{"no desc", loc(`priceId="57" priceCount="1" x="1" y="2" z="3"`), `"desc"`},
		{"bad price", loc(`desc="d" priceId="57" priceCount="x" x="1" y="2" z="3"`), `"priceCount"`},
		{"no z", loc(`desc="d" priceId="57" priceCount="1" x="1" y="2"`), `"z"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "teleports.xml")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadTeleports(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadTeleports error = %v, want it to contain %s", err, tt.wantErr)
			}
		})
	}
	if _, err := LoadTeleports(filepath.Join(t.TempDir(), "teleports.xml")); err == nil {
		t.Fatal("LoadTeleports of a missing file: no error")
	}
}
