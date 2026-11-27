// Package botmetrics counts what the bots of one swarm do and serves the
// totals on a live expvar endpoint bound to loopback (docs/plan.md, "7.
// Metrics").
//
// Every bot owns one Bot for as long as it exists, across its sessions; a
// deleted bot's Bot is folded into the Registry's retained total
// (Registry.Remove), so the sums never fall.
// Only that bot's goroutines write it, through per-bot atomics nobody else
// writes, so no swarm-wide atomic or mutex sits on a per-frame or per-action
// path. One Registry.Run goroutine sums every Bot on a period into a
// Snapshot, which the endpoint serves. Histograms are fixed arrays: the
// metrics do not grow with the run.
//
// Metrics carry no account names and no secrets: bots are only counted.
package botmetrics

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// otherRule names the rule hits of a name the registry was not given.
const otherRule = "other"

// Registry holds the Bots of one swarm and their latest sum.
type Registry struct {
	// rules and ruleIndex are read-only after NewRegistry; the last index
	// of rules is otherRule.
	rules     []string
	ruleIndex map[string]int

	breakerOpen atomic.Bool
	// acceptClosed counts the login gate's dials closed at accept.
	acceptClosed atomic.Uint64

	mu   sync.Mutex
	bots []*Bot // guarded by mu
	// retained is the counters of the removed Bots (Remove), shaped like
	// one Bot with no account and its held gauge always zero; guarded by
	// mu.
	retained Bot
	// partiesFormed and partiesBroken count the organizer's bot
	// parties (docs/parties/panel.md, "Metrics"); guarded by mu.
	partiesFormed uint64
	partiesBroken map[string]uint64

	last atomic.Pointer[Snapshot]
	// navCache reads the nav cache's counts (SetNavCache); nil: none.
	navCache atomic.Pointer[func() NavCache]
}

// NewRegistry returns a registry that counts hits of the named decision
// rules; a hit of any other name counts as "other".
func NewRegistry(rules ...string) *Registry {
	r := &Registry{ruleIndex: make(map[string]int, len(rules))}
	for _, name := range rules {
		if _, ok := r.ruleIndex[name]; ok || name == otherRule || name == "" {
			continue
		}
		r.ruleIndex[name] = len(r.rules)
		r.rules = append(r.rules, name)
	}
	r.rules = append(r.rules, otherRule)
	r.retained.rules = make([]atomic.Uint64, len(r.rules))
	return r
}

// NewBot adds one bot's counters to the registry. Call it once per bot, not
// per session, and Remove it when the bot is deleted: the live bots, which
// the swarm's fixed bot cap bounds, bound the registry.
func (r *Registry) NewBot() *Bot {
	b := &Bot{reg: r, rules: make([]atomic.Uint64, len(r.rules))}
	r.mu.Lock()
	r.bots = append(r.bots, b)
	r.mu.Unlock()
	return b
}

// Remove folds b's counters into the registry's retained total and drops b,
// so no sum of Collect falls and the registry holds only the live bots;
// Snapshot.Bots falls by one and Snapshot.ValuablesHeld by b's gauge, whose
// units go away with the bot. Call it once b records nothing more: after
// the run that recorded into it ended, since a count recorded later is
// lost. A nil b, or one the registry does not hold (removed before), is
// ignored, so a removal never counts twice.
func (r *Registry) Remove(b *Bot) {
	if b == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.Index(r.bots, b)
	if i < 0 {
		return
	}
	r.retained.fold(b)
	r.bots = slices.Delete(r.bots, i, i+1)
}

// SetBreaker records the state of the process-wide credential breaker: open
// once it has halted every login.
func (r *Registry) SetBreaker(open bool) { r.breakerOpen.Store(open) }

// AcceptClosed counts a login or game dial the server closed at accept: each
// starts one quiet period of the process's login gate.
func (r *Registry) AcceptClosed() { r.acceptClosed.Add(1) }

// PartyFormed counts a bot party the organizer formed.
func (r *Registry) PartyFormed() {
	r.mu.Lock()
	r.partiesFormed++
	r.mu.Unlock()
}

// PartyBroken counts a bot party the organizer broke for why, one of the
// organizer's fixed reasons (botparty.Why*), so the keys stay few.
func (r *Registry) PartyBroken(why string) {
	r.mu.Lock()
	if r.partiesBroken == nil {
		r.partiesBroken = make(map[string]uint64)
	}
	r.partiesBroken[why]++
	r.mu.Unlock()
}

