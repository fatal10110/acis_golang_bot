package botclient

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Brain timing defaults and fixed waits.
const (
	defaultTick       = time.Second
	defaultTickJitter = 500 * time.Millisecond
	defaultReactMin   = 300 * time.Millisecond
	defaultReactMax   = 1200 * time.Millisecond
	// defaultPvPReactMin and defaultPvPReactMax bound the reaction to a PvP
	// event (D35).
	defaultPvPReactMin   = 200 * time.Millisecond
	defaultPvPReactMax   = 600 * time.Millisecond
	defaultLogoutTimeout = 15 * time.Second
	// logoutRetry is how long a bot whose Logout was refused (in combat)
	// plays on before it asks again.
	logoutRetry = 10 * time.Second
	// landingTimeout bounds the wait for the landing an Appearing asks:
	// the server answers it on the player's queue at once, so a landing
	// not shown by then is not coming and the bot decides without it.
	landingTimeout = 5 * time.Second
	// restartRetry is how long a dead bot waits for the answer to its
	// restart request before it asks again: a refused restart answers
	// nothing, and a siege attacker's request waits out the attacker
	// respawn delay, dropping repeats meanwhile.
	restartRetry = 10 * time.Second
	// toggleSettle is how long a sit/stand toggle waits for its
	// ChangeWaitType before the brain may toggle again.
	toggleSettle = 2 * time.Second
	// verdictQueue is how many database verdicts on the bot's commands may
	// wait for the loop; one past it is dropped.
	verdictQueue = 4
)

var (
	// errLeftWorld reports a LeaveWorld the bot did not ask for.
	errLeftWorld = errors.New("server took the character out of the world")
	// errLogoutTimeout reports a Logout the server never answered.
	errLogoutTimeout = errors.New("no answer to Logout")
	// errLoggedOut ends the loop after the LeaveWorld a Logout asked for.
	errLoggedOut = errors.New("logged out")
)

// Sender writes one game packet. *GameConn is the real one; the brain is
// its only writer while Run runs.
type Sender interface {
	Send(payload []byte) error
}

// BrainConfig configures a Brain. Zero durations take the defaults; a nil
// Rand is seeded at random.
type BrainConfig struct {
	Persona Persona
	// Metrics, when set, counts what the brain does, and the session loop
	// of a Bot counts its reconnects and server aborts there; one per bot,
	// shared by its sessions. Nil counts nothing.
	Metrics *botmetrics.Bot
	// Tick is the base time between decisions; each adds a uniform
	// jitter up to TickJitter.
	Tick, TickJitter time.Duration
	// ReactMin and ReactMax bound the uniform reaction delay between an
	// event about the bot and the decision that acts on it.
	ReactMin, ReactMax time.Duration
	// PvPReactMin and PvPReactMax bound it instead for a PvP event
	// (Brain.pvpEvent; D35), which moves a reaction armed later earlier.
	PvPReactMin, PvPReactMax time.Duration
	// LogoutTimeout is how long a Logout waits for an answer.
	LogoutTimeout time.Duration
	// LogoutLimit, when set, bounds a requested logout in all, refusals
	// included: past it Run returns errLogoutTimeout. Zero plays on until
	// the server lets the bot out.
	LogoutLimit time.Duration
	Rand        *rand.Rand
	// Spots, when set, are the farming spots the bot picks from (rule
	// "spot"), shared by every bot of the process. Nil farms nowhere.
	Spots *SpotRegistry
	// Travel, when set, is the travel map the bot finds gatekeepers and
	// NPC spawns and merchants on (rules "travel", "newbie" and "shop"),
	// shared by every bot of
	// the process. Nil travels nowhere and visits only NPCs in sight.
	Travel *TravelMap
	// ArenaFirst, when above 0, is how long after the brain first looks
	// at an arena trip (PvPPolicy.Arenas) the first one falls due, in
	// place of arenaEvery give or take arenaJitter; later trips keep that
	// timer. Zero keeps the timer for the first trip too: it is the live
	// arena scenario's seam (TestLivePvPArena), which cannot wait over an
	// hour, and no preset sets it.
	ArenaFirst time.Duration
	// Stores, when set, are the towns a merchant opens its store in and
	// the bot stores open there (rule "store"), shared by every bot of the
	// process. Nil opens no store.
	Stores *StoreRegistry
	// Ads, when set, is the ad board a merchant's valuables round claims
	// its Trade lines on (docs/valuables/ads.md "Ad board"), shared by every
	// bot of the process. Nil says no ad.
	Ads *AdBoard
	// Authority, when set, decides who may command the bot by whisper,
	// shared by every bot of the process. Nil obeys nobody.
	Authority *Authority
	// PartyClaims, when set, are the party leaders the bots of the process
	// joined (PartyClaims.Apply), shared by every bot of the process: the
	// brain gives its claim back when its party ends or its leader changes,
	// and when it stops. Nil claims no leader.
	PartyClaims *PartyClaims
	// Ground, when set, are the ground items the bots of the process are
	// after (GroundClaims), shared by every bot of the process, so no two
	// chase one item. Nil leaves that to the server.
	Ground *GroundClaims
	// Outcomes, when set, is the queue the brain hands a record of each
	// spot stay to (Outcome, OP0), shared by every bot of the process and
	// written by the command that opened the log. Nil records nothing.
	Outcomes *OutcomeQueue
	// Challenges, when set, are the challenge holds of the process
	// (ChallengeHolds), shared by every bot of the process, so one player
	// gets at most one own challenge per challengeSameEvery from all of
	// them and a decline holds them all. Nil holds per bot only.
	Challenges *ChallengeHolds
	// Nav, when set, is the navigation data the bot walks on, shared by
	// every bot of the process (D13). Nil walks nowhere: only the
	// non-walking personas run without it (D13a).
	Nav *Nav
	// CraftBoard, when set, is the crafting order board of the process
	// (warehouse lease, orders, ledger), shared by every bot of the process:
	// the brain reads it on its tick and gives back what it holds when it
	// stops. Nil crafts and orders nothing.
	CraftBoard *CraftBoard
	// Branch, when set, pins the first profession's class id the bot takes
	// while a base class (BranchChoice.Pin, D22); one the branch tables do
	// not name for the bot's base class is ignored. Zero draws it from the
	// hash of the account name and role.
	Branch int32

	// account is the bot's account name the branch draw hashes
	// (BranchChoice.Account); a Bot sets it.
	account string
	// markers mark the bots' names: the brain answers no whisper from a
	// name any of them marks. A Bot sets its SessionConfig.Markers; none
	// takes botnames.DefaultMarker.
	markers botnames.Markers
	// chat is the bot's whisper replies for the caps, carried from session
	// to session by its Bot; nil starts an empty one.
	chat *chatLog
	// valuables is the bot's valuables ledger and round, carried from
	// session to session by its Bot; nil starts an empty one.
	valuables *valuables
	// link is the Bot's mailbox, standing order and status board; nil
	// takes no panel command and publishes no status.
	link *botLink
	// log takes the brain's diagnostics, the Bot's logger; nil discards
	// them.
	log *slog.Logger
	// burstSkips counts the EnterWorld burst frames NewWorld skipped, which
	// NewBrain logs and counts (Brain.skipped).
	burstSkips BurstSkips
}

