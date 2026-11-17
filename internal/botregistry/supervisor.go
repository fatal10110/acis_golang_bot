package botregistry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

const (
	// CoolDown is how long a bot the server aborted repeatedly
	// (botclient.ErrRepeatedAborts) waits before it restarts by itself: a
	// run of home line drops can end every bot at once.
	CoolDown = 10 * time.Minute
	// ImportPreset is the preset of an imported bot: what the swarms
	// before the panel ran.
	ImportPreset = "support"
)

// Stop reasons the supervisor records for ends it caused.
const (
	reasonDespawned = "despawned"
	reasonDeleted   = "deleted"
	reasonOverCap   = "over -max-online at start"
)

var (
	// ErrStopped answers a request to a supervisor that stopped.
	ErrStopped = errors.New("supervisor stopped")
	// ErrMaxOnline refuses a spawn past -max-online.
	ErrMaxOnline = errors.New("online cap reached")
	// ErrNoBot refuses a request for an account the registry has no bot
	// of.
	ErrNoBot = errors.New("no such bot")
	// ErrOnline refuses a spawn of a bot already online, or a delete of a
	// bot not despawned.
	ErrOnline = errors.New("bot is online")
	// ErrOffline refuses a respawn of a bot not online.
	ErrOffline = errors.New("bot is offline")
)

// Run is one run of a registry bot: what a RunFunc plays.
type Run struct {
	Account string
	// Character is the bot's character from the registry: its known name,
	// or its derived names with its class or the never-create flag.
	Character botclient.Character
	Preset    string
	// Markers are the markers of every prefix of the registry, for the bot
	// filter, as of the run's start.
	Markers botnames.Markers
	// Metrics is the bot's slot: one per registry bot, reused by every
	// run; nil without a metrics registry.
	Metrics *botmetrics.Bot
	// Learned hands the supervisor the character the run learned; it is
	// botclient.BotConfig.Learned. It returns once the supervisor took the
	// report: the supervisor goroutine always receives run reports, and
	// handles one request or report at a time, so it waits at most for one
	// registry save.
	Learned func(name string, classID int32)
}

// RunFunc plays one run until ctx ends, which returns nil, or the bot stops
// by itself, which returns why (botclient.Bot.Run).
type RunFunc func(ctx context.Context, r Run) error

// Config is what a Supervisor runs on.
type Config struct {
	// Registry is the open registry; the Supervisor owns it from Run on.
	Registry *Registry
	Run      RunFunc
	// Gate is the login gate every run passes; ResumeLogins resets its
	// breaker.
	Gate *botclient.LoginGate
	// Clock times the cool-down and the stop times.
	Clock botclient.Clock
	// MaxOnline bounds the bots online at once, on spawn and on restore.
	MaxOnline int
	// Metrics, when set, gets one slot per registry bot.
	Metrics *botmetrics.Registry
	// CheckPreset reports a preset the panel cannot run; nil takes any.
	CheckPreset func(name string) error
	// Rand draws new bots' classes and appearances; nil is seeded at
	// random.
	Rand *rand.Rand
	// Log receives the supervisor's reports; nil discards them.
	Log *slog.Logger
}

// RunState is what a bot's run is doing.
type RunState string

// The run states.
const (
	// RunNone: no run, none due.
	RunNone RunState = "none"
	// RunActive: a run plays.
	RunActive RunState = "running"
	// RunStopping: a run was canceled and has not reported yet.
	RunStopping RunState = "stopping"
	// RunHalted: the credential breaker ended the run; "resume logins"
	// restarts it.
	RunHalted RunState = "halted"
	// RunCoolingDown: the run ended on repeated aborts; it restarts after
	// CoolDown.
	RunCoolingDown RunState = "cooling down"
)

// Status is a bot of the registry and what its run does.
type Status struct {
	Bot
	Run RunState
	// RestartAt is when a cooling-down bot restarts.
	RestartAt time.Time
}

// runState is a bot's run, owned by the supervisor goroutine. A bot without
// one has no run and none due.
type runState struct {
	// cancel is set while a run is live: started and not reported.
	cancel context.CancelFunc
	// stopping marks a live run the supervisor canceled; restart starts a
	// new one when it reports.
	stopping, restart bool
	halted            bool
	// retryAt is when a cooling-down bot restarts; zero otherwise.
	retryAt time.Time
}

