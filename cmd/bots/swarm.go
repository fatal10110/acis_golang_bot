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
	"strings"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
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
	// pprof is the loopback address of the profile endpoint; empty serves
	// none.
	pprof string
	// datapack is the server datapack's XML directory the bots pick
	// farming spots from; empty farms nowhere. spawnMultiplier is the
	// server's SpawnMultiplier, which scales each spot's bot cap.
	// spawnEvents is the server's SpawnEvents (-spawn-events).
	datapack        string
	spawnMultiplier float64
	spawnEvents     string
	gm              gmOptions
	store           storeOptions
	geo             geoOptions
	// persona is what every bot plays (-persona).
	persona string
	// clanInviters are the -clan-inviters names.
	clanInviters []string
	// sevenSigns is Seven Signs member mode (-sevensigns-members,
	// -sevensigns-max-members, -signs-seal).
	sevenSigns botclient.SevenSignsPolicy
	// support is how many of a -persona farmer swarm's bots, the last
	// accounts, play the support preset instead (-support).
	support int
	// noParties turns the bot party organizer off (-no-parties).
	noParties bool
	// outcomeLog is the outcome log's file (-outcome-log); empty records
	// nothing.
	outcomeLog string
	// crashFile keeps the runtime's crash output (-crash-file); empty is
	// crash.log in the default data directory.
	crashFile string
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
	addGeoFlags(fs, &o.geo)
	addPersonaFlag(fs, &o.persona)
	fs.IntVar(&o.support, "support", 0, "how many of a -persona farmer swarm's bots, the last accounts, play -persona support instead: the Support members the party organizer groups with the farmers")
	fs.BoolVar(&o.noParties, "no-parties", false, "run no bot party organizer: the bots only join the parties players invite them to; default: farmer and support bots form bot parties")
	inviters := fs.String("clan-inviters", "", "comma-separated character names whose clan invitation a crafter accepts while clanless; default: any player's")
	signsMembers := addSevenSignsFlags(fs, &o.sevenSigns)
	fs.StringVar(&o.datapack, "datapack", "", "server datapack XML directory (aCis_datapack/data/xml) the bots pick farming spots from; default: farm nowhere")
	fs.Float64Var(&o.spawnMultiplier, "spawn-multiplier", 1, "the server's SpawnMultiplier (npcs.properties), which scales how many bots a farming spot takes")
	addSpawnEventsFlag(fs, &o.spawnEvents)
	fs.StringVar(&o.metrics, "metrics", "", "loopback address (for example 127.0.0.1:6060) that serves live metrics at "+botmetrics.VarsPath+"; default: serve none")
	fs.StringVar(&o.pprof, "pprof", "", pprofUsage)
	addOutcomeLogFlag(fs, &o.outcomeLog)
	fs.StringVar(&o.crashFile, "crash-file", "", crashFileUsage)
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
	if err := o.store.validate(); err != nil {
		return o, err
	}
	if err := validatePersona(o.persona, o.datapack); err != nil {
		return o, err
	}
	if err := o.geo.validate(o.datapack); err != nil {
		return o, err
	}
	p, _ := findPreset(o.persona)
	if !p.store && flagSet(fs, "store-budget") {
		return o, fmt.Errorf("-store-budget applies to -persona merchant, not %s", o.persona)
	}
	for _, name := range valuablesFlags {
		if !p.persona().Valuables.Enabled && flagSet(fs, name) {
			return o, fmt.Errorf("-%s applies to -persona merchant, not %s", name, o.persona)
		}
	}
	for _, n := range strings.Split(*inviters, ",") {
		if n = strings.TrimSpace(n); n != "" {
			o.clanInviters = append(o.clanInviters, n)
		}
	}
	if o.support < 0 || o.support > o.accounts.Count {
		return o, fmt.Errorf("-support %d: want 0 to -bots %d", o.support, o.accounts.Count)
	}
	if o.support > 0 && o.persona != botparty.PresetFarmer {
		return o, fmt.Errorf("-support applies to -persona farmer, not %s", o.persona)
	}
	if p, _ := findPreset(o.persona); !p.inviters && flagSet(fs, "clan-inviters") {
		return o, fmt.Errorf("-clan-inviters applies to -persona crafter, not %s", o.persona)
	}
	if err := checkSevenSignsFlags(fs, o.persona, *signsMembers, &o.sevenSigns); err != nil {
		return o, err
	}
	if o.metrics != "" {
		if err := botmetrics.CheckAddr(o.metrics); err != nil {
			return o, fmt.Errorf("-metrics: %w", err)
		}
	}
	if err := checkPprofFlag(o.pprof); err != nil {
		return o, err
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
// Farmer and support bots form bot parties through the organizer
// (botregistry.Organizer) unless -no-parties.
func runSwarm(ctx context.Context, args []string, stderr io.Writer) int {
	o, err := parseSwarmFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 2
	}
	defer startCrashLog(o.crashFile, "swarm", stderr)()
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
	p, _ := findPreset(o.persona) // validated by parseSwarmFlags
	w, cleanup, err := buildShared(wiringOptions{
		allowlisted: o.allowlisted, outcomeLog: o.outcomeLog, datapack: o.datapack, spawnMultiplier: o.spawnMultiplier,
		spawnEvents: o.spawnEvents, store: o.store, geo: o.geo, stores: p.store,
	}, clock, log)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	// Deferred first of the run's pieces: the navigation stays mapped and
	// the outcome writer takes the stays of every bot until all returned.
	defer cleanup()
	gate, sh := w.gate, w.shared
	sh.clanInviters = o.clanInviters
	sh.sevenSigns = o.sevenSigns
	cfg := botclient.SessionConfig{
		LoginAddr:  o.loginAddr,
		GameAddr:   o.gameAddr,
		ServerID:   byte(o.serverID),
		Timeout:    o.timeout,
		Dial:       []botclient.DialOption{dialOpt},
		Recorder:   rec,
		Persistent: o.accounts.Persistent,
		Markers:    botnames.Markers{o.marker},
		Clock:      clock,
		Gate:       gate,
		Log:        log,
	}
	if err := walkReady(p, sh.nav); err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	auth, stopAuth, err := startAuthority(ctx, o.gm, clock, log)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	// Deferred first, so it runs after the metrics endpoint stopped and
	// every bot returned.
	defer stopAuth()
	sh.auth = auth
	var reg *botmetrics.Registry
	if o.metrics != "" {
		reg = botmetrics.NewRegistry(botclient.RuleNames()...)
		gate.ReportBreaker(reg)
		if sh.nav != nil {
			sh.nav.ReportCache(reg)
		}
	}
	// The last -support accounts play support (validated by
	// parseSwarmFlags: only beside farmers).
	split := len(accounts) - o.support
	bots, err := newBots(cfg, p, p.brain(sh), o.marker, accounts[:split], reg)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	members := organized(bots, p.name)
	if o.support > 0 {
		sp, _ := findPreset(botparty.PresetSupport)
		sb, err := newBots(cfg, sp, sp.brain(sh), o.marker, accounts[split:], reg)
		if err != nil {
			fmt.Fprintf(stderr, "bots swarm: %v\n", err)
			return 1
		}
		bots = append(bots, sb...)
		members = append(members, organized(sb, sp.name)...)
	}
	parties, err := swarmParties(o.noParties, members, clock, reg, log)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	if reg != nil {
		// Bound before any login: a taken port fails the run, not the bots.
		ln, err := botmetrics.Listen(o.metrics)
		if err != nil {
			fmt.Fprintf(stderr, "bots swarm: metrics: %v\n", err)
			return 1
		}
		tick := time.NewTicker(metricsPeriod)
		defer tick.Stop()
		// Deferred after tick.Stop, so it runs first: the endpoint serves
		// until every bot returned, then both goroutines are joined.
		defer serveMetrics(ctx, ln, reg, tick.C, log)()
		log.Info("metrics serve", "addr", ln.Addr().String(), "path", botmetrics.VarsPath)
	}
	pln, err := listenPprof(o.pprof, log)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	// Deferred after the bots are joined below: the profiles stay available
	// until every bot returned.
	defer servePprof(ctx, pln, log)()
	log.Info("swarm starts", "bots", len(bots), "support", o.support, "parties", parties != nil, "prefix", o.accounts.Prefix, "persistent", o.accounts.Persistent, "allowlisted", o.allowlisted)
	// The organizer plans until every bot returned (a signal, or the
	// credential breaker), then it is stopped and joined.
	partyCtx, stopParties := context.WithCancel(ctx)
	defer stopParties()
	var organizer sync.WaitGroup
	if parties != nil {
		organizer.Go(func() { parties.Run(partyCtx) })
	}
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
	stopParties()
	organizer.Wait()
	if rec != nil {
		if err := rec.Err(); err != nil {
			log.Error("recording failed", "err", err)
		}
	}
	code := swarmExit(ctx.Err(), gate.Err())
	log.Info("swarm stopped", "exit", code)
	return code
}