// Brain is one bot's decision loop. Run is its goroutine: it owns the world
// model, drains the session's event queue, decides on a jittered tick and
// writes every packet the bot sends through the pacer.
type Brain struct {
	world   *World
	events  *Events
	out     Sender
	clock   Clock
	rng     *rand.Rand
	persona Persona
	cfg     BrainConfig
	log     *slog.Logger

	// built is the build last compiled, for builtFor once builtOK (build).
	built    Build
	builtFor buildKey
	builtOK  bool

	pace pacer
	// nextTick is the next regular decision; reactAt, when set, replaces it
	// with the reaction to an event.
	nextTick, reactAt time.Time
	// pvp is the PvP world model: opponent records, the duel.
	pvp pvpWorld
	// karma is the death and karma memory (pvp_karma.go).
	karma karmaState
	// anchor is where idle drift stays around: the arrival point
	// (Brain.idleCenter; a town bot's is its town).
	anchor Point
	// engaged is the target the bot already attacked; it is not clicked
	// again until a refusal (ActionFailed), a move or a sit/stand toggle
	// drops the attack, or the target or combat changes.
	engaged int32
	// castAt is the cast asked for and not yet answered by the bot's own
	// MagicSkillUse, zero for none (castTimeout bounds the wait); castSkill
	// is its skill, castObject its target.
	castAt     time.Time
	castSkill  int32
	castObject int32
	// spoilTarget is the monster of the Spoil or Sweeper cast last sent
	// (castSkill), 0 once the server answered it or another cast went
	// out: what a spoil or sweep system message is about (spoilEvent).
	// spoils is what the bot knows of the spoil of each monster it cast
	// Spoil on, until the monster dies, its sweep ends or it leaves sight.
	spoilTarget int32
	spoils      map[int32]spoilMark
	// sweepHeld is when the server's refusal of the last Sweeper cast
	// (castRefused) stops holding the sweep: until then ruleSweep yields
	// so the bot rests or fights.
	sweepHeld time.Time
	// opened is the last target an attack of the bot went out on: a melee
	// click or an attack step's cast (Action.opens). An Opener step casts
	// only on another (stepState.fresh).
	opened int32
	// once is the Once steps' record (onceRecord): the skills the server
	// took a cast of on the bot's target.
	once onceRecord
	// side is a step cast on another attacker than the fight's (Dryad Root,
	// TargetOtherAttacker) whose select went out and whose cast is due once
	// it is the bot's target; sided is the last attacker one went to, which
	// no other side cast goes to again (otherCast in rotation.go).
	side  sideCast
	sided int32
	// skillToggledAt is when a cast of each toggle skill last went out
	// (toggleSettling).
	skillToggledAt map[int32]time.Time
	// progressAt is the last sign an attack is getting somewhere: an
	// attempt sent, a swing or cast of the bot's own, a move toward the
	// target. Engaged and still past stuckAfter, the bot side steps.
	progressAt time.Time
	// tries counts attacks and casts sent per target since one of the
	// bot's own swings or casts last landed on it; blacklist holds the
	// targets given up on until a time (Persona.BlacklistAfter).
	tries     map[int32]int
	blacklist holds[int32]
	// resting is set while rule 6 has the bot sit until recovered.
	resting bool
	// toggledAt is the last sit/stand toggle not yet confirmed.
	toggledAt time.Time
	// appearing is set from the bot's own loading-screen teleport until its
	// Appearing goes out: the server keeps the character out of the world
	// until then.
	appearing bool
	// appearedAt is when the last Appearing went out: the server shows the
	// objects around the landing point only after it.
	appearedAt time.Time
	// landingUntil is set from an Appearing sent until the server shows the
	// landing, zero otherwise: no decision is made before, as the world
	// holds nothing around the landing point yet (#199). The server shows
	// the objects there, then sends UserInfo (network/client_ingame.go
	// handleAppearing); landingUntil bounds the wait for it.
	landingUntil time.Time
	// restartedAt is the restart request or resurrection answer not yet
	// answered by a Revive.
	restartedAt time.Time
	// lead is the party leader the bot follows and its loss (rule
	// "leader").
	lead leaderWatch
	// partyLead is the lead bot's invitations and oustings and the join
	// bot's handover (rule "party").
	partyLead partyLead
	// rooms is the bot's party-matching volunteering (rule "rooms").
	rooms roomVolunteer
	// partyAnswered is set when the bot sends RequestAnswerJoinParty and
	// cleared on each party window after it (slotPartyEvent): a window
	// with it clear reached the bot by no invitation of its own.
	partyAnswered bool
	// escape is the breaker of every /unstuck the bot sends (the walker's
	// and rule "leader"'s).
	escape escapeBreaker
	// train is the trainer visit (rule 9 errand).
	train trainerVisit
	// spot is the farming spot (rule 9 errand).
	spot spotVisit
	// stay is the spot stay the outcome log records (cfg.Outcomes).
	stay spotStay
	// assist is the creature the party leader attacked (rule 9 assist).
	assist assistPlan
	// buffedAt is when the bot last cast each buff on each target
	// (buffRetry).
	buffedAt map[buffKey]time.Time
	// follow is the slot behind the party leader (rule 9 errand).
	follow followState
	// fleeing is the run away from an attacker under way (flee).
	fleeing fleeRun
	// newbie is the Newbie Guide visit and trip the gatekeeper trip (rule
	// 9 errands).
	newbie newbieVisit
	trip   travelTrip
	// class is the class master visit (rule 9 errand "classchange").
	class classVisit
	// dyes is the Symbol Maker visit (rule 9 errand "dyes").
	dyes dyeVisit
	// ench is the enchant rule's exchange (ruleEnchant).
	ench enchantState
	// supplyTrip is the farmer's trip home to restock (rule "supply").
	supplyTrip supplyTrip
	// prep is the town preparation gate.
	prep townPrep
	// rota is a town bot's rotation between town and farming spells.
	rota townRota
	// stashDue is set by a supply trip's arrival in town until the
	// half-speed stash is queued or found not needed (queueStash).
	stashDue bool
	// loot is the loot queue (rule 8).
	loot lootQueue
	// shedding is set while rule "overweight" drops what the bot carries:
	// from the immobile band until the load is under the half-speed band.
	shedding bool
	// shop is the merchant visit (rule 9 errand).
	shop shopVisit
	// market is the visit to another player's sell store (rule 9 errand
	// "market").
	market marketVisit
	// meter is what the bot spends of each consumable in combat
	// (Persona.Supply).
	meter useMeter
	// wh is the clan warehouse errand (rule 9 errand "warehouse").
	wh warehouseErrand
	// look is the session's look at its private warehouse (rule
	// "valuables").
	look valuablesLook
	// round is this session's part of the valuables round (rule
	// "valuables"): its keeper visits, walks back and closing.
	round sessionRound
	// held is the pending round's hold on the store (rule "valuables").
	held storeHold
	// learn is the recipe learning errand (rule 9 errand "recipe").
	learn recipeLearn
	// craft is the clan crafter's cycle (rule "craft").
	craft crafterCycle
	// member is the clan member option (Persona.ClanMember, rule
	// "member").
	member memberState
	// store is the merchant's private sell store (rule "store").
	store storeState
	// gear is the gear errand (rule 9 errand "gear").
	gear gearState
	// gearVal is the character model the gear and dye errands last valued
	// gear with (Brain.valuer), nil before the first.
	gearVal *gearModel
	// members are the models of the bot members of each class and level
	// a leader fit spots with (Brain.memberModel), nil before the first.
	members map[memberKey]memberModel
	// usedAt is when the bot last used (UseItem) each item template.
	usedAt map[int32]time.Time
	// shotAsked is the last auto-use request of each shot template.
	shotAsked map[int32]shotAsk
	// shotOff are the shot templates the bot itself turned auto use off
	// for, until the server reports them on again (ExAutoSoulShot), so a
	// request to turn one on again that the server dropped is repeated.
	shotOff map[int32]bool
	// tradedAt is when the bot last accepted each partner's trade request,
	// by partner object id (TradePolicy.Every).
	tradedAt map[int32]time.Time
	logout   logoutState
	// orders are the GM commands in force; verdicts brings the answers to
	// commands the authority checks in the database.
	orders   orders
	verdicts chan verdict
	// mail is the Bot's mailbox (BrainConfig.link), nil for none.
	mail <-chan mail
	// inspect are the panel's inspects waiting for their answer.
	inspect inspectState
	// orderPending is set while the standing follow or assist waits for
	// its player to come in sight (resolveOrder).
	orderPending bool
	// towns are the travel map's towns, for the status (nearestTown).
	towns []Point
	// chat is the whisper reply waiting for its time (rule "chat").
	chat chatReply
	// whispers are the valuables responder's replies waiting for their
	// time (rule "chat", whisperHeard).
	whispers whisperQueue
	// valuablesSold are the session's valuable sales in adena, counted
	// against ValuablePolicy.Budget (docs/valuables/release.md "Valuables
	// budget (D34)"): the trades' so far (valuableSaleEvent).
	valuablesSold int64
	// announce are the craft order lines still to say, in order (rule
	// "announce"): orders' answers and finished GM orders' clan chat lines,
	// at most craftCommandCap.
	announce []Action
	// lastRule is the rule that matched the latest decision.
	lastRule string
	// metrics is written only by this loop (BrainConfig.Metrics).
	metrics *botmetrics.Bot
	// decodeLogged holds the opcodes whose undecodable frames the session
	// already logged (skipped); bounded by the decoder tables.
	decodeLogged map[opcodeKey]bool
	// walk walks the bot on checked segments (BrainConfig.Nav).
	walk walker
	// engage is the stand point the fight walks to (engageWalk).
	engage engageState
	// unseen is the last cast the server refused for sight (sightWalk).
	unseen unseenState
	// doors reports the doors the bot sees to the door table, nil without
	// one.
	doors *DoorReporter
}

