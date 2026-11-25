package botclient

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"syscall"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// ProtocolRevision is the game protocol revision the bot announces:
// Interlude's 746, one the server accepts (server internal/gameserver/
// network/dispatch.go validProtocolRevision).
const ProtocolRevision int32 = 746

// LoginFail reasons (server internal/loginserver/network/serverpackets/
// loginfail.go) the bot tells apart.
const (
	loginFailSystemError      int32 = 0x01
	loginFailPasswordWrong    int32 = 0x02
	loginFailUserOrPassWrong  int32 = 0x03
	loginFailAccessFailed     int32 = 0x04
	loginFailAccountInUse     int32 = 0x07
	loginFailServerOverloaded int32 = 0x0f
	loginFailMaintenance      int32 = 0x10
)

// Session timing and limits.
const (
	// eventLimit bounds each session's event queue.
	eventLimit = 256
	// retryBase and retryMax bound a bot's own backoff between attempts.
	retryBase = 2 * time.Second
	retryMax  = 5 * time.Minute
	// stableSession is how long a session must stay in the world to reset
	// the bot's backoff and abort count.
	stableSession = 10 * time.Minute
	// maxAborts is how many sessions in a row the server may abort (close
	// with no ServerClose: the 2 MiB outbound cap of a slow reader, server
	// internal/gameserver/network/conn.go outboundHighWater; or a reset or
	// EOF) before the bot stops. A dead line the dialer's timeouts end
	// (deadLine) is no abort.
	maxAborts = 3
	// inUseWindow is how long a bot keeps retrying AccountInUse, from the
	// first one after a run start or an end that was not a kick. After a
	// dead line the server holds the old session; each AccountInUse makes
	// the login server kick it (server internal/loginserver/clientlink.go,
	// roster.kick), and its writer closes within about a minute (its write
	// deadline to the dead peer). Past the window the account counts as
	// taken by another login.
	inUseWindow = 3 * time.Minute
	// inUseRetryMax caps the backoff between AccountInUse retries.
	inUseRetryMax = 30 * time.Second
	// shutdownGrace bounds the logout a canceled bot runs, refusals (in
	// combat) included; past it the connection is closed and the character
	// lingers as after a drop.
	shutdownGrace = 30 * time.Second
	// kickWait plus a jitter below kickJitter is the least a bot waits to
	// log in again after the server's ServerClose. A kick by another login
	// only evicts the holder: the login that caused it was refused with
	// AccountInUse (server internal/loginserver/clientlink.go, roster.kick),
	// and its client must log in again. The wait leaves it that time
	// instead of racing it back into the account.
	kickWait   = 60 * time.Second
	kickJitter = 30 * time.Second
)

var (
	// ErrAccountTaken stops a bot whose account another client holds: a
	// kick by another login is reported, not fought.
	ErrAccountTaken = errors.New("account in use by another login")
	// ErrRepeatedAborts stops a bot the server aborted maxAborts times in a
	// row.
	ErrRepeatedAborts = errors.New("server aborted the session repeatedly")
	// ErrCommandedLogout stops a bot a GM's whispered "logout" logged out:
	// it stays out until the swarm restarts, and the session is no abort.
	ErrCommandedLogout = errors.New("logged out on a GM command")
	// errConnLost reports a game connection that ended under the brain.
	errConnLost = errors.New("game connection lost")
)

// connLost is queued by the session's reader goroutine after ReadEvents
// ends with an error, behind every event read before: the brain sees a
// ServerClose or LeaveWorld before it.
type connLost struct{ err error }

func (connLost) event() {}

