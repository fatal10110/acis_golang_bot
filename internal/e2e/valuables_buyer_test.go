package e2e

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// buyerNames are the candidate names of the valuables scenarios' buyer
// (docs/valuables/preset.md "Live scenarios (V9)"), tried in order: unlike
// every other run character's, none carries the bot marker, so the merchant
// answers its whispers and asks it to trade.
var buyerNames = []string{"Quennor", "Varlith", "Brisandel", "Tolmerick", "Ostravel", "Yendrimar"}

// checkBuyerNames reports a candidate list the buyer cannot take: none or
// more than botclient.MaxNameCandidates, a spelling the server refuses
// (botnames.Valid), or a name botnames.DefaultMarker marks, which the
// merchant would never answer.
func checkBuyerNames(names []string) error {
	if len(names) == 0 || len(names) > botclient.MaxNameCandidates {
		return fmt.Errorf("%d buyer names, want 1 to %d", len(names), botclient.MaxNameCandidates)
	}
	for _, n := range names {
		switch {
		case !botnames.Valid(n):
			return fmt.Errorf("buyer name %q: the server refuses its spelling", n)
		case botnames.DefaultMarker.Marks(n):
			return fmt.Errorf("buyer name %q carries the bot marker %q", n, botnames.DefaultMarker.Text)
		}
	}
	return nil
}

// buyerCharacter is the buyer's character of account: a human fighter
// named from buyerNames. It fails the scenario when a name is marked or
// misspelt.
func buyerCharacter(t *testing.T, account string) botclient.Character {
	t.Helper()
	if err := checkBuyerNames(buyerNames); err != nil {
		t.Fatal(err)
	}
	ch := character(t, account, &botclient.HumanFighter)
	ch.Names = slices.Clone(buyerNames)
	return ch
}

func TestValuablesBuyerNames(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		wantErr bool
	}{
		{"the buyer's", buyerNames, false},
		{"marked suffix", []string{"Quennor", "AldenBot"}, true},
		{"marked any case", []string{"aldenbot"}, true},
		{"misspelt", []string{"Al den"}, true},
		{"too long", []string{"Quennorvarlithbrisa"}, true},
		{"none", nil, true},
		{"too many", slices.Repeat([]string{"Quennor"}, botclient.MaxNameCandidates+1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkBuyerNames(tt.names); (err != nil) != tt.wantErr {
				t.Errorf("checkBuyerNames(%q) = %v, want error %v", tt.names, err, tt.wantErr)
			}
		})
	}
}
