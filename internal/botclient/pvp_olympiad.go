package botclient

import (
	"bytes"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The Olympiad's system messages (serverpackets/olympiad_match.go,
// systemmessage_olympiad.go; network/olympiad_manager.go registerOlympiad,
// sendUnregisterResult). S1 is a number for 1492, 1495 and 1499, a name for
// 1497; 1657 and 1658 carry a name, then the points.
const (
	msgStadiumInS1Seconds  = 1492
	msgOpponentEndedGame   = 1493
	msgOpponentUnfit       = 1494
	msgGameStartsInS1      = 1495
	msgGameStarted         = 1496
	msgS1WonTheGame        = 1497
	msgGameEndedInTie      = 1498
	msgS1GainedS2Points    = 1657
	msgS1LostS2Points      = 1658
	msgRegisteredClassed   = 1503
	msgRegisteredNoClass   = 1504
	msgUnregistered        = 1505
	msgNotOnWaitingList    = 1506
	msgOlympiadNotRunning  = 1651
	msgAlreadyOnClassList  = 1689
	msgAlreadyOnNoClass    = 1690
	msgOlympiadNoblesse    = 1674
	msgCantJoinOnSubclass  = 1500
	msgOnlyNoblesJoin      = 1501
	msgAlreadyInEventList  = 1502
	msgInventoryTooFull    = 1691
	msgCantJoinCursed      = 1750
	msgRequestCannotBeMade = 1803
)

// The Olympiad's server facts.
const (
	// olympiadManager is the Grand Olympiad Manager (npcs/31000-31999.xml,
	// type OlympiadManagerNpc), the only NPC taking registrations.
	olympiadManager int32 = 31688
	// olympiadClosing is how long before the competition window's end the
	// server refuses a registration (olympiad/registration.go
	// RegisterClosing, 600000 ms).
	olympiadClosing = 10 * time.Minute
	// olympiadCallLimit bounds the call to a stadium (1492) without the
	// stadium's ExOlympiadMode: the teleport comes about 30 s after the
	// first 1492 (olympiad/stadium.go arenaCountdown), and a match
	// cancelled before it (1493, 1494) sends none.
	olympiadCallLimit = 2 * time.Minute
	// olympiadMatchLimit bounds a match from its call to
	// ExOlympiadMatchEnd: about 30 s to the teleport, 61 s of wait, the
	// fight (OlyBattle, 6 min on the owner's server) and 40 s to the town.
	olympiadMatchLimit = 15 * time.Minute
	// olympiadLinks bounds the links one manager visit follows to the
	// registration (noble_main, Chat 10, Chat 12, OlympiadNoble 5).
	olympiadLinks = 4
)

// The manager's commands (data/html/olympiad): the registration for classed
// games on noble_12.htm, reached from noble_main.htm by Chat 10 and Chat 12,
// and the unregistration on noble_main.htm (folk_olympiad.go
// olympiadNobleCommand; olympiad_manager.go olympiadNobleBypass). The bot
// never sends choice 4 (non-classed, the larger draw), 10 (points for
// passes) or anything at a Monument of Heroes.
const (
	olympiadRegister   = "OlympiadNoble 5"
	olympiadUnregister = "OlympiadNoble 1"
	becomeNobleCommand = "become_noble"
)

// olympiadRegisterPath is the manager's links to the registration, the
// registration first: on each page the visit follows the first it has.
var olympiadRegisterPath = []string{olympiadRegister, "Chat 12", "Chat 10"}

// OlympiadPolicy is how an Olympiad bot (PvPPolicy.Olympiad,
// docs/pvp/olympiad.md, D42) registers. Every value is a configurable
// default (D6); the window mirrors the server's events.properties.
type OlympiadPolicy struct {
	// Managers are the NPC template ids of the Olympiad managers, Reach how
	// far the bot walks to one's spawn (as ClassChangePolicy.Reach).
	Managers []int32
	Reach    int32
	// Start is the competition window's start after midnight in Zone, and
	// Period its length (OlyStartTime 18:00, OlyCPeriod 6 h; the server
	// reads them in its own local time, UTC on the owner's stack).
	Start, Period time.Duration
	Zone          *time.Location
	// Wait is how long a registered bot waits in town for a draw before it
	// unregisters; Tries how many registrations it makes a day.
	Wait  time.Duration
	Tries int
	// ThirdClass lets a third-profession bot play Olympiad; off, it plays it
	// as off. On by default (owner, 2026-11-28, #729, D42): the server
	// registers only nobles of a third profession (folk_olympiad.go
	// olympiadNobleCommand), so with it off no bot registers, though a bot
	// below its third profession still takes noblesse. It has no effect
	// unless PvP.Olympiad is on, which is opt-in per bot. A competing bot
	// still never claims hero status, spectates or trades points.
	ThirdClass bool
}

// defaultOlympiadPolicy registers at a Grand Olympiad Manager nearby inside
// the owner's window, waits 30 minutes for a draw, twice a day at most, and
// lets a third-profession bot compete (#729).
func defaultOlympiadPolicy() OlympiadPolicy {
	return OlympiadPolicy{
		Managers:   []int32{olympiadManager},
		Reach:      8000,
		Start:      18 * time.Hour,
		Period:     6 * time.Hour,
		Zone:       time.UTC,
		Wait:       30 * time.Minute,
		Tries:      2,
		ThirdClass: true,
	}
}

// open reports whether the server takes a registration at now: inside a
// competition window and at least olympiadClosing before its end.
func (p OlympiadPolicy) open(now time.Time) bool {
	zone := p.Zone
	if zone == nil {
		zone = time.UTC
	}
	t := now.In(zone)
	y, m, d := t.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, zone).Add(p.Start)
	// Yesterday's window may run past midnight.
	for _, s := range []time.Time{start, start.AddDate(0, 0, -1)} {
		if !t.Before(s) && t.Before(s.Add(p.Period-olympiadClosing)) {
			return true
		}
	}
	return false
}

