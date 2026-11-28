package botwire

// Opcodes of the game packets the bot reads and sends, redeclared from the
// acis_golang server's packet packages (the file each value comes from is
// named in its group). Login and game opcodes overlap, so each constant
// names its protocol and direction.

// Game server to client (internal/gameserver/network/serverpackets).
const (
	OpGameVersionCheck       = 0x00 // versioncheck.go
	OpGameMoveToLocation     = 0x01 // movetolocation.go
	OpGameUserInfo           = 0x04 // userinfo.go
	OpGameDeleteObject       = 0x12 // deleteobject.go
	OpGameCharSelectInfo     = 0x13 // charselectinfo.go
	OpGameAuthLoginFail      = 0x14 // authloginfail.go
	OpGameCharSelected       = 0x15 // charselected.go
	OpGameCharCreateOk       = 0x19 // charcreateok.go
	OpGameCharCreateFail     = 0x1a // charcreatefail.go
	OpGameActionFailed       = 0x25 // actionfailed.go
	OpGameServerClose        = 0x26 // serverclose.go
	OpGameTeleportToLocation = 0x28 // teleporttolocation.go
	OpGameStopMove           = 0x47 // movement_control.go
	OpGameValidateLocation   = 0x61 // movement_control.go
	OpGameLeaveWorld         = 0x7e // lifecycle.go
)

// Game server to client, in-world state (internal/gameserver/network/serverpackets).
const (
	OpGameCharInfo         = 0x03 // charinfo.go
	OpGameDie              = 0x06 // die.go
	OpGameRevive           = 0x07 // lifecycle.go
	OpGameStatusUpdate     = 0x0e // targeting.go
	OpGameNPCInfo          = 0x16 // npcinfo.go
	OpGameTargetUnselected = 0x2a // targeting.go
	OpGameAutoAttackStart  = 0x2b // stance_social.go
	OpGameAutoAttackStop   = 0x2c // autoattack.go
	OpGameChangeWaitType   = 0x2f // stance_social.go
	OpGameCreatureSay      = 0x4a // creaturesay.go
	OpGameDoorInfo         = 0x4c // worldobjects.go
	OpGameDoorStatusUpdate = 0x4d // worldobjects.go
	OpGameMoveToPawn       = 0x60 // movetopawn.go
	OpGameSystemMessage    = 0x64 // systemmessage.go
	OpGameMyTargetSelected = 0xa6 // targeting.go
)

// Client to game server (internal/gameserver/network/clientpackets).
const (
	OpGameProtocolVersion        = 0x00 // protocolversion.go
	OpGameEnterWorld             = 0x03 // enterworld.go
	OpGameAuthLogin              = 0x08 // authlogin.go
	OpGameLogout                 = 0x09 // lifecycle.go
	OpGameRequestCharacterCreate = 0x0b // requestcharactercreate.go
	OpGameRequestGameStart       = 0x0d // requestgamestart.go
)

// Client to game server, sent by the brain in the world
// (internal/gameserver/network/clientpackets).
const (
	OpGameMoveBackwardToLocation = 0x01 // movebackwardtolocation.go
	OpGameAction                 = 0x04 // wiresafe.go, targetaction.go
	OpGameRequestItemList        = 0x0f // client_dispatch.go, client_ingame.go
	OpGameRequestSocialAction    = 0x1b // wiresafe.go, stance_social.go
	OpGameAppearing              = 0x30 // wiresafe.go, appearing.go
	OpGameSay2                   = 0x38 // say2.go
	OpGameRequestActionUse       = 0x45 // wiresafe.go, requestactionuse.go
	OpGameValidatePosition       = 0x48 // validateposition.go
	OpGameRequestRestartPoint    = 0x6d // requestrestartpoint.go
	// OpGameSendBypassBuildCmd is a GM's admin command; only the e2e
	// harness's GM client sends it (EncodeSendBypassBuildCmd).
	OpGameSendBypassBuildCmd = 0x5b // admin.go
)