// logoutState is a logout run inside the loop.
type logoutState struct {
	// requested is set once the session or a GM command asked to log out,
	// commanded once a GM command did.
	requested, commanded bool
	// pending is set while a Logout waits for its answer until deadline.
	pending  bool
	refused  bool
	deadline time.Time
	// retryAt is when the next Logout may go out.
	retryAt time.Time
	// giveUpAt, when set, ends the loop with errLogoutTimeout
	// (BrainConfig.LogoutLimit).
	giveUpAt time.Time
}

// NewBrain returns the brain of a bot in world w, fed by q and writing to
// out. Run starts it.
func NewBrain(w *World, q *Events, out Sender, clock Clock, cfg BrainConfig) *Brain {
	cfg.Tick = cmp.Or(cfg.Tick, defaultTick)
	cfg.TickJitter = cmp.Or(cfg.TickJitter, defaultTickJitter)
	cfg.ReactMin = cmp.Or(cfg.ReactMin, defaultReactMin)
	cfg.ReactMax = max(cmp.Or(cfg.ReactMax, defaultReactMax), cfg.ReactMin)
	cfg.PvPReactMin = cmp.Or(cfg.PvPReactMin, defaultPvPReactMin)
	cfg.PvPReactMax = max(cmp.Or(cfg.PvPReactMax, defaultPvPReactMax), cfg.PvPReactMin)
	cfg.LogoutTimeout = cmp.Or(cfg.LogoutTimeout, defaultLogoutTimeout)
	if len(cfg.markers) == 0 {
		cfg.markers = botnames.Markers{botnames.DefaultMarker}
	}
	if cfg.chat == nil {
		cfg.chat = newChatLog()
	}
	if cfg.valuables == nil {
		cfg.valuables = newValuables()
	}
	if cfg.log == nil {
		cfg.log = slog.New(slog.DiscardHandler)
	}
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	b := &Brain{
		world: w, events: q, out: out, clock: clock, rng: rng, persona: cfg.Persona, cfg: cfg, log: cfg.log, metrics: cfg.Metrics,
		tries: make(map[int32]int), blacklist: make(holds[int32]),
		usedAt: make(map[int32]time.Time), shotAsked: make(map[int32]shotAsk), shotOff: make(map[int32]bool),
		buffedAt: make(map[buffKey]time.Time), tradedAt: make(map[int32]time.Time),
		verdicts: make(chan verdict, verdictQueue),
		spot:     spotVisit{avoid: make(holds[int])}, trip: travelTrip{shunned: make(holds[int])},
		shop: shopVisit{shunned: make(holds[int32])}, newbie: newbieVisit{shunned: make(holds[int32])},
		train:  trainerVisit{shunned: make(holds[int32]), walkShunned: make(holds[int32])},
		market: marketVisit{cooled: make(holds[int32])}, wh: warehouseErrand{shunned: make(holds[int32])},
		partyLead: partyLead{heard: make(holds[string]), retry: make(holds[string])},
		rooms:     roomVolunteer{retry: make(holds[string])},
		pvp:       pvpWorld{find: findState{held: make(holds[string])}}, karma: karmaState{avoid: make(holds[string])},
	}
	b.wireValuables()
	if cfg.link != nil {
		b.mail = cfg.link.mail
		b.craft.resumeFrom(cfg.link.crafter)
	}
	if cfg.Persona.ClanMember {
		b.wireMember()
	}
	b.walk = newWalker(cfg.Nav)
	if cfg.Nav != nil {
		w.SetGround(cfg.Nav.Engine)
		w.SetWater(cfg.Nav.Water)
		if cfg.Nav.Doors != nil {
			b.doors = NewDoorReporter(cfg.Nav.Doors)
		}
	}
	for k, s := range cfg.burstSkips {
		b.skipped(k, s.length, s.count, "burst", s.err)
	}
	return b
}

