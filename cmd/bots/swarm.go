package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// exitStopped is the exit code of a swarm that stopped by itself: the
// credential breaker tripped, or every bot stopped for a reason a restart
// would only repeat (a refused credential, a taken account, repeated
// aborts). The systemd unit does not restart on it
// (RestartPreventExitStatus): a restart loop is a login loop.
const exitStopped = 3

// metricsPeriod is how often the swarm sums its bots' metrics for the
// endpoint.
const metricsPeriod = 10 * time.Second

// swarmOptions are the flags of `bots swarm`.
type swarmOptions struct {
	accounts  botaccounts.Config
	loginAddr string
	gameAddr  string
	serverID  uint
	timeout   time.Duration
	localAddr string
	record    string
	marker    botnames.Marker
	// allowlisted makes the login gate unspaced.
	allowlisted bool
	// metrics is the loopback address of the metrics endpoint; empty
	// serves none.
	metrics string
	// datapack is the server datapack's XML directory the bots pick
	// farming spots from; empty farms nowhere. spawnMultiplier is the
	// server's SpawnMultiplier, which scales each spot's bot cap.
	datapack        string
	spawnMultiplier float64
	gm              gmOptions
	store           storeOptions
	// persona is what every bot plays (-persona).
	persona string
}

func parseSwarmFlags(args []string, stderr io.Writer) (swarmOptions, error) {
	var o swarmOptions
	fs := flag.NewFlagSet("bots swarm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addAccountFlags(fs, &o.accounts)
	fs.StringVar(&o.loginAddr, "login", "127.0.0.1:2106", "login server address (host:port)")
	fs.StringVar(&o.gameAddr, "game", "", "game server address (host:port) to dial instead of the one the login server advertises")
	fs.UintVar(&o.serverID, "server-id", 1, "game server id to play on")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Second, "longest wait for each read or write of a login or game entry")
	fs.StringVar(&o.localAddr, "local-addr", "", "source IP of every login and game dial, when the host has several and only one is allowlisted on the servers; default: the OS picks")
	fs.StringVar(&o.record, "record", "", "directory that receives the first frame of every game opcode read (test fixtures); default: record nothing")
	fs.StringVar(&o.marker.Text, "name-marker", botnames.DefaultMarker.Text, "text that marks the bots' character names")
	placement := fs.String("name-marker-at", "suffix", "where the marker goes in a name: suffix or prefix")
	addAllowlistedFlag(fs, &o.allowlisted)
	addGMFlags(fs, &o.gm)
	addStoreFlags(fs, &o.store)
	addPersonaFlag(fs, &o.persona)
	fs.StringVar(&o.datapack, "datapack", "", "server datapack XML directory (aCis_datapack/data/xml) the bots pick farming spots from; default: farm nowhere")
	fs.Float64Var(&o.spawnMultiplier, "spawn-multiplier", 1, "the server's SpawnMultiplier (npcs.properties), which scales how many bots a farming spot takes")
	fs.StringVar(&o.metrics, "metrics", "", "loopback address (for example 127.0.0.1:6060) that serves live metrics at "+botmetrics.VarsPath+"; default: serve none")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	switch *placement {
	case "suffix":
		o.marker.Placement = botnames.Suffix
	case "prefix":
		o.marker.Placement = botnames.Prefix
	default:
		return o, fmt.Errorf("-name-marker-at %q: want suffix or prefix", *placement)
	}
	if err := o.marker.Validate(); err != nil {
		return o, fmt.Errorf("-name-marker: %w", err)
	}
	if o.serverID < 1 || o.serverID > 255 {
		return o, fmt.Errorf("-server-id %d: want 1-255", o.serverID)
	}
	if _, err := botclient.ParseLocalAddr(o.localAddr); err != nil {
		return o, fmt.Errorf("-local-addr: %w", err)
	}
	if o.timeout <= 0 {
		return o, fmt.Errorf("-timeout %s: want a positive duration", o.timeout)
	}
	if o.spawnMultiplier <= 0 || math.IsNaN(o.spawnMultiplier) || math.IsInf(o.spawnMultiplier, 1) {
		return o, fmt.Errorf("-spawn-multiplier %g: want a positive number", o.spawnMultiplier)
	}
	if err := o.gm.validate(); err != nil {
		return o, err
	}
	if err := o.store.validate(o.datapack); err != nil {
		return o, err
	}
	if err := validatePersona(o.persona, o.datapack); err != nil {
		return o, err
	}
	if o.metrics != "" {
		if err := botmetrics.CheckAddr(o.metrics); err != nil {
			return o, fmt.Errorf("-metrics: %w", err)
		}
	}
	return o, nil
}

