package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestOutcomeLogFlag checks -outcome-log on swarm and ui: off by default.
func TestOutcomeLogFlag(t *testing.T) {
	swarm := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	ui := []string{"-secret-file", "s"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"off by default", nil, ""},
		{"file", []string{"-outcome-log", "stays.jsonl"}, "stays.jsonl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			so, err := parseSwarmFlags(append(swarm, tt.args...), io.Discard)
			if err != nil || so.outcomeLog != tt.want {
				t.Errorf("swarm: outcome log, err = %q, %v; want %q, nil", so.outcomeLog, err, tt.want)
			}
			uo, err := parseUIFlags(append(ui, tt.args...), io.Discard)
			if err != nil || uo.outcomeLog != tt.want {
				t.Errorf("ui: outcome log, err = %q, %v; want %q, nil", uo.outcomeLog, err, tt.want)
			}
		})
	}
}

// TestOutcomeLogOff checks no -outcome-log starts nothing and gives the
// brains no queue.
func TestOutcomeLogOff(t *testing.T) {
	q, stop, err := startOutcomeLog("", slog.New(slog.DiscardHandler))
	if err != nil || q != nil {
		t.Fatalf("startOutcomeLog(\"\") = %v, %v; want nil queue", q, err)
	}
	stop()
	for _, p := range presets {
		if cfg := p.brain(shared{outcomes: q}); cfg.Outcomes != nil {
			t.Errorf("%s: Outcomes set with no log", p.name)
		}
	}
}

// TestOutcomeLogPresets checks only the farming presets get the queue.
func TestOutcomeLogPresets(t *testing.T) {
	q := botclient.NewOutcomeQueue()
	for _, p := range presets {
		cfg := p.brain(shared{outcomes: q})
		if got := cfg.Outcomes != nil; got != p.farms {
			t.Errorf("%s: Outcomes set %v, want %v", p.name, got, p.farms)
		}
	}
}

// TestOutcomeLogWriter checks the writer writes every queued record as one
// JSON line, those still queued at stop included, and is joined by stop.
func TestOutcomeLogWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stays.jsonl")
	q, stop, err := startOutcomeLog(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		q.Put(botclient.Outcome{Spot: "s", Level: int32(i + 1)})
	}
	stop()
	lines := readOutcomeLines(t, path)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	for i, o := range lines {
		if o.Spot != "s" || o.Level != int32(i+1) {
			t.Errorf("line %d = %+v, want spot s level %d", i, o, i+1)
		}
	}
	// Windows keeps no POSIX mode bits: os.Stat reports 0666 there.
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if m := st.Mode().Perm(); m != 0o600 {
			t.Errorf("mode %v; want 0600", m)
		}
	}
}

// TestOutcomeLogRotate checks after two rotations exactly two files exist,
// neither past the limit, and every line in them is a whole record.
func TestOutcomeLogRotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stays.jsonl")
	rec := botclient.Outcome{Spot: "s"}
	raw, _ := json.Marshal(rec)
	line := int64(len(raw) + 1)
	limit := 3*line + line/2
	l, err := openOutcomeFile(path, limit)
	if err != nil {
		t.Fatal(err)
	}
	// Three lines a file: the 4th rotates once, the 7th twice.
	for i := range 8 {
		rec.Level = int32(i)
		if err := l.write(rec); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("files = %d, want 2", len(entries))
	}
	old, cur := readOutcomeLines(t, path+".1"), readOutcomeLines(t, path)
	if len(old) != 3 || old[0].Level != 3 || len(cur) != 2 || cur[0].Level != 6 {
		t.Errorf("old %+v, current %+v; want levels 3-5 and 6-7", old, cur)
	}
	for _, p := range []string{path, path + ".1"} {
		if st, err := os.Stat(p); err != nil || st.Size() > limit {
			t.Errorf("%s: size %d, %v; want at most %d", p, st.Size(), err, limit)
		}
	}
}

// readOutcomeLines reads the records of an outcome log, failing on a line
// that is not a whole record.
func readOutcomeLines(t *testing.T, path string) []botclient.Outcome {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []botclient.Outcome
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var o botclient.Outcome
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			t.Fatalf("%s: line %q: %v", path, sc.Text(), err)
		}
		out = append(out, o)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