// Snapshot is the sum of every Bot at one collection, the removed ones
// included (Registry.Remove). Counters only grow;
// QueueDepth counts the events the brain drained at each wakeup, and
// WakeLagMicros how late each timed wakeup ran after it was due.
type Snapshot struct {
	At time.Time `json:"at"`
	// Bots counts the live slots: it falls when a bot is removed.
	Bots       int    `json:"bots"`
	Events     uint64 `json:"events"`
	Sent       uint64 `json:"sent"`
	Paced      uint64 `json:"paced"`
	Reconnects uint64 `json:"reconnects"`
	Aborts     uint64 `json:"aborts"`
	// Panics counts the panics the bots recovered from: in an attempt or
	// in a session's reader (botclient.ErrPanic).
	Panics uint64 `json:"panics_total"`
	// DecodeErrors counts the server packets the bots could not decode, by
	// opcode, only the opcodes seen (Bot.DecodeErrors): a frame in the
	// world or in the EnterWorld burst the bot skipped, or a packet before
	// the world that ended its attempt (docs/plan.md D45).
	DecodeErrors map[string]uint64 `json:"decode_errors_total"`
	// NavBlocked, NavDesync and NavDoorReplan count the server's answers
	// to the walker's own segments that differ from its check: a blocked
	// stop, another destination, and another destination on a segment
	// across a door (docs/navigation.md, "Server answers to its own moves").
	NavBlocked    uint64 `json:"nav_blocked_total"`
	NavDesync     uint64 `json:"nav_desync_total"`
	NavDoorReplan uint64 `json:"nav_door_replan_total"`
	// NavSegments counts the moves a blocked stop can end: the walker's
	// segments and its talk clicks, the base of the blocked rate
	// (docs/navigation.md, Testing).
	NavSegments uint64 `json:"nav_segments_total"`
	BreakerOpen bool   `json:"breaker_open"`
	// AcceptClosed counts the dials the server closed at accept (a ban,
	// or a server that failed before its first write).
	AcceptClosed  uint64            `json:"login_accept_closed_total"`
	Rules         map[string]uint64 `json:"rules"`
	QueueDepth    HistogramSnapshot `json:"queue_depth"`
	WakeLagMicros HistogramSnapshot `json:"wake_lag_us"`
	// NavWalkIdleMillis is how long each walker segment went out after the
	// previous segment's modeled arrival, in milliseconds; NavWalkEfficiency
	// is each arrived walk's route length over its elapsed time times the
	// run speed, in per mille (docs/walking.md, "Measurement (#304)").
	NavWalkIdleMillis HistogramSnapshot `json:"nav_walk_idle_ms"`
	NavWalkEfficiency HistogramSnapshot `json:"nav_walk_efficiency_permille"`
	// NavPlans counts the walker's route plans by outcome (the PlanOutcome
	// names, every one present), and NavPlanMicros is how long each took, in
	// microseconds (docs/walking.md, "Plan metrics (#497)").
	NavPlans      map[string]uint64 `json:"nav_plans_total"`
	NavPlanMicros HistogramSnapshot `json:"nav_plan_us"`
	// NavLocates counts the nav hierarchy's Locates by how they were
	// answered (the Locate* names, every one present); NavLRUBytes is the
	// bytes its cluster result LRU holds and NavPlaces the cells its place
	// table holds (docs/navigation.md, "Cache"; #496).
	NavLocates  map[string]uint64 `json:"nav_locate_total"`
	NavLRUBytes int64             `json:"nav_lru_bytes"`
	NavPlaces   int               `json:"nav_places"`
	// The merchants' valuables (docs/valuables/preset.md "Wiring"): the
	// Trade ad lines, the whisper replies, the sales by kind (ValuablesSold
	// keyed by via, "trade" and "store"), the private deposits and the
	// entries released to the store. ValuablesHeld is a gauge: the units
	// the ledgers hold now, carried and stored.
	ValuablesAds      uint64            `json:"valuables_ads_total"`
	ValuablesReplies  uint64            `json:"valuables_replies_total"`
	ValuablesSold     map[string]uint64 `json:"valuables_sold_total"`
	ValuablesDeposits uint64            `json:"valuables_deposits_total"`
	ValuablesReleased uint64            `json:"valuables_released_total"`
	ValuablesHeld     int64             `json:"valuables_held"`
	// PartiesFormed counts the bot parties the organizer (panel or swarm) formed,
	// by hand or by itself, and PartiesBroken the ones it broke, keyed by
	// the reason (docs/parties/panel.md, "Metrics").
	PartiesFormed uint64            `json:"parties_formed_total"`
	PartiesBroken map[string]uint64 `json:"parties_broken_total"`
	// The PvP counters (docs/pvp/modes.md "Panel and metrics (X12)"),
	// every label present: the first attack on each opponent by its kind,
	// the deaths to a player by the killer's kind, the duels by result and
	// the interrupts by step (cast, landed, too_late).
	PvPFights     map[string]uint64 `json:"pvp_fights_total"`
	PvPDeaths     map[string]uint64 `json:"pvp_deaths_total"`
	Duels         map[string]uint64 `json:"duels_total"`
	PvPInterrupts map[string]uint64 `json:"pvp_interrupts_total"`
}

