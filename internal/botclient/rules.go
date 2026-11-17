package botclient

import (
	"math"
	"slices"
	"time"
)

// Persona is the rule set a brain plays: the thresholds of the shared rules
// and hooks for the persona's own choices. Every value is a configurable
// default, not a safety boundary (D6); the operational limits are in
// pacing.go. A nil hook does nothing.
type Persona struct {
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
	// Potions are the healing potion template ids rule 3 drinks, best
	// first; none drinks nothing.
	Potions []int32
	// Shots are the shot template ids whose auto use the bot turns on when
	// it carries them (rule "shots"); none turns none on.
	Shots []int32
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
	// Answer answers an open prompt (rule 2); a zero Action waits.
	Answer func(w *World, now time.Time) Action
	// JoinParty decides whether to accept the party invitation of the
	// player named requester (rule 2); nil declines every invitation.
	JoinParty func(w *World, requester string, now time.Time) bool
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
	// the bot visits the nearest one in sight.
	Trainers map[int32][]int32
	// Spots is how the bot picks and keeps its farming spot when the brain
	// has a spot registry (BrainConfig.Spots, rule 9 errand "spot").
	Spots SpotPolicy
	// EngagedWindow is how long after another player's last sign of
	// fighting a monster (an attack or a cast between them, or one closing
	// in on the other) the bot leaves the monster alone as a fresh target
	// (anti-KS, rule 9 errand "farm"); a party member's fight is no sign.
	// A monster attacking the bot is fought all the same. 0 skips none.
	EngagedWindow time.Duration
	// Follow is how the bot follows the leader of its party (rule
	// "follow").
	Follow FollowPolicy
	// Newbie is when the bot asks a Newbie Guide for the newbie buffs
	// (rule 9 errand "newbie").
	Newbie NewbiePolicy
	// Travel is how the bot travels by gatekeeper to a spot out of walking
	// reach when the brain has a travel map (BrainConfig.Travel, rule 9
	// errand "travel").
	Travel TravelPolicy
	// Shop is what the bot buys and sells at merchants (rule 9 errand
	// "shop").
	Shop ShopPolicy
	// Gear is how the bot wears and buys better weapons and armor (rule 9
	// errand "gear", and the shop errand).
	Gear GearPolicy
	// Loot is what the bot picks up after a fight and drops when
	// overweight (rule 8).
	Loot LootPolicy
	// Goal is the persona's goal (rule 9). It reports false when it has
	// nothing to do, which leaves the turn to idle drift.
	Goal func(w *World, now time.Time) (Action, bool)
}