// SessionConfig is what every bot of a process shares.
type SessionConfig struct {
	// LoginAddr is the login server's host:port.
	LoginAddr string
	// GameAddr, when set, replaces the game address the login server
	// advertises.
	GameAddr string
	ServerID byte
	// Timeout bounds each read and write of a login or game entry.
	Timeout time.Duration
	// Dial applies to every login and game dial (WithLocalAddr).
	Dial []DialOption
	// Recorder is the frame tap of every game connection: the -record
	// Recorder, or the e2e harness's watch; nil taps nothing.
	Recorder FrameTap
	// Persistent bots reconnect after a session in the world ends; others
	// stop.
	Persistent bool
	// Markers are the markers of every bot name the process knows: no
	// brain answers a whisper from a name one of them marks, so bots of
	// different prefixes see each other as bots. None takes
	// botnames.DefaultMarker. Each bot names its own character with one
	// marker, through its Character.
	Markers botnames.Markers
	Clock   Clock
	Gate    *LoginGate
	// Log receives the bot's reports; nil discards them.
	Log *slog.Logger
}

// BotConfig is what is a bot's own: its character, its brain and who hears
// of the character it learns.
type BotConfig struct {
	// Character is the bot's character (see Character); the Bot pins its
	// name once a session chose or created it.
	Character Character
	// Brain configures each session's brain, from the bot's preset; Rand,
	// LogoutLimit and the markers are replaced per session. Its Metrics
	// also count the bot's reconnects and server aborts.
	Brain BrainConfig
	// Learned, when set, is called once per Bot, on Run's goroutine, with
	// the name and class id of the character it learned: one chosen or
	// created by a session of a bot whose Character had no known name. It
	// is called before the session enters the world, while the bot holds
	// its login gate turn, so it must not block: every other bot's login
	// waits for that turn. The owner takes the report without waiting and
	// saves it later (botregistry.Run.Learned).
	Learned func(name string, classID int32)
	// Preset names the persona preset the bot plays, for its status.
	Preset string
}

// Bot is one account's player: a loop of session attempts, one at a time,
// each with its own connection, world model, event queue, reader and brain.
// Run is its goroutine, called once. Status and Command are safe from any
// goroutine.
type Bot struct {
	cfg      SessionConfig
	brain    BrainConfig
	learned  func(name string, classID int32)
	account  string
	password string
	net      sessionNet
	rng      *rand.Rand
	log      *slog.Logger

	// Owned by Run.
	//
	// char is the bot's character; its name is pinned once known (pin).
	char  Character
	retry backoff
	// inUseSince is when the first AccountInUse since the last good login
	// came; zero while none came.
	inUseSince time.Time
	// aborts counts sessions aborted since the last stable one.
	aborts int
	// kicked is set while the last session ended on the server's
	// ServerClose and no login has succeeded since.
	kicked bool
	// chat is the whisper replies of every session, for the chat caps;
	// each session's brain owns it while it runs.
	chat *chatLog
	// valuables is the valuables ledger and round of every session; each
	// session's brain owns it while it runs.
	valuables *valuables
	// reconnects counts the attempts after the first.
	reconnects int

	// link is the bot's mailbox, standing order and status board; Run
	// and each session's brain own the order and write the board.
	link botLink

	// mu guards stopped, set once Run returned, and inSession, set while
	// a session's brain runs (Command).
	mu        sync.Mutex
	stopped   bool
	inSession bool
}

