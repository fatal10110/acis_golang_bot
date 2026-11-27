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
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botpanel"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// defaultListen is the web panel's address when -listen is not given.
const defaultListen = "127.0.0.1:8090"

// logLines is how many recent log lines the panel keeps.
const logLines = 5000

// uiOptions are the flags of `bots ui`.
type uiOptions struct {
	secretFile string
	// dataDir holds the registry; empty is botregistry.DefaultDirName under the user's
	// config directory.
	dataDir string
	// prefix and marker are the panel's own prefix and marker, given when
	// the registry is created; a later start takes the stored ones, and
	// refuses others.
	prefix    string
	marker    botnames.Marker
	maxOnline int
	// importPrefix, importCount and importMarker import an earlier swarm's
	// accounts 1 to importCount (-import-prefix, -import, -import-marker).
	importPrefix string
	importCount  int
	importMarker botnames.Marker

	loginAddr   string
	gameAddr    string
	serverID    uint
	timeout     time.Duration
	localAddr   string
	allowlisted bool
	metrics     string
	// outcomeLog is the outcome log's file (-outcome-log); empty records
	// nothing.
	outcomeLog string
	// pprof is the loopback address of the profile endpoint; empty serves
	// none. It never shares the panel's listener.
	pprof string
	// listen is the panel's loopback address.
	listen          string
	datapack        string
	spawnMultiplier float64
	// spawnEvents is the server's SpawnEvents (-spawn-events).
	spawnEvents string
	gm          gmOptions
	store       storeOptions
	geo         geoOptions
}

