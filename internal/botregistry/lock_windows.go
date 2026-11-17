package botregistry

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock takes an exclusive lock on the first byte of f without waiting;
// ErrLocked when another handle holds it. Windows drops the lock when the
// handle closes or the process dies.
func tryLock(f *os.File) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrLocked
	}
	return err
}
