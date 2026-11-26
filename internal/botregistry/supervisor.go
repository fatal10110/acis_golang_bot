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
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

const (
	// CoolDown is how long a bot the server aborted repeatedly
	// (botclient.ErrRepeatedAborts) waits before it restarts by itself: a
	// run of home line drops can end every bot at once.
	CoolDown = 10 * time.Minute
	// ImportPreset is the preset of an imported bot: the default one, so
	// an imported swarm farms alone as a fresh bot does (D21).
	ImportPreset = "farmer"
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
	// ErrNoRun refuses a command to a bot with no run playing: offline,
	// stopped, halted, cooling down, or logging out for a respawn.
	ErrNoRun = errors.New("bot is not running")
	// ErrBranch refuses a branch pin that is not a first profession of the
	// bot's base class (botclient.BranchPins).
	ErrBranch = errors.New("not a branch of the bot's class")
)

// Run is one run of a registry bot: what a RunFunc plays.
type Run struct {
	Account string
	// Character is the bot's character from the registry: its known name,
	// or its derived names with its class or the never-create flag.
	Character botclient.Character
	Preset    string
	// Branch is the bot's pinned branch (Bot.Branch), 0 for none: the
	// brain config's (botclient.BrainConfig.Branch).
	Branch int32
	// PvP is the bot's PvP mode switches (Bot.PvP), nil for its preset's
	// defaults.
	PvP *PvPModes
	// Markers are the markers of every prefix of the registry, for the bot
	// filter, as of the run's start.
	Markers botnames.Markers
	// Metrics is the bot's slot: one per registry bot, reused by every
	// run; nil without a metrics registry.
	Metrics *botmetrics.Bot
	// Learned hands the supervisor the character the run learned; it is
	// botclient.BotConfig.Learned, which a bot calls while it holds its
	// login gate turn. It never waits on the supervisor: it posts the
	// report to the supervisor's mailbox and returns. The supervisor saves
	// the name and class to the registry file as soon as it takes the
	// report, and always before the run's end report, which the run posts
	// after it: a run that ends right after reporting never loses them. It
	// is called before the RunFunc returns.
	Learned func(name string, classID int32)
	// Live hands the supervisor the run's bot once the RunFunc built it,
	// before it runs: until the run reports its end, the supervisor reads
	// its status (Status.Live) and passes it commands (Command). Like
	// Learned it posts to the mailbox and never waits.
	Live func(b *botclient.Bot)
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
	// breaker, SetLoginSpacing sets its spacing, and New applies the
	// spacing the registry kept.
	Gate *botclient.LoginGate
	// Clock times the cool-down, the registry flush and the stop times.
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
	// Party, when set, runs the bot party organizer on its policy every
	// PartyTick (docs/parties/organizer.md); nil runs none. The control
	// panel sets it, a swarm has no organizer (D25).
	Party *botparty.Policy
}

// PartyTick is how often the organizer plans the bot parties.
const PartyTick = 30 * time.Second

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
	// Live is the latest snapshot of the bot of its live run
	// (botclient.Bot.Status); nil while no run handed one over (Run.Live).
	Live *botclient.Status
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
	// bot is the live run's bot once it handed it over (Run.Live), nil
	// otherwise.
	bot runBot
	// newRun marks a bot handed over since the last party pass. sent is
	// the last organizer order handed to the bot and what became of it;
	// answer is its answer channel while the answer is pending.
	newRun bool
	sent   botparty.Sent
	answer <-chan error
}

