package e2e

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/ownerfile"
)

// A run whose GM pool is not pinned (Config.GMMint) mints the GMs its
// scenarios lease (#367): the owner's GM files are its roots, and a root
// promotes each minted GM once, with //set access, while the minted
// character is out of the world. A minted GM is an account of the run's
// prefix, <prefix>gm<N>, whose password derives from the secret like every
// e2e account's, and whose one character is the account's name with the bot
// marker. The record of a prefix's minted accounts (names, never passwords)
// is a file next to the GM files, written under the mint lock, so two runs
// never mint the same N, and a later run reuses what an earlier one minted.

// mintedLevel is the access level a minted GM gets: Admin, the level of the
// owner's GMs (datapack accessLevels.xml).
const mintedLevel = 7

// promoteWait bounds the wait for the server's answer to //set access.
const promoteWait = 10 * time.Second

// sysMsgS1 is the system message of one text parameter, the answer of the
// admin commands (gameserver network/admin.go sendText, serverpackets
// SystemMessageS1).
const sysMsgS1 int32 = 1987

// maxMintRecordSize bounds the mint record.
const maxMintRecordSize = 64 << 10

// mintRecord is the record of the GMs a prefix's runs minted, in dir.
type mintRecord struct {
	dir, prefix string
}

// namespace is what every minted account name starts with.
func (r mintRecord) namespace() string { return r.prefix + "gm" }

// file is the record: "minted-<prefix>", which gmFiles never takes for a GM
// file. Its lock file (lockFile) is the mint lock.
func (r mintRecord) file() string { return filepath.Join(r.dir, "minted-"+r.prefix) }

// gmFile is the path the lock file of the minted GM account derives from
// (lockFile); no file is ever written there.
func (r mintRecord) gmFile(account string) string { return filepath.Join(r.dir, account) }

// mintedCharacter is the character name of the minted GM account.
func mintedCharacter(account string) string { return account + botnames.DefaultMarker.Text }

// mintedN is n of name, ns followed by n (from 1, no leading zero), and
// whether name is one.
func mintedN(name, ns string) (int, bool) {
	digits, ok := strings.CutPrefix(name, ns)
	if !ok || digits == "" || digits[0] == '0' || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil && len(name) <= botaccounts.MaxAccountLen
}

// isMintedCharacter reports whether name is the character of an account of
// namespace ns.
func isMintedCharacter(name, ns string) bool {
	account, ok := strings.CutSuffix(name, botnames.DefaultMarker.Text)
	if !ok {
		return false
	}
	_, ok = mintedN(account, ns)
	return ok
}

// names reads the record: the accounts minted so far, none without a
// record. Like the GM files it must be a regular file no one but its owner
// may access, and holds one account of the namespace a line; blank lines
// and lines starting with # are skipped.
func (r mintRecord) names() ([]string, error) {
	f, err := os.Open(r.file())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mint record: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("mint record: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("mint record %s: not a regular file", r.file())
	}
	if err := ownerfile.Check(f); err != nil {
		return nil, fmt.Errorf("mint record %s: %w", r.file(), err)
	}
	var names []string
	sc := bufio.NewScanner(io.LimitReader(f, maxMintRecordSize))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, ok := mintedN(line, r.namespace()); !ok {
			return nil, fmt.Errorf("mint record %s: line %d: not an account of %s<N>", r.file(), n, r.namespace())
		}
		names = append(names, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("mint record %s: %w", r.file(), err)
	}
	return names, nil
}

// next is the account to mint after names: the namespace followed by one
// more than the highest n of names.
func (r mintRecord) next(names []string) (string, error) {
	top := 0
	for _, name := range names {
		if n, ok := mintedN(name, r.namespace()); ok {
			top = max(top, n)
		}
	}
	account := r.namespace() + strconv.Itoa(top+1)
	if len(account) > botaccounts.MaxAccountLen || !botnames.Valid(mintedCharacter(account)) {
		return "", fmt.Errorf("mint GM: prefix %q too long for account %s and its character", r.prefix, account)
	}
	return account, nil
}

