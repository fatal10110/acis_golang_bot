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
// one is held until ctx ends.
func (p *gmPool) take(ctx context.Context) (*gmLease, error) {
	if len(p.files) == 0 {
		return nil, fmt.Errorf("no GM file (%s)", EnvGMFile)
	}
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
		timer := p.clock.NewTimer(gmRetry)
		select {
		case <-freed:
		case <-timer.C():
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("every GM of the pool (%d) is in use: %w", len(p.files), ctx.Err())
		}
		timer.Stop()
	}
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
