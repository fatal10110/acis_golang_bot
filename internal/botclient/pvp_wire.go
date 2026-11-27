package botclient

// SystemMessage ids the PvP rules read, named as the server names them
// (serverpackets/systemmessage.go, systemmessage_duel.go,
// olympiad_match.go). 139 is msgResisted (spoil.go). S1 and S2 are
// character names unless noted.
const (
	msgYouDidS1Dmg           = 35  // S1 a number
	msgS1GaveYouS2Dmg        = 36  // S2 a number
	msgCantAtkPeacezone      = 84  // the bot stands in a peace zone
	msgTargetInPeacezone     = 85  // its target does
	msgS1IsBusyTryLater      = 153 // the challenged player is busy
	msgEnteredCombatZone     = 283 // an arena or a running siege
	msgLeftCombatZone        = 284
	msgOtherPartyIsFrozen    = 692 // the duel opponent is frozen
	msgNoOpponentForDuel     = 1926
	msgS1ChallengedToDuel    = 1927
	msgS1AcceptedYourDuel    = 1929
	msgYouAcceptedS1Duel     = 1930
	msgS1DeclinedYourDuel    = 1931
	msgS1ChallengedYouToDuel = 1938
	msgUnableToRequestDuel   = 1940
	msgDuelBeginsInS1Seconds = 1945 // S1 a number
	msgLetTheDuelBegin       = 1949
	msgS1WonTheDuel          = 1950
	msgDuelEndedInTie        = 1952
	msgS1WithdrewS2Won       = 1955

	// The party duel's (network/duel.go requestDuelStart, answerDuel,
	// beginDuel, duelEndMessages): the acceptances name the other side's
	// leader, the results a side by its leader's name; 1944 goes to every
	// member of both sides as the duel begins, and only a leader ever sees
	// the challenge.
	msgYouAcceptedS1PartyDuel    = 1933
	msgS1AcceptedYourPartyDuel   = 1934
	msgOpposingPartyDeclined     = 1936
	msgChallengedNotInParty      = 1937
	msgOpposingPartyUnableToDuel = 1942
	msgTransportedToDuelSite     = 1944
	msgS1PartyWonTheDuel         = 1951
	msgS1PartyWithdrewS2Won      = 1956

	// The refusals of a challenge, S1 the player who cannot duel. The
	// server has no 2025 or 2026.
	msgS1CannotDuelPrivateStore    = 2017
	msgS1CannotDuelFishing         = 2018
	msgS1CannotDuelHPOrMPBelowHalf = 2019
	msgS1CannotDuelProhibitedArea  = 2020
	msgS1CannotDuelInBattle        = 2021
	msgS1CannotDuelAlreadyDuelling = 2022
	msgS1CannotDuelChaotic         = 2023
	msgS1CannotDuelOlympiad        = 2024
	msgS1CannotDuelRiding          = 2027
	msgS1CannotReceiveDuelTooFar   = 2028
)
