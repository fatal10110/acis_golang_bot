package e2e

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// MaxOnline is the most e2e characters a scenario keeps online at once,
// its GM's included.
const MaxOnline = 4

// MaxScenarios is the most scenarios a run keeps open at once, so a run
// keeps at most MaxScenarios*MaxOnline characters online.
const MaxScenarios = 2

// EntryTimeout bounds each read and write of a login or game entry.
const EntryTimeout = 15 * time.Second

// ErrLoginFailUsed reports a second wrong-password login in one run: the
// login server bans the IP after 3 in a row (LoginTryBeforeBan).
var ErrLoginFailUsed = errors.New("the run's one wrong-password login is used")

// ErrMoveRefused reports a move the server answered ActionFailed: no path
// to the point, or the character cannot move now.
var ErrMoveRefused = errors.New("move refused")

// network is how a Target reaches the servers; tests replace it.
type network interface {
	login(ctx context.Context, account, password string) (botclient.LoginResult, error)
	checkCredentials(ctx context.Context, account, password string) error
	dialGame(ctx context.Context, addr string) (*botclient.GameConn, error)
}

type netDialer struct{ addr string }

func (n netDialer) login(ctx context.Context, account, password string) (botclient.LoginResult, error) {
	return botclient.Login(ctx, n.addr, account, password, ServerID, EntryTimeout)
}

func (n netDialer) checkCredentials(ctx context.Context, account, password string) error {
	return botclient.CheckCredentials(ctx, n.addr, account, password, EntryTimeout)
}

func (n netDialer) dialGame(ctx context.Context, addr string) (*botclient.GameConn, error) {
	return botclient.DialGame(ctx, addr, botclient.ProtocolRevision, EntryTimeout)
}

// Target is one scenario of a run against a server. Every login of the run
// passes the run's login gate, spaced under the accept filter unless the
// config says the source address is allowlisted, and its credential breaker,
// which the first refused credential trips for the rest of the run and,
// through the marker it leaves (Config.HaltFile), for every later run. At
// most MaxScenarios scenarios of a run are open at once (Scenario), each with
// at most MaxOnline characters online (Hold), its own GM from the run's pool
// (EnterGM) and its own GM command scope. Its methods may be called from any
// goroutine.
type Target struct {
	*run
	// slots holds a token per character of the scenario online.
	slots chan struct{}

	mu    sync.Mutex
	scope *Scope // guarded by mu
}

// run is what the scenarios of one run share.
type run struct {
	cfg    Config
	secret botaccounts.Secret
	clock  botclient.Clock
	gate   *botclient.LoginGate
	net    network
	// gms is the pool the scenarios lease their GMs from; roots, of a
	// minting run (Config.GMMint), the GM files that promote the GMs it
	// mints, nil otherwise.
	gms   *gmPool
	roots *gmPool
	// scenarios holds a token per scenario open.
	scenarios chan struct{}
	// loginFail is set once the run's one wrong-password login was sent.
	loginFail atomic.Bool
}

// NewTarget opens a run against cfg's server: it refuses a prefix that
// overlaps the old swarm's or panel (the owner's `bots ui` prefix, "" for
// none), the breaker's marker an earlier run left (Config.HaltFile), and a
// secret that is missing, readable by others or weak.
func NewTarget(cfg Config, panel string) (*Target, error) {
	if err := CheckPrefix(cfg.Prefix, panel); err != nil {
		return nil, err
	}
	if err := checkHalt(cfg.HaltFile()); err != nil {
		return nil, err
	}
	secret, err := botaccounts.LoadSecret(cfg.SecretFile)
	if err != nil {
		return nil, err
	}
	if err := secret.Strong(); err != nil {
		return nil, fmt.Errorf("secret file %s: %w", cfg.SecretFile, err)
	}
	return newTarget(cfg, secret, botclient.SystemClock{}, netDialer{addr: cfg.Login}), nil
}

