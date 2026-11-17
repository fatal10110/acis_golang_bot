package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// exitStopped is the exit code of a swarm that stopped by itself: the
// credential breaker tripped, or every bot stopped for a reason a restart
// would only repeat (a refused credential, a taken account, repeated
// aborts). The systemd unit does not restart on it
// (RestartPreventExitStatus): a restart loop is a login loop.
const exitStopped = 3

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
	fs.StringVar(&o.localAddr, "local-addr", "", "source IP of every login and game dial (for example 127.0.0.2, the one address allowlisted on the servers); default: the OS picks")
	fs.StringVar(&o.record, "record", "", "directory that receives the first frame of every game opcode read (test fixtures); default: record nothing")
	fs.StringVar(&o.marker.Text, "name-marker", botnames.DefaultMarker.Text, "text that marks the bots' character names")
	placement := fs.String("name-marker-at", "suffix", "where the marker goes in a name: suffix or prefix")
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
	return o, nil
}

// runSwarm is `bots swarm`: it plays one bot per account of the swarm
// instance (-bots N of them, never more, each one session at a time) until
// a signal, which logs every bot out. Every login and game dial of the
// process passes one LoginGate (see botclient). Reports go to stderr.
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
	cfg := botclient.SessionConfig{
		LoginAddr:  o.loginAddr,
		GameAddr:   o.gameAddr,
		ServerID:   byte(o.serverID),
		Timeout:    o.timeout,
		Dial:       []botclient.DialOption{dialOpt},
		Recorder:   rec,
		Persistent: o.accounts.Persistent,
		Marker:     o.marker,
		Brain:      botclient.BrainConfig{Persona: botclient.DefaultPersona()},
		Clock:      clock,
		Gate:       gate,
		Log:        log,
	}
	bots, err := newBots(cfg, accounts)
	if err != nil {
		fmt.Fprintf(stderr, "bots swarm: %v\n", err)
		return 1
	}
	log.Info("swarm starts", "bots", len(bots), "prefix", o.accounts.Prefix, "persistent", o.accounts.Persistent)
	var wg sync.WaitGroup
	for _, b := range bots {
		wg.Go(func() {
			if err := b.Run(ctx); err != nil {
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

// newBots returns one bot per account.
func newBots(cfg botclient.SessionConfig, accounts []botaccounts.Account) ([]*botclient.Bot, error) {
	rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	bots := make([]*botclient.Bot, 0, len(accounts))
	for _, a := range accounts {
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
