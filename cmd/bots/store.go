package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// storeOptions are the merchant flags of `bots swarm`: with a budget every
// bot opens a private sell store of its loot when it stands in a town of
// the datapack's travel map with room for it.
type storeOptions struct {
	// budget is the adena each bot's store sales may bring in per session;
	// 0 opens no store.
	budget int64
	// perTown and spacing are the process-wide limits: bot stores per town,
	// and the least distance between two.
	perTown, spacing int
}

func addStoreFlags(fs *flag.FlagSet, o *storeOptions) {
	fs.Int64Var(&o.budget, "store-budget", 0, "adena each bot's private store may sell for per session; the bots open stores of their loot in towns (needs -datapack); default: open none")
	fs.IntVar(&o.perTown, "stores-per-town", 5, "most bot stores open in one town at a time")
	fs.IntVar(&o.spacing, "store-spacing", 150, "least distance between two bot stores, in game units")
}

func (o storeOptions) validate(datapack string) error {
	switch {
	case o.budget < 0:
		return fmt.Errorf("-store-budget %d: want 0 or more", o.budget)
	case o.budget > 0 && datapack == "":
		return errors.New("-store-budget needs -datapack: stores open only in the travel map's towns")
	case o.perTown < 1:
		return fmt.Errorf("-stores-per-town %d: want 1 or more", o.perTown)
	case o.spacing < 0 || o.spacing > math.MaxInt32:
		return fmt.Errorf("-store-spacing %d: want 0 to %d", o.spacing, math.MaxInt32)
	}
	return nil
}

// apply makes the bots of cfg merchants when o has a budget, sharing one
// store registry over the towns of cfg's travel map.
func (o storeOptions) apply(cfg *botclient.BrainConfig, log *slog.Logger) {
	if o.budget <= 0 || cfg.Travel == nil {
		return
	}
	towns := cfg.Travel.Towns()
	log.Info("store towns", "towns", len(towns), "per_town", o.perTown)
	cfg.Stores = botclient.NewStoreRegistry(towns, o.perTown, int32(o.spacing))
	cfg.Persona.Store.Budget = o.budget
}
