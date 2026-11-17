package botclient

import (
	"cmp"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// Brain timing defaults and fixed waits.
const (
	defaultTick          = time.Second
	defaultTickJitter    = 500 * time.Millisecond
	defaultReactMin      = 300 * time.Millisecond
	defaultReactMax      = 1200 * time.Millisecond
	defaultLogoutTimeout = 15 * time.Second
	// logoutRetry is how long a bot whose Logout was refused (in combat)
	// plays on before it asks again.
	logoutRetry = 10 * time.Second
	// restartRetry is how long a dead bot waits for the answer to its
	// restart request before it asks again: a refused restart answers
	// nothing, and a siege attacker's request waits out the attacker
	// respawn delay, dropping repeats meanwhile.
	restartRetry = 10 * time.Second
	// toggleSettle is how long a sit/stand toggle waits for its
	// ChangeWaitType before the brain may toggle again.
	toggleSettle = 2 * time.Second
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
	// Metrics, when set, counts what the brain does; nil counts nothing.
	Metrics *botmetrics.Bot
	// Tick is the base time between decisions; each adds a uniform
	// jitter up to TickJitter.
	Tick, TickJitter time.Duration
	// ReactMin and ReactMax bound the uniform reaction delay between an
	// event about the bot and the decision that acts on it.
	ReactMin, ReactMax time.Duration
	// LogoutTimeout is how long a Logout waits for an answer.
	LogoutTimeout time.Duration
	// LogoutLimit, when set, bounds a requested logout in all, refusals
	// included: past it Run returns errLogoutTimeout. Zero plays on until
	// the server lets the bot out.
	LogoutLimit time.Duration
	Rand        *rand.Rand
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

	pace pacer
	// nextTick is the next regular decision; reactAt, when set, replaces it
	// with the reaction to an event.
	nextTick, reactAt time.Time
	// anchor is where idle drift stays around: the arrival point.
	anchor Point
	// engaged is the target the bot already attacked; it is not clicked
	// again until a refusal (ActionFailed), a move or a sit/stand toggle
	// drops the attack, or the target or combat changes.
	engaged int32
	// resting is set while rule 6 has the bot sit until recovered.
	resting bool
	// toggledAt is the last sit/stand toggle not yet confirmed.
	toggledAt time.Time
	// appearing is set from the bot's own loading-screen teleport until its
	// Appearing goes out: the server keeps the character out of the world
	// until then.
	appearing bool
	// restartedAt is the restart request not yet answered by a Revive.
	restartedAt time.Time
	logout      logoutState
	// lastRule is the rule that matched the latest decision.
	lastRule string
	// metrics is written only by this loop (BrainConfig.Metrics).
	metrics *botmetrics.Bot
}

// logoutState is a logout run inside the loop.
type logoutState struct {
	// requested is set once the session asked to log out.
	requested bool
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
	cfg.LogoutTimeout = cmp.Or(cfg.LogoutTimeout, defaultLogoutTimeout)
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	return &Brain{world: w, events: q, out: out, clock: clock, rng: rng, persona: cfg.Persona, cfg: cfg, metrics: cfg.Metrics}
}

// Run plays until done closes or the session ends. Closing logout asks the
// brain to log out: it sends Logout and waits for the answer inside the
// loop, playing on and asking again every logoutRetry while the server
// refuses (ActionFailed with no LeaveWorld within logoutRefusalGrace).
//
// Run returns nil after done closes or after the LeaveWorld that answers a
// Logout. A LeaveWorld it did not ask for, ServerClose, the connection
// ending (the session queues connLost), a Logout left unanswered and a
// failed send are errors. The session closes the
// connection and joins its reader afterwards in every case.
func (b *Brain) Run(done, logout <-chan struct{}) error {
	b.start(b.clock.Now())
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
}

// handle applies a batch of events received at now. The first event about
// the bot arms the reaction delay; later ones fold into that decision.
func (b *Brain) handle(batch []Event, now time.Time) error {
	self := b.world.Self.ObjectID
	for _, ev := range batch {
		b.world.Apply(ev, now)
		switch e := ev.(type) {
		case LeaveWorld:
			if b.logout.requested {
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
		case MyTargetSelected:
			if e.ObjectID != b.engaged {
				b.engaged = 0
			}
		case TargetUnselected:
			if e.ObjectID == self {
				b.engaged = 0
			}
		case AutoAttackStop:
			if e.ObjectID == self {
				b.engaged = 0
			}
		case ChangeWaitType:
			if e.ObjectID == self {
				b.toggledAt = time.Time{}
			}
		case TeleportToLocation:
			// A Fast teleport is an in-place correction with no loading
			// screen: the server never takes the character out of the
			// world for it (instant_teleport.go), so it asks no Appearing.
			if e.ObjectID == self {
				b.anchor = e.Pos
				b.appearing = b.appearing || !e.Fast
			}
		case Revive:
			if e.ObjectID == self {
				b.restartedAt = time.Time{}
			}
		}
		if b.reactAt.IsZero() && salient(ev, self) {
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
	case MyTargetSelected:
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
// Appearing goes out first; until it is out, the bot sends nothing else.
func (b *Brain) wake(now time.Time) error {
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
		sent, err := b.send(Action{kind: actLogout, payload: encodeLogout()}, now)
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
		if b.appearing {
			sent, err := b.send(Action{kind: actAppear, payload: encodeAppearing()}, now)
			if err != nil {
				return err
			}
			if !sent {
				b.reactAt = now.Add(time.Second / frameRate)
				return nil
			}
		}
		b.reactAt = time.Time{}
		b.nextTick = now.Add(b.tickDelay())
		name, a := b.decide(now)
		b.lastRule = name
		b.metrics.Rule(name)
		if a.kind != actHold {
			if _, err := b.send(a, now); err != nil {
				return err
			}
		}
	}
	if !b.appearing && now.Sub(b.pace.lastSent) >= keepaliveAfter {
		s := &b.world.Self
		ka := Action{kind: actKeepalive, payload: encodeValidatePosition(s.Position(now), s.Heading)}
		if _, err := b.send(ka, now); err != nil {
			return err
		}
	}
	return nil
}

// decisionAt is when the next decision is due.
func (b *Brain) decisionAt() time.Time {
	if !b.reactAt.IsZero() {
		return b.reactAt
	}
	return b.nextTick
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
	switch a.kind {
	case actAttack:
		b.engaged = a.object
	case actMove:
		// A move replaces the attack intention on the server.
		b.engaged = 0
	case actSitStand:
		// Sitting down drops the attack intention as well.
		b.engaged = 0
		b.toggledAt = now
	case actAppear:
		b.appearing = false
	case actRestart:
		b.restartedAt = now
	}
	return true, nil
}

func (b *Brain) tickDelay() time.Duration {
	return b.cfg.Tick + time.Duration(b.rng.Int64N(int64(b.cfg.TickJitter)+1))
}

func (b *Brain) reactDelay() time.Duration {
	return b.cfg.ReactMin + time.Duration(b.rng.Int64N(int64(b.cfg.ReactMax-b.cfg.ReactMin)+1))
}