// serveMetrics serves reg on ln, unless ln is nil, and collects it on every
// tick, on goroutines that outlive ctx's cancellation (the endpoint keeps
// serving while the bots log out). The returned stop ends them and joins
// them; it closes ln.
func serveMetrics(ctx context.Context, ln net.Listener, reg *botmetrics.Registry, tick <-chan time.Time, log *slog.Logger) (stop func()) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	if ln != nil {
		wg.Go(func() {
			if err := botmetrics.Serve(ctx, ln, botmetrics.Handler(reg)); err != nil {
				log.Error("metrics endpoint failed", "err", err)
			}
		})
	}
	wg.Go(func() { reg.Run(ctx, tick) })
	return func() {
		cancel()
		wg.Wait()
	}
}

// loadSpots reads the datapack whose XML directory is dir and returns the
// spot registry and the travel map every bot of the process shares, and the
// datapack for the caller to take what else it needs from (the panel's
// catalog) and drop. With the navigation n the registry leaves out the
// spots whose centre is in water (Nav.DrySpots, #275). supply is what the
// travel map computes the items' supply with.
func loadSpots(dir string, multiplier float64, supply botclient.SupplyConfig, n *botclient.Nav, log *slog.Logger) (*botclient.SpotRegistry, *botclient.TravelMap, *datapack.Datapack, error) {
	dp, err := datapack.Load(dir)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, w := range dp.Spawnlist.Warnings {
		log.Debug("datapack", "warning", w)
	}
	spots, wet := n.DrySpots(dp.Spots())
	log.Info("farming spots", "spots", len(spots), "wet_dropped", wet, "makers", len(dp.Spawnlist.Makers), "warnings", len(dp.Spawnlist.Warnings))
	travel := botclient.NewTravelMap(dp, supply)
	log.Info("travel map", "gatekeepers", travel.Gatekeepers())
	return botclient.NewSpotRegistry(spots, multiplier), travel, dp, nil
}

