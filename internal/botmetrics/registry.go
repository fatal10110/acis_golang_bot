// Package botmetrics counts what the bots of one swarm do and serves the
// totals on a live expvar endpoint bound to loopback (docs/plan.md, "7.
// Metrics").
//
// Every bot owns one Bot for the life of the process, across its sessions.
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

	mu   sync.Mutex
	bots []*Bot // guarded by mu
	// partiesFormed and partiesBroken count the panel organizer's bot
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
	return r
}

// NewBot adds one bot's counters to the registry. Call it once per bot, not
// per session: a Bot is never removed, so the swarm's fixed bot cap bounds
// the registry.
func (r *Registry) NewBot() *Bot {
	b := &Bot{reg: r, rules: make([]atomic.Uint64, len(r.rules))}
	r.mu.Lock()
	r.bots = append(r.bots, b)
	r.mu.Unlock()
	return b
}

// SetBreaker records the state of the process-wide credential breaker: open
// once it has halted every login.
func (r *Registry) SetBreaker(open bool) { r.breakerOpen.Store(open) }

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

// Snapshot is the sum of every Bot at one collection. Counters only grow;
// QueueDepth counts the events the brain drained at each wakeup, and
// WakeLagMicros how late each timed wakeup ran after it was due.
type Snapshot struct {
	At         time.Time `json:"at"`
	Bots       int       `json:"bots"`
	Events     uint64    `json:"events"`
	Sent       uint64    `json:"sent"`
	Paced      uint64    `json:"paced"`
	Reconnects uint64    `json:"reconnects"`
	Aborts     uint64    `json:"aborts"`
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
	NavSegments   uint64            `json:"nav_segments_total"`
	BreakerOpen   bool              `json:"breaker_open"`
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
	// PartiesFormed counts the bot parties the panel's organizer formed,
	// by hand or by itself, and PartiesBroken the ones it broke, keyed by
	// the reason (docs/parties/panel.md, "Metrics").
	PartiesFormed uint64            `json:"parties_formed_total"`
	PartiesBroken map[string]uint64 `json:"parties_broken_total"`
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

// Collect sums every Bot as of at, stores the sum as the latest and returns
// it.
func (r *Registry) Collect(at time.Time) *Snapshot {
	s := &Snapshot{At: at, BreakerOpen: r.breakerOpen.Load(), Rules: make(map[string]uint64, len(r.rules))}
	var soldTrade, soldStore uint64
	var plans [planOutcomes]uint64
	hits := make([]uint64, len(r.rules))
	r.mu.Lock()
	s.Bots = len(r.bots)
	s.PartiesFormed = r.partiesFormed
	s.PartiesBroken = make(map[string]uint64, len(r.partiesBroken))
	for why, n := range r.partiesBroken {
		s.PartiesBroken[why] = n
	}
	for _, b := range r.bots {
		s.Events += b.events.Load()
		s.Sent += b.sent.Load()
		s.Paced += b.paced.Load()
		s.Reconnects += b.reconnects.Load()
		s.Aborts += b.aborts.Load()
		s.NavBlocked += b.navBlocked.Load()
		s.NavDesync += b.navDesync.Load()
		s.NavDoorReplan += b.navDoorReplan.Load()
		s.NavSegments += b.navSegments.Load()
		s.ValuablesAds += b.valuablesAds.Load()
		s.ValuablesReplies += b.valuablesReplies.Load()
		soldTrade += b.valuablesSoldTrade.Load()
		soldStore += b.valuablesSoldStore.Load()
		s.ValuablesDeposits += b.valuablesDeposits.Load()
		s.ValuablesReleased += b.valuablesReleased.Load()
		s.ValuablesHeld += b.valuablesHeld.Load()
		for i := range b.rules {
			hits[i] += b.rules[i].Load()
		}
		for i := range b.navPlans {
			plans[i] += b.navPlans[i].Load()
		}
		s.QueueDepth.add(&b.depth)
		s.WakeLagMicros.add(&b.wakeLag)
		s.NavWalkIdleMillis.add(&b.walkIdle)
		s.NavWalkEfficiency.add(&b.walkEfficiency)
		s.NavPlanMicros.add(&b.planTime)
	}
	r.mu.Unlock()
	s.NavPlans = make(map[string]uint64, planOutcomes)
	for i, n := range plans {
		s.NavPlans[planNames[i]] = n
	}
	s.ValuablesSold = map[string]uint64{ViaTrade: soldTrade, ViaStore: soldStore}
	r.collectNavCache(s)
	for i, name := range r.rules {
		s.Rules[name] = hits[i]
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
	return &Snapshot{Rules: map[string]uint64{}, NavPlans: plans, NavLocates: navLocates(NavCache{}), ValuablesSold: map[string]uint64{ViaTrade: 0, ViaStore: 0}}
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
	navBlocked, navDesync, navDoorReplan    atomic.Uint64
	navSegments                             atomic.Uint64
	navPlans                                [planOutcomes]atomic.Uint64
	valuablesAds, valuablesReplies          atomic.Uint64
	valuablesSoldTrade, valuablesSoldStore  atomic.Uint64
	valuablesDeposits, valuablesReleased    atomic.Uint64
	valuablesHeld                           atomic.Int64
	rules                                   []atomic.Uint64 // indexed like reg.rules
	depth                                   Histogram
	wakeLag                                 Histogram // microseconds
	walkIdle                                Histogram // milliseconds
	walkEfficiency                          Histogram // per mille
	planTime                                Histogram // microseconds
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
