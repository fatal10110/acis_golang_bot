// Package e2e is the e2e harness: the target scenarios run against, set by
// BOT_E2E_* environment variables that point at files, and the safety
// envelope every scenario passes through (the login gate and its credential
// breaker, the cap on characters online, the GM command filter and the send
// pacer). The scenarios are the package's _test files: each skips when its
// config is missing and fails instead under BOT_REQUIRE_E2E=1. The runbook
// is docs/e2e-live.md.
package e2e

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The environment variables of the harness. None carries a secret: the
// secret and the GM's password are read from the files they name.
const (
	// EnvRequire=1 makes a scenario whose config is missing fail instead of
	// skip; set it whenever a run is claimed.
	EnvRequire = "BOT_REQUIRE_E2E"
	// EnvLogin is the login server's host:port.
	EnvLogin = "BOT_E2E_LOGIN"
	// EnvSecretFile is the HMAC secret file (mode 0600) of the e2e accounts.
	EnvSecretFile = "BOT_E2E_SECRET_FILE"
	// EnvPrefix is the e2e accounts' prefix, DefaultPrefix when unset.
	EnvPrefix = "BOT_E2E_PREFIX"
	// EnvAllowlisted=1 drops the login gate's spacing: set it only when the
	// source address is on AcceptFilterAllowlist of both servers.
	EnvAllowlisted = "BOT_E2E_ALLOWLISTED"
	// EnvGMFile is the GM file (mode 0600: account, password and character),
	// or a list of them (filepath.ListSeparator between): the run's GM pool,
	// one GM per scenario open. Unset, the pool is every GM file of the
	// config dir (gmFiles).
	EnvGMFile = "BOT_E2E_GM_FILE"
	// EnvDatapack is a local copy of the server's datapack XML directory.
	EnvDatapack = "BOT_E2E_DATAPACK"
	// EnvAllowLoginFail=1 enables the wrong-password scenario, which trips
	// the credential breaker for the rest of its process.
	EnvAllowLoginFail = "BOT_E2E_ALLOW_LOGINFAIL"
	// EnvGeodata, EnvGeoengine, EnvGeodataCache and EnvNavCache are the
	// navigation the whole bots walk on (botclient.NavFiles, D13a): the
	// geodata region directory, the local copy of the server's
	// geoengine.properties, the `bots geopack` cache and the `bots navbuild`
	// cache, all built outside the worktree (docs/home.md).
	EnvGeodata      = "BOT_E2E_GEODATA"
	EnvGeoengine    = "BOT_E2E_GEOENGINE"
	EnvGeodataCache = "BOT_E2E_GEODATA_CACHE"
	EnvNavCache     = "BOT_E2E_NAV_CACHE"
)

// DefaultPrefix is the e2e accounts' prefix when EnvPrefix is unset.
const DefaultPrefix = "e2e"

// ConfigDirName is the gitignored folder at the root of the primary checkout
// that holds the default secret and GM files.
const ConfigDirName = ".e2e"

// ServerID is the game server the e2e characters play on.
const ServerID byte = 1

// reservedPrefixes are account prefixes no run may share: the old swarm's,
// still running on the live host.
var reservedPrefixes = []string{"hz"}

// Config is the target of a run.
type Config struct {
	Login       string
	SecretFile  string
	Prefix      string
	Allowlisted bool
	GMFiles     []string
	// GMMint is the folder of the GM files when no GM file is pinned: the
	// run mints the GMs its scenarios lease (#367) and records them there,
	// the GM files being the roots that promote them; "" for a pinned
	// pool, which never mints.
	GMMint         string
	Datapack       string
	AllowLoginFail bool
	// Nav names the caches of NeedNav.
	Nav botclient.NavFiles
}

// ConfigFromEnv reads the config through getenv; the secret and GM files
// default to dir's (DefaultDir), none when dir is empty.
func ConfigFromEnv(getenv func(string) string, dir string) Config {
	c := Config{
		Login:          getenv(EnvLogin),
		SecretFile:     getenv(EnvSecretFile),
		Prefix:         cmp.Or(getenv(EnvPrefix), DefaultPrefix),
		Allowlisted:    getenv(EnvAllowlisted) == "1",
		Datapack:       getenv(EnvDatapack),
		AllowLoginFail: getenv(EnvAllowLoginFail) == "1",
		Nav: botclient.NavFiles{
			Geodata: getenv(EnvGeodata), Geoengine: getenv(EnvGeoengine),
			Cache: getenv(EnvGeodataCache), NavCache: getenv(EnvNavCache),
		},
	}
	if v := getenv(EnvGMFile); v != "" {
		c.GMFiles = filepath.SplitList(v)
	}
	if dir != "" {
		c.SecretFile = cmp.Or(c.SecretFile, filepath.Join(dir, "secret"))
		if len(c.GMFiles) == 0 {
			c.GMFiles = gmFiles(dir)
			c.GMMint = dir
		}
	}
	return c
}

