package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestParsePersonaFlag(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{"default", base, "farmer", ""},
		{"farmer", append(base, "-persona", "farmer"), "farmer", ""},
		{"town", append(base, "-persona", "town", "-datapack", "dp"), "town", ""},
		{"town without datapack", append(base, "-persona", "town"), "", "-datapack"},
		{"merchant", append(base, "-persona", "merchant", "-datapack", "dp"), "merchant", ""},
		{"merchant without datapack", append(base, "-persona", "merchant"), "", "-datapack"},
		{"support", append(base, "-persona", "support"), "support", ""},
		{"unknown", append(base, "-persona", "bogus"), "", "want one of farmer, support, town, merchant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseSwarmFlags(tt.args, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || o.persona != tt.want {
				t.Fatalf("persona = %q, %v, want %q", o.persona, err, tt.want)
			}
		})
	}
}

// TestPresetBrain: each preset yields its persona, and the BrainConfigs of
// every preset point at one instance of each process-wide piece they use:
// town bots get no spot registry and only merchants the store registry and
// a budget.
func TestPresetBrain(t *testing.T) {
	sh := shared{
		spots:       botclient.NewSpotRegistry(nil, 1),
		travel:      &botclient.TravelMap{},
		stores:      botclient.NewStoreRegistry(nil, 5, 150),
		claims:      botclient.NewPartyClaims(),
		ground:      botclient.NewGroundClaims(),
		auth:        botclient.NewAuthority(botclient.AuthorityConfig{}),
		board:       botclient.NewCraftBoard(),
		storeBudget: 70000,
	}
	tests := []struct {
		preset     string
		wantHops   bool
		wantSpots  bool
		wantBudget int64
		wantGround botclient.GroundLoot
	}{
		{"farmer", false, true, 0, botclient.GroundAny},
		{"support", false, true, 0, botclient.GroundFit},
		{"town", true, false, 0, botclient.GroundFit},
		{"merchant", false, true, 70000, botclient.GroundAny},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			p, ok := findPreset(tt.preset)
			if !ok {
				t.Fatalf("preset %q not in the catalog", tt.preset)
			}
			cfg := p.brain(sh)
			if got := cfg.Persona.Town.HopMax > 0; got != tt.wantHops {
				t.Errorf("town hops %v, want %v", got, tt.wantHops)
			}
			if got := cfg.Persona.Store.Budget; got != tt.wantBudget {
				t.Errorf("store budget %d, want %d", got, tt.wantBudget)
			}
			if got := cfg.Spots != nil; got != tt.wantSpots || got && cfg.Spots != sh.spots {
				t.Errorf("spots %p, want shared %p: %v", cfg.Spots, sh.spots, tt.wantSpots)
			}
			if wantStores := tt.wantBudget > 0; (cfg.Stores != nil) != wantStores || wantStores && cfg.Stores != sh.stores {
				t.Errorf("stores %p, want shared %p: %v", cfg.Stores, sh.stores, wantStores)
			}
			if cfg.Travel != sh.travel || cfg.PartyClaims != sh.claims || cfg.Authority != sh.auth {
				t.Errorf("travel %p, claims %p, authority %p; want the shared %p, %p, %p", cfg.Travel, cfg.PartyClaims, cfg.Authority, sh.travel, sh.claims, sh.auth)
			}
			if cfg.Ground != sh.ground || cfg.Persona.Loot.Ground != tt.wantGround {
				t.Errorf("ground claims %p (want the shared %p), ground loot %d, want %d", cfg.Ground, sh.ground, cfg.Persona.Loot.Ground, tt.wantGround)
			}
			if cfg.CraftBoard != sh.board {
				t.Errorf("craft board %p, want the shared %p", cfg.CraftBoard, sh.board)
			}
			if cfg.Persona.JoinParty == nil {
				t.Error("JoinParty nil: the claim and the GM hooks do not wrap the persona")
			}
			if cfg.Persona.JoinClan == nil {
				t.Error("JoinClan nil: the GM hook does not wrap the persona")
			}
		})
	}
}

// TestPresetClaimsAcrossPresets: a support bot and a merchant built from
// one shared table hold one claim per party leader between them.
func TestPresetClaimsAcrossPresets(t *testing.T) {
	sh := shared{claims: botclient.NewPartyClaims()}
	support, _ := findPreset("support")
	merchant, _ := findPreset("merchant")
	a, b := support.brain(sh), merchant.brain(sh)
	if a.PartyClaims != b.PartyClaims || a.PartyClaims != sh.claims {
		t.Fatalf("claims %p and %p, want one table %p", a.PartyClaims, b.PartyClaims, sh.claims)
	}
	const playerID = int32(0x20000202)
	t0 := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	world := func(self int32) *botclient.World {
		w, err := botclient.NewWorld(botclient.Self{ObjectID: self, Name: "bot"}, t0)
		if err != nil {
			t.Fatal(err)
		}
		w.Apply(botclient.CharInfo{ObjectID: playerID, Name: "Player"}, t0)
		return w
	}
	if !a.Persona.JoinParty(world(0x10000201), "Player", t0) {
		t.Fatal("support bot refused the first invitation")
	}
	if b.Persona.JoinParty(world(0x10000202), "Player", t0) {
		t.Fatal("merchant joined a leader the support bot holds")
	}
}
