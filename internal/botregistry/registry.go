// Package botregistry is the control panel's bot registry and its
// supervisor (docs/plan.md section 8): the bots the panel created or
// imported, stored in bots.json in a data directory, and the one goroutine
// that owns them and runs them.
package botregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

const (
	// FileName is the registry file in the data directory.
	FileName = "bots.json"
	// LockName is the file the panel holds its lock on. It is not
	// FileName, which every save replaces.
	LockName = "bots.lock"
	// Version is the registry file format this package reads and writes.
	Version = 1
)

// ErrLocked reports a data directory another process holds: two panels
// never share a registry.
var ErrLocked = errors.New("data directory in use by another process")

// Prefix is one account prefix of the registry: the marker its bots' names
// carry and its account counter.
type Prefix struct {
	Name   string          `json:"name"`
	Marker botnames.Marker `json:"marker"`
	// Counter is the highest account number handed out or imported: the
	// next new bot of the prefix takes Counter+1. It only grows.
	Counter int `json:"counter"`
}

// Position is a bot's last known place in the world.
type Position struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
	Z int32 `json:"z"`
}

// Bot is one bot of the registry. It holds no secret: the password derives
// from the account and the secret file.
type Bot struct {
	Account string `json:"account"`
	Prefix  string `json:"prefix"`
	// Name is the character's name once a run learned it; empty before.
	Name string `json:"name,omitempty"`
	// Class is the character's class id: the starting class drawn for a new
	// bot, or the class a run learned. Nil for an imported bot not adopted
	// yet.
	Class *int32 `json:"class,omitempty"`
	// NeverCreate is set for an imported bot: it adopts a character bearing
	// one of its derived names, never creates one.
	NeverCreate bool   `json:"never_create,omitempty"`
	Preset      string `json:"preset"`
	// Online is whether the bot should be in the world: a panel start
	// restores the bots marked online.
	Online bool `json:"online"`
	// Level and Position are the last known ones; the status feed writes
	// them.
	Level    int32     `json:"level,omitempty"`
	Position *Position `json:"position,omitempty"`
	// StoppedAt and StopReason are when and why its last run ended.
	StoppedAt  time.Time `json:"stopped_at,omitzero"`
	StopReason string    `json:"stop_reason,omitempty"`
}

