package main

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// TestPresetRoles checks the role each preset's persona compiles its build
// for (#323, #328, #331, D21): support and town are Support, farmer,
// merchant and pvp are Damage (they farm alone), crafter is Crafter.
func TestPresetRoles(t *testing.T) {
	want := map[string]botclient.Role{
		"farmer": botclient.RoleDamage, "support": botclient.RoleSupport, "merchant": botclient.RoleDamage,
		"town": botclient.RoleSupport, "crafter": botclient.RoleCrafter, "pvp": botclient.RoleDamage,
	}
	for _, p := range presets {
		if got := p.persona().Role; got != want[p.name] {
			t.Errorf("preset %s: role %v, want %v", p.name, got, want[p.name])
		}
	}
}

// TestDefaultPresetIsFarmer checks the default -persona and the panel's first
// preset is farmer (D21), and that a farmer gets the spot registry.
func TestDefaultPresetIsFarmer(t *testing.T) {
	if presets[0].name != "farmer" {
		t.Fatalf("first preset %q, want farmer", presets[0].name)
	}
	sh := shared{spots: botclient.NewSpotRegistry(nil, 1), claims: botclient.NewPartyClaims()}
	cfg := presets[0].brain(sh)
	if cfg.Spots != sh.spots || cfg.Persona.Role != botclient.RoleDamage || !cfg.Persona.ClanMember {
		t.Fatalf("farmer brain: spots %p role %v member %v, want the spot registry, damage, clan member", cfg.Spots, cfg.Persona.Role, cfg.Persona.ClanMember)
	}
}

// TestUIRunnerPinsBranch checks a panel run's brain config carries the
// bot's pinned branch from the registry (D22) and its metrics slot.
func TestUIRunnerPinsBranch(t *testing.T) {
	u := uiRunner{shared: shared{claims: botclient.NewPartyClaims()}}
	p, _ := findPreset("farmer")
	slot := botmetrics.NewRegistry().NewBot()
	for _, branch := range []int32{0, 7} {
		cfg := u.brain(p, botregistry.Run{Account: "hz1", Preset: "farmer", Branch: branch, Metrics: slot})
		if cfg.Branch != branch || cfg.Metrics != slot || cfg.Persona.Role != botclient.RoleDamage {
			t.Fatalf("pin %d: brain branch %d metrics %p role %v", branch, cfg.Branch, cfg.Metrics, cfg.Persona.Role)
		}
	}
}
