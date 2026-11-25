package e2e

import (
	"slices"
	"strings"
	"testing"
)

// speakerNames are the candidate names of TestLiveChatReplies' speaker
// (<prefix>ch3), tried in order. None carries the bot marker, so the bot
// under test answers its whispers: the GMs, minted (<prefix>mint<N>Bot) or
// pinned, are marked like every other run character, and a bot never
// replies to a marked name (#485). Disjoint from buyerNames, so the two
// scenarios never contend for a name.
var speakerNames = []string{"Halvaren", "Merrowyn", "Desmarel", "Corvinth", "Ilsabeth", "Turwenna"}

// TestChatSpeakerNamesDisjoint checks the speaker and the valuables buyer
// take their names from separate lists (the server compares names in any
// case).
func TestChatSpeakerNamesDisjoint(t *testing.T) {
	for _, n := range speakerNames {
		if slices.ContainsFunc(buyerNames, func(b string) bool { return strings.EqualFold(b, n) }) {
			t.Errorf("speaker name %q is a buyer name too", n)
		}
	}
}
