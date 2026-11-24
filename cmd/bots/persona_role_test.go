package main

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestPresetRoles checks the role each preset's persona compiles its build
// for (#323): support, merchant and town are Support, crafter is Crafter.
func TestPresetRoles(t *testing.T) {
	want := map[string]botclient.Role{
		"support": botclient.RoleSupport, "merchant": botclient.RoleSupport,
		"town": botclient.RoleSupport, "crafter": botclient.RoleCrafter,
	}
	for _, p := range presets {
		if got := p.persona().Role; got != want[p.name] {
			t.Errorf("preset %s: role %v, want %v", p.name, got, want[p.name])
		}
	}
}