// runSwarm is `bots swarm`: it plays one bot per account of the swarm
// instance (-bots N of them, never more, each one session at a time) until
// a signal, which logs every bot out. Every login and game dial of the
// process passes one LoginGate (see botclient). Reports go to stderr. With
// -metrics it serves the bots' metrics on loopback until every bot returned.
// With -datapack the bots pick farming spots from one registry they share.
// With -commanders or -gm-dsn-file they obey whispered GM commands.
func runSwarm(ctx context.Context, args []string, stderr io.Writer) int {
	o, err := parseSwarmFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 2
	}
	accounts, err := o.accounts.Accounts()
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	var rec *botclient.Recorder
	if o.record != "" {
		if rec, err = botclient.NewRecorder(o.record); err != nil {
			fmt.Fprintf(stderr, "bots swarm: %v\n", err)
			return 1
		}
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	dialOpt, _ := botclient.ParseLocalAddr(o.localAddr) // validated by parseSwarmFlags
	clock := botclient.SystemClock{}
	gate := botclient.NewLoginGate(clock, nil)
	if o.allowlisted {
		gate = botclient.NewAllowlistedLoginGate(clock, nil)
	}
	cfg := botclient.SessionConfig{
		LoginAddr:  o.loginAddr,
		GameAddr:   o.gameAddr,
		ServerID:   byte(o.serverID),
		Timeout:    o.timeout,
		Dial:       []botclient.DialOption{dialOpt},
		Recorder:   rec,
		Persistent: o.accounts.Persistent,
		Marker:     o.marker,
		Clock:      clock,
		Gate:       gate,
		Log:        log,
	}
	if o.datapack != "" {
		if cfg.Brain.Spots, cfg.Brain.Travel, err = loadSpots(o.datapack, o.spawnMultiplier, log); err != nil {
			fmt.Fprintf(stderr, "bots swarm: %v\n", err)
			return 1
		}
	}
	applyPersona(o.persona, &cfg.Brain)
	o.store.apply(&cfg.Brain, log)
	auth, stopAuth, err := startAuthority(ctx, o.gm, clock, log)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	// Deferred first, so it runs after the metrics endpoint stopped and
	// every bot returned.
	defer stopAuth()
	applyPartyAndAuthority(&cfg.Brain, auth)
	var reg *botmetrics.Registry
	if o.metrics != "" {
		reg = botmetrics.NewRegistry(botclient.RuleNames()...)
		gate.ReportBreaker(reg)
	}
	bots, err := newBots(cfg, accounts, reg)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	if reg != nil {
		// Bound before any login: a taken port fails the run, not the bots.
		ln, err := botmetrics.Listen(o.metrics)
		if err != nil {
			fmt.Fprintf(stderr, "bots swarm: %v\n", err)
			return 1
		}
		tick := time.NewTicker(metricsPeriod)
		defer tick.Stop()
		// Deferred after tick.Stop, so it runs first: the endpoint serves
		// until every bot returned, then both goroutines are joined.
		defer serveMetrics(ctx, ln, reg, tick.C, log)()
		log.Info("metrics serve", "addr", ln.Addr().String(), "path", botmetrics.VarsPath)
	}
	log.Info("swarm starts", "bots", len(bots), "prefix", o.accounts.Prefix, "persistent", o.accounts.Persistent, "allowlisted", o.allowlisted)
	var wg sync.WaitGroup
	for _, b := range bots {
		wg.Go(func() {
			switch err := b.Run(ctx); {
			case errors.Is(err, botclient.ErrCommandedLogout):
				log.Info("bot stopped", "account", b.Account(), "err", err)
			case err != nil:
				log.Error("bot stopped", "account", b.Account(), "err", err)
			}
		})
	}
	wg.Wait()
	if rec != nil {
		if err := rec.Err(); err != nil {
			log.Error("recording failed", "err", err)
		}
	}
	code := swarmExit(ctx.Err(), gate.Err())
	log.Info("swarm stopped", "exit", code)
	return code
}

