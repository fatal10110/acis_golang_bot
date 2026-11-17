package botregistry

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

var (
	t0        = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	npcMarker = botnames.Marker{Text: "Npc", Placement: botnames.Prefix}
)

// openPanel opens a registry in dir with panel prefix hz and the default
// marker; the test closes it.
func openPanel(t *testing.T, dir string) *Registry {
	t.Helper()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if err := r.SetPanel("hz", botnames.DefaultMarker); err != nil {
		t.Fatal(err)
	}
	return r
}

func accounts(bots []Bot) []string {
	out := make([]string, len(bots))
	for i, b := range bots {
		out[i] = b.Account
	}
	return out
}

func TestRegistryNewBotNeverReusesAnAccount(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	for _, want := range []string{"hz1", "hz2"} {
		b, err := r.NewBot(Bot{Preset: "town", Online: true})
		if err != nil || b.Account != want || b.Prefix != "hz" {
			t.Fatalf("NewBot = %+v, %v; want account %s", b, err, want)
		}
	}
	if err := r.Drop("hz2", "deleted", t0); err != nil {
		t.Fatal(err)
	}
	if b, err := r.NewBot(Bot{Preset: "town"}); err != nil || b.Account != "hz3" {
		t.Fatalf("NewBot after a drop = %+v, %v; want hz3", b, err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	r2 := openPanel(t, dir)
	if got := accounts(r2.Bots()); !slices.Equal(got, []string{"hz1", "hz3"}) {
		t.Fatalf("bots after reopen = %v", got)
	}
	if ts := r2.Tombstones(); len(ts) != 1 || ts[0].Account != "hz2" || ts[0].Reason != "deleted" || !ts[0].At.Equal(t0) {
		t.Fatalf("tombstones after reopen = %+v", ts)
	}
	if b, err := r2.NewBot(Bot{}); err != nil || b.Account != "hz4" {
		t.Fatalf("NewBot after reopen = %+v, %v; want hz4", b, err)
	}
}

func TestRegistryImport(t *testing.T) {
	tests := []struct {
		name string
		// held are the panel's bots before the import.
		held    int
		prefix  string
		marker  botnames.Marker
		n       int
		want    []string
		wantNew string
		wantErr string
	}{
		{name: "panel prefix moves the counter past n", prefix: "hz", marker: botnames.DefaultMarker, n: 3, want: []string{"hz1", "hz2", "hz3"}, wantNew: "hz4"},
		{name: "held accounts are skipped", held: 2, prefix: "hz", marker: botnames.DefaultMarker, n: 3, want: []string{"hz3"}, wantNew: "hz4"},
		{name: "counter never goes back", held: 5, prefix: "hz", marker: botnames.DefaultMarker, n: 2, want: nil, wantNew: "hz6"},
		{name: "other prefix with its marker", prefix: "old", marker: npcMarker, n: 2, want: []string{"old1", "old2"}, wantNew: "hz1"},
		{name: "panel prefix with another marker", prefix: "hz", marker: npcMarker, n: 2, wantErr: "marker"},
		{name: "bad prefix", prefix: "Hz", marker: botnames.DefaultMarker, n: 2, wantErr: "prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := openPanel(t, t.TempDir())
			for range tt.held {
				if _, err := r.NewBot(Bot{}); err != nil {
					t.Fatal(err)
				}
			}
			added, err := r.Import(tt.prefix, tt.marker, tt.n, ImportPreset)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Import err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := accounts(added); !slices.Equal(got, tt.want) {
				t.Fatalf("imported %v, want %v", got, tt.want)
			}
			for _, b := range added {
				if !b.NeverCreate || b.Preset != ImportPreset || b.Online || b.Prefix != tt.prefix {
					t.Fatalf("imported bot %+v: want never-create, %s, offline", b, ImportPreset)
				}
			}
			if m, ok := r.Marker(tt.prefix); !ok || m != tt.marker {
				t.Fatalf("marker of %s = %+v, %v", tt.prefix, m, ok)
			}
			if !slices.Contains(r.Markers(), tt.marker) || !slices.Contains(r.Markers(), botnames.DefaultMarker) {
				t.Fatalf("markers %v miss the panel's or the import's", r.Markers())
			}
			if b, err := r.NewBot(Bot{}); err != nil || b.Account != tt.wantNew {
				t.Fatalf("NewBot after import = %+v, %v; want %s", b, err, tt.wantNew)
			}
		})
	}
}

func TestRegistrySetPanel(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	if err := r.SetPanel("hz", botnames.DefaultMarker); err != nil {
		t.Fatalf("same panel again: %v", err)
	}
	if err := r.SetPanel("ab", botnames.DefaultMarker); err == nil {
		t.Fatal("another prefix accepted")
	}
	if err := r.SetPanel("hz", npcMarker); err == nil {
		t.Fatal("another marker accepted")
	}
	if p, ok := r.Panel(); !ok || p.Name != "hz" || p.Marker != botnames.DefaultMarker {
		t.Fatalf("Panel = %+v, %v", p, ok)
	}
}

func TestRegistryFileHoldsNoSecretAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	class := int32(18)
	b, err := r.NewBot(Bot{Class: &class, Preset: "merchant", Online: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Update(b.Account, func(b *Bot) {
		b.Name, b.Level, b.Position = "AldenBot", 12, &Position{X: 1, Y: -2, Z: 3}
		b.Account, b.Prefix = "changed", "changed"
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "password") {
		t.Fatalf("registry file mentions a password:\n%s", raw)
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	got := f.Bots[0]
	if got.Account != "hz1" || got.Prefix != "hz" || got.Name != "AldenBot" || *got.Class != 18 || got.Level != 12 || *got.Position != (Position{1, -2, 3}) {
		t.Fatalf("saved bot = %+v", got)
	}
	if f.Prefixes[0].Marker != botnames.DefaultMarker || !strings.Contains(string(raw), `"placement": "suffix"`) {
		t.Fatalf("saved prefix = %+v", f.Prefixes[0])
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("data dir holds %d entries, want bots.json and bots.lock (no temp file left)", len(entries))
	}
}

func TestRegistryRefusesABadFile(t *testing.T) {
	tests := []struct {
		name, content, wantErr string
	}{
		{"unknown version", `{"version": 2, "prefixes": [], "bots": []}`, "version 2"},
		{"no version", `{"prefixes": [], "bots": []}`, "version 0"},
		{"unknown prefix", `{"version": 1, "prefixes": [], "bots": [{"account": "hz1", "prefix": "hz"}]}`, "not listed"},
		{"bad placement", `{"version": 1, "prefixes": [{"name": "hz", "marker": {"text": "Bot", "placement": "middle"}}]}`, "placement"},
		{"account twice", `{"version": 1, "prefixes": [{"name": "hz", "marker": {"text": "Bot", "placement": "suffix"}}], "bots": [{"account": "hz1", "prefix": "hz"}], "tombstones": [{"account": "hz1"}]}`, "twice"},
		{"not json", `{`, "unexpected end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, FileName), []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			r, err := Open(dir)
			if err == nil {
				_ = r.Close()
				t.Fatal("Open accepted the file")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Open err = %v, want %q", err, tt.wantErr)
			}
			// A refused file releases the lock.
			r, err = Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			_ = r.Close()
		})
	}
}

func TestRegistryLockRefusesASecondOpen(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Open err = %v, want ErrLocked", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = Open(dir)
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	_ = r.Close()
}

// lockHelperEnv makes the test binary a helper process that opens the
// registry of the directory it names, reports it, and holds it until killed.
const lockHelperEnv = "BOTREGISTRY_LOCK_HELPER_DIR"

func TestRegistryLockOfAKilledProcessDoesNotBlock(t *testing.T) {
	if dir := os.Getenv(lockHelperEnv); dir != "" {
		if _, err := Open(dir); err != nil {
			os.Exit(1)
		}
		_, _ = os.Stdout.WriteString("locked\n")
		_, _ = io.Copy(io.Discard, os.Stdin) // blocks until killed
		os.Exit(0)
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRegistryLockOfAKilledProcessDoesNotBlock$")
	cmd.Env = append(os.Environ(), lockHelperEnv+"="+dir)
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
	if err != nil || line != "locked\n" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("helper said %q, %v", line, err)
	}
	if _, err := Open(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("Open while the helper holds the lock: %v, want ErrLocked", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("Open after the holder was killed: %v", err)
	}
	_ = r.Close()
}
