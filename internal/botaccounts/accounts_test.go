package botaccounts

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// genSecret returns a fresh random secret: tests never carry a fixed one,
// except the published RFC 4231 vector below.
func genSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// writeSecret writes content to a temp file of mode perm and returns its path.
func writeSecret(t *testing.T, content string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bots.secret")
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	// WriteFile's mode passes through the umask; set it exactly.
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPasswordVector(t *testing.T) {
	// RFC 4231 test case 2: HMAC-SHA256("Jefe", "what do ya want for
	// nothing?") = 5bdcc146bf60754e6a04..., whose first 10 bytes in base32
	// are the password.
	s := Secret{key: []byte("Jefe")}
	if got, want := s.Password("what do ya want for nothing?"), "LPOMCRV7MB2U42QE"; got != want {
		t.Fatalf("Password = %q, want %q", got, want)
	}
}

func TestPassword(t *testing.T) {
	a := Secret{key: []byte(genSecret(t))}
	b := Secret{key: []byte(genSecret(t))}
	tests := []struct {
		name string
		x, y string
		same bool
	}{
		{"deterministic", a.Password("bot1"), a.Password("bot1"), true},
		{"server lowercases the account", a.Password("Bot1"), a.Password("bot1"), true},
		{"differs per account", a.Password("bot1"), a.Password("bot2"), false},
		{"differs per secret", a.Password("bot1"), b.Password("bot1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if (tt.x == tt.y) != tt.same {
				t.Fatalf("%q vs %q: same = %v, want %v", tt.x, tt.y, tt.x == tt.y, tt.same)
			}
		})
	}
	for _, account := range Names("bot", 200) {
		p := a.Password(account)
		if len(p) != PasswordLen {
			t.Fatalf("Password(%q) has %d bytes, want %d", account, len(p), PasswordLen)
		}
		for _, c := range []byte(p) {
			if c <= 0x20 || c > 0x7e {
				t.Fatalf("Password(%q) = %q carries byte %#x", account, p, c)
			}
		}
	}
}

func TestLoadSecret(t *testing.T) {
	good := genSecret(t)
	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr string
		is      error
	}{
		{name: "0600", path: func(t *testing.T) string { return writeSecret(t, good, 0o600) }},
		{name: "0400", path: func(t *testing.T) string { return writeSecret(t, good, 0o400) }},
		{name: "trailing newline", path: func(t *testing.T) string { return writeSecret(t, good+"\n", 0o600) }},
		{name: "missing", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent") }, is: fs.ErrNotExist},
		{name: "directory", path: func(t *testing.T) string { return t.TempDir() }, wantErr: "not a regular file"},
		{name: "empty", path: func(t *testing.T) string { return writeSecret(t, "", 0o600) }, wantErr: "empty"},
		{name: "whitespace only", path: func(t *testing.T) string { return writeSecret(t, " \n\t\n", 0o600) }, wantErr: "empty"},
		{name: "too large", path: func(t *testing.T) string { return writeSecret(t, strings.Repeat(good, 200), 0o600) }, wantErr: "longer than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path(t)
			s, err := LoadSecret(path)
			switch {
			case tt.is != nil:
				if !errors.Is(err, tt.is) {
					t.Fatalf("err = %v, want %v", err, tt.is)
				}
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatal(err)
			default:
				if string(s.key) != good {
					t.Fatal("loaded secret differs from the file's")
				}
			}
			if err != nil && strings.Contains(err.Error(), good) {
				t.Fatal("error quotes the secret")
			}
		})
	}
}

func TestStrong(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		ok     bool
	}{
		{"generated", genSecret(t), true},
		{"16 bytes", genSecret(t)[:16], true},
		{"15 bytes", genSecret(t)[:15], false},
		{"repeated character", strings.Repeat("x", 32), false},
		{"changeme", "changemechangeme", false},
		{"placeholder word, any case", "My-SECRET-for-the-bots-2026", false},
		{"password", genSecret(t) + "password", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Secret{key: []byte(tt.secret)}.Strong()
			if tt.ok != (err == nil) {
				t.Fatalf("Strong() = %v, want ok %v", err, tt.ok)
			}
			if err != nil && !errors.Is(err, ErrWeakSecret) {
				t.Fatalf("err = %v, want ErrWeakSecret", err)
			}
		})
	}
}

func TestValidatePrefix(t *testing.T) {
	tests := []struct {
		prefix string
		count  int
		ok     bool
	}{
		{"bot", 1, true},
		{"swarm2a", 500, true},
		{"abcdefghijk", 999, true},   // 11 + 3 = 14
		{"abcdefghijk", 1000, false}, // 11 + 4 = 15
		{"abcdefghijklmn", 1, false},
		{"", 1, false},
		{"bot1", 1, false}, // ends in a digit: bot1+1 = bot+11
		{"Bot", 1, false},  // the server lowercases account names
		{"bot_", 1, false},
		{"bot", 0, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%d", tt.prefix, tt.count), func(t *testing.T) {
			err := ValidatePrefix(tt.prefix, tt.count)
			if tt.ok != (err == nil) {
				t.Fatalf("ValidatePrefix = %v, want ok %v", err, tt.ok)
			}
		})
	}
}

