package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// testAccounts loads n accounts with prefix from a freshly generated secret.
func testAccounts(t *testing.T, prefix string, n int) []botaccounts.Account {
	t.Helper()
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bots.secret")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(b)), 0o600); err != nil {
		t.Fatal(err)
	}
	accounts, err := botaccounts.Config{SecretFile: path, Prefix: prefix, Count: n, Persistent: true}.Accounts()
	if err != nil {
		t.Fatal(err)
	}
	return accounts
}

func TestParseAccountsFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    accountsOptions
		wantErr string
	}{
		{
			name: "defaults",
			args: []string{"-secret-file", "/run/bots.secret", "-prefix", "swarma", "-bots", "20"},
			want: accountsOptions{
				accounts:  botaccounts.Config{SecretFile: "/run/bots.secret", Prefix: "swarma", Count: 20},
				loginAddr: "127.0.0.1:2106", timeout: 10 * time.Second, interval: time.Second,
			},
		},
		{
			name: "persistent",
			args: []string{"-persistent", "-prefix", "p", "-bots", "1", "-login", "10.0.0.1:2106", "-interval", "2s"},
			want: accountsOptions{
				accounts:  botaccounts.Config{Prefix: "p", Count: 1, Persistent: true},
				loginAddr: "10.0.0.1:2106", timeout: 10 * time.Second, interval: 2 * time.Second,
			},
		},
		{name: "interval past the login rate", args: []string{"-interval", "300ms"}, wantErr: "-interval"},
		{name: "zero timeout", args: []string{"-timeout", "0s"}, wantErr: "-timeout"},
		{name: "stray argument", args: []string{"extra"}, wantErr: "unexpected arguments"},
		{name: "no secret flag", args: []string{"-secret", "x"}, wantErr: "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAccountsFlags(tt.args, &bytes.Buffer{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("options = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestProvision(t *testing.T) {
	accounts := testAccounts(t, "bot", 4)
	refused := &botclient.LoginError{Packet: "LoginFail", Reason: 2}
	tests := []struct {
		name      string
		fail      map[string]error // check error per account
		paceErr   error
		wantDone  int
		wantCalls []string
		wantErr   string
	}{
		{name: "all ready", wantDone: 4, wantCalls: []string{"bot1", "bot2", "bot3", "bot4"}},
		{
			name:      "credential refused stops every later login",
			fail:      map[string]error{"bot2": refused},
			wantDone:  1,
			wantCalls: []string{"bot1", "bot2"},
			wantErr:   "account bot2: login server replied LoginFail (reason 2) (an existing account",
		},
		{
			name:      "server down stops",
			fail:      map[string]error{"bot1": errors.New("dial login server: connection refused")},
			wantCalls: []string{"bot1"},
			wantErr:   "account bot1: dial login server",
		},
		{
			name:      "canceled while pacing",
			paceErr:   context.Canceled,
			wantDone:  1,
			wantCalls: []string{"bot1"},
			wantErr:   "context canceled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			paces := 0
			check := func(_ context.Context, account, password string) error {
				for _, a := range accounts {
					if a.Name == account && a.Password() != password {
						t.Errorf("account %s checked with another password", account)
					}
				}
				calls = append(calls, account)
				return tt.fail[account]
			}
			pace := func(context.Context) error {
				// Pacing comes between logins only.
				if paces != len(calls)-1 {
					t.Errorf("pace %d after %d logins", paces, len(calls))
				}
				paces++
				return tt.paceErr
			}
			var out bytes.Buffer
			done, err := provision(t.Context(), accounts, check, pace, &out)
			if done != tt.wantDone {
				t.Errorf("done = %d, want %d", done, tt.wantDone)
			}
			if strings.Join(calls, ",") != strings.Join(tt.wantCalls, ",") {
				t.Errorf("logins %v, want %v", calls, tt.wantCalls)
			}
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			text := out.String()
			if err != nil {
				text += err.Error()
			}
			for _, a := range accounts {
				if strings.Contains(text, a.Password()) {
					t.Fatalf("output %q carries %s's password", text, a.Name)
				}
			}
		})
	}
}

func TestRunAccountsRefusesBeforeLogin(t *testing.T) {
	dir := t.TempDir()
	weak := filepath.Join(dir, "weak.secret")
	if err := os.WriteFile(weak, []byte("changeme"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"persistent, default secret", []string{"-persistent", "-secret-file", weak, "-prefix", "bot", "-bots", "2"}, 1, "persistent mode"},
		{"persistent, no secret file", []string{"-persistent", "-prefix", "bot", "-bots", "2"}, 1, "no secret file"},
		{"persistent, missing secret file", []string{"-persistent", "-secret-file", filepath.Join(dir, "absent"), "-prefix", "bot", "-bots", "2"}, 1, "no such file"},
		{"bad flag", []string{"-interval", "1ms"}, 2, "-interval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			// The login address is unroutable: a refusal must come before
			// any dial.
			args := append([]string{"-login", "192.0.2.1:1", "-timeout", "1ms"}, tt.args...)
			if code := run(t.Context(), append([]string{"accounts"}, args...), &stdout, &stderr); code != tt.code {
				t.Fatalf("exit %d, want %d; stderr %q", code, tt.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr %q, want %q", stderr.String(), tt.want)
			}
			if strings.Contains(stderr.String()+stdout.String(), "changeme") {
				t.Fatal("output quotes the secret")
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout %q: no account may be checked", stdout.String())
			}
		})
	}
}
