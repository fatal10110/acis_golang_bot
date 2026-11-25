// Package botparty is the bot party organizer's planner (docs/parties/organizer.md,
// docs/parties/composition.md): given its state and every candidate bot, Plan returns the next
// state and the lead, join and unparty orders that move bots into parties. It is pure: no
// goroutine, no clock of its own, no wire dependency; the panel's supervisor owns its State and
// carries its orders out (P5b).
package botparty

import (
	"strings"
	"time"
)

// Presets that take part in bot parties (D26): a farmer is Damage and leads, a support joins.
const (
	PresetFarmer  = "farmer"
	PresetSupport = "support"
)

// ModeOff is the registry party mode that keeps a bot out of auto parties and pins.
const ModeOff = "off"

// Why a bot cleared its own slot (botclient.Status.PartyLeft).
const (
	LeftLeft    = "left"
	LeftTimeout = "timeout"
)

// MaxSize is the most bots a party holds (D26).
const MaxSize = 4

// Point is a world position (botclient.Point).
type Point struct{ X, Y, Z int32 }

// dist2 is the squared straight-line distance between a and b.
func dist2(a, b Point) int64 {
	dx, dy, dz := int64(a.X)-int64(b.X), int64(a.Y)-int64(b.Y), int64(a.Z)-int64(b.Z)
	return dx*dx + dy*dy + dz*dz
}

// OrderKind is the kind of an organizer order.
type OrderKind uint8

// The organizer's orders (botclient.CommandLead, CommandJoin, CommandUnparty).
const (
	OrderLead OrderKind = iota + 1
	OrderJoin
	OrderUnparty
)

func (k OrderKind) String() string {
	switch k {
	case OrderLead:
		return "lead"
	case OrderJoin:
		return "join"
	case OrderUnparty:
		return "unparty"
	}
	return ""
}

// Order is one order to one bot: lead Names, join Names (the leader first, then the party's other
// bot members) or unparty (no Names). Names are character names.
type Order struct {
	Account string
	Kind    OrderKind
	Names   []string
}

// Slot is the order as the bot's party slot reads it (botclient.Status.Party): "lead A B",
// "join L A", "unparty".
func (o Order) Slot() string {
	if len(o.Names) == 0 {
		return o.Kind.String()
	}
	return o.Kind.String() + " " + strings.Join(o.Names, " ")
}

// Answer is what became of the last order handed to a run (organizer.md, "Which orders arrived").
type Answer uint8

// The answers. The zero Answer is no order handed, or ErrBusy at Bot.Command.
const (
	AnswerNone Answer = iota
	// AnswerPending: in the mailbox or queued for the next session.
	AnswerPending
	// AnswerBadCommand: Command.valid refused it (ErrBadCommand, a bug).
	AnswerBadCommand
	// AnswerError: answered with an error (ErrStopped, ErrWhispered), not carried out.
	AnswerError
	// AnswerOK: answered nil, the brain set or cleared the slot.
	AnswerOK
)

// Sent is the last order handed to the current run and its answer.
type Sent struct {
	Order  Order
	Answer Answer
}

// Candidate is one registry bot with a live run, as the supervisor reads it this pass. Every
// timer lives in State.
type Candidate struct {
	Account   string
	Character string
	// Preset is the persona preset (D21): PresetFarmer is Damage, PresetSupport Support.
	Preset  string
	ClassID int32
	Level   int32
	// Heals is botclient.Status.Heals: the run's skill list holds its row's heal.
	Heals bool
	// Town is the town of the travel map nearest to Position, nil without one.
	Town     *Point
	Position Point
	HP       int32
	// Online is true while the run is online in the world; a reconnecting run's snapshot is
	// stale.
	Online bool
	// Leader and Members are the live party: the leader's name and the other members' names.
	Leader  string
	Members []string
	// Spot is the spot held or farmed beside, "" for none.
	Spot string
	// Slot is the party slot (botclient.Status.Party), PartyLeft why the bot cleared it itself.
	Slot      string
	PartyLeft string
	// NewRun is a run the supervisor started since the last pass: its Sent is dropped.
	NewRun bool
	Sent   Sent
	// Order is the panel's standing order ("stay", "go Wolves"), "" for none; Whispered is true
	// while a GM's whispered order holds the session.
	Order     string
	Whispered bool
	// Mode is the registry party mode: "" auto, ModeOff. Group and Lead are the registry pin:
	// the pin's name ("" for none) and whether the bot leads it.
	Mode  string
	Group string
	Lead  bool
}

// PartyState is where a party stands.
type PartyState uint8

// The party states.
const (
	// Forming: ordered, the leader's snapshot lists no bot member yet.
	Forming PartyState = iota
	// Active: an online leader snapshot listed a bot member.
	Active
	// Broken: unparty went out to every member this pass; the next pass forgets the party.
	Broken
)

func (s PartyState) String() string {
	switch s {
	case Forming:
		return "forming"
	case Active:
		return "active"
	case Broken:
		return "broken"
	}
	return ""
}

