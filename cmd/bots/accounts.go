package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// minLoginInterval bounds the accounts step's login rate: one connection per
// interval stays under the login server's accept filter of 3 connections per
// second per IP (docs/plan.md, "Sessions, reconnect, accounts and host").
const minLoginInterval = 500 * time.Millisecond

// accountsOptions are the flags of `bots accounts`.
type accountsOptions struct {
	accounts  botaccounts.Config
	loginAddr string
	timeout   time.Duration
	interval  time.Duration
}

// addAccountFlags registers the flags that select a swarm instance's
// accounts; every bots command that logs in shares them. The secret is
// named by its file's path only: never pass a secret on the command line.
func addAccountFlags(fs *flag.FlagSet, c *botaccounts.Config) {
	fs.StringVar(&c.SecretFile, "secret-file", "", "path of the secret file (mode 0600) the per-bot passwords derive from")
	fs.StringVar(&c.Prefix, "prefix", "", "this swarm instance's account prefix: lowercase letters and digits ending in a letter")
	fs.IntVar(&c.Count, "bots", 0, "number of bot accounts: PREFIX1 to PREFIXN")
	fs.BoolVar(&c.Persistent, "persistent", false, "persistent swarm: keeps bots in the world, refuses a weak or placeholder secret")
}

func parseAccountsFlags(args []string, stderr io.Writer) (accountsOptions, error) {
	var o accountsOptions
	fs := flag.NewFlagSet("bots accounts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addAccountFlags(fs, &o.accounts)
	fs.StringVar(&o.loginAddr, "login", "127.0.0.1:2106", "login server address (host:port)")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Second, "longest wait for each login-server read or write")
	fs.DurationVar(&o.interval, "interval", time.Second, fmt.Sprintf("pause between logins, at least %s", minLoginInterval))
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	if o.interval < minLoginInterval {
		return o, fmt.Errorf("-interval %s: want at least %s (login server accepts under 3 connections per second per IP)", o.interval, minLoginInterval)
	}
	if o.timeout <= 0 {
		return o, fmt.Errorf("-timeout %s: want a positive duration", o.timeout)
	}
	return o, nil
}

// runAccounts is `bots accounts`: it logs each account of the swarm
// instance in once, up to the server list, so the login server creates it
// with its derived password (AutoCreateAccounts) or verifies the password of
// an existing one. Run it before the swarm, not beside one on the same IP.
func runAccounts(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	o, err := parseAccountsFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "bots accounts: %v\n", err)
		return 2
	}
	accounts, err := o.accounts.Accounts()
	if err != nil {
		fmt.Fprintf(stderr, "bots accounts: %v\n", err)
		return 1
	}
	check := func(ctx context.Context, account, password string) error {
		return botclient.CheckCredentials(ctx, o.loginAddr, account, password, o.timeout)
	}
	n, err := provision(ctx, accounts, check, pause(o.interval), stdout)
	if err != nil {
		fmt.Fprintf(stderr, "bots accounts: %v; stopped after %d of %d accounts\n", err, n, len(accounts))
		return 1
	}
	fmt.Fprintf(stdout, "%d accounts with prefix %s ready\n", n, o.accounts.Prefix)
	return 0
}

// checkFunc logs account in with password up to the server list.
type checkFunc func(ctx context.Context, account, password string) error

// provision checks each account in order, one login at a time, waiting on
// pace before every login after the first, and returns how many passed. The
// first failure stops it: a refused credential counts toward the login
// server's per-IP ban (3 in a row ban the IP for 10 minutes), so nothing
// here retries.
func provision(ctx context.Context, accounts []botaccounts.Account, check checkFunc, pace func(context.Context) error, out io.Writer) (int, error) {
	for i, a := range accounts {
		if i > 0 {
			if err := pace(ctx); err != nil {
				return i, err
			}
		}
		if err := check(ctx, a.Name, a.Password()); err != nil {
			if le, ok := errors.AsType[*botclient.LoginError](err); ok && le.Packet == "LoginFail" {
				return i, fmt.Errorf("account %s: %w (an existing account with another password, or AutoCreateAccounts off; all logins stopped to spare the IP a ban)", a.Name, err)
			}
			return i, fmt.Errorf("account %s: %w", a.Name, err)
		}
		fmt.Fprintf(out, "account %s ok\n", a.Name)
	}
	return len(accounts), nil
}

// pause waits d or until ctx ends.
func pause(d time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
}
