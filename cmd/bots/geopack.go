package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
)

// runGeopack packs the region files a copy of geoengine.properties lists
// into the geodata cache the bots map read-only at run time, as the
// server's geopack does, and reports the stored size by block kind and the
// region-list hash. It reads each region file once and sends nothing.
func runGeopack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("geopack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	geodataDir := fs.String("geodata", "", "directory of the geodata region files (the datapack's data/geodata, copied with rsync -a)")
	geoengine := fs.String("geoengine", "", "local copy of the server's geoengine.properties: region list and GeoDataType")
	out := fs.String("out", "geodata.cache", "cache file to write; the bots' -geodata-cache")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := geopack(*geodataDir, *geoengine, *out, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "geopack: %v\n", err)
		return 1
	}
	return 0
}

func geopack(geodataDir, geoengine, out string, stdout, stderr io.Writer) error {
	if geodataDir == "" || geoengine == "" {
		return errors.New("-geodata and -geoengine are required")
	}
	settings, err := geodata.LoadSettings(geoengine)
	if err != nil {
		return err
	}
	if len(settings.Regions) == 0 {
		return fmt.Errorf("%s lists no regions", geoengine)
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	stats, err := geodata.WriteCache(out, geodataDir, settings.GeoType, settings.Regions, log)
	if err != nil {
		return err
	}
	total := stats.IndexBytes + stats.ComplexBytes + stats.MultilayerBytes
	fmt.Fprintf(stdout, "regions %d (list hash %s): flat %d, complex %d, multilayer %d blocks\n",
		len(settings.Regions), geodata.RegionListHash(settings.Regions), stats.FlatBlocks, stats.ComplexBlocks, stats.MultilayerBlocks)
	fmt.Fprintf(stdout, "stored bytes: index %d, complex %d, multilayer %d, total %d (%.1f MB)\n",
		stats.IndexBytes, stats.ComplexBytes, stats.MultilayerBytes, total, float64(total)/1e6)
	fmt.Fprintf(stdout, "wrote %s\n", out)
	return nil
}
