package botclient

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

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

// decodeExDuelAskStart: requester name, int32 party flag.
func decodeExDuelAskStart(r *wire.Reader) Event {
	return ExDuelAskStart{Requester: r.ReadString(), Party: r.ReadInt32() != 0}
}

// decodeExDuelUpdateUserInfo: name, object id, class id, level, then HP, max
// HP, MP, max MP, CP, max CP, each an int32.
func decodeExDuelUpdateUserInfo(r *wire.Reader) Event {
	u := ExDuelUpdateUserInfo{Name: r.ReadString(), ObjectID: r.ReadInt32()}
	u.ClassID, u.Level = r.ReadInt32(), r.ReadInt32()
	u.HP, u.MaxHP = r.ReadInt32(), r.ReadInt32()
	u.MP, u.MaxMP = r.ReadInt32(), r.ReadInt32()
	u.CP, u.MaxCP = r.ReadInt32(), r.ReadInt32()
	return u
}

// encodeRequestDuelStart challenges the player named name to a duel, or its
// party to a party duel. Its sender's pacing lands with its rule (X4); the
// per-connection frame bucket bounds it meanwhile.
func encodeRequestDuelStart(name string, party bool) []byte {
	w := wire.NewPacketWriter(opGameClientExtended)
	w.WriteUint16(opGameRequestDuelStart)
	w.WriteString(name)
	w.WriteInt32(wire.BoolInt32(party))
	return w.Bytes()
}

// encodeRequestDuelAnswerStart answers an ExDuelAskStart: party echoes the
// kind of duel asked, accepted takes it. The middle int32 is unused by the
// server.
func encodeRequestDuelAnswerStart(party, accepted bool) []byte {
	w := wire.NewPacketWriter(opGameClientExtended)
	w.WriteUint16(opGameRequestDuelAnswerStart)
	w.WriteInt32(wire.BoolInt32(party))
	w.WriteInt32(0)
	w.WriteInt32(wire.BoolInt32(accepted))
	return w.Bytes()
}

// encodeRequestTargetCancel is the Esc key: unselect 0 stops the bot's cast
// inside its interrupt window and keeps the target, else clears the target
// (network/targeting.go). The guards send one per own cast (guardState
// escFor), within the per-connection frame limits (network/floodprotector.go).
func encodeRequestTargetCancel() []byte {
	w := wire.NewPacketWriter(opGameRequestTargetCancel)
	w.WriteUint16(0) // unselect
	return w.Bytes()
}
