package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadHennasSample(t *testing.T) {
	want := map[int32]Henna{
		1:  {SymbolID: 1, DyeID: 4445, Price: 37000, STR: 1, CON: -3, Classes: []int32{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54, 56}},
		8:  {SymbolID: 8, DyeID: 4452, Price: 37000, INT: 1, WIT: -3, Classes: []int32{1, 4, 7, 11, 19, 22, 26, 32, 35, 39, 45, 47, 54, 56}},
		10: {SymbolID: 10, DyeID: 4454, Price: 37000, MEN: 1, WIT: -3, Classes: []int32{11, 15, 26, 29, 39, 42, 50}},
		// No price: a free draw, as on the server; a stat of 0 changes nothing.
		46: {SymbolID: 46, DyeID: 4490, Classes: []int32{16, 30}},
	}
	got, err := LoadHennas(filepath.Join("testdata", "xml", "hennas.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadHennas = %+v, want %+v", got, want)
	}
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dp.Hennas, want) {
		t.Fatalf("Load Hennas = %+v, want %+v", dp.Hennas, want)
	}
}

// TestLoadHennasLaterWins checks that a later symbol of an id replaces the
// earlier one, as in the server's table.
func TestLoadHennasLaterWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hennas.xml")
	body := `<list><henna symbolId="1" dyeId="2" classes="3"/><henna symbolId="1" dyeId="5" classes="6"/></list>`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHennas(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[1].DyeID != 5 {
		t.Fatalf("LoadHennas = %+v, want the second symbol only", got)
	}
}

func TestLoadHennasErrors(t *testing.T) {
	tests := []struct {
		name, attrs, wantErr string
	}{
		{"no symbolId", `dyeId="2" classes="3"`, `"symbolId"`},
		{"no dyeId", `symbolId="1" classes="3"`, `"dyeId"`},
		{"no classes", `symbolId="1" dyeId="2"`, `"classes"`},
		{"empty classes", `symbolId="1" dyeId="2" classes=""`, `"classes"`},
		{"empty class part", `symbolId="1" dyeId="2" classes="3;;4"`, `"classes"`},
		{"trailing separator", `symbolId="1" dyeId="2" classes="3;"`, `"classes"`},
		{"bad class", `symbolId="1" dyeId="2" classes="3;x"`, `"classes"`},
		{"bad price", `symbolId="1" dyeId="2" price="x" classes="3"`, `"price"`},
		{"bad stat", `symbolId="1" dyeId="2" INT="x" classes="3"`, `"INT"`},
		{"symbolId past int32", `symbolId="2147483648" dyeId="2" classes="3"`, `"symbolId"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hennas.xml")
			if err := os.WriteFile(path, []byte("<list><henna "+tt.attrs+"/></list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadHennas(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), path) {
				t.Fatalf("LoadHennas = %v, want an error naming %s and %s", err, path, tt.wantErr)
			}
		})
	}
	if _, err := LoadHennas(filepath.Join(t.TempDir(), "missing.xml")); err == nil {
		t.Fatal("LoadHennas of a missing file = nil, want an error")
	}
}