// Run plays until done closes or the session ends. Closing logout asks the
// brain to log out: it sends Logout and waits for the answer inside the
// loop, playing on and asking again every logoutRetry while the server
// refuses (ActionFailed with no LeaveWorld within logoutRefusalGrace).
//
// Run returns nil after done closes or after the LeaveWorld that answers a
// Logout, ErrCommandedLogout when a GM command asked for that Logout. A
// LeaveWorld it did not ask for, ServerClose, the connection
// ending (the session queues connLost), a Logout left unanswered and a
// failed send are errors. The session closes the
// connection and joins its reader afterwards in every case.
func (b *Brain) Run(done, logout <-chan struct{}) error {
	b.start(b.clock.Now())
	// An inspect still waiting when the session ends gets the model as
	// it stands.
	defer func() { b.answerInspect(b.clock.Now(), true) }()
	defer func() {
		for _, r := range playRules {
			if r.stop != nil {
				r.stop(b)
			}
		}
	}()
	due := b.due()
	timer := b.clock.NewTimer(due.Sub(b.clock.Now()))
	defer timer.Stop()
	batch := make([]Event, 0, 64)
	for {
		var err error
		select {
		case <-done:
			return nil
		case <-logout:
			logout = nil
			b.requestLogout(b.clock.Now())
		case <-b.events.Ready():
			batch = b.events.Drain(batch[:0])
			b.metrics.Drained(len(batch))
			err = b.handle(batch, b.clock.Now())
			clear(batch)
		case v := <-b.verdicts:
			b.obey(v, b.clock.Now())
		case m := <-b.mail:
			b.command(m, b.clock.Now())
		case <-timer.C():
			now := b.clock.Now()
			b.metrics.WakeLag(now.Sub(due))
			err = b.wake(now)
		}
		if errors.Is(err, errLoggedOut) {
			return nil
		}
		if err != nil {
			return err
		}
		due = b.due()
		timer.Reset(max(due.Sub(b.clock.Now()), 0))
	}
}

