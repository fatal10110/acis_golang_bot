package botclient

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Forwards to the wire package (docs/plan.md D46): the events, their
// constants, the login and handshake types and the opcodes are declared in
// botwire; these names keep
// botclient's brain, encoders and tests reading them as before.

// The events and the types they carry.
type (
	ExOpenMPCC                     = botwire.ExOpenMPCC
	ExCloseMPCC                    = botwire.ExCloseMPCC
	ExMPCCPartyInfoUpdate          = botwire.ExMPCCPartyInfoUpdate
	ExMPCCShowPartyMemberInfo      = botwire.ExMPCCShowPartyMemberInfo
	ChannelMember                  = botwire.ChannelMember
	ExMultiPartyCommandChannelInfo = botwire.ExMultiPartyCommandChannelInfo
	ChannelPartyRow                = botwire.ChannelPartyRow
	AskJoinPledge                  = botwire.AskJoinPledge
	JoinPledge                     = botwire.JoinPledge
	PledgeShowMemberListAll        = botwire.PledgeShowMemberListAll
	PledgeShowMemberListDeleteAll  = botwire.PledgeShowMemberListDeleteAll
	AttackHit                      = botwire.AttackHit
	Attack                         = botwire.Attack
	MagicSkillUse                  = botwire.MagicSkillUse
	MagicSkillLaunched             = botwire.MagicSkillLaunched
	KnownSkill                     = botwire.KnownSkill
	SkillList                      = botwire.SkillList
	SkillTimer                     = botwire.SkillTimer
	SkillCoolTime                  = botwire.SkillCoolTime
	RelationChanged                = botwire.RelationChanged
	ConfirmDlg                     = botwire.ConfirmDlg
	EtcStatusUpdate                = botwire.EtcStatusUpdate
	NpcHtmlMessage                 = botwire.NpcHtmlMessage
	DoorInfo                       = botwire.DoorInfo
	DoorStatusUpdate               = botwire.DoorStatusUpdate
	ChooseInventoryItem            = botwire.ChooseInventoryItem
	EnchantResult                  = botwire.EnchantResult
	EnchantCode                    = botwire.EnchantCode
	Event                          = botwire.Event
	Point                          = botwire.Point
	MoveToLocation                 = botwire.MoveToLocation
	StopMove                       = botwire.StopMove
	ValidateLocation               = botwire.ValidateLocation
	TeleportToLocation             = botwire.TeleportToLocation
	DeleteObject                   = botwire.DeleteObject
	LeaveWorld                     = botwire.LeaveWorld
	ServerClose                    = botwire.ServerClose
	UserInfo                       = botwire.UserInfo
	StatusFields                   = botwire.StatusFields
	StatusUpdate                   = botwire.StatusUpdate
	CharInfo                       = botwire.CharInfo
	NPCInfo                        = botwire.NPCInfo
	MoveToPawn                     = botwire.MoveToPawn
	RestartPoints                  = botwire.RestartPoints
	Die                            = botwire.Die
	Revive                         = botwire.Revive
	MyTargetSelected               = botwire.MyTargetSelected
	TargetUnselected               = botwire.TargetUnselected
	SystemMessage                  = botwire.SystemMessage
	ActionFailed                   = botwire.ActionFailed
	CreatureSay                    = botwire.CreatureSay
	WaitType                       = botwire.WaitType
	ChangeWaitType                 = botwire.ChangeWaitType
	AutoAttackStart                = botwire.AutoAttackStart
	AutoAttackStop                 = botwire.AutoAttackStop
	ExDuelAskStart                 = botwire.ExDuelAskStart
	ExDuelReady                    = botwire.ExDuelReady
	ExDuelStart                    = botwire.ExDuelStart
	ExDuelEnd                      = botwire.ExDuelEnd
	ExDuelUpdateUserInfo           = botwire.ExDuelUpdateUserInfo
	ExOlympiadMode                 = botwire.ExOlympiadMode
	ExOlympiadMatchEnd             = botwire.ExOlympiadMatchEnd
	ExOlympiadUserInfo             = botwire.ExOlympiadUserInfo
	MagicSkillCanceled             = botwire.MagicSkillCanceled
	HennaRow                       = botwire.HennaRow
	HennaEquipList                 = botwire.HennaEquipList
	WornSymbol                     = botwire.WornSymbol
	HennaInfo                      = botwire.HennaInfo
	Item                           = botwire.Item
	ItemList                       = botwire.ItemList
	ItemState                      = botwire.ItemState
	ItemChange                     = botwire.ItemChange
	InventoryUpdate                = botwire.InventoryUpdate
	ExAutoSoulShot                 = botwire.ExAutoSoulShot
	GroundItem                     = botwire.GroundItem
	SpawnItem                      = botwire.SpawnItem
	DropItem                       = botwire.DropItem
	GetItem                        = botwire.GetItem
	AskJoinParty                   = botwire.AskJoinParty
	JoinParty                      = botwire.JoinParty
	PartyMember                    = botwire.PartyMember
	PartySmallWindowAll            = botwire.PartySmallWindowAll
	PartySmallWindowAdd            = botwire.PartySmallWindowAdd
	PartySmallWindowDelete         = botwire.PartySmallWindowDelete
	PartySmallWindowDeleteAll      = botwire.PartySmallWindowDeleteAll
	PartySmallWindowUpdate         = botwire.PartySmallWindowUpdate
	PartyMemberAt                  = botwire.PartyMemberAt
	PartyMemberPosition            = botwire.PartyMemberPosition
	PartyRoom                      = botwire.PartyRoom
	PartyMatchList                 = botwire.PartyMatchList
	PartyMatchDetail               = botwire.PartyMatchDetail
	ExClosePartyRoom               = botwire.ExClosePartyRoom
	RecipeBookItemList             = botwire.RecipeBookItemList
	RecipeItemMakeInfo             = botwire.RecipeItemMakeInfo
	ShopItem                       = botwire.ShopItem
	BuyList                        = botwire.BuyList
	SellList                       = botwire.SellList
	MultiSellItem                  = botwire.MultiSellItem
	MultiSellEntry                 = botwire.MultiSellEntry
	MultiSellList                  = botwire.MultiSellList
	StoreItem                      = botwire.StoreItem
	PrivateStoreManageListSell     = botwire.PrivateStoreManageListSell
	PrivateStoreListSell           = botwire.PrivateStoreListSell
	PrivateStoreMsgSell            = botwire.PrivateStoreMsgSell
	Effect                         = botwire.Effect
	AbnormalStatusUpdate           = botwire.AbnormalStatusUpdate
	PartySpelled                   = botwire.PartySpelled
	SendTradeRequest               = botwire.SendTradeRequest
	TradeItem                      = botwire.TradeItem
	TradeStart                     = botwire.TradeStart
	TradeOwnAdd                    = botwire.TradeOwnAdd
	TradeOtherAdd                  = botwire.TradeOtherAdd
	TradeAvail                     = botwire.TradeAvail
	TradeItemUpdate                = botwire.TradeItemUpdate
	TradePressOwnOk                = botwire.TradePressOwnOk
	TradePressOtherOk              = botwire.TradePressOtherOk
	SendTradeDone                  = botwire.SendTradeDone
	AcquirableSkill                = botwire.AcquirableSkill
	AcquireSkillList               = botwire.AcquireSkillList
	SkillRequirement               = botwire.SkillRequirement
	AcquireSkillInfo               = botwire.AcquireSkillInfo
	AcquireSkillDone               = botwire.AcquireSkillDone
	WarehouseItem                  = botwire.WarehouseItem
	WarehouseDepositList           = botwire.WarehouseDepositList
	WarehouseWithdrawList          = botwire.WarehouseWithdrawList
)

