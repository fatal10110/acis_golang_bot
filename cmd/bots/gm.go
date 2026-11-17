package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/gmdb"
	"github.com/fatal10110/acis_golang_bot/internal/ownerfile"
)

// maxDSNFileSize bounds the data source name file read.
const maxDSNFileSize = 4096

// gmOptions are who may command the bots by whisper (docs/plan.md, "In-game
// control"): an allowlist of GM object ids, or the server's database read
// with the data source name in a file. With neither every command is
// refused.
type gmOptions struct {
	commanders []int32
	// dsnFile is the path of the file (mode 0600, or an owner-only ACL on Windows) holding the data source
	// name: the name is a secret and never a flag value.
	dsnFile  string
	minLevel int
}

// addGMFlags registers the GM authority flags.
func addGMFlags(fs *flag.FlagSet, o *gmOptions) {
	fs.Func("commanders", "comma-separated object ids of the GMs who command the bots by whisper, with no database check; excludes -gm-dsn-file", func(s string) error {
		ids, err := parseCommanders(s)
		o.commanders = ids
		return err
	})
	fs.StringVar(&o.dsnFile, "gm-dsn-file", "", "path of the file (mode 0600, or an owner-only ACL on Windows) holding the game database's data source name the bots check a whispering GM's access level in (docs/gm-control.md); excludes -commanders")
	fs.IntVar(&o.minLevel, "gm-min-level", 0, "access level from which a character commands the bots (1-8) in place of the server's isGM levels; needs -gm-dsn-file")
}

// parseCommanders reads a comma-separated list of object ids.
func parseCommanders(s string) ([]int32, error) {
	var ids []int32
	for f := range strings.SplitSeq(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		id, err := strconv.ParseInt(f, 10, 32)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("object id %q: want a positive number", f)
		}
		ids = append(ids, int32(id))
	}
	if len(ids) == 0 {
		return nil, errors.New("no object id")
	}
	return ids, nil
}

func (o gmOptions) validate() error {
	if len(o.commanders) > 0 && o.dsnFile != "" {
		return errors.New("-commanders and -gm-dsn-file: choose one")
	}
	if o.minLevel != 0 && o.dsnFile == "" {
		return errors.New("-gm-min-level needs -gm-dsn-file")
	}
	if o.minLevel < 0 || o.minLevel > 8 {
		return fmt.Errorf("-gm-min-level %d: want 1-8", o.minLevel)
	}
	return nil
}

// readDSNFile reads the data source name from path, which only its owner
// may read (ownerfile.Check). Errors never carry the file's content.
func readDSNFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("gm dsn file: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("gm dsn file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("gm dsn file %s: not a regular file", path)
	}
	if err := ownerfile.Check(f); err != nil {
		return "", fmt.Errorf("gm dsn file %s: %w", path, err)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxDSNFileSize+1))
	if err != nil {
		return "", fmt.Errorf("gm dsn file %s: %w", path, err)
	}
	if len(b) > maxDSNFileSize {
		return "", fmt.Errorf("gm dsn file %s: longer than %d bytes", path, maxDSNFileSize)
	}
	dsn := string(bytes.TrimSpace(b))
	if dsn == "" {
		return "", fmt.Errorf("gm dsn file %s: empty", path)
	}
	return dsn, nil
}

// startAuthority builds the authority o describes and, with a database,
// runs its checks on a goroutine. The returned stop ends the checks, joins
// the goroutine and closes the database; call it after every bot returned.
// With neither commanders nor a database it returns a nil authority: no
// bot obeys anyone.
func startAuthority(ctx context.Context, o gmOptions, clock botclient.Clock, log *slog.Logger) (*botclient.Authority, func(), error) {
	switch {
	case len(o.commanders) > 0:
		log.Info("gm authority", "commanders", len(o.commanders))
		return botclient.NewAuthority(botclient.AuthorityConfig{Commanders: o.commanders, Clock: clock}), func() {}, nil
	case o.dsnFile == "":
		log.Info("gm authority", "commanders", 0, "database", false)
		return nil, func() {}, nil
	}
	dsn, err := readDSNFile(o.dsnFile)
	if err != nil {
		return nil, nil, err
	}
	levels, err := gmdb.Open(dsn)
	if err != nil {
		return nil, nil, err
	}
	a := botclient.NewAuthority(botclient.AuthorityConfig{Lookup: levels, MinLevel: int32(o.minLevel), Clock: clock, Log: log})
	// Commands still arrive while the bots log out after ctx ends: the
	// checks run until stop.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	wg.Go(func() { a.Run(ctx) })
	log.Info("gm authority", "database", true, "min_level", o.minLevel)
	return a, func() {
		cancel()
		wg.Wait()
		if err := levels.Close(); err != nil {
			log.Error("gm database close failed", "err", err)
		}
	}, nil
}
