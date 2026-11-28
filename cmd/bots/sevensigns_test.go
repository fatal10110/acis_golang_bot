package main

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestParseSevenSignsFlags: Seven Signs member mode applies to -persona
// pvp alone, and config loading refuses a member list longer than
// -sevensigns-max-members (1 by default) and the Seal of Strife (D43).
func TestParseSevenSignsFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3", "-datapack", "dp"}
	pvp := append(slices.Clone(base), "-persona", "pvp")
	tests := []struct {
		name    string
		args    []string
		want    botclient.SevenSignsPolicy
		wantErr string
	}{
		{name: "off by default", args: pvp, want: botclient.SevenSignsPolicy{MaxMembers: 1}},
		{name: "one member", args: append(slices.Clone(pvp), "-sevensigns-members", " Alden "), want: botclient.SevenSignsPolicy{Members: []string{"Alden"}, MaxMembers: 1}},
		{name: "two members over the default cap", args: append(slices.Clone(pvp), "-sevensigns-members", "Alden,Grae"), wantErr: "2 members, at most 1"},
		{
			name: "two members under a raised cap", args: append(slices.Clone(pvp), "-sevensigns-members", "Alden,,Grae", "-sevensigns-max-members", "2"),
			want: botclient.SevenSignsPolicy{Members: []string{"Alden", "Grae"}, MaxMembers: 2},
		},
		{name: "a cap of 0 refuses any member", args: append(slices.Clone(pvp), "-sevensigns-members", "Alden", "-sevensigns-max-members", "0"), wantErr: "1 members, at most 0"},
		{name: "avarice", args: append(slices.Clone(pvp), "-signs-seal", "Avarice"), want: botclient.SevenSignsPolicy{MaxMembers: 1, Seal: botclient.SealAvarice}},
		{name: "gnosis", args: append(slices.Clone(pvp), "-signs-seal", "gnosis"), want: botclient.SevenSignsPolicy{MaxMembers: 1, Seal: botclient.SealGnosis}},
		{name: "strife refused", args: append(slices.Clone(pvp), "-signs-seal", "strife"), wantErr: "never votes the Seal of Strife"},
		{name: "unknown seal", args: append(slices.Clone(pvp), "-signs-seal", "dawn"), wantErr: "want avarice or gnosis"},
		{name: "members for another preset", args: append(slices.Clone(base), "-sevensigns-members", "Alden"), wantErr: "-sevensigns-members applies to -persona pvp"},
		{name: "seal for another preset", args: append(slices.Clone(base), "-signs-seal", "gnosis"), wantErr: "-signs-seal applies to -persona pvp"},
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
			got := o.sevenSigns
			if !slices.Equal(got.Members, tt.want.Members) || got.MaxMembers != tt.want.MaxMembers || got.Seal != tt.want.Seal {
				t.Fatalf("seven signs = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestSevenSignsPresetBrain: only the pvp preset's brain gets the member
// mode; every preset plays it off by default (D6).
func TestSevenSignsPresetBrain(t *testing.T) {
	policy := botclient.SevenSignsPolicy{Members: []string{"Alden"}, MaxMembers: 1}
	for _, p := range presets {
		sh := shared{travel: &botclient.TravelMap{}, claims: botclient.NewPartyClaims(), sevenSigns: policy}
		got := p.brain(sh).Persona.SevenSigns
		if want := p.name == presetPvP; (len(got.Members) > 0) != want {
			t.Errorf("preset %s: seven signs %+v, want members %v", p.name, got, want)
		}
		if def := p.persona().SevenSigns; len(def.Members) > 0 {
			t.Errorf("preset %s names Seven Signs members by default: %+v", p.name, def)
		}
	}
}
