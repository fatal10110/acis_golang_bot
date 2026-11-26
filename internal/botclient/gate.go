package botclient

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// Login gate timing.
const (
	// dialSpacing is the least time between two dial starts of the process.
	// The servers' accept filter (server internal/commons/netutil/
	// acceptfilter.go, DefaultAcceptFilterConfig) counts a try for every
	// connection from an IP that arrives under BurstWindow (1 s) after the
	// previous one, never clears a try on a slower connection, and bans the
	// IP on the burst connection after BurstLimit (3) tries until 300 s pass
	// with no connection. Every bot of a process dials from the same source
	// address, so on a spaced gate every login and game dial starts at least
	// this long after the previous one and no connection ever counts a try:
	// tries left by an earlier `bots accounts` run never add up. The 250 ms
	// above the window absorb timer and scheduling lag; dialJitter is added on
	// top. An allowlisted gate skips the spacing (NewAllowlistedLoginGate).
	// It is the floor of a spaced gate's spacing: SetSpacing only raises it.
	dialSpacing = 1250 * time.Millisecond
	dialJitter  = 500 * time.Millisecond
	// MaxLoginSpacing is the most SetSpacing raises a gate's spacing to.
	MaxLoginSpacing = time.Minute
	// refusedBase and refusedMax bound the process-wide backoff while the
	// servers refuse connections (down or restarting).
	refusedBase = 2 * time.Second
	refusedMax  = 2 * time.Minute
	// serverBanTime is the servers' default accept filter BanTime (server
	// internal/commons/netutil/acceptfilter.go, DefaultAcceptFilterConfig):
	// the filter forgets a banned address once it made no connection for
	// that long, a connection closed at accept included (record sets
	// lastAccess before it tests the ban; Sweep, every acceptSweep).
	serverBanTime = 300 * time.Second
	acceptSweep   = 5 * time.Second
	// banQuiet is how long no bot of the process dials after a dial closed
	// at accept (ErrClosedAtAccept): the ban time, the sweep that lifts it
	// and a margin for timer lag. Any dial before it would refresh the ban.
	// A server configured with a longer BanTime needs it raised by hand.
	banQuiet = serverBanTime + acceptSweep + 25*time.Second
)

// ErrLoginsHalted reports that the credential breaker tripped: no bot of the
// process logs in again until the breaker is reset (LoginGate.Reset).
var ErrLoginsHalted = errors.New("all logins halted by the credential breaker")

// ErrSpacing reports a spacing outside a gate's bounds (LoginGate.SetSpacing).
var ErrSpacing = errors.New("login spacing out of bounds")

// LoginGate is the process-wide gate every bot passes to dial the login and
// game servers. A bot takes a turn, during which no other bot dials, and
// spaces each dial of its turn under the accept filter (dialSpacing), unless
// the gate is unspaced (NewAllowlistedLoginGate). One backoff spaces every
// bot while the servers are not ready: refused dials and not-ready endings
// of different bots step it, an entry into the world resets it. A dial
// closed at accept keeps every bot quiet for banQuiet. The credential
// breaker (Halt) stops every later login until Reset. All of its methods may
// be called from any goroutine.
type LoginGate struct {
	clock Clock
	// spaced is set at construction on a spaced gate, and by the turn holder
	// once an allowlisted gate saw a dial closed at accept; it is never
	// cleared. It is written under mu, with spacing, and loaded anywhere.
	spaced atomic.Bool
	// spacing is the least time between two dial starts, in nanoseconds:
	// MinSpacing to MaxLoginSpacing. SetSpacing and the turn holder store
	// it under mu; the turn holder loads it in Dial.
	spacing atomic.Int64
	// turn holds one token: taking it starts a turn, putting it back ends it.
	turn chan struct{}

	// rng, next, refused, notReadySeen and notReadyBy are owned by the turn
	// holder.
	rng *rand.Rand
	// next is the earliest start of the next dial.
	next time.Time
	// refused is the fleet backoff while the servers are not ready.
	refused backoff
	// notReadySeen is set once a not-ready ending came since the last entry
	// into the world; notReadyBy is the bot it came from.
	notReadySeen bool
	notReadyBy   string

	// mu guards the breaker's current generation, the pair halt and err, the
	// writes of spaced and spacing, and metrics. Halt sets err before it
	// closes halt, so a waiter woken by a closed halt that is still current
	// reads a non-nil err; Reset installs a new open pair, so a waiter woken
	// by a stale halt waits on the new one.
	mu   sync.Mutex
	halt chan struct{} // closed when the generation trips
	err  error         // the generation's cause once tripped
	// metrics records the open breaker and the dials closed at accept;
	// guarded by mu, nil records nothing.
	metrics *botmetrics.Registry
}

