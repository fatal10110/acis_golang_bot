package botclient

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// frameSource is what the reader reads: *GameConn, or a scripted source in
// tests.
type frameSource interface {
	Read(deadline time.Time) ([]byte, error)
}

// ReadEvents is the session's reader. It reads the connection until it
// fails or done closes, decodes the frames whose opcode has a decoder into
// events and queues them on q for the brain; every other frame is dropped
// without parsing. The -record tap (SetRecorder) sees every frame read,
// decoded or not.
//
// Teardown: the session owns done and the connection. It closes done, then
// closes the connection so a blocked Read returns, and joins ReadEvents
// before it dials again; a wait on a full q ends on done alone. ReadEvents
// returns nil when it ends after done closed, else the read or decode error
// that ended it (a malformed frame ends the session). ServerClose and LeaveWorld are
// queued as events like any other; the read error after them is still
// returned.
//
// ReadEvents is the connection's only reader while it runs (see GameConn).
func (g *GameConn) ReadEvents(q *Events, done <-chan struct{}) error {
	return readEvents(g, q, done)
}

func readEvents(src frameSource, q *Events, done <-chan struct{}) error {
	for {
		raw, err := src.Read(time.Time{})
		if err != nil {
			select {
			case <-done:
				return nil
			default:
				return fmt.Errorf("read game frame: %w", err)
			}
		}
		ev, ok, err := decodeEvent(raw)
		if err != nil {
			return err
		}
		if ok && !q.push(ev, done) {
			return nil
		}
	}
}

// errTrailingBytes reports a frame longer than its decoder's layout.
var errTrailingBytes = errors.New("trailing bytes")

// eventDecoders holds a decoder per opcode the brain consumes; the reader
// drops frames of every other opcode. A decoder copies every field out of
// the frame and leaves error checks to decodeEvent.
var eventDecoders = [256]func(*wire.Reader) Event{
	opGameMoveToLocation:     decodeMoveToLocation,
	opGameDeleteObject:       decodeDeleteObject,
	opGameServerClose:        func(*wire.Reader) Event { return ServerClose{} },
	opGameTeleportToLocation: decodeTeleportToLocation,
	opGameStopMove:           decodeStopMove,
	opGameValidateLocation:   decodeValidateLocation,
	opGameLeaveWorld:         func(*wire.Reader) Event { return LeaveWorld{} },

	opGameUserInfo:         decodeUserInfoEvent,
	opGameStatusUpdate:     decodeStatusUpdate,
	opGameCharInfo:         decodeCharInfo,
	opGameNPCInfo:          decodeNPCInfo,
	opGameMoveToPawn:       decodeMoveToPawn,
	opGameDie:              decodeDie,
	opGameRevive:           func(r *wire.Reader) Event { return Revive{ObjectID: r.ReadInt32()} },
	opGameMyTargetSelected: decodeMyTargetSelected,
	opGameTargetUnselected: decodeTargetUnselected,
	opGameSystemMessage:    decodeSystemMessage,
	opGameActionFailed:     func(*wire.Reader) Event { return ActionFailed{} },
	opGameCreatureSay:      decodeCreatureSay,
	opGameChangeWaitType:   decodeChangeWaitType,
	opGameAutoAttackStart:  func(r *wire.Reader) Event { return AutoAttackStart{ObjectID: r.ReadInt32()} },
	opGameAutoAttackStop:   func(r *wire.Reader) Event { return AutoAttackStop{ObjectID: r.ReadInt32()} },

	opGameAttack:                    decodeAttack,
	opGameMagicSkillUse:             decodeMagicSkillUse,
	opGameMagicSkillLaunched:        decodeMagicSkillLaunched,
	opGameSkillList:                 decodeSkillList,
	opGameSkillCoolTime:             decodeSkillCoolTime,
	opGameRelationChanged:           decodeRelationChanged,
	opGameAbnormalStatusUpdate:      decodeAbnormalStatusUpdate,
	opGamePartySpelled:              decodePartySpelled,
	opGameAskJoinParty:              decodeAskJoinParty,
	opGameJoinParty:                 func(r *wire.Reader) Event { return JoinParty{Response: r.ReadInt32()} },
	opGamePartySmallWindowAll:       decodePartySmallWindowAll,
	opGamePartySmallWindowAdd:       decodePartySmallWindowAdd,
	opGamePartySmallWindowDeleteAll: func(*wire.Reader) Event { return PartySmallWindowDeleteAll{} },
	opGamePartySmallWindowDelete:    decodePartySmallWindowDelete,
	opGamePartySmallWindowUpdate:    func(r *wire.Reader) Event { return PartySmallWindowUpdate{Member: readPartyMember(r)} },
	opGamePartyMemberPosition:       decodePartyMemberPosition,
	opGameNpcHtmlMessage:            decodeNpcHtmlMessage,
	opGameAcquireSkillList:          decodeAcquireSkillList,
	opGameAcquireSkillInfo:          decodeAcquireSkillInfo,
	opGameAcquireSkillDone:          func(*wire.Reader) Event { return AcquireSkillDone{} },

	opGameItemList:        decodeItemList,
	opGameInventoryUpdate: decodeInventoryUpdate,
}