// start readies the brain to play from now.
func (b *Brain) start(now time.Time) {
	b.pace = newPacer(now)
	b.nextTick = now.Add(b.tickDelay())
	b.anchor = b.world.Self.Position(now)
	// A fresh login prepares before it leaves for a spot.
	b.armPrep()
	b.applyStanding(now)
	b.publishStatus(now, true)
}

// handle applies a batch of events received at now. The first event about
// the bot arms the reaction delay; later ones fold into that decision.
func (b *Brain) handle(batch []Event, now time.Time) error {
	self := b.world.Self.ObjectID
	for _, ev := range batch {
		if e, ok := ev.(decodeSkipped); ok {
			b.skipped(e.key, e.length, 1, "world", e.err)
			continue
		}
		b.meterEvent(ev, now)
		b.observeWalk(ev, now)
		if b.doors != nil {
			b.doors.Observe(ev)
		}
		inParty := b.world.Party.In()
		b.partyLeadEvent(ev, now)
		// Before Apply: SendTradeDone clears the trade it counts.
		b.valuableSaleEvent(ev)
		// Before Apply: the bot's own Die clears the attackers table its
		// killer is read from.
		b.karmaEvent(ev, now)
		// Before Apply: the leader's jump is read from where it stood.
		b.partyTripEvent(ev, now)
		b.world.Apply(ev, now)
		pvpAt := b.pvpEvent(ev, now)
		b.meter.fighting = b.world.Self.InCombat
		b.leaderEvent(ev, now)
		b.slotPartyEvent(ev, now)
		b.spotJoinEvent(ev, inParty)
		b.partyClaimEvent(ev)
		b.inviteEvent(ev)
		b.roomEvent(ev, now)
		b.reweighSpot()
		// Before the switch: the bot's own Die ends its stay (diedAtSpot).
		b.stayEvent(ev, now)
		switch e := ev.(type) {
		case LeaveWorld:
			switch {
			case b.logout.commanded:
				return ErrCommandedLogout
			case b.logout.requested:
				return errLoggedOut
			}
			return errLeftWorld
		case ServerClose:
			return errServerClose
		case connLost:
			return fmt.Errorf("%w: %w", errConnLost, e.err)
		case ActionFailed:
			// A refused click leaves no attack in flight: rule 5 may click
			// the target again.
			b.engaged = 0
			if l := &b.logout; l.pending && !l.refused {
				l.refused = true
				if grace := now.Add(logoutRefusalGrace); grace.Before(l.deadline) {
					l.deadline = grace
				}
			}
		case SystemMessage:
			// Before castRefused: a refusal of Sweeper counts only while
			// its cast is in flight (spoilEvent).
			b.spoilEvent(ev, now)
			b.castRefused(e.ID, now)
			b.warehouseEvent(ev, now)
			b.recipeEvent(ev, now)
			b.craftEvent(ev, now)
			b.crystallizeEvent(e.ID, now)
			b.dyeRefused(e.ID, now)
			b.lootRefused(e.ID, now)
		case ExAutoSoulShot:
			if e.Enabled {
				delete(b.shotOff, e.TemplateID)
			}
		case MyTargetSelected:
			if e.ObjectID != b.engaged {
				b.engaged = 0
			}
			// Selecting self for a TargetSelf step or a side cast is not a
			// target change: the Once record stays for the enemy.
			if e.ObjectID != self && e.ObjectID != b.once.target {
				b.once = onceRecord{}
			}
		case TargetUnselected:
			if e.ObjectID == self {
				b.engaged = 0
				b.once = onceRecord{}
			}
		case AutoAttackStop:
			if e.ObjectID == self {
				b.engaged = 0
			}
		case ChangeWaitType:
			if e.ObjectID == self {
				b.toggledAt = time.Time{}
				if e.Type == WaitStanding {
					b.store.stoodAt = now
				}
			}
		case TeleportToLocation:
			// A Fast teleport is an in-place correction with no loading
			// screen: the server never takes the character out of the
			// world for it (instant_teleport.go), so it asks no Appearing.
			b.lootEvent(ev, now)
			if e.ObjectID == self {
				b.anchor = e.Pos
				b.appearing = b.appearing || !e.Fast
				// Somewhere else: walk to the spot again.
				b.stayPause(now)
				b.spot.arrived = false
				if !e.Fast {
					// A teleport made right after a landing (one the
					// server held for it) asks its own Appearing at the
					// reaction to it.
					b.landingUntil = time.Time{}
					gk := b.trip.v.active() && b.trip.step == tripGone
					b.arenaTeleported(now, gk)
					b.tripTeleported(now)
					if b.supplyTrip.teleported(now, gk, b.trip.onward) {
						b.townArrival()
					}
				}
			}
		case UserInfo:
			if !b.landingUntil.IsZero() {
				// Landed: the objects around the landing point came
				// first. The decision the landing held is due now, or
				// at the reaction to a teleport read with it.
				b.landingUntil, b.nextTick = time.Time{}, now
			}
		case Revive:
			if e.ObjectID == self {
				b.restartedAt = time.Time{}
				// Back on its feet, mostly in town: it prepares before it
				// leaves for a spot.
				b.armPrep()
			}
		case Attack:
			if e.AttackerID == self {
				for _, h := range e.Hits {
					b.landed(h.TargetID, now)
				}
			}
			// The first hit is the swing's target; a pole's others are
			// splash.
			if len(e.Hits) > 0 {
				b.leaderHit(e.AttackerID, e.Hits[0].TargetID, now)
			}
			b.lootEvent(ev, now)
		case MagicSkillUse:
			if e.CasterID == self {
				b.standCaster(e)
				b.onceCast(e, now)
				b.castAt = time.Time{}
				if e.TargetID == b.unseen.id {
					// The server took a cast at it: it is in sight.
					b.unseen = unseenState{}
				}
				b.landed(e.TargetID, now.Add(e.HitTime))
				b.spoilEvent(ev, now)
			}
			b.leaderHit(e.CasterID, e.TargetID, now)
			b.lootEvent(ev, now)
		case DropItem, SpawnItem, GetItem:
			b.lootEvent(ev, now)
		case Die:
			// A dead target ends the attack in flight: there is nothing
			// to disengage from.
			if e.ObjectID == b.engaged {
				b.engaged = 0
			}
			// The server gives a dead NPC's object id to its respawn
			// (data/manager/npcs_respawn.go): that one is opened afresh.
			if e.ObjectID == b.opened {
				b.opened = 0
			}
			b.forgetOnce(e.ObjectID)
			b.forgetSide(e.ObjectID)
			b.spoilEvent(ev, now)
			// A dead bot frees its spot for another and avoids it for a
			// while; it picks again once back on its feet.
			if e.ObjectID == self {
				b.diedAtSpot(now)
				if b.store.active() {
					b.closeStore(now)
				}
			}
		case DeleteObject:
			b.lootEvent(ev, now)
			delete(b.tries, e.ObjectID)
			delete(b.spoils, e.ObjectID)
			if e.ObjectID == b.opened {
				b.opened = 0
			}
			b.forgetOnce(e.ObjectID)
			b.forgetSide(e.ObjectID)
		case NpcHtmlMessage:
			b.trainEvent(ev, now)
			b.tripPage(e, now)
			b.newbiePage(e)
			b.classPage(e)
			b.dyePage(e)
			b.shopEvent(ev)
			b.warehouseEvent(ev, now)
		case AcquireSkillList, AcquireSkillInfo, AcquireSkillDone:
			b.trainEvent(ev, now)
		case CreatureSay:
			b.heard(e, now)
		case BuyList, SellList, ItemList, InventoryUpdate:
			if _, ok := ev.(ItemList); ok {
				b.answerInspect(now, false)
			}
			b.dyeSettled()
			b.shopEvent(ev)
			b.marketEvent(ev)
			b.warehouseEvent(ev, now)
		case WarehouseDepositList, WarehouseWithdrawList:
			b.warehouseEvent(ev, now)
		case RecipeBookItemList:
			b.recipeEvent(ev, now)
		case RecipeItemMakeInfo:
			b.craftEvent(ev, now)
		case HennaEquipList:
			b.dyeList(e)
		case HennaInfo:
			b.dyeAnswered()
		case ChooseInventoryItem:
			b.enchantChosen(e)
		case EnchantResult:
			b.enchantResult(e, now)
		case PrivateStoreManageListSell:
			b.storeManageList(e)
		case PrivateStoreListSell:
			b.marketEvent(ev)
		}
		switch {
		case !pvpAt.IsZero():
			// A PvP event's reaction moves one armed later earlier,
			// never later (D35).
			if b.reactAt.IsZero() || pvpAt.Before(b.reactAt) {
				b.reactAt = pvpAt
			}
		case b.reactAt.IsZero() && salient(ev, self):
			b.reactAt = now.Add(b.reactDelay())
		}
	}
	return nil
}

