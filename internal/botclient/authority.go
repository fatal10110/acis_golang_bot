package botclient

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Authority limits. They keep whispers, which have no flood gate on the
// server (chat.go chatTell), from turning into database load, so they are
// operational limits, not persona defaults (D6).
const (
	// authBurst and authRefill are the per-sender token bucket: a sender's
	// commands start at most authBurst database checks at once and one per
	// authRefill after that.
	authBurst  = 5
	authRefill = 2 * time.Second
	// deniedFor is how long a sender the database showed is no GM has its
	// commands refused with no new check (the negative cache).
	deniedFor = time.Minute
	// knownFor is how long a sender the database showed is a GM counts as
	// one for the party and trade hooks, which never query (Authority.Hooks).
	knownFor = 10 * time.Minute
	// lookupTimeout bounds one database check.
	lookupTimeout = 5 * time.Second
	// authWorkers is how many database checks run at once: the database
	// user's MAX_USER_CONNECTIONS (docs/gm-control.md).
	authWorkers = 2
	// authQueue is how many checks may wait for a worker; a command past it
	// is refused.
	authQueue = 16
	// authSweepAbove is the size of a per-sender map past which expired
	// entries are dropped.
	authSweepAbove = 1024
)

// Access levels as the server defines them (aCis_datapack/data/xml/
// accessLevels.xml): levels -1 (Banned) to 8, of which only 7 (Admin) and 8
// (Master) are isGM="true". The server plays a level the table does not
// define as the user level (access_level.go).
const accessLevelMax = 8

var gmAccessLevels = map[int32]bool{7: true, 8: true}

// LevelLookup reads a character's access level and whether it is pending
// deletion (deletetime set) from the game server's database. A character
// the database lacks reads as level 0, not deleted. Errors never carry the
// connection's secrets.
type LevelLookup interface {
	AccessLevel(ctx context.Context, objectID int32) (level int32, deleted bool, err error)
}

// AuthorityConfig configures an Authority: Commanders (the no-database
// allowlist of GM object ids) or Lookup (the database check), not both.
// With neither every command is refused.
type AuthorityConfig struct {
	Commanders []int32
	Lookup     LevelLookup
	// MinLevel, when above 0, is the access level from which a character
	// commands bots in place of the table's isGM.
	MinLevel int32
	Clock    Clock
	// Log reports failed database checks; nil logs nothing.
	Log *slog.Logger
}

// Authority decides who may command bots by whisper (docs/plan.md, "In-game
// control"). One is shared by every bot of a process. Commanders are
// answered at once; a database check runs on one of Run's workers and its
// verdict goes back to the asking brain. The configuration is read-only
// after NewAuthority; mu owns the per-sender state.
type Authority struct {
	clock      Clock
	log        *slog.Logger
	commanders map[int32]bool
	lookup     LevelLookup
	minLevel   int32
	queue      chan authRequest

	mu      sync.Mutex
	buckets map[int32]*tokenBucket
	// denied is the negative cache: senders refused until a time.
	denied map[int32]time.Time
	// known are the senders last seen to be GMs: commanders who whispered
	// and characters the database showed are GMs.
	known map[int32]knownGM
}

type knownGM struct {
	name string
	at   time.Time
}

// tell is a command whispered to a bot by sender, named name.
type tell struct {
	sender int32
	name   string
	cmd    command
}

// verdict is the authority's answer to a tell.
type verdict struct {
	tell
	ok bool
}

type authRequest struct {
	tell
	reply chan<- verdict
}

// NewAuthority returns the authority cfg describes. With a Lookup, Run
// must run for its checks to be answered.
func NewAuthority(cfg AuthorityConfig) *Authority {
	a := &Authority{
		clock: cfg.Clock, log: cfg.Log, lookup: cfg.Lookup, minLevel: cfg.MinLevel,
		commanders: make(map[int32]bool, len(cfg.Commanders)),
		queue:      make(chan authRequest, authQueue),
		buckets:    make(map[int32]*tokenBucket),
		denied:     make(map[int32]time.Time),
		known:      make(map[int32]knownGM),
	}
	if a.log == nil {
		a.log = slog.New(slog.DiscardHandler)
	}
	for _, id := range cfg.Commanders {
		a.commanders[id] = true
	}
	return a
}

// Run answers database checks on authWorkers goroutines until ctx ends,
// then joins them. Checks still queued are dropped unanswered. Without a
// Lookup it returns at once.
func (a *Authority) Run(ctx context.Context) {
	if a.lookup == nil {
		return
	}
	var wg sync.WaitGroup
	for range authWorkers {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case r := <-a.queue:
					a.check(ctx, r)
				}
			}
		})
	}
	wg.Wait()
}