func newTarget(cfg Config, secret botaccounts.Secret, clock botclient.Clock, n network) *Target {
	gate := botclient.NewLoginGate(clock, nil)
	if cfg.Allowlisted {
		gate = botclient.NewAllowlistedLoginGate(clock, nil)
	}
	r := &run{
		cfg: cfg, secret: secret, clock: clock, gate: gate, net: n,
		gms:       newGMPool(cfg.GMFiles, clock),
		scenarios: make(chan struct{}, MaxScenarios),
	}
	if cfg.GMMint != "" {
		r.roots, r.gms = r.gms, newMintPool(cfg.GMMint, cfg.Prefix, clock)
	}
	return r.target()
}

func (r *run) target() *Target {
	return &Target{run: r, slots: make(chan struct{}, MaxOnline), scope: newScope()}
}

// Scenario opens another scenario of t's run, waiting while MaxScenarios
// are open; done closes it, once, after its characters are offline. It
// refuses to open once the breaker is tripped, by this run or, through its
// marker, by another.
func (t *Target) Scenario(ctx context.Context) (s *Target, done func(), err error) {
	if err := t.gate.Err(); err != nil {
		return nil, nil, err
	}
	if err := checkHalt(t.cfg.HaltFile()); err != nil {
		return nil, nil, err
	}
	select {
	case t.scenarios <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	var once sync.Once
	return t.run.target(), func() { once.Do(func() { <-t.scenarios }) }, nil
}

// Config is the run's config.
func (t *Target) Config() Config { return t.cfg }

// Account returns account n (from 1) of a scenario group: the prefix, the
// group (lowercase letters) and n, with its derived password. Each group
// keeps its own accounts, so what one scenario leaves on its characters
// never meets another.
func (t *Target) Account(group string, n int) (name, password string) {
	name = t.cfg.Prefix + group + strconv.Itoa(n)
	return name, t.secret.Password(name)
}

// Password is account's derived password. Never log it.
func (t *Target) Password(account string) string { return t.secret.Password(account) }

// Hold takes one of the MaxOnline places for a character about to go
// online, waiting while all are taken; release gives it back, once.
func (t *Target) Hold(ctx context.Context) (release func(), err error) {
	select {
	case t.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var once sync.Once
	return func() { once.Do(func() { <-t.slots }) }, nil
}

// SessionConfig is a botclient session config on the run's login gate, for
// a scenario that runs a whole Bot; each such bot takes a place (Hold).
func (t *Target) SessionConfig(log *slog.Logger) botclient.SessionConfig {
	return botclient.SessionConfig{
		LoginAddr: t.cfg.Login,
		ServerID:  ServerID,
		Timeout:   EntryTimeout,
		Clock:     t.clock,
		Gate:      t.gate,
		Log:       log,
	}
}

// Halted is the credential breaker's error once a refused credential
// tripped it, else nil.
func (t *Target) Halted() error { return t.gate.Err() }

// Own makes name (objectID 0 when not known yet) one of the scenario's
// characters, which GM commands may then touch.
func (t *Target) Own(name string, objectID int32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scope.addChar(name, objectID)
}

// OwnDoor makes the door id one the scenario may open and close. A door is
// the world's, not the run's: a scenario claims only doors no other
// scenario touches, and leaves each as the datapack starts it.
func (t *Target) OwnDoor(id int32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scope.doors[id] = true
}

// Enter logs account in and enters the world as ch on a place of its own,
// recording every game frame read into rec when it is set. A refused
// credential trips the breaker. The character becomes one of the
// scenario's.
func (t *Target) Enter(ctx context.Context, account, password string, ch botclient.Character, rec *botclient.Recorder) (*Client, error) {
	release, err := t.Hold(ctx)
	if err != nil {
		return nil, err
	}
	conn, self, err := t.enter(ctx, account, password, ch, rec)
	if err != nil {
		release()
		return nil, fmt.Errorf("account %s: %w", account, err)
	}
	t.Own(self.Name, self.ObjectID)
	return &Client{Account: account, Self: self, conn: conn, clock: t.clock, pacer: botclient.NewPacer(t.clock), release: release, pos: botclient.Point{X: self.X, Y: self.Y, Z: self.Z}}, nil
}

// accountInUse is the login server's LoginFail reason for an account a game
// session still holds (loginserver serverpackets/loginfail.go).
const accountInUse int32 = 0x07

// InUse reports a login the server refused because a game session still
// holds the account: one that just logged out, dropped or was kicked.
func InUse(err error) bool {
	le, ok := errors.AsType[*botclient.LoginError](err)
	return ok && le.Packet == "LoginFail" && le.Reason == accountInUse
}

// inUseRetry spaces the logins of EnterWhenFree.
const inUseRetry = 3 * time.Second

// EnterWhenFree is Enter with the account's derived password, tried again
// while the account is in use (InUse) until ctx ends.
func (t *Target) EnterWhenFree(ctx context.Context, account string, ch botclient.Character, rec *botclient.Recorder) (*Client, error) {
	return t.enterWhenFree(ctx, account, t.Password(account), ch, rec)
}

// enterWhenFree is Enter tried again while the account is in use (InUse)
// until ctx ends: the login server kicks the session still holding it.
func (t *Target) enterWhenFree(ctx context.Context, account, password string, ch botclient.Character, rec *botclient.Recorder) (*Client, error) {
	for {
		c, err := t.Enter(ctx, account, password, ch, rec)
		if !InUse(err) {
			return c, err
		}
		timer := t.clock.NewTimer(inUseRetry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, err
		case <-timer.C():
		}
	}
}

// enter takes a turn of the gate for the two dials of an entry.
func (t *Target) enter(ctx context.Context, account, password string, ch botclient.Character, rec *botclient.Recorder) (*botclient.GameConn, botclient.Self, error) {
	// The marker of a refusal in another process halts this one's next
	// login, not only its next scenario: a mint retried after another
	// process's refused one would be refused again.
	if err := checkHalt(t.cfg.HaltFile()); err != nil {
		return nil, botclient.Self{}, err
	}
	turn, err := t.gate.Take(ctx, account)
	if err != nil {
		return nil, botclient.Self{}, err
	}
	defer turn.Done()
	if err := turn.Dial(ctx); err != nil {
		return nil, botclient.Self{}, err
	}
	res, err := t.net.login(ctx, account, password)
	turn.Dialed(err)
	if err != nil {
		if botclient.CredentialRefused(err) {
			// Trip the breaker while the turn is held, as a Bot does.
			err = errors.Join(err, t.trip(account, err, true))
		}
		return nil, botclient.Self{}, fmt.Errorf("login: %w", err)
	}
	if err := turn.Dial(ctx); err != nil {
		return nil, botclient.Self{}, err
	}
	conn, err := t.net.dialGame(ctx, res.GameAddr)
	turn.Dialed(err)
	if err != nil {
		return nil, botclient.Self{}, err
	}
	conn.SetRecorder(rec)
	self, err := botclient.EnterGame(ctx, conn, account, res.Keys, ch, nil)
	if err != nil {
		conn.Close()
		return nil, botclient.Self{}, fmt.Errorf("enter game: %w", err)
	}
	// Reset the fleet backoff while the turn is held, as a Bot does.
	turn.Entered()
	return conn, self, nil
}

// CheckCredentials runs a login session for account up to the server list,
// through the gate: the first one creates the account (AutoCreateAccounts).
// A refused credential trips the breaker.
func (t *Target) CheckCredentials(ctx context.Context, account, password string) error {
	return t.checkCredentials(ctx, account, password, true)
}

// checkCredentials is CheckCredentials; a refusal leaves the breaker's
// marker only when persist is set.
func (t *Target) checkCredentials(ctx context.Context, account, password string, persist bool) error {
	turn, err := t.gate.Take(ctx, account)
	if err != nil {
		return err
	}
	defer turn.Done()
	if err := turn.Dial(ctx); err != nil {
		return err
	}
	err = t.net.checkCredentials(ctx, account, password)
	turn.Dialed(err)
	if botclient.CredentialRefused(err) {
		err = errors.Join(err, t.trip(account, err, persist))
	}
	return err
}

// trip halts every later login of the run after account's credential was
// refused with err and, when persist is set, leaves the breaker's marker
// (Config.HaltFile) that halts every later run. It returns the error of
// writing the marker, nil when it was written or not wanted.
func (t *Target) trip(account string, err error, persist bool) error {
	halt := fmt.Errorf("account %s: %w", account, err)
	t.gate.Halt(halt)
	path := t.cfg.HaltFile()
	if !persist || path == "" {
		return nil
	}
	line := t.clock.Now().UTC().Format(time.RFC3339) + " " + halt.Error() + "\n"
	if werr := os.WriteFile(path, []byte(line), 0o600); werr != nil {
		return fmt.Errorf("credential breaker marker: %w", werr)
	}
	return nil
}

// WrongPassword sends account's credentials with a wrong password, once per
// run and only when the config allows it. The login server's refusal trips
// the breaker: no later login of the run goes out. It leaves no marker: the
// refusal is the one asked for, and the next good login of account takes
// its failure back on the server.
func (t *Target) WrongPassword(ctx context.Context, account string) error {
	if !t.cfg.AllowLoginFail {
		return fmt.Errorf("%s=1 unset", EnvAllowLoginFail)
	}
	if t.loginFail.Swap(true) {
		return ErrLoginFailUsed
	}
	return t.checkCredentials(ctx, account, "wrong"+t.secret.Password(account), false)
}

// Client is a character of the run online on a connection the scenario
// drives itself: one goroutine reads it (Await) and sends on it, each send
// paced like the brain's.
type Client struct {
	Account string
	Self    botclient.Self
	conn    *botclient.GameConn
	clock   botclient.Clock
	pacer   *botclient.Pacer
	release func()
	once    sync.Once
	closed  atomic.Bool
	// pos is the character's position as the server last placed or sent
	// it: entry, its own teleports, moves and stops.
	pos botclient.Point
	// target is the character's target as the server last confirmed it
	// (MyTargetSelected) or cleared it (its own TargetUnselected); 0 for
	// none.
	target int32
}

// Pos is where the character stands, or heads to while it moves: where it
// entered, or where the server last put it (its own teleport, stop or
// position fix) or sends it (its own move's destination).
func (c *Client) Pos() botclient.Point { return c.pos }

// Target is the character's target as of the last event read: what the
// server last selected for it, 0 once it cleared it.
func (c *Client) Target() int32 { return c.target }

// Send sends payload once the pacer lets one more frame out.
func (c *Client) Send(ctx context.Context, payload []byte) error {
	if err := c.pacer.Frame(ctx); err != nil {
		return err
	}
	return c.conn.Send(payload)
}

// Await reads the connection until a frame decodes to an event match
// accepts, for at most timeout (and not past ctx's deadline); other frames
// are skipped. A teleport of the character itself is completed on the way:
// its position follows and Appearing goes out, without which the server
// keeps the character out of the world. Its own moves, stops and position
// fixes move Pos too.
func (c *Client) Await(ctx context.Context, timeout time.Duration, match func(botclient.Event) bool) (botclient.Event, error) {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for {
		raw, err := c.conn.Read(deadline)
		if err != nil {
			return nil, err
		}
		ev, ok, err := botclient.DecodeEvent(raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if err := c.follow(ctx, ev); err != nil {
			return nil, err
		}
		if match(ev) {
			return ev, nil
		}
	}
}

// follow moves Pos after an event about the character itself, answers its
// teleport with Appearing, and keeps Target.
func (c *Client) follow(ctx context.Context, ev botclient.Event) error {
	switch e := ev.(type) {
	case botclient.MyTargetSelected:
		c.target = e.ObjectID
	case botclient.TargetUnselected:
		if e.ObjectID == c.Self.ObjectID {
			c.target = 0
		}
	case botclient.TeleportToLocation:
		if e.ObjectID == c.Self.ObjectID {
			c.pos = e.Pos
			return c.Send(ctx, botwire.EncodeAppearing())
		}
	case botclient.MoveToLocation:
		if e.ObjectID == c.Self.ObjectID {
			// The destination's Z is head height; keep the floor's.
			c.pos = botclient.Point{X: e.Dest.X, Y: e.Dest.Y, Z: c.pos.Z}
		}
	case botclient.StopMove:
		if e.ObjectID == c.Self.ObjectID {
			c.pos = e.Pos
		}
	case botclient.ValidateLocation:
		if e.ObjectID == c.Self.ObjectID {
			c.pos = e.Pos
		}
	}
	return nil
}

// Watch reads the connection for d (not past ctx's deadline), handing fn
// every event, as Await does with a match that never accepts; the time
// running out is no error. A frame the deadline cuts in two is finished by
// the next read (wire.FrameReader keeps what it read).
func (c *Client) Watch(ctx context.Context, d time.Duration, fn func(botclient.Event)) error {
	_, err := c.Await(ctx, d, func(ev botclient.Event) bool {
		fn(ev)
		return false
	})
	if errors.Is(err, os.ErrDeadlineExceeded) && ctx.Err() == nil {
		return nil
	}
	return err
}

// Move sends the character to `to`, walking or running as it is set to,
// and waits for the server to start the move; Pos is then the move's
// destination, which geodata may have cut short. It does not wait for the
// arrival. A move the server answers ActionFailed is ErrMoveRefused.
func (c *Client) Move(ctx context.Context, to botclient.Point) (botclient.MoveToLocation, error) {
	if err := c.Send(ctx, botwire.EncodeMoveBackwardToLocation(to, c.pos)); err != nil {
		return botclient.MoveToLocation{}, err
	}
	own := func(ev botclient.Event) bool {
		m, ok := ev.(botclient.MoveToLocation)
		return ok && m.ObjectID == c.Self.ObjectID
	}
	ev, err := c.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		_, failed := ev.(botclient.ActionFailed)
		return failed || own(ev)
	})
	if err != nil {
		return botclient.MoveToLocation{}, fmt.Errorf("move to %v: %w", to, err)
	}
	if m, ok := ev.(botclient.MoveToLocation); ok {
		return m, nil
	}
	// ActionFailed also answers other requests (a bypass) and may lead an
	// accepted move: the move is refused only when none follows.
	ev, err = c.Await(ctx, moveAfterFail, own)
	if errors.Is(err, os.ErrDeadlineExceeded) && ctx.Err() == nil {
		return botclient.MoveToLocation{}, fmt.Errorf("move to %v: %w", to, ErrMoveRefused)
	}
	if err != nil {
		return botclient.MoveToLocation{}, fmt.Errorf("move to %v: %w", to, err)
	}
	return ev.(botclient.MoveToLocation), nil
}

// moveAfterFail is how long Move waits for the move after an ActionFailed.
const moveAfterFail = 2 * time.Second

// Closed reports whether the connection is closed (Logout or Close).
func (c *Client) Closed() bool { return c.closed.Load() }

// Logout logs the character out and closes the connection. A refused
// logout (ErrLogoutRefused) leaves it online and open.
func (c *Client) Logout(ctx context.Context) error {
	if err := c.pacer.Frame(ctx); err != nil {
		return err
	}
	err := c.conn.Logout(ctx)
	if errors.Is(err, botclient.ErrLogoutRefused) {
		return err
	}
	c.Close()
	return err
}

// leaveLinger outlasts the server's attack stance, which refuses a logout,
// and the lost-connection linger of a character in attack stance (15 s,
// gameserver network/disconnect_delay.go disconnectCombatDelay).
const leaveLinger = 16 * time.Second

// Leave ends c's session so that its character is out of the world before
// c's place, and a pooled GM's lease, is given back: the next holder of the
// account must not enter while the character is still there (the game
// server refuses that selection silently and closes the session still
// holding it, gameserver network/character_select.go). A closed c is left
// alone.
func (c *Client) Leave(ctx context.Context) error {
	if c.Closed() {
		return nil
	}
	err := leave(ctx, c)
	c.Close()
	return err
}

// leaver is a session leave ends: the Client, or a test's fake.
type leaver interface {
	logout(ctx context.Context) error
	drop()
	wait(ctx context.Context, d time.Duration) error
}

// leave logs s out, once more after the attack stance when the server
// refuses the logout. When the logout still fails the connection is dropped
// and leave waits out the lost-connection linger, which keeps the character
// in the world up to 15 s.
func leave(ctx context.Context, s leaver) error {
	err := s.logout(ctx)
	if errors.Is(err, botclient.ErrLogoutRefused) {
		if werr := s.wait(ctx, leaveLinger); werr != nil {
			s.drop()
			return errors.Join(err, werr)
		}
		err = s.logout(ctx)
	}
	if err == nil {
		return nil
	}
	s.drop()
	return errors.Join(err, s.wait(ctx, leaveLinger))
}

// logout sends Logout, paced; the server closes the connection when it
// accepts it.
func (c *Client) logout(ctx context.Context) error {
	if err := c.pacer.Frame(ctx); err != nil {
		return err
	}
	return c.conn.Logout(ctx)
}

// drop closes the connection without giving the place back.
func (c *Client) drop() { _ = c.conn.Close() }

// wait waits d on c's clock, or until ctx ends.
func (c *Client) wait(ctx context.Context, d time.Duration) error {
	timer := c.clock.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C():
		return nil
	}
}

