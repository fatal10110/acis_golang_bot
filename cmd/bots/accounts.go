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

// minLoginInterval bounds the accounts step's login rate. The login server's
// accept filter (server internal/commons/netutil/acceptfilter.go,
// DefaultAcceptFilterConfig) counts a try for every connection from an IP that
// arrives under 1 s after the previous one, never clears a try on a slower
// connection, and bans the IP on the 4th such burst connection until 300 s of
// silence. So no two logins may start within 1 s of each other: the pause runs
// after each login ends, so successive dials are at least this far apart, and
// the 250 ms above the window absorbs timer and scheduling jitter.
const minLoginInterval = 1250 * time.Millisecond

// defaultLoginInterval is the -interval default.
const defaultLoginInterval = 1500 * time.Millisecond

// addAllowlistedFlag registers -allowlisted, shared by every bots command
// that logs in.
func addAllowlistedFlag(fs *flag.FlagSet, p *bool) {
	fs.BoolVar(p, "allowlisted", false, "the bots' source address (-local-addr, or the one the OS picks) is on AcceptFilterAllowlist of both servers: logins are not spaced (docs/home.md)")
}

// accountsOptions are the flags of `bots accounts`.
type accountsOptions struct {
	accounts  botaccounts.Config
	loginAddr string
	timeout   time.Duration
	interval  time.Duration
	localAddr string
	// allowlisted drops the interval floor and defaults the interval to 0.
	allowlisted bool
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
	fs.DurationVar(&o.interval, "interval", defaultLoginInterval, fmt.Sprintf("pause between the end of one login and the next dial, at least %s; with -allowlisted any value from 0 and 0 when unset", minLoginInterval))
	fs.StringVar(&o.localAddr, "local-addr", "", "source IP to dial from, when the host has several and only one is allowlisted on the servers; default: the OS picks")
	addAllowlistedFlag(fs, &o.allowlisted)
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	if o.allowlisted {
		intervalSet := false
		fs.Visit(func(f *flag.Flag) { intervalSet = intervalSet || f.Name == "interval" })
		if !intervalSet {
			o.interval = 0
		}
		if o.interval < 0 {
			return o, fmt.Errorf("-interval %s: want 0 or more", o.interval)
		}
	} else if o.interval < minLoginInterval {
		return o, fmt.Errorf("-interval %s: want at least %s (the login server counts every connection under 1 s after the previous one from the IP and bans the IP on the 4th)", o.interval, minLoginInterval)
	}
	if _, err := botclient.ParseLocalAddr(o.localAddr); err != nil {
		return o, fmt.Errorf("-local-addr: %w", err)
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
	dialOpt, _ := botclient.ParseLocalAddr(o.localAddr) // validated by parseAccountsFlags
	check := func(ctx context.Context, account, password string) error {
		return botclient.CheckCredentials(ctx, o.loginAddr, account, password, o.timeout, dialOpt)
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

// pause waits d or until ctx ends; a d of 0 does not wait.
func pause(d time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		if d <= 0 {
			return ctx.Err()
		}
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
