package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// recordMints is a minter that records the accounts it mints.
func recordMints(minted *[]string) minter {
	return func(_ context.Context, account string) error {
		*minted = append(*minted, account)
		return nil
	}
}

func TestMintPoolMintsWhenExhaustedAndReuses(t *testing.T) {
	dir := t.TempDir()
	var minted []string
	p := newMintPool(dir, "e2e", newParkClock())
	first, err := p.take(t.Context(), recordMints(&minted))
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.take(t.Context(), recordMints(&minted))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"e2egm1", "e2egm2"}; !slices.Equal(minted, want) || first.minted != want[0] || second.minted != want[1] {
		t.Fatalf("minted %q, leased %s and %s, want %q", minted, first.minted, second.minted, want)
	}
	names, err := p.mint.names()
	if err != nil || !slices.Equal(names, minted) {
		t.Fatalf("record = %q, %v, want %q", names, err, minted)
	}
	fi, err := os.Stat(p.mint.file())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("record mode = %v, %v, want 0600", fi, err)
	}
	gm, err := second.load(testSecret(t))
	if err != nil {
		t.Fatal(err)
	}
	if gm.Account != "e2egm2" || gm.Character != "e2egm2Bot" || gm.Password() != testSecret(t).Password("e2egm2") {
		t.Fatalf("minted GM = %v, want e2egm2/e2egm2Bot with its derived password", gm)
	}
	first.release()
	second.release()

	// The next run reuses what this one minted, and mints only past it.
	minted = nil
	next := newMintPool(dir, "e2e", newParkClock())
	reused, err := next.take(t.Context(), recordMints(&minted))
	if err != nil {
		t.Fatal(err)
	}
	defer reused.release()
	if reused.minted != "e2egm1" || len(minted) != 0 {
		t.Fatalf("next run leased %s after minting %q, want e2egm1 with no mint", reused.minted, minted)
	}
	other, err := next.take(t.Context(), recordMints(&minted))
	if err != nil {
		t.Fatal(err)
	}
	defer other.release()
	third, err := next.take(t.Context(), recordMints(&minted))
	if err != nil {
		t.Fatal(err)
	}
	defer third.release()
	if other.minted != "e2egm2" || third.minted != "e2egm3" || !slices.Equal(minted, []string{"e2egm3"}) {
		t.Fatalf("next run leased %s and %s after minting %q, want e2egm2, then e2egm3 minted", other.minted, third.minted, minted)
	}
}

