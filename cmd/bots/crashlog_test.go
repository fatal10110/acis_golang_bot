package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

func TestCrashFilePath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base) // Linux and the BSDs
	t.Setenv("AppData", base)         // Windows
	t.Setenv("HOME", base)            // macOS: $HOME/Library/Application Support
	def := filepath.Join(base, "acis-bots", "crash.log")
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		def = filepath.Join(base, "Library", "Application Support", "acis-bots", "crash.log")
	}
	explicit := filepath.Join(base, "elsewhere", "swarm-crash.log")
	tests := []struct {
		name, file, want string
	}{
		{"default data directory", "", def},
		{"-crash-file", explicit, explicit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := crashFilePath(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("path %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenCrashLog(t *testing.T) {
	now := time.Date(2026, 11, 27, 9, 30, 0, 0, time.UTC)
	header := crashHeader("ui", now)
	old := []byte("=== an earlier start ===\nfatal error: out of memory\n")
	big := bytes.Repeat([]byte("goroutine 7 [running]:\n"), crashFileCap/23+1)
	tests := []struct {
		name string
		// setup prepares the directory dir that holds path.
		setup func(t *testing.T, dir, path string)
		// want is crash.log afterwards; wantOld is crash.log.1 (nil: absent).
		want, wantOld []byte
		wantWarn      string
	}{
		{"first start", func(*testing.T, string, string) {}, []byte(header), nil, ""},
		{"small file appended to", func(t *testing.T, _, path string) {
			writeCrashTestFile(t, path, old)
		}, append(append([]byte{}, old...), header...), nil, ""},
		{"file past the cap rotated", func(t *testing.T, _, path string) {
			writeCrashTestFile(t, path, big)
			writeCrashTestFile(t, path+".1", old) // the older generation is replaced
		}, []byte(header), big, ""},
		// The move fails (here crash.log.1 is a directory; on Windows another
		// bots process holding the file): it is truncated instead.
		{"rotation refused", func(t *testing.T, _, path string) {
			writeCrashTestFile(t, path, big)
			writeCrashTestFile(t, filepath.Join(path+".1", "keep"), old)
		}, []byte(header), nil, "truncated it instead"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "acis-bots")
			path := filepath.Join(dir, crashFileName)
			tt.setup(t, dir, path)
			var stderr bytes.Buffer
			stop, err := openCrashLog(path, "ui", now, &stderr)
			if err != nil {
				t.Fatal(err)
			}
			stop() // releases the runtime's handle: Windows removes the dir
			if got := readCrashTestFile(t, path); !bytes.Equal(got, tt.want) {
				t.Fatalf("crash.log %d bytes, want %d", len(got), len(tt.want))
			}
			if tt.wantOld != nil && !bytes.Equal(readCrashTestFile(t, path+".1"), tt.wantOld) {
				t.Fatal("crash.log.1 is not the rotated file")
			}
			if tt.wantOld == nil && tt.wantWarn == "" {
				if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
					t.Fatalf("crash.log.1 exists: %v", err)
				}
			}
			if !strings.Contains(stderr.String(), tt.wantWarn) || (tt.wantWarn == "") != (stderr.Len() == 0) {
				t.Fatalf("stderr %q, want %q", stderr.String(), tt.wantWarn)
			}
			if runtime.GOOS != "windows" {
				checkCrashTestMode(t, dir, 0o700)
				checkCrashTestMode(t, path, 0o600)
			}
		})
	}
}

func TestCrashHeader(t *testing.T) {
	now := time.Date(2026, 11, 27, 9, 30, 0, 0, time.FixedZone("CET", 3600))
	h := crashHeader("swarm", now)
	for _, want := range []string{"2026-11-27T09:30:00+01:00", "bots swarm start", runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH, "build "} {
		if !strings.Contains(h, want) {
			t.Errorf("header %q lacks %q", h, want)
		}
	}
	if strings.Count(h, "\n") != 2 || !strings.HasPrefix(h, "\n") {
		t.Errorf("header %q is not one line after a blank one", h)
	}
}

