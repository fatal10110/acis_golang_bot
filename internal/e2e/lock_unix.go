//go:build unix

package e2e

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive flock on f without waiting; errLocked when
// another open file description holds it. The kernel drops the lock when the
// file closes or the process dies.
func tryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errLocked
	}
	return err
}