// Tombstone is an account the registry dropped: a deleted bot's, or one
// that held characters of no bot. It is never handed out again.
type Tombstone struct {
	Account string    `json:"account"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// file is the registry file's content.
type file struct {
	Version int `json:"version"`
	// Panel is the prefix new bots take their accounts from; empty until
	// the panel sets it (SetPanel).
	Panel      string      `json:"panel,omitempty"`
	Prefixes   []Prefix    `json:"prefixes"`
	Bots       []Bot       `json:"bots"`
	Tombstones []Tombstone `json:"tombstones,omitempty"`
}

// Registry is the open registry of one data directory, locked for the
// process. It is not safe for concurrent use: one goroutine owns it (the
// Supervisor, once it runs). A mutation applies in memory, then saves the
// whole file; a failed save is returned, and the next save writes it.
type Registry struct {
	dir  string
	lock *os.File
	data file
}

// Open locks dir and reads its registry; a directory without a registry
// file opens an empty one. A directory another process holds is ErrLocked;
// the lock is the process's, released when it closes the registry or dies.
func Open(dir string) (*Registry, error) {
	lock, err := os.OpenFile(filepath.Join(dir, LockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("registry lock: %w", err)
	}
	if err := tryLock(lock); err != nil {
		_ = lock.Close()
		if errors.Is(err, ErrLocked) {
			return nil, fmt.Errorf("%s: %w", dir, ErrLocked)
		}
		return nil, fmt.Errorf("registry lock %s: %w", dir, err)
	}
	r := &Registry{dir: dir, lock: lock, data: file{Version: Version}}
	if err := r.load(); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return r, nil
}

// Close releases the data directory.
func (r *Registry) Close() error { return r.lock.Close() }

func (r *Registry) load() error {
	path := filepath.Join(r.dir, FileName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("registry %s: %w", path, err)
	}
	if err := f.validate(); err != nil {
		return fmt.Errorf("registry %s: %w", path, err)
	}
	r.data = f
	return nil
}

// validate reports a file this package cannot use.
func (f file) validate() error {
	if f.Version != Version {
		return fmt.Errorf("version %d: want %d", f.Version, Version)
	}
	prefixes := make(map[string]bool, len(f.Prefixes))
	for _, p := range f.Prefixes {
		if err := botaccounts.ValidatePrefix(p.Name, max(p.Counter, 1)); err != nil {
			return err
		}
		if err := p.Marker.Validate(); err != nil {
			return fmt.Errorf("prefix %s: %w", p.Name, err)
		}
		if prefixes[p.Name] {
			return fmt.Errorf("prefix %s listed twice", p.Name)
		}
		prefixes[p.Name] = true
	}
	if f.Panel != "" && !prefixes[f.Panel] {
		return fmt.Errorf("panel prefix %s not listed", f.Panel)
	}
	held := make(map[string]bool, len(f.Bots)+len(f.Tombstones))
	for _, b := range f.Bots {
		if !prefixes[b.Prefix] {
			return fmt.Errorf("bot %s: prefix %q not listed", b.Account, b.Prefix)
		}
		if held[b.Account] {
			return fmt.Errorf("account %s listed twice", b.Account)
		}
		if b.Name != "" && !botnames.Valid(b.Name) {
			return fmt.Errorf("bot %s: character name %q: want 1-16 letters or digits", b.Account, b.Name)
		}
		held[b.Account] = true
	}
	for _, t := range f.Tombstones {
		if held[t.Account] {
			return fmt.Errorf("account %s listed twice", t.Account)
		}
		held[t.Account] = true
	}
	return nil
}

// save writes the registry atomically: a temp file in the data directory,
// synced, renamed over FileName.
func (r *Registry) save() error {
	b, err := json.MarshalIndent(r.data, "", "  ")
	if err != nil {
		return fmt.Errorf("registry save: %w", err)
	}
	tmp, err := os.CreateTemp(r.dir, ".bots-*.json")
	if err != nil {
		return fmt.Errorf("registry save: %w", err)
	}
	_, err = tmp.Write(append(b, '\n'))
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(r.dir, FileName))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("registry save: %w", err)
	}
	if d, err := os.Open(r.dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Panel returns the prefix new bots take their accounts from, once set.
func (r *Registry) Panel() (Prefix, bool) {
	if r.data.Panel == "" {
		return Prefix{}, false
	}
	return r.prefix(r.data.Panel)
}

// SetPanel sets the panel's own prefix and its marker, once: a registry
// that has one refuses another prefix or marker.
func (r *Registry) SetPanel(name string, m botnames.Marker) error {
	if r.data.Panel != "" {
		p, _ := r.Panel()
		if p.Name != name || p.Marker != m {
			return fmt.Errorf("the registry's panel prefix is %s with marker %s at %s, not %s with %s at %s",
				p.Name, p.Marker.Text, p.Marker.Placement, name, m.Text, m.Placement)
		}
		return nil
	}
	if err := r.addPrefix(name, m); err != nil {
		return err
	}
	r.data.Panel = name
	return r.save()
}

// addPrefix lists prefix name with marker m, or checks the listed one has m.
func (r *Registry) addPrefix(name string, m botnames.Marker) error {
	if err := botaccounts.ValidatePrefix(name, 1); err != nil {
		return err
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if p, ok := r.prefix(name); ok {
		if p.Marker != m {
			return fmt.Errorf("prefix %s has marker %s at %s in the registry, not %s at %s",
				name, p.Marker.Text, p.Marker.Placement, m.Text, m.Placement)
		}
		return nil
	}
	r.data.Prefixes = append(r.data.Prefixes, Prefix{Name: name, Marker: m})
	return nil
}

func (r *Registry) prefix(name string) (Prefix, bool) {
	i := slices.IndexFunc(r.data.Prefixes, func(p Prefix) bool { return p.Name == name })
	if i < 0 {
		return Prefix{}, false
	}
	return r.data.Prefixes[i], true
}

// Marker is the marker of prefix's bot names.
func (r *Registry) Marker(prefix string) (botnames.Marker, bool) {
	p, ok := r.prefix(prefix)
	return p.Marker, ok
}

// Markers are the markers of every prefix: the brains' bot filter.
func (r *Registry) Markers() botnames.Markers {
	ms := make(botnames.Markers, 0, len(r.data.Prefixes))
	for _, p := range r.data.Prefixes {
		if !slices.Contains(ms, p.Marker) {
			ms = append(ms, p.Marker)
		}
	}
	return ms
}

// Bots returns a copy of every bot, in the order they were added.
func (r *Registry) Bots() []Bot { return slices.Clone(r.data.Bots) }

// Bot returns the bot of account.
func (r *Registry) Bot(account string) (Bot, bool) {
	i := r.index(account)
	if i < 0 {
		return Bot{}, false
	}
	return r.data.Bots[i], true
}

func (r *Registry) index(account string) int {
	return slices.IndexFunc(r.data.Bots, func(b Bot) bool { return b.Account == account })
}

// Tombstones returns a copy of the dropped accounts.
func (r *Registry) Tombstones() []Tombstone { return slices.Clone(r.data.Tombstones) }

// Holds reports whether account is a bot's or a tombstone's.
func (r *Registry) Holds(account string) bool {
	return r.index(account) >= 0 || slices.ContainsFunc(r.data.Tombstones, func(t Tombstone) bool { return t.Account == account })
}

// NewBot adds b on the next account of the panel's prefix, one the registry
// never held, and moves the counter to it. b's Account and Prefix are
// replaced.
func (r *Registry) NewBot(b Bot) (Bot, error) {
	i := slices.IndexFunc(r.data.Prefixes, func(p Prefix) bool { return p.Name == r.data.Panel })
	if r.data.Panel == "" || i < 0 {
		return Bot{}, errors.New("the registry has no panel prefix")
	}
	p := &r.data.Prefixes[i]
	for {
		n := p.Counter + 1
		if err := botaccounts.ValidatePrefix(p.Name, n); err != nil {
			return Bot{}, err
		}
		p.Counter = n
		account := p.Name + strconv.Itoa(n)
		if !r.Holds(account) {
			b.Account, b.Prefix = account, p.Name
			r.data.Bots = append(r.data.Bots, b)
			return b, r.save()
		}
	}
}

// Import adds accounts 1 to n of prefix, named with marker m, that the
// registry does not hold yet (tombstones included): never-create bots of
// preset, offline. It moves prefix's counter past n, the panel's prefix
// included, and returns the bots it added.
func (r *Registry) Import(prefix string, m botnames.Marker, n int, preset string) ([]Bot, error) {
	if err := botaccounts.ValidatePrefix(prefix, n); err != nil {
		return nil, err
	}
	if err := r.addPrefix(prefix, m); err != nil {
		return nil, err
	}
	var added []Bot
	for _, account := range botaccounts.Names(prefix, n) {
		if r.Holds(account) {
			continue
		}
		b := Bot{Account: account, Prefix: prefix, NeverCreate: true, Preset: preset}
		r.data.Bots = append(r.data.Bots, b)
		added = append(added, b)
	}
	p := &r.data.Prefixes[slices.IndexFunc(r.data.Prefixes, func(p Prefix) bool { return p.Name == prefix })]
	p.Counter = max(p.Counter, n)
	return added, r.save()
}

// Update applies fn to account's bot and saves; account and prefix stay.
func (r *Registry) Update(account string, fn func(*Bot)) error {
	i := r.index(account)
	if i < 0 {
		return fmt.Errorf("no bot %s", account)
	}
	b := &r.data.Bots[i]
	prefix := b.Prefix
	fn(b)
	b.Account, b.Prefix = account, prefix
	return r.save()
}

// Drop removes account's bot and keeps a tombstone of its account.
func (r *Registry) Drop(account, reason string, at time.Time) error {
	i := r.index(account)
	if i < 0 {
		return fmt.Errorf("no bot %s", account)
	}
	r.data.Bots = slices.Delete(r.data.Bots, i, i+1)
	r.data.Tombstones = append(r.data.Tombstones, Tombstone{Account: account, Reason: reason, At: at})
	return r.save()
}