func (st *runState) state() RunState {
	switch {
	case st == nil:
		return RunNone
	case st.cancel != nil && st.stopping:
		return RunStopping
	case st.cancel != nil:
		return RunActive
	case st.halted:
		return RunHalted
	case !st.retryAt.IsZero():
		return RunCoolingDown
	}
	return RunNone
}

// event is a run's report: the character it learned, or its end.
type event struct {
	account string
	// learned is set for a learned character, with name and class.
	learned bool
	name    string
	class   int32
	err     error
}

// Supervisor owns the registry, its file and every bot's run (docs/plan.md
// section 8, "Ownership"). Run is its goroutine; the request methods (List,
// Create, Import, Spawn, Despawn, Respawn, SetPreset, Delete, ResumeLogins)
// are safe from any goroutine and answer at once, without waiting on a run.
// It sends no packet: every run logs in through the login gate, which paces
// it.
type Supervisor struct {
	cfg    Config
	reqs   chan func()
	events chan event
	done   chan struct{}
	wg     sync.WaitGroup

	// Owned by Run.
	runs    map[string]*runState
	slots   map[string]*botmetrics.Bot
	timer   botclient.Timer
	base    context.Context
	closing bool
	rng     *rand.Rand
	log     *slog.Logger
}

// New returns the supervisor of cfg; Run starts it.
func New(cfg Config) (*Supervisor, error) {
	switch {
	case cfg.Registry == nil, cfg.Run == nil, cfg.Gate == nil, cfg.Clock == nil:
		return nil, errors.New("supervisor: registry, run, gate and clock are required")
	case cfg.MaxOnline < 1:
		return nil, fmt.Errorf("supervisor: max online %d: want 1 or more", cfg.MaxOnline)
	}
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Supervisor{
		cfg:    cfg,
		reqs:   make(chan func()),
		events: make(chan event),
		done:   make(chan struct{}),
		runs:   make(map[string]*runState),
		slots:  make(map[string]*botmetrics.Bot),
		rng:    rng,
		log:    log,
	}, nil
}

// Run restores the bots marked online, up to MaxOnline, then serves
// requests and run reports until ctx ends. It then cancels every run, keeps
// draining reports until all arrived, joins every run goroutine and returns.
// The bots stay marked online, so the next start restores them. Call it
// once.
func (s *Supervisor) Run(ctx context.Context) error {
	defer close(s.done)
	s.base = context.WithoutCancel(ctx)
	s.restore()
	for {
		var tick <-chan time.Time
		if s.timer != nil {
			tick = s.timer.C()
		}
		select {
		case <-ctx.Done():
			s.shutdown()
			return nil
		case fn := <-s.reqs:
			fn()
		case e := <-s.events:
			s.handle(e)
		case <-tick:
			s.wake()
		}
	}
}

// shutdown cancels every run and handles reports until none is live.
func (s *Supervisor) shutdown() {
	s.closing = true
	if s.timer != nil {
		s.timer.Stop()
	}
	for _, st := range s.runs {
		if st.cancel != nil {
			st.cancel()
		}
	}
	for s.live() > 0 {
		s.handle(<-s.events)
	}
	s.wg.Wait()
}

func (s *Supervisor) live() int {
	n := 0
	for _, st := range s.runs {
		if st.cancel != nil {
			n++
		}
	}
	return n
}

// restore starts the bots marked online, in registry order, up to
// MaxOnline; the rest are marked offline.
func (s *Supervisor) restore() {
	started := 0
	for _, b := range s.cfg.Registry.Bots() {
		if !b.Online {
			continue
		}
		if started >= s.cfg.MaxOnline {
			s.stopped(b.Account, false, reasonOverCap)
			continue
		}
		started++
		s.start(b.Account)
	}
	if started > 0 {
		s.log.Info("bots restored", "bots", started)
	}
}

// online counts the bots that hold or will hold a session: marked online,
// or with a live run.
func (s *Supervisor) online() int {
	n := 0
	for _, b := range s.cfg.Registry.Bots() {
		if b.Online || s.runs[b.Account] != nil && s.runs[b.Account].cancel != nil {
			n++
		}
	}
	return n
}