// Close drops the connection with no Logout and gives the place back; the
// server keeps the character in the world a moment longer. Later calls do
// nothing.
func (c *Client) Close() {
	c.once.Do(func() {
		c.closed.Store(true)
		c.conn.Close()
		c.release()
	})
}

// GMClient is a scenario's GM online: its commands pass the scenario's filter
// (Scope.check) before they are sent, paced like dialog clicks.
type GMClient struct {
	*Client
	t *Target
	// file is the pool's GM file the scenario holds.
	file string
	// karmaIn is the karma the GM entered with, which EnterGM cleared.
	karmaIn int32
}

// EnterGM logs in a GM of the run's pool that no other scenario, of this
// process or another, holds: with every one held, a minting run mints
// another (Target.mintGM), a pinned one waits. The GM is the scenario's
// until it logs out or its connection closes. It enters with karma 0
// (resetKarma).
func (t *Target) EnterGM(ctx context.Context) (*GMClient, error) {
	lease, err := t.gms.take(ctx, t.mintGM)
	if err != nil {
		return nil, err
	}
	gm, err := lease.load(t.secret)
	if err != nil {
		lease.release()
		return nil, err
	}
	// A session of the GM's account that has not left yet (a run killed
	// before its cleanup) holds the account: the login server kicks it and
	// refuses this login with AccountInUse until it is gone.
	c, err := t.enterWhenFree(ctx, gm.Account, gm.Password(), botclient.Character{Name: gm.Character}, nil)
	if err != nil {
		lease.release()
		return nil, err
	}
	place := c.release
	c.release = func() { place(); lease.release() }
	// The GM is no run character: no command may touch it by name.
	t.mu.Lock()
	delete(t.scope.chars, strings.ToLower(gm.Character))
	t.mu.Unlock()
	g := &GMClient{Client: c, t: t, file: lease.file}
	if err := g.resetKarma(ctx); err != nil {
		_ = c.Leave(ctx)
		return nil, fmt.Errorf("GM %s: %w", gm.Character, err)
	}
	return g, nil
}

