package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geoquery"
)

// runGeoquery writes the geodata parity query file: a curated set of
// height, canmove, los and validlocation questions (walls, ledges both
// ways, multilayer heights, random short pairs) answered by the bot's
// engine over the mapped cache, in the dump format the server's
// `geoprobe -expected-dump` checks. It reads geodata only and sends nothing.
func runGeoquery(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("geoquery", flag.ContinueOnError)
	fs.SetOutput(stderr)
	geodataDir := fs.String("geodata", "", "directory of the geodata region files the cache was packed from")
	geoengine := fs.String("geoengine", "", "local copy of the server's geoengine.properties: region list and GeoDataType")
	cache := fs.String("geodata-cache", "geodata.cache", "geodata cache written by bots geopack")
	out := fs.String("out", "geoquery.dump", "query file to write, for geoprobe -expected-dump")
	seed := fs.Uint64("seed", 1, "seed of the site sampling; the same seed gives the same file")
	n := fs.Int("n", 1000, "sites of each kind (wall, ledge, multilayer, random)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := writeGeoquery(*geodataDir, *geoengine, *cache, *out, *seed, *n, stdout); err != nil {
		fmt.Fprintf(stderr, "geoquery: %v\n", err)
		return 1
	}
	return 0
}

func writeGeoquery(geodataDir, geoengine, cachePath, out string, seed uint64, n int, stdout io.Writer) (err error) {
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
	// The engine runs on its default options, as geoprobe's does: geoprobe
	// takes the region format from -config but never its engine options.
	c, err := geodata.Open(geodataDir, settings.GeoType, settings.Regions, cachePath)
	if err != nil {
		return err
	}
	defer c.Close()
	set := geoquery.Curate(c.Engine, settings.Regions, seed, n)
	records := make([]geoquery.Record, len(set.Queries))
	for i, q := range set.Queries {
		records[i] = geoquery.Evaluate(c.Engine, q)
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	if err := geoquery.WriteDump(f, records); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "regions %d (list hash %s), seed %d: sites wall %d, ledge %d, multilayer %d, random %d\n",
		len(settings.Regions), geodata.RegionListHash(settings.Regions), seed,
		set.Sites[geoquery.Wall], set.Sites[geoquery.Ledge], set.Sites[geoquery.Multilayer], set.Sites[geoquery.Random])
	fmt.Fprintf(stdout, "wrote %d queries to %s\n", len(records), out)
	return nil
}