// ask decides t at now. It answers at once (true) for a commander, for a
// sender in the negative cache or out of tokens, and when there is no
// database or its queue is full; otherwise it queues a database check whose
// verdict goes to reply, dropped if reply is full.
func (a *Authority) ask(t tell, now time.Time, reply chan<- verdict) (verdict, bool) {
	refuse := verdict{tell: t}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.commanders[t.sender] {
		a.known[t.sender] = knownGM{name: t.name, at: now}
		return verdict{tell: t, ok: true}, true
	}
	if a.lookup == nil || now.Before(a.denied[t.sender]) {
		return refuse, true
	}
	a.sweep(now)
	tb := a.buckets[t.sender]
	if tb == nil {
		tb = &tokenBucket{tokens: authBurst, filled: now}
		a.buckets[t.sender] = tb
	}
	if !tb.take(now) {
		return refuse, true
	}
	select {
	case a.queue <- authRequest{tell: t, reply: reply}:
		return verdict{}, false
	default:
		return refuse, true
	}
}

// check runs one database check and sends its verdict. A failed check is
// refused and not cached: it says nothing about the sender.
func (a *Authority) check(ctx context.Context, r authRequest) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	level, deleted, err := a.lookup.AccessLevel(ctx, r.sender)
	cancel()
	ok := err == nil && !deleted && a.grants(level)
	now := a.clock.Now()
	a.mu.Lock()
	switch {
	case ok:
		a.known[r.sender] = knownGM{name: r.name, at: now}
		delete(a.denied, r.sender)
	case err == nil:
		a.denied[r.sender] = now.Add(deniedFor)
		delete(a.known, r.sender)
	}
	a.mu.Unlock()
	if err != nil {
		a.log.Warn("gm check failed", "sender", r.sender, "err", err)
	}
	select {
	case r.reply <- verdict{tell: r.tell, ok: ok}:
	default:
	}
}

// grants reports whether a character at access level commands bots: a
// game master's level of the server's table, or MinLevel and above when
// set. A negative level is refused, and a level the table does not define
// plays as the user level.
func (a *Authority) grants(level int32) bool {
	if level < 0 {
		return false
	}
	if level > accessLevelMax {
		level = 0
	}
	if a.minLevel > 0 {
		return level >= a.minLevel
	}
	return gmAccessLevels[level]
}

// sweep drops expired negative cache entries and full buckets once either
// map grows past authSweepAbove. mu is held.
func (a *Authority) sweep(now time.Time) {
	if len(a.denied) > authSweepAbove {
		for id, until := range a.denied {
			if !now.Before(until) {
				delete(a.denied, id)
			}
		}
	}
	if len(a.buckets) > authSweepAbove {
		for id, tb := range a.buckets {
			if tb.full(now) {
				delete(a.buckets, id)
			}
		}
	}
	if len(a.known) > authSweepAbove {
		for id, k := range a.known {
			if now.Sub(k.at) >= knownFor {
				delete(a.known, id)
			}
		}
	}
}

// knows reports whether id is a commander, or a GM a database check
// confirmed within knownFor of now. It never queries.
func (a *Authority) knows(id int32, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.commanders[id] {
		return true
	}
	k, ok := a.known[id]
	return ok && now.Sub(k.at) < knownFor
}

// knowsName reports whether the player named name is one a knows: a known
// sender of that name, or a player of that name in sight.
func (a *Authority) knowsName(w *World, name string, now time.Time) bool {
	for id, o := range w.Objects {
		if o.Kind == KindPlayer && o.Name == name && a.knows(id, now) {
			return true
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, k := range a.known {
		if k.name == name && (a.commanders[id] || now.Sub(k.at) < knownFor) {
			return true
		}
	}
	return false
}

// Hooks returns p with its party and trade hooks pointed at a: the bot
// accepts the party invitation of a GM it knows (a commander, or a GM a
// recent database check confirmed) and otherwise asks p's own JoinParty,
// and a known GM is exempt from the trade policy, besides any p's own
// Exempt exempts. Neither hook queries the database: both run on the brain
// goroutine.
func (a *Authority) Hooks(p Persona) Persona {
	join, exempt := p.JoinParty, p.Trade.Exempt
	p.JoinParty = func(w *World, requester string, now time.Time) bool {
		return a.knowsName(w, requester, now) || (join != nil && join(w, requester, now))
	}
	p.Trade.Exempt = func(w *World, partner int32) bool {
		return a.knows(partner, a.clock.Now()) || (exempt != nil && exempt(w, partner))
	}
	return p
}

// tokenBucket is one sender's database checks: authBurst at once, one per
// authRefill after.
type tokenBucket struct {
	tokens float64
	filled time.Time
}

func (t *tokenBucket) refill(now time.Time) {
	if now.After(t.filled) {
		t.tokens = min(authBurst, t.tokens+float64(now.Sub(t.filled))/float64(authRefill))
		t.filled = now
	}
}

// take spends a token at now, reporting whether there was one.
func (t *tokenBucket) take(now time.Time) bool {
	t.refill(now)
	if t.tokens < 1 {
		return false
	}
	t.tokens--
	return true
}

// full reports whether the bucket is back to authBurst at now.
func (t *tokenBucket) full(now time.Time) bool {
	t.refill(now)
	return t.tokens >= authBurst
}