// Game server to client, combat and skills
// (internal/gameserver/network/serverpackets).
const (
	OpGameAttack             = 0x05 // attack.go
	OpGameMagicSkillUse      = 0x48 // magic_skill.go
	OpGameSkillList          = 0x58 // skilllist.go
	OpGameMagicSkillLaunched = 0x76 // magic_skill.go
	OpGameSkillCoolTime      = 0xc1 // skillcooltime.go
	OpGameRelationChanged    = 0xce // relationchanged.go
)

// Game server to client, effect icons
// (internal/gameserver/network/serverpackets).
const (
	OpGameAbnormalStatusUpdate = 0x7f // status_effects.go
	OpGamePartySpelled         = 0xee // status_effects.go
	OpGameEtcStatusUpdate      = 0xf3 // etcstatusupdate.go
)

// Client to game server, combat (internal/gameserver/network/clientpackets).
const (
	OpGameAttackRequest        = 0x0a // wiresafe.go, targetaction.go
	OpGameRequestMagicSkillUse = 0x2f // wiresafe.go, magic_skill_use.go
)

// Game server to client, ground items
// (internal/gameserver/network/serverpackets).
const (
	OpGameSpawnItem = 0x0b // grounditems.go
	OpGameDropItem  = 0x0c // grounditems.go
	OpGameGetItem   = 0x0d // grounditems.go
)

// Client to game server, ground items
// (internal/gameserver/network/clientpackets).
const (
	OpGameRequestDropItem = 0x12 // wiresafe.go, item_ops.go
)

// Party, both directions (internal/gameserver/network/serverpackets/party.go
// and clientpackets/party.go).
const (
	OpGameAskJoinParty              = 0x39
	OpGameJoinParty                 = 0x3a
	OpGamePartySmallWindowAll       = 0x4e
	OpGamePartySmallWindowAdd       = 0x4f
	OpGamePartySmallWindowDeleteAll = 0x50
	OpGamePartySmallWindowDelete    = 0x51
	OpGamePartySmallWindowUpdate    = 0x52
	OpGamePartyMemberPosition       = 0xa7

	OpGameRequestJoinParty       = 0x29
	OpGameRequestAnswerJoinParty = 0x2a
	OpGameRequestWithdrawParty   = 0x2b
	OpGameRequestOustPartyMember = 0x2c
)

// Party matching, both directions (internal/gameserver/network/serverpackets/
// partymatch.go and clientpackets/partymatch.go). The uint16 server
// sub-opcodes follow OpGameExtended, the client ones OpGameClientExtended.
const (
	OpGamePartyMatchList          = 0x96
	OpGamePartyMatchDetail        = 0x97
	OpGameExClosePartyRoom uint16 = 0x000f

	OpGameRequestListPartyWaiting                    = 0x6f
	OpGameRequestManagePartyRoom                     = 0x70
	OpGameRequestJoinPartyRoom                       = 0x71
	OpGameRequestWithdrawPartyRoom            uint16 = 0x0003
	OpGameRequestExitPartyMatchingWaitingRoom uint16 = 0x0017
)

// Clan, both directions (internal/gameserver/network/serverpackets/pledge.go,
// pledgememberlist.go and clientpackets/pledge.go).
const (
	OpGameAskJoinPledge                 = 0x32
	OpGameJoinPledge                    = 0x33
	OpGamePledgeShowMemberListAll       = 0x53
	OpGamePledgeShowMemberListDeleteAll = 0x82

	OpGameRequestJoinPledge       = 0x24
	OpGameRequestAnswerJoinPledge = 0x25
)

// Clan warehouse, both directions: WarehouseDepositList and
// WarehouseWithdrawList (serverpackets/warehouse.go),
// SendWareHouseDepositList and SendWareHouseWithDrawList
// (clientpackets/warehouse.go; network/state.go).
const (
	OpGameWarehouseDepositList  = 0x41
	OpGameWarehouseWithdrawList = 0x42

	OpGameSendWareHouseDepositList  = 0x31
	OpGameSendWareHouseWithdrawList = 0x32
)

// Recipe book and crafting, both directions: RecipeBookItemList and
// RecipeItemMakeInfo (serverpackets/recipe.go), RequestRecipeBookOpen and
// RequestRecipeItemMakeSelf (clientpackets/recipe.go).
const (
	OpGameRecipeBookItemList = 0xd6
	OpGameRecipeItemMakeInfo = 0xd7

	OpGameRequestRecipeBookOpen     = 0xac
	OpGameRequestRecipeItemMakeSelf = 0xaf
)

