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

// checkUnmarkedNames reports a candidate list a scenario's unmarked
// character (the valuables buyer, the chat scenario's speaker) cannot take:
// none or more than botclient.MaxNameCandidates, a spelling the server
// refuses (botnames.Valid), or a name botnames.DefaultMarker marks, which a
// bot would never answer.
func checkUnmarkedNames(names []string) error {
	if len(names) == 0 || len(names) > botclient.MaxNameCandidates {
		return fmt.Errorf("%d unmarked names, want 1 to %d", len(names), botclient.MaxNameCandidates)
	}
	for _, n := range names {
		switch {
		case !botnames.Valid(n):
			return fmt.Errorf("unmarked name %q: the server refuses its spelling", n)
		case botnames.DefaultMarker.Marks(n):
			return fmt.Errorf("unmarked name %q carries the bot marker %q", n, botnames.DefaultMarker.Text)
		}
	}
	return nil
}

// unmarkedCharacter is a human fighter of account named from names, tried
// in order. It fails the scenario when a name is marked or misspelt.
func unmarkedCharacter(t *testing.T, account string, names []string) botclient.Character {
	t.Helper()
	if err := checkUnmarkedNames(names); err != nil {
		t.Fatal(err)
	}
	ch := character(t, account, &botclient.HumanFighter)
	ch.Names = slices.Clone(names)
	return ch
}

// buyerCharacter is the buyer's character of account: a human fighter
// named from buyerNames (unmarkedCharacter).
func buyerCharacter(t *testing.T, account string) botclient.Character {
	t.Helper()
	return unmarkedCharacter(t, account, buyerNames)
}

func TestValuablesBuyerNames(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		wantErr bool
	}{
		{"the buyer's", buyerNames, false},
		{"the chat speaker's", speakerNames, false},
		{"marked suffix", []string{"Quennor", "AldenBot"}, true},
		{"marked any case", []string{"aldenbot"}, true},
		{"misspelt", []string{"Al den"}, true},
		{"too long", []string{"Quennorvarlithbrisa"}, true},
		{"none", nil, true},
		{"too many", slices.Repeat([]string{"Quennor"}, botclient.MaxNameCandidates+1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkUnmarkedNames(tt.names); (err != nil) != tt.wantErr {
				t.Errorf("checkUnmarkedNames(%q) = %v, want error %v", tt.names, err, tt.wantErr)
			}
		})
	}
}
