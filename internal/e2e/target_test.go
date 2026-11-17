package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// fakeNet is a fake target: it records every credential it is sent and
// answers with the errors set, and no game server listens.
type fakeNet struct {
	loginErr, checkErr error

	mu   sync.Mutex
	sent []string // account/password of every login and check
}

var errNoGame = errors.New("no game server")

func (n *fakeNet) record(account, password string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, account+"/"+password)
}

func (n *fakeNet) logins() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.sent)
}

func (n *fakeNet) login(_ context.Context, account, password string) (botclient.LoginResult, error) {
	n.record(account, password)
	return botclient.LoginResult{GameAddr: "game:7777"}, n.loginErr
}

func (n *fakeNet) checkCredentials(_ context.Context, account, password string) error {
	n.record(account, password)
	return n.checkErr
}

func (n *fakeNet) dialGame(context.Context, string) (*botclient.GameConn, error) {
	return nil, errNoGame
}

// parkClock never moves: a gate that must wait parks on a timer that never
// fires, and every timer made signals parked.
type parkClock struct {
	now    time.Time
	parked chan struct{}
}

func newParkClock() *parkClock {
	return &parkClock{now: time.Unix(1000, 0), parked: make(chan struct{}, 8)}
}

func (c *parkClock) Now() time.Time { return c.now }

func (c *parkClock) NewTimer(time.Duration) botclient.Timer {
	c.parked <- struct{}{}
	return idleTimer{}
}

type idleTimer struct{}

func (idleTimer) C() <-chan time.Time { return nil }
func (idleTimer) Reset(time.Duration) {}
func (idleTimer) Stop()               {}

