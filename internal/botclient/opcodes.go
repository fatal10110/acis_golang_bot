package botclient

// Opcodes of the packets this package reads and sends, redeclared from the
// acis_golang server's packet packages (the file each value comes from is
// named in its group). Login and game opcodes overlap, so each constant
// names its protocol and direction.

// Login server to client (internal/loginserver/network/serverpackets).
const (
	opLoginInit          = 0x00 // init.go
	opLoginFail          = 0x01 // loginfail.go
	opLoginAccountKicked = 0x02 // accountkicked.go
	opLoginOk            = 0x03 // loginok.go
	opLoginServerList    = 0x04 // serverlist.go
	opLoginPlayFail      = 0x06 // playfail.go
	opLoginPlayOk        = 0x07 // playok.go
	opLoginGGAuth        = 0x0b // ggauth.go
)

// Client to login server (internal/loginserver/network/clientpackets).
const (
	opLoginRequestAuthLogin   = 0x00 // requestauthlogin.go
	opLoginRequestServerLogin = 0x02 // requestserverlogin.go
	opLoginRequestServerList  = 0x05 // requestserverlist.go
	opLoginAuthGameGuard      = 0x07 // authgameguard.go
)

// Game server to client (internal/gameserver/network/serverpackets).
const (
	opGameVersionCheck       = 0x00 // versioncheck.go
	opGameMoveToLocation     = 0x01 // movetolocation.go
	opGameUserInfo           = 0x04 // userinfo.go
	opGameDeleteObject       = 0x12 // deleteobject.go
	opGameCharSelectInfo     = 0x13 // charselectinfo.go
	opGameAuthLoginFail      = 0x14 // authloginfail.go
	opGameCharSelected       = 0x15 // charselected.go
	opGameCharCreateOk       = 0x19 // charcreateok.go
	opGameCharCreateFail     = 0x1a // charcreatefail.go
	opGameActionFailed       = 0x25 // actionfailed.go
	opGameServerClose        = 0x26 // serverclose.go
	opGameTeleportToLocation = 0x28 // teleporttolocation.go
	opGameStopMove           = 0x47 // movement_control.go
	opGameValidateLocation   = 0x61 // movement_control.go
	opGameLeaveWorld         = 0x7e // lifecycle.go
)

// Game server to client, in-world state (internal/gameserver/network/serverpackets).
const (
	opGameCharInfo         = 0x03 // charinfo.go
	opGameDie              = 0x06 // die.go
	opGameRevive           = 0x07 // lifecycle.go
	opGameStatusUpdate     = 0x0e // targeting.go
	opGameNPCInfo          = 0x16 // npcinfo.go
	opGameTargetUnselected = 0x2a // targeting.go
	opGameAutoAttackStart  = 0x2b // stance_social.go
	opGameAutoAttackStop   = 0x2c // autoattack.go
	opGameChangeWaitType   = 0x2f // stance_social.go
	opGameCreatureSay      = 0x4a // creaturesay.go
	opGameMoveToPawn       = 0x60 // movetopawn.go
	opGameSystemMessage    = 0x64 // systemmessage.go
	opGameMyTargetSelected = 0xa6 // targeting.go
)

// Client to game server (internal/gameserver/network/clientpackets).
const (
	opGameProtocolVersion        = 0x00 // protocolversion.go
	opGameEnterWorld             = 0x03 // enterworld.go
	opGameAuthLogin              = 0x08 // authlogin.go
	opGameLogout                 = 0x09 // lifecycle.go
	opGameRequestCharacterCreate = 0x0b // requestcharactercreate.go
	opGameRequestGameStart       = 0x0d // requestgamestart.go
)

// Client to game server, sent by the brain in the world
// (internal/gameserver/network/clientpackets).
const (
	opGameMoveBackwardToLocation = 0x01 // movebackwardtolocation.go
	opGameAction                 = 0x04 // wiresafe.go, targetaction.go
	opGameRequestSocialAction    = 0x1b // wiresafe.go, stance_social.go
	opGameAppearing              = 0x30 // wiresafe.go, appearing.go
	opGameSay2                   = 0x38 // say2.go
	opGameRequestActionUse       = 0x45 // wiresafe.go, requestactionuse.go
	opGameValidatePosition       = 0x48 // validateposition.go
	opGameRequestRestartPoint    = 0x6d // requestrestartpoint.go
)

// Game server to client, combat and skills
// (internal/gameserver/network/serverpackets).
const (
	opGameAttack             = 0x05 // attack.go
	opGameMagicSkillUse      = 0x48 // magic_skill.go
	opGameSkillList          = 0x58 // skilllist.go
	opGameMagicSkillLaunched = 0x76 // magic_skill.go
	opGameSkillCoolTime      = 0xc1 // skillcooltime.go
	opGameRelationChanged    = 0xce // relationchanged.go
)

// Client to game server, combat (internal/gameserver/network/clientpackets).
const (
	opGameAttackRequest        = 0x0a // wiresafe.go, targetaction.go
	opGameRequestMagicSkillUse = 0x2f // wiresafe.go, magic_skill_use.go
)

// Party, both directions (internal/gameserver/network/serverpackets/party.go
// and clientpackets/party.go).
const (
	opGameAskJoinParty              = 0x39
	opGameJoinParty                 = 0x3a
	opGamePartySmallWindowAll       = 0x4e
	opGamePartySmallWindowAdd       = 0x4f
	opGamePartySmallWindowDeleteAll = 0x50
	opGamePartySmallWindowDelete    = 0x51
	opGamePartySmallWindowUpdate    = 0x52
	opGamePartyMemberPosition       = 0xa7

	opGameRequestJoinParty       = 0x29
	opGameRequestAnswerJoinParty = 0x2a
	opGameRequestWithdrawParty   = 0x2b
)

// Items, both directions: ItemList and InventoryUpdate
// (internal/gameserver/network/serverpackets/itemlist.go,
// inventoryupdate.go), the extended ExAutoSoulShot (exautosoulshot.go,
// exsendmanorlist.go), UseItem (clientpackets/useitem.go) and the extended
// RequestAutoSoulShot (clientpackets/item_ops.go, extended.go).
const (
	opGameItemList        = 0x1b
	opGameInventoryUpdate = 0x27
	// opGameExtended leads a server packet whose uint16 sub-opcode follows.
	opGameExtended              = 0xfe
	opGameExAutoSoulShot uint16 = 0x0012

	opGameUseItem = 0x14
	// opGameClientExtended leads a client packet whose uint16 sub-opcode
	// follows.
	opGameClientExtended             = 0xd0
	opGameRequestAutoSoulShot uint16 = 0x0005
)

// Dialogs and the class trainer, both directions
// (serverpackets/npchtmlmessage.go and acquireskill.go,
// clientpackets/wiresafe.go).
const (
	opGameNpcHtmlMessage   = 0x0f
	opGameAcquireSkillList = 0x8a
	opGameAcquireSkillInfo = 0x8b
	opGameAcquireSkillDone = 0x8e

	opGameRequestBypassToServer   = 0x21
	opGameRequestAcquireSkillInfo = 0x6b
	opGameRequestAcquireSkill     = 0x6c
)