func parseUIFlags(args []string, stderr io.Writer) (uiOptions, error) {
	var o uiOptions
	fs := flag.NewFlagSet("bots ui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.secretFile, "secret-file", "", "path of the secret file (mode 0600, or an owner-only ACL on Windows) the per-bot passwords derive from")
	fs.StringVar(&o.dataDir, "data-dir", "", "directory of the bot registry (bots.json), locked while the panel runs; default: "+botregistry.DefaultDirName+" under the user's config directory")
	fs.StringVar(&o.prefix, "prefix", "", "the panel's account prefix for new bots, lowercase letters and digits ending in a letter; required when the registry is created, then stored")
	fs.StringVar(&o.marker.Text, "name-marker", botnames.DefaultMarker.Text, "text that marks the panel's new bots' character names (with -prefix, when the registry is created)")
	placement := fs.String("name-marker-at", "suffix", "where the marker goes in a name: suffix or prefix")
	fs.IntVar(&o.maxOnline, "max-online", 10, "most bots online at once, on spawn and on restore; raise the server's MaximumOnlineUsers by as many")
	fs.StringVar(&o.importPrefix, "import-prefix", "", "import an earlier swarm's accounts of this prefix (with -import N) as never-create bots of the "+botregistry.ImportPreset+" preset")
	fs.IntVar(&o.importCount, "import", 0, "import accounts 1 to N of -import-prefix")
	fs.StringVar(&o.importMarker.Text, "import-marker", botnames.DefaultMarker.Text, "the name marker the imported swarm ran with")
	importPlacement := fs.String("import-marker-at", "suffix", "where the imported swarm's marker goes in a name: suffix or prefix")
	fs.StringVar(&o.loginAddr, "login", "127.0.0.1:2106", "login server address (host:port)")
	fs.StringVar(&o.gameAddr, "game", "", "game server address (host:port) to dial instead of the one the login server advertises")
	fs.UintVar(&o.serverID, "server-id", 1, "game server id to play on")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Second, "longest wait for each read or write of a login or game entry")
	fs.StringVar(&o.localAddr, "local-addr", "", "source IP of every login and game dial, when the host has several and only one is allowlisted on the servers; default: the OS picks")
	addAllowlistedFlag(fs, &o.allowlisted)
	addGMFlags(fs, &o.gm)
	addStoreFlags(fs, &o.store)
	addGeoFlags(fs, &o.geo)
	fs.StringVar(&o.datapack, "datapack", "", "server datapack XML directory (aCis_datapack/data/xml); the presets town and merchant and farming need it")
	fs.Float64Var(&o.spawnMultiplier, "spawn-multiplier", 1, "the server's SpawnMultiplier (npcs.properties), which scales how many bots a farming spot takes")
	addSpawnEventsFlag(fs, &o.spawnEvents)
	fs.StringVar(&o.metrics, "metrics", "", "loopback address (for example 127.0.0.1:6060) that serves live metrics at "+botmetrics.VarsPath+"; default: serve none (the panel's dashboard shows them)")
	fs.StringVar(&o.pprof, "pprof", "", pprofUsage)
	addOutcomeLogFlag(fs, &o.outcomeLog)
	fs.StringVar(&o.listen, "listen", defaultListen, "loopback address the web panel serves on (open http://ADDR/ in a browser)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	var err error
	if o.marker.Placement, err = botnames.ParsePlacement(*placement); err != nil {
		return o, fmt.Errorf("-name-marker-at: %w", err)
	}
	if o.importMarker.Placement, err = botnames.ParsePlacement(*importPlacement); err != nil {
		return o, fmt.Errorf("-import-marker-at: %w", err)
	}
	switch {
	case o.secretFile == "":
		return o, errors.New("no -secret-file: the per-bot passwords derive from it")
	case o.prefix == "" && (flagSet(fs, "name-marker") || flagSet(fs, "name-marker-at")):
		return o, errors.New("-name-marker and -name-marker-at go with -prefix")
	case o.maxOnline < 1:
		return o, fmt.Errorf("-max-online %d: want 1 or more", o.maxOnline)
	case (o.importPrefix == "") != (o.importCount == 0):
		return o, errors.New("-import-prefix and -import go together")
	case o.importPrefix == "" && (flagSet(fs, "import-marker") || flagSet(fs, "import-marker-at")):
		return o, errors.New("-import-marker and -import-marker-at go with -import-prefix")
	case o.serverID < 1 || o.serverID > 255:
		return o, fmt.Errorf("-server-id %d: want 1-255", o.serverID)
	case o.timeout <= 0:
		return o, fmt.Errorf("-timeout %s: want a positive duration", o.timeout)
	case o.spawnMultiplier <= 0 || math.IsNaN(o.spawnMultiplier) || math.IsInf(o.spawnMultiplier, 1):
		return o, fmt.Errorf("-spawn-multiplier %g: want a positive number", o.spawnMultiplier)
	}
	if o.prefix != "" {
		if err := botaccounts.ValidatePrefix(o.prefix, 1); err != nil {
			return o, fmt.Errorf("-prefix: %w", err)
		}
		if err := o.marker.Validate(); err != nil {
			return o, fmt.Errorf("-name-marker: %w", err)
		}
	}
	if o.importPrefix != "" {
		if err := botaccounts.ValidatePrefix(o.importPrefix, o.importCount); err != nil {
			return o, fmt.Errorf("-import-prefix: %w", err)
		}
		if err := o.importMarker.Validate(); err != nil {
			return o, fmt.Errorf("-import-marker: %w", err)
		}
	}
	if _, err := botclient.ParseLocalAddr(o.localAddr); err != nil {
		return o, fmt.Errorf("-local-addr: %w", err)
	}
	if err := o.gm.validate(); err != nil {
		return o, err
	}
	if err := o.store.validate(); err != nil {
		return o, err
	}
	if err := o.geo.validate(o.datapack); err != nil {
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
	if err := botpanel.CheckAddr(o.listen); err != nil {
		return o, fmt.Errorf("-listen: %w", err)
	}
	return o, nil
}

// prepareRegistry sets the panel's prefix of reg (stored at creation and
// checked after) and runs the -import the flags ask for. It runs before the
// supervisor owns reg.
func prepareRegistry(reg *botregistry.Registry, o uiOptions, log *slog.Logger) error {
	if o.prefix != "" {
		if err := reg.SetPanel(o.prefix, o.marker); err != nil {
			return err
		}
	}
	p, ok := reg.Panel()
	if !ok {
		return errors.New("a new registry needs -prefix")
	}
	log.Info("registry", "panel_prefix", p.Name, "marker", p.Marker.Text, "marker_at", p.Marker.Placement.String(), "bots", len(reg.Bots()))
	if o.importCount == 0 {
		return nil
	}
	added, err := reg.Import(o.importPrefix, o.importMarker, o.importCount, botregistry.ImportPreset)
	if err != nil {
		return fmt.Errorf("-import-prefix %s: %w", o.importPrefix, err)
	}
	log.Info("accounts imported", "prefix", o.importPrefix, "added", len(added))
	return nil
}

// uiRunner plays the runs of a panel's bots: each a botclient.Bot of the
// run's account, character and preset over the process-wide pieces.
type uiRunner struct {
	cfg    botclient.SessionConfig
	shared shared
	secret botaccounts.Secret
	// datapack is set when the panel loaded one.
	datapack string
}

// run is a botregistry.RunFunc. The password derives from the secret here
// and is never stored.
func (u uiRunner) run(ctx context.Context, r botregistry.Run) error {
	if err := validatePersona(r.Preset, u.datapack); err != nil {
		return fmt.Errorf("preset: %w", err)
	}
	p, _ := findPreset(r.Preset)
	if err := walkReady(p, u.shared.nav); err != nil {
		return fmt.Errorf("preset: %w", err)
	}
	cfg := u.cfg
	cfg.Markers = r.Markers
	brain := u.brain(p, r)
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	ch := r.Character
	if p.class != nil {
		// Used only when the bot creates its character.
		ch.Class = *p.class
	}
	b, err := botclient.NewBot(cfg, r.Account, u.secret.Password(r.Account), botclient.BotConfig{Character: ch, Brain: brain, Learned: r.Learned, Preset: r.Preset, Ledger: r.Ledger}, rng)
	if err != nil {
		return err
	}
	if r.Live != nil {
		r.Live(b)
	}
	return b.Run(ctx)
}

// brain is the BrainConfig of r's bot of preset p: the preset's, with the
// run's metrics slot, the bot's pinned branch (D22) and its PvP mode
// switches (docs/pvp/modes.md).
func (u uiRunner) brain(p preset, r botregistry.Run) botclient.BrainConfig {
	cfg := p.brain(u.shared)
	cfg.Metrics, cfg.Branch = r.Metrics, r.Branch
	if m := r.PvP; m != nil {
		v := &cfg.Persona.PvP
		v.Duels, v.Hunt, v.Arenas, v.ForceAttack = m.Duels, m.Hunt, m.Arenas, m.PK
	}
	return cfg
}

// runUI is `bots ui`: it locks the data directory, opens its bot registry,
// and runs the supervisor, which logs back in every bot marked online (up to
// -max-online, each with its own preset, all persistent) until a signal,
// which logs every bot out and keeps them marked online for the next start.
// Every login passes one LoginGate. The web panel (botpanel) on -listen
// drives the supervisor's requests and shows the bots, the gate, the metrics
// and the latest log lines.
func runUI(ctx context.Context, args []string, stderr io.Writer) int {
	o, err := parseUIFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "bots ui: %v\n", err)
		return 2
	}
	if err := playUI(ctx, o, stderr); err != nil {
		fmt.Fprintf(stderr, "bots ui: %v\n", err)
		return 1
	}
	return 0
}