// start starts a run of account's bot, which has none live. When it cannot,
// the bot is stopped and its run state, halted or cooling down, is dropped.
func (s *Supervisor) start(account string) {
	b, ok := s.cfg.Registry.Bot(account)
	if !ok {
		return
	}
	ch, err := s.character(b)
	if err != nil {
		s.log.Error("bot not started", "account", account, "err", err)
		delete(s.runs, account)
		s.stopped(account, false, err.Error())
		return
	}
	slot, ok := s.slots[account]
	if !ok && s.cfg.Metrics != nil {
		slot = s.cfg.Metrics.NewBot()
		s.slots[account] = slot
	}
	ctx, cancel := context.WithCancel(s.base)
	s.runs[account] = &runState{cancel: cancel}
	r := Run{
		Account:   account,
		Character: ch,
		Preset:    b.Preset,
		Markers:   s.cfg.Registry.Markers(),
		Metrics:   slot,
		Learned: func(name string, classID int32) {
			s.events <- event{account: account, learned: true, name: name, class: classID}
		},
	}
	s.wg.Go(func() {
		err := s.cfg.Run(ctx, r)
		s.events <- event{account: account, err: err}
	})
	s.log.Info("bot starts", "account", account, "preset", b.Preset)
}

// character is the Character of b: its known name, or its derived names with
// the never-create flag or its stored class.
func (s *Supervisor) character(b Bot) (botclient.Character, error) {
	if b.Name != "" {
		return botclient.Character{Name: b.Name}, nil
	}
	m, ok := s.cfg.Registry.Marker(b.Prefix)
	if !ok {
		return botclient.Character{}, fmt.Errorf("prefix %s not in the registry", b.Prefix)
	}
	gen, err := botnames.New(m, botnames.AccountSource(b.Account))
	if err != nil {
		return botclient.Character{}, err
	}
	ch := botclient.Character{Names: gen.Names(botclient.MaxNameCandidates), NeverCreate: b.NeverCreate}
	if b.NeverCreate {
		return ch, nil
	}
	if b.Class == nil {
		return botclient.Character{}, errors.New("no class to create a character with")
	}
	i := slices.IndexFunc(botclient.StartingClasses(), func(c botclient.StartingClass) bool { return c.ID == *b.Class })
	if i < 0 {
		return botclient.Character{}, fmt.Errorf("class %d: not a starting class", *b.Class)
	}
	ch.Class = botclient.StartingClasses()[i]
	ch.Appearance = botclient.RandomAppearance(s.rng)
	return ch, ch.Validate()
}

// stopped records the end of account's run with reason and the online flag.
func (s *Supervisor) stopped(account string, online bool, reason string) {
	now := s.cfg.Clock.Now()
	s.update(account, func(b *Bot) {
		b.Online = online
		b.StoppedAt, b.StopReason = now, reason
	})
}

func (s *Supervisor) update(account string, fn func(*Bot)) {
	if err := s.cfg.Registry.Update(account, fn); err != nil {
		s.log.Error("registry update failed", "account", account, "err", err)
	}
}

// handle applies a run's report.
func (s *Supervisor) handle(e event) {
	if e.learned {
		class := e.class
		s.update(e.account, func(b *Bot) { b.Name, b.Class = e.name, &class })
		s.log.Info("bot learned its character", "account", e.account, "character", e.name, "class", e.class)
		return
	}
	st := s.runs[e.account]
	st.cancel()
	st.cancel = nil
	if s.closing {
		delete(s.runs, e.account)
		return
	}
	if st.stopping {
		st.stopping = false
		if st.restart {
			st.restart = false
			s.start(e.account)
			return
		}
		delete(s.runs, e.account)
		s.stopped(e.account, false, reasonDespawned)
		return
	}
	s.ended(e.account, st, e.err)
}

// ended maps the end of a run the supervisor did not cancel to the bot's
// state (docs/plan.md section 8): a commanded logout is offline; a halt by
// the credential breaker stays online until "resume logins", or restarts at
// once when the breaker was reset since; repeated aborts restart after
// CoolDown; an account holding characters of no bot is dropped and the new
// bot moves to the next account; every other end is stopped with its
// reason.
func (s *Supervisor) ended(account string, st *runState, err error) {
	if err == nil {
		err = errors.New("run ended")
	}
	b, _ := s.cfg.Registry.Bot(account)
	s.log.Info("bot ended", "account", account, "err", err)
	switch {
	case errors.Is(err, botclient.ErrLoginsHalted):
		if s.cfg.Gate.Err() == nil {
			s.start(account)
			return
		}
		st.halted = true
		s.stopped(account, true, err.Error())
	case errors.Is(err, botclient.ErrRepeatedAborts):
		st.retryAt = s.cfg.Clock.Now().Add(CoolDown)
		s.stopped(account, true, err.Error())
		s.arm()
	case errors.Is(err, botclient.ErrAccountHasCharacters) && b.Name == "" && !b.NeverCreate:
		delete(s.runs, account)
		s.replace(b, err)
	default:
		delete(s.runs, account)
		s.stopped(account, false, err.Error())
	}
}