// The via labels of Snapshot.ValuablesSold.
const (
	ViaTrade = "trade"
	ViaStore = "store"
)

// PlanOutcome is how one of the walker's route plans ended.
type PlanOutcome uint8

// The outcomes of a route plan: a route to walk, or the walker's reason for
// none (the search ran out of its budget, a closed door, the refused
// segments cut every way, no route, isolated ground, the bot off the
// navigation). Isolated counts the /unstuck asks too.
const (
	PlanOK PlanOutcome = iota
	PlanBudget
	PlanDoor
	PlanBlocked
	PlanNoRoute
	PlanIsolated
	PlanNoNav
	planOutcomes
)

// planNames are the Snapshot.NavPlans keys, indexed by PlanOutcome.
var planNames = [planOutcomes]string{"ok", "budget", "door", "blocked", "no_route", "isolated", "no_nav"}

// String is the outcome's Snapshot.NavPlans key.
func (o PlanOutcome) String() string {
	if o >= planOutcomes {
		return "unknown"
	}
	return planNames[o]
}

// Collect sums every Bot and the removed Bots' retained total as of at,
// stores the sum as the latest and returns it.
func (r *Registry) Collect(at time.Time) *Snapshot {
	s := &Snapshot{At: at, BreakerOpen: r.breakerOpen.Load(), AcceptClosed: r.acceptClosed.Load(), Rules: make(map[string]uint64, len(r.rules))}
	// sum adds up the Bots through the fold Remove uses, so a counter is
	// summed and retained alike.
	sum := Bot{rules: make([]atomic.Uint64, len(r.rules))}
	r.mu.Lock()
	s.Bots = len(r.bots)
	s.PartiesFormed = r.partiesFormed
	s.PartiesBroken = make(map[string]uint64, len(r.partiesBroken))
	for why, n := range r.partiesBroken {
		s.PartiesBroken[why] = n
	}
	sum.fold(&r.retained)
	for _, b := range r.bots {
		sum.fold(b)
		s.ValuablesHeld += b.valuablesHeld.Load()
	}
	r.mu.Unlock()
	s.Events = sum.events.Load()
	s.Sent = sum.sent.Load()
	s.Paced = sum.paced.Load()
	s.Reconnects = sum.reconnects.Load()
	s.Aborts = sum.aborts.Load()
	s.Panics = sum.panics.Load()
	s.NavBlocked = sum.navBlocked.Load()
	s.NavDesync = sum.navDesync.Load()
	s.NavDoorReplan = sum.navDoorReplan.Load()
	s.NavSegments = sum.navSegments.Load()
	s.ValuablesAds = sum.valuablesAds.Load()
	s.ValuablesReplies = sum.valuablesReplies.Load()
	s.ValuablesDeposits = sum.valuablesDeposits.Load()
	s.ValuablesReleased = sum.valuablesReleased.Load()
	s.ValuablesSold = map[string]uint64{ViaTrade: sum.valuablesSoldTrade.Load(), ViaStore: sum.valuablesSoldStore.Load()}
	s.QueueDepth.add(&sum.depth)
	s.WakeLagMicros.add(&sum.wakeLag)
	s.NavWalkIdleMillis.add(&sum.walkIdle)
	s.NavWalkEfficiency.add(&sum.walkEfficiency)
	s.NavPlanMicros.add(&sum.planTime)
	s.NavPlans = make(map[string]uint64, planOutcomes)
	for i := range sum.navPlans {
		s.NavPlans[planNames[i]] = sum.navPlans[i].Load()
	}
	s.DecodeErrors = make(map[string]uint64)
	for i := range sum.decodeErrors {
		if n := sum.decodeErrors[i].Load(); n != 0 {
			s.DecodeErrors[decodeSlotName(i)] = n
		}
	}
	var pvp pvpSums
	pvp.add(&sum.pvp)
	pvp.fill(s)
	r.collectNavCache(s)
	for i, name := range r.rules {
		s.Rules[name] = sum.rules[i].Load()
	}
	r.last.Store(s)
	return s
}

