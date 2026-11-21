package geodata

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// TestLoadSettings pins the server's reading of geoengine.properties: a
// region key present counts whatever its value, a commented-out line does
// not, out-of-range tiles are ignored, GeoDataType defaults to L2OFF, and
// the CanSee heights come from the file.
func TestLoadSettings(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Settings
		wantErr string
	}{
		{
			name: "listed regions and options",
			body: "GeoDataType = L2OFF\nMaxObstacleHeight = 40\nPartOfCharacterHeight = 80\n16_10 = false\n#16_11\n! 16_12\n26_25\n27_10\n",
			want: Settings{GeoType: L2OFF, Regions: []Region{{16, 10}, {26, 25}}, Options: engine.Options{MaxObstacleHeight: 40, PartOfCharacterHeight: 80}},
		},
		{
			name: "defaults",
			body: "20_18\n",
			want: Settings{GeoType: L2OFF, Regions: []Region{{20, 18}}, Options: engine.DefaultOptions()},
		},
		{name: "unknown type", body: "GeoDataType = L2X\n", wantErr: "unknown GeoDataType"},
		{name: "bad height", body: "MaxObstacleHeight = high\n", wantErr: "MaxObstacleHeight"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "geoengine.properties")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadSettings(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadSettings() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadSettings(): %v", err)
			}
			if got.GeoType != tt.want.GeoType || got.Options != tt.want.Options || !slices.Equal(got.Regions, tt.want.Regions) {
				t.Errorf("LoadSettings() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRegionListHash(t *testing.T) {
	a := RegionListHash([]Region{{16, 10}, {16, 11}})
	if len(a) != 16 {
		t.Fatalf("hash %q, want 16 hex digits", a)
	}
	if a == RegionListHash([]Region{{16, 10}}) || a == RegionListHash([]Region{{16, 11}, {16, 10}}) {
		t.Error("different region lists hash the same")
	}
	if a != RegionListHash([]Region{{16, 10}, {16, 11}}) {
		t.Error("the same region list hashes differently")
	}
}
