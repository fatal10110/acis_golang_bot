package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestPresetValuables: only the merchant preset enables the valuables
// policy, at its defaults with the process's -valuables-budget, and every
// merchant claims its ad lines on the one shared ad board; every other
// preset keeps the zero policy and no board (docs/valuables/preset.md
// "Wiring").
func TestPresetValuables(t *testing.T) {
	sh := shared{
		travel: &botclient.TravelMap{}, stores: botclient.NewStoreRegistry(nil, 5, 150), claims: botclient.NewPartyClaims(),
		storeBudget: 70000, ads: botclient.NewAdBoard(time.Minute, 30), valuablesBudget: 900000,
	}
	want := botclient.DefaultValuablePolicy()
	want.Budget = 900000
	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			cfg := p.brain(sh)
			if p.name != "merchant" {
				if cfg.Persona.Valuables != (botclient.ValuablePolicy{}) || cfg.Ads != nil {
					t.Errorf("valuables %+v, ads %p; want the zero policy and no board", cfg.Persona.Valuables, cfg.Ads)
				}
				return
			}
			if cfg.Persona.Valuables != want {
				t.Errorf("valuables %+v, want %+v", cfg.Persona.Valuables, want)
			}
			if other := p.brain(sh); cfg.Ads != sh.ads || other.Ads != sh.ads {
				t.Errorf("ad boards %p and %p, want the shared %p", cfg.Ads, other.Ads, sh.ads)
			}
		})
	}
}

// TestParseValuablesFlags: the valuables budget and the ad board limits
// default to D34's and the design's, apply to the merchant preset only and
// reject values that lift the limits.
func TestParseValuablesFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3", "-datapack", "xml"}
	merchant := append(base[:len(base):len(base)], "-persona", "merchant")
	tests := []struct {
		name    string
		args    []string
		budget  int64
		gap     time.Duration
		perHour int
		wantErr string
	}{
		{name: "defaults", args: merchant, budget: 2000000, gap: time.Minute, perHour: 30},
		{name: "set", args: append(merchant[:len(merchant):len(merchant)], "-valuables-budget", "500000", "-ad-region-gap", "2m", "-ads-per-hour", "10"), budget: 500000, gap: 2 * time.Minute, perHour: 10},
		{name: "budget of another preset", args: append(base[:len(base):len(base)], "-valuables-budget", "5"), wantErr: "-valuables-budget applies to -persona merchant"},
		{name: "gap of another preset", args: append(base[:len(base):len(base)], "-persona", "town", "-ad-region-gap", "2m"), wantErr: "-ad-region-gap applies to -persona merchant"},
		{name: "rate of another preset", args: append(base[:len(base):len(base)], "-ads-per-hour", "5"), wantErr: "-ads-per-hour applies to -persona merchant"},
		{name: "zero budget", args: append(merchant[:len(merchant):len(merchant)], "-valuables-budget", "0"), wantErr: "-valuables-budget 0"},
		{name: "no region gap", args: append(merchant[:len(merchant):len(merchant)], "-ad-region-gap", "0s"), wantErr: "-ad-region-gap"},
		{name: "no line an hour", args: append(merchant[:len(merchant):len(merchant)], "-ads-per-hour", "0"), wantErr: "-ads-per-hour"},
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
			if err != nil {
				t.Fatal(err)
			}
			if s := o.store; s.valuablesBudget != tt.budget || s.adRegionGap != tt.gap || s.adsPerHour != tt.perHour {
				t.Fatalf("valuables budget %d, ad gap %v, ads an hour %d; want %d, %v, %d", s.valuablesBudget, s.adRegionGap, s.adsPerHour, tt.budget, tt.gap, tt.perHour)
			}
		})
	}
}