// DefaultPersona is a support player that rests when low, fights back when
// hit, takes a resurrection when killed or restarts in town after a while,
// sits by its dead leader, falls back to town when the leader is gone,
// learns the support skills of a mystic
// at its trainer, heals and buffs its party with them, assists its party
// leader, follows the leader of a party it joined and otherwise drifts
// around where it stands.
func DefaultPersona() Persona {
	return Persona{
		Death:          restartToVillage,
		Recover:        defaultRecoverPolicy(),
		JoinParty:      acceptParty,
		CriticalHP:     20,
		FleeDistance:   600,
		OutmatchedHP:   50,
		BlacklistAfter: 4,
		BlacklistFor:   3 * time.Minute,
		PvP:            PvPPolicy{FightFlagged: true},
		Potions:        healingPotions,
		Shots:          shotTemplates,
		SitHP:          40,
		SitMP:          30,
		StandHP:        90,
		StandMP:        90,
		IdleChance:     0.1,
		IdleRadius:     250,
		Emotes:         []int32{2, 6, 7, 10, 11}, // greet, yes, bow, laugh, applaud
		LearnSkills:    defaultLearnSkills(),
		Trainers:       defaultTrainers(),
		Spots:          defaultSpotPolicy(),
		EngagedWindow:  10 * time.Second,
		MPReserve:      50,
		Heals:          defaultHeals,
		Buffs:          defaultBuffs,
		RebuffBefore:   time.Minute,
		AssistMin:      500 * time.Millisecond,
		AssistMax:      1500 * time.Millisecond,
		Follow:         defaultFollowPolicy(),
		Newbie:         defaultNewbiePolicy(),
		Travel:         defaultTravelPolicy(),
		Shop:           defaultShopPolicy(),
		Gear:           defaultGearPolicy(),
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
	// the carried item a gear UseItem equips.
	object int32
	// item is the template id of the item a UseItem or RequestAutoSoulShot
	// acts on.
	item int32
	// skill is the skill a cast casts.
	skill int32
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
	actLeaveParty
	actUseItem
	actAutoShot
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
)

// gate is the reuse gate the action's frame passes. Appearing,
// RequestRestartPoint, RequestAnswerJoinParty, RequestWithdrawParty, UseItem,
// RequestAutoSoulShot, RequestAcquireSkillInfo and RequestAcquireSkill have
// no reuse gate on the server: the frame bucket bounds them, restartRetry
// spaces a repeated restart, an invitation is answered once, potionReuse
// spaces the potions the bot drinks (the potions' shared heal over time),
// shotRetry the toggles of a shot template, and a trainer visit sends one
// skill request per server answer. RequestBuyItem and RequestSellItem have
// none either: a shop visit sends one per BuyList or SellList it opened.
// RequestMagicSkillUse (attack, heal and
// buff casts) has none either: a cast waits for the one before it
// (castPending, castTimeout) and its skill's reuse, and buffRetry spaces
// one buff on one target. AnswerTradeRequest, AddTradeItem and TradeDone
// have no reuse gate either (no flood protector covers them): a request is
// answered once, and a trade gets one add, one confirm and one cancel
// (Trade.added, confirmed, cancelAt). A pickup click (Action) has none:
// lootRetry spaces the clicks on one item. A whisper (Say2 Tell), a GM
// command's reply or a chat reply, has no server gate either (chat.go
// chatTell): it keeps the chat gate. DlgAnswer and RequestUserCommand have
// none either: an offer is answered once, and escapeRetry spaces a repeated
// /unstuck. The sell store packets
// (RequestPrivateStoreManageSell, SetPrivateStoreMsgSell,
// SetPrivateStoreListSell, RequestPrivateStoreQuitSell) have no flood
// protector either (network/privatestore_dispatch.go): an opening sends
// each once per answer it waits for, and StorePolicy.Retry spaces the
// openings.
func (a Action) gate() gate {
	switch a.kind {
	case actEmote:
		return gateEmote
	case actSay, actTell, actChatReply:
		return gateChat
	case actBypass:
		return gateBypass
	case actDrop:
		return gateDrop
	}
	return gateNone
}

func moveAction(to, from Point) Action {
	return Action{kind: actMove, payload: encodeMoveBackwardToLocation(to, from)}
}

// attackAction clicks the selected object id, which attacks it.
func attackAction(id int32, from Point) Action {
	return Action{kind: actAttack, payload: encodeAction(id, from), object: id}
}

func sitStandAction() Action {
	return Action{kind: actSitStand, payload: encodeRequestActionUse(actionSitStand)}
}

func emoteAction(id int32) Action {
	return Action{kind: actEmote, payload: encodeRequestSocialAction(id)}
}

// sayAction says text on the all channel.
func sayAction(text string) Action {
	return Action{kind: actSay, payload: encodeSay2(text, chatAll)}
}

// restartToVillage is the death policy that restarts in the nearest town
// when the death window offers it, and otherwise waits.
func restartToVillage(w *World, _ time.Time) Action {
	if !w.Prompts.Restart.Points.Village {
		return Action{}
	}
	return Action{kind: actRestart, payload: encodeRequestRestartPoint(restartVillage)}
}

// rule is one row of the decision table. run reports whether the rule
// matched; a matching rule ends the decision even when it sends nothing.
type rule struct {
	name string
	run  func(b *Brain, now time.Time) (Action, bool)
}

// playRules are rules 1-10, first match wins. "heal" is rule 3's heal of
// the party, ahead of fighting so a support in combat still heals;
// "assist" (the leader's target), "train" (the trainer), "gear" (wearing
// better gear, ahead of the shop so a bought piece is worn before the next
// sale), "shop" (a merchant's buy and sell windows), "follow" (the slot
// behind the party leader), "newbie" (the Newbie Guide's buffs), "travel"
// (a gatekeeper trip to a spot out of walking reach), "spot" (getting to the
// farming spot) and "farm" (attacking a monster there) are rule 9 errands,
// ahead of the persona's goal: a party member assists its leader, visits its
// trainer, then follows instead of farming. "loot" is rule 8, after the
// rebuff and ahead of every errand. "store" is the merchant's private store:
// it opens where the bot stands, ahead of every errand that would walk it
// away, and puts the bot in store mode (storeRules). "command" sends what a
// GM's whispered command asks for (a reply, a party withdrawal), and "stay"
// holds a bot a GM told to stay, ahead of looting and every errand that
// would move it. "chat" whispers a delayed reply to a player's whisper
// (ChatPolicy), after the flee and heal rules so it never delays one; a
// due reply waits for them. "leader" handles a dead or lost leader ahead of
// following it. The shot toggle is set-up, not a plan rule: it runs once
// per shot template carried, ahead of the fight it serves, after the flee
// rules so it never delays one.
var playRules = []rule{
	{"dead", (*Brain).ruleDead},
	{"prompt", (*Brain).rulePrompt},
	{"command", (*Brain).ruleCommand},
	{"critical", (*Brain).ruleCritical},
	{"heal", (*Brain).ruleHeal},
	{"outnumbered", (*Brain).ruleOutnumbered},
	{"chat", (*Brain).ruleChat},
	{"shots", (*Brain).ruleShots},
	{"attacked", (*Brain).ruleAttacked},
	{"rest", (*Brain).ruleRest},
	{"rebuff", (*Brain).ruleRebuff},
	{"stay", (*Brain).ruleStay},
	{"loot", (*Brain).ruleLoot},
	{"store", (*Brain).ruleStore},
	{"assist", (*Brain).ruleAssist},
	{"train", (*Brain).ruleTrain},
	{"gear", (*Brain).ruleGear},
	{"shop", (*Brain).ruleShop},
	{"leader", (*Brain).ruleLeader},
	{"follow", (*Brain).ruleFollow},
	{"newbie", (*Brain).ruleNewbie},
	{"travel", (*Brain).ruleTravel},
	{"spot", (*Brain).ruleSpot},
	{"farm", (*Brain).ruleFarm},
	{"goal", (*Brain).ruleGoal},
	{"idle", (*Brain).ruleIdle},
}

// storeRules replace rules 3-10 in store mode: the bot opens its store
// and stays seated in it, watching for it to close.
var storeRules = []rule{
	{"dead", (*Brain).ruleDead},
	{"prompt", (*Brain).rulePrompt},
	{"command", (*Brain).ruleCommand},
	{"chat", (*Brain).ruleChat},
	{"store", (*Brain).ruleStore},
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
	for _, r := range rules {
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
// the persona, then a trade request, and an open trade holds the bot
// (ruleTrade); the restart prompt opens only while dead, so rule 1 answers
// it first.
func (b *Brain) rulePrompt(now time.Time) (Action, bool) {
	if inv := b.world.Prompts.Party; inv.Pending(now) {
		join := b.persona.JoinParty
		return answerPartyAction(join != nil && join(b.world, inv.Requester, now)), true
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

// ruleCritical is rule 3: in combat with HP critical, stand up, then drink
// a potion whose reuse has run out, else heal itself, else flee away from
// the target.
func (b *Brain) ruleCritical(now time.Time) (Action, bool) {
	s := &b.world.Self
	if !s.InCombat || !below(s.HP, s.MaxHP, b.persona.CriticalHP) {
		return Action{}, false
	}
	if !s.Sitting {
		if a, ok := b.drinkPotion(now); ok {
			return a, true
		}
	}
	if a, ok := b.heal(s.ObjectID, now); ok {
		return a, true
	}
	return b.flee(b.world.Target, now), true
}

// flee runs FleeDistance away from object id, or in a random direction
// when id is not in sight, standing up first.
func (b *Brain) flee(id int32, now time.Time) Action {
	s := &b.world.Self
	if s.Sitting {
		return b.toggle(now)
	}
	if s.Moving(now) {
		return Action{}
	}
	from := s.Position(now)
	angle := b.rng.Float64() * 2 * math.Pi
	if o := b.world.object(id); o != nil {
		if t := o.Position(now); t.X != from.X || t.Y != from.Y {
			angle = math.Atan2(float64(from.Y-t.Y), float64(from.X-t.X))
		}
	}
	return moveAction(offset(from, angle, b.persona.FleeDistance), from)
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
// recover, then stand.
func (b *Brain) ruleRest(now time.Time) (Action, bool) {
	s, p := &b.world.Self, &b.persona
	if s.InCombat {
		return Action{}, false
	}
	if b.resting {
		if below(s.HP, s.MaxHP, p.StandHP) || below(s.MP, s.MaxMP, p.StandMP) {
			if !s.Sitting && !s.Moving(now) {
				return b.toggle(now), true
			}
			return Action{}, true
		}
		if s.Sitting {
			return b.toggle(now), true
		}
		b.resting = false
		return Action{}, false
	}
	if !below(s.HP, s.MaxHP, p.SitHP) && !below(s.MP, s.MaxMP, p.SitMP) {
		return Action{}, false
	}
	b.resting = true
	if s.Sitting || s.Moving(now) {
		return Action{}, true
	}
	return b.toggle(now), true
}

// ruleGoal is rule 9: the persona's goal.
func (b *Brain) ruleGoal(now time.Time) (Action, bool) {
	if b.persona.Goal == nil {
		return Action{}, false
	}
	return b.persona.Goal(b.world, now)
}

// ruleIdle is rule 10: now and then a small move, a turn, an emote or a
// sit; a bot sitting idle stands up instead.
func (b *Brain) ruleIdle(now time.Time) (Action, bool) {
	s, p := &b.world.Self, &b.persona
	if s.InCombat || s.Moving(now) || b.rng.Float64() >= p.IdleChance {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	from := s.Position(now)
	angle := b.rng.Float64() * 2 * math.Pi
	switch r := b.rng.IntN(10); {
	case r < 5:
		dist := int32(float64(p.IdleRadius) * math.Sqrt(b.rng.Float64()))
		to := offset(b.anchor, angle, dist)
		to.Z = from.Z
		return moveAction(to, from), true
	case r < 7:
		return moveAction(offset(from, angle, turnStep), from), true
	case r < 9 && len(p.Emotes) > 0:
		return emoteAction(p.Emotes[b.rng.IntN(len(p.Emotes))]), true
	default:
		return b.toggle(now), true
	}
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
