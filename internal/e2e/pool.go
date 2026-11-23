package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

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
type gmPool struct {
	files []string
	clock botclient.Clock

	mu    sync.Mutex
	freed chan struct{} // closed when a lease of the process ends; guarded by mu
}

func newGMPool(files []string, clock botclient.Clock) *gmPool {
	return &gmPool{files: files, clock: clock, freed: make(chan struct{})}
}

// gmLease is a GM file a scenario holds.
type gmLease struct {
	file string
	lock *os.File
	pool *gmPool
	once sync.Once
}

// lockFile is the lock of GM file gm.
func lockFile(gm string) string { return gm + ".lock" }

// take leases the first GM of the pool no one holds, waiting while every
// one is held for at most gmWait, or until ctx ends.
func (p *gmPool) take(ctx context.Context) (*gmLease, error) {
	if len(p.files) == 0 {
		return nil, fmt.Errorf("no GM file (%s)", EnvGMFile)
	}
	deadline := p.clock.Now().Add(gmWait)
	for {
		p.mu.Lock()
		freed := p.freed
		p.mu.Unlock()
		for _, gm := range p.files {
			f, err := lockGM(gm)
			if err == nil {
				return &gmLease{file: gm, lock: f, pool: p}, nil
			}
			if !errors.Is(err, errLocked) {
				return nil, err
			}
		}
		left := deadline.Sub(p.clock.Now())
		if left <= 0 {
			return nil, fmt.Errorf("GM pool exhausted: every GM of the pool (%d) was in use for %s; pin as many GMs (%s) as scenarios run at once, or run them one at a time", len(p.files), gmWait, EnvGMFile)
		}
		timer := p.clock.NewTimer(min(gmRetry, left))
		select {
		case <-freed:
		case <-timer.C():
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("GM pool exhausted: every GM of the pool (%d) is in use: %w", len(p.files), ctx.Err())
		}
		timer.Stop()
	}
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
