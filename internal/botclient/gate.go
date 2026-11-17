package botclient

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
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
	dialSpacing = 1250 * time.Millisecond
	dialJitter  = 500 * time.Millisecond
	// refusedBase and refusedMax bound the process-wide backoff while the
	// servers refuse connections (down or restarting).
	refusedBase = 2 * time.Second
	refusedMax  = 2 * time.Minute
)

// ErrLoginsHalted reports that the credential breaker tripped: no bot of the
// process logs in again.
var ErrLoginsHalted = errors.New("all logins halted by the credential breaker")

// LoginGate is the process-wide gate every bot passes to dial the login and
// game servers. A bot takes a turn, during which no other bot dials, and
// spaces each dial of its turn under the accept filter (dialSpacing), unless
// the gate is unspaced (NewAllowlistedLoginGate). A refused dial backs every
// bot off. The credential breaker (Halt) stops every later login. All of its
// methods may be called from any goroutine.
type LoginGate struct {
	clock Clock
	// spaced is set at construction and never changes.
	spaced bool
	// turn holds one token: taking it starts a turn, putting it back ends it.
	turn chan struct{}
	// halt is closed when the breaker trips.
	halt chan struct{}

	// rng, next and refused are owned by the turn holder.
	rng *rand.Rand
	// next is the earliest start of the next dial.
	next    time.Time
	refused backoff

	mu  sync.Mutex
	err error // the breaker's cause once tripped; guarded by mu
	// metrics records the open breaker; guarded by mu, nil records nothing.
	metrics *botmetrics.Registry
}

// NewLoginGate returns an open gate that spaces dials under the accept
// filter, timed by clock and jittered by rng; a nil rng is seeded at random.
func NewLoginGate(clock Clock, rng *rand.Rand) *LoginGate {
	g := newLoginGate(clock, rng)
	g.spaced = true
	return g
}

// NewAllowlistedLoginGate returns an open gate that does not space dials, for
// a source address on AcceptFilterAllowlist of both servers: the accept
// filter skips allowlisted addresses (server internal/commons/netutil/
// acceptfilter.go), so no burst ban can follow. Turns stay one at a time, so
// the credential breaker still halts logins on the first refused credential
// (the credential ban has no allowlist), and refused dials still back off.
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
// it opens, Done once it has entered the world or given up.
type Turn struct {
	g    *LoginGate
	done bool
}

// Take waits for the gate. It fails once the breaker has tripped or ctx
// ends.
func (g *LoginGate) Take(ctx context.Context) (*Turn, error) {
	if err := cmp.Or(g.Err(), ctx.Err()); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.halt:
		return nil, g.Err()
	case <-g.turn:
	}
	if err := g.Err(); err != nil {
		g.turn <- struct{}{}
		return nil, err
	}
	return &Turn{g: g}, nil
}

// Dial waits until the turn may start its next dial: on a spaced gate
// dialSpacing plus a jitter after the previous dial start of the process,
// and past the refused backoff. It fails once the breaker has tripped or ctx
// ends.
func (t *Turn) Dial(ctx context.Context) error {
	g := t.g
	for {
		if err := cmp.Or(g.Err(), ctx.Err()); err != nil {
			return err
		}
		now := g.clock.Now()
		if !now.Before(g.next) {
			g.next = now
			if g.spaced {
				g.next = now.Add(dialSpacing + jitter(g.rng, dialJitter))
			}
			return nil
		}
		timer := g.clock.NewTimer(g.next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-g.halt:
		case <-timer.C():
		}
		timer.Stop()
	}
}

// Dialed reports how the turn's last dial ended. A refused connection pushes
// the next dial of every bot out by the refused backoff, re-jittered and
// doubled on every refusal in a row up to refusedMax; any other outcome
// resets the backoff.
func (t *Turn) Dialed(err error) {
	g := t.g
	if !isRefused(err) {
		g.refused.reset()
		return
	}
	if at := g.clock.Now().Add(g.refused.next(g.rng)); at.After(g.next) {
		g.next = at
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
// of the process. Later calls keep the first cause.
func (g *LoginGate) Halt(cause error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return
	}
	g.err = fmt.Errorf("%w: %w", ErrLoginsHalted, cause)
	close(g.halt)
	if g.metrics != nil {
		g.metrics.SetBreaker(true)
	}
}

// ReportBreaker makes Halt record the open breaker in r. Call it before the
// first Take.
func (g *LoginGate) ReportBreaker(r *botmetrics.Registry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.metrics = r
}

// Err is the breaker's error once it tripped, else nil.
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
