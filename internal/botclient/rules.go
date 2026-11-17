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
	// Store keeps the brain in store mode (rules 3-10 replaced by holding
	// still) even while the server reports no store open.
	Store bool
	// CriticalHP is the HP percent under which a bot in combat flees
	// FleeDistance away from its target (rule 3); 0 never flees.
	CriticalHP   int
	FleeDistance int32
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
	// Death decides for a dead bot (rule 1); a zero Action waits. A
	// restart it asks for goes out at most once per restartRetry.
	Death func(w *World, now time.Time) Action
	// Answer answers an open prompt (rule 2); a zero Action waits.
	Answer func(w *World, now time.Time) Action
	// JoinParty decides whether to accept the party invitation of the
	// player named requester (rule 2); nil declines every invitation.
	JoinParty func(w *World, requester string, now time.Time) bool
	// Goal is the persona's goal (rule 9). It reports false when it has
	// nothing to do, which leaves the turn to idle drift.
	Goal func(w *World, now time.Time) (Action, bool)
}

// DefaultPersona is a passive player that rests when low, fights back when
// hit, restarts in town when killed and otherwise drifts around where it
// stands.
func DefaultPersona() Persona {
	return Persona{
		Death:        restartToVillage,
		JoinParty:    acceptParty,
		CriticalHP:   20,
		FleeDistance: 600,
		SitHP:        40,
		SitMP:        30,
		StandHP:      90,
		StandMP:      90,
		IdleChance:   0.1,
		IdleRadius:   250,
		Emotes:       []int32{2, 6, 7, 10, 11}, // greet, yes, bow, laugh, applaud
	}
}

// turnStep is the length of the short step that turns the character.
const turnStep = 40

// Action is one packet the brain may send. The zero Action sends nothing.
type Action struct {
	kind    actionKind
	payload []byte
	// object is the object an attack acts on.
	object int32
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
	actAnswerParty
	actLeaveParty
)

// gate is the reuse gate the action's frame passes. Appearing,
// RequestRestartPoint, RequestAnswerJoinParty and RequestWithdrawParty have
// no reuse gate on the server: the frame bucket bounds them, restartRetry
// spaces a repeated restart, and an invitation is answered once.
func (a Action) gate() gate {
	switch a.kind {
	case actEmote:
		return gateEmote
	case actSay:
		return gateChat
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

// playRules are rules 1-10, first match wins. Rules 4 (two or more
// attackers), 7 (rebuff) and 8 (loot) need events and models that land with
// combat (#20), buffs (#22) and loot (#29); rule 3 only flees until
// potions (#23) and heals (#22) exist.
var playRules = []rule{
	{"dead", (*Brain).ruleDead},
	{"prompt", (*Brain).rulePrompt},
	{"critical", (*Brain).ruleCritical},
	{"attacked", (*Brain).ruleAttacked},
	{"rest", (*Brain).ruleRest},
	{"goal", (*Brain).ruleGoal},
	{"idle", (*Brain).ruleIdle},
}

// storeRules replace rules 3-10 in store mode: the bot stays where it is.
var storeRules = []rule{
	{"dead", (*Brain).ruleDead},
	{"prompt", (*Brain).rulePrompt},
	{"store", func(*Brain, time.Time) (Action, bool) { return Action{}, true }},
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
	if b.persona.Store || b.world.Self.StoreType != 0 {
		rules = storeRules
	}
	for _, r := range rules {
		if a, ok := r.run(b, now); ok {
			return r.name, a
		}
	}
	return "", Action{}
}

// ruleDead is rule 1: a dead bot only follows its death policy. A restart
// the server left unanswered is asked again only after restartRetry.
func (b *Brain) ruleDead(now time.Time) (Action, bool) {
	if !b.world.Self.Dead {
		return Action{}, false
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
// the persona; the restart prompt opens only while dead, so rule 1 answers
// it first. The slot is kept for the prompts later milestones add (trade
// request).
func (b *Brain) rulePrompt(now time.Time) (Action, bool) {
	if inv := b.world.Prompts.Party; inv.Pending(now) {
		join := b.persona.JoinParty
		return answerPartyAction(join != nil && join(b.world, inv.Requester, now)), true
	}
	if !b.world.Prompts.Restart.Open {
		return Action{}, false
	}
	if b.persona.Answer != nil {
		return b.persona.Answer(b.world, now), true
	}
	return Action{}, true
}

// ruleCritical is rule 3: in combat with HP critical, flee away from the
// target, standing up first.
func (b *Brain) ruleCritical(now time.Time) (Action, bool) {
	s := &b.world.Self
	if !s.InCombat || !below(s.HP, s.MaxHP, b.persona.CriticalHP) {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	if s.Moving(now) {
		return Action{}, true
	}
	from := s.Position(now)
	angle := b.rng.Float64() * 2 * math.Pi
	if o := b.world.object(b.world.Target); o != nil {
		if t := o.Position(now); t.X != from.X || t.Y != from.Y {
			angle = math.Atan2(float64(from.Y-t.Y), float64(from.X-t.X))
		}
	}
	return moveAction(offset(from, angle, b.persona.FleeDistance), from), true
}

// ruleAttacked is rule 5: in combat, stand up and attack the selected
// monster once (the server repeats the swing). Players are left to the PvP
// defaults of #20.
func (b *Brain) ruleAttacked(now time.Time) (Action, bool) {
	s := &b.world.Self
	if !s.InCombat {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	id := b.world.Target
	if o := b.world.object(id); o != nil && o.Kind == KindNPC && o.Attackable && !o.Dead && b.engaged != id {
		return attackAction(id, s.Position(now)), true
	}
	return Action{}, true
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