// salient reports whether ev is news about the bot it should react to.
func salient(ev Event, self int32) bool {
	switch e := ev.(type) {
	case Die:
		return e.ObjectID == self
	case Revive:
		return e.ObjectID == self
	case StatusUpdate:
		return e.ObjectID == self
	case AutoAttackStart:
		return e.ObjectID == self
	case AutoAttackStop:
		return e.ObjectID == self
	case TargetUnselected:
		return e.ObjectID == self
	case TeleportToLocation:
		return e.ObjectID == self
	case MyTargetSelected, AskJoinParty, AskJoinPledge, ConfirmDlg, NpcHtmlMessage, AcquireSkillList, AcquireSkillInfo, AcquireSkillDone, BuyList, SellList,
		PrivateStoreManageListSell, HennaEquipList:
		return true
	case SendTradeRequest, TradeStart, TradeOwnAdd, TradeOtherAdd, TradePressOtherOk:
		return true
	case Attack:
		return e.AttackerID != self && slices.ContainsFunc(e.Hits, func(h AttackHit) bool { return h.TargetID == self })
	case MagicSkillUse:
		return e.CasterID != self && e.TargetID == self
	case RelationChanged:
		// A player flagging or unflagging may change whom the bot fights.
		return true
	}
	return false
}

// requestLogout asks the loop to log out at its next wake.
func (b *Brain) requestLogout(now time.Time) {
	b.logout.requested = true
	b.logout.retryAt = now
	if b.cfg.LogoutLimit > 0 {
		b.logout.giveUpAt = now.Add(b.cfg.LogoutLimit)
	}
}