// The constants typed by the events' types.
const (
	AbnormalStealth           = botwire.AbnormalStealth
	RecipeMadeNone            = botwire.RecipeMadeNone
	RecipeMadeFailed          = botwire.RecipeMadeFailed
	RecipeMadeSuccess         = botwire.RecipeMadeSuccess
	HitSoulshot               = botwire.HitSoulshot
	HitCritical               = botwire.HitCritical
	HitShield                 = botwire.HitShield
	HitMiss                   = botwire.HitMiss
	EnchantSuccess            = botwire.EnchantSuccess
	EnchantBrokenWithCrystals = botwire.EnchantBrokenWithCrystals
	EnchantCancelled          = botwire.EnchantCancelled
	EnchantUnsuccess          = botwire.EnchantUnsuccess
	EnchantBrokenNoCrystals   = botwire.EnchantBrokenNoCrystals
	StatusHP                  = botwire.StatusHP
	StatusMaxHP               = botwire.StatusMaxHP
	StatusMP                  = botwire.StatusMP
	StatusMaxMP               = botwire.StatusMaxMP
	StatusCP                  = botwire.StatusCP
	StatusMaxCP               = botwire.StatusMaxCP
	StatusSP                  = botwire.StatusSP
	StatusLoad                = botwire.StatusLoad
	StatusMaxLoad             = botwire.StatusMaxLoad
	StatusKarma               = botwire.StatusKarma
	WaitSitting               = botwire.WaitSitting
	WaitStanding              = botwire.WaitStanding
	WaitFakeDeathStart        = botwire.WaitFakeDeathStart
	WaitFakeDeathStop         = botwire.WaitFakeDeathStop
	ItemAdded                 = botwire.ItemAdded
	ItemModified              = botwire.ItemModified
	ItemRemoved               = botwire.ItemRemoved
	classMaster               = botwire.ClassMaster
)

