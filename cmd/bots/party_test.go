package main

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestApplyPartyAndAuthority checks the swarm's party wiring: one claim
// table limits the persona's invitations from a player, and a GM the
// authority knows is never limited.
func TestApplyPartyAndAuthority(t *testing.T) {
	const gmID, playerID = int32(0x20000201), int32(0x20000202)
	t0 := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	world := func(self int32) *botclient.World {
		w, _ := botclient.NewWorld(botclient.Self{ObjectID: self, Name: "bot"}, t0)
		w.Apply(botclient.CharInfo{ObjectID: gmID, Name: "Gm"}, t0)
		w.Apply(botclient.CharInfo{ObjectID: playerID, Name: "Player"}, t0)
		return w
	}
	tests := []struct {
		name      string
		auth      bool
		requester string
		want      [2]bool // the first bot's answer, then the second's
	}{
		{"player, with authority", true, "Player", [2]bool{true, false}},
		{"GM, with authority", true, "Gm", [2]bool{true, true}},
		{"player, no authority", false, "Player", [2]bool{true, false}},
		{"GM unknown without authority", false, "Gm", [2]bool{true, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := botclient.BrainConfig{Persona: botclient.DefaultPersona()}
			var auth *botclient.Authority
			if tt.auth {
				auth = botclient.NewAuthority(botclient.AuthorityConfig{Commanders: []int32{gmID}})
			}
			applyPartyAndAuthority(&cfg, botclient.NewPartyClaims(), auth)
			if cfg.PartyClaims == nil || cfg.Authority != auth {
				t.Fatalf("PartyClaims %p, Authority %p, want a table and %p", cfg.PartyClaims, cfg.Authority, auth)
			}
			for i, self := range []int32{0x10000201, 0x10000202} {
				if got := cfg.Persona.JoinParty(world(self), tt.requester, t0); got != tt.want[i] {
					t.Errorf("bot %d JoinParty(%q) = %v, want %v", i, tt.requester, got, tt.want[i])
				}
			}
		})
	}
}