// day is the server's calendar day of now, which the daily tries count.
func (p OlympiadPolicy) day(now time.Time) string {
	if p.Zone != nil {
		now = now.In(p.Zone)
	} else {
		now = now.UTC()
	}
	return now.Format(time.DateOnly)
}

// olympiadMatch is the bot's Olympiad match, from the call to a stadium to
// ExOlympiadMatchEnd. The zero olympiadMatch is none.
type olympiadMatch struct {
	// called is when the first 1492 (or the stadium's ExOlympiadMode)
	// came.
	called time.Time
	// side is the bot's side from ExOlympiadMode, 0 before the stadium.
	side uint8
	// started is set from 1496 to the result (1497, 1498) or the release
	// (ExOlympiadMode none): the fight.
	started bool
	// foe is the drawn opponent, by its ExOlympiadUserInfo.
	foe int32
}

// olympiad is the bot's Olympiad state (docs/pvp/olympiad.md), owned by the
// brain goroutine.
type olympiad struct {
	match olympiadMatch
	// day is the server day tries and stopped count for.
	day   string
	tries int
	// stopped is set by a refusal: no registration again that day.
	stopped bool
	// v is the visit to a manager, links the links it followed.
	v     npcVisit
	links int
	// asked is the OlympiadNoble command whose answer is awaited, "" for
	// none.
	asked string
	// registered is when the server put the bot on its waiting list, zero
	// while it is not on it.
	registered time.Time
	// retryAt is when a visit that found no manager may start again.
	retryAt time.Time
	// refusedLogged is set once the third-profession refusal was logged.
	refusedLogged bool
}

// olympiadOn reports whether the bot plays Olympiad: the mode on, and below
// its third profession unless OlympiadPolicy.ThirdClass lets it.
func (b *Brain) olympiadOn() bool {
	if !b.persona.PvP.Olympiad {
		return false
	}
	if classTier(b.world.Self.ClassID) >= 3 && !b.persona.Olympiad.ThirdClass {
		if o := &b.oly; !o.refusedLogged {
			o.refusedLogged = true
			b.log.Info("olympiad refused: a third-profession bot (#729)", "class", b.world.Self.ClassID)
		}
		return false
	}
	return true
}