// karmaWait bounds the wait for the server's answer to //set karma.
const karmaWait = 10 * time.Second

// resetKarma clears the karma the GM may carry in from an earlier
// scenario's kills (#569): a bot's hit on a GM with karma never flags the
// bot, and the pvp persona hunts it. Fresh from login the GM has no
// target, so the server sets the GM's own karma, and answers with its name
// and the value set even when the karma was 0 already (admin_set.go). It
// records the karma the GM entered with in karmaIn.
func (g *GMClient) resetKarma(ctx context.Context) error {
	ev, ok, err := botclient.DecodeEvent(g.Self.UserInfo)
	ui, isUI := ev.(botclient.UserInfo)
	if err != nil || !ok || !isUI {
		return fmt.Errorf("decode the GM's UserInfo: %v", err)
	}
	g.karmaIn = ui.Karma
	if err := g.Command(ctx, "set karma 0"); err != nil {
		return err
	}
	ev, err = g.Await(ctx, karmaWait, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.ID == sysMsgS1 && strings.Contains(m.Text[0], "karma")
	})
	if err != nil {
		return fmt.Errorf("reset karma: no answer: %w", err)
	}
	return karmaReset(g.Self.Name, ev.(botclient.SystemMessage).Text[0])
}

// karmaReset checks answer, the server's answer to //set karma 0: it set
// the karma of the GM itself, name, to 0.
func karmaReset(name, answer string) error {
	if want := "You successfully set " + name + "'s karma to 0."; !strings.EqualFold(answer, want) {
		return fmt.Errorf("reset karma: the server answered %q, want %q", answer, want)
	}
	return nil
}