// NewBot returns the bot of account, playing bot's character with bot's
// brain. A character that could not be played fails here.
func NewBot(cfg SessionConfig, account, password string, bot BotConfig, rng *rand.Rand) (*Bot, error) {
	ch := bot.Character
	if err := ch.Validate(); err != nil {
		return nil, fmt.Errorf("account %s: %w", account, err)
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	b := &Bot{
		cfg:       cfg,
		brain:     bot.Brain,
		learned:   bot.Learned,
		account:   account,
		password:  password,
		char:      ch,
		rng:       rand.New(rand.NewPCG(rng.Uint64(), rng.Uint64())),
		log:       log.With("account", account),
		retry:     backoff{base: retryBase, max: retryMax},
		chat:      newChatLog(),
		valuables: newValuables(),
	}
	b.net = netDialer{cfg: &b.cfg}
	b.link.mail = make(chan mail, mailboxSize)
	b.link.board.store(Status{State: StateNew, Character: ch.Name, Preset: bot.Preset})
	return b, nil
}

// Account is the bot's account name.
func (b *Bot) Account() string { return b.account }

// Run plays sessions until ctx ends, which logs the bot out and returns nil,
// or until the bot stops for good, which returns why: the credential
// breaker (ErrLoginsHalted), a refused credential, a banned account or
// character, a character that cannot be created, a character choice that
// stops (ErrCharacterMissing, ErrAccountHasCharacters,
// ErrNoCharacterToAdopt), ErrAccountTaken,
// ErrRepeatedAborts, ErrCommandedLogout, or, for a bot not persistent, the
// end of its session. It publishes the bot's state on the way (Status);
// once it returns the bot is stopped and refuses commands.
func (b *Bot) Run(ctx context.Context) (err error) {
	defer func() {
		b.publish(StateStopped, cmp.Or(err, ctx.Err()))
		b.stop()
	}()
	for {
		entered, played, err := b.attempt(ctx)
		if ctx.Err() != nil {
			return nil
		}
		delay, stop := b.after(entered, played, err)
		if stop != nil {
			return stop
		}
		b.log.Info("bot reconnects", "in", delay.Round(time.Millisecond), "after", err)
		b.publish(StateReconnecting, err)
		if !b.wait(ctx, delay) {
			return nil
		}
		b.reconnects++
		b.brain.Metrics.Reconnect()
	}
}

// attempt runs one session: a login and a game entry in a turn of the gate,
// then play. entered reports a session that reached the world, played how
// long it stayed.
func (b *Bot) attempt(ctx context.Context) (entered bool, played time.Duration, err error) {
	b.publish(StateWaiting, nil)
	turn, err := b.cfg.Gate.Take(ctx)
	if err != nil {
		return false, 0, err
	}
	b.publish(StateEntering, nil)
	g, self, err := b.enter(ctx, turn)
	turn.Done()
	if err != nil {
		return false, 0, err
	}
	start := b.cfg.Clock.Now()
	err = b.play(ctx, g, self)
	return true, b.cfg.Clock.Now().Sub(start), err
}

// enter logs in and enters the world, each dial paced by turn.
func (b *Bot) enter(ctx context.Context, turn *Turn) (gameSession, Self, error) {
	if err := turn.Dial(ctx); err != nil {
		return nil, Self{}, err
	}
	res, err := b.net.login(ctx, b.account, b.password)
	turn.Dialed(err)
	if err != nil {
		err = fmt.Errorf("login: %w", err)
		if credentialRefused(err) {
			// Trip the breaker while the turn is still held: a bot waiting
			// in Take must not get the turn and send its credentials first.
			b.cfg.Gate.Halt(fmt.Errorf("account %s: %w", b.account, err))
		}
		return nil, Self{}, err
	}
	// What ends this attempt now is a kick or not (a failed game entry is
	// not): either starts a new AccountInUse window.
	b.inUseSince, b.kicked = time.Time{}, false
	if err := turn.Dial(ctx); err != nil {
		return nil, Self{}, err
	}
	g, err := b.net.dialGame(ctx, cmp.Or(b.cfg.GameAddr, res.GameAddr))
	turn.Dialed(err)
	if err != nil {
		return nil, Self{}, err
	}
	self, err := b.net.enterGame(ctx, g, b.account, res.Keys, b.char, b.pin)
	if err != nil {
		g.Close()
		return nil, Self{}, fmt.Errorf("enter game: %w", err)
	}
	return g, self, nil
}

// pin makes the character a session chose or created the bot's known one:
// every later attempt of the run, reconnects included, selects it by name
// and never creates one. A name the bot did not know is reported to its
// owner once (BotConfig.Learned). It runs on Run's goroutine, inside
// enterGame.
func (b *Bot) pin(name string, classID int32) {
	if b.char.Name != "" {
		return
	}
	b.char = Character{Name: name}
	b.log.Info("character pinned", "name", name, "class_id", classID)
	if b.learned != nil {
		b.learned(name, classID)
	}
}

// play runs one session in the world: a fresh world model and event queue,
// the reader goroutine and the brain, until the brain ends. Once ctx ends it
// asks the brain to log out, which takes at most shutdownGrace. It closes
// the connection and joins the reader before it returns.
func (b *Bot) play(ctx context.Context, g gameSession, self Self) error {
	w, err := NewWorld(self, b.cfg.Clock.Now())
	if err != nil {
		g.Close()
		return err
	}
	q := NewEvents(eventLimit)
	done, logout := make(chan struct{}), make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		if err := g.ReadEvents(q, done); err != nil {
			q.push(connLost{err: err}, done)
		}
	}()
	cfg := b.brain
	cfg.Rand = rand.New(rand.NewPCG(b.rng.Uint64(), b.rng.Uint64()))
	cfg.LogoutLimit = shutdownGrace
	cfg.markers, cfg.chat, cfg.valuables, cfg.link, cfg.log = b.cfg.Markers, b.chat, b.valuables, &b.link, b.log
	cfg.account = b.account
	brain := NewBrain(w, q, g, b.cfg.Clock, cfg)
	stop := context.AfterFunc(ctx, func() { close(logout) })
	b.setInSession(true)
	err = brain.Run(done, logout)
	b.setInSession(false)
	stop()
	close(done)
	g.Close()
	<-readerDone
	return err
}

