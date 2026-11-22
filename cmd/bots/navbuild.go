package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// runNavbuild builds the nav hierarchy (docs/navigation.md, section 2) from
// the packed geodata cache, doors.xml and zones/WaterZone.xml, never the raw
// region files, and
// writes the nav cache walking personas map read-only. It works cluster by
// cluster on at most half the cores by default, and sends nothing.
func runNavbuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("navbuild", flag.ContinueOnError)
	fs.SetOutput(stderr)
	geodataDir := fs.String("geodata", "", "directory of the geodata region files the cache was packed from (only stat'ed)")
	geoengine := fs.String("geoengine", "", "local copy of the server's geoengine.properties: region list and GeoDataType")
	cache := fs.String("geodata-cache", "geodata.cache", "geodata cache written by `bots geopack`")
	datapack := fs.String("datapack", "", "local copy of the datapack's data/xml directory, holding doors.xml and zones/WaterZone.xml")
	out := fs.String("out", "nav.cache", "nav cache file to write")
	workers := fs.Int("workers", max(1, runtime.NumCPU()/2), "clusters built at once (default half the cores)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := navbuild(ctx, *geodataDir, *geoengine, *cache, *datapack, *out, *workers, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "navbuild: %v\n", err)
		return 1
	}
	return 0
}

// navbuildHeapLimit is the soft heap limit navbuild runs under.
const navbuildHeapLimit = 1 << 30

func navbuild(ctx context.Context, geodataDir, geoengine, cachePath, datapack, out string, workers int, stdout, stderr io.Writer) error {
	if geodataDir == "" || geoengine == "" || datapack == "" {
		return errors.New("-geodata, -geoengine and -datapack are required")
	}
	if workers < 1 {
		return errors.New("-workers must be at least 1")
	}
	settings, err := geodata.LoadSettings(geoengine)
	if err != nil {
		return err
	}
	if len(settings.Regions) == 0 {
		return fmt.Errorf("%s lists no regions", geoengine)
	}
	doorsPath := filepath.Join(datapack, "doors.xml")
	doorsXML, err := os.ReadFile(doorsPath)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	doors, err := geodata.LoadDoors(doorsPath, log)
	if err != nil {
		return err
	}
	water, err := loadWater(datapack)
	if err != nil {
		return err
	}
	c, err := geodata.Open(geodataDir, settings.GeoType, settings.Regions, cachePath)
	if err != nil {
		return err
	}
	defer c.Close()

	var clusters []nav.ClusterKey
	for _, r := range settings.Regions {
		clusters = append(clusters, nav.RegionClusters(r.X, r.Y, nav.DefaultClusterCells)...)
	}
	// A soft heap limit: the GC works harder rather than let the build
	// pass 1 GiB of heap (docs/navigation.md, navbuild peak heap). It
	// keeps a lower limit the operator set with GOMEMLIMIT.
	if debug.SetMemoryLimit(-1) > navbuildHeapLimit {
		debug.SetMemoryLimit(navbuildHeapLimit)
	}
	began := time.Now()
	image, stats, err := nav.Build(ctx, nav.BuildConfig{
		Engine:   c.Engine,
		Doors:    doors.All(),
		Water:    water,
		Clusters: clusters,
		Workers:  workers,
		Key:      nav.NavKey{GeoCRC: c.CRC(), DoorsHash: nav.DoorsHash(doorsXML)},
		Log:      log,
	})
	if err != nil {
		return err
	}
	if err := nav.WriteNavCache(out, image); err != nil {
		return err
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Fprintf(stdout, "regions %d, clusters %d, workers %d, built in %s, heap from the OS %.0f MB\n",
		len(settings.Regions), stats.Clusters, workers, time.Since(began).Round(time.Second), float64(mem.HeapSys)/1e6)
	fmt.Fprintf(stdout, "entrance nodes %d, edges %d (intra-cluster %d, crossings %d), door refs %d, doors %d (%d skipped)\n",
		stats.Nodes, stats.IntraEdges+stats.CrossEdges, stats.IntraEdges, stats.CrossEdges, stats.DoorRefs, stats.Doors, stats.SkippedDoors)
	fmt.Fprintf(stdout, "SCCs %d (largest %d nodes), condensation edges %d\n", stats.SCCs, stats.LargestSCC, stats.CondEdges)
	fmt.Fprintf(stdout, "wrote %s (%.1f MB)\n", out, float64(stats.Bytes)/1e6)
	return nil
}

// loadWater reads the datapack's water zones (xmlDir/zones/WaterZone.xml)
// into the planner's water model, the one a walker builds the same way to
// open the nav cache (NavKey.WaterHash).
func loadWater(xmlDir string) (*nav.Water, error) {
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
