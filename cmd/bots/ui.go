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
	"os"
	"path/filepath"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// dataDirName is the data directory of `bots ui` under os.UserConfigDir
// when -data-dir is not given.
const dataDirName = "acis-bots"

// uiOptions are the flags of `bots ui`.
type uiOptions struct {
	secretFile string
	// dataDir holds the registry; empty is dataDirName under the user's
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

	loginAddr       string
	gameAddr        string
	serverID        uint
	timeout         time.Duration
	localAddr       string
	allowlisted     bool
	metrics         string
	datapack        string
	spawnMultiplier float64
	gm              gmOptions
	store           storeOptions
}

func parseUIFlags(args []string, stderr io.Writer) (uiOptions, error) {
	var o uiOptions
	fs := flag.NewFlagSet("bots ui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.secretFile, "secret-file", "", "path of the secret file (mode 0600) the per-bot passwords derive from")
	fs.StringVar(&o.dataDir, "data-dir", "", "directory of the bot registry (bots.json), locked while the panel runs; default: "+dataDirName+" under the user's config directory")
	fs.StringVar(&o.prefix, "prefix", "", "the panel's account prefix for new bots, lowercase letters and digits ending in a letter; required when the registry is created, then stored")
	fs.StringVar(&o.marker.Text, "name-marker", botnames.DefaultMarker.Text, "text that marks the panel's new bots' character names (with -prefix, when the registry is created)")
	placement := fs.String("name-marker-at", "suffix", "where the marker goes in a name: suffix or prefix")
	fs.IntVar(&o.maxOnline, "max-online", 10, "most bots online at once, on spawn and on restore; raise the server's MaximumOnlineUsers by as many")
	fs.StringVar(&o.importPrefix, "import-prefix", "", "import an earlier swarm's accounts of this prefix (with -import N) as never-create support bots")
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
	fs.StringVar(&o.datapack, "datapack", "", "server datapack XML directory (aCis_datapack/data/xml); the presets town and merchant and farming need it")
	fs.Float64Var(&o.spawnMultiplier, "spawn-multiplier", 1, "the server's SpawnMultiplier (npcs.properties), which scales how many bots a farming spot takes")
	fs.StringVar(&o.metrics, "metrics", "", "loopback address (for example 127.0.0.1:6060) that serves live metrics at "+botmetrics.VarsPath+"; default: serve none")
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
	if o.metrics != "" {
		if err := botmetrics.CheckAddr(o.metrics); err != nil {
			return o, fmt.Errorf("-metrics: %w", err)
		}
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
	cfg := u.cfg
	cfg.Markers = r.Markers
	brain := p.brain(u.shared)
	brain.Metrics = r.Metrics
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	b, err := botclient.NewBot(cfg, r.Account, u.secret.Password(r.Account), botclient.BotConfig{Character: r.Character, Brain: brain, Learned: r.Learned}, rng)
	if err != nil {
		return err
	}
	return b.Run(ctx)
}

// runUI is `bots ui`: it locks the data directory, opens its bot registry,
// and runs the supervisor, which logs back in every bot marked online (up to
// -max-online, each with its own preset, all persistent) until a signal,
// which logs every bot out and keeps them marked online for the next start.
// Every login passes one LoginGate. The web panel that drives the
// supervisor's requests is not served yet.
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
		dir = filepath.Join(base, dataDirName)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	reg, err := botregistry.Open(dir)
	if err != nil {
		return err
	}
	defer reg.Close()
	log := slog.New(slog.NewTextHandler(stderr, nil))
	if err := prepareRegistry(reg, o, log); err != nil {
		return err
	}
	dialOpt, _ := botclient.ParseLocalAddr(o.localAddr) // validated by parseUIFlags
	clock := botclient.SystemClock{}
	gate := botclient.NewLoginGate(clock, nil)
	if o.allowlisted {
		gate = botclient.NewAllowlistedLoginGate(clock, nil)
	}
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
		shared:   shared{claims: botclient.NewPartyClaims(), storeBudget: o.store.budget},
		secret:   secret,
		datapack: o.datapack,
	}
	if o.datapack != "" {
		sh := &runner.shared
		if sh.spots, sh.travel, err = loadSpots(o.datapack, o.spawnMultiplier, log); err != nil {
			return err
		}
		sh.stores = o.store.registry(sh.travel, log)
	}
	auth, stopAuth, err := startAuthority(ctx, o.gm, clock, log)
	if err != nil {
		return err
	}
	// Deferred first, so it runs after the metrics endpoint stopped and
	// every bot returned.
	defer stopAuth()
	runner.shared.auth = auth
	var metrics *botmetrics.Registry
	if o.metrics != "" {
		metrics = botmetrics.NewRegistry(botclient.RuleNames()...)
		gate.ReportBreaker(metrics)
	}
	sup, err := botregistry.New(botregistry.Config{
		Registry:    reg,
		Run:         runner.run,
		Gate:        gate,
		Clock:       clock,
		MaxOnline:   o.maxOnline,
		Metrics:     metrics,
		CheckPreset: func(name string) error { return validatePersona(name, o.datapack) },
		Log:         log,
	})
	if err != nil {
		return err
	}
	if metrics != nil {
		ln, err := botmetrics.Listen(o.metrics)
		if err != nil {
			return err
		}
		tick := time.NewTicker(metricsPeriod)
		defer tick.Stop()
		defer serveMetrics(ctx, ln, metrics, tick.C, log)()
		log.Info("metrics serve", "addr", ln.Addr().String(), "path", botmetrics.VarsPath)
	}
	log.Info("panel starts", "data_dir", dir, "max_online", o.maxOnline, "allowlisted", o.allowlisted)
	err = sup.Run(ctx)
	log.Info("panel stopped")
	return err
}