// extEventDecoders holds a decoder per extended (0xfe) sub-opcode the brain
// consumes, like eventDecoders; every other extended frame is dropped.
var extEventDecoders = map[uint16]func(*wire.Reader) Event{
	opGameExAutoSoulShot: decodeExAutoSoulShot,
}

// decodeEvent decodes a non-empty frame; ok is false for an opcode, or an
// extended sub-opcode, the brain does not consume, and for an extended frame
// too short to carry its sub-opcode. A frame shorter or longer than its
// layout is an error: each decoder consumes exactly len(raw).
func decodeEvent(raw []byte) (ev Event, ok bool, err error) {
	decode := eventDecoders[raw[0]]
	if raw[0] == opGameExtended && len(raw) >= 3 {
		decode = extEventDecoders[uint16(raw[1])|uint16(raw[2])<<8]
	}
	if decode == nil {
		return nil, false, nil
	}
	r := wire.NewPacketReader(raw)
	if raw[0] == opGameExtended {
		r.Skip(2) // the sub-opcode, read above
	}
	ev = decode(r)
	if err := r.Err(); err != nil {
		return nil, false, fmt.Errorf("decode opcode %s: %w", opcodeName(raw), err)
	}
	if n := r.Remaining(); n != 0 {
		return nil, false, fmt.Errorf("decode opcode %s: %w (%d)", opcodeName(raw), errTrailingBytes, n)
	}
	return ev, true, nil
}

// opcodeName names a decoded frame's opcode for an error: the opcode byte,
// and the sub-opcode of an extended frame.
func opcodeName(raw []byte) string {
	if raw[0] == opGameExtended {
		return fmt.Sprintf("%#02x:%#04x", raw[0], uint16(raw[1])|uint16(raw[2])<<8)
	}
	return fmt.Sprintf("%#02x", raw[0])
}

func readPoint(r *wire.Reader) Point {
	return Point{X: r.ReadInt32(), Y: r.ReadInt32(), Z: r.ReadInt32()}
}

// decodeMoveToLocation: object id, destination, origin
// (serverpackets/movetolocation.go).
func decodeMoveToLocation(r *wire.Reader) Event {
	return MoveToLocation{ObjectID: r.ReadInt32(), Dest: readPoint(r), Origin: readPoint(r)}
}

// decodeStopMove: object id, position, heading
// (serverpackets/movement_control.go).
func decodeStopMove(r *wire.Reader) Event {
	return StopMove{ObjectID: r.ReadInt32(), Pos: readPoint(r), Heading: r.ReadInt32()}
}

// decodeValidateLocation has StopMove's layout
// (serverpackets/movement_control.go).
func decodeValidateLocation(r *wire.Reader) Event {
	return ValidateLocation{ObjectID: r.ReadInt32(), Pos: readPoint(r), Heading: r.ReadInt32()}
}

// decodeTeleportToLocation: object id, position, fast flag as int32
// (serverpackets/teleporttolocation.go).
func decodeTeleportToLocation(r *wire.Reader) Event {
	return TeleportToLocation{ObjectID: r.ReadInt32(), Pos: readPoint(r), Fast: r.ReadInt32() != 0}
}

// decodeDeleteObject: object id, then 0 for a seated character and 1
// otherwise (serverpackets/deleteobject.go).
func decodeDeleteObject(r *wire.Reader) Event {
	return DeleteObject{ObjectID: r.ReadInt32(), Seated: r.ReadInt32() == 0}
}