// after decides what follows an attempt that ended with err: the delay
// before the next one, or why the bot stops.
func (b *Bot) after(entered bool, played time.Duration, err error) (time.Duration, error) {
	if entered {
		return b.afterSession(played, err)
	}
	var le *LoginError
	var ge *GameStartError
	switch {
	case errors.Is(err, ErrLoginsHalted):
		return 0, err
	case errors.As(err, &le) && le.Packet == "LoginFail":
		switch le.Reason {
		case loginFailPasswordWrong, loginFailUserOrPassWrong:
			// enter already tripped the breaker.
			b.log.Error("credential refused: all logins halted", "err", err)
			return 0, err
		case loginFailAccountInUse:
			return b.afterInUse(err)
		case loginFailSystemError, loginFailAccessFailed, loginFailServerOverloaded, loginFailMaintenance:
			// AccessFailed reports the login server's own failures (a
			// database error, a login purged past its timeout), none of
			// which count toward the IP ban.
			return b.retry.next(b.rng), nil
		}
		return 0, err
	case errors.As(err, &le) && le.Packet == "PlayFail":
		// The server is full or not ready; a later try may pass.
		return b.retry.next(b.rng), nil
	case errors.As(err, &le): // AccountKicked: the account is banned.
		return 0, err
	case errors.As(err, &ge) && ge.Packet == "CharCreateFail",
		errors.Is(err, errCharacterUnplayable),
		errors.Is(err, errNoCharacterSlot),
		errors.Is(err, ErrCharacterMissing),
		errors.Is(err, ErrAccountHasCharacters),
		errors.Is(err, ErrNoCharacterToAdopt):
		return 0, err
	case isRefused(err):
		// The gate already backs every bot off.
		return 0, nil
	}
	return b.retry.next(b.rng), nil
}

// afterInUse decides after an AccountInUse. After the server's ServerClose
// another login holds the account: the bot stops at once. Otherwise the
// server may still hold the bot's own session of a dead line, which each
// AccountInUse evicts: the bot retries, at most inUseRetryMax apart, until
// inUseWindow has passed since the first one.
func (b *Bot) afterInUse(err error) (time.Duration, error) {
	now := b.cfg.Clock.Now()
	if b.inUseSince.IsZero() {
		b.inUseSince = now
	}
	if in := now.Sub(b.inUseSince); b.kicked || in >= inUseWindow {
		b.log.Warn("account taken by another login: bot stops", "kicked", b.kicked, "for", in.Round(time.Second), "err", err)
		return 0, fmt.Errorf("%w: %w", ErrAccountTaken, err)
	}
	return min(b.retry.next(b.rng), inUseRetryMax), nil
}