// The game opcodes, server to client and client to server.
const (
	opGameVersionCheck              = botwire.OpGameVersionCheck
	opGameMoveToLocation            = botwire.OpGameMoveToLocation
	opGameUserInfo                  = botwire.OpGameUserInfo
	opGameCharSelectInfo            = botwire.OpGameCharSelectInfo
	opGameAuthLoginFail             = botwire.OpGameAuthLoginFail
	opGameCharSelected              = botwire.OpGameCharSelected
	opGameCharCreateOk              = botwire.OpGameCharCreateOk
	opGameCharCreateFail            = botwire.OpGameCharCreateFail
	opGameActionFailed              = botwire.OpGameActionFailed
	opGameServerClose               = botwire.OpGameServerClose
	opGameTeleportToLocation        = botwire.OpGameTeleportToLocation
	opGameStopMove                  = botwire.OpGameStopMove
	opGameLeaveWorld                = botwire.OpGameLeaveWorld
	opGameCharInfo                  = botwire.OpGameCharInfo
	opGameDie                       = botwire.OpGameDie
	opGameRevive                    = botwire.OpGameRevive
	opGameNPCInfo                   = botwire.OpGameNPCInfo
	opGameChangeWaitType            = botwire.OpGameChangeWaitType
	opGameProtocolVersion           = botwire.OpGameProtocolVersion
	opGameAuthLogin                 = botwire.OpGameAuthLogin
	opGameLogout                    = botwire.OpGameLogout
	opGameRequestCharacterCreate    = botwire.OpGameRequestCharacterCreate
	opGameRequestGameStart          = botwire.OpGameRequestGameStart
	opGameMoveBackwardToLocation    = botwire.OpGameMoveBackwardToLocation
	opGameAction                    = botwire.OpGameAction
	opGameRequestItemList           = botwire.OpGameRequestItemList
	opGameRequestSocialAction       = botwire.OpGameRequestSocialAction
	opGameAppearing                 = botwire.OpGameAppearing
	opGameSay2                      = botwire.OpGameSay2
	opGameValidatePosition          = botwire.OpGameValidatePosition
	opGameRequestRestartPoint       = botwire.OpGameRequestRestartPoint
	opGameAttackRequest             = botwire.OpGameAttackRequest
	opGameRequestMagicSkillUse      = botwire.OpGameRequestMagicSkillUse
	opGameRequestDropItem           = botwire.OpGameRequestDropItem
	opGameRequestJoinParty          = botwire.OpGameRequestJoinParty
	opGameRequestAnswerJoinParty    = botwire.OpGameRequestAnswerJoinParty
	opGameRequestWithdrawParty      = botwire.OpGameRequestWithdrawParty
	opGameRequestOustPartyMember    = botwire.OpGameRequestOustPartyMember
	opGameRequestJoinPartyRoom      = botwire.OpGameRequestJoinPartyRoom
	opGameRequestAnswerJoinPledge   = botwire.OpGameRequestAnswerJoinPledge
	opGameSendWareHouseDepositList  = botwire.OpGameSendWareHouseDepositList
	opGameSendWareHouseWithdrawList = botwire.OpGameSendWareHouseWithdrawList
	opGameRequestHennaEquip         = botwire.OpGameRequestHennaEquip
	opGameRequestHennaUnequip       = botwire.OpGameRequestHennaUnequip
	opGameRequestEnchantItem        = botwire.OpGameRequestEnchantItem
	opGameRequestUserCommand        = botwire.OpGameRequestUserCommand
	opGameAddTradeItem              = botwire.OpGameAddTradeItem
	opGameTradeDone                 = botwire.OpGameTradeDone
	opGameAnswerTradeRequest        = botwire.OpGameAnswerTradeRequest
	opGameExtended                  = botwire.OpGameExtended
	opGameExAutoSoulShot            = botwire.OpGameExAutoSoulShot
	opGameUseItem                   = botwire.OpGameUseItem
	opGameClientExtended            = botwire.OpGameClientExtended
	opGameRequestAutoSoulShot       = botwire.OpGameRequestAutoSoulShot
	opGameRequestAcquireSkillInfo   = botwire.OpGameRequestAcquireSkillInfo
	opGameRequestAcquireSkill       = botwire.OpGameRequestAcquireSkill
	opGameRequestSellItem           = botwire.OpGameRequestSellItem
	opGameRequestBuyItem            = botwire.OpGameRequestBuyItem
	opGameRequestPrivateStoreBuy    = botwire.OpGameRequestPrivateStoreBuy
)