// TestMintPoolOneMintAtATime has a second pool on the same record, as a
// second test process has, take while the first one mints: it mints
// nothing until the first is done, then mints the next N.
func TestMintPoolOneMintAtATime(t *testing.T) {
	dir := t.TempDir()
	clock := &stepClock{now: time.Unix(1000, 0)}
	other := newMintPool(dir, "e2e", clock)
	var otherMinted []string
	var during error
	p := newMintPool(dir, "e2e", newParkClock())
	held, err := p.take(t.Context(), func(context.Context, string) error {
		_, during = other.take(t.Context(), recordMints(&otherMinted))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	if during == nil || !strings.Contains(during.Error(), "GM pool exhausted") || len(otherMinted) != 0 {
		t.Fatalf("take while another mints = %v after minting %q, want GM pool exhausted, no mint", during, otherMinted)
	}
	l, err := other.take(t.Context(), recordMints(&otherMinted))
	if err != nil {
		t.Fatal(err)
	}
	defer l.release()
	if held.minted != "e2egm1" || l.minted != "e2egm2" || !slices.Equal(otherMinted, []string{"e2egm2"}) {
		t.Fatalf("minted %s, then %s (%q), want e2egm1, then e2egm2", held.minted, l.minted, otherMinted)
	}
}

func TestMintPoolFailedMint(t *testing.T) {
	dir := t.TempDir()
	p := newMintPool(dir, "e2e", newParkClock())
	refused := errors.New("refused")
	if _, err := p.take(t.Context(), func(context.Context, string) error { return refused }); !errors.Is(err, refused) {
		t.Fatalf("take with a failed mint = %v, want %v", err, refused)
	}
	if names, err := p.mint.names(); err != nil || len(names) != 0 {
		t.Fatalf("record after a failed mint = %q, %v, want none", names, err)
	}
	// The account is minted again, its lock given back.
	var minted []string
	l, err := p.take(t.Context(), recordMints(&minted))
	if err != nil {
		t.Fatal(err)
	}
	defer l.release()
	if l.minted != "e2egm1" {
		t.Fatalf("leased %s after a failed mint, want e2egm1 minted again", l.minted)
	}
}

func TestMintRecord(t *testing.T) {
	tests := []struct {
		name    string
		content string
		mode    os.FileMode
		want    []string
		next    string
		err     bool
	}{
		{name: "names and comments", content: "# minted\ne2egm1\n\ne2egm3\n", mode: 0o600, want: []string{"e2egm1", "e2egm3"}, next: "e2egm4"},
		{name: "shared", content: "e2egm1\n", mode: 0o644, err: true},
		{name: "another prefix", content: "e2xgm1\n", mode: 0o600, err: true},
		{name: "leading zero", content: "e2egm01\n", mode: 0o600, err: true},
		{name: "no n", content: "e2egm\n", mode: 0o600, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mintRecord{dir: t.TempDir(), prefix: "e2e"}
			if err := os.WriteFile(r.file(), []byte(tt.content), tt.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(r.file(), tt.mode); err != nil {
				t.Fatal(err)
			}
			got, err := r.names()
			if (err != nil) != tt.err || !slices.Equal(got, tt.want) {
				t.Fatalf("names = %q, %v, want %q, error %v", got, err, tt.want, tt.err)
			}
			if tt.err {
				return
			}
			if next, err := r.next(got); err != nil || next != tt.next {
				t.Fatalf("next = %s, %v, want %s", next, err, tt.next)
			}
		})
	}
	if _, err := (mintRecord{dir: t.TempDir(), prefix: "abcdefghijkl"}).next(nil); err == nil {
		t.Error("next for a prefix too long for its accounts succeeded")
	}
	r := mintRecord{dir: filepath.Join(t.TempDir(), "none"), prefix: "e2e"}
	if names, err := r.names(); err != nil || names != nil {
		t.Errorf("names without a record = %q, %v", names, err)
	}
}

func TestScopeCheckRootGM(t *testing.T) {
	root := newScope()
	root.mint = "e2egm"
	root.addChar("e2eBot", 101)
	root.target = 101
	scenario := newScope()
	scenario.addChar("e2egm1Bot", 101)
	scenario.target = 101
	tests := []struct {
		scope *Scope
		cmd   string
		ok    bool
	}{
		{root, "set access e2egm1Bot 7", true},
		{root, "set access e2egm12Bot 7", true},
		{root, "set access e2egm1Bot 8", false},
		{root, "set access e2egm1Bot -1", false},
		{root, "set access e2egm1Bot 0", false},
		{root, "set access e2egm1 7", false},
		{root, "set access e2egm01Bot 7", false},
		{root, "set access e2egmBot 7", false},
		{root, "set access e2exgm1Bot 7", false},
		{root, "set access e2eBot 7", false},
		{root, "set access 7", false},
		{root, "set access e2egm1Bot 7 x", false},
		{root, "set level 5", false},
		{root, "teleport 1 2 3", false},
		{root, "kick e2eBot", false},
		{scenario, "set access e2egm1Bot 7", false},
		{scenario, "set access 7", false},
	}
	for _, tt := range tests {
		err := tt.scope.check(tt.cmd)
		if (err == nil) != tt.ok || err != nil && !errors.Is(err, ErrCommandRefused) {
			t.Errorf("check(%q) from mint %q = %v, want allowed %v", tt.cmd, tt.scope.mint, err, tt.ok)
		}
	}
}

func TestEnterGMMintThatFailsKeepsNothing(t *testing.T) {
	files := gmDir(t, "gm")
	dir := filepath.Dir(files[0])
	tg, _ := fakeTarget(t, Config{Allowlisted: true, Prefix: "e2e", GMFiles: files, GMMint: dir}, &fakeNet{})
	if tg.roots == nil || tg.gms.mint == nil {
		t.Fatal("a run with GMMint set does not mint")
	}
	if _, err := tg.EnterGM(t.Context()); !errors.Is(err, errNoGame) {
		t.Fatalf("EnterGM = %v, want the minted account's entry to fail with %v", err, errNoGame)
	}
	if names, err := tg.gms.mint.names(); err != nil || len(names) != 0 {
		t.Fatalf("record after a failed mint = %q, %v", names, err)
	}
	root, err := tg.roots.take(t.Context(), nil)
	if err != nil {
		t.Fatalf("root kept by a failed mint: %v", err)
	}
	root.release()
}