func TestStartCrashLogDegrades(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	writeCrashTestFile(t, file, nil)
	var stderr bytes.Buffer
	stop := startCrashLog(filepath.Join(file, "crash.log"), "swarm", &stderr)
	stop()
	if !strings.Contains(stderr.String(), "bots swarm: crash log:") || !strings.Contains(stderr.String(), "crashes reach stderr only") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

// TestRunCrashLog runs both commands to a stop: each writes its start header
// to its crash log, and a second panel refused on a held data directory
// leaves the running one's log alone.
func TestRunCrashLog(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	secret := filepath.Join(t.TempDir(), "secret")
	writeCrashTestFile(t, secret, []byte(hex.EncodeToString(key)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the panel stops at once: no bot is online

	dir := t.TempDir()
	args := []string{"-secret-file", secret, "-data-dir", dir, "-prefix", "home"}
	if code := runUI(ctx, args, io.Discard); code != 0 {
		t.Fatalf("panel: exit %d", code)
	}
	reg, err := botregistry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	if code := runUI(ctx, args, io.Discard); code != 1 {
		t.Fatalf("second panel: exit %d", code)
	}
	if got := strings.Count(string(readCrashTestFile(t, filepath.Join(dir, crashFileName))), "bots ui start"); got != 1 {
		t.Fatalf("%d ui headers, want 1", got)
	}

	// No secret file: the swarm fails after the crash log started.
	file := filepath.Join(t.TempDir(), "swarm-crash.log")
	if code := runSwarm(ctx, []string{"-prefix", "bot", "-bots", "1", "-crash-file", file}, io.Discard); code != 1 {
		t.Fatalf("swarm: exit %d", code)
	}
	if !strings.Contains(string(readCrashTestFile(t, file)), "bots swarm start") {
		t.Fatal("no swarm header")
	}
}

// crashHelperEnv carries "<how>:<crash log>" to TestCrashLogKeepsARealCrash's
// child, which sets up the crash log and then dies.
const crashHelperEnv = "BOTS_CRASH_HELPER"

// TestCrashLogKeepsARealCrash re-runs the test binary as a child that sets
// up the crash log and then dies, and reads what reached stderr and the
// file. Everything the runtime prints once it is dying reaches both: an
// unrecovered panic from its first line, and every goroutine trace, the
// faulting one first. The "fatal error: <message>" line of a runtime throw
// is printed before the runtime marks itself dying (runtime.throw and
// runtime.fatal, then fatalthrow), so it reaches stderr only; the file
// still holds the faulting goroutine, whose frames name the throw site.
func TestCrashLogKeepsARealCrash(t *testing.T) {
	if how, path, ok := strings.Cut(os.Getenv(crashHelperEnv), ":"); ok {
		if _, err := openCrashLog(path, "ui", time.Now(), os.Stderr); err != nil {
			t.Fatal(err)
		}
		if how == "fatal" {
			dieOfAFatalError()
		}
		done := make(chan struct{})
		go panicUnrecovered() // off the test goroutine, whose runner recovers
		<-done
		return
	}
	tests := []struct {
		how string
		// both is in stderr and the file; stderrOnly in stderr.
		both       []string
		stderrOnly string
	}{
		{"panic", []string{"panic: a deliberate crash", "goroutine ", "panicUnrecovered"}, ""},
		{"fatal", []string{"goroutine ", "dieOfAFatalError"}, "fatal error: sync: unlock of unlocked mutex"},
	}
	for _, tt := range tests {
		t.Run(tt.how, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), crashFileName)
			cmd := exec.Command(os.Args[0], "-test.run=^TestCrashLogKeepsARealCrash$")
			cmd.Env = append(os.Environ(), crashHelperEnv+"="+tt.how+":"+path)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err == nil {
				t.Fatal("the child survived its crash")
			}
			file := string(readCrashTestFile(t, path))
			if !strings.HasPrefix(file, "\n=== ") {
				t.Error("crash.log does not start with its header")
			}
			for _, want := range tt.both {
				if !strings.Contains(stderr.String(), want) || !strings.Contains(file, want) {
					t.Errorf("stderr or crash.log lacks %q\nstderr:\n%s\ncrash.log:\n%s", want, stderr.String(), file)
				}
			}
			if !strings.Contains(stderr.String(), tt.stderrOnly) {
				t.Errorf("stderr lacks %q:\n%s", tt.stderrOnly, stderr.String())
			}
		})
	}
}

// dieOfAFatalError throws a runtime fatal error, which no recover stops.
func dieOfAFatalError() {
	var mu sync.Mutex
	mu.Unlock()
}

func panicUnrecovered() {
	panic("a deliberate crash")
}

func writeCrashTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readCrashTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func checkCrashTestMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s mode %o, want %o", path, got, want)
	}
}
