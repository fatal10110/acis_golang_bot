package e2e

import (
	"slices"
	"strings"
	"testing"
)

// pvpNames are the candidate names of the PvP scenarios' clients
// (pvp_live_test.go), tried in order. None carries the bot marker: a bot
// answers no duel from a marked name and fights a marked player only once
// it hit the bot first (D38). Disjoint from buyerNames, speakerNames and
// roomLeaderNames, so no two scenarios contend for a name.
var pvpNames = []string{"Brennoth", "Calvadrin", "Ithrawen", "Morvessa", "Sardovic", "Tamberlyn"}

// TestPvPNames checks the PvP clients' candidates are names they can take,
// and no other scenario's (the server compares names in any case).
func TestPvPNames(t *testing.T) {
	if err := checkUnmarkedNames(pvpNames); err != nil {
		t.Fatal(err)
	}
	for _, n := range pvpNames {
		if slices.ContainsFunc(slices.Concat(buyerNames, speakerNames, roomLeaderNames), func(o string) bool { return strings.EqualFold(o, n) }) {
			t.Errorf("PvP client name %q is another scenario's", n)
		}
	}
}
