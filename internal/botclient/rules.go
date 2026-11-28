package botclient

import (
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Persona is the rule set a brain plays: its role, the thresholds of the
// shared rules and hooks for the persona's own choices. Every value is a
// configurable default, not a safety boundary (D6); the operational limits
// are in pacing.go. A nil hook does nothing.
//
// Overrides: the brain plays the build CompileBuild compiles for the
// persona's Role and the bot's class and level (Brain.build). The persona's
// tables and thresholds the build replaced (CriticalHP, CrowdedHP,
// OutmatchedHP, MPReserve, SitHP, SitMP, StandHP, StandMP, AttackSkills,
// Heals, Buffs, LearnSkills, Trainers, ClassChange.Builds, Dyes.Symbols and
// Gear.Classes) are its overrides: a non-nil table or a non-zero threshold
// replaces the build's, a negative threshold turns it off. Without a Role
// they are all the persona plays.
type Persona struct {
	// Role is the part the bot plays, which its build is compiled for;
	// RoleNone compiles none.
	Role Role
	// Store is how a merchant sells its loot in a private sell store (rule
	// "store"); while it opens one or keeps one open the bot is in store
	// mode, which replaces rules 3-10.
	Store StorePolicy
	// CriticalHP is the HP percent under which a bot in combat drinks the
	// first of Potions it carries whose reuse has run out, or else heals
	// itself with a ready heal of Heals, or else flees FleeDistance away
	// from its target (rule 3); 0 never does any.
	CriticalHP   int
	FleeDistance int32
	// CrowdedHP is the HP percent under which a bot in combat with two or
	// more attackers it may fight already drinks a potion as under
	// CriticalHP (rule 3), so it drinks before OutmatchedHP has it flee; it
	// heals itself and flees only under CriticalHP. 0 never drinks early.
	CrowdedHP int
	// FleeOutrun has the bot flee (rules 3 and 4) only while it outruns
	// every creature attacking it: one that runs as fast as it or faster
	// keeps hitting a bot that runs away, which hits nothing back, so it
	// fights on instead, drinking its potions (#281).
	FleeOutrun bool
	// Potions are the healing potion template ids rule 3 drinks, best
	// first; none drinks nothing. DefaultPersona's are a built-in list;
	// TravelMap.WithItemIndex replaces it with the datapack's.
	Potions []int32
	// AutoShots turns auto use on for the shots the wielded weapon charges
	// (rule "shots"): of its grade and the class's kind, soulshots, or
	// blessed spiritshots before plain ones for a mystic, the beginners'
	// ones first while carried; and off for any other shot the server has
	// on. Shots are shot templates the bot keeps besides: shots no weapon it
	// wields, wears next or falls back to charges, beginners' ones aside,
	// are junk.
	AutoShots bool
	Shots     []int32
	// OutmatchedHP is the HP percent under which a bot hit by two or more
	// attackers flees from the hardest hitter instead of fighting it (rule
	// 4); 0 always fights.
	OutmatchedHP int
	// AttackSkills are the skills the bot casts at its combat target before
	// melee, first ready one first: known (SkillList), enabled and out of
	// reuse; none while its MP is under MPReserve.
	AttackSkills []int32
	// MPReserve is the MP percent kept for heals and buffs: under it the
	// bot casts no AttackSkills. 0 keeps none.
	MPReserve int
	// Heals are the heals the bot casts on a party member, itself
	// included, whose HP is under the heal's percent, first ready one
	// first, so the strongest goes first (rule 3). The leader is healed
	// first, then the bot, then the others. Only known skills are cast.
	Heals []Heal
	// Buffs are the buffs the bot keeps on itself and on the party members
	// in sight, the leader first, renewed out of combat once missing or
	// with under RebuffBefore left (rule 7). Only known skills are cast.
	Buffs        []Buff
	RebuffBefore time.Duration
	// AssistMin and AssistMax bound the delay after the party leader's
	// attack or hostile cast on a creature the bot may fight before the
	// bot attacks it too (rule 9 assist); a bare selection by the leader
	// is no assist. AssistMax 0 never assists.
	AssistMin, AssistMax time.Duration
	// BlacklistAfter is how many attempts on a target with no swing or cast
	// landing in between make the bot leave it alone for BlacklistFor; 0
	// never does.
	BlacklistAfter int
	BlacklistFor   time.Duration
	PvP            PvPPolicy
	// SevenSigns is Seven Signs member mode (docs/pvp/cp-potions.md, D43):
	// the characters it names sign up with Dusk, farm the dungeons their
	// membership opens and keep their seal stones (sevensigns.go). No
	// preset names any.
	SevenSigns SevenSignsPolicy
	// SitHP and SitMP are the percents under which a bot out of combat sits
	// to rest; it stands once both reach StandHP and StandMP (rule 6). 0
	// never sits for that resource.
	SitHP, SitMP     int
	StandHP, StandMP int
	// IdleChance is the chance per decision (0 to 1) of an idle drift (rule
	// 10): a move within IdleRadius of where the bot arrived, a turn, an
	// emote from Emotes (the server plays ids 2-13) or a sit.
	IdleChance float64
	IdleRadius int32
	Emotes     []int32
	// Death decides for a dead bot (rule 1) once Recover lets it; a zero
	// Action waits. A restart it asks for goes out at most once per
	// restartRetry.
	Death func(w *World, now time.Time) Action
	// Recover is how the bot answers a resurrection offer, how long it
	// waits for one, and how it handles a dead or lost leader (rules
	// "dead" and "leader").
	Recover RecoverPolicy
	// Lead times a lead bot's invitations and a join bot's handover (rule
	// "party").
	Lead LeadPolicy
	// Rooms is how a partyless support bot volunteers for the
	// party-matching rooms players open (rule "rooms").
	Rooms RoomPolicy
	// Answer answers an open prompt (rule 2); a zero Action waits.
	Answer func(w *World, now time.Time) Action
	// JoinParty decides whether to accept the party invitation of the
	// player named requester (rule 2); nil declines every invitation.
	JoinParty func(w *World, requester string, now time.Time) bool
	// JoinClan decides whether to accept the invitation of the player named
	// requester into the clan named clan (rule 2); nil declines every
	// invitation, and an invitation whose inviter is unknown is declined.
	// AcceptClanFrom is the crafter's policy (D20).
	JoinClan func(w *World, requester, clan string, now time.Time) bool
	// Trade is how the bot answers trade requests and trades (rule 2).
	Trade TradePolicy
	// Chat is how the bot answers whispers (rule "chat").
	Chat ChatPolicy
	// LearnSkills are the skills the bot learns with SP at a trainer, by
	// class id, first listed first (rule 9 errand); a class with none never
	// visits one. A visit is due once per level and when the SP reaches the
	// cheapest wanted skill the last visit could not afford.
	LearnSkills map[int32][]int32
	// Trainers are the NPC template ids of the trainers of each class id;
	// the bot visits the nearest one in sight, else walks to the nearest
	// spawn of one within TrainerReach (from the travel map,
	// BrainConfig.Travel); 0 visits only one in sight.
	Trainers     map[int32][]int32
	TrainerReach int32
	// Spots is how the bot picks and keeps its farming spot when the brain
	// has a spot registry (BrainConfig.Spots, rule 9 errand "spot").
	Spots SpotPolicy
	// Follow is how the bot follows the leader of its party (rule
	// "follow").
	Follow FollowPolicy
	// Newbie is when the bot asks a Newbie Guide for the newbie buffs
	// (rule 9 errand "newbie").
	Newbie NewbiePolicy
	// ClassChange is when and to what the bot changes class at the class
	// master (rule 9 errand "classchange").
	ClassChange ClassChangePolicy
	// Olympiad is how an Olympiad bot (PvP.Olympiad) registers (rule 9
	// errand "olympiad entry").
	Olympiad OlympiadPolicy
	// Dyes is which dye symbols the bot wears and where it draws them
	// (rule 9 errand "dyes").
	Dyes DyePolicy
	// Enchant is how the bot enchants the gear it wears with the scrolls
	// it carries (rule "enchant").
	Enchant EnchantPolicy
	// Travel is how the bot travels by gatekeeper to a spot out of walking
	// reach when the brain has a travel map (BrainConfig.Travel, rule 9
	// errand "travel").
	Travel TravelPolicy
	// Town is when the bot takes a gatekeeper to another town between idle
	// spells (rule "travel"); the zero TownPolicy never does.
	Town TownPolicy
	// Shop is what the bot buys and sells at merchants (rule 9 errand
	// "shop").
	Shop ShopPolicy
	// Market is how the bot buys from other players' sell stores (rule 9
	// errand "market").
	Market MarketPolicy
	// Supply sizes the shots, HP potions and Scrolls of Escape the shop
	// errand buys from what the bot spends (the use meter).
	Supply SupplyPolicy
	// Gear is how the bot wears and buys better weapons and armor (rule 9
	// errand "gear", and the shop errand).
	Gear GearPolicy
	// Warehouse is the clan warehouse errand (rule 9 errand "warehouse").
	Warehouse WarehousePolicy
	// Valuables is how a merchant tells a valuable among its loot and
	// sells it (docs/valuables); the zero ValuablePolicy, every preset's
	// but the merchant's, finds none.
	Valuables ValuablePolicy
	// Recipes is recipe learning (rule 9 errand "recipe").
	Recipes RecipePolicy
	// Crafter is the clan crafter's cycle (rule "craft"); the zero
	// CrafterPolicy crafts nothing.
	Crafter CrafterPolicy
	// Loot is what the bot picks up after a fight and drops when
	// overweight (rule 8).
	Loot LootPolicy
	// Holds is the persona's keep hook (docs/items.md section 5), asked per
	// carried item: an item it keeps is never sold to a merchant (junk),
	// listed in the store (storeLoot) or offered in a trade (tradePrice),
	// and one it marks NoWear is never worn by the gear errand. nil keeps
	// nothing more. The clan crafter plan (M9) keeps its materials and the
	// graded gear it deposits for crystallizing with it.
	Holds func(w *World, it Item) Hold
	// ClanMember is the clan member option (docs/crafting.md section 7),
	// active while the bot is in a clan with a crafter on the board: the
	// keep hook keeps recipe scrolls, the materials of the clan crafters'
	// recipes and the graded gear it would not wear; a supply trip's
	// arrival in town visits the clan warehouse to withdraw the products
	// the board credits to it and deposit what it keeps with the ore of the
	// shot orders it posts; the extra supply hook buys that ore; and the
	// clan crafters are a shot source. NewBrain wraps the persona's hooks
	// for it (wireMember).
	ClanMember bool
	// Goal is the persona's goal (rule 9). It reports false when it has
	// nothing to do, which leaves the turn to idle drift.
	Goal func(w *World, now time.Time) (Action, bool)
}

// Hold is the persona's keep hook's answer for one carried item
// (Persona.Holds).
type Hold struct {
	// Keep keeps the item: never sold, listed or offered.
	Keep bool
	// NoWear keeps the gear errand from wearing it, so a piece kept for
	// crystallizing is deposited, not worn.
	NoWear bool
}

// DefaultPersona is a support player that rests when low, fights back when
// hit, takes a resurrection when killed or restarts in town after a while,
// sits by its dead leader, falls back to town when the leader is gone,
// learns the support skills of a mystic
// at its trainer, heals and buffs its party with them, assists its party
// leader, follows the leader of a party it joined and otherwise drifts
// around where it stands. It plays the Support role: its skills, trainers,
// heals, buffs, attack spells, professions, gear and dye tables and its
// thresholds are its build's (CompileBuild), which it overrides none of.
func DefaultPersona() Persona {
	return Persona{
		Role:           RoleSupport,
		Death:          restartToVillage,
		Recover:        defaultRecoverPolicy(),
		Lead:           defaultLeadPolicy(),
		Rooms:          defaultRoomPolicy(),
		JoinParty:      acceptParty,
		FleeOutrun:     true,
		FleeDistance:   600,
		BlacklistAfter: 4,
		BlacklistFor:   3 * time.Minute,
		PvP:            PvPPolicy{FightFlagged: true},
		Potions:        healingPotions,
		AutoShots:      true,
		IdleChance:     0.1,
		IdleRadius:     250,
		Emotes:         []int32{2, 6, 7, 10, 11}, // greet, yes, bow, laugh, applaud
		TrainerReach:   8000,
		Spots:          defaultSpotPolicy(),
		RebuffBefore:   time.Minute,
		AssistMin:      500 * time.Millisecond,
		AssistMax:      1500 * time.Millisecond,
		Follow:         defaultFollowPolicy(),
		Newbie:         defaultNewbiePolicy(),
		ClassChange:    defaultClassChangePolicy(),
		Olympiad:       defaultOlympiadPolicy(),
		Dyes:           defaultDyePolicy(),
		Enchant:        defaultEnchantPolicy(),
		Travel:         defaultTravelPolicy(),
		Shop:           defaultShopPolicy(),
		Market:         defaultMarketPolicy(),
		Supply:         defaultSupplyPolicy(),
		Gear:           defaultGearPolicy(),
		Warehouse:      defaultWarehousePolicy(),
		Loot:           defaultLootPolicy(),
		Trade:          defaultTradePolicy(),
		Store:          defaultStorePolicy(),
		Chat:           defaultChatPolicy(),
	}
}

// turnStep is the length of the short step that turns the character.
const turnStep = 40

// Action is one packet the brain may send. The zero Action sends nothing.
type Action struct {
	kind    actionKind
	payload []byte
	// object is the object an attack, a selection or a cast acts on, or
	// the item a gear UseItem puts on or takes off.
	object int32
	// item is the template id of the item a UseItem or RequestAutoSoulShot
	// acts on.
	item int32
	// skill is the skill a cast casts; opens marks a cast of an attack
	// step, which uses up the target's opener (Brain.opened).
	skill int32
	opens bool
	// walk marks a move that is one of the walker's segments (goTo), or
	// the walker's own /unstuck.
	walk bool
	// yes marks a duel answer that accepts the challenge, or a
	// challenge to a party duel.
	yes bool
	// interrupts marks a cast that answers an opponent's cast
	// (Brain.interruptCast), what bot_pvp_interrupts_total counts.
	interrupts bool
	// learn marks a recipe book open of the recipe errand (ruleRecipe),
	// the one its attempt waits on; the craft row's open is not marked.
	learn bool
}

type actionKind uint8

const (
	actHold actionKind = iota
	actMove
	actAttack
	actSitStand
	actEmote
	actSay
	actKeepalive
	actLogout
	actAppear
	actRestart
	actSelect
	actCast
	actAnswerParty
	actAnswerClan
	actLeaveParty
	actUseItem
	actAutoShot
	actAutoShotOff
	actTalk
	actBypass
	actSkillInfo
	actLearnSkill
	actWalkRun
	actBuyItem
	actSellItem
	actAnswerTrade
	actTradeAdd
	actTradeConfirm
	actTradeCancel
	// actTradeRequest asks a whisperer of a valuables round for a trade
	// (valuables_trade.go).
	actTradeRequest
	actTell
	actPickup
	actDrop
	actDlgAnswer
	actUserCommand
	actStoreManage
	actStoreTitle
	actStoreList
	actStoreQuit
	actChatReply
	actWarehouseDeposit
	actWarehouseWithdraw
	actRecipeBook
	actCraft
	// actCrystallize breaks an item into crystals (crystallize.go).
	actCrystallize
	actStoreBuy
	// actAnswer is a craft order's answer (a whisper or a clan chat line),
	// actAnnounce a finished GM order's clan chat line (craftorder.go).
	actAnswer
	actAnnounce
	// actHennaEquip draws a dye symbol, actHennaUnequip removes one
	// (dyes.go).
	actHennaEquip
	actHennaUnequip
	// actEnchant enchants an item with the selected scroll (enchant.go).
	actEnchant
	// actItemList asks for the inventory for the panel's inspect
	// (inspect.go).
	actItemList
	// actInviteParty invites a member, actOustParty expels one and
	// actChangeLeader hands the party over (partylead.go).
	actInviteParty
	actOustParty
	actChangeLeader
	// actRoomList lists the party-matching rooms, actRoomJoin asks to
	// enter one, actRoomExit leaves the waiting list and actRoomWithdraw a
	// room (partymatch.go).
	actRoomList
	actRoomJoin
	actRoomExit
	actRoomWithdraw
	// actTargetCancel is the Esc key the guards stop the bot's own cast
	// with (pvp_guards.go), actDuelAnswer the answer to a duel challenge
	// (Brain.declineDuel), actDuelStart the bot's challenge
	// (Brain.ruleChallenge).
	actTargetCancel
	actDuelAnswer
	actDuelStart
	// actChannelAsk asks for the bot's command channel: its overview or a
	// party's members (channel.go).
	actChannelAsk
)

// gate is the reuse gate the action's frame passes. Appearing,
// RequestRestartPoint, RequestAnswerJoinParty, RequestAnswerJoinPledge,
// RequestWithdrawParty, UseItem, RequestAutoSoulShot,
// RequestAcquireSkillInfo and RequestAcquireSkill have no reuse gate on the
// server: the frame bucket bounds them, restartRetry spaces a repeated
// restart, an invitation is answered once, potionReuse spaces the potions
// the bot drinks (the potions' shared heal over time), buffPotionWait a buff
// potion until its effect shows (then its stack stops it), shotRetry the toggles
// of a shot template, and a trainer visit sends one skill request per server
// answer. RequestBuyItem and RequestSellItem have none either: a shop visit
// sends one per BuyList or SellList it opened. RequestMagicSkillUse (attack,
// heal and buff casts) has none either: a cast waits for the one before it
// (castPending, castTimeout) and its skill's reuse, and buffRetry spaces one
// buff on one target. AnswerTradeRequest, AddTradeItem and TradeDone have no
// reuse gate either (no flood protector covers them): a request is answered
// once, and a trade gets one add per offered row, each after the server's
// answer to the one before (TradeOwnAdd), one confirm and one cancel
// (Trade.offer, added, confirmed, cancelAt). A pickup click (Action) has none: lootRetry spaces
// the clicks on one item. A whisper (Say2 Tell), a GM command's reply or a
// chat reply, has no server gate either (chat.go chatTell): it keeps the
// chat gate, as does a clan chat line (Say2 Clan: no server gate either,
// network/chat.go chatClan). DlgAnswer and RequestUserCommand have none either: an offer is
// answered once, and escapeRetry spaces a repeated /unstuck. The sell store
// packets (RequestPrivateStoreManageSell, SetPrivateStoreMsgSell,
// SetPrivateStoreListSell, RequestPrivateStoreQuitSell) have no flood
// protector either (network/privatestore_dispatch.go): an opening sends each
// once per answer it waits for, and StorePolicy.Retry spaces the openings.
// RequestPrivateStoreBuy has none either: a store visit sends one per
// PrivateStoreListSell it opened, and MarketPolicy.Cooldown spaces the
// visits to one store. RequestRecipeBookOpen has none either: the recipe errand sends one per
// page it waits for (recipeReplyTimeout). RequestHennaEquip and
// RequestHennaUnequip have none either (network/henna.go): the dye errand
// sends one per answer it waits for (dyeReplyTimeout), and a refused or
// unanswered one ends the visit for DyePolicy.RetryAfter. RequestEnchantItem
// and an enchant scroll's UseItem have none either (network/client_dispatch.go):
// the enchant rule sends one per answer it waits for (enchantReplyTimeout),
// and an unfinished exchange waits EnchantPolicy.RetryAfter.
// RequestItemList has none either (network/client_dispatch.go, no flood
// protector): the panel's inspect sends one per inspect at most, and the
// panel bounds the inspects (inspect.go).
func (a Action) gate() gate {
	switch a.kind {
	case actEmote:
		return gateEmote
	case actSay, actTell, actChatReply, actAnswer, actAnnounce:
		return gateChat
	case actBypass:
		return gateBypass
	case actDrop:
		return gateDrop
	case actCraft, actCrystallize:
		// RequestCrystallizeItem has no server gate (network/client_dispatch.go
		// registers it whenLive, no flood protector): the crafter sends one
		// per answer or crystallizeTimeout, and it keeps the craft gate.
		return gateCraft
	}
	return gateNone
}

func moveAction(to, from Point) Action {
	return Action{kind: actMove, payload: botwire.EncodeMoveBackwardToLocation(to, from)}
}

// attackAction clicks the selected object id, which attacks it.
func attackAction(id int32, from Point) Action {
	return Action{kind: actAttack, payload: botwire.EncodeAction(id, from), object: id}
}

func sitStandAction() Action {
	return Action{kind: actSitStand, payload: botwire.EncodeRequestActionUse(actionSitStand)}
}

func emoteAction(id int32) Action {
	return Action{kind: actEmote, payload: botwire.EncodeRequestSocialAction(id)}
}

// sayAction says text on the all channel.
func sayAction(text string) Action {
	return Action{kind: actSay, payload: botwire.EncodeSay2(text, chatAll)}
}

// restartToVillage is the death policy that restarts in the nearest town
// when the death window offers it, and otherwise waits.
func restartToVillage(w *World, _ time.Time) Action {
	if !w.Prompts.Restart.Points.Village {
		return Action{}
	}
	return Action{kind: actRestart, payload: botwire.EncodeRequestRestartPoint(restartVillage)}
}

// rule is one row of the decision table. run reports whether the rule
// matched; a matching rule ends the decision even when it sends nothing.
type rule struct {
	name string
	run  func(b *Brain, now time.Time) (Action, bool)
	// karma marks a rule a bot with karma skips (docs/pvp/karma.md "With
	// karma"): its duels (a karma player cannot duel) and arena trips, and
	// every errand that takes it to a town NPC or a gatekeeper, who refuse
	// it (folk_bypass.go, players.properties KarmaPlayerCanShop, CanUseGK).
	// Its hunts are ruleHunt's own gate.
	karma bool
	// stop gives back what the rule holds in a registry shared with other
	// bots (a spot, a store place, a party claim, a craft board entry) when
	// Run ends. Run calls it on every playRules row, karma or not.
	stop func(b *Brain)
}

// playRules are rules 1-10, first match wins. "heal" is rule 3's heal of
// the party, ahead of fighting so a support in combat still heals; "raise"
// casts Resurrection out of combat on a dead member of the bot's own bot
// party, right after "heal" so a living member's heal goes first;
// "assist" (the leader's target), "classchange" (the class master, ahead
// of the trainer so the new class learns its skills there), "train" (the
// trainer), "gear" (wearing
// better gear, ahead of the shop so a bought piece is worn before the next
// sale), "market" (other players' sell stores, ahead of the shop so a
// store's cheaper offer is taken first), "shop" (a merchant's buy and sell windows), "dyes" (drawing the
// dye symbols at a Symbol Maker, after the shop that buys their dyes), "warehouse" (the clan
// warehouse errand), "member" (settling a clan member's warehouse visit
// with the craft board; it never takes the turn), "recipe" (recipe learning), "craft" (the clan
// crafter's cycle, after the two errands that run its warehouse requests and
// its recipe scrolls; it holds the turn while a cycle runs), "follow" (the slot behind the party leader), "newbie" (the Newbie Guide's buffs), "travel"
// (a gatekeeper trip to a spot out of walking reach), "spot" (getting to the
// farming spot) and "farm" (attacking a monster there) are rule 9 errands,
// ahead of the persona's goal: a party member assists its leader, visits its
// trainer, then follows instead of farming. "loot" is rule 8, after the
// rebuff and ahead of every errand. "valuables" runs a merchant's
// look at its private warehouse and its valuables rounds, holding the turn
// while one runs (docs/valuables/round.md), after "loot" and ahead of
// "store". "store" is the merchant's private store:
// it opens where the bot stands, ahead of every errand that would walk it
// away, and puts the bot in store mode (storeRules). "command" sends what a
// GM's whispered command asks for (a reply, a party withdrawal), and "stay"
// holds a bot a GM told to stay, ahead of looting and every errand that
// would move it. "chat" whispers a delayed reply to a player's whisper
// (ChatPolicy), after the flee and heal rules so it never delays one; a
// due reply waits for them. "announce" says the queued craft order lines
// (answers and finished GM orders) in turn, after "chat". "party" sends a lead
// bot's invitations and oustings and a join bot's handover after
// "announce", ahead of every errand (docs/parties/leader.md). "channel" asks
// for the bot's command channel's parties and members right after it, so
// MayFight's rule 2 knows them before a fight (channel.go). "leader" handles a dead or lost leader ahead of
// following it. The shot toggle is set-up, not a plan rule: it runs once
// per shot template carried, ahead of the fight it serves, after the flee
// rules so it never delays one. "buffpotion" drinks the build's buff potion
// at the farming spot out of combat, after the rebuff so a support's buff
// lands first. "recover" reads a Scroll: Recovery out of combat while the
// bot has a death penalty level, anywhere: lifting the penalty comes first
// (#491), so it goes ahead of "rest", "rebuff", "buffpotion" and every
// errand, behind only the death, command, chat, party, fight and
// overweight rules. "supply" is the farmer's trip home to restock (D16),
// after the errands that already serve it and ahead of following and
// farming;
// "escape" holds every rule after it while its Scroll of Escape's recall
// runs, after "critical" so a potion still goes down. "enchant" uses the
// enchant scrolls on the worn gear in town, after the errands that wear,
// buy and draw and ahead of every rule that would walk the bot out of town.
// "sweep" casts Sweeper on a corpse the bot spoiled while its sweep window
// lasts, right after "outnumbered" so only the critical, escape, heal,
// outnumbered and flee rules go first, and ahead of "loot". "duel" fights
// the duel opponent and holds every rule after it while the duel is on
// (ruleDuel), after "raise", and declines a challenge the persona takes no
// duels for. "guard" stops a fight the guards against accidental karma
// forbid (pvp_guards.go): an Esc for the bot's own cast in its window and a
// disengage move, right after "escape" so only the critical and escape
// rules go first. "overweight" drops what an immobile bot
// carries (#488) ahead of "attacked", since a drop needs no move and an
// attacker the bot cannot reach at speed 0 would hold it in the fight; it
// gives way to the fight while the drop gate is closed. "overweight hold"
// then holds "rest", whose regeneration the immobile band cuts to a tenth,
// and every errand, trip and farming rule until the bot has shed.
// "arena fight", "hunt" and "challenge" find fights (pvp_finding.go,
// pvp_arena.go, docs/pvp/finding.md): arena targets on an arena trip,
// flagged players of its tier and duel challenges, after the rest and
// buff rules so it starts them rested and buffed. "arena" is the arena
// trip, an errand right after "supply"; "arena leave" its leave, ahead
// of "attacked" so no attack starts the fight it stops again. "party
// buff" and "party fight" are party PvP (pvp_party.go, docs/pvp/party.md):
// the PvP self buffs against the party's enemy set, after "duel" so a duel
// holds them, and the fight against the party's enemy (the enemy healer
// first), right after "sweep": "outnumbered" leaves its target to it once
// past its flee, and it goes ahead of "attacked" and "assist".
// "olympiad" is MayFight rule 0's fight, right after "duel"; "olympiad
// wait" holds every rule after the rest and buff rules in an Olympiad match
// and on its waiting list; "olympiad entry" is the registration errand
// (pvp_olympiad.go, docs/pvp/olympiad.md).
var playRules = []rule{
	{name: "dead", run: (*Brain).ruleDead},
	{name: "prompt", run: (*Brain).rulePrompt},
	{name: "command", run: (*Brain).ruleCommand},
	{name: "critical", run: (*Brain).ruleCritical},
	{name: "escape", run: (*Brain).ruleEscape},
	{name: "guard", run: (*Brain).ruleGuard},
	{name: "heal", run: (*Brain).ruleHeal},
	{name: "raise", run: (*Brain).ruleRaise},
	{name: "duel", run: (*Brain).ruleDuel},
	{name: "olympiad", run: (*Brain).ruleOlympiad},
	{name: "party buff", run: (*Brain).rulePartyBuff},
	{name: "outnumbered", run: (*Brain).ruleOutnumbered},
	{name: "sweep", run: (*Brain).ruleSweep},
	{name: "party fight", run: (*Brain).rulePartyFight},
	{name: "chat", run: (*Brain).ruleChat},
	{name: "announce", run: (*Brain).ruleAnnounce},
	{name: "party", run: (*Brain).ruleParty},
	{name: "channel", run: (*Brain).ruleChannel},
	{name: "shots", run: (*Brain).ruleShots},
	{name: "overweight", run: (*Brain).ruleOverweight},
	{name: "arena leave", run: (*Brain).ruleArenaLeave},
	{name: "attacked", run: (*Brain).ruleAttacked},
	{name: "overweight hold", run: (*Brain).ruleOverweightHold},
	{name: "recover", run: (*Brain).ruleRecover},
	{name: "rest", run: (*Brain).ruleRest},
	{name: "rebuff", run: (*Brain).ruleRebuff},
	{name: "buffpotion", run: (*Brain).ruleBuffPotion},
	{name: "olympiad wait", run: (*Brain).ruleOlympiadWait},
	{name: "arena fight", run: (*Brain).ruleArenaFight},
	{name: "hunt", run: (*Brain).ruleHunt},
	{name: "challenge", run: (*Brain).ruleChallenge, karma: true},
	{name: "rooms", run: (*Brain).ruleRooms, stop: (*Brain).releasePartyClaim},
	{name: "stay", run: (*Brain).ruleStay},
	{name: "loot", run: (*Brain).ruleLoot},
	{name: "valuables", run: (*Brain).ruleValuables, karma: true},
	{name: "store", run: (*Brain).ruleStore, stop: (*Brain).releaseStore},
	{name: "assist", run: (*Brain).ruleAssist},
	{name: "classchange", run: (*Brain).ruleClassChange, karma: true},
	{name: "olympiad entry", run: (*Brain).ruleOlympiadEntry},
	{name: "train", run: (*Brain).ruleTrain, karma: true},
	{name: "gear", run: (*Brain).ruleGear, karma: true},
	{name: "market", run: (*Brain).ruleMarket, karma: true},
	{name: "shop", run: (*Brain).ruleShop, karma: true},
	{name: "dyes", run: (*Brain).ruleDyes, karma: true},
	{name: "warehouse", run: (*Brain).ruleWarehouse, karma: true},
	{name: "enchant", run: (*Brain).ruleEnchant},
	{name: "member", run: (*Brain).ruleMember, karma: true},
	{name: "recipe", run: (*Brain).ruleRecipe, karma: true},
	{name: "craft", run: (*Brain).ruleCraft, stop: (*Brain).releaseCraftBoard},
	{name: "supply", run: (*Brain).ruleSupply, karma: true},
	{name: "arena", run: (*Brain).ruleArena, karma: true},
	{name: "leader", run: (*Brain).ruleLeader},
	{name: "follow", run: (*Brain).ruleFollow},
	{name: "newbie", run: (*Brain).ruleNewbie, karma: true},
	{name: "signs", run: (*Brain).ruleSigns, karma: true},
	{name: "travel", run: (*Brain).ruleTravel, karma: true},
	{name: "spot", run: (*Brain).ruleSpot, stop: (*Brain).leaveSpot},
	{name: "farm", run: (*Brain).ruleFarm},
	{name: "goal", run: (*Brain).ruleGoal},
	{name: "idle", run: (*Brain).ruleIdle},
}

// storeRules replace rules 3-10 in store mode: the bot opens its store
// and stays seated in it, watching for it to close.
var storeRules = []rule{
	{name: "dead", run: (*Brain).ruleDead},
	{name: "prompt", run: (*Brain).rulePrompt},
	{name: "command", run: (*Brain).ruleCommand},
	{name: "chat", run: (*Brain).ruleChat},
	{name: "store", run: (*Brain).ruleStore},
}

// RuleNames lists every name decide returns for a matching rule, for a
// metrics registry (botmetrics.NewRegistry).
func RuleNames() []string {
	var names []string
	for _, r := range slices.Concat(playRules, storeRules) {
		if !slices.Contains(names, r.name) {
			names = append(names, r.name)
		}
	}
	return names
}

// decide runs the decision table at now and returns the matching rule's
// name ("" for none) and its action.
func (b *Brain) decide(now time.Time) (string, Action) {
	rules := playRules
	if b.store.active() || b.world.Self.StoreType != 0 {
		rules = storeRules
	}
	b.syncValuables(now)
	b.endStaleRound(now)
	b.karmaTick(now)
	karma := b.world.Self.Karma > 0
	for _, r := range rules {
		if karma && r.karma {
			continue
		}
		if a, ok := r.run(b, now); ok {
			return r.name, a
		}
	}
	return "", Action{}
}

// ruleDead is rule 1: a dead bot answers a resurrection offer first (the
// rule 2 prompt that only a dead bot gets), per Recover; in a party it
// waits Recover.ResurrectWait from its death for one; then it follows its
// death policy. A restart the server left unanswered is asked again only
// after restartRetry, which an answered offer starts as well.
func (b *Brain) ruleDead(now time.Time) (Action, bool) {
	w, r := b.world, &b.persona.Recover
	if !w.Self.Dead {
		return Action{}, false
	}
	if o := w.Prompts.Resurrect; o.Open {
		return answerResurrectAction(o, r.AcceptResurrection), true
	}
	if w.Party.In() && now.Sub(w.Prompts.Restart.At) < r.ResurrectWait {
		return Action{}, true
	}
	if b.persona.Death == nil {
		return Action{}, true
	}
	a := b.persona.Death(b.world, now)
	if a.kind == actRestart && !b.restartedAt.IsZero() && now.Sub(b.restartedAt) < restartRetry {
		return Action{}, true
	}
	return a, true
}

// rulePrompt is rule 2: an open prompt waits for the persona's answer. A
// party invitation still answerable is accepted or declined at once, per
// the party slot (slotAnswer), else the persona, then a clan invitation,
// then a trade request, and an open trade holds the bot (ruleTrade); the
// restart prompt opens only while dead, so rule 1 answers it first.
func (b *Brain) rulePrompt(now time.Time) (Action, bool) {
	if inv := b.world.Prompts.Party; inv.Pending(now) {
		if accept, ok := b.slotAnswer(inv.Requester); ok {
			return answerPartyAction(accept), true
		}
		join := b.persona.JoinParty
		return answerPartyAction(join != nil && join(b.world, inv.Requester, now)), true
	}
	if inv := b.world.Prompts.Clan; inv.Pending(now) {
		join := b.persona.JoinClan
		return answerClanAction(join != nil && inv.Requester != "" && join(b.world, inv.Requester, inv.Clan, now)), true
	}
	if a, ok := b.ruleTrade(now); ok {
		return a, true
	}
	if !b.world.Prompts.Restart.Open {
		return Action{}, false
	}
	if b.persona.Answer != nil {
		return b.persona.Answer(b.world, now), true
	}
	return Action{}, true
}

// ruleCritical is rule 3: in combat with HP critical, cast a ready heal or
// drain step whose gates pass (sustain, standing up first), else drink a
// potion whose reuse has run out, else heal itself, else flee away from the
// target unless FleeOutrun holds it in the fight (mayFlee), which leaves
// the turn to the fight rules. With two or more attackers on the bot and HP
// under CrowdedHP it heals or drains, else drinks a potion, early,
// standing, and leaves the turn to the next rules when none is ready. No
// potion goes out over a heal or drain in flight (sustaining), and no heal
// or drain during a recall (escaping). Against a player it fights (pvpFoe)
// it does not flee: the PvP layer's break-off decides (docs/pvp/layer.md).
// Nor in a duel, which never kills: a run would stray from the other side
// and cancel it (duel/duel.go disturbed), as a party duel's defeated
// member, frozen at 1 HP until its side's end, would.
func (b *Brain) ruleCritical(now time.Time) (Action, bool) {
	s := &b.world.Self
	if !s.InCombat {
		return Action{}, false
	}
	th := b.build().Thresholds
	esc := b.escaping(now)
	if !below(s.HP, s.MaxHP, th.CriticalHP) {
		if s.Sitting || !below(s.HP, s.MaxHP, th.CrowdedHP) || len(b.attackers(now)) < 2 {
			return Action{}, false
		}
		if !esc {
			if a, ok := b.sustain(now); ok {
				return a, true
			}
			if b.sustaining(now) {
				return Action{}, true
			}
		}
		return b.drinkPotion(now)
	}
	if !esc {
		if a, ok := b.sustain(now); ok {
			return a, true
		}
		if b.sustaining(now) {
			return Action{}, true
		}
	}
	if !s.Sitting {
		if a, ok := b.drinkPotion(now); ok {
			return a, true
		}
	}
	if esc {
		// A heal or a run away sent mid-recall would run after the
		// teleport (ruleEscape); the potion above does not wait.
		return Action{}, true
	}
	if a, ok := b.heal(s.ObjectID, now); ok {
		return a, true
	}
	if !b.mayFlee(now) || b.pvpFoe(now) || b.inDuel(now) || b.inOlympiad(now) {
		return Action{}, false
	}
	return b.flee(b.world.Target, now), true
}

// fleeRun is the run away from an attacker under way.
type fleeRun struct {
	// from is the attacker the run is away from, to where it goes, at the
	// last decision that kept it.
	from int32
	to   Point
	at   time.Time
}

// fleeKeep is how long after the last decision that kept it a run away is
// kept: a flee again later picks a fresh point away from the attacker.
const fleeKeep = 5 * time.Second

// fleeTurns are the directions a flee point is tried in, in turn, off
// straight away from the attacker: ahead first, then wider either side.
var fleeTurns = [...]float64{0, math.Pi / 4, -math.Pi / 4, math.Pi / 2, -math.Pi / 2, 3 * math.Pi / 4, -3 * math.Pi / 4, math.Pi}

// flee runs FleeDistance away from object id, or in a random direction
// when id is not in sight, standing up first and casting the kit's Flee
// skill first when it is ready. The point is on ground of the
// bot's SCC (walker.inSCC), walked through the walker as checked segments
// (D13), each from standstill; a run that arrived or ended unreachable
// picks a fresh point at once. None while Ultimate Defense holds the bot
// (immobile).
func (b *Brain) flee(id int32, now time.Time) Action {
	s, v := &b.world.Self, &b.fleeing
	if s.Sitting {
		return b.toggle(now)
	}
	if b.immobile(now) {
		// Ultimate Defense: a move would only idle the bot.
		return Action{}
	}
	if f := b.build().Kit.Flee; f != 0 && b.castReady(f, now) {
		return b.castOn(s.ObjectID, f, false, now)
	}
	if s.Moving(now) {
		return Action{}
	}
	keep := v.from == id && v.to != (Point{}) && now.Sub(v.at) <= fleeKeep
	for range 2 {
		if !keep {
			v.from, v.to = id, b.fleePoint(id, now)
		}
		v.at = now
		a, st := b.goTo(v.to, walkSlop, walkFlee, now)
		if st == walkWalking {
			return a
		}
		keep = false
	}
	v.to = Point{}
	return Action{}
}

// fleePoint is where the bot flees to from id: FleeDistance away from it, on
// ground of the bot's SCC, in the first of fleeTurns off straight away that
// leaves at least half that distance, else the farthest of them. With
// karma a heading whose line enters the karma margin is skipped like a
// blocked one (karmaLine), unless every heading does.
func (b *Brain) fleePoint(id int32, now time.Time) Point {
	from := b.world.Self.Position(now)
	angle := b.rng.Float64() * 2 * math.Pi
	if o := b.world.object(id); o != nil {
		if t := o.Position(now); t.X != from.X || t.Y != from.Y {
			angle = math.Atan2(float64(from.Y-t.Y), float64(from.X-t.X))
		}
	}
	w, dist := b.walker(), b.persona.FleeDistance
	for _, karma := range []bool{true, false} {
		best, bestD := from, int64(-1)
		for _, turn := range fleeTurns {
			to := w.inSCC(from, offset(from, angle+turn, dist))
			if karma && b.karmaLine(from, to) {
				continue
			}
			d := horizontal2(from, to)
			if 4*d >= int64(dist)*int64(dist) {
				return to
			}
			if d > bestD {
				best, bestD = to, d
			}
		}
		if bestD >= 0 {
			return best
		}
	}
	return from
}

// ruleAttacked is rule 5: in combat, stand up and fight back. The selected
// target is fought while the PvP policy allows it and nothing else is
// hitting the bot; otherwise the hardest hitter is selected. An attack on a
// target the policy no longer allows (a player who unflagged) is dropped.
// Hit only by creatures it may not fight (blacklisted), the bot runs away.
// With no one left to fight, it assists the party leader, or else leaves
// the turn to the loot it queued (rule 8).
func (b *Brain) ruleAttacked(now time.Time) (Action, bool) {
	s := &b.world.Self
	if !s.InCombat {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	id := b.world.Target
	if s := b.firstSummon(id, now); s != 0 {
		// Its summon stands in for the target (standIn).
		id = s
	}
	fightable := b.mayFight(id, now)
	hits := b.attackers(now)
	switch {
	case !fightable && id != 0 && b.engaged == id:
		return b.disengage(now), true
	case fightable && (len(hits) == 0 || b.world.attacking(id, now)):
		return b.fight(id, now), true
	case len(hits) > 0:
		return b.fight(hits[0], now), true
	}
	if id := b.unfought(now); id != 0 {
		return b.flee(id, now), true
	}
	if id := b.assistTarget(now); id != 0 {
		return b.fight(id, now), true
	}
	// The combat stance outlasts the kill: queued loot goes to rule 8 now,
	// while it is still locked to the bot (the server's killrewards.go).
	return Action{}, !b.lootQueued(now)
}

// ruleRest is rule 6: out of combat with HP or MP low, sit until both
// recover, then stand, by the build's thresholds; it sits with Relax where
// the kit has it, its combat toggles off first (sitDown). A bot that knows
// one of its rotation's skills also sits under MPReserve: there fight casts
// no attack, and the next pull would be all melee.
func (b *Brain) ruleRest(now time.Time) (Action, bool) {
	s, p := &b.world.Self, b.build().Thresholds
	if s.InCombat {
		return Action{}, false
	}
	sitMP := p.SitMP
	if b.knowsAttackSkill() {
		sitMP = max(sitMP, p.MPReserve)
	}
	if b.resting {
		if below(s.HP, s.MaxHP, p.StandHP) || below(s.MP, s.MaxMP, max(p.StandMP, sitMP)) {
			if !s.Sitting && !s.Moving(now) {
				return b.sitDown(now), true
			}
			return Action{}, true
		}
		if s.Sitting {
			return b.toggle(now), true
		}
		b.resting = false
		return Action{}, false
	}
	if !below(s.HP, s.MaxHP, p.SitHP) && !below(s.MP, s.MaxMP, sitMP) {
		return Action{}, false
	}
	b.resting = true
	if s.Sitting || s.Moving(now) {
		return Action{}, true
	}
	return b.sitDown(now), true
}

// ruleGoal is rule 9: the persona's goal.
func (b *Brain) ruleGoal(now time.Time) (Action, bool) {
	if b.persona.Goal == nil {
		return Action{}, false
	}
	return b.persona.Goal(b.world, now)
}

// ruleIdle is rule 10: now and then a small move around idleCenter (a
// stroll in the open, driftPoint), a turn, an emote or a sit; a bot sitting
// idle stands up instead.
func (b *Brain) ruleIdle(now time.Time) (Action, bool) {
	s, p := &b.world.Self, &b.persona
	if s.InCombat || s.Moving(now) {
		return Action{}, false
	}
	if w := &b.walk; w.purpose == walkIdle && w.status == walkWalking && w.dest != (Point{}) {
		// The drift under way goes on, segment by segment.
		if a, st := b.goTo(w.dest, walkSlop, walkIdle, now); st == walkWalking {
			return a, true
		}
	}
	if b.rng.Float64() >= p.IdleChance {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	from := s.Position(now)
	angle := b.rng.Float64() * 2 * math.Pi
	switch r := b.rng.IntN(10); {
	case r < 5:
		if to, ok := b.driftPoint(from, angle); ok {
			if a, st := b.goTo(to, walkSlop, walkIdle, now); st == walkWalking {
				return a, true
			}
		}
		// No drift from here (no geodata, or nowhere to go): a turn, or
		// nothing when no turn step holds on the geodata.
		return b.turnAround(angle, now), true
	case r < 7:
		return b.turnAround(angle, now), true
	case r < 9 && len(p.Emotes) > 0:
		return emoteAction(p.Emotes[b.rng.IntN(len(p.Emotes))]), true
	default:
		return b.toggle(now), true
	}
}

// driftTries is how many drift points driftPoint draws before the drift
// gives up for a turn.
const driftTries = 4

// driftPoint is where an idle drift from from walks: a point within
// IdleRadius of idleCenter, first along angle, then along fresh random
// angles, up to driftTries of them. On geodata the point is ground of the
// bot's SCC (inSCC) in the open (walker.open: no roof, upper floor or deck
// over it), and, while the bot stands in the open within the radius, one
// straight checked segment away (walker.segmentOK): a drift is a stroll in
// the open, never a route into a building or round a wall (#617). A bot
// beyond the radius (out in the field, or just landed) routes back toward
// the centre, and a bot under a roof (logged in or sent indoors) routes out
// to the open.
// Without geodata the first point is taken as it is.
func (b *Brain) driftPoint(from Point, angle float64) (Point, bool) {
	p, w := &b.persona, b.walker()
	center := b.idleCenter(from)
	r := int64(p.IdleRadius)
	near := horizontal2(from, center) <= r*r && w.open(from)
	for i := range driftTries {
		if i > 0 {
			angle = b.rng.Float64() * 2 * math.Pi
		}
		dist := int32(float64(p.IdleRadius) * math.Sqrt(b.rng.Float64()))
		to := offset(center, angle, dist)
		to.Z = from.Z
		to = w.inSCC(from, to)
		if w.nav == nil {
			return to, true
		}
		if !w.open(to) || near && !w.segmentOK(from, to) {
			continue
		}
		return to, true
	}
	return Point{}, false
}

// toggle is the sit/stand toggle, or nothing while an earlier toggle waits
// for the server's ChangeWaitType: a second toggle would undo the first.
func (b *Brain) toggle(now time.Time) Action {
	if !b.toggledAt.IsZero() && now.Sub(b.toggledAt) < toggleSettle {
		return Action{}
	}
	return sitStandAction()
}

// below reports whether cur is under pct percent of maxV; never for an
// unknown maximum.
func below(cur, maxV int32, pct int) bool {
	return maxV > 0 && int64(cur)*100 < int64(maxV)*int64(pct)
}

// offset is the point dist away from p in the horizontal direction angle.
func offset(p Point, angle float64, dist int32) Point {
	return Point{
		X: p.X + int32(math.Round(float64(dist)*math.Cos(angle))),
		Y: p.Y + int32(math.Round(float64(dist)*math.Sin(angle))),
		Z: p.Z,
	}
}
