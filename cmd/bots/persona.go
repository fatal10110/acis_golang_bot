package main

import (
	"flag"
	"fmt"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Persona names of the -persona flag.
const (
	personaSupport = "support"
	personaTown    = "town"
)

func addPersonaFlag(fs *flag.FlagSet, persona *string) {
	fs.StringVar(persona, "persona", personaSupport, "what every bot plays: support (party support that farms the datapack's spots) or town (idles in town and runs errands: merchants, trainer, Newbie Helper, gatekeeper hops; farms nowhere)")
}

func validatePersona(persona string) error {
	switch persona {
	case personaSupport, personaTown:
		return nil
	}
	return fmt.Errorf("-persona %q: want %s or %s", persona, personaSupport, personaTown)
}

// applyPersona gives the bots of cfg the persona named persona; town bots
// get no spot registry, so they farm nowhere.
func applyPersona(persona string, cfg *botclient.BrainConfig) {
	switch persona {
	case personaTown:
		cfg.Persona = botclient.TownPersona()
		cfg.Spots = nil
	default:
		cfg.Persona = botclient.DefaultPersona()
	}
}