// runBot is the bot of a live run as the supervisor uses it: a
// *botclient.Bot, or a test's fake whose status and answers the test sets.
type runBot interface {
	Status() botclient.Status
	Command(c botclient.Command) (<-chan error, error)
	Inspect(wait time.Duration) (<-chan botclient.Inspection, error)
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

// FlushDelay is how long a deferred registry change (Registry.UpdateLater:
// a run's end, progress or online flag) waits before the supervisor writes
// the file; every change in that time is written by the one save.
const FlushDelay = 3 * time.Second

// event is a run's report: its bot, the character it learned, or its end.
type event struct {
	account string
	// bot is set for the run's bot handed over (Run.Live).
	bot runBot
	// learned is set for a learned character, with name and class.
	learned bool
	name    string
	class   int32
	err     error
}

// Supervisor owns the registry, its file and every bot's run (docs/plan.md
// section 8, "Ownership"). Run is its goroutine; the request methods (List,
// Create, Import, Spawn, Despawn, Respawn, SetPreset, Delete, ResumeLogins,
// SetLoginSpacing, Command)
// are safe from any goroutine and answer at once, without waiting on a run.
// It sends no packet: every run logs in through the login gate, which paces
// it.
type Supervisor struct {
	cfg  Config
	reqs chan func()
	mail *mailbox
	done chan struct{}
	wg   sync.WaitGroup

	// Owned by Run.
	runs  map[string]*runState
	slots map[string]*botmetrics.Bot
	timer botclient.Timer
	// flush is the registry flush timer, running while flushing is set.
	flush    botclient.Timer
	flushing bool
	// party is the organizer's state, partyTimer its PartyTick timer, nil
	// without an organizer (Config.Party).
	party      botparty.State
	partyTimer botclient.Timer
	base       context.Context
	closing    bool
	rng        *rand.Rand
	log        *slog.Logger
}

// New returns the supervisor of cfg; Run starts it.
func New(cfg Config) (*Supervisor, error) {
	switch {
	case cfg.Registry == nil, cfg.Run == nil, cfg.Gate == nil, cfg.Clock == nil:
		return nil, errors.New("supervisor: registry, run, gate and clock are required")
	case cfg.MaxOnline < 1:
		return nil, fmt.Errorf("supervisor: max online %d: want 1 or more", cfg.MaxOnline)
	case cfg.Party != nil && (cfg.Party.Size < 2 || cfg.Party.Size > botparty.MaxSize):
		return nil, fmt.Errorf("supervisor: party size %d: want 2 to %d", cfg.Party.Size, botparty.MaxSize)
	}
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if d := cfg.Registry.LoginSpacing(); d > 0 {
		// A spacing kept under an allowlisted gate may be under a spaced
		// gate's floor: the gate keeps its own then.
		if err := cfg.Gate.SetSpacing(d); err != nil {
			log.Warn("kept login spacing not applied", "err", err, "spacing", cfg.Gate.Spacing())
		}
	}
	return &Supervisor{
		cfg:   cfg,
		reqs:  make(chan func()),
		mail:  newMailbox(),
		done:  make(chan struct{}),
		runs:  make(map[string]*runState),
		slots: make(map[string]*botmetrics.Bot),
		rng:   rng,
		log:   log,
	}, nil
}

// Run restores the bots marked online, up to MaxOnline, then serves
// requests and run reports until ctx ends. It then cancels every run, keeps
// draining reports until all arrived, joins every run goroutine, writes the
// registry file once and returns. The bots stay marked online, so the next
// start restores them. Call it once.
func (s *Supervisor) Run(ctx context.Context) error {
	defer close(s.done)
	s.base = context.WithoutCancel(ctx)
	s.restore()
	if s.cfg.Party != nil {
		s.partyTimer = s.cfg.Clock.NewTimer(PartyTick)
	}
	for {
		var tick, flush, party <-chan time.Time
		if s.timer != nil {
			tick = s.timer.C()
		}
		if s.partyTimer != nil {
			party = s.partyTimer.C()
		}
		if s.armFlush() {
			flush = s.flush.C()
		}
		select {
		case <-ctx.Done():
			s.shutdown()
			return nil
		case fn := <-s.reqs:
			fn()
		case <-s.mail.ready:
			s.handleMail()
		case <-tick:
			s.wake()
		case <-flush:
			s.flushing = false
			s.save()
		case <-party:
			s.partyPass()
			s.partyTimer.Reset(PartyTick)
		}
	}
}

// shutdown cancels every run, handles reports until none is live, joins
// every run goroutine and writes the registry's deferred changes.
func (s *Supervisor) shutdown() {
	s.closing = true
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.flush != nil {
		s.flush.Stop()
	}
	if s.partyTimer != nil {
		s.partyTimer.Stop()
	}
	for _, st := range s.runs {
		if st.cancel != nil {
			st.cancel()
		}
	}
	for s.live() > 0 {
		<-s.mail.ready
		s.handleMail()
	}
	s.wg.Wait()
	s.save()
}

// handleMail applies the reports in the mailbox, in the order posted.
func (s *Supervisor) handleMail() {
	for _, e := range s.mail.take() {
		s.handle(e)
	}
}

// armFlush starts the flush timer when the registry holds deferred changes
// and the timer does not run; it reports whether the timer runs. A save in
// the meantime writes them, and the flush then finds nothing to write.
func (s *Supervisor) armFlush() bool {
	switch {
	case s.flushing || !s.cfg.Registry.Dirty():
	case s.flush == nil:
		s.flush = s.cfg.Clock.NewTimer(FlushDelay)
		s.flushing = true
	default:
		s.flush.Reset(FlushDelay)
		s.flushing = true
	}
	return s.flushing
}

// save writes the registry's deferred changes, if any.
func (s *Supervisor) save() {
	if err := s.cfg.Registry.Flush(); err != nil {
		s.log.Error("registry save failed", "err", err)
	}
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
		Branch:    b.Branch,
		PvP:       b.PvP,
		Markers:   s.cfg.Registry.Markers(),
		Metrics:   slot,
		Learned: func(name string, classID int32) {
			s.mail.post(event{account: account, learned: true, name: name, class: classID})
		},
		Live: func(b *botclient.Bot) {
			s.mail.post(event{account: account, bot: b})
		},
	}
	s.wg.Go(func() {
		err := s.cfg.Run(ctx, r)
		s.mail.post(event{account: account, err: err})
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

// stopped records the end of account's run with reason and the online flag,
// deferred (update).
func (s *Supervisor) stopped(account string, online bool, reason string) {
	now := s.cfg.Clock.Now()
	s.update(account, func(b *Bot) {
		b.Online = online
		b.StoppedAt, b.StopReason = now, reason
	})
}

// update applies fn to account's bot in memory; the flush timer, the next
// save or the shutdown writes it (Registry.UpdateLater). It is for the
// online flag, the stop record and the progress only.
func (s *Supervisor) update(account string, fn func(*Bot)) {
	if err := s.cfg.Registry.UpdateLater(account, fn); err != nil {
		s.log.Error("registry update failed", "account", account, "err", err)
	}
}

// handle applies a run's report.
func (s *Supervisor) handle(e event) {
	if e.bot != nil {
		// A run hands its bot over before it reports its end, so its run
		// state is live. A new bot holds no order of the organizer: the
		// old one's mailbox answers ErrStopped.
		st := s.runs[e.account]
		st.bot, st.newRun = e.bot, true
		st.sent, st.answer = botparty.Sent{}, nil
		return
	}
	if e.learned {
		// Saved at once, with every deferred change (docs/plan.md section 8,
		// "Character pinning").
		class := e.class
		if err := s.cfg.Registry.Update(e.account, func(b *Bot) { b.Name, b.Class = e.name, &class }); err != nil {
			s.log.Error("registry update failed", "account", e.account, "err", err)
		}
		s.log.Info("bot learned its character", "account", e.account, "character", e.name, "class", e.class)
		return
	}
	st := s.runs[e.account]
	st.cancel()
	st.cancel = nil
	if st.bot != nil {
		snap := st.bot.Status()
		s.update(e.account, func(b *Bot) { keepProgress(b, snap) })
		st.bot, st.newRun = nil, false
		st.sent, st.answer = botparty.Sent{}, nil
	}
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

// keepProgress copies the level, class and position of a run's last
// snapshot into its registry bot (docs/plan.md section 8, "Status"), so a
// class changed at the class master is kept; a snapshot of a bot that never
// entered the world (level 0) keeps the stored ones.
func keepProgress(b *Bot, st botclient.Status) {
	if st.Level <= 0 {
		return
	}
	b.Level = st.Level
	class := st.ClassID
	b.Class = &class
	b.Position = &Position{X: st.Position.X, Y: st.Position.Y, Z: st.Position.Z}
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
			if st != nil && st.bot != nil {
				snap := st.bot.Status()
				v.Live = &snap
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

// SetBranch pins a bot's branch, a first profession of its base class
// (botclient.BranchPins), or clears the pin with 0 (D22). An online bot
// whose pin changed respawns with it. A bot whose class is not known yet
// (an imported one) takes any table's branch; its brain ignores one of
// another base class.
func (s *Supervisor) SetBranch(ctx context.Context, account string, branch int32) error {
	return s.request(ctx, func() error {
		b, ok := s.cfg.Registry.Bot(account)
		if !ok {
			return ErrNoBot
		}
		if err := checkBranch(b, branch); err != nil {
			return err
		}
		if branch == b.Branch {
			return nil
		}
		if err := s.cfg.Registry.Update(account, func(b *Bot) { b.Branch = branch }); err != nil {
			return err
		}
		if !b.Online {
			return nil
		}
		return s.respawn(account)
	})
}

// SetPvP sets a bot's PvP mode switches, or with nil gives it back its
// preset's defaults (docs/pvp/modes.md). An online bot whose switches
// changed respawns with them, through the login gate like any respawn.
func (s *Supervisor) SetPvP(ctx context.Context, account string, modes *PvPModes) error {
	return s.request(ctx, func() error {
		b, ok := s.cfg.Registry.Bot(account)
		if !ok {
			return ErrNoBot
		}
		if b.PvP == nil && modes == nil || b.PvP != nil && modes != nil && *b.PvP == *modes {
			return nil
		}
		var set *PvPModes
		if modes != nil {
			c := *modes
			set = &c
		}
		if err := s.cfg.Registry.Update(account, func(b *Bot) { b.PvP = set }); err != nil {
			return err
		}
		if !b.Online {
			return nil
		}
		return s.respawn(account)
	})
}

// checkBranch refuses a pin for b that is not 0 nor a first profession of
// b's base class, or of any base class while b's class is not known.
func checkBranch(b Bot, branch int32) error {
	if branch == 0 {
		return nil
	}
	base, ok := botclient.BranchBase(branch)
	if !ok {
		return fmt.Errorf("%w: class %d", ErrBranch, branch)
	}
	if b.Class == nil {
		return nil
	}
	if own, ok := datapack.BaseClass(*b.Class); ok && own != base {
		return fmt.Errorf("%w: class %d is a branch of class %d, the bot's base class is %d", ErrBranch, branch, base, own)
	}
	return nil
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

// SetLoginSpacing sets the login gate's spacing from the next dial on
// (botclient.LoginGate.SetSpacing, which refuses one under the gate's floor
// with botclient.ErrSpacing) and keeps it in the registry for the next
// start.
func (s *Supervisor) SetLoginSpacing(ctx context.Context, d time.Duration) error {
	return s.request(ctx, func() error {
		if err := s.cfg.Gate.SetSpacing(d); err != nil {
			return err
		}
		s.log.Info("login spacing set", "spacing", d)
		return s.cfg.Registry.SetLoginSpacing(d)
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

// Command hands c to the bot of account's live run (botclient.Bot.Command)
// and returns the channel its one answer comes on. It never waits for the
// bot: a full mailbox answers botclient.ErrBusy, and a bot with no run
// playing ErrNoRun. No command sends a whisper; the bot's brain paces every
// packet it causes.
func (s *Supervisor) Command(ctx context.Context, account string, c botclient.Command) (<-chan error, error) {
	var answer <-chan error
	err := s.request(ctx, func() error {
		if _, ok := s.cfg.Registry.Bot(account); !ok {
			return ErrNoBot
		}
		st := s.runs[account]
		if st == nil || st.bot == nil || st.stopping {
			return ErrNoRun
		}
		var err error
		answer, err = st.bot.Command(c)
		return err
	})
	return answer, err
}

// Inspect hands an inspect to the bot of account's live run
// (botclient.Bot.Inspect), which answers by wait at the latest, and returns
// the channel its one Inspection comes on. It never waits for the bot: a
// bot with no run playing answers ErrNoRun, one between sessions
// botclient.ErrOffline, a full mailbox botclient.ErrBusy.
func (s *Supervisor) Inspect(ctx context.Context, account string, wait time.Duration) (<-chan botclient.Inspection, error) {
	var answer <-chan botclient.Inspection
	err := s.request(ctx, func() error {
		if _, ok := s.cfg.Registry.Bot(account); !ok {
			return ErrNoBot
		}
		st := s.runs[account]
		if st == nil || st.bot == nil || st.stopping {
			return ErrNoRun
		}
		var err error
		answer, err = st.bot.Inspect(wait)
		return err
	})
	return answer, err
}