// gmFiles are the GM files of dir: "gm" and "gm" followed by digits ("gm2",
// "gm3", ...), in name order; none when dir cannot be read.
func gmFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		digits := strings.TrimPrefix(name, "gm")
		if e.Type().IsRegular() && digits != name && strings.Trim(digits, "0123456789") == "" {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files
}

// HaltFileName is the credential breaker's marker, next to the secret file:
// a refused credential leaves it, and while it exists no run logs in
// (NewTarget). The login server counts failed logins per address over every
// account and keeps the count for at least an hour; only a good login of the
// same account takes one back, which a stale secret never gives. So a run
// halted by a refusal halts every later one until the owner fixes the secret
// and deletes the marker.
const HaltFileName = "halted"

// HaltFile is the breaker's marker for c: HaltFileName in the secret file's
// folder, or "" without a secret file.
func (c Config) HaltFile() string {
	if c.SecretFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.SecretFile), HaltFileName)
}

// checkHalt reports the marker an earlier run's refused credential left at
// path, wrapping botclient.ErrLoginsHalted; nil when there is none.
func checkHalt(path string) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("credential breaker marker: %w", err)
	}
	return fmt.Errorf("%w by an earlier run (%s: %s): fix the secret file, then delete the marker",
		botclient.ErrLoginsHalted, path, strings.TrimSpace(string(b)))
}

// DefaultDir is the ConfigDirName folder at the root of the primary
// checkout, found through git from the working directory (a linked worktree
// shares the primary checkout's git directory), or "" outside a checkout.
func DefaultDir() string {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) != ".git" {
		return ""
	}
	return filepath.Join(filepath.Dir(common), ConfigDirName)
}

// Need is what a scenario needs besides the login server and the secret,
// which every scenario needs.
type Need uint8

const (
	// NeedGM is the GM file.
	NeedGM Need = iota + 1
	// NeedDatapack is the datapack copy.
	NeedDatapack
	// NeedLoginFail is EnvAllowLoginFail.
	NeedLoginFail
	// NeedNav is the navigation a whole bot walks on: the geodata and nav
	// caches with the datapack, whose doors.xml the door table reads.
	NeedNav
)

// Missing names what c lacks for a scenario of needs: an unset variable or
// a file that is not there. None means it may run.
func (c Config) Missing(needs ...Need) []string {
	var miss []string
	if c.Login == "" {
		miss = append(miss, EnvLogin+" unset")
	}
	miss = missingFile(miss, EnvSecretFile, c.SecretFile)
	for _, n := range needs {
		switch n {
		case NeedGM:
			if len(c.GMFiles) == 0 {
				miss = append(miss, EnvGMFile+" unset and no GM file in the config dir")
			}
			for _, f := range c.GMFiles {
				miss = missingFile(miss, EnvGMFile, f)
			}
		case NeedDatapack:
			miss = missingFile(miss, EnvDatapack, c.Datapack)
		case NeedLoginFail:
			if !c.AllowLoginFail {
				miss = append(miss, EnvAllowLoginFail+"=1 unset")
			}
		case NeedNav:
			miss = missingFile(miss, EnvDatapack, c.Datapack)
			miss = missingFile(miss, EnvGeodata, c.Nav.Geodata)
			miss = missingFile(miss, EnvGeoengine, c.Nav.Geoengine)
			miss = missingFile(miss, EnvGeodataCache, c.Nav.Cache)
			miss = missingFile(miss, EnvNavCache, c.Nav.NavCache)
		}
	}
	return miss
}

func missingFile(miss []string, env, path string) []string {
	if path == "" {
		return append(miss, env+" unset")
	}
	if _, err := os.Stat(path); err != nil {
		return append(miss, fmt.Sprintf("%s: %v", env, err))
	}
	return miss
}

// ErrReservedPrefix reports an e2e prefix that overlaps the accounts of
// bots the run must leave alone.
var ErrReservedPrefix = errors.New("prefix reserved")

// CheckPrefix refuses an e2e prefix that is not a valid account prefix, or
// that overlaps (one starts with the other) the old swarm's prefix or panel,
// the prefix of the owner's `bots ui` panel ("" for none): the run would log
// in as their bots.
func CheckPrefix(prefix, panel string) error {
	if err := botaccounts.ValidatePrefix(prefix, 1); err != nil {
		return err
	}
	for _, r := range append([]string{panel}, reservedPrefixes...) {
		if r != "" && (strings.HasPrefix(prefix, r) || strings.HasPrefix(r, prefix)) {
			return fmt.Errorf("e2e prefix %q overlaps %q: %w", prefix, r, ErrReservedPrefix)
		}
	}
	return nil
}
