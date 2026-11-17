package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadBuyListsSample(t *testing.T) {
	got, err := LoadBuyLists(filepath.Join("testdata", "xml", "buyLists.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := []BuyList{
		{ID: 14, NPCID: 30004, Products: []Product{{1835, 8, -1}, {2509, 17, -1}, {1060, 103, -1}}},
		// count is the limited stock; price defaults to 0, as on the server.
		{ID: 29, NPCID: 30094, Products: []Product{{1048, 110, -1}, {5900, 11000, 1}, {1, 0, -1}}},
		{ID: 900, NPCID: 0, Products: []Product{{57, 1, -1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadBuyLists = %+v, want %+v", got, want)
	}
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dp.BuyLists, want) {
		t.Fatalf("Load BuyLists = %+v, want %+v", dp.BuyLists, want)
	}
}

func TestLoadBuyListsErrors(t *testing.T) {
	tests := []struct {
		name, body, wantErr string
	}{
		{"malformed", "<list><buyList", "decode"},
		{"no id", `<list><buyList npcId="1"/></list>`, `"id"`},
		{"no npcId", `<list><buyList id="1"/></list>`, `"npcId"`},
		{"product without id", `<list><buyList id="1" npcId="2"><product price="3"/></buyList></list>`, `"id"`},
		{"bad price", `<list><buyList id="1" npcId="2"><product id="3" price="x"/></buyList></list>`, `"price"`},
		{"bad count", `<list><buyList id="1" npcId="2"><product id="3" count="x"/></buyList></list>`, `"count"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "buyLists.xml")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadBuyLists(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadBuyLists error = %v, want it to contain %s", err, tt.wantErr)
			}
		})
	}
	if _, err := LoadBuyLists(filepath.Join(t.TempDir(), "buyLists.xml")); err == nil {
		t.Fatal("LoadBuyLists of a missing file: no error")
	}
}