// wake runs whatever is due at now: the logout steps, a decision, the
// keepalive. While a Logout waits for its answer the bot makes no decision.
// After a loading-screen teleport the decision is the reaction to it and
// Appearing goes out first; until it is out, the bot sends nothing else, and
// the next decision waits for the landing it asks.
func (b *Brain) wake(now time.Time) error {
	if err := b.inspectWake(now); err != nil {
		return err
	}
	b.interruptsDue(now)
	l := &b.logout
	if !l.giveUpAt.IsZero() && !now.Before(l.giveUpAt) {
		return errLogoutTimeout
	}
	if l.pending {
		if now.Before(l.deadline) {
			return nil
		}
		if !l.refused {
			return errLogoutTimeout
		}
		l.pending, l.refused = false, false
		l.retryAt = now.Add(logoutRetry)
	}
	if l.requested && !now.Before(l.retryAt) {
		sent, err := b.send(Action{kind: actLogout, payload: botwire.EncodeLogout()}, now)
		if err != nil {
			return err
		}
		if !sent {
			l.retryAt = now.Add(time.Second / frameRate)
		} else {
			l.pending, l.deadline = true, now.Add(b.cfg.LogoutTimeout)
			return nil
		}
	}
	if !now.Before(b.decisionAt()) {
		// The alarms', the whisper's and the walk's wake-ups are spent
		// here, before Appearing may postpone the decision, so they cannot
		// keep decisionAt past due.
		for _, a := range b.alarms() {
			a.spend(now)
		}
		b.whispers.spendWake(now)
		b.spendWalkWake(now)
		if b.appearing {
			sent, err := b.send(Action{kind: actAppear, payload: botwire.EncodeAppearing()}, now)
			if err != nil {
				return err
			}
			if !sent {
				b.reactAt = now.Add(time.Second / frameRate)
				return nil
			}
			// The decision waits for the landing: a prompt answered now
			// would find nobody in sight.
			b.reactAt, b.landingUntil = time.Time{}, now.Add(landingTimeout)
			return nil
		}
		// Past here a landing still awaited timed out: the bot decides
		// on what it has.
		b.landingUntil = time.Time{}
		b.reactAt = time.Time{}
		b.nextTick = now.Add(b.tickDelay())
		b.tidyPartyClaim(now)
		b.resolveOrder()
		b.walk.asked = false
		name, a := b.decide(now)
		b.lastRule = name
		b.metrics.Rule(name)
		b.publishStatus(now, false)
		if a.kind != actHold {
			if _, err := b.send(a, now); err != nil {
				return err
			}
		}
		// The decision spent reactAt: an arena opponent's walk still
		// heading for the swing margin wakes the guards again.
		if t := b.edgeCross(now); !t.IsZero() {
			b.wakeBy(t)
		}
	}
	if !b.appearing && now.Sub(b.pace.lastSent) >= keepaliveAfter {
		s := &b.world.Self
		ka := Action{kind: actKeepalive, payload: botwire.EncodeValidatePosition(s.Position(now), s.Heading)}
		if _, err := b.send(ka, now); err != nil {
			return err
		}
	}
	return nil
}

// decisionAt is when the next decision is due: the reaction to an event
// or the regular tick, or the leader's assist, the follow re-path, a
// whisper reply or the walk wake when that comes sooner, and never before a
// landing awaited times out.
func (b *Brain) decisionAt() time.Time {
	t := b.nextTick
	if !b.reactAt.IsZero() {
		t = b.reactAt
	}
	for _, a := range b.alarms() {
		if at, ok := a.due(); ok && at.Before(t) {
			t = at
		}
	}
	if at, ok := b.whispers.wakeAt(); ok && at.Before(t) {
		t = at
	}
	if at, ok := b.walkWake(); ok && at.Before(t) {
		t = at
	}
	if t.Before(b.landingUntil) {
		t = b.landingUntil
	}
	return t
}

// alarms are the wake-ups of the leader's assist, the follow re-path and
// the canned whisper reply, which decisionAt reads and wake spends.
func (b *Brain) alarms() [3]*alarm {
	return [3]*alarm{&b.assist.alarm, &b.follow.alarm, &b.chat.alarm}
}

// due is when the loop must wake next.
func (b *Brain) due() time.Time {
	l := &b.logout
	var t time.Time
	if l.pending {
		t = l.deadline
	} else {
		t = b.decisionAt()
		if l.requested && l.retryAt.Before(t) {
			t = l.retryAt
		}
		if k := b.pace.lastSent.Add(keepaliveAfter); !b.appearing && k.Before(t) {
			t = k
		}
	}
	if !l.giveUpAt.IsZero() && l.giveUpAt.Before(t) {
		t = l.giveUpAt
	}
	if at, ok := b.inspect.due(); ok && at.Before(t) {
		t = at
	}
	return t
}