// addSpawnEventsFlag adds -spawn-events, the server's SpawnEvents the bot
// cannot read, to fs.
func addSpawnEventsFlag(fs *flag.FlagSet, events *string) {
	fs.StringVar(events, "spawn-events", strings.Join(botclient.DefaultSupplyConfig().SpawnEvents, ";"),
		"semicolon-separated SpawnEvents of the server (npcs.properties): the events whose spawns run, what the items' supply counts")
}

// supplyConfig is the default SupplyConfig with the -spawn-events list
// events: its names split at ";", blanks trimmed and empty names dropped.
func supplyConfig(events string) botclient.SupplyConfig {
	cfg := botclient.DefaultSupplyConfig()
	cfg.SpawnEvents = nil
	for _, e := range strings.Split(events, ";") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.SpawnEvents = append(cfg.SpawnEvents, e)
		}
	}
	return cfg
}

// newBots returns one new bot per account playing preset p, each with a copy
// of brain and its character named with marker (botclient.NewCharacter), of
// p's class when it forces one,
// counting into its own Bot of reg (a slot: its sessions share it); a nil
// reg counts nothing.
func newBots(cfg botclient.SessionConfig, p preset, brain botclient.BrainConfig, marker botnames.Marker, accounts []botaccounts.Account, reg *botmetrics.Registry) ([]*botclient.Bot, error) {
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	bots := make([]*botclient.Bot, 0, len(accounts))
	for _, a := range accounts {
		bot := botclient.BotConfig{Brain: brain, Preset: p.name}
		if reg != nil {
			bot.Brain.Metrics = reg.NewBot()
		}
		ch, err := botclient.NewCharacter(marker, a.Name, rng)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", a.Name, err)
		}
		if p.class != nil {
			ch.Class = *p.class
		}
		bot.Character = ch
		b, err := botclient.NewBot(cfg, a.Name, a.Password(), bot, rng)
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

// flagSet reports whether the command line set the flag named name.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}
