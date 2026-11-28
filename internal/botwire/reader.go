package botwire

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// ErrTrailingBytes reports a frame longer than its decoder's layout.
var ErrTrailingBytes = errors.New("trailing bytes")

// eventDecoders holds a decoder per opcode the brain consumes; the reader
// drops frames of every other opcode. A decoder copies every field out of
// the frame and leaves error checks to DecodeEvent.
var eventDecoders = [256]func(*wire.Reader) Event{
	OpGameMoveToLocation:     decodeMoveToLocation,
	OpGameDeleteObject:       decodeDeleteObject,
	OpGameServerClose:        func(*wire.Reader) Event { return ServerClose{} },
	OpGameTeleportToLocation: decodeTeleportToLocation,
	OpGameStopMove:           decodeStopMove,
	OpGameValidateLocation:   decodeValidateLocation,
	OpGameLeaveWorld:         func(*wire.Reader) Event { return LeaveWorld{} },

	OpGameUserInfo:         decodeUserInfoEvent,
	OpGameStatusUpdate:     decodeStatusUpdate,
	OpGameCharInfo:         decodeCharInfo,
	OpGameNPCInfo:          decodeNPCInfo,
	OpGameMoveToPawn:       decodeMoveToPawn,
	OpGameDie:              decodeDie,
	OpGameRevive:           func(r *wire.Reader) Event { return Revive{ObjectID: r.ReadInt32()} },
	OpGameMyTargetSelected: decodeMyTargetSelected,
	OpGameTargetUnselected: decodeTargetUnselected,
	OpGameSystemMessage:    decodeSystemMessage,
	OpGameActionFailed:     func(*wire.Reader) Event { return ActionFailed{} },
	OpGameCreatureSay:      decodeCreatureSay,
	OpGameChangeWaitType:   decodeChangeWaitType,
	OpGameAutoAttackStart:  func(r *wire.Reader) Event { return AutoAttackStart{ObjectID: r.ReadInt32()} },
	OpGameAutoAttackStop:   func(r *wire.Reader) Event { return AutoAttackStop{ObjectID: r.ReadInt32()} },
	OpGameDoorInfo:         decodeDoorInfo,
	OpGameDoorStatusUpdate: decodeDoorStatusUpdate,

	OpGameAttack:                    decodeAttack,
	OpGameMagicSkillUse:             decodeMagicSkillUse,
	OpGameMagicSkillLaunched:        decodeMagicSkillLaunched,
	OpGameMagicSkillCanceled:        func(r *wire.Reader) Event { return MagicSkillCanceled{CasterID: r.ReadInt32()} },
	OpGameSkillList:                 decodeSkillList,
	OpGameSkillCoolTime:             decodeSkillCoolTime,
	OpGameRelationChanged:           decodeRelationChanged,
	OpGameAbnormalStatusUpdate:      decodeAbnormalStatusUpdate,
	OpGameEtcStatusUpdate:           decodeEtcStatusUpdate,
	OpGamePartySpelled:              decodePartySpelled,
	OpGameAskJoinParty:              decodeAskJoinParty,
	OpGamePartyMatchList:            decodePartyMatchList,
	OpGamePartyMatchDetail:          decodePartyMatchDetail,
	OpGameConfirmDlg:                decodeConfirmDlg,
	OpGameJoinParty:                 func(r *wire.Reader) Event { return JoinParty{Response: r.ReadInt32()} },
	OpGamePartySmallWindowAll:       decodePartySmallWindowAll,
	OpGamePartySmallWindowAdd:       decodePartySmallWindowAdd,
	OpGamePartySmallWindowDeleteAll: func(*wire.Reader) Event { return PartySmallWindowDeleteAll{} },
	OpGamePartySmallWindowDelete:    decodePartySmallWindowDelete,
	OpGamePartySmallWindowUpdate:    func(r *wire.Reader) Event { return PartySmallWindowUpdate{Member: readPartyMember(r)} },
	OpGamePartyMemberPosition:       decodePartyMemberPosition,
	OpGameNpcHtmlMessage:            decodeNpcHtmlMessage,
	OpGameAcquireSkillList:          decodeAcquireSkillList,
	OpGameAcquireSkillInfo:          decodeAcquireSkillInfo,
	OpGameAcquireSkillDone:          func(*wire.Reader) Event { return AcquireSkillDone{} },
	OpGameBuyList:                   decodeBuyList,
	OpGameSellList:                  decodeSellList,
	OpGameWarehouseDepositList:      decodeWarehouseDepositList,
	OpGameWarehouseWithdrawList:     decodeWarehouseWithdrawList,
	OpGameRecipeBookItemList:        decodeRecipeBookItemList,
	OpGameRecipeItemMakeInfo:        decodeRecipeItemMakeInfo,
	OpGameHennaEquipList:            decodeHennaEquipList,
	OpGameHennaInfo:                 decodeHennaInfo,
	OpGameChooseInventoryItem:       decodeChooseInventoryItem,
	OpGameEnchantResult:             decodeEnchantResult,

	OpGamePrivateStoreManageListSell: decodePrivateStoreManageListSell,
	OpGamePrivateStoreListSell:       decodePrivateStoreListSell,
	OpGamePrivateStoreMsgSell:        decodePrivateStoreMsgSell,

	OpGameItemList:        decodeItemList,
	OpGameInventoryUpdate: decodeInventoryUpdate,

	OpGameSpawnItem: decodeSpawnItem,
	OpGameDropItem:  decodeDropItem,
	OpGameGetItem:   decodeGetItem,

	OpGameSendTradeRequest:  func(r *wire.Reader) Event { return SendTradeRequest{SenderID: r.ReadInt32()} },
	OpGameTradeStart:        decodeTradeStart,
	OpGameTradeOwnAdd:       decodeTradeOwnAdd,
	OpGameTradeOtherAdd:     decodeTradeOtherAdd,
	OpGameTradeItemUpdate:   decodeTradeItemUpdate,
	OpGameTradePressOwnOk:   func(*wire.Reader) Event { return TradePressOwnOk{} },
	OpGameTradePressOtherOk: func(*wire.Reader) Event { return TradePressOtherOk{} },
	OpGameSendTradeDone:     func(r *wire.Reader) Event { return SendTradeDone{Success: r.ReadInt32() == 1} },

	OpGameAskJoinPledge:                 decodeAskJoinPledge,
	OpGameJoinPledge:                    func(r *wire.Reader) Event { return JoinPledge{ClanID: r.ReadInt32()} },
	OpGamePledgeShowMemberListAll:       decodePledgeShowMemberListAll,
	OpGamePledgeShowMemberListDeleteAll: func(*wire.Reader) Event { return PledgeShowMemberListDeleteAll{} },
}

