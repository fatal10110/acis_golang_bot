package e2e

import (
	"slices"
	"strings"
	"testing"
)

// roomLeaderNames are the candidate names of TestLivePartyRoomJoin's room
// leader (<prefix>pr2), tried in order. None carries the bot marker: a bot
// asks to enter no room whose leader's name is marked, and every GM is
// marked (#485). Disjoint from buyerNames and speakerNames, so no two
// scenarios contend for a name.
var roomLeaderNames = []string{"Orlavine", "Kestrimor", "Pellandra", "Wystarel", "Fennomir", "Adrevain"}

// TestRoomLeaderNames checks the room leader's candidates are names it can
// take, and no other scenario's (the server compares names in any case).
func TestRoomLeaderNames(t *testing.T) {
	if err := checkUnmarkedNames(roomLeaderNames); err != nil {
		t.Fatal(err)
	}
	for _, n := range roomLeaderNames {
		if slices.ContainsFunc(slices.Concat(buyerNames, speakerNames), func(o string) bool { return strings.EqualFold(o, n) }) {
			t.Errorf("room leader name %q is another scenario's", n)
		}
	}
}
