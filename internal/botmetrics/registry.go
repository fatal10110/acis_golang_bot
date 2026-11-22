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

	last atomic.Pointer[Snapshot]
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
}

// Collect sums every Bot as of at, stores the sum as the latest and returns
// it.
func (r *Registry) Collect(at time.Time) *Snapshot {
	s := &Snapshot{At: at, BreakerOpen: r.breakerOpen.Load(), Rules: make(map[string]uint64, len(r.rules))}
	hits := make([]uint64, len(r.rules))
	r.mu.Lock()
	s.Bots = len(r.bots)
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
		for i := range b.rules {
			hits[i] += b.rules[i].Load()
		}
		s.QueueDepth.add(&b.depth)
		s.WakeLagMicros.add(&b.wakeLag)
	}
	r.mu.Unlock()
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
	return &Snapshot{Rules: map[string]uint64{}}
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
	rules                                   []atomic.Uint64 // indexed like reg.rules
	depth                                   Histogram
	wakeLag                                 Histogram // microseconds
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