func playUI(ctx context.Context, o uiOptions, stderr io.Writer) error {
	secret, err := botaccounts.LoadSecret(o.secretFile)
	if err != nil {
		return err
	}
	if err := secret.Strong(); err != nil {
		return fmt.Errorf("secret file %s: %w", o.secretFile, err)
	}
	dir := o.dataDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("no -data-dir: %w", err)
		}
		dir = filepath.Join(base, botregistry.DefaultDirName)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	reg, err := botregistry.Open(dir)
	if err != nil {
		return err
	}
	defer reg.Close()
	ring := botpanel.NewRing(logLines)
	log := slog.New(ring.Handler(slog.NewTextHandler(stderr, nil), nil))
	if err := prepareRegistry(reg, o, log); err != nil {
		return err
	}
	dialOpt, _ := botclient.ParseLocalAddr(o.localAddr) // validated by parseUIFlags
	clock := botclient.SystemClock{}
	w, cleanup, err := buildShared(wiringOptions{
		allowlisted: o.allowlisted, outcomeLog: o.outcomeLog, datapack: o.datapack, spawnMultiplier: o.spawnMultiplier,
		spawnEvents: o.spawnEvents, store: o.store, geo: o.geo, stores: true,
	}, clock, log)
	if err != nil {
		return err
	}
	// The navigation stays mapped and the outcome writer takes the stays of
	// every bot until all returned (sup.Run).
	defer cleanup()
	gate := w.gate
	runner := uiRunner{
		cfg: botclient.SessionConfig{
			LoginAddr:  o.loginAddr,
			GameAddr:   o.gameAddr,
			ServerID:   byte(o.serverID),
			Timeout:    o.timeout,
			Dial:       []botclient.DialOption{dialOpt},
			Persistent: true,
			Clock:      clock,
			Gate:       gate,
			Log:        log,
		},
		shared:   w.shared,
		secret:   secret,
		datapack: o.datapack,
	}
	// catalog names the items and effects of the panel's inspects.
	var catalog *botpanel.Catalog
	if w.dp != nil {
		catalog = botpanel.NewCatalog(w.dp)
	}
	auth, stopAuth, err := startAuthority(ctx, o.gm, clock, log)
	if err != nil {
		return err
	}
	// Deferred first, so it runs after the metrics endpoint stopped and
	// every bot returned.
	defer stopAuth()
	runner.shared.auth = auth
	// The dashboard shows the metrics: the registry runs with or without
	// -metrics, which only adds their endpoint.
	metrics := botmetrics.NewRegistry(botclient.RuleNames()...)
	gate.ReportBreaker(metrics)
	if n := runner.shared.nav; n != nil {
		n.ReportCache(metrics)
	}
	// The panel's bots form bot parties (D25): the organizer plans them
	// every botregistry.PartyTick.
	party := botparty.DefaultPolicy()
	sup, err := botregistry.New(botregistry.Config{
		Registry:    reg,
		Run:         runner.run,
		Gate:        gate,
		Clock:       clock,
		MaxOnline:   o.maxOnline,
		Metrics:     metrics,
		CheckPreset: func(name string) error { return validatePersona(name, o.datapack) },
		Log:         log,
		Party:       &party,
	})
	if err != nil {
		return err
	}
	panel, err := botpanel.New(botpanel.Config{
		Supervisor: sup,
		Gate:       gate,
		Metrics:    metrics,
		MaxOnline:  o.maxOnline,
		Presets:    offeredPresets(o.datapack, runner.shared.nav != nil),
		Spots:      runner.shared.spots,
		Catalog:    catalog,
		Logs:       ring,
		Log:        log,
	})
	if err != nil {
		return err
	}
	var metricsLn net.Listener
	if o.metrics != "" {
		if metricsLn, err = botmetrics.Listen(o.metrics); err != nil {
			return fmt.Errorf("metrics: %w", err)
		}
		log.Info("metrics serve", "addr", metricsLn.Addr().String(), "path", botmetrics.VarsPath)
	}
	tick := time.NewTicker(metricsPeriod)
	defer tick.Stop()
	defer serveMetrics(ctx, metricsLn, metrics, tick.C, log)()
	pprofLn, err := listenPprof(o.pprof, log)
	if err != nil {
		return err
	}
	defer servePprof(ctx, pprofLn, log)()
	ln, err := botpanel.Listen(o.listen)
	if err != nil {
		return err
	}
	// A panel that fails to serve stops the bots too; the supervisor's stop
	// ends the panel. Both are joined before playUI returns.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := panel.Serve(ctx, ln); err != nil {
			log.Error("panel failed", "err", err)
			cancel()
		}
	})
	log.Info("panel starts", "url", "http://"+ln.Addr().String()+"/", "data_dir", dir, "max_online", o.maxOnline, "allowlisted", o.allowlisted)
	err = sup.Run(ctx)
	cancel()
	wg.Wait()
	log.Info("panel stopped")
	return err
}

// offeredPresets are the presets of the catalog the panel lists: those that
// need the datapack are disabled, with the reason, without it.
func offeredPresets(datapack string, walkable bool) []botpanel.Preset {
	var out []botpanel.Preset
	for _, p := range presets {
		bp := botpanel.Preset{Name: p.name, Help: p.help}
		switch {
		case p.datapack && datapack == "":
			bp.Disabled = "needs -datapack"
		case p.walks && !walkable:
			bp.Disabled = "needs -geodata, -geoengine, -geodata-cache and -nav-cache"
		}
		out = append(out, bp)
	}
	return out
}
