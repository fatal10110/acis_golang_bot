//go:build !windows

package ownerfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckMode(t *testing.T) {
	tests := []struct {
		perm   os.FileMode
		shared bool
	}{
		{perm: 0o600},
		{perm: 0o400},
		{perm: 0o700},
		{perm: 0o640, shared: true},
		{perm: 0o644, shared: true},
		{perm: 0o602, shared: true},
		{perm: 0o660, shared: true},
		{perm: 0o604, shared: true},
	}
	for _, tt := range tests {
		t.Run(tt.perm.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret")
			if err := os.WriteFile(path, []byte("x"), tt.perm); err != nil {
				t.Fatal(err)
			}
			// WriteFile's mode passes through the umask; set it exactly.
			if err := os.Chmod(path, tt.perm); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			err = Check(f)
			if got := errors.Is(err, ErrShared); got != tt.shared || (!tt.shared && err != nil) {
				t.Fatalf("Check = %v, want shared %v", err, tt.shared)
			}
		})
	}
}
