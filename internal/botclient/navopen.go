package botclient

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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
// table over doors.xml of datapack: the navigation every bot of a process
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
	c, err := geodata.Open(f.Geodata, settings.GeoType, settings.Regions, f.Cache, settings.Options)
	if err != nil {
		return nil, nil, err
	}
	g, err := nav.OpenNavCache(f.NavCache, nav.NavKey{GeoCRC: c.CRC(), DoorsHash: nav.DoorsHash(doorsXML), ClusterCells: nav.DefaultClusterCells})
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	log.Info("geodata", "regions", len(settings.Regions), "region_hash", geodata.RegionListHash(settings.Regions), "geo_crc", c.CRC())
	n = &Nav{
		Engine: c.Engine,
		Hier:   nav.NewHierarchy(g, c.Engine, f.LRUBytes),
		Doors:  NewDoorTable(c.Engine, doors, log),
	}
	return n, func() { g.Close(); c.Close() }, nil
}