// Member is a bot of a party.
type Member struct {
	Account   string
	Character string
}

// Party is the record of one bot party. A timer is the zero time while its condition fails, and
// the first pass it held while it holds.
type Party struct {
	ID       int
	Leader   Member
	Members  []Member
	FormedAt time.Time
	State    PartyState
	// Since is when the party entered its State.
	Since time.Time
	// Why is the break's cause, "" for none.
	Why string
	// Dissolve is the cause of a break the panel asked for (Dissolve, Form), "" for none: the next
	// pass breaks the party with it.
	Dissolve string
	// Await are the accounts of a party the panel formed that are ordered only once they hold no
	// slot, have no order pending and are online in no party: unparty first when they hold a
	// slot, so the player-led-party rule never clears the new one (docs/parties/panel.md).
	Await map[string]bool
	// Pinned is a pin of the panel; Group its name.
	Pinned bool
	Group  string
	// NoHealerSince: no living healer member online.
	NoHealerSince time.Time
	// NoSpotSince: the leader online in the world with no spot.
	NoSpotSince time.Time
	// LeaderGoneSince: the leader not online in the world, or online with its lead delivered,
	// a bot member online and leading no party or another.
	LeaderGoneSince time.Time
	// GoneSince: a member not online in the world; BehindSince: a member online beyond the
	// party range of its online leader.
	GoneSince   map[string]time.Time
	BehindSince map[string]time.Time
	// Ordered are the accounts this party ordered a lead or a join: only their answers say the
	// bot holds this party's slot.
	Ordered map[string]bool
}

// accounts are the party's accounts, the leader first.
func (pt *Party) accounts() []string {
	out := make([]string, 0, 1+len(pt.Members))
	out = append(out, pt.Leader.Account)
	for _, m := range pt.Members {
		out = append(out, m.Account)
	}
	return out
}

// clone is a deep copy of pt.
func (pt Party) clone() Party {
	pt.Members = append([]Member(nil), pt.Members...)
	pt.GoneSince = cloneMap(pt.GoneSince)
	pt.BehindSince = cloneMap(pt.BehindSince)
	pt.Ordered = cloneMap(pt.Ordered)
	if pt.Await != nil {
		pt.Await = cloneMap(pt.Await)
	}
	return pt
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// State is the organizer's state: returned by one pass, handed to the next.
type State struct {
	Parties []Party
	// Freed is when a bot left a party; it is grouped again Cooldown after.
	Freed map[string]time.Time
	// PairBans is until when two bots (pairKey) may not share a party.
	PairBans map[[2]string]time.Time
	// Unpin are the accounts whose registry pin (PartyGroup, PartyLead) the supervisor clears
	// after this pass: a bot that left its pin, every bot of a dissolved pin.
	Unpin []string
	// NextID is the id of the next party formed.
	NextID int
	// AutoOff is the panel's switch of auto parties off: a pass forms and fills none and breaks
	// every auto party; pins are kept.
	AutoOff bool
}

func (s State) clone() State {
	out := State{
		Parties:  make([]Party, 0, len(s.Parties)),
		Freed:    cloneMap(s.Freed),
		PairBans: cloneMap(s.PairBans),
		NextID:   s.NextID,
		AutoOff:  s.AutoOff,
	}
	for _, pt := range s.Parties {
		out.Parties = append(out.Parties, pt.clone())
	}
	return out
}

// pairKey is the PairBans key of two accounts, in order.
func pairKey(a, b string) [2]string {
	if b < a {
		a, b = b, a
	}
	return [2]string{a, b}
}

// Policy is the organizer's tuning (docs/parties/organizer.md, composition.md).
type Policy struct {
	// Size is the party size grouping fills to, 2 to MaxSize.
	Size int
	// Band is the most levels bot members of a forming party lie apart; MaxSpread the most
	// before a party breaks.
	Band, MaxSpread int32
	// MaxTravel is the straight-line reach of a member from its leader at forming.
	MaxTravel int32
	// Range is the party range a member keeps to its leader.
	Range int32
	// FormPerPass is the most new parties a pass.
	FormPerPass int
	// Cooldown is how long a freed bot waits; RePairAfter how long a pair stays apart after a
	// fall-behind or a no-spot break.
	Cooldown, RePairAfter time.Duration
	// LeaderGone, BehindFor, NoSpotFor and NoHealerFor are the timers' limits.
	LeaderGone, BehindFor, NoSpotFor, NoHealerFor time.Duration
}

// DefaultPolicy is the design's tuning.
func DefaultPolicy() Policy {
	return Policy{
		Size:        3,
		Band:        3,
		MaxSpread:   5,
		MaxTravel:   20000,
		Range:       1500,
		FormPerPass: 2,
		Cooldown:    5 * time.Minute,
		RePairAfter: 30 * time.Minute,
		LeaderGone:  5 * time.Minute,
		BehindFor:   10 * time.Minute,
		NoSpotFor:   15 * time.Minute,
		NoHealerFor: 10 * time.Minute,
	}
}
