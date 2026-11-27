package botwire

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Effect is one effect icon: the skill that put it there, at Level, with
// Left to run, unless Permanent (a toggle, or an effect with no end).
type Effect struct {
	SkillID, Level int32
	Left           time.Duration
	Permanent      bool
}

// AbnormalStatusUpdate is the bot's own whole effect icon list, sent on
// every change to its effects (network/party_spelled.go updateEffectIcons).
type AbnormalStatusUpdate struct{ Effects []Effect }

// PartySpelled is the whole effect icon list of the party member ObjectID
// (Kind partySpelledPlayer) or of a member's summon, sent to every member,
// the owner included, on every change to its effects and to a member who
// joins (network/party.go IconRefresh). It is not resent as time passes.
type PartySpelled struct {
	Kind, ObjectID int32
	Effects        []Effect
}

func (AbnormalStatusUpdate) IsEvent() {}

func (PartySpelled) IsEvent() {}

// effectSize is the wire size of one effect entry: skill id, uint16
// level, seconds left.
const effectSize = 4 + 2 + 4

// decodeAbnormalStatusUpdate: a uint16 count, then per effect skill id,
// uint16 level and whole seconds left, -1 for a permanent one
// (serverpackets/status_effects.go FrameAbnormalStatusUpdate).
func decodeAbnormalStatusUpdate(r *wire.Reader) Event {
	n := int(r.ReadUint16())
	e := AbnormalStatusUpdate{Effects: make([]Effect, 0, capFor(r, n, effectSize))}
	for ; n > 0 && r.Err() == nil; n-- {
		e.Effects = append(e.Effects, readEffect(r))
	}
	return e
}

// decodePartySpelled: kind, object id, an int32 count, then per effect
// skill id, uint16 level and whole seconds left (FramePartySpelled). A
// permanent effect's -1 ms truncates to 0 there, so 0 reads permanent: an
// effect under a second from its end reads the same, and its end resends
// the list without it.
func decodePartySpelled(r *wire.Reader) Event {
	e := PartySpelled{Kind: r.ReadInt32(), ObjectID: r.ReadInt32()}
	n := int(r.ReadInt32())
	e.Effects = make([]Effect, 0, capFor(r, n, effectSize))
	for ; n > 0 && r.Err() == nil; n-- {
		e.Effects = append(e.Effects, readEffect(r))
	}
	return e
}

// readEffect reads one effect entry; seconds left under one are permanent.
func readEffect(r *wire.Reader) Effect {
	e := Effect{SkillID: r.ReadInt32(), Level: int32(r.ReadUint16())}
	if s := r.ReadInt32(); s > 0 {
		e.Left = time.Duration(s) * time.Second
	} else {
		e.Permanent = true
	}
	return e
}
