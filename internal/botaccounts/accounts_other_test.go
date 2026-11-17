//go:build !windows

package botaccounts

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestLoadSecretMode: a secret file with a group or other mode bit is
// refused. Windows ignores mode bits: accounts_windows_test.go checks the
// DACL instead.
func TestLoadSecretMode(t *testing.T) {
	good := genSecret(t)
	for _, perm := range []os.FileMode{0o644, 0o640, 0o602} {
		t.Run(perm.String(), func(t *testing.T) {
			_, err := LoadSecret(writeSecret(t, good, perm))
			if !errors.Is(err, ErrSecretMode) {
				t.Fatalf("err = %v, want ErrSecretMode", err)
			}
			if strings.Contains(err.Error(), good) {
				t.Fatal("error quotes the secret")
			}
		})
	}
}

// TestConfigAccountsRefusesALooseSecretFile: Accounts loads the secret
// through LoadSecret, so a 0644 secret file stops it.
func TestConfigAccountsRefusesALooseSecretFile(t *testing.T) {
	cfg := Config{SecretFile: writeSecret(t, genSecret(t), 0o644), Prefix: "bot", Count: 3, Persistent: true}
	if _, err := cfg.Accounts(); !errors.Is(err, ErrSecretMode) {
		t.Fatalf("err = %v, want ErrSecretMode", err)
	}
}
