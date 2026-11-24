package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// errLocked reports a GM lock another open file holds, in this process or
// another.
var errLocked = errors.New("GM in use")

// gmRetry spaces the pool's looks for a GM another process freed; a GM
// another scenario of the process frees wakes a waiter at once.
const gmRetry = 2 * time.Second

// gmWait bounds a scenario's wait for a GM, apart from the scenario's own
// deadline: a run that pins fewer GMs than it runs scenarios at once fails
// the scenario left without one at once, with the pool named, instead of
// letting it wait out its whole time budget (#250).
const gmWait = 2 * time.Minute

// gmPool hands each scenario a GM of its own: two logins of one account
// drop each other, so a GM serves one scenario at a time over every process
// of the machine. A scenario holds its GM through a lock on the GM file's
// lockFile, which the kernel drops when the file closes or the process dies.
// A pool of GM files (a pinned pool, or the roots of a minting one) holds
// files; a minting pool holds the GMs its record names and mints another
// when every one is held.
type gmPool struct {
	files []string
	mint  *mintRecord // nil: a pool of files, which never mints
	clock botclient.Clock

	mu    sync.Mutex
	freed chan struct{} // closed when a lease of the process ends; guarded by mu
}

func newGMPool(files []string, clock botclient.Clock) *gmPool {
	return &gmPool{files: files, clock: clock, freed: make(chan struct{})}
}

// newMintPool is the minting pool of prefix's GMs recorded in dir.
func newMintPool(dir, prefix string, clock botclient.Clock) *gmPool {
	return &gmPool{mint: &mintRecord{dir: dir, prefix: prefix}, clock: clock, freed: make(chan struct{})}
}

// gmLease is a GM a scenario holds.
type gmLease struct {
	file string // the GM file; for a minted GM what its lock file derives from
	// minted is the minted GM's account, "" for a GM file.
	minted string
	lock   *os.File
	pool   *gmPool
	once   sync.Once
}

// lockFile is the lock of GM file gm.
func lockFile(gm string) string { return gm + ".lock" }

// minter makes account, the next account of a minting pool, a GM
// (Target.mintGM).
type minter func(ctx context.Context, account string) error

// take leases the first GM of the pool no one holds. With every one held a
// minting pool mints another through mint, one mint at a time over every
// process (the record's lock); otherwise it waits for at most gmWait, or
// until ctx ends. mint is nil for a pool of files.
func (p *gmPool) take(ctx context.Context, mint minter) (*gmLease, error) {
	if len(p.files) == 0 && p.mint == nil {
		return nil, fmt.Errorf("no GM file (%s)", EnvGMFile)
	}
	deadline := p.clock.Now().Add(gmWait)
	for {
		p.mu.Lock()
		freed := p.freed
		p.mu.Unlock()
		if l, err := p.tryTake(); l != nil || err != nil {
			return l, err
		}
		if p.mint != nil && mint != nil {
			// nil, nil: another scenario is minting; wait for it.
			if l, err := p.mintNext(ctx, mint); l != nil || err != nil {
				return l, err
			}
		}
		left := deadline.Sub(p.clock.Now())
		if left <= 0 {
			return nil, p.exhausted(fmt.Sprintf("for %s", gmWait))
		}
		timer := p.clock.NewTimer(min(gmRetry, left))
		select {
		case <-freed:
		case <-timer.C():
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("%w: %w", p.exhausted("until the scenario ended"), ctx.Err())
		}
		timer.Stop()
	}
}

// exhausted is the error of a take that found every GM held for how long.
func (p *gmPool) exhausted(how string) error {
	if p.mint != nil {
		return fmt.Errorf("GM pool exhausted: every minted GM (%s<N>) was in use and another scenario was minting one, %s", p.mint.namespace(), how)
	}
	return fmt.Errorf("GM pool exhausted: every GM of the pool (%d) was in use %s; pin as many GMs (%s) as scenarios run at once, run them one at a time, or leave %s unset to mint GMs", len(p.files), how, EnvGMFile, EnvGMFile)
}

// tryTake leases the first GM of the pool no one holds, nil when every one
// is held.
func (p *gmPool) tryTake() (*gmLease, error) {
	if p.mint == nil {
		for _, gm := range p.files {
			if l, err := p.lease(gm, ""); l != nil || err != nil {
				return l, err
			}
		}
		return nil, nil
	}
	names, err := p.mint.names()
	if err != nil {
		return nil, err
	}
	for _, account := range names {
		if l, err := p.lease(p.mint.gmFile(account), account); l != nil || err != nil {
			return l, err
		}
	}
	return nil, nil
}

// lease locks the GM of file, nil when another holds it.
func (p *gmPool) lease(file, minted string) (*gmLease, error) {
	f, err := lockGM(file)
	if errors.Is(err, errLocked) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &gmLease{file: file, minted: minted, lock: f, pool: p}, nil
}

// mintNext mints the next GM of the record under its lock and leases it;
// nil, nil while another holds the lock. It first looks again for a GM no
// one holds, which another run may have minted meanwhile. Lock order: the
// mint lock, then the new GM's, then a root's (mint).
func (p *gmPool) mintNext(ctx context.Context, mint minter) (*gmLease, error) {
	lock, err := lockGM(p.mint.file())
	if errors.Is(err, errLocked) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mint lock: %w", err)
	}
	defer lock.Close()
	if l, err := p.tryTake(); l != nil || err != nil {
		return l, err
	}
	names, err := p.mint.names()
	if err != nil {
		return nil, err
	}
	account, err := p.mint.next(names)
	if err != nil {
		return nil, err
	}
	l, err := p.lease(p.mint.gmFile(account), account)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, fmt.Errorf("mint GM %s: its lock is held, yet the record does not name it", account)
	}
	if err := mint(ctx, account); err != nil {
		_ = l.lock.Close()
		return nil, fmt.Errorf("mint GM %s: %w", account, err)
	}
	if err := p.mint.add(names, account); err != nil {
		_ = l.lock.Close()
		return nil, err
	}
	return l, nil
}

// load is the leased GM: a GM file's, or the minted account's with its
// password derived from secret.
func (l *gmLease) load(secret botaccounts.Secret) (GM, error) {
	if l.minted != "" {
		return minted(l.minted, secret), nil
	}
	return LoadGM(l.file)
}

// gmShortfall is the warning for a run whose pool holds gms GMs while up to
// parallel scenarios of it run at once (go test -parallel, capped by
// MaxScenarios), "" when every one of them can hold a GM.
func gmShortfall(gms, parallel int) string {
	at := min(parallel, MaxScenarios)
	if gms == 0 || gms >= at {
		return ""
	}
	return fmt.Sprintf("%s pins %d GM(s) for up to %d scenarios at once: a scenario that needs a GM while every one is held fails with \"GM pool exhausted\" after %s; pin as many GMs as scenarios run at once, or run them one at a time (-parallel 1)", EnvGMFile, gms, at, gmWait)
}

// lockGM opens gm's lock file and locks it without waiting.
func lockGM(gm string) (*os.File, error) {
	f, err := os.OpenFile(lockFile(gm), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("GM lock: %w", err)
	}
	if err := tryLock(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("GM lock %s: %w", lockFile(gm), err)
	}
	return f, nil
}

// release gives the GM back to the pool, once.
func (l *gmLease) release() {
	l.once.Do(func() {
		_ = l.lock.Close()
		l.pool.mu.Lock()
		close(l.pool.freed)
		l.pool.freed = make(chan struct{})
		l.pool.mu.Unlock()
	})
}
