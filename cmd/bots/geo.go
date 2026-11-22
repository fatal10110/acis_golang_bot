package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// geoOptions are the run-time geodata and nav cache flags of `bots swarm`
// and `bots ui` (docs/navigation.md, D13, D13a): the walking personas walk
// on the packed geodata cache (`bots geopack`) and the nav cache built from
// it (`bots navbuild`), both mapped read-only and never rebuilt here.
type geoOptions struct {
	geodata, geoengine, cache, navCache string
	// lruBytes bounds the process-wide cluster result LRU.
	lruBytes int64
}

func addGeoFlags(fs *flag.FlagSet, o *geoOptions) {
	fs.StringVar(&o.geodata, "geodata", "", "directory of the geodata region files the caches were built from (only stat'ed); the walking personas need it with -geoengine, -geodata-cache, -nav-cache and -datapack")
	fs.StringVar(&o.geoengine, "geoengine", "", "local copy of the server's geoengine.properties: region list and GeoDataType")
	fs.StringVar(&o.cache, "geodata-cache", "", "geodata cache written by `bots geopack`")
	fs.StringVar(&o.navCache, "nav-cache", "", "nav cache written by `bots navbuild`")
	fs.Int64Var(&o.lruBytes, "nav-lru-bytes", nav.DefaultMemberCacheBytes, "bound of the process-wide cluster result cache of the nav hierarchy, in bytes")
}

// set reports whether any geodata flag was given.
func (o geoOptions) set() bool {
	return o.geodata != "" || o.geoengine != "" || o.cache != "" || o.navCache != ""
}

// validate checks the flags without reading a file: all four or none, and
// with the datapack, whose doors.xml the door table and the nav cache key
// read.
func (o geoOptions) validate(datapack string) error {
	if !o.set() {
		return nil
	}
	if o.geodata == "" || o.geoengine == "" || o.cache == "" || o.navCache == "" {
		return errors.New("-geodata, -geoengine, -geodata-cache and -nav-cache go together")
	}
	if datapack == "" {
		return errors.New("-geodata needs -datapack: the door table reads its doors.xml")
	}
	if o.lruBytes <= 0 {
		return fmt.Errorf("-nav-lru-bytes %d: want a positive size", o.lruBytes)
	}
	return nil
}

// open maps the geodata cache and the nav cache and builds the door table
// over doors.xml of datapack: the navigation every bot of the process
// shares. The caches stay mapped until close, which the caller runs once
// every bot returned. A missing, stale or corrupt cache is an error naming
// the command that builds it. It logs the region-list hash, to compare with
// the server's.
func (o geoOptions) open(datapack string, log *slog.Logger) (n *botclient.Nav, closeNav func(), err error) {
	settings, err := geodata.LoadSettings(o.geoengine)
	if err != nil {
		return nil, nil, err
	}
	if len(settings.Regions) == 0 {
		return nil, nil, fmt.Errorf("%s lists no regions", o.geoengine)
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
	c, err := geodata.Open(o.geodata, settings.GeoType, settings.Regions, o.cache, settings.Options)
	if err != nil {
		return nil, nil, err
	}
	g, err := nav.OpenNavCache(o.navCache, nav.NavKey{GeoCRC: c.CRC(), DoorsHash: nav.DoorsHash(doorsXML), ClusterCells: nav.DefaultClusterCells})
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	log.Info("geodata", "regions", len(settings.Regions), "region_hash", geodata.RegionListHash(settings.Regions), "geo_crc", c.CRC())
	n = &botclient.Nav{
		Engine: c.Engine,
		Hier:   nav.NewHierarchy(g, c.Engine, o.lruBytes),
		Doors:  botclient.NewDoorTable(c.Engine, doors, log),
	}
	return n, func() { g.Close(); c.Close() }, nil
}

// walkReady refuses a walking preset without the navigation (D13a): it
// walks on checked geodata, and today's legs are not kept as a fallback.
func walkReady(p preset, n *botclient.Nav) error {
	if p.walks && n == nil {
		return fmt.Errorf("-persona %s walks on geodata: it needs -geodata, -geoengine, -geodata-cache and -nav-cache (run `bots geopack`, then `bots navbuild`)", p.name)
	}
	return nil
}