// add records account after names, through a new file of mode 0600 renamed
// over the record. Only the mint lock's holder calls it.
func (r mintRecord) add(names []string, account string) error {
	var b strings.Builder
	b.WriteString("# GM accounts the e2e harness minted for prefix " + r.prefix + " (#367): names only\n")
	for _, name := range append(names, account) {
		b.WriteString(name + "\n")
	}
	tmp := r.file() + ".new"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("mint record: %w", err)
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("mint record: %w", err)
	}
	_, err = f.WriteString(b.String())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, r.file())
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("mint record: %w", err)
	}
	return nil
}

// minted is the GM of the minted account: its password derives from secret.
func minted(account string, secret botaccounts.Secret) GM {
	return GM{Account: account, password: secret.Password(account), Character: mintedCharacter(account)}
}

// mintGM makes account, the next account of the run's minted namespace, a
// GM, on t's places (Hold): first the account's character, created by its
// first entry (the login server creates the account on its first login),
// which then leaves the world; then a root GM, leased from the owner's GM
// files, has the server store the character's access level. The server
// stores the level of a character out of the world ahead of the next
// selection's read (admin_offline.go adminByName, access_level.go
// storeOfflineAccessLevel), so the GM's next entry is a GM's: no relog
// follows. Its logins pass the run's gate and breaker; its one command
// passes the root's scope and the pacer.
func (t *Target) mintGM(ctx context.Context, account string) error {
	ch, err := botclient.NewCharacter(botnames.DefaultMarker, account, rand.New(rand.NewPCG(1, 1)))
	if err != nil {
		return err
	}
	name := mintedCharacter(account)
	ch.Names = []string{name}
	c, err := t.place(newScope()).enterWhenFree(ctx, account, t.Password(account), ch, nil)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if c.Self.Name != name {
		_ = c.Leave(ctx)
		return fmt.Errorf("create %s: the account plays %s", name, c.Self.Name)
	}
	if err := c.Leave(ctx); err != nil {
		return fmt.Errorf("create %s: leave: %w", name, err)
	}
	return t.promote(ctx, name)
}

// promote has a root GM set the access level of name, a character of the
// minted namespace out of the world, to mintedLevel.
func (t *Target) promote(ctx context.Context, name string) error {
	lease, err := t.roots.take(ctx, nil)
	if err != nil {
		return fmt.Errorf("root GM: %w", err)
	}
	gm, err := LoadGM(lease.file)
	if err != nil {
		lease.release()
		return err
	}
	scope := newScope()
	scope.mint = t.gms.mint.namespace()
	rt := t.place(scope)
	c, err := rt.enterWhenFree(ctx, gm.Account, gm.Password(), botclient.Character{Name: gm.Character}, nil)
	if err != nil {
		lease.release()
		return fmt.Errorf("root GM %v: %w", gm, err)
	}
	place := c.release
	c.release = func() { place(); lease.release() }
	defer func() { _ = c.Leave(ctx) }()
	rt.mu.Lock()
	delete(rt.scope.chars, strings.ToLower(gm.Character))
	rt.mu.Unlock()
	root := &GMClient{Client: c, t: rt, file: lease.file}
	if err := root.Command(ctx, fmt.Sprintf("set access %s %d", name, mintedLevel)); err != nil {
		return fmt.Errorf("promote %s: %w", name, err)
	}
	ev, err := root.Await(ctx, promoteWait, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.ID == sysMsgS1 && strings.HasPrefix(m.Text[0], name)
	})
	if err != nil {
		return fmt.Errorf("promote %s: no answer: %w", name, err)
	}
	if got, want := ev.(botclient.SystemMessage).Text[0], fmt.Sprintf("%s's access level is now set to %d.", name, mintedLevel); got != want {
		return fmt.Errorf("promote %s: the server answered %q", name, got)
	}
	return nil
}

// place is a target on t's run and places with its own GM command scope.
func (t *Target) place(scope *Scope) *Target {
	return &Target{run: t.run, slots: t.slots, scope: scope}
}