// NewLoginGate returns an open gate that spaces dials under the accept
// filter, timed by clock and jittered by rng; a nil rng is seeded at random.
func NewLoginGate(clock Clock, rng *rand.Rand) *LoginGate {
	g := newLoginGate(clock, rng)
	g.spaced.Store(true)
	g.spacing.Store(int64(dialSpacing))
	return g
}

// NewAllowlistedLoginGate returns an open gate that does not space dials, for
// a source address on AcceptFilterAllowlist of both servers: the accept
// filter skips allowlisted addresses (server internal/commons/netutil/
// acceptfilter.go), so no burst ban can follow. Turns stay one at a time, so
// the credential breaker still halts logins on the first refused credential
// (the credential ban has no allowlist), refused dials still back off, and
// the first dial closed at accept turns the spacing on for good: the address
// may not be on the list after all.
func NewAllowlistedLoginGate(clock Clock, rng *rand.Rand) *LoginGate {
	return newLoginGate(clock, rng)
}

func newLoginGate(clock Clock, rng *rand.Rand) *LoginGate {
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	g := &LoginGate{
		clock:   clock,
		turn:    make(chan struct{}, 1),
		halt:    make(chan struct{}),
		rng:     rng,
		refused: backoff{base: refusedBase, max: refusedMax},
	}
	g.turn <- struct{}{}
	return g
}

// Turn is one bot's exclusive use of the gate. Dial before every connection
// it opens and Dialed after it, Entered once it has entered the world, Done
// once it has entered or given up.
type Turn struct {
	g *LoginGate
	// who is the bot's identity, its account name: a key that tells bots
	// apart, never logged by the gate.
	who  string
	done bool
}

// Take waits for the gate, for the bot who, its account name. It fails once
// the breaker has tripped or ctx ends; a breaker that trips and is reset
// while Take waits does not fail it.
func (g *LoginGate) Take(ctx context.Context, who string) (*Turn, error) {
	for {
		halt, err := g.breaker()
		if err := cmp.Or(err, ctx.Err()); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-halt:
			continue // fail on the generation if it is still current
		case <-g.turn:
		}
		if err := g.Err(); err != nil {
			g.turn <- struct{}{}
			return nil, err
		}
		return &Turn{g: g, who: who}, nil
	}
}

// Dial waits until the turn may start its next dial: the gate's spacing
// after the previous dial start of the process, plus a jitter on a spaced
// gate, and past the fleet backoff and any quiet period. It fails once the breaker has tripped or ctx
// ends; a breaker that trips and is reset while Dial waits does not fail it.
func (t *Turn) Dial(ctx context.Context) error {
	g := t.g
	for {
		halt, err := g.breaker()
		if err := cmp.Or(err, ctx.Err()); err != nil {
			return err
		}
		now := g.clock.Now()
		if !now.Before(g.next) {
			g.next = now.Add(g.Spacing())
			if g.spaced.Load() {
				g.next = g.next.Add(jitter(g.rng, dialJitter))
			}
			return nil
		}
		timer := g.clock.NewTimer(g.next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-halt: // fail on the generation if it is still current
		case <-timer.C():
		}
		timer.Stop()
	}
}

// MinSpacing is the floor of the gate's spacing: dialSpacing on a spaced
// gate, which keeps every dial out of the accept filter's burst window, and
// none on an allowlisted gate, which the filter skips, until it saw a dial
// closed at accept.
func (g *LoginGate) MinSpacing() time.Duration {
	if g.spaced.Load() {
		return dialSpacing
	}
	return 0
}

// Spacing is the least time between two dial starts of the process; a
// spaced gate adds a jitter of up to dialJitter on top.
func (g *LoginGate) Spacing() time.Duration {
	return time.Duration(g.spacing.Load())
}

// SetSpacing sets the gate's spacing from the next dial on. It refuses a
// spacing under MinSpacing or over MaxLoginSpacing with ErrSpacing, so the
// spacing never drops under the accept filter's limit.
func (g *LoginGate) SetSpacing(d time.Duration) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if d < g.MinSpacing() || d > MaxLoginSpacing {
		return fmt.Errorf("%w: %s, want %s to %s", ErrSpacing, d, g.MinSpacing(), MaxLoginSpacing)
	}
	g.spacing.Store(int64(d))
	return nil
}

// Dialed reports how the turn's last dial, or the login session on it,
// ended:
//   - a refused connection steps the fleet backoff: the next dial of every
//     bot waits it, re-jittered and doubled on every step up to refusedMax;
//   - a connection closed at accept (ErrClosedAtAccept) keeps every bot
//     from dialing for banQuiet, after which the next dial is the probe, and
//     turns an allowlisted gate's spacing on;
//   - a not-ready ending (notReady) steps the fleet backoff when the last
//     not-ready ending since the last entry came from another bot: two bots
//     in a row mean the server, one alone may be its account. It records
//     the turn's bot either way;
//   - anything else, AccountInUse included, leaves the gate as it is.
//
// Only Entered resets the fleet backoff.
func (t *Turn) Dialed(err error) {
	g := t.g
	switch {
	case isRefused(err):
		g.step()
	case errors.Is(err, ErrClosedAtAccept):
		g.closedAtAccept()
	case notReady(err):
		if g.notReadySeen && g.notReadyBy != t.who {
			g.step()
		}
		g.notReadySeen, g.notReadyBy = true, t.who
	}
}