// afterSession decides after a session in the world: a logout a GM
// commanded, the server's ServerClose (a GM kick, a kick by another login,
// a shutdown), a dead line or an abort.
func (b *Bot) afterSession(played time.Duration, err error) (time.Duration, error) {
	if errors.Is(err, ErrCommandedLogout) {
		b.log.Info("logged out on a GM command: bot stops", "played", played.Round(time.Second))
		return 0, err
	}
	if played >= stableSession {
		b.retry.reset()
		b.aborts = 0
	}
	b.kicked = errors.Is(err, errServerClose) || errors.Is(err, errLeftWorld)
	dead := deadLine(err)
	if !b.kicked && !dead {
		b.aborts++
		b.brain.Metrics.Abort()
	}
	b.log.Warn("session ended", "played", played.Round(time.Second), "kicked", b.kicked, "deadLine", dead, "err", err)
	switch {
	case !b.cfg.Persistent:
		return 0, fmt.Errorf("session ended: %w", err)
	case b.aborts >= maxAborts:
		b.log.Error("server aborted the session repeatedly: bot stops", "aborts", b.aborts)
		return 0, fmt.Errorf("%w (%d since the last stable session): %w", ErrRepeatedAborts, b.aborts, err)
	}
	d := b.retry.next(b.rng)
	if b.kicked {
		d = max(d, kickWait+jitter(b.rng, kickJitter))
	}
	return d, nil
}

// deadLine reports a connection the kernel ended because the dialer's
// keepalive or user timeout found the line dead: ETIMEDOUT, or the ICMP
// unreachable error the kernel saw first (Linux tcp_write_err reports
// sk_err_soft when set). A reset or EOF is not one: the peer answered.
func deadLine(err error) bool {
	return errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH)
}

// credentialRefused reports a LoginFail the login server counts toward its
// IP ban (server internal/loginserver/clientlink.go, recordFailedAttempt).
func credentialRefused(err error) bool {
	le, ok := errors.AsType[*LoginError](err)
	return ok && le.Packet == "LoginFail" &&
		(le.Reason == loginFailPasswordWrong || le.Reason == loginFailUserOrPassWrong)
}

// wait waits d or until ctx ends, which it reports as false.
func (b *Bot) wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := b.cfg.Clock.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C():
		return true
	}
}

// sessionNet opens a bot's connections; netDialer is the real one, tests
// fake it.
type sessionNet interface {
	login(ctx context.Context, account, password string) (LoginResult, error)
	dialGame(ctx context.Context, addr string) (gameSession, error)
	enterGame(ctx context.Context, g gameSession, account string, keys SessionKeys, ch Character, chosen func(string, int32)) (Self, error)
}

// gameSession is a game connection: *GameConn.
type gameSession interface {
	Sender
	ReadEvents(q *Events, done <-chan struct{}) error
	Close() error
}

// netDialer opens real connections.
type netDialer struct{ cfg *SessionConfig }

func (n netDialer) login(ctx context.Context, account, password string) (LoginResult, error) {
	return Login(ctx, n.cfg.LoginAddr, account, password, n.cfg.ServerID, n.cfg.Timeout, n.cfg.Dial...)
}

func (n netDialer) dialGame(ctx context.Context, addr string) (gameSession, error) {
	g, err := DialGame(ctx, addr, ProtocolRevision, n.cfg.Timeout, n.cfg.Dial...)
	if err != nil {
		return nil, err
	}
	g.SetRecorder(n.cfg.Recorder)
	return g, nil
}

func (n netDialer) enterGame(ctx context.Context, g gameSession, account string, keys SessionKeys, ch Character, chosen func(string, int32)) (Self, error) {
	return EnterGame(ctx, g.(*GameConn), account, keys, ch, chosen)
}