// serveMetrics serves reg on ln and collects it on every tick, on two
// goroutines that outlive ctx's cancellation (the endpoint keeps serving
// while the bots log out). The returned stop ends both and joins them; it
// closes ln.
func serveMetrics(ctx context.Context, ln net.Listener, reg *botmetrics.Registry, tick <-chan time.Time, log *slog.Logger) (stop func()) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := botmetrics.Serve(ctx, ln, botmetrics.Handler(reg)); err != nil {
			log.Error("metrics endpoint failed", "err", err)
		}
	})
	wg.Go(func() { reg.Run(ctx, tick) })
	return func() {
		cancel()
		wg.Wait()
	}
}

// loadSpots reads the datapack whose XML directory is dir and returns the
// spot registry and the travel map every bot of the process shares. The
// datapack itself is dropped once they are built.
func loadSpots(dir string, multiplier float64, log *slog.Logger) (*botclient.SpotRegistry, *botclient.TravelMap, error) {
	dp, err := datapack.Load(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, w := range dp.Spawnlist.Warnings {
		log.Debug("datapack", "warning", w)
	}
	spots := dp.Spots()
	log.Info("farming spots", "spots", len(spots), "makers", len(dp.Spawnlist.Makers), "warnings", len(dp.Spawnlist.Warnings))
	travel := botclient.NewTravelMap(dp)
	log.Info("travel map", "gatekeepers", travel.Gatekeepers())
	return botclient.NewSpotRegistry(spots, multiplier), travel, nil
}

// newBots returns one bot per account, each counting into its own Bot of
// reg (a slot: its sessions share it); a nil reg counts nothing.
func newBots(cfg botclient.SessionConfig, accounts []botaccounts.Account, reg *botmetrics.Registry) ([]*botclient.Bot, error) {
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	bots := make([]*botclient.Bot, 0, len(accounts))
	for _, a := range accounts {
		if reg != nil {
			cfg.Brain.Metrics = reg.NewBot()
		}
		b, err := botclient.NewBot(cfg, a.Name, a.Password(), rng)
		if err != nil {
			return nil, err
		}
		bots = append(bots, b)
	}
	return bots, nil
}

// swarmExit is the exit code of a swarm whose bots all returned: 0 after a
// signal, exitStopped when it stopped by itself or the credential breaker
// tripped.
func swarmExit(ctxErr, breaker error) int {
	if ctxErr != nil && breaker == nil {
		return 0
	}
	return exitStopped
}

// applyPartyAndAuthority points cfg at one party leader claim table for
// every bot of the process, and at auth when set. The claim limits the
// persona's own party hook, which auth's hooks ask only for a requester auth
// does not know as a GM: a GM's invitation is never limited. Without auth no
// GM is known, and every invitation is limited.
func applyPartyAndAuthority(cfg *botclient.BrainConfig, auth *botclient.Authority) {
	botclient.NewPartyClaims().Apply(cfg)
	if auth != nil {
		cfg.Authority = auth
		cfg.Persona = auth.Hooks(cfg.Persona)
	}
}
