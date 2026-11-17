// Package botaccounts names the bots' accounts and derives their passwords
// (docs/plan.md, "Sessions, reconnect, accounts and host"). Each swarm
// instance owns the accounts prefix1..prefixN, so two swarms with different
// prefixes never log into each other's accounts. Each account's password is
// HMAC-SHA256(secret, account) from a secret file of mode 0600, so no
// password is stored and one leaked password opens one account only.
//
// The secret and the passwords are never formatted: Secret and Account
// redact themselves under every fmt verb, and no error here quotes them.
package botaccounts

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/ownerfile"
)

// MaxAccountLen is the longest account name: the login credential block
// holds 14 bytes of name (server loginserver/network/clientpackets
// requestauthlogin.go; botclient's authNameMax).
const MaxAccountLen = 14

// PasswordLen is a derived password's length: the credential block holds 16
// bytes of password, and 16 base32 characters carry 80 bits of the HMAC.
const PasswordLen = 16

// MinSecretLen is the shortest secret persistent mode accepts.
const MinSecretLen = 16

// maxSecretFileSize caps how much of the secret file is read.
const maxSecretFileSize = 4096

var (
	// ErrSecretMode reports a secret file others than its owner may access:
	// a group or other mode bit on unix, a wider DACL on Windows. It is
	// ownerfile.ErrShared.
	ErrSecretMode = ownerfile.ErrShared
	// ErrWeakSecret reports a secret persistent mode refuses: too short, one
	// repeated character, or a placeholder.
	ErrWeakSecret = errors.New("secret is too weak or a placeholder")
)

// placeholders are words a hand-written or shipped default secret carries; a
// generated secret does not. Matched case-insensitively anywhere in it.
var placeholders = [...]string{"changeme", "change-me", "change_me", "replaceme", "secret", "password", "default", "example", "placeholder"}

// validPrefix is an account prefix: lowercase letters and digits, ending in a
// letter. The server lowercases account names (requestauthlogin.go), so an
// uppercase prefix would share accounts with its lowercase twin; ending in a
// letter keeps the index digits apart, so "bot" and "bot1" never both own
// "bot11".
var validPrefix = regexp.MustCompile(`^[a-z0-9]*[a-z]$`)

// passwordEncoding spells a password in printable ASCII with no padding:
// no NUL, nothing the server trims (it trims bytes <= 0x20 at both ends).
var passwordEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Secret is the key the passwords derive from. Its zero value has no key.
type Secret struct{ key []byte }

// LoadSecret reads the secret from path: a regular file private to its
// owner (ownerfile.Check: mode 0600 or stricter, on Windows an owner-only
// DACL) holding at most 4 KiB. Surrounding whitespace, such as the
// trailing newline of `echo`, is not part of the secret. An empty secret is
// refused; persistent mode also wants Strong.
func LoadSecret(path string) (Secret, error) {
	f, err := os.Open(path)
	if err != nil {
		return Secret{}, fmt.Errorf("secret file: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Secret{}, fmt.Errorf("secret file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return Secret{}, fmt.Errorf("secret file %s: not a regular file", path)
	}
	if err := ownerfile.Check(f); err != nil {
		return Secret{}, fmt.Errorf("secret file %s: %w", path, err)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSecretFileSize+1))
	if err != nil {
		return Secret{}, fmt.Errorf("secret file %s: %w", path, err)
	}
	if len(b) > maxSecretFileSize {
		return Secret{}, fmt.Errorf("secret file %s: longer than %d bytes", path, maxSecretFileSize)
	}
	key := bytes.TrimSpace(b)
	if len(key) == 0 {
		return Secret{}, fmt.Errorf("secret file %s: empty", path)
	}
	return Secret{key: bytes.Clone(key)}, nil
}

// Strong reports a secret persistent mode refuses: shorter than
// MinSecretLen, one character repeated, or carrying a placeholder word.
func (s Secret) Strong() error {
	if len(s.key) < MinSecretLen {
		return fmt.Errorf("%w: shorter than %d bytes", ErrWeakSecret, MinSecretLen)
	}
	if bytes.Count(s.key, s.key[:1]) == len(s.key) {
		return fmt.Errorf("%w: one character repeated", ErrWeakSecret)
	}
	lower := strings.ToLower(string(s.key))
	for _, p := range placeholders {
		if strings.Contains(lower, p) {
			return fmt.Errorf("%w: looks like a placeholder", ErrWeakSecret)
		}
	}
	return nil
}

// Password derives account's password: HMAC-SHA256 keyed with the secret
// over the account name as the server stores it (lowercased), its first 10
// bytes in unpadded standard base32 (A-Z, 2-7): PasswordLen characters.
func (s Secret) Password(account string) string {
	m := hmac.New(sha256.New, s.key)
	_, _ = m.Write([]byte(strings.ToLower(account)))
	return passwordEncoding.EncodeToString(m.Sum(nil)[:10])
}

// Format redacts the secret under every verb.
func (Secret) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, "[redacted]") }

// Account is one bot account and its derived password.
type Account struct {
	Name     string
	password string
}

// Password is the account's derived password. Never log it.
func (a Account) Password() string { return a.password }

// Format prints the account name only, under every verb.
func (a Account) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, a.Name) }

// ValidatePrefix reports a prefix that is not lowercase letters and digits
// ending in a letter, or that leaves no room for count's index within
// MaxAccountLen.
func ValidatePrefix(prefix string, count int) error {
	if count < 1 {
		return fmt.Errorf("bot count %d: want at least 1", count)
	}
	if !validPrefix.MatchString(prefix) {
		return fmt.Errorf("account prefix %q: want lowercase letters and digits ending in a letter", prefix)
	}
	if n := len(prefix) + len(strconv.Itoa(count)); n > MaxAccountLen {
		return fmt.Errorf("account prefix %q with %d bots: account names reach %d characters, the server takes %d", prefix, count, n, MaxAccountLen)
	}
	return nil
}

// Names returns the accounts of one swarm instance: prefix1 to prefixN.
// The prefix must pass ValidatePrefix.
func Names(prefix string, count int) []string {
	names := make([]string, count)
	for i := range names {
		names[i] = prefix + strconv.Itoa(i+1)
	}
	return names
}

// Config selects one swarm instance's accounts.
type Config struct {
	// SecretFile is the path of the secret file; never the secret itself.
	SecretFile string
	// Prefix starts every account name of this swarm instance.
	Prefix string
	// Count is how many bot accounts the instance runs.
	Count int
	// Persistent marks a long-running swarm that keeps its bots in the
	// world; it refuses a weak or placeholder secret.
	Persistent bool
}

// Accounts validates c, loads the secret and returns the instance's
// accounts with their passwords. A missing secret file is refused in every
// mode; persistent mode also refuses a secret that is not Strong.
func (c Config) Accounts() ([]Account, error) {
	if err := ValidatePrefix(c.Prefix, c.Count); err != nil {
		return nil, err
	}
	if c.SecretFile == "" {
		return nil, errors.New("no secret file: the per-bot passwords derive from it")
	}
	secret, err := LoadSecret(c.SecretFile)
	if err != nil {
		return nil, err
	}
	if c.Persistent {
		if err := secret.Strong(); err != nil {
			return nil, fmt.Errorf("persistent mode: secret file %s: %w", c.SecretFile, err)
		}
	}
	names := Names(c.Prefix, c.Count)
	accounts := make([]Account, len(names))
	for i, name := range names {
		accounts[i] = Account{Name: name, password: secret.Password(name)}
	}
	return accounts, nil
}
