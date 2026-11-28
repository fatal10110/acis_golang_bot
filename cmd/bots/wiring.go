package main

import (
	"log/slog"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// wiringOptions are the flags `bots swarm` and `bots ui` both build their
// shared set from (buildShared).
type wiringOptions struct {
	allowlisted     bool
	outcomeLog      string
	datapack        string
	spawnMultiplier float64
	spawnEvents     string
	store           storeOptions
	geo             geoOptions
	// stores builds the store registry over the travel map: the swarm's
	// store presets only, the panel always (it picks presets at runtime).
	stores bool
	// servitorLines turns every bot's persona switch ServitorLines on.
	servitorLines bool
}

// wiring is the set both commands build the same way (buildShared).
type wiring struct {
	// gate spaces the logins of every bot of the process.
	gate *botclient.LoginGate
	// shared holds every piece but clanInviters and auth, the callers'.
	shared shared
	// dp is the datapack the spots were read from, nil without -datapack:
	// the panel takes its catalog from it.
	dp *datapack.Datapack
}

// buildShared builds the set both commands share: the login gate, the claims
// and boards, the budgets and the ad board, the outcome log, the navigation,
// then the spots, the travel map and the store registry over it, in that
// order: the navigation opens before the spots because the registry drops
// the spots in water (loadSpots). Without -datapack and the geodata flags
// those pieces are nil. cleanup closes the navigation, then stops and joins
// the outcome log's writer: call it once every bot returned. On an error
// buildShared has released what it built and cleanup is nil.
func buildShared(o wiringOptions, clock botclient.Clock, log *slog.Logger) (w wiring, cleanup func(), err error) {
	w.gate = botclient.NewLoginGate(clock, nil)
	if o.allowlisted {
		w.gate = botclient.NewAllowlistedLoginGate(clock, nil)
	}
	w.shared = shared{
		claims: botclient.NewPartyClaims(), ground: botclient.NewGroundClaims(), challenges: botclient.NewChallengeHolds(), board: botclient.NewCraftBoard(), storeBudget: o.store.budget,
		ads: o.store.adBoard(), valuablesBudget: o.store.valuablesBudget, servitorLines: o.servitorLines,
	}
	outcomes, stopOutcomes, err := startOutcomeLog(o.outcomeLog, log)
	if err != nil {
		return wiring{}, nil, err
	}
	// The writer takes the stays of every bot until all returned, then it
	// is joined: it stops last.
	w.shared.outcomes, cleanup = outcomes, stopOutcomes
	if o.geo.set() {
		n, closeNav, err := o.geo.open(o.datapack, log)
		if err != nil {
			cleanup()
			return wiring{}, nil, err
		}
		// The caches stay mapped until every bot returned.
		w.shared.nav, cleanup = n, func() { closeNav(); stopOutcomes() }
	}
	if o.datapack != "" {
		sh := &w.shared
		if sh.spots, sh.travel, w.dp, err = loadSpots(o.datapack, o.spawnMultiplier, supplyConfig(o.spawnEvents), sh.nav, log); err != nil {
			cleanup()
			return wiring{}, nil, err
		}
		if o.stores {
			sh.stores = o.store.registry(sh.travel, log)
		}
	}
	return w, cleanup, nil
}
