package geodata

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

const doorXML = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<door id="19210001" type="DOOR" level="1" name="gludio_castle_outter_001">
		<position x="-18408" y="0x1B9A8" z="-2768"/>
		<coordinates>
			<loc x="-18481" y="113059" z="-2799"/>
			<loc x="-18351" y="113059" z="-2799"/>
			<loc x="-18351" y="113071" z="-2799"/>
			<loc x="-18481" y="113071" z="-2799"/>
		</coordinates>
		<stats hp="253200" pDef="644" mDef="518" height="320"/>
		<function castle="1" opened="TRUE" openType="TIME" openTime="300"/>
	</door>
	<door id="2" type="WALL" level="1" name="far_away">
		<position x="900000" y="0" z="0"/>
		<coordinates><loc x="900000" y="0"/><loc x="900010" y="0"/><loc x="900010" y="10"/></coordinates>
		<stats hp="1" pDef="1" mDef="1" height="10"/>
	</door>
</list>`

// TestLoadDoors pins the server's doors.xml reading: attributes folded from
// the door, position, stats and function elements, integer literals in the
// position, the function defaults, and an out-of-world door logged and
// skipped.
func TestLoadDoors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doors.xml")
	if err := os.WriteFile(path, []byte(doorXML), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	table, err := LoadDoors(path, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("LoadDoors(): %v", err)
	}
	if table.Len() != 1 {
		t.Fatalf("loaded %d doors, want 1 (the out-of-world one skipped)", table.Len())
	}
	if !strings.Contains(logs.String(), "door_id=2") {
		t.Errorf("logs %q, want the skipped door 2", logs.String())
	}
	got, ok := table.Get(19210001)
	if !ok {
		t.Fatal("door 19210001 missing")
	}
	want := &door.Template{
		ID: 19210001, Name: "gludio_castle_outter_001", Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: -18408, Y: 113064, Z: -2768},
		Coordinates: []location.Point{
			{X: -18481, Y: 113059}, {X: -18351, Y: 113059}, {X: -18351, Y: 113071}, {X: -18481, Y: 113071},
		},
		HP: 253200, PDef: 644, MDef: 518, Height: 320,
		CastleID: 1, Opened: true, OpenKind: door.OpenTime, OpenTime: 300,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("template = %+v, want %+v", got, want)
	}
}

func TestLoadDoorsRejectsMalformedDoors(t *testing.T) {
	const coords = `<coordinates><loc x="0" y="0"/><loc x="10" y="0"/><loc x="10" y="10"/></coordinates>`
	const stats = `<stats hp="1" pDef="1" mDef="1" height="10"/>`
	tests := []struct {
		name, body, want string
	}{
		{"missing name", `<door id="1" type="DOOR" level="1"><position x="0" y="0" z="0"/>` + coords + stats + `</door>`, "name is required"},
		{"unknown type", `<door id="1" type="GATE" level="1" name="d"><position x="0" y="0" z="0"/>` + coords + stats + `</door>`, "unknown type"},
		{"two corners", `<door id="1" type="DOOR" level="1" name="d"><position x="0" y="0" z="0"/><coordinates><loc x="0" y="0"/><loc x="1" y="1"/></coordinates>` + stats + `</door>`, "at least 3 points"},
		{"corner without y", `<door id="1" type="DOOR" level="1" name="d"><position x="0" y="0" z="0"/><coordinates><loc x="0"/><loc x="1" y="1"/><loc x="2" y="2"/></coordinates>` + stats + `</door>`, "x and y are required"},
		{"bad literal", `<door id="1" type="DOOR" level="1" name="d"><position x="0x" y="0" z="0"/>` + coords + stats + `</door>`, "invalid integer literal"},
		{"duplicate id", `<door id="1" type="DOOR" level="1" name="d"><position x="0" y="0" z="0"/>` + coords + stats + `</door><door id="1" type="DOOR" level="1" name="e"><position x="0" y="0" z="0"/>` + coords + stats + `</door>`, "duplicate template id 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doors.xml")
			if err := os.WriteFile(path, []byte("<list>"+tt.body+"</list>"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadDoors(path, slog.New(slog.DiscardHandler))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("LoadDoors() error = %v, want %q", err, tt.want)
			}
		})
	}
}