// Last returns the latest collection, or an empty one before the first.
// The caller must not modify it.
func (r *Registry) Last() *Snapshot {
	if s := r.last.Load(); s != nil {
		return s
	}
	plans := make(map[string]uint64, planOutcomes)
	for _, n := range planNames {
		plans[n] = 0
	}
	s := &Snapshot{Rules: map[string]uint64{}, NavPlans: plans, NavLocates: navLocates(NavCache{}), ValuablesSold: map[string]uint64{ViaTrade: 0, ViaStore: 0}, DecodeErrors: map[string]uint64{}}
	(&pvpSums{}).fill(s)
	return s
}

// Run collects on every tick until ctx ends. The caller owns the ticker
// (time.NewTicker in the swarm, a channel in tests) and runs Run on a
// goroutine it joins.
func (r *Registry) Run(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-tick:
			r.Collect(t)
		}
	}
}

// Bot is one bot's counters. Its methods are safe on a nil *Bot, which
// records nothing, so a bot runs the same without metrics.
type Bot struct {
	reg                                     *Registry
	events, sent, paced, reconnects, aborts atomic.Uint64
	panics                                  atomic.Uint64
	decodeErrors                            [decodeSlots]atomic.Uint64
	navBlocked, navDesync, navDoorReplan    atomic.Uint64
	navSegments                             atomic.Uint64
	navPlans                                [planOutcomes]atomic.Uint64
	valuablesAds, valuablesReplies          atomic.Uint64
	valuablesSoldTrade, valuablesSoldStore  atomic.Uint64
	valuablesDeposits, valuablesReleased    atomic.Uint64
	valuablesHeld                           atomic.Int64
	pvp                                     pvpCounters
	rules                                   []atomic.Uint64 // indexed like reg.rules
	depth                                   Histogram
	wakeLag                                 Histogram // microseconds
	walkIdle                                Histogram // milliseconds
	walkEfficiency                          Histogram // per mille
	planTime                                Histogram // microseconds
}

// fold adds src's counters, rule hits, PvP counters and histograms to b:
// every count Collect sums, and not the valuablesHeld gauge. src is a Bot of
// b's registry, so its rules index like b's.
func (b *Bot) fold(src *Bot) {
	b.events.Add(src.events.Load())
	b.sent.Add(src.sent.Load())
	b.paced.Add(src.paced.Load())
	b.reconnects.Add(src.reconnects.Load())
	b.aborts.Add(src.aborts.Load())
	b.panics.Add(src.panics.Load())
	for i := range src.decodeErrors {
		b.decodeErrors[i].Add(src.decodeErrors[i].Load())
	}
	b.navBlocked.Add(src.navBlocked.Load())
	b.navDesync.Add(src.navDesync.Load())
	b.navDoorReplan.Add(src.navDoorReplan.Load())
	b.navSegments.Add(src.navSegments.Load())
	for i := range src.navPlans {
		b.navPlans[i].Add(src.navPlans[i].Load())
	}
	b.valuablesAds.Add(src.valuablesAds.Load())
	b.valuablesReplies.Add(src.valuablesReplies.Load())
	b.valuablesSoldTrade.Add(src.valuablesSoldTrade.Load())
	b.valuablesSoldStore.Add(src.valuablesSoldStore.Load())
	b.valuablesDeposits.Add(src.valuablesDeposits.Load())
	b.valuablesReleased.Add(src.valuablesReleased.Load())
	b.pvp.fold(&src.pvp)
	for i := range src.rules {
		b.rules[i].Add(src.rules[i].Load())
	}
	b.depth.fold(&src.depth)
	b.wakeLag.fold(&src.wakeLag)
	b.walkIdle.fold(&src.walkIdle)
	b.walkEfficiency.fold(&src.walkEfficiency)
	b.planTime.fold(&src.planTime)
}

// Drained records one wakeup of the brain that drained n queued events.
func (b *Bot) Drained(n int) {
	if b == nil {
		return
	}
	b.events.Add(uint64(n))
	b.depth.Observe(int64(n))
}

