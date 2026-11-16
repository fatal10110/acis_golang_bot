// Command bots runs playable bots against an acis_golang server through the
// real client protocol (docs/plan.md).
//
//	bots accounts -secret-file PATH -prefix PREFIX -bots N [-persistent] ...
//
// creates or verifies one swarm instance's accounts (see accounts.go).
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
	if len(args) > 0 && args[0] == "accounts" {
		return runAccounts(ctx, args[1:], stdout, stderr)
	}
	fmt.Fprintln(stderr, "usage: bots accounts [flags] (the swarm run is not implemented yet)")
	return 2
}