// Command sends cmd (without "//") once the scenario's filter accepts it.
func (g *GMClient) Command(ctx context.Context, cmd string) error {
	g.t.mu.Lock()
	err := g.t.scope.check(cmd)
	if err == nil && strings.HasPrefix(strings.ToLower(cmd), "recall") {
		g.t.scope.target = 0 // the server clears the GM's target
	}
	g.t.mu.Unlock()
	if err != nil {
		return err
	}
	if err := g.pacer.Command(ctx); err != nil {
		return err
	}
	return g.Send(ctx, botwire.EncodeSendBypassBuildCmd(cmd))
}

// Select makes objectID, a run character or a monster the run spawned, the
// GM's target, and waits for the server to confirm it.
func (g *GMClient) Select(ctx context.Context, objectID int32) error {
	g.t.mu.Lock()
	owned := g.t.scope.owns(objectID)
	g.t.mu.Unlock()
	if !owned {
		return fmt.Errorf("%w: target %d is not the run's", ErrCommandRefused, objectID)
	}
	if err := g.selectOnce(ctx, objectID); err != nil {
		return fmt.Errorf("select %d: %w", objectID, err)
	}
	g.t.mu.Lock()
	g.t.scope.target = objectID
	g.t.mu.Unlock()
	return nil
}