func TestNamesDisjointAcrossPrefixes(t *testing.T) {
	// Prefixes where one extends another, the case a digit-ending prefix
	// would break.
	prefixes := []string{"a", "ab", "b", "bot", "bota", "swarma", "swarmb"}
	owner := make(map[string]string)
	for _, p := range prefixes {
		if err := ValidatePrefix(p, 120); err != nil {
			t.Fatal(err)
		}
		names := Names(p, 120)
		if names[0] != p+"1" || names[119] != p+"120" {
			t.Fatalf("Names(%q) = %q ... %q", p, names[0], names[119])
		}
		for _, n := range names {
			if prev, ok := owner[n]; ok {
				t.Fatalf("account %q belongs to both %q and %q", n, prev, p)
			}
			owner[n] = p
		}
	}
}

func TestConfigAccounts(t *testing.T) {
	strong := genSecret(t)
	tests := []struct {
		name    string
		cfg     func(t *testing.T) Config
		wantErr string
		is      error
	}{
		{
			name: "persistent, strong secret",
			cfg: func(t *testing.T) Config {
				return Config{SecretFile: writeSecret(t, strong, 0o600), Prefix: "bot", Count: 3, Persistent: true}
			},
		},
		{
			name: "non-persistent takes a short secret",
			cfg: func(t *testing.T) Config {
				return Config{SecretFile: writeSecret(t, "changeme", 0o600), Prefix: "bot", Count: 3}
			},
		},
		{
			name: "persistent refuses a default secret",
			cfg: func(t *testing.T) Config {
				return Config{SecretFile: writeSecret(t, "changeme", 0o600), Prefix: "bot", Count: 3, Persistent: true}
			},
			is: ErrWeakSecret,
		},
		{
			name: "persistent refuses no secret file",
			cfg: func(*testing.T) Config {
				return Config{Prefix: "bot", Count: 3, Persistent: true}
			},
			wantErr: "no secret file",
		},
		{
			name: "persistent refuses a missing secret file",
			cfg: func(t *testing.T) Config {
				return Config{SecretFile: filepath.Join(t.TempDir(), "absent"), Prefix: "bot", Count: 3, Persistent: true}
			},
			is: fs.ErrNotExist,
		},
		{
			name: "non-persistent refuses no secret file",
			cfg: func(*testing.T) Config {
				return Config{Prefix: "bot", Count: 3}
			},
			wantErr: "no secret file",
		},
		{
			name: "loose mode",
			cfg: func(t *testing.T) Config {
				return Config{SecretFile: writeSecret(t, strong, 0o644), Prefix: "bot", Count: 3, Persistent: true}
			},
			is: ErrSecretMode,
		},
		{
			name: "bad prefix",
			cfg: func(t *testing.T) Config {
				return Config{SecretFile: writeSecret(t, strong, 0o600), Prefix: "Bot", Count: 3}
			},
			wantErr: "account prefix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg(t)
			accounts, err := cfg.Accounts()
			switch {
			case tt.is != nil:
				if !errors.Is(err, tt.is) {
					t.Fatalf("err = %v, want %v", err, tt.is)
				}
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatal(err)
			default:
				if len(accounts) != cfg.Count {
					t.Fatalf("%d accounts, want %d", len(accounts), cfg.Count)
				}
				for i, a := range accounts {
					if want := fmt.Sprintf("bot%d", i+1); a.Name != want {
						t.Fatalf("account %d = %q, want %q", i, a.Name, want)
					}
					if len(a.Password()) != PasswordLen {
						t.Fatalf("account %q: password of %d bytes", a.Name, len(a.Password()))
					}
				}
			}
			if err != nil {
				for _, s := range []string{strong, "changeme"} {
					if strings.Contains(err.Error(), s) {
						t.Fatalf("error %q quotes the secret", err)
					}
				}
			}
		})
	}
}

func TestFormatRedacts(t *testing.T) {
	key := genSecret(t)
	s := Secret{key: []byte(key)}
	a := Account{Name: "bot1", password: s.Password("bot1")}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X"} {
		for _, v := range []any{s, &s, a, []Account{a}, struct{ A Account }{a}} {
			out := fmt.Sprintf(verb, v)
			for _, leak := range []string{key, a.Password(), hex.EncodeToString([]byte(key)), hex.EncodeToString([]byte(a.Password()))} {
				if strings.Contains(strings.ToLower(out), strings.ToLower(leak)) {
					t.Fatalf("Sprintf(%s, %T) = %q leaks a secret", verb, v, out)
				}
			}
		}
	}
	if got := fmt.Sprint(a); got != "bot1" {
		t.Fatalf("Sprint(account) = %q, want the name", got)
	}
}
