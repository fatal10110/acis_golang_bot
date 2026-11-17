package main

import (
	"flag"
	"fmt"
	"log/slog"
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// defaultStoreBudget is a merchant's -store-budget when none is given.
const defaultStoreBudget = 50000

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
}

func addStoreFlags(fs *flag.FlagSet, o *storeOptions) {
	fs.Int64Var(&o.budget, "store-budget", defaultStoreBudget, "adena each merchant's private store may sell for per session (-persona merchant)")
	fs.IntVar(&o.perTown, "stores-per-town", 5, "most bot stores open in one town at a time")
	fs.IntVar(&o.spacing, "store-spacing", 150, "least distance between two bot stores, in game units")
}

func (o storeOptions) validate() error {
	switch {
	case o.budget < 1:
		return fmt.Errorf("-store-budget %d: want 1 or more", o.budget)
	case o.perTown < 1:
		return fmt.Errorf("-stores-per-town %d: want 1 or more", o.perTown)
	case o.spacing < 0 || o.spacing > math.MaxInt32:
		return fmt.Errorf("-store-spacing %d: want 0 to %d", o.spacing, math.MaxInt32)
	}
	return nil
}

// registry returns the store registry every merchant of the process
// shares, over the towns of travel.
func (o storeOptions) registry(travel *botclient.TravelMap, log *slog.Logger) *botclient.StoreRegistry {
	towns := travel.Towns()
	log.Info("store towns", "towns", len(towns), "per_town", o.perTown)
	return botclient.NewStoreRegistry(towns, o.perTown, int32(o.spacing))
}
