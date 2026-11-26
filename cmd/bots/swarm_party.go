package main

import (
	"log/slog"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// swarmParties returns the bot party organizer of the swarm's bots on the
// default policy, as the panel runs it (docs/parties/organizer.md), nil
// when -no-parties turns it off or no bot plays a party preset (farmer,
// support): the others never join bot parties.
func swarmParties(off bool, bots []botregistry.OrganizedBot, clock botclient.Clock, reg *botmetrics.Registry, log *slog.Logger) (*botregistry.Organizer, error) {
	party := slices.ContainsFunc(bots, func(b botregistry.OrganizedBot) bool {
		return b.Preset == botparty.PresetFarmer || b.Preset == botparty.PresetSupport
	})
	if off || !party {
		return nil, nil
	}
	return botregistry.NewOrganizer(botregistry.OrganizerConfig{
		Bots: bots, Policy: botparty.DefaultPolicy(), Clock: clock, Metrics: reg, Log: log,
	})
}

// organized are bots, each of preset, as the organizer orders them.
func organized(bots []*botclient.Bot, preset string) []botregistry.OrganizedBot {
	out := make([]botregistry.OrganizedBot, len(bots))
	for i, b := range bots {
		out[i] = botregistry.OrganizedBot{Account: b.Account(), Preset: preset, Bot: b}
	}
	return out
}
