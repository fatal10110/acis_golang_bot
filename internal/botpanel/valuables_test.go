package botpanel

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestInspectValuablesView checks the held valuables of a merchant's
// inspect (docs/valuables/preset.md "Wiring"): name, place, rounds and
// ask, and no valuables key for a bot holding none.
func TestInspectValuablesView(t *testing.T) {
	in := botclient.Inspection{
		At: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
		Valuables: []botclient.HeldValuable{
			{TemplateID: 70, Carried: 1, Ask: 900000},
			{TemplateID: 70, Enchant: 4, Carried: 1, Stored: 1, Rounds: 2, Ask: 1500000},
			{TemplateID: 2, Stored: 3, Rounds: 3, Ask: 120000, Released: true},
		},
	}
	raw, err := json.Marshal(inspectViewOf(in, inspectFresh, testCatalog()).Valuables)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[
		{"id":70,"name":"Claymore","place":"carried","carried":1,"stored":0,"rounds":0,"ask":900000},
		{"id":70,"name":"Claymore","enchant":4,"place":"carried, stored","carried":1,"stored":1,"rounds":2,"ask":1500000},
		{"id":2,"name":"Long Sword","place":"stored","carried":0,"stored":3,"rounds":3,"ask":120000,"released":true}]`
	var got, wantV any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantV); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantV) {
		t.Errorf("valuables\n%s\nwant\n%s", raw, want)
	}

	in.Valuables = nil
	raw, err = json.Marshal(inspectViewOf(in, inspectFresh, testCatalog()))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if v, ok := fields["valuables"]; ok {
		t.Errorf("view of a bot holding none has valuables %s", v)
	}
}