// Give has the GM give objectID, a run character, count of template tmpl
// (adminGive gives to the GM's target, or to the GM itself with none).
func (g *GMClient) Give(ctx context.Context, objectID, tmpl, count int32) error {
	return g.OnTarget(ctx, objectID, fmt.Sprintf("give %d %d", tmpl, count))
}

// OnTarget runs the admin command cmd with objectID, a run character, as
// the GM's target. The server runs a target command on the GM itself when
// it has none, and may clear the target a moment after selecting it, as it
// does around a recalled character's arrival (#206). So the GM first reads
// its pending frames for giveSettle, and selects objectID again while its
// target is another or none, up to selectTries times.
func (g *GMClient) OnTarget(ctx context.Context, objectID int32, cmd string) error {
	return g.OnTargetWatch(ctx, objectID, cmd, func(botclient.Event) {})
}

// OnTargetWatch is OnTarget handing fn every event the GM reads while it
// settles, for a scenario that keeps watching through the GM.
func (g *GMClient) OnTargetWatch(ctx context.Context, objectID int32, cmd string, fn func(botclient.Event)) error {
	for range selectTries {
		if err := g.Watch(ctx, giveSettle, fn); err != nil {
			return err
		}
		if g.Target() == objectID {
			return g.Command(ctx, cmd)
		}
		if err := g.Select(ctx, objectID); err != nil {
			return err
		}
	}
	return fmt.Errorf("%s: the GM's selection of %d does not hold", cmd, objectID)
}