// nobleDue reports whether the bot takes noblesse on its class master visit:
// Olympiad on, not noble, and not asked yet this run (one become_noble per
// bot: the server's UserInfo shows it noble from then on).
func (b *Brain) nobleDue() bool {
	return b.world.Self.ObjectID != 0 && !b.world.Self.Noble && !b.class.nobleAsked && b.olympiadOn()
}

// olympiadAt is the bot's match at now, dropped once stale: a call with no
// stadium past olympiadCallLimit, a match past olympiadMatchLimit.
func (b *Brain) olympiadAt(now time.Time) *olympiadMatch {
	m := &b.oly.match
	if !m.called.IsZero() {
		age := now.Sub(m.called)
		if m.side == 0 && age > olympiadCallLimit || age > olympiadMatchLimit {
			*m = olympiadMatch{}
		}
	}
	return m
}

// inOlympiad reports whether the bot is in an Olympiad match at now, from
// the call to a stadium to ExOlympiadMatchEnd.
func (b *Brain) inOlympiad(now time.Time) bool { return !b.olympiadAt(now).called.IsZero() }

// olympiadFoe is the drawn opponent the bot may attack at now (MayFight rule
// 0), 0 outside the fight: the server refuses an attack on a competitor
// before 1496 (attackable/playable.go PlayableRefuses) and needs no force
// between the two of a started match (character_social.go
// SameActiveOlympiadMatch).
func (b *Brain) olympiadFoe(now time.Time) int32 {
	m := b.olympiadAt(now)
	if !m.started || m.side == 0 {
		return 0
	}
	return m.foe
}

// olympiadEvent keeps the Olympiad state with ev, received at now, and
// reports when the PvP reaction to it is due.
func (b *Brain) olympiadEvent(ev Event, now time.Time) time.Time {
	m := b.olympiadAt(now)
	switch e := ev.(type) {
	case SystemMessage:
		return b.olympiadMessage(e, now)
	case ExOlympiadMode:
		switch e.Mode {
		case botwire.OlympiadModeSide1, botwire.OlympiadModeSide2:
			if m.called.IsZero() {
				m.called = now
			}
			m.side = e.Mode
			b.olympiadDrawn()
		case botwire.OlympiadModeNone:
			m.started, m.foe = false, 0
		default:
			return time.Time{}
		}
		return b.pvpReact(now)
	case ExOlympiadUserInfo:
		if m.side != 0 && e.ObjectID != b.world.Self.ObjectID {
			m.foe = e.ObjectID
		}
	case ExOlympiadMatchEnd:
		if !m.called.IsZero() {
			*m = olympiadMatch{}
			return b.pvpReact(now)
		}
	case ExAutoSoulShot:
		// The stadium's strip turned the bot's automatic shots off
		// (network/olympiad_match.go stripCompetitor): rule "shots" turns
		// them on again, as after its own toggle.
		if !e.Enabled && m.side != 0 {
			b.shotOff[e.TemplateID] = true
		}
	}
	return time.Time{}
}

// olympiadMessage moves the match and the registration on with system
// message e.
func (b *Brain) olympiadMessage(e SystemMessage, now time.Time) time.Time {
	m, o := b.olympiadAt(now), &b.oly
	switch e.ID {
	case msgStadiumInS1Seconds:
		if m.called.IsZero() {
			m.called = now
			b.olympiadDrawn()
			b.log.Info("olympiad: called to a stadium", "in", e.Number)
		}
		return b.pvpReact(now)
	case msgOpponentEndedGame, msgOpponentUnfit:
		if m.side == 0 {
			*m = olympiadMatch{}
		}
	case msgGameStarted:
		if m.side != 0 {
			m.started = true
			return b.pvpReact(now)
		}
	case msgS1WonTheGame, msgGameEndedInTie:
		m.started, m.foe = false, 0
	case msgS1GainedS2Points, msgS1LostS2Points:
		b.log.Info("olympiad: match over", "won", e.ID == msgS1GainedS2Points, "points", e.Number)
	}
	if o.asked != "" {
		b.olympiadAnswer(e.ID, now)
	}
	return time.Time{}
}

// olympiadDrawn ends the registration of a bot drawn for a match: the
// server took it off its waiting list.
func (b *Brain) olympiadDrawn() {
	o := &b.oly
	o.registered, o.asked, o.v, o.links = time.Time{}, "", npcVisit{}, 0
}

