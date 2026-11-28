package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadInstantTeleports(t *testing.T) {
	want := map[int32][]Teleport{31114: {{X: -53174, Y: -250275, Z: -7911}}}
	got, err := LoadInstantTeleports(filepath.Join("testdata", "xml", "instantTeleports.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadInstantTeleports = %+v, want %+v", got, want)
	}
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dp.InstantTeleports, want) {
		t.Fatalf("Load InstantTeleports = %+v, want %+v", dp.InstantTeleports, want)
	}
}

func TestLoadInstantTeleportsErrors(t *testing.T) {
	tests := []struct{ name, xml string }{
		{name: "no npcId", xml: `<list><telPosList><loc x="1" y="2" z="3"/></telPosList></list>`},
		{name: "no z", xml: `<list><telPosList npcId="1"><loc x="1" y="2"/></telPosList></list>`},
		{name: "bad x", xml: `<list><telPosList npcId="1"><loc x="a" y="2" z="3"/></telPosList></list>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "instantTeleports.xml")
			if err := os.WriteFile(path, []byte(tt.xml), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadInstantTeleports(path); err == nil {
				t.Fatal("LoadInstantTeleports = nil error, want one")
			}
		})
	}
}

func TestSevenSignsEvent(t *testing.T) {
	for ev, want := range map[string]bool{
		"ssq_event": true, "ssq_seal1_twilight": true, "ssq_seal2_dawn": true, "ssq_seal1_none": true,
		"": false, "christmas": false, "valentine": false,
	} {
		if got := SevenSignsEvent(ev); got != want {
			t.Errorf("SevenSignsEvent(%q) = %v, want %v", ev, got, want)
		}
	}
}
