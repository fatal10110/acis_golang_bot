package nav

import (
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestOwnerSwimSample counts the spot centres on a wet cell, which the spot
// registry drops (#275), and re-measures the #161 route sample with swimming
// (docs/navigation.md, "Swimming"): every third farming spot, routed with
// the cell planner from the nearest STANDARD teleport destination within
// 8000 units. For each route that passes water when water is ignored it
// logs the planner keeping out of water and the planner swimming with a
// breath of 1200 units (base swim speed 50 times swimBreath, 24 s). It
// reads the owner's geodata, so it skips unless BOT_NAV_GEODATA,
// BOT_NAV_GEOENGINE, BOT_NAV_GEODATA_CACHE and BOT_NAV_DATAPACK (the xml
// directory) are set.
func TestOwnerSwimSample(t *testing.T) {
	env := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			t.Skipf("%s not set: the owner-geodata benchmarks run locally only", k)
		}
		return v
	}
	dir, props, cachePath, xmlDir := env("BOT_NAV_GEODATA"), env("BOT_NAV_GEOENGINE"), env("BOT_NAV_GEODATA_CACHE"), env("BOT_NAV_DATAPACK")
	settings, err := geodata.LoadSettings(props)
	if err != nil {
		t.Fatal(err)
	}
	c, err := geodata.Open(dir, settings.GeoType, settings.Regions, cachePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	dp, err := datapack.Load(xmlDir)
	if err != nil {
		t.Fatal(err)
	}
	boxes := make([]WaterBox, len(dp.WaterZones))
	for i, z := range dp.WaterZones {
		boxes[i] = WaterBox{X1: int(z.X1), Y1: int(z.Y1), X2: int(z.X2), Y2: int(z.Y2), MinZ: int(z.MinZ), MaxZ: int(z.MaxZ)}
	}
	water := NewWater(boxes)
	e := c.Engine
	var dests []location.Location
	for _, list := range dp.Teleports {
		for _, tp := range list {
			if tp.Kind == datapack.TeleportStandard && e.HasGeo(int(tp.X), int(tp.Y)) {
				dests = append(dests, location.Location{X: int(tp.X), Y: int(tp.Y), Z: int(tp.Z)})
			}
		}
	}
	plain := Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000}
	avoid := plain
	avoid.Water = water
	swim := avoid
	swim.Swim = 1200

	length := func(r []location.Location) float64 {
		d := 0.0
		for i := 1; i < len(r); i++ {
			d += newLine3(r[i-1], r[i]).length()
		}
		return d
	}
	passes := func(r []location.Location) bool {
		for i := 1; i < len(r); i++ {
			if _, _, ok := water.Span(r[i-1], r[i]); ok {
				return true
			}
		}
		return false
	}
	outcome := func(err error) string {
		switch {
		case err == nil:
			return "ok"
		case errors.Is(err, ErrWater):
			return "water"
		case errors.Is(err, ErrBreath):
			return "breath"
		case errors.Is(err, ErrBudget):
			return "budget"
		case errors.Is(err, ErrUnreachable):
			return "no route"
		}
		return err.Error()
	}
	spots := dp.Spots()
	centres := 0
	for _, s := range spots {
		if avoid.WetAt(location.Location{X: int(s.Center.X), Y: int(s.Center.Y), Z: int(s.Z)}) {
			centres++
		}
	}
	// The spot registry drops these up front (botclient.Nav.DrySpots, #275).
	t.Logf("spot centres on a wet cell: %d of %d", centres, len(spots))
	planned, wet := 0, 0
	avoided, swum := map[string]int{}, map[string]int{}
	for i := 0; i < len(spots); i += 3 {
		s := spots[i]
		to := location.Location{X: int(s.Center.X), Y: int(s.Center.Y), Z: int(s.Z)}
		if !e.HasGeo(to.X, to.Y) {
			continue
		}
		var from location.Location
		best := math.Inf(1)
		for _, d := range dests {
			if dd := math.Hypot(float64(d.X-to.X), float64(d.Y-to.Y)); dd < best && dd <= 8000 {
				from, best = d, dd
			}
		}
		if math.IsInf(best, 1) {
			continue
		}
		route, err := plain.Route(from, to)
		if err != nil {
			continue
		}
		planned++
		if !passes(route) {
			continue
		}
		wet++
		ra, errA := avoid.Route(from, to)
		start := time.Now()
		rs, errS := swim.Route(from, to)
		took := time.Since(start)
		avoided[outcome(errA)]++
		swum[outcome(errS)]++
		t.Logf("spot %s %v <- %v: dry %.0f; avoid %s %.0f; swim %s %.0f in %d segments, %v",
			s.Maker, to, from, length(route), outcome(errA), length(ra), outcome(errS), length(rs), max(len(rs)-1, 0), took.Round(time.Millisecond))
	}
	t.Logf("routes planned without water %d, passing water %d; keeping out of water %v; swimming %v", planned, wet, avoided, swum)
}
