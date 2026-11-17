package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// preset is one entry of the persona catalog: a persona plus its options
// (docs/plan.md section 8). Each bot gets its own BrainConfig from its
// preset (preset.brain), pointing at the process-wide pieces it uses.
type preset struct {
	name string
	help string
	// datapack is set for a preset offered only with the datapack: its
	// bots need the travel map or the farming spots.
	datapack bool
	persona  func() botclient.Persona
	// farms gives the bots the spot registry: without it they farm
	// nowhere.
	farms bool
	// store makes the bots merchants: they open private stores of their
	// loot with the store budget.
	store bool
}

// presets is the persona catalog, the -persona names in order.
var presets = []preset{
	{name: "support", help: "party support that farms the datapack's spots", persona: botclient.DefaultPersona, farms: true},
	{name: "town", help: "idles in town and runs errands: merchants, trainer, Newbie Helper, gatekeeper hops; farms nowhere (needs -datapack)", datapack: true, persona: botclient.TownPersona},
	{name: "merchant", help: "support that farms the datapack's spots for loot and sells it in a town store, up to -store-budget a session (needs -datapack)", datapack: true, persona: botclient.DefaultPersona, farms: true, store: true},
}

// findPreset returns the preset of the catalog named name.
func findPreset(name string) (preset, bool) {
	for _, p := range presets {
		if p.name == name {
			return p, true
		}
	}
	return preset{}, false
}

func addPersonaFlag(fs *flag.FlagSet, persona *string) {
	var b strings.Builder
	b.WriteString("what every bot plays, a preset of the catalog:")
	for _, p := range presets {
		fmt.Fprintf(&b, " %s (%s);", p.name, p.help)
	}
	fs.StringVar(persona, "persona", presets[0].name, strings.TrimSuffix(b.String(), ";"))
}

// validatePersona checks the -persona value: a preset of the catalog, one
// that needs the datapack only with it.
func validatePersona(persona, datapack string) error {
	p, ok := findPreset(persona)
	if !ok {
		names := make([]string, len(presets))
		for i, p := range presets {
			names[i] = p.name
		}
		return fmt.Errorf("-persona %q: want one of %s", persona, strings.Join(names, ", "))
	}
	if p.datapack && datapack == "" {
		return fmt.Errorf("-persona %s needs -datapack: its bots use the travel map", persona)
	}
	return nil
}

// shared are the process-wide pieces, one instance each, that every
// preset's BrainConfig points at; a nil one is left out. claims is always
// set: one bot per non-GM party leader holds across presets.
type shared struct {
	spots  *botclient.SpotRegistry
	travel *botclient.TravelMap
	stores *botclient.StoreRegistry
	claims *botclient.PartyClaims
	auth   *botclient.Authority
	// storeBudget is a merchant's store budget per session.
	storeBudget int64
}

// brain returns the BrainConfig of a bot of p over s; the caller sets its
// Metrics. A preset leaves out the pieces it does not use: only farming
// presets get the spot registry, only merchants the store registry.
func (p preset) brain(s shared) botclient.BrainConfig {
	cfg := botclient.BrainConfig{Persona: p.persona(), Travel: s.travel}
	if p.farms {
		cfg.Spots = s.spots
	}
	if p.store && s.stores != nil {
		cfg.Stores = s.stores
		cfg.Persona.Store.Budget = s.storeBudget
	}
	applyPartyAndAuthority(&cfg, s.claims, s.auth)
	return cfg
}

// applyPartyAndAuthority points cfg at claims, the party leader claim table
// of every bot of the process, and at auth when set. The claim limits the
// persona's own party hook, which auth's hooks ask only for a requester auth
// does not know as a GM: a GM's invitation is never limited. Without auth no
// GM is known, and every invitation is limited.
func applyPartyAndAuthority(cfg *botclient.BrainConfig, claims *botclient.PartyClaims, auth *botclient.Authority) {
	claims.Apply(cfg)
	if auth != nil {
		cfg.Authority = auth
		cfg.Persona = auth.Hooks(cfg.Persona)
	}
}
