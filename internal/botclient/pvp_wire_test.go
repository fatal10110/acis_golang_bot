package botclient

import (
	"testing"
	"time"
)

// The SystemMessage ids match the server's constants
// (serverpackets/systemmessage.go, systemmessage_duel.go,
// olympiad_match.go).
func TestPvPSystemMessageIDs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		got, srv int32
	}{
		{"YouDidS1Dmg", msgYouDidS1Dmg, 35},
		{"S1GaveYouS2Dmg", msgS1GaveYouS2Dmg, 36},
		{"CantAtkPeacezone", msgCantAtkPeacezone, 84},
		{"TargetInPeacezone", msgTargetInPeacezone, 85},
		{"S1ResistedYourS2", msgResisted, 139},
		{"S1IsBusyTryLater", msgS1IsBusyTryLater, 153},
		{"EnteredCombatZone", msgEnteredCombatZone, 283},
		{"LeftCombatZone", msgLeftCombatZone, 284},
		{"OtherPartyIsFrozen", msgOtherPartyIsFrozen, 692},
		{"NoOpponentForDuel", msgNoOpponentForDuel, 1926},
		{"S1ChallengedToDuel", msgS1ChallengedToDuel, 1927},
		{"S1AcceptedYourDuel", msgS1AcceptedYourDuel, 1929},
		{"YouAcceptedS1Duel", msgYouAcceptedS1Duel, 1930},
		{"S1DeclinedYourDuel", msgS1DeclinedYourDuel, 1931},
		{"S1ChallengedYouToDuel", msgS1ChallengedYouToDuel, 1938},
		{"UnableToRequestDuel", msgUnableToRequestDuel, 1940},
		{"DuelBeginsInS1Seconds", msgDuelBeginsInS1Seconds, 1945},
		{"LetTheDuelBegin", msgLetTheDuelBegin, 1949},
		{"S1WonTheDuel", msgS1WonTheDuel, 1950},
		{"DuelEndedInTie", msgDuelEndedInTie, 1952},
		{"S1WithdrewS2Won", msgS1WithdrewS2Won, 1955},
		{"S1CannotDuelPrivateStore", msgS1CannotDuelPrivateStore, 2017},
		{"S1CannotDuelFishing", msgS1CannotDuelFishing, 2018},
		{"S1CannotDuelHPOrMPBelowHalf", msgS1CannotDuelHPOrMPBelowHalf, 2019},
		{"S1CannotDuelProhibitedArea", msgS1CannotDuelProhibitedArea, 2020},
		{"S1CannotDuelInBattle", msgS1CannotDuelInBattle, 2021},
		{"S1CannotDuelAlreadyDuelling", msgS1CannotDuelAlreadyDuelling, 2022},
		{"S1CannotDuelChaotic", msgS1CannotDuelChaotic, 2023},
		{"S1CannotDuelOlympiad", msgS1CannotDuelOlympiad, 2024},
		{"S1CannotDuelRiding", msgS1CannotDuelRiding, 2027},
		{"S1CannotReceiveDuelTooFar", msgS1CannotReceiveDuelTooFar, 2028},
	} {
		if tt.got != tt.srv {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.srv)
		}
	}
}

// The bot's own MagicSkillCanceled ends its cast at once; another caster's
// leaves the bot's cast running.
func TestWorldMagicSkillCanceled(t *testing.T) {
	tests := []struct {
		name    string
		caster  int32
		casting bool
	}{
		{"own cast", selfID, false},
		{"another caster", pcID, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			w.Apply(MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: 1177, HitTime: 2 * time.Second}, t0)
			if !w.Self.Casting(t0) {
				t.Fatal("not casting after its MagicSkillUse")
			}
			w.Apply(MagicSkillCanceled{CasterID: tt.caster}, at(time.Second))
			if got := w.Self.Casting(at(time.Second)); got != tt.casting {
				t.Fatalf("Casting = %v, want %v", got, tt.casting)
			}
		})
	}
}