// Sent records a packet the bot wrote.
func (b *Bot) Sent() {
	if b != nil {
		b.sent.Add(1)
	}
}

// Paced records a packet the pacer held back.
func (b *Bot) Paced() {
	if b != nil {
		b.paced.Add(1)
	}
}

// Rule records a decision the named rule made; "" (no rule matched) is not
// counted.
func (b *Bot) Rule(name string) {
	if b == nil || name == "" {
		return
	}
	i, ok := b.reg.ruleIndex[name]
	if !ok {
		i = len(b.rules) - 1
	}
	b.rules[i].Add(1)
}

// WakeLag records how late a timed wakeup ran after it was due.
func (b *Bot) WakeLag(d time.Duration) {
	if b != nil {
		b.wakeLag.Observe(d.Microseconds())
	}
}

// Reconnect records a new session after the previous one ended.
func (b *Bot) Reconnect() {
	if b != nil {
		b.reconnects.Add(1)
	}
}

// Abort records a session the server aborted (closed the connection
// without a ServerClose or LeaveWorld, as at its outbound cap).
func (b *Bot) Abort() {
	if b != nil {
		b.aborts.Add(1)
	}
}

// Panic records a panic the bot recovered from (botclient.ErrPanic).
func (b *Bot) Panic() {
	if b != nil {
		b.panics.Add(1)
	}
}

// NavSegment records one of the walker's segments or talk clicks sent.
func (b *Bot) NavSegment() {
	if b != nil {
		b.navSegments.Add(1)
	}
}

// NavWalkIdle records how long a walker segment went out after the
// previous segment of its walk arrived, as the walker modeled the arrival.
func (b *Bot) NavWalkIdle(d time.Duration) {
	if b != nil {
		b.walkIdle.Observe(d.Milliseconds())
	}
}

// NavWalkEfficiency records an arrived walk's route length over its elapsed
// time times the run speed, in per mille: 1000 is a straight run without a
// stop.
func (b *Bot) NavWalkEfficiency(permille int64) {
	if b != nil {
		b.walkEfficiency.Observe(permille)
	}
}

// NavPlan records one of the walker's route plans: how it ended and how
// long it took. An outcome out of range counts nothing.
func (b *Bot) NavPlan(o PlanOutcome, d time.Duration) {
	if b == nil || o >= planOutcomes {
		return
	}
	b.navPlans[o].Add(1)
	b.planTime.Observe(d.Microseconds())
}

// NavBlocked records a blocked stop of one of the walker's segments.
func (b *Bot) NavBlocked() {
	if b != nil {
		b.navBlocked.Add(1)
	}
}

// NavDesync records a segment the server answered with another destination.
func (b *Bot) NavDesync() {
	if b != nil {
		b.navDesync.Add(1)
	}
}

// NavDoorReplan records a segment across a door the server answered with
// another destination: the door state differed, which never counts toward
// unreachable.
func (b *Bot) NavDoorReplan() {
	if b != nil {
		b.navDoorReplan.Add(1)
	}
}

// ValuablesAd records a Trade ad line of a valuables round.
func (b *Bot) ValuablesAd() {
	if b != nil {
		b.valuablesAds.Add(1)
	}
}

// ValuablesReply records a whisper reply of the valuables responder.
func (b *Bot) ValuablesReply() {
	if b != nil {
		b.valuablesReplies.Add(1)
	}
}

// ValuablesSold records one sale of valuables via ViaTrade (an entry a
// settled trade sold) or ViaStore (a listed row that sold units); another via counts
// nothing.
func (b *Bot) ValuablesSold(via string) {
	if b == nil {
		return
	}
	switch via {
	case ViaTrade:
		b.valuablesSoldTrade.Add(1)
	case ViaStore:
		b.valuablesSoldStore.Add(1)
	}
}

// ValuablesDeposited records a private deposit of valuables the server
// made.
func (b *Bot) ValuablesDeposited() {
	if b != nil {
		b.valuablesDeposits.Add(1)
	}
}

// ValuablesReleased records n ledger entries the cap released to the
// store.
func (b *Bot) ValuablesReleased(n int) {
	if b != nil && n > 0 {
		b.valuablesReleased.Add(uint64(n))
	}
}

// ValuablesHeld sets the gauge of the units of valuables the bot's ledger
// holds, carried and stored.
func (b *Bot) ValuablesHeld(units int64) {
	if b != nil {
		b.valuablesHeld.Store(units)
	}
}