// olympiadAnswer takes system message id as the answer to the awaited
// OlympiadNoble command. A refusal stops the registrations for the day; it
// is logged with its message id. Unregistered, the bot farms for
// OlympiadPolicy.Wait before its next try.
func (b *Brain) olympiadAnswer(id int32, now time.Time) {
	o := &b.oly
	switch o.asked {
	case olympiadRegister:
		switch id {
		case msgRegisteredClassed, msgRegisteredNoClass, msgAlreadyOnClassList, msgAlreadyOnNoClass:
			o.registered = now
			b.log.Info("olympiad: registered", "msg", id)
		case msgOlympiadNotRunning, msgRequestCannotBeMade, msgOnlyNoblesJoin, msgCantJoinOnSubclass,
			msgCantJoinCursed, msgInventoryTooFull, msgAlreadyInEventList:
			o.stopped = true
			b.log.Info("olympiad: registration refused", "msg", id)
		default:
			return
		}
	case olympiadUnregister:
		switch id {
		case msgUnregistered, msgNotOnWaitingList:
		case msgOlympiadNotRunning, msgOlympiadNoblesse:
			b.log.Info("olympiad: unregistration refused", "msg", id)
		default:
			return
		}
		// Off the list it farms a Wait before its next try.
		o.registered, o.retryAt = time.Time{}, now.Add(b.persona.Olympiad.Wait)
	}
	o.asked, o.v, o.links = "", npcVisit{}, 0
}

// olympiadPage moves the manager visit on with page ev. A page answering
// an OlympiadNoble command is a refusal (noble_cant_*.htm, or
// noble_nopoints1.htm for a bot with no points left): no registration
// again that day.
func (b *Brain) olympiadPage(ev NpcHtmlMessage) {
	o := &b.oly
	if !o.v.active() || !o.v.page(ev) || o.asked == "" {
		return
	}
	if o.asked == olympiadRegister {
		o.stopped = true
		b.log.Info("olympiad: registration refused by a page")
	} else {
		o.registered = time.Time{}
	}
	o.asked, o.v, o.links = "", npcVisit{}, 0
}

// olympiadSent records a packet of the manager visit that went out at now:
// an OlympiadNoble command awaits its answer, a registration counts a try.
func (b *Brain) olympiadSent(a Action, now time.Time) {
	o := &b.oly
	if !o.v.active() {
		return
	}
	if a.kind == actBypass && a.object != 0 && a.object == o.v.obj {
		for _, cmd := range []string{olympiadRegister, olympiadUnregister} {
			if bytes.Equal(a.payload, botwire.EncodeRequestBypassToServer(npcBypass(o.v.obj, cmd))) {
				o.asked = cmd
				if cmd == olympiadRegister {
					o.tries++
				}
			}
		}
		o.links++
	}
	o.v.sent(a, now)
}

// mayRegister reports whether the bot registers at now: inside the window
// and olympiadClosing before its end, under its daily tries and not stopped
// that day, and passing every check the server makes (registration.go
// Register, folk_olympiad.go olympiadNobleCommand) the bot can see itself:
// a noble of a third profession on its base class, not overweight (the
// third weight band, or its inventory 80 % full), alive, and in no party
// (the stadium's strip takes it out of its party).
func (b *Brain) mayRegister(now time.Time) bool {
	o, p, s := &b.oly, &b.persona.Olympiad, &b.world.Self
	if o.stopped || o.tries >= p.Tries || now.Before(o.retryAt) || !p.open(now) {
		return false
	}
	full := s.InvLimit > 0 && 5*int32(len(b.world.Inventory.Items)) >= 4*s.InvLimit
	return s.Noble && classTier(s.ClassID) >= 3 && s.ClassID == s.BaseClass && !s.Dead &&
		!b.world.Party.In() && weightBand(s.Load, s.MaxLoad) <= slowBand && !full
}

// olympiadWaiting reports whether the bot waits on the waiting list at now,
// short of OlympiadPolicy.Wait.
func (b *Brain) olympiadWaiting(now time.Time) bool {
	r := b.oly.registered
	return !r.IsZero() && now.Sub(r) < b.persona.Olympiad.Wait
}