// RequestCrystallizeItem (clientpackets/item_ops.go): the crafter breaks an
// item into crystals.
const OpGameRequestCrystallizeItem = 0x72

// Dye symbols, both directions: HennaEquipList (serverpackets/henna.go),
// HennaInfo (serverpackets/hennainfo.go), and RequestHennaEquip and
// RequestHennaUnequip (clientpackets/henna.go). The draw window opens from
// the Symbol Maker's Draw link (a bypass), so RequestHennaItemList is not
// sent.
const (
	OpGameHennaEquipList = 0xe2
	OpGameHennaInfo      = 0xe4

	OpGameRequestHennaEquip   = 0xbc
	OpGameRequestHennaUnequip = 0xbf
)

// Enchanting, both directions: ChooseInventoryItem and EnchantResult
// (serverpackets/enchant.go), and RequestEnchantItem
// (clientpackets/wiresafe.go, item_ops.go). The scroll itself goes out as a
// UseItem.
const (
	OpGameChooseInventoryItem = 0x6f
	OpGameEnchantResult       = 0x81

	OpGameRequestEnchantItem = 0x58
)

// Questions and slash commands, both directions: ConfirmDlg
// (serverpackets/confirmdlg.go), DlgAnswer (clientpackets/wiresafe.go,
// dlganswer.go) and RequestUserCommand (clientpackets/usercommand.go).
const (
	OpGameConfirmDlg = 0xed

	OpGameDlgAnswer          = 0xc5
	OpGameRequestUserCommand = 0xaa
)

// Player trade, both directions (serverpackets/shop_trade.go,
// clientpackets/wiresafe.go and shop_trade.go). TradeUpdate and
// TradeItemUpdate share opcode 0x74.
const (
	OpGameTradeStart        = 0x1e
	OpGameTradeOwnAdd       = 0x20
	OpGameTradeOtherAdd     = 0x21
	OpGameSendTradeDone     = 0x22
	OpGameSendTradeRequest  = 0x5e
	OpGameTradeItemUpdate   = 0x74
	OpGameTradePressOwnOk   = 0x75
	OpGameTradePressOtherOk = 0x7c

	OpGameTradeRequest       = 0x15
	OpGameAddTradeItem       = 0x16
	OpGameTradeDone          = 0x17
	OpGameAnswerTradeRequest = 0x44
)

// Items, both directions: ItemList and InventoryUpdate
// (internal/gameserver/network/serverpackets/itemlist.go,
// inventoryupdate.go), the extended ExAutoSoulShot (exautosoulshot.go,
// exsendmanorlist.go), UseItem (clientpackets/useitem.go) and the extended
// RequestAutoSoulShot (clientpackets/item_ops.go, extended.go).
const (
	OpGameItemList        = 0x1b
	OpGameInventoryUpdate = 0x27
	// OpGameExtended leads a server packet whose uint16 sub-opcode follows.
	OpGameExtended              = 0xfe
	OpGameExAutoSoulShot uint16 = 0x0012

	OpGameUseItem = 0x14
	// OpGameClientExtended leads a client packet whose uint16 sub-opcode
	// follows.
	OpGameClientExtended             = 0xd0
	OpGameRequestAutoSoulShot uint16 = 0x0005
	// OpGameRequestChangePartyLeader follows OpGameClientExtended
	// (clientpackets/party.go).
	OpGameRequestChangePartyLeader uint16 = 0x0004
)

// Dialogs and the class trainer, both directions
// (serverpackets/npchtmlmessage.go and acquireskill.go,
// clientpackets/wiresafe.go).
const (
	OpGameNpcHtmlMessage   = 0x0f
	OpGameAcquireSkillList = 0x8a
	OpGameAcquireSkillInfo = 0x8b
	OpGameAcquireSkillDone = 0x8e

	OpGameRequestBypassToServer   = 0x21
	OpGameRequestAcquireSkillInfo = 0x6b
	OpGameRequestAcquireSkill     = 0x6c
)

