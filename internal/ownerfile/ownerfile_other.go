//go:build !windows

package ownerfile

import (
	"fmt"
	"os"
)

// Check returns nil when f, an open file, has no group or other mode bit
// (0600 or stricter); otherwise an error wrapping ErrShared that names the
// mode. The error never carries the file's content.
func Check(f *os.File) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("mode %#o, want 0600: %w", perm, ErrShared)
	}
	return nil
}