// Entered reports that the turn's attempt entered the world: the servers are
// ready, so the fleet backoff and the last not-ready ending are forgotten.
// Call it before Done.
func (t *Turn) Entered() {
	g := t.g
	g.refused.reset()
	g.notReadySeen, g.notReadyBy = false, ""
}

// step pushes the next dial out by the fleet backoff's next step; the turn
// holder calls it.
func (g *LoginGate) step() {
	if at := g.clock.Now().Add(g.refused.next(g.rng)); at.After(g.next) {
		g.next = at
	}
}

// closedAtAccept starts a quiet period of banQuiet, turns the spacing on and
// counts the dial; the turn holder calls it.
func (g *LoginGate) closedAtAccept() {
	if at := g.clock.Now().Add(banQuiet); at.After(g.next) {
		g.next = at
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.spaced.Load() {
		g.spaced.Store(true)
		g.spacing.Store(max(g.spacing.Load(), int64(dialSpacing)))
	}
	if g.metrics != nil {
		g.metrics.AcceptClosed()
	}
}

// Done ends the turn; a second call does nothing.
func (t *Turn) Done() {
	if t.done {
		return
	}
	t.done = true
	t.g.turn <- struct{}{}
}

// Halt trips the credential breaker: the login server bans the IP after 3
// refused credentials in a row from it (server internal/loginserver/
// clientlink.go, DefaultLoginTryBeforeBan) for 10 minutes, and counts per IP,
// not per account. So the first refused credential stops every later login
// of the process until Reset. Later calls keep the first cause.
func (g *LoginGate) Halt(cause error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.haltLocked(cause)
}

// haltLocked trips the current generation unless it already tripped; the
// caller holds mu.
func (g *LoginGate) haltLocked(cause error) {
	if g.err != nil {
		return
	}
	g.err = fmt.Errorf("%w: %w", ErrLoginsHalted, cause)
	close(g.halt)
	if g.metrics != nil {
		g.metrics.SetBreaker(true)
	}
}

// Reset closes a tripped breaker: a new generation starts, logins resume, and
// the next Halt trips it again with its own cause. Bots a halt already ended
// stay ended; bots still waiting in Take or Dial wait on. Reset of an
// untripped breaker does nothing.
func (g *LoginGate) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resetLocked()
}

// resetLocked starts a new generation if the current one tripped; the caller
// holds mu.
func (g *LoginGate) resetLocked() {
	if g.err == nil {
		return
	}
	g.halt, g.err = make(chan struct{}), nil
	if g.metrics != nil {
		g.metrics.SetBreaker(false)
	}
}

// breaker returns the current generation: its halt channel and its cause,
// nil until it trips.
func (g *LoginGate) breaker() (<-chan struct{}, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.halt, g.err
}

// ReportBreaker makes Halt record the open breaker in r, and the gate count
// every dial closed at accept there. Call it before the first Take.
func (g *LoginGate) ReportBreaker(r *botmetrics.Registry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.metrics = r
}

// Err is the breaker's error once the current generation tripped, else nil.
func (g *LoginGate) Err() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.err
}

// backoff is an exponential delay re-jittered on every step: step n waits a
// uniform time in [d/2, d), d being base doubled n times and capped at max.
type backoff struct {
	base, max time.Duration
	n         int
}

func (b *backoff) next(rng *rand.Rand) time.Duration {
	d := b.base
	for i := 0; i < b.n && d < b.max; i++ {
		d *= 2
	}
	d = min(d, b.max)
	b.n++
	half := d / 2
	return half + jitter(rng, d-half)
}

func (b *backoff) reset() { b.n = 0 }

// jitter is a uniform duration in [0, d); d must be positive.
func jitter(rng *rand.Rand, d time.Duration) time.Duration {
	return time.Duration(rng.Int64N(int64(d)))
}

// isRefused reports a dial the server refused: it is down or restarting.
func isRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }

// notReady reports an attempt the servers ended because they are not ready
// for it, after a connection: PlayFail (the game server is offline or full),
// the login server's system LoginFails, and a packet before the world the
// bot could not decode (errDecode). None counts toward an IP ban: PlayFail
// comes after the credentials passed.
func notReady(err error) bool {
	if errors.Is(err, errDecode) {
		return true
	}
	var le *LoginError
	if !errors.As(err, &le) {
		return false
	}
	switch le.Packet {
	case "PlayFail":
		return true
	case "LoginFail":
		switch le.Reason {
		case loginFailSystemError, loginFailAccessFailed, loginFailServerOverloaded, loginFailMaintenance:
			return true
		}
	}
	return false
}