// replace drops b's account, which holds characters of no bot, and starts b
// again on the next account of the panel's prefix.
func (s *Supervisor) replace(b Bot, cause error) {
	if err := s.cfg.Registry.Drop(b.Account, cause.Error(), s.cfg.Clock.Now()); err != nil {
		s.log.Error("registry update failed", "account", b.Account, "err", err)
	}
	nb, err := s.cfg.Registry.NewBot(Bot{Class: b.Class, Preset: b.Preset, Online: true})
	if err != nil {
		s.log.Error("bot not replaced", "account", b.Account, "err", err)
		if nb.Account == "" {
			return
		}
	}
	s.log.Info("bot moves to the next account", "from", b.Account, "to", nb.Account)
	s.start(nb.Account)
}

// arm sets the timer for the earliest cool-down end.
func (s *Supervisor) arm() {
	var next time.Time
	for _, st := range s.runs {
		if !st.retryAt.IsZero() && (next.IsZero() || st.retryAt.Before(next)) {
			next = st.retryAt
		}
	}
	switch {
	case next.IsZero():
		if s.timer != nil {
			s.timer.Stop()
		}
	case s.timer == nil:
		s.timer = s.cfg.Clock.NewTimer(next.Sub(s.cfg.Clock.Now()))
	default:
		s.timer.Reset(next.Sub(s.cfg.Clock.Now()))
	}
}

// wake restarts the bots whose cool-down ended.
func (s *Supervisor) wake() {
	now := s.cfg.Clock.Now()
	for account, st := range s.runs {
		if !st.retryAt.IsZero() && !st.retryAt.After(now) {
			st.retryAt = time.Time{}
			s.start(account)
		}
	}
	s.arm()
}

// do runs fn on the supervisor goroutine and returns once it ran.
func (s *Supervisor) do(ctx context.Context, fn func()) error {
	ran := make(chan struct{})
	select {
	case s.reqs <- func() { defer close(ran); fn() }:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrStopped
	}
	<-ran
	return nil
}

// List returns every bot of the registry with its run state.
func (s *Supervisor) List(ctx context.Context) ([]Status, error) {
	var out []Status
	err := s.do(ctx, func() {
		for _, b := range s.cfg.Registry.Bots() {
			st := s.runs[b.Account]
			v := Status{Bot: b, Run: st.state()}
			if v.Run == RunCoolingDown {
				v.RestartAt = st.retryAt
			}
			out = append(out, v)
		}
	})
	return out, err
}

// Create adds a new bot of preset on the next account of the panel's prefix
// and spawns it. A nil class draws one of the starting classes; either way
// the registry stores it. It is refused at MaxOnline.
func (s *Supervisor) Create(ctx context.Context, preset string, class *int32) (Bot, error) {
	var b Bot
	var err error
	if derr := s.do(ctx, func() { b, err = s.create(preset, class) }); derr != nil {
		return Bot{}, derr
	}
	return b, err
}

func (s *Supervisor) create(preset string, class *int32) (Bot, error) {
	if err := s.checkPreset(preset); err != nil {
		return Bot{}, err
	}
	classes := botclient.StartingClasses()
	c := classes[s.rng.IntN(len(classes))].ID
	if class != nil {
		if !slices.ContainsFunc(classes, func(sc botclient.StartingClass) bool { return sc.ID == *class }) {
			return Bot{}, fmt.Errorf("class %d: not a starting class", *class)
		}
		c = *class
	}
	if s.online() >= s.cfg.MaxOnline {
		return Bot{}, ErrMaxOnline
	}
	b, err := s.cfg.Registry.NewBot(Bot{Class: &c, Preset: preset, Online: true})
	if b.Account == "" {
		return Bot{}, err
	}
	s.start(b.Account)
	return b, err
}

func (s *Supervisor) checkPreset(preset string) error {
	if s.cfg.CheckPreset == nil {
		return nil
	}
	return s.cfg.CheckPreset(preset)
}

