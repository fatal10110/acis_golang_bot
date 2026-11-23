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
	f.Add(packet(opGameDoorInfo, 0x10000001, 24190001, 0, 1, 1, 1000, 1000, 0, 0))
	f.Add(packet(opGameDoorStatusUpdate, 0x10000001, 0, 0, 0, 24190001, 1000, 1000))
	f.Add(packet(opGameTeleportToLocation, 1, 2, 3, 4))
	f.Add(packet(opGameLeaveWorld))
	f.Add([]byte{opGameUserInfo, 1})
	f.Add(userInfoPacket(testUserInfo, 120, 80, 1.3, 2))
	f.Add(charInfoPacket(testCharInfo, 120, 80, 1.3, 1))
	f.Add(npcInfoPacket(testNPCInfo, 130, 40, 1.5))
	f.Add(packet(opGameStatusUpdate, 7, 2, 9, 300, 18, 333))
	f.Add(packet(opGameEtcStatusUpdate, 0, 2, 0, 0, 0, 0, 3))
	f.Add(packet(opGameSystemMessage, 1147, 3, 0, "text", 4, 1216, 2, 7, 1, 2, 3))
	f.Add(packet(opGameCreatureSay, 7, 1, "bot7", "hello"))
	f.Add(packet(opGameAskJoinParty, "GM", 1))
	f.Add(packet(opGameAskJoinPledge, gmID, "Crafters"))
	f.Add(clanRoster(clanID, 0, "Crafters", "GM", "GM", "bot7"))
	f.Add(packet(opGameConfirmDlg, 1842, 2, 0, "GM", 7, 1, 2, 3, 30000, gmID))
	f.Add(partyWindowAll(gmID, 1, []int32{0, 3}, gmRow, mateRow))
	f.Add(packet(opGamePartySmallWindowUpdate, vitals(gmRow)...))
	f.Add(packet(opGamePartyMemberPosition, 2, selfID, 1, 2, 3, gmID, -4, 5, -6))
	f.Add(packet(opGameSendTradeRequest, pcID))
	f.Add(packet(opGameTradeStart, append([]any{pcID, uint16(1)}, tradeRow(goodsRow)...)...))
	f.Add(packet(opGameTradeOtherAdd, append([]any{uint16(1)}, tradeRow(adenaRow(500))...)...))
	f.Add(packet(opGameTradeItemUpdate, append([]any{uint16(1), uint16(3)}, tradeRow(goodsRow)...)...))
	f.Add(packet(opGameSendTradeDone, 1))
	f.Add(packet(opGameNpcHtmlMessage, trainerObj, trainerPage(trainerObj), 0))
	f.Add(skillList(AcquirableSkill{1011, 1, 160}, AcquirableSkill{1068, 1, 470}))
	f.Add(packet(opGameAcquireSkillInfo, 1011, 1, 160, 0, 1, 99, 1152, 1, 50))
	f.Add(itemList(1, potionRow, swordRow))
	f.Add(inventoryUpdate(ItemChange{ItemAdded, shotRow}, ItemChange{ItemRemoved, potionRow}))
	f.Add(exAutoSoulShot(1835, 1))
	f.Add(packet(opGameSpawnItem, adenaDrop, 57, 1, 2, 3, 1, 120, 0))
	f.Add(packet(opGameDropItem, npcID, junkDrop, junkTemplate, 950, 0, 0, 1, 3, 1))
	f.Add(packet(opGameGetItem, pcID, adenaDrop, 900, 20, -5))
	f.Add(shopWindow(opGameBuyList, 5000, 29, 3, bookOffer, potionOffer))
	f.Add(shopWindow(opGameSellList, 300, 0, 1, skinRow))
	f.Add(manageList(selfID, 1000, []StoreItem{skinCand, swordCand}, []StoreItem{swordListed}))
	f.Add(packet(opGamePrivateStoreListSell, append([]any{selfID, int32(0), int32(5000)}, storeRowFields(true, swordListed)...)...))
	f.Add(packet(opGamePrivateStoreMsgSell, selfID, "Loot for sale"))
	f.Add(packet(opGameHennaEquipList, int32(50000), int32(2), int32(1), int32(1), int32(4445), int32(10), int32(37000), int32(1)))
	f.Add(packet(opGameHennaInfo, byte(0), byte(1), byte(0), byte(0), byte(0), byte(0), int32(2), int32(1), int32(1), int32(1)))
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