// ruleOlympiad is MayFight rule 0's fight (docs/pvp/olympiad.md "In the
// match"): from 1496 to the result the bot fights its drawn opponent with
// its PvP order, plain Action and casts, holding every rule below it: no
// break-off and no flee (leaving the stadium forfeits). Before the fight
// it leaves the decision to the rules below until "olympiad wait".
func (b *Brain) ruleOlympiad(now time.Time) (Action, bool) {
	id := b.olympiadFoe(now)
	switch {
	case id == 0:
		return Action{}, false
	case !b.mayFight(id, now):
		return Action{}, true
	case b.world.Self.Sitting:
		return b.toggle(now), true
	}
	return b.fight(id, now), true
}

// ruleOlympiadWait holds every rule after the rest and buff rules while
// the bot is in a match (its wait in the stadium: rebuffed, its shots on
// again, no walk out) and while it waits on the waiting list in town for
// OlympiadPolicy.Wait: no farming, no trip out of the town, no dungeon,
// which would forfeit a draw.
func (b *Brain) ruleOlympiadWait(now time.Time) (Action, bool) {
	return Action{}, b.inOlympiad(now) || b.olympiadOn() && b.olympiadWaiting(now)
}

// ruleOlympiadEntry is a rule 9 errand (docs/pvp/olympiad.md
// "Registering"): with registration open and every check passed
// (mayRegister) the bot walks to an Olympiad manager, talks to it, follows
// the page's links to the registration for classed games and registers
// once; OlympiadPolicy.Wait later it goes back and unregisters
// (OlympiadNoble 1). The answer is a system message; a refusal, a refusal
// page, a page without the link or no answer within trainerReplyTimeout
// stops the registrations for the day. Bypasses pass the bypass gate
// (gateBypass), at most OlympiadPolicy.Tries registrations and as many
// unregistrations a day; legs and clicks the frame pacer, a visit spending
// at most visitClicks clicks.
func (b *Brain) ruleOlympiadEntry(now time.Time) (Action, bool) {
	o, p := &b.oly, &b.persona.Olympiad
	if !b.olympiadOn() || b.inOlympiad(now) || b.world.Self.InCombat {
		return Action{}, false
	}
	if day := p.day(now); day != o.day {
		o.day, o.tries, o.stopped = day, 0, false
	}
	v := &o.v
	if o.asked != "" {
		if now.Before(v.waitUntil) {
			return Action{}, true
		}
		// Unanswered: the server refused in silence.
		b.log.Info("olympiad: no answer", "command", o.asked)
		if o.asked == olympiadRegister {
			o.stopped = true
		} else {
			o.registered = time.Time{}
		}
		o.asked, o.v, o.links = "", npcVisit{}, 0
		return Action{}, false
	}
	path := olympiadRegisterPath
	switch {
	case !o.registered.IsZero():
		if b.olympiadWaiting(now) {
			return Action{}, false
		}
		path = []string{olympiadUnregister}
	case !b.mayRegister(now):
		o.v, o.links = npcVisit{}, 0
		return Action{}, false
	}
	if !v.active() {
		best, ok := b.pickVisit(p.Managers, p.Reach, now)
		if !ok {
			o.retryAt = now.Add(visitShunFor)
			return Action{}, false
		}
		o.v, o.links = best, 0
	}
	if v.opened {
		for _, cmd := range path {
			if link := npcBypass(v.obj, cmd); o.links < olympiadLinks && b.world.Dialog.Has(link) {
				return bypassTo(v.obj, link), true
			}
		}
		// A page offering neither: the manager takes nothing from the bot.
		b.log.Info("olympiad: the manager's page has no link", "want", path[0])
		if o.registered.IsZero() {
			o.stopped = true
		}
		o.registered, o.v, o.links = time.Time{}, npcVisit{}, 0
		return Action{}, false
	}
	a, ok := b.approach(v, now)
	if !ok {
		o.v, o.links, o.retryAt = npcVisit{}, 0, now.Add(visitShunFor)
		return Action{}, false
	}
	return a, true
}
