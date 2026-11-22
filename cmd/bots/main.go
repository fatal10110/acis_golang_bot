// Command bots runs playable bots against an acis_golang server through the
// real client protocol (docs/plan.md).
//
//	bots accounts -secret-file PATH -prefix PREFIX -bots N [-persistent] ...
//
// creates or verifies one swarm instance's accounts (see accounts.go).
//
//	bots swarm -secret-file PATH -prefix PREFIX -bots N [-persistent] ...
//
// plays them until a signal (see swarm.go).
//
//	bots ui -secret-file PATH [-data-dir DIR] [-prefix PREFIX] [-max-online N] ...
//
// runs the bots of a registry, the control panel's (see ui.go).
//
//	bots geopack -geodata DIR -geoengine FILE [-out geodata.cache]
//
// packs the listed geodata regions into the cache the bots map (see geopack.go).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run dispatches a subcommand and returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "accounts":
			return runAccounts(ctx, args[1:], stdout, stderr)
		case "swarm":
			return runSwarm(ctx, args[1:], stderr)
		case "ui":
			return runUI(ctx, args[1:], stderr)
		case "geopack":
			return runGeopack(args[1:], stdout, stderr)
		case "geoquery":
			return runGeoquery(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintln(stderr, "usage: bots accounts|swarm|ui|geopack|geoquery [flags]")
	return 2
}