// send writes a's packet if the pacer lets it go out at now, reporting
// whether it did.
func (b *Brain) send(a Action, now time.Time) (bool, error) {
	g := a.gate()
	if !b.pace.allow(g, now) {
		b.metrics.Paced()
		return false, nil
	}
	if err := b.out.Send(a.payload); err != nil {
		return false, fmt.Errorf("send game packet: %w", err)
	}
	b.pace.sent(g, now)
	b.metrics.Sent()
	b.guardSent(a, now)
	b.fightSent(a, now)
	switch a.kind {
	case actAttack:
		b.engaged, b.opened = a.object, a.object
		b.attempt(a.object, now)
	case actCast:
		b.castAt, b.castSkill, b.castObject = now, a.skill, a.object
		b.spoilTarget = 0
		if a.skill == skillSpoil || a.skill == skillSweeper {
			b.spoilTarget = a.object
		}
		if a.opens {
			b.opened = a.object
		}
		if b.isToggle(a.skill) {
			if b.skillToggledAt == nil {
				b.skillToggledAt = make(map[int32]time.Time)
			}
			b.skillToggledAt[a.skill] = now
		}
		b.attempt(a.object, now)
		b.world.holdSkill(a.skill, now.Add(castTimeout))
		b.buffSent(a, now)
		if a.interrupts {
			b.interruptSent(a.object)
		}
	case actMove:
		// A move replaces the attack intention on the server.
		b.engaged = 0
	case actSitStand:
		// Sitting down drops the attack intention as well.
		b.engaged = 0
		b.toggledAt = now
	case actDuelAnswer:
		// The server takes the challenge with the first answer. An
		// acceptance keeps the challenge, with its challenger's name, for
		// 1930, ExDuelReady or a refusal to move on.
		b.pvp.asked = duelAsk{}
		if d := &b.pvp.duel; d.Phase == DuelAsked && !d.Challenged && !a.yes {
			*d = Duel{Result: d.Result}
		}
	case actDuelStart:
		b.challengeSent(a, now)
	case actAppear:
		b.appearing, b.appearedAt = false, now
	case actRestart:
		b.restartedAt = now
	case actDlgAnswer:
		// The server closes the offer with the first answer; an accepted
		// one revives the bot at once.
		b.world.Prompts.Resurrect = ResurrectOffer{}
		b.restartedAt = now
	case actUserCommand:
		b.escape.asked(now, a.walk)
		if !a.walk {
			b.lead.escapeAt = now
		}
	case actLeaveParty:
		// A party the bot left on its own loses it no leader.
		b.lead = leaderWatch{}
		b.orders.withdraw = false
	case actAnswerParty:
		// The server takes the invitation with the first answer. A
		// withdrawal not sent yet was for a party the bot is no longer in.
		b.world.Prompts.Party = PartyInvite{}
		b.partyAnswered = true
		if b.orders.withdraw {
			b.orders.withdraw, b.lead = false, leaderWatch{}
		}
	case actAnswerClan:
		// The server takes the invitation with the first answer, or lets
		// it lapse.
		b.world.Prompts.Clan = ClanInvite{}
	case actUseItem:
		b.usedAt[a.item] = now
		b.supplyTrip.sent(a, now)
		b.cureSent(a.item, now)
	case actAutoShot:
		b.shotAsked[a.item] = shotAsk{at: now, on: true}
	case actAutoShotOff:
		b.shotAsked[a.item] = shotAsk{at: now}
		b.shotOff[a.item] = true
	case actWalkRun:
		b.follow.runToggledAt = now
	case actChatReply:
		b.chatSent(now)
	case actAnswer, actAnnounce:
		b.announce = b.announce[1:]
	}
	if a.kind == b.orders.reply.kind {
		b.orders.reply = Action{}
	}
	if (a.kind == actMove && a.walk) || a.kind == actTalk {
		b.metrics.NavSegment()
	}
	if a.kind == actMove && a.walk {
		b.meterSegment(now)
	}
	b.walk.sent(a, now)
	b.roomSent(a, now)
	b.tradeSent(a, now)
	b.train.sent(a, now)
	b.trip.sent(a, now)
	b.newbie.sent(a, now, b.world.Self.Level)
	b.class.sent(a, now)
	b.dyes.sent(a, now)
	b.ench.sent(a, now)
	b.shop.sent(a, now)
	b.market.sent(a, now)
	b.wh.sent(a, now)
	b.learn.sent(a, &b.world.Recipes, now)
	b.craftSent(a, now)
	b.gear.sent(a, now)
	b.storeSent(a, now)
	b.loot.sent(a, now)
	return true, nil
}

// between is a uniform random duration in lo-hi, both ends included; lo
// when hi is not above it. It draws from the rng once whatever the bounds,
// so the draws that follow do not depend on them.
func (b *Brain) between(lo, hi time.Duration) time.Duration {
	return lo + time.Duration(b.rng.Int64N(int64(max(hi-lo, 0))+1))
}

func (b *Brain) tickDelay() time.Duration {
	return b.between(b.cfg.Tick, b.cfg.Tick+b.cfg.TickJitter)
}

func (b *Brain) reactDelay() time.Duration {
	return b.between(b.cfg.ReactMin, b.cfg.ReactMax)
}