// decodeEvent is botwire.DecodeEvent.
func decodeEvent(raw []byte) (ev Event, ok bool, err error) { return botwire.DecodeEvent(raw) }

// The client packets' row types and the values they carry.
type (
	buyRow       = botwire.BuyRow
	sellRow      = botwire.SellRow
	storeRow     = botwire.StoreRow
	warehouseRow = botwire.WarehouseRow
)

const (
	actionSitStand = botwire.ActionSitStand
	actionWalkRun  = botwire.ActionWalkRun
	chatAll        = botwire.ChatAll
	chatTell       = botwire.ChatTell
	restartVillage = botwire.RestartVillage
)

// The login and handshake types: the session API and cmd/bots name them.
type (
	SessionKeys   = botwire.SessionKeys
	ServerEntry   = botwire.ServerEntry
	Self          = botwire.Self
	StartingClass = botwire.StartingClass
	Appearance    = botwire.Appearance
	charSlot      = botwire.CharSlot
)

// The races and sexes RequestCharacterCreate carries.
const (
	RaceHuman   = botwire.RaceHuman
	RaceElf     = botwire.RaceElf
	RaceDarkElf = botwire.RaceDarkElf
	RaceOrc     = botwire.RaceOrc
	RaceDwarf   = botwire.RaceDwarf
	SexMale     = botwire.SexMale
	SexFemale   = botwire.SexFemale
)

// The starting classes.
var (
	HumanFighter   = botwire.HumanFighter
	HumanMystic    = botwire.HumanMystic
	ElvenFighter   = botwire.ElvenFighter
	ElvenMystic    = botwire.ElvenMystic
	DarkFighter    = botwire.DarkFighter
	DarkMystic     = botwire.DarkMystic
	OrcFighter     = botwire.OrcFighter
	OrcMystic      = botwire.OrcMystic
	DwarvenFighter = botwire.DwarvenFighter
)

// StartingClasses is botwire.StartingClasses.
func StartingClasses() []StartingClass { return botwire.StartingClasses() }

// RandomAppearance is botwire.RandomAppearance.
func RandomAppearance(r *rand.Rand) Appearance { return botwire.RandomAppearance(r) }

// The login opcodes the login client and its errors name.
const (
	opLoginInit          = botwire.OpLoginInit
	opLoginFail          = botwire.OpLoginFail
	opLoginAccountKicked = botwire.OpLoginAccountKicked
	opLoginOk            = botwire.OpLoginOk
	opLoginServerList    = botwire.OpLoginServerList
	opLoginPlayFail      = botwire.OpLoginPlayFail
	opLoginPlayOk        = botwire.OpLoginPlayOk
	opLoginGGAuth        = botwire.OpLoginGGAuth
)
