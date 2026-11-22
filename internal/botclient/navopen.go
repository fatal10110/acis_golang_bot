package botclient

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// NavFiles name what OpenNav maps: the geodata region directory the caches
// were built from (only stat'ed), the local copy of the server's
// geoengine.properties, the geodata cache of `bots geopack` and the nav cache
// of `bots navbuild` (docs/navigation.md, D13).
type NavFiles struct {
	Geodata, Geoengine, Cache, NavCache string
	// LRUBytes bounds the process-wide cluster result LRU of the hierarchy.
	LRUBytes int64
}

// OpenNav maps the geodata cache and the nav cache of f and builds the door
// table over doors.xml of datapack and the water model over its water zones: the navigation every bot of a process
// shares. The caches stay mapped until closeNav, which the caller runs once
// every bot returned. A missing, stale or corrupt cache is an error naming
// the command that builds it; neither cache is ever rebuilt here. It logs
// the region-list hash, to compare with the server's.
func OpenNav(f NavFiles, datapack string, log *slog.Logger) (n *Nav, closeNav func(), err error) {
	settings, err := geodata.LoadSettings(f.Geoengine)
	if err != nil {
		return nil, nil, err
	}
	if len(settings.Regions) == 0 {
		return nil, nil, fmt.Errorf("%s lists no regions", f.Geoengine)
	}
	doorsPath := filepath.Join(datapack, "doors.xml")
	doorsXML, err := os.ReadFile(doorsPath)
	if err != nil {
		return nil, nil, err
	}
	doors, err := geodata.LoadDoors(doorsPath, log)
	if err != nil {
		return nil, nil, err
	}
	water, err := LoadWater(datapack)
	if err != nil {
		return nil, nil, err
	}
	c, err := geodata.Open(f.Geodata, settings.GeoType, settings.Regions, f.Cache, settings.Options)
	if err != nil {
		return nil, nil, err
	}
	g, err := nav.OpenNavCache(f.NavCache, nav.NavKey{GeoCRC: c.CRC(), DoorsHash: nav.DoorsHash(doorsXML), ClusterCells: nav.DefaultClusterCells, WaterHash: water.Hash()})
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	log.Info("geodata", "regions", len(settings.Regions), "region_hash", geodata.RegionListHash(settings.Regions), "geo_crc", c.CRC())
	n = &Nav{
		Engine: c.Engine,
		Hier:   nav.NewHierarchy(g, c.Engine, water, f.LRUBytes),
		Doors:  NewDoorTable(c.Engine, doors, log),
		Water:  water,
	}
	return n, func() { g.Close(); c.Close() }, nil
}

// LoadWater reads the datapack's water zones (xmlDir/zones/WaterZone.xml)
// into the planner's water model: `bots navbuild` keys the nav cache with
// it (NavKey.WaterHash), and a walker opening the cache builds it the same
// way.
func LoadWater(xmlDir string) (*nav.Water, error) {
	zones, err := datapack.LoadWaterZones(filepath.Join(xmlDir, "zones", "WaterZone.xml"))
	if err != nil {
		return nil, err
	}
	boxes := make([]nav.WaterBox, len(zones))
	for i, z := range zones {
		boxes[i] = nav.WaterBox{X1: int(z.X1), Y1: int(z.Y1), X2: int(z.X2), Y2: int(z.Y2), MinZ: int(z.MinZ), MaxZ: int(z.MaxZ)}
	}
	return nav.NewWater(boxes), nil
}