// giveSettle is how long OnTarget reads the GM's frames before it trusts its
// target: the server's TargetUnselected follows the selection within a
// frame or two.
const giveSettle = 500 * time.Millisecond

// selectTries is how often Select clicks an object the server does not
// select: one that just teleported is out of the GM's world until it
// appears again, and the click is dropped or answered ActionFailed.
const selectTries = 3

// selectOnce clicks objectID until the server selects it, up to
// selectTries times, each answered within selectWait (selectAfterFail
// once ActionFailed answered it). A click on an object the server did
// select acts on it, so the next click goes out only when the last one
// was not selected.
func (g *GMClient) selectOnce(ctx context.Context, objectID int32) error {
	selected := func(ev botclient.Event) bool {
		s, ok := ev.(botclient.MyTargetSelected)
		return ok && s.ObjectID == objectID
	}
	var err error
	for range selectTries {
		if err := g.Send(ctx, botwire.EncodeAction(objectID, g.pos)); err != nil {
			return err
		}
		var ev botclient.Event
		ev, err = g.Await(ctx, selectWait, func(ev botclient.Event) bool {
			_, failed := ev.(botclient.ActionFailed)
			return failed || selected(ev)
		})
		if err == nil {
			if selected(ev) {
				return nil
			}
			if _, err = g.Await(ctx, selectAfterFail, selected); err == nil {
				return nil
			}
		}
		if !errors.Is(err, os.ErrDeadlineExceeded) || ctx.Err() != nil {
			return err
		}
	}
	return err
}

