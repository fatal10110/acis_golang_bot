package datapack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestLoadWaterZones checks the water zone loader against the server's
// cuboid rules: two nodes, corners and Z band normalized, and every other
// shape refused rather than dropped.
func TestLoadWaterZones(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    []WaterZone
		wantErr string
	}{
		{
			name: "cuboids in file order, normalized",
			body: `<list>
				<zone name="a" shape="Cuboid" minZ="-4784" maxZ="-3784"><node x="-163840" y="163839"/><node x="-131072" y="196607"/></zone>
				<zone shape="Cuboid" minZ="337" maxZ="237"><node x="-117492" y="46588"/><node x="-118668" y="45364"/></zone>
			</list>`,
			want: []WaterZone{
				{Name: "a", X1: -163840, Y1: 163839, X2: -131072, Y2: 196607, MinZ: -4784, MaxZ: -3784},
				{X1: -118668, Y1: 45364, X2: -117492, Y2: 46588, MinZ: 237, MaxZ: 337},
			},
		},
		{name: "no zones", body: `<list/>`, want: []WaterZone{}},
		{
			name:    "polygon",
			body:    `<list><zone shape="NPoly" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="0"/><node x="5" y="5"/></zone></list>`,
			wantErr: `shape "NPoly" is not modeled`,
		},
		{
			name:    "cuboid with three nodes",
			body:    `<list><zone shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/><node x="9" y="9"/></zone></list>`,
			wantErr: "wants 2 nodes, has 3",
		},
		{
			name:    "missing maxZ",
			body:    `<list><zone shape="Cuboid" minZ="0"><node x="0" y="0"/><node x="5" y="5"/></zone></list>`,
			wantErr: `"maxZ"`,
		},
		{
			name:    "bad node",
			body:    `<list><zone shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="zero"/><node x="5" y="5"/></zone></list>`,
			wantErr: `"y"`,
		},
		{name: "malformed", body: `<list><zone`, wantErr: "decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "WaterZone.xml")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := LoadWaterZones(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadWaterZones error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadWaterZones: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadWaterZones = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestWaterZoneContains checks the server's inclusive bounds.
func TestWaterZoneContains(t *testing.T) {
	z := WaterZone{X1: 0, Y1: 0, X2: 10, Y2: 20, MinZ: -5, MaxZ: 5}
	for _, tt := range []struct {
		x, y, h int32
		want    bool
	}{
		{0, 0, -5, true},
		{10, 20, 5, true},
		{5, 5, 0, true},
		{-1, 5, 0, false},
		{5, 21, 0, false},
		{5, 5, 6, false},
		{5, 5, -6, false},
	} {
		if got := z.Contains(tt.x, tt.y, tt.h); got != tt.want {
			t.Errorf("Contains(%d, %d, %d) = %v, want %v", tt.x, tt.y, tt.h, got, tt.want)
		}
	}
}

// TestLoadReadsWaterZones checks that Load reads zones/WaterZone.xml and
// fails without it.
func TestLoadReadsWaterZones(t *testing.T) {
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dp.WaterZones) != 2 || dp.WaterZones[1].MinZ != 237 {
		t.Errorf("WaterZones = %+v, want the fixture's two zones", dp.WaterZones)
	}
	if _, err := LoadWaterZones(filepath.Join(t.TempDir(), "WaterZone.xml")); err == nil {
		t.Error("LoadWaterZones read a missing file")
	}
}
