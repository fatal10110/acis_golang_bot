package geodata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/fatal10110/acis_golang_bot/internal/geo/config"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// Settings is what a copy of the server's geoengine.properties gives the
// bot: the region list, the region file format, and the engine options
// (MaxObstacleHeight and PartOfCharacterHeight, used by CanSee).
type Settings struct {
	GeoType GeoType
	Regions []Region
	Options engine.Options
}

// LoadSettings reads the geoengine.properties file at path as the server
// does: the regions by ListedRegions, GeoDataType defaulting to L2OFF.
func LoadSettings(path string) (Settings, error) {
	props, err := config.LoadFile(path)
	if err != nil {
		return Settings{}, err
	}
	opts, err := engine.OptionsFromProperties(props)
	if err != nil {
		return Settings{}, fmt.Errorf("geodata: %s: %w", path, err)
	}
	s := Settings{
		GeoType: GeoType(props.String("GeoDataType", string(L2OFF))),
		Regions: ListedRegions(props),
		Options: opts,
	}
	if !s.GeoType.Valid() {
		return Settings{}, fmt.Errorf("geodata: %s: unknown GeoDataType %q", path, s.GeoType)
	}
	return s, nil
}

// RegionListHash is a short hash of the region list, in its tile order, to
// compare the regions a bot loads with the ones the server lists.
func RegionListHash(regions []Region) string {
	h := sha256.New()
	for _, r := range regions {
		fmt.Fprintf(h, "%d_%d\n", r.X, r.Y)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