func testSecret(t *testing.T) botaccounts.Secret {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("k3Y-for-e2e-unit-tests-only-0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := botaccounts.LoadSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fakeTarget(t *testing.T, cfg Config, n *fakeNet) (*Target, *parkClock) {
	t.Helper()
	cfg.Prefix = DefaultPrefix
	clock := newParkClock()
	return newTarget(cfg, testSecret(t), clock, n), clock
}

func TestTargetCap(t *testing.T) {
	tg, _ := fakeTarget(t, Config{Allowlisted: true}, &fakeNet{})
	var releases []func()
	for i := range MaxOnline {
		r, err := tg.Hold(t.Context())
		if err != nil {
			t.Fatalf("place %d: %v", i+1, err)
		}
		releases = append(releases, r)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := tg.Hold(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("place %d = %v, want it to wait until ctx ends", MaxOnline+1, err)
	}
	got := make(chan error, 1)
	go func() {
		r, err := tg.Hold(t.Context())
		if err == nil {
			r()
		}
		got <- err
	}()
	releases[0]()
	releases[0]() // a second release gives nothing back
	if err := <-got; err != nil {
		t.Fatalf("waiting place = %v", err)
	}
	if n := len(tg.slots); n != MaxOnline-1 {
		t.Fatalf("%d places taken, want %d", n, MaxOnline-1)
	}
}

func TestTargetEnterGivesThePlaceBack(t *testing.T) {
	n := &fakeNet{}
	tg, _ := fakeTarget(t, Config{Allowlisted: true}, n)
	account, password := tg.Account("a", 1)
	if account != "e2ea1" || password != tg.secret.Password("e2ea1") {
		t.Fatalf("Account = %q, want e2ea1 with its derived password", account)
	}
	ch := botclient.Character{Name: "e2eBot"}
	for range MaxOnline + 1 {
		if _, err := tg.Enter(t.Context(), account, password, ch, nil); !errors.Is(err, errNoGame) {
			t.Fatalf("Enter = %v, want %v", err, errNoGame)
		}
	}
	if len(tg.slots) != 0 {
		t.Fatalf("%d places kept by failed entries", len(tg.slots))
	}
}

func TestTargetBreaker(t *testing.T) {
	refused := &botclient.LoginError{Packet: "LoginFail", Reason: 0x02}
	tests := []struct {
		name  string
		first func(*Target) error
		n     *fakeNet
	}{
		{"refused login", func(tg *Target) error {
			_, err := tg.Enter(context.Background(), "e2ea1", "pw", botclient.Character{Name: "e2eBot"}, nil)
			return err
		}, &fakeNet{loginErr: refused}},
		{"refused check", func(tg *Target) error { return tg.CheckCredentials(context.Background(), "e2ea1", "pw") }, &fakeNet{checkErr: refused}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg, _ := fakeTarget(t, Config{Allowlisted: true}, tt.n)
			if err := tt.first(tg); !errors.As(err, new(*botclient.LoginError)) {
				t.Fatalf("first login = %v, want the LoginError", err)
			}
			if !errors.Is(tg.Halted(), botclient.ErrLoginsHalted) {
				t.Fatalf("breaker after a refused credential: %v", tg.Halted())
			}
			if _, err := tg.Enter(t.Context(), "e2ea2", "pw", botclient.Character{Name: "e2eBot"}, nil); !errors.Is(err, botclient.ErrLoginsHalted) {
				t.Fatalf("login after the breaker = %v", err)
			}
			if err := tg.CheckCredentials(t.Context(), "e2ea2", "pw"); !errors.Is(err, botclient.ErrLoginsHalted) {
				t.Fatalf("check after the breaker = %v", err)
			}
			if got := tt.n.logins(); len(got) != 1 {
				t.Fatalf("credentials sent %d times, want once", len(got))
			}
		})
	}
}

func TestTargetWrongPassword(t *testing.T) {
	refused := &botclient.LoginError{Packet: "LoginFail", Reason: 0x02}
	n := &fakeNet{checkErr: refused}
	tg, _ := fakeTarget(t, Config{Allowlisted: true}, n)
	if err := tg.WrongPassword(t.Context(), "e2ea1"); err == nil || len(n.logins()) != 0 {
		t.Fatalf("wrong password without %s = %v, %d sent", EnvAllowLoginFail, err, len(n.logins()))
	}

	tg, _ = fakeTarget(t, Config{Allowlisted: true, AllowLoginFail: true}, n)
	if err := tg.WrongPassword(t.Context(), "e2ea1"); !errors.Is(err, refused) {
		t.Fatalf("wrong password = %v, want the refusal", err)
	}
	sent := n.logins()
	if len(sent) != 1 || sent[0] == "e2ea1/"+tg.secret.Password("e2ea1") {
		t.Fatalf("sent %d credentials (the right password: %v), want one wrong", len(sent), len(sent) == 1)
	}
	if !errors.Is(tg.Halted(), botclient.ErrLoginsHalted) {
		t.Fatal("the wrong password did not trip the breaker")
	}
	if err := tg.WrongPassword(t.Context(), "e2ea1"); !errors.Is(err, ErrLoginFailUsed) {
		t.Fatalf("second wrong password = %v, want ErrLoginFailUsed", err)
	}
	if len(n.logins()) != 1 {
		t.Fatal("the second wrong password went out")
	}
}

func TestTargetPacing(t *testing.T) {
	tests := []struct {
		name        string
		allowlisted bool
		paced       bool
	}{
		{"spaced by default", false, true},
		{"allowlisted", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &fakeNet{}
			tg, clock := fakeTarget(t, Config{Allowlisted: tt.allowlisted}, n)
			if err := tg.CheckCredentials(t.Context(), "e2ea1", "pw"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tg.CheckCredentials(ctx, "e2ea2", "pw") }()
			if !tt.paced {
				if err := <-done; err != nil {
					t.Fatalf("second login on an allowlisted gate = %v", err)
				}
				if len(clock.parked) != 0 {
					t.Fatal("an allowlisted gate waited")
				}
				return
			}
			<-clock.parked // the second dial waits out the spacing
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("second login = %v, want it held until ctx ended", err)
			}
			if len(n.logins()) != 1 {
				t.Fatal("the second login went out inside the spacing")
			}
		})
	}
}
