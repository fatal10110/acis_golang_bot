package e2e

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// gmDir writes GM files of names into a new directory, mode 0600, and
// returns their paths.
func gmDir(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var files []string
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("account=e2e"+name+"\npassword=pw\ncharacter=e2e"+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	return files
}

func TestPoolGMFiles(t *testing.T) {
	files := gmDir(t, "gm", "gm2", "gm10", "gm.lock", "gm2.lock", "gmx", "gm-old", "secret", "halted")
	dir := filepath.Dir(files[0])
	if err := os.Mkdir(filepath.Join(dir, "gm3"), 0o700); err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "gm"), filepath.Join(dir, "gm10"), filepath.Join(dir, "gm2")}
	if got := gmFiles(dir); !slices.Equal(got, want) {
		t.Fatalf("gmFiles = %q, want %q", got, want)
	}
	if got := gmFiles(filepath.Join(dir, "none")); got != nil {
		t.Fatalf("gmFiles of no dir = %q", got)
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"the dir's pool", nil, want},
		{"one pinned", map[string]string{EnvGMFile: "/g"}, []string{"/g"}},
		{"a list pinned", map[string]string{EnvGMFile: "/g" + string(filepath.ListSeparator) + "/h"}, []string{"/g", "/h"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfigFromEnv(env(tt.env), dir).GMFiles; !slices.Equal(got, tt.want) {
				t.Fatalf("GMFiles = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPoolLeasesDifferentGMsAndWaits(t *testing.T) {
	files := gmDir(t, "gm", "gm2")
	clock := newParkClock()
	p := newGMPool(files, clock)
	first, err := p.take(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.take(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.file != files[0] || second.file != files[1] {
		t.Fatalf("leases %s and %s, want %s and %s", first.file, second.file, files[0], files[1])
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.take(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("take with the pool exhausted = %v, want it to wait until ctx ends", err)
	}
	<-clock.parked
	got := make(chan *gmLease, 1)
	go func() {
		l, err := p.take(t.Context(), nil)
		if err != nil {
			t.Error(err)
		}
		got <- l
	}()
	<-clock.parked // the waiter found every GM held
	first.release()
	first.release() // a second release gives nothing back
	third := <-got
	if third == nil || third.file != files[0] {
		t.Fatalf("waiter got %v, want %s", third, files[0])
	}
	third.release()
	second.release()
}

func TestPoolNoGM(t *testing.T) {
	if _, err := newGMPool(nil, newParkClock()).take(t.Context(), nil); err == nil {
		t.Fatal("take from an empty pool succeeded")
	}
}

// gmLockHelperEnv makes the test binary a helper process that leases a GM
// of the pool of the files it names, reports which, and holds it until
// killed.
const gmLockHelperEnv = "E2E_GM_LOCK_HELPER_FILES"

func TestPoolGMLockOfAnotherProcess(t *testing.T) {
	if list := os.Getenv(gmLockHelperEnv); list != "" {
		l, err := newGMPool(filepath.SplitList(list), botclient.SystemClock{}).take(context.Background(), nil)
		if err != nil {
			os.Exit(1)
		}
		_, _ = os.Stdout.WriteString(filepath.Base(l.file) + "\n")
		_, _ = io.Copy(io.Discard, os.Stdin) // blocks until killed
		os.Exit(0)
	}
	files := gmDir(t, "gm", "gm2")
	cmd := exec.Command(os.Args[0], "-test.run=^TestPoolGMLockOfAnotherProcess$")
	cmd.Env = append(os.Environ(), gmLockHelperEnv+"="+files[0]+string(filepath.ListSeparator)+files[1])
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "gm\n" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("helper said %q, %v", line, err)
	}
	p := newGMPool(files, newParkClock())
	mine, err := p.take(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if mine.file != files[1] {
		t.Fatalf("lease while the helper holds gm = %s, want %s", mine.file, files[1])
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.take(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("take with both GMs held = %v, want it to wait", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	freed, err := p.take(t.Context(), nil)
	if err != nil {
		t.Fatalf("take after the holder was killed: %v", err)
	}
	if freed.file != files[0] {
		t.Fatalf("lease after the kill = %s, want %s", freed.file, files[0])
	}
	freed.release()
	mine.release()
}

func TestPoolEnterGMGivesTheGMBack(t *testing.T) {
	files := gmDir(t, "gm")
	tg, _ := fakeTarget(t, Config{Allowlisted: true, GMFiles: files}, &fakeNet{})
	for range 2 {
		if _, err := tg.EnterGM(t.Context()); !errors.Is(err, errNoGame) {
			t.Fatalf("EnterGM = %v, want %v", err, errNoGame)
		}
	}
	l, err := tg.gms.take(t.Context(), nil)
	if err != nil {
		t.Fatalf("GM kept by a failed entry: %v", err)
	}
	l.release()
}

func TestPoolScenarios(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret")
	tg, _ := fakeTarget(t, Config{Allowlisted: true, SecretFile: secret}, &fakeNet{})
	var done []func()
	var open []*Target
	for i := range MaxScenarios {
		s, d, err := tg.Scenario(t.Context())
		if err != nil {
			t.Fatalf("scenario %d: %v", i+1, err)
		}
		open, done = append(open, s), append(done, d)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := tg.Scenario(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("scenario %d = %v, want it to wait until ctx ends", MaxScenarios+1, err)
	}
	// Each scenario has its own cap and GM command scope on the shared run.
	open[0].Own("e2eBot", 7)
	if open[1].scope.isChar("e2eBot") || open[1].scope.owns(7) {
		t.Fatal("a scenario's character is in another scenario's scope")
	}
	if open[0].slots == open[1].slots || open[0].run != open[1].run {
		t.Fatal("scenarios share a cap, or not the run")
	}
	done[0]()
	done[0]() // a second done gives nothing back
	_, d, err := tg.Scenario(t.Context())
	if err != nil {
		t.Fatalf("scenario after one closed: %v", err)
	}
	d()
	if err := os.WriteFile(tg.cfg.HaltFile(), []byte("e2ea1: refused\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tg.Scenario(t.Context()); !errors.Is(err, botclient.ErrLoginsHalted) {
		t.Fatalf("scenario with the breaker's marker = %v, want ErrLoginsHalted", err)
	}
	done[1]()
}