// Merchants, both directions: SellList and BuyList
// (serverpackets/shop_trade.go), RequestSellItem and RequestBuyItem
// (clientpackets/wiresafe.go, shop_trade.go).
const (
	OpGameSellList = 0x10
	OpGameBuyList  = 0x11

	OpGameRequestSellItem = 0x1e
	OpGameRequestBuyItem  = 0x1f
)

// Multisell, both directions: MultiSellList (serverpackets/multisell.go)
// and MultiSellChoose (clientpackets/multisell.go). MultiSellList shares
// its byte with OpGameClientExtended, the lead of a client packet, and
// MultiSellChoose its byte with OpGamePartyMemberPosition, a server packet.
const (
	OpGameMultiSellList   = 0xd0
	OpGameMultiSellChoose = 0xa7
)

// Private sell store, both directions: PrivateStoreManageListSell,
// PrivateStoreListSell and PrivateStoreMsgSell
// (serverpackets/privatestore.go), RequestPrivateStoreManageSell,
// SetPrivateStoreListSell, RequestPrivateStoreQuitSell,
// SetPrivateStoreMsgSell and RequestPrivateStoreBuy
// (clientpackets/privatestore.go).
const (
	OpGamePrivateStoreManageListSell = 0x9a
	OpGamePrivateStoreListSell       = 0x9b
	OpGamePrivateStoreMsgSell        = 0x9c

	OpGameRequestPrivateStoreManageSell = 0x73
	OpGameSetPrivateStoreListSell       = 0x74
	OpGameRequestPrivateStoreQuitSell   = 0x76
	OpGameSetPrivateStoreMsgSell        = 0x77
	OpGameRequestPrivateStoreBuy        = 0x79
)

// The Olympiad match packets (serverpackets/olympiad_match.go), extended
// (0xfe): a competitor's match window and its mode. ExOlympiadSpelledInfo
// (0x2a) goes to spectators only, and the bot never watches.
const (
	OpGameExOlympiadUserInfo uint16 = 0x0029
	OpGameExOlympiadMode     uint16 = 0x002b
	OpGameExOlympiadMatchEnd uint16 = 0x002c
)

// Duels and the cast cancel, both directions: the extended (0xfe) duel
// window packets (serverpackets/duel.go), MagicSkillCanceled
// (serverpackets/magic_skill.go), the extended (0xd0) duel requests
// (clientpackets/duel.go) and RequestTargetCancel
// (clientpackets/targetaction.go, wiresafe.go). RequestDuelSurrender is
// never sent (D39).
const (
	OpGameExDuelAskStart       uint16 = 0x004b
	OpGameExDuelReady          uint16 = 0x004c
	OpGameExDuelStart          uint16 = 0x004d
	OpGameExDuelEnd            uint16 = 0x004e
	OpGameExDuelUpdateUserInfo uint16 = 0x004f

	OpGameMagicSkillCanceled = 0x49

	OpGameRequestDuelStart       uint16 = 0x0027
	OpGameRequestDuelAnswerStart uint16 = 0x0028

	OpGameRequestTargetCancel = 0x37
)

// OpGameRequestDestroyItem destroys an item (clientpackets/wiresafe.go).
// Only the e2e harness sends it (EncodeRequestDestroyItem): a
// bot never destroys what it carries.
const OpGameRequestDestroyItem = 0x59

// Login server to client (internal/loginserver/network/serverpackets).
const (
	OpLoginInit          = 0x00 // init.go
	OpLoginFail          = 0x01 // loginfail.go
	OpLoginAccountKicked = 0x02 // accountkicked.go
	OpLoginOk            = 0x03 // loginok.go
	OpLoginServerList    = 0x04 // serverlist.go
	OpLoginPlayFail      = 0x06 // playfail.go
	OpLoginPlayOk        = 0x07 // playok.go
	OpLoginGGAuth        = 0x0b // ggauth.go
)

// Client to login server (internal/loginserver/network/clientpackets).
const (
	OpLoginRequestAuthLogin   = 0x00 // requestauthlogin.go
	OpLoginRequestServerLogin = 0x02 // requestserverlogin.go
	OpLoginRequestServerList  = 0x05 // requestserverlist.go
	OpLoginAuthGameGuard      = 0x07 // authgameguard.go
)
