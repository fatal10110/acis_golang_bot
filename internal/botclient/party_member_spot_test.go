package botclient

import (
	"testing"
	"time"
)

// TestPartyMemberClaimsNothing: a join member in its named leader's party
// follows: ruleSpot claims no spot and farms beside none
// (docs/parties/spots.md). A bot with no slot, or a join slot in a party a
// player leads, still claims.
func TestPartyMemberClaimsNothing(t *testing.T) {
	playerParty := slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Player"})
	tests := []struct {
		name  string
		slot  Command
		party PartySmallWindowAll
		claim bool
	}{
		{name: "a join member in its leader's party", slot: joinSlot, party: memberParty},
		{name: "no slot in the same party", party: memberParty, claim: true},
		{name: "a join slot in a player's party", slot: joinSlot, party: playerParty, claim: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 1, testSpot("a", 2000, 0, 1, 3, 9), testSpot("b", 0, 2000, 1, 3, 9))
			b.cfg.link = newSlotLink(standing{})
			if tt.slot.Kind != 0 {
				b.setPartySlot(tt.slot)
			}
			feed(t, b, 0, tt.party)
			b.ruleSpot(at(time.Second))
			r := b.cfg.Spots
			taken := 0
			for i := range r.spots {
				taken += r.Held(i) + r.Beside(i)
			}
			if got := b.spot.farming(); got != tt.claim || (taken > 0) != tt.claim {
				t.Fatalf("farming %v, %d held or beside (spot %+v), want claimed %v", got, taken, b.spot, tt.claim)
			}
		})
	}
}