const (
	selectWait      = 5 * time.Second
	selectAfterFail = time.Second
)

// spawnReach is how far from the GM a monster it spawns may appear: the
// GM's target stands next to it when it spawns.
const spawnReach = 300

// Spawn spawns a monster of template npcID at the GM's target, or at the GM
// with none, and returns its object id; the run owns it until Delete. The
// GM stands next to its target: a monster of the template that appears
// farther is not the one spawned.
func (g *GMClient) Spawn(ctx context.Context, npcID int32) (int32, error) {
	if err := g.Command(ctx, "spawn "+strconv.Itoa(int(npcID))); err != nil {
		return 0, err
	}
	ev, err := g.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		n, ok := ev.(botclient.NPCInfo)
		return ok && n.NPCID == npcID && within(n.Pos, g.pos, spawnReach) && !g.owned(n.ObjectID)
	})
	if err != nil {
		return 0, fmt.Errorf("spawn %d: %w", npcID, err)
	}
	id := ev.(botclient.NPCInfo).ObjectID
	g.t.mu.Lock()
	g.t.scope.npcs[id] = true
	g.t.mu.Unlock()
	return id, nil
}

func (g *GMClient) owned(objectID int32) bool {
	g.t.mu.Lock()
	defer g.t.mu.Unlock()
	return g.t.scope.npcs[objectID]
}

// Delete removes objectID, a monster the run spawned, from the world.
func (g *GMClient) Delete(ctx context.Context, objectID int32) error {
	if err := g.Select(ctx, objectID); err != nil {
		return err
	}
	if err := g.Command(ctx, "delete"); err != nil {
		return err
	}
	if _, err := g.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.DeleteObject)
		return ok && d.ObjectID == objectID
	}); err != nil {
		return fmt.Errorf("delete %d: %w", objectID, err)
	}
	g.t.mu.Lock()
	delete(g.t.scope.npcs, objectID)
	g.t.scope.target = 0
	g.t.mu.Unlock()
	return nil
}

// Forget drops objectID, a monster the run spawned that the world removed
// (killed), from the run's monsters: cleanup has nothing left to delete.
func (g *GMClient) Forget(objectID int32) {
	g.t.mu.Lock()
	defer g.t.mu.Unlock()
	delete(g.t.scope.npcs, objectID)
	if g.t.scope.target == objectID {
		g.t.scope.target = 0
	}
}

// Spawned lists the monsters the run spawned and has not deleted.
func (g *GMClient) Spawned() []int32 {
	g.t.mu.Lock()
	defer g.t.mu.Unlock()
	ids := make([]int32, 0, len(g.t.scope.npcs))
	for id := range g.t.scope.npcs {
		ids = append(ids, id)
	}
	return ids
}

// within reports whether a and b are at most r apart on the ground.
func within(a, b botclient.Point, r int64) bool {
	dx, dy := int64(a.X)-int64(b.X), int64(a.Y)-int64(b.Y)
	return dx*dx+dy*dy <= r*r
}
