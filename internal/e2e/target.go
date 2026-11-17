package e2e

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// MaxOnline is the most e2e characters a run keeps online at once, the GM's
// included.
const MaxOnline = 4

// EntryTimeout bounds each read and write of a login or game entry.
const EntryTimeout = 15 * time.Second

// ErrLoginFailUsed reports a second wrong-password login in one run: the
// login server bans the IP after 3 in a row (LoginTryBeforeBan).
var ErrLoginFailUsed = errors.New("the run's one wrong-password login is used")

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

// Target is one run against a server: every login of the run passes its
// login gate, spaced under the accept filter unless the config says the
// source address is allowlisted, and its credential breaker, which the
// first refused credential trips for the rest of the run. At most MaxOnline
// characters of the run are online at once (Hold). Its methods may be
// called from any goroutine.
type Target struct {
	cfg    Config
	secret botaccounts.Secret
	clock  botclient.Clock
	gate   *botclient.LoginGate
	net    network
	// slots holds a token per character online.
	slots chan struct{}
	// loginFail is set once the run's one wrong-password login was sent.
	loginFail atomic.Bool

	mu    sync.Mutex
	scope *Scope // guarded by mu
}

// NewTarget opens a run against cfg's server: it refuses a prefix that
// overlaps the old swarm's or panel (the owner's `bots ui` prefix, "" for
// none), and a secret that is missing, readable by others or weak.
func NewTarget(cfg Config, panel string) (*Target, error) {
	if err := CheckPrefix(cfg.Prefix, panel); err != nil {
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
	return &Target{
		cfg: cfg, secret: secret, clock: clock, gate: gate, net: n,
		slots: make(chan struct{}, MaxOnline),
		scope: newScope(),
	}
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

// Own makes name (objectID 0 when not known yet) one of the run's
// characters, which GM commands may then touch.
func (t *Target) Own(name string, objectID int32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scope.addChar(name, objectID)
}

// Enter logs account in and enters the world as ch on a place of its own,
// recording every game frame read into rec when it is set. A refused
// credential trips the breaker. The character becomes one of the run's.
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
	return &Client{Account: account, Self: self, conn: conn, pacer: botclient.NewPacer(t.clock), release: release, pos: botclient.Point{X: self.X, Y: self.Y, Z: self.Z}}, nil
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
	for {
		c, err := t.Enter(ctx, account, t.Password(account), ch, rec)
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
	turn, err := t.gate.Take(ctx)
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
			t.gate.Halt(fmt.Errorf("account %s: %w", account, err))
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
	return conn, self, nil
}

// CheckCredentials runs a login session for account up to the server list,
// through the gate: the first one creates the account (AutoCreateAccounts).
// A refused credential trips the breaker.
func (t *Target) CheckCredentials(ctx context.Context, account, password string) error {
	turn, err := t.gate.Take(ctx)
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
		t.gate.Halt(fmt.Errorf("account %s: %w", account, err))
	}
	return err
}

// WrongPassword sends account's credentials with a wrong password, once per
// run and only when the config allows it. The login server's refusal trips
// the breaker: no later login of the run goes out.
func (t *Target) WrongPassword(ctx context.Context, account string) error {
	if !t.cfg.AllowLoginFail {
		return fmt.Errorf("%s=1 unset", EnvAllowLoginFail)
	}
	if t.loginFail.Swap(true) {
		return ErrLoginFailUsed
	}
	return t.CheckCredentials(ctx, account, "wrong"+t.secret.Password(account))
}

// Client is a character of the run online on a connection the scenario
// drives itself: one goroutine reads it (Await) and sends on it, each send
// paced like the brain's.
type Client struct {
	Account string
	Self    botclient.Self
	conn    *botclient.GameConn
	pacer   *botclient.Pacer
	release func()
	once    sync.Once
	closed  atomic.Bool
	// pos is the character's position as its own teleports moved it.
	pos botclient.Point
}

// Pos is where the character stands: where it entered, or where its last
// teleport put it.
func (c *Client) Pos() botclient.Point { return c.pos }

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
// keeps the character out of the world.
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
		if tp, isTP := ev.(botclient.TeleportToLocation); isTP && tp.ObjectID == c.Self.ObjectID {
			c.pos = tp.Pos
			if err := c.Send(ctx, botclient.EncodeAppearing()); err != nil {
				return nil, err
			}
		}
		if match(ev) {
			return ev, nil
		}
	}
}

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

// GMClient is the run's GM online: its commands pass the run's filter
// (Scope.check) before they are sent, paced like dialog clicks.
type GMClient struct {
	*Client
	t *Target
}

// EnterGM logs the GM file's GM in.
func (t *Target) EnterGM(ctx context.Context) (*GMClient, error) {
	gm, err := LoadGM(t.cfg.GMFile)
	if err != nil {
		return nil, err
	}
	c, err := t.Enter(ctx, gm.Account, gm.Password(), botclient.Character{Name: gm.Character}, nil)
	if err != nil {
		return nil, err
	}
	// The GM is no run character: no command may touch it by name.
	t.mu.Lock()
	delete(t.scope.chars, strings.ToLower(gm.Character))
	t.mu.Unlock()
	return &GMClient{Client: c, t: t}, nil
}

// Command sends cmd (without "//") once the run's filter accepts it.
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
	return g.Send(ctx, botclient.EncodeSendBypassBuildCmd(cmd))
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
	if err := g.Send(ctx, botclient.EncodeAction(objectID, g.pos)); err != nil {
		return err
	}
	if _, err := g.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		s, ok := ev.(botclient.MyTargetSelected)
		return ok && s.ObjectID == objectID
	}); err != nil {
		return fmt.Errorf("select %d: %w", objectID, err)
	}
	g.t.mu.Lock()
	g.t.scope.target = objectID
	g.t.mu.Unlock()
	return nil
}

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