// Import adds accounts 1 to n of prefix, whose names carry marker m, as
// offline never-create bots of ImportPreset (Registry.Import).
func (s *Supervisor) Import(ctx context.Context, prefix string, m botnames.Marker, n int) ([]Bot, error) {
	var bots []Bot
	var err error
	if derr := s.do(ctx, func() { bots, err = s.cfg.Registry.Import(prefix, m, n, ImportPreset) }); derr != nil {
		return nil, derr
	}
	return bots, err
}

// Spawn marks an offline bot online and starts it; past MaxOnline it is
// refused. A bot whose despawn has not reported yet starts once it has.
func (s *Supervisor) Spawn(ctx context.Context, account string) error {
	return s.request(ctx, func() error {
		b, ok := s.cfg.Registry.Bot(account)
		switch {
		case !ok:
			return ErrNoBot
		case b.Online:
			return ErrOnline
		}
		st := s.runs[account]
		if (st == nil || st.cancel == nil) && s.online() >= s.cfg.MaxOnline {
			return ErrMaxOnline
		}
		s.update(account, func(b *Bot) { b.Online, b.StopReason = true, "" })
		if st != nil && st.cancel != nil {
			st.restart = true
			return nil
		}
		s.start(account)
		return nil
	})
}

// Despawn logs a bot out and marks it offline; it stays in the registry.
func (s *Supervisor) Despawn(ctx context.Context, account string) error {
	return s.request(ctx, func() error {
		if _, ok := s.cfg.Registry.Bot(account); !ok {
			return ErrNoBot
		}
		st := s.runs[account]
		if st != nil && st.cancel != nil {
			st.stopping, st.restart = true, false
			st.cancel()
			s.update(account, func(b *Bot) { b.Online = false })
			return nil
		}
		delete(s.runs, account)
		s.arm()
		s.stopped(account, false, reasonDespawned)
		return nil
	})
}

// Respawn logs an online bot out and back in: its new run starts once the
// old one reported, so the server never sees the account twice. A halted or
// cooling-down bot starts at once.
func (s *Supervisor) Respawn(ctx context.Context, account string) error {
	return s.request(ctx, func() error { return s.respawn(account) })
}

func (s *Supervisor) respawn(account string) error {
	b, ok := s.cfg.Registry.Bot(account)
	switch {
	case !ok:
		return ErrNoBot
	case !b.Online:
		return ErrOffline
	}
	st := s.runs[account]
	if st != nil && st.cancel != nil {
		st.stopping, st.restart = true, true
		st.cancel()
		return nil
	}
	delete(s.runs, account)
	s.arm()
	s.start(account)
	return nil
}

// SetPreset changes a bot's preset; an online bot respawns with it.
func (s *Supervisor) SetPreset(ctx context.Context, account, preset string) error {
	return s.request(ctx, func() error {
		b, ok := s.cfg.Registry.Bot(account)
		if !ok {
			return ErrNoBot
		}
		if err := s.checkPreset(preset); err != nil {
			return err
		}
		if err := s.cfg.Registry.Update(account, func(b *Bot) { b.Preset = preset }); err != nil {
			return err
		}
		if !b.Online {
			return nil
		}
		return s.respawn(account)
	})
}

// Delete drops an offline bot and keeps a tombstone of its account: no new
// bot ever takes it.
func (s *Supervisor) Delete(ctx context.Context, account string) error {
	return s.request(ctx, func() error {
		b, ok := s.cfg.Registry.Bot(account)
		switch {
		case !ok:
			return ErrNoBot
		case b.Online || s.runs[account] != nil:
			return ErrOnline
		}
		return s.cfg.Registry.Drop(account, reasonDeleted, s.cfg.Clock.Now())
	})
}

// ResumeLogins resets the credential breaker (botclient.LoginGate.Reset)
// and restarts the bots it ended.
func (s *Supervisor) ResumeLogins(ctx context.Context) error {
	return s.request(ctx, func() error {
		s.cfg.Gate.Reset()
		for account, st := range s.runs {
			if st.halted {
				st.halted = false
				s.start(account)
			}
		}
		return nil
	})
}

// request runs fn on the supervisor goroutine and returns its error.
func (s *Supervisor) request(ctx context.Context, fn func() error) error {
	var err error
	if derr := s.do(ctx, func() { err = fn() }); derr != nil {
		return derr
	}
	return err
}
