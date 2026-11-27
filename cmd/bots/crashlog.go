package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// crashFileName is the runtime crash log's name in the data directory.
const crashFileName = "crash.log"

// crashFileCap bounds the crash log: a start that finds it past this size
// moves it to crash.log.1 (the one older generation kept) and starts a new
// one. The runtime writes to it only once per run, when it dies, so the
// file holds at most the cap plus one crash.
const crashFileCap = 1 << 20

// crashFileUsage is swarm's -crash-file help text.
const crashFileUsage = "file that keeps the Go runtime's crash output (a fatal error or an unrecovered panic) in addition to stderr; default: crash.log in acis-bots under the user's config directory"

// defaultDataDir is acis-bots under the user's config directory: where
// `bots ui` keeps its registry without -data-dir, and where the crash log
// goes without -data-dir or -crash-file.
func defaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, botregistry.DefaultDirName), nil
}

// crashFilePath is the crash log of a run: file when set, else crash.log in
// the default data directory.
func crashFilePath(file string) (string, error) {
	if file != "" {
		return file, nil
	}
	dir, err := defaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, crashFileName), nil
}

// startCrashLog points the runtime's crash output at file (crashFilePath:
// empty is the default one) through openCrashLog, for command cmd. Any
// failure is a warning on stderr: the run goes on and its crashes reach
// stderr only. The returned stop turns the crash log off; a run defers it
// so it covers the whole run.
func startCrashLog(file, cmd string, stderr io.Writer) (stop func()) {
	path, err := crashFilePath(file)
	if err == nil {
		stop, err = openCrashLog(path, cmd, time.Now(), stderr)
	}
	if err != nil {
		fmt.Fprintf(stderr, "bots %s: crash log: %v (crashes reach stderr only)\n", cmd, err)
		return func() {}
	}
	return stop
}

// openCrashLog rotates path (rotateCrashLog), opens it for append with mode
// 0600 (a goroutine dump can hold strings) in a directory it creates 0700,
// writes a start header stamped now, and hands the file to
// debug.SetCrashOutput: a fatal error or an unrecovered panic is then
// written to it as well as to stderr. A rotation failure is a warning on
// stderr. The returned stop turns the crash output off and releases the
// runtime's handle.
func openCrashLog(path, cmd string, now time.Time, stderr io.Writer) (stop func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := rotateCrashLog(path, crashFileCap); err != nil {
		fmt.Fprintf(stderr, "bots %s: crash log: %v\n", cmd, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	// The runtime keeps a duplicate of the handle, so f closes here.
	defer f.Close()
	if _, err := io.WriteString(f, crashHeader(cmd, now)); err != nil {
		return nil, err
	}
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		return nil, err
	}
	return func() { _ = debug.SetCrashOutput(nil, debug.CrashOptions{}) }, nil
}

// rotateCrashLog moves path to path.1, replacing it, when path is larger
// than limit. When the move fails (on Windows another bots process that
// holds the file open blocks it) path is truncated instead, so the file
// stays bounded; the returned error says which happened. A missing or
// small file is left as it is.
func rotateCrashLog(path string, limit int64) error {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Size() <= limit {
		return nil
	}
	renameErr := os.Rename(path, path+".1")
	if renameErr == nil {
		return nil
	}
	if err := os.Truncate(path, 0); err != nil {
		return fmt.Errorf("rotate: %w; truncate: %w", renameErr, err)
	}
	return fmt.Errorf("rotate: %w; truncated it instead", renameErr)
}

// crashHeader is the line a start writes to the crash log: the time, the
// command, the process and the build. It holds no flag value: a secret
// never reaches the file through it.
func crashHeader(cmd string, now time.Time) string {
	rev, modified := "unknown", false
	version := ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			version = " " + v
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
	}
	if modified {
		rev += "+modified"
	}
	return fmt.Sprintf("\n=== %s bots %s start: pid %d, build%s %s, %s %s/%s ===\n",
		now.Format(time.RFC3339), cmd, os.Getpid(), version, rev, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
