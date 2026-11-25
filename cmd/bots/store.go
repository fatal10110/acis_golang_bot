package main

import (
	"flag"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// defaultStoreBudget is a merchant's -store-budget when none is given.
const defaultStoreBudget = 50000

// defaultValuablesBudget is a merchant's -valuables-budget when none is
// given (D34, docs/valuables/release.md "Valuables budget (D34)").
const defaultValuablesBudget = 2000000

// storeOptions are the merchant flags of `bots swarm`: a bot of the
// merchant preset opens a private sell store of its loot when it stands in
// a town of the datapack's travel map with room for it.
type storeOptions struct {
	// budget is the adena each merchant's store sales may bring in per
	// session.
	budget int64
	// perTown and spacing are the process-wide limits: bot stores per town,
	// and the least distance between two.
	perTown, spacing int
	// valuablesBudget is what each merchant's valuables sales, by trade and
	// store, may bring in per session (D34).
	valuablesBudget int64
	// adRegionGap and adsPerHour are the process-wide limits on the
	// merchants' Trade ad lines (docs/valuables/ads.md "Ad board"): one a
	// restart region each adRegionGap, adsPerHour an hour.
	adRegionGap time.Duration
	adsPerHour  int
}

func addStoreFlags(fs *flag.FlagSet, o *storeOptions) {
	fs.Int64Var(&o.budget, "store-budget", defaultStoreBudget, "adena each merchant's private store may sell for per session (-persona merchant)")
	fs.IntVar(&o.perTown, "stores-per-town", 5, "most bot stores open in one town at a time")
	fs.IntVar(&o.spacing, "store-spacing", 150, "least distance between two bot stores, in game units")
	fs.Int64Var(&o.valuablesBudget, "valuables-budget", defaultValuablesBudget, "adena each merchant's valuables may sell for per session, by trade and store, apart from -store-budget (-persona merchant)")
	fs.DurationVar(&o.adRegionGap, "ad-region-gap", botclient.DefaultAdRegionGap, "least time between two Trade ad lines of the process's merchants in one region (-persona merchant)")
	fs.IntVar(&o.adsPerHour, "ads-per-hour", botclient.DefaultAdProcessPerHour, "most Trade ad lines the process's merchants say in an hour (-persona merchant)")
}

func (o storeOptions) validate() error {
	switch {
	case o.budget < 1:
		return fmt.Errorf("-store-budget %d: want 1 or more", o.budget)
	case o.perTown < 1:
		return fmt.Errorf("-stores-per-town %d: want 1 or more", o.perTown)
	case o.spacing < 0 || o.spacing > math.MaxInt32:
		return fmt.Errorf("-store-spacing %d: want 0 to %d", o.spacing, math.MaxInt32)
	case o.valuablesBudget < 1:
		return fmt.Errorf("-valuables-budget %d: want 1 or more", o.valuablesBudget)
	case o.adRegionGap < time.Second:
		return fmt.Errorf("-ad-region-gap %v: want 1s or more", o.adRegionGap)
	case o.adsPerHour < 1:
		return fmt.Errorf("-ads-per-hour %d: want 1 or more", o.adsPerHour)
	}
	return nil
}

// valuablesFlags are the flags that apply to a preset whose persona
// enables the valuables policy only.
var valuablesFlags = []string{"valuables-budget", "ad-region-gap", "ads-per-hour"}

// adBoard returns the ad board every merchant of the process shares.
func (o storeOptions) adBoard() *botclient.AdBoard {
	return botclient.NewAdBoard(o.adRegionGap, o.adsPerHour)
}

// registry returns the store registry every merchant of the process
// shares, over the towns of travel.
func (o storeOptions) registry(travel *botclient.TravelMap, log *slog.Logger) *botclient.StoreRegistry {
	towns := travel.Towns()
	log.Info("store towns", "towns", len(towns), "per_town", o.perTown)
	return botclient.NewStoreRegistry(towns, o.perTown, int32(o.spacing))
}
