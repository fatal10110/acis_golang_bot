package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// pprofUsage is the -pprof flag's help text, shared by swarm and ui.
const pprofUsage = "loopback address (for example 127.0.0.1:6061) that serves Go profiles at " + botmetrics.PprofPath + " (an owner diagnostic; heap profiles can hold strings); default: serve none"

// checkPprofFlag validates a -pprof value; empty is off.
func checkPprofFlag(addr string) error {
	if addr == "" {
		return nil
	}
	if err := botmetrics.CheckAddr(addr); err != nil {
		return fmt.Errorf("-pprof: %w", err)
	}
	return nil
}

// listenPprof binds the profile endpoint; empty addr serves none and
// returns a nil listener. Called before the first login, so a taken port
// fails the run, not the bots.
func listenPprof(addr string, log *slog.Logger) (net.Listener, error) {
	if addr == "" {
		return nil, nil
	}
	ln, err := botmetrics.Listen(addr)
	if err != nil {
		return nil, fmt.Errorf("pprof: %w", err)
	}
	log.Info("pprof serve", "addr", ln.Addr().String(), "path", botmetrics.PprofPath)
	return ln, nil
}

// servePprof serves the profiles on ln, unless ln is nil, on a goroutine
// that outlives ctx's cancellation (the endpoint serves while the bots log
// out). The returned stop ends it, joins it and closes ln.
func servePprof(ctx context.Context, ln net.Listener, log *slog.Logger) (stop func()) {
	if ln == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := botmetrics.ServePprof(ctx, ln); err != nil {
			log.Error("pprof endpoint failed", "err", err)
		}
	})
	return func() {
		cancel()
		wg.Wait()
	}
}
