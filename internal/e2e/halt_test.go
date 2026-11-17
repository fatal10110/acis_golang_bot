package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestConfigHaltFile(t *testing.T) {
	tests := []struct {
		secret, want string
	}{
		{"", ""},
		{"/p/.e2e/secret", "/p/.e2e/" + HaltFileName},
		{"/run/e2e.secret", "/run/" + HaltFileName},
	}
	for _, tt := range tests {
		if got := (Config{SecretFile: tt.secret}).HaltFile(); got != tt.want {
			t.Errorf("HaltFile of %q = %q, want %q", tt.secret, got, tt.want)
		}
	}
}

// TestTargetBreakerMarker: a refused credential leaves the marker that
// halts every later run; the asked-for wrong password leaves none.
func TestTargetBreakerMarker(t *testing.T) {
	const password = "the-derived-password"
	refused := &botclient.LoginError{Packet: "LoginFail", Reason: 0x02}
	tests := []struct {
		name   string
		cfg    Config
		trip   func(*Target) error
		n      *fakeNet
		marker bool
	}{
		{"refused login", Config{Allowlisted: true}, func(tg *Target) error {
			_, err := tg.Enter(context.Background(), "e2ea1", password, botclient.Character{Name: "e2eBot"}, nil)
			return err
		}, &fakeNet{loginErr: refused}, true},
		{"refused check", Config{Allowlisted: true}, func(tg *Target) error {
			return tg.CheckCredentials(context.Background(), "e2ea1", password)
		}, &fakeNet{checkErr: refused}, true},
		{"wrong password", Config{Allowlisted: true, AllowLoginFail: true}, func(tg *Target) error {
			return tg.WrongPassword(context.Background(), "e2ea1")
		}, &fakeNet{checkErr: refused}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.SecretFile = filepath.Join(t.TempDir(), "secret")
			tg, _ := fakeTarget(t, tt.cfg, tt.n)
			if err := tt.trip(tg); !errors.Is(err, refused) {
				t.Fatalf("trip = %v, want the refusal", err)
			}
			if !errors.Is(tg.Halted(), botclient.ErrLoginsHalted) {
				t.Fatal("the refusal did not trip the run's breaker")
			}
			path := tt.cfg.HaltFile()
			info, err := os.Stat(path)
			if !tt.marker {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("marker after the asked-for refusal: %v", err)
				}
				if err := checkHalt(path); err != nil {
					t.Fatalf("checkHalt = %v, want nil", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("no marker: %v", err)
			}
			if mode := info.Mode().Perm(); mode != 0o600 {
				t.Fatalf("marker mode %v, want 0600", mode)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(b); !strings.Contains(got, "e2ea1") || strings.Contains(got, password) {
				t.Fatalf("marker %q: want the account and no password", got)
			}
			if err := checkHalt(path); !errors.Is(err, botclient.ErrLoginsHalted) {
				t.Fatalf("checkHalt = %v, want ErrLoginsHalted", err)
			}
		})
	}
}

// TestNewTargetRefusesMarker: a later run stops before it reads the secret
// or dials anything while the marker exists.
func TestNewTargetRefusesMarker(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Login: "h:2106", SecretFile: filepath.Join(dir, "secret"), Prefix: DefaultPrefix}
	if err := checkHalt(cfg.HaltFile()); err != nil {
		t.Fatalf("checkHalt without a marker = %v", err)
	}
	if err := os.WriteFile(cfg.HaltFile(), []byte("account e2ea1: refused\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTarget(cfg, ""); !errors.Is(err, botclient.ErrLoginsHalted) {
		t.Fatalf("NewTarget with the marker = %v, want ErrLoginsHalted", err)
	}
}