// extEventDecoders holds a decoder per extended (0xfe) sub-opcode the brain
// consumes, like eventDecoders; every other extended frame is dropped.
var extEventDecoders = map[uint16]func(*wire.Reader) Event{
	OpGameExAutoSoulShot:   decodeExAutoSoulShot,
	OpGameExClosePartyRoom: func(*wire.Reader) Event { return ExClosePartyRoom{} },

	OpGameExOpenMPCC:                     func(*wire.Reader) Event { return ExOpenMPCC{} },
	OpGameExCloseMPCC:                    func(*wire.Reader) Event { return ExCloseMPCC{} },
	OpGameExMPCCPartyInfoUpdate:          decodeExMPCCPartyInfoUpdate,
	OpGameExMPCCShowPartyMemberInfo:      decodeExMPCCShowPartyMemberInfo,
	OpGameExMultiPartyCommandChannelInfo: decodeExMultiPartyCommandChannelInfo,

	OpGameExDuelAskStart:       decodeExDuelAskStart,
	OpGameExDuelReady:          func(r *wire.Reader) Event { return ExDuelReady{Party: r.ReadInt32() != 0} },
	OpGameExDuelStart:          func(r *wire.Reader) Event { return ExDuelStart{Party: r.ReadInt32() != 0} },
	OpGameExDuelEnd:            func(r *wire.Reader) Event { return ExDuelEnd{Party: r.ReadInt32() != 0} },
	OpGameExDuelUpdateUserInfo: decodeExDuelUpdateUserInfo,

	OpGameExOlympiadUserInfo: decodeExOlympiadUserInfo,
	OpGameExOlympiadMode:     func(r *wire.Reader) Event { return ExOlympiadMode{Mode: r.ReadUint8()} },
	OpGameExOlympiadMatchEnd: func(*wire.Reader) Event { return ExOlympiadMatchEnd{} },
}

// DecodeEvent decodes a non-empty frame; ok is false for an opcode, or an
// extended sub-opcode, the brain does not consume, and for an extended frame
// too short to carry its sub-opcode. A frame shorter or longer than its
// layout is an error: each decoder consumes exactly len(raw).
func DecodeEvent(raw []byte) (ev Event, ok bool, err error) {
	decode := eventDecoders[raw[0]]
	if raw[0] == OpGameExtended && len(raw) >= 3 {
		decode = extEventDecoders[uint16(raw[1])|uint16(raw[2])<<8]
	}
	if decode == nil {
		return nil, false, nil
	}
	r := wire.NewPacketReader(raw)
	if raw[0] == OpGameExtended {
		r.Skip(2) // the sub-opcode, read above
	}
	ev = decode(r)
	if err := r.Err(); err != nil {
		return nil, false, fmt.Errorf("decode opcode %s: %w", opcodeName(raw), err)
	}
	if n := r.Remaining(); n != 0 {
		return nil, false, fmt.Errorf("decode opcode %s: %w (%d)", opcodeName(raw), ErrTrailingBytes, n)
	}
	return ev, true, nil
}

// opcodeName names a decoded frame's opcode for an error as the bot's logs
// name it: the opcode byte, and the sub-opcode of an extended frame (an
// extended frame too short to carry it has sub-opcode 0).
func opcodeName(raw []byte) string {
	if raw[0] != OpGameExtended {
		return fmt.Sprintf("%#02x", raw[0])
	}
	var sub uint16
	if len(raw) >= 3 {
		sub = uint16(raw[1]) | uint16(raw[2])<<8
	}
	return fmt.Sprintf("%#02x:%#04x", raw[0], sub)
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
