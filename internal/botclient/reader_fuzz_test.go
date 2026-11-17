package botclient

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/testsupport/decodefuzz"
)

// FuzzDecodeEvent: any non-empty frame decodes or fails without panicking,
// within a heap budget bounded by its length, and an accepted frame decodes
// to the same event again.
func FuzzDecodeEvent(f *testing.F) {
	f.Add(moveToLocation(1, Point{X: 1}, Point{Y: 2}))
	f.Add(stopMove(1, Point{Z: 3}, 4))
	f.Add(packet(opGameDeleteObject, 1, 0))
	f.Add(packet(opGameTeleportToLocation, 1, 2, 3, 4))
	f.Add(packet(opGameLeaveWorld))
	f.Add([]byte{opGameUserInfo, 1})
	f.Add(userInfoPacket(testUserInfo, 120, 80, 1.3, 2))
	f.Add(charInfoPacket(testCharInfo, 120, 80, 1.3, 1))
	f.Add(npcInfoPacket(testNPCInfo, 130, 40, 1.5))
	f.Add(packet(opGameStatusUpdate, 7, 2, 9, 300, 18, 333))
	f.Add(packet(opGameSystemMessage, 1147, 3, 0, "text", 4, 1216, 2, 7, 1, 2, 3))
	f.Add(packet(opGameCreatureSay, 7, 1, "bot7", "hello"))
	f.Add(packet(opGameAskJoinParty, "GM", 1))
	f.Add(partyWindowAll(gmID, 1, []int32{0, 3}, gmRow, mateRow))
	f.Add(packet(opGamePartySmallWindowUpdate, vitals(gmRow)...))
	f.Add(packet(opGamePartyMemberPosition, 2, selfID, 1, 2, 3, gmID, -4, 5, -6))
	f.Add(itemList(1, potionRow, swordRow))
	f.Add(inventoryUpdate(ItemChange{ItemAdded, shotRow}, ItemChange{ItemRemoved, potionRow}))
	f.Add(exAutoSoulShot(1835, 1))
	f.Add([]byte{0xfe, 0x2e, 0x00})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) == 0 {
			return // GameConn.Read rejects empty frames
		}
		decodefuzz.Bounded(t, "decodeEvent", raw, func() { _, _, _ = decodeEvent(raw) })
		ev, ok, err := decodeEvent(raw)
		if err != nil || !ok {
			return
		}
		again, _, _ := decodeEvent(raw)
		if !reflect.DeepEqual(again, ev) {
			t.Fatalf("decodeEvent not repeatable: %v then %v", ev, again)
		}
	})
}
