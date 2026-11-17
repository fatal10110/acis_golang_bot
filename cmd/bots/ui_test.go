package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botpanel"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

func TestParseUIFlags(t *testing.T) {
	base := []string{"-secret-file", "s"}
	npc := botnames.Marker{Text: "Npc", Placement: botnames.Prefix}
	tests := []struct {
		name    string
		args    []string
		check   func(uiOptions) bool
		wantErr string
	}{
		{"defaults", base, func(o uiOptions) bool {
			return o.dataDir == "" && o.prefix == "" && o.marker == botnames.DefaultMarker && o.maxOnline == 10 &&
				o.importPrefix == "" && o.importCount == 0 && o.loginAddr == "127.0.0.1:2106" && o.serverID == 1 && o.listen == defaultListen
		}, ""},
		{
			"panel prefix and marker", append(base, "-data-dir", "d", "-prefix", "home", "-name-marker", "Npc", "-name-marker-at", "prefix", "-max-online", "40"),
			func(o uiOptions) bool {
				return o.dataDir == "d" && o.prefix == "home" && o.marker == npc && o.maxOnline == 40
			}, "",
		},
		{
			"import", append(base, "-import-prefix", "hz", "-import", "20", "-import-marker", "Npc", "-import-marker-at", "prefix"),
			func(o uiOptions) bool {
				return o.importPrefix == "hz" && o.importCount == 20 && o.importMarker == npc
			}, "",
		},
		{"no secret", []string{"-prefix", "home"}, nil, "-secret-file"},
		{"marker without prefix", append(base, "-name-marker", "Npc"), nil, "go with -prefix"},
		{"bad prefix", append(base, "-prefix", "Home"), nil, "-prefix"},
		{"bad marker", append(base, "-prefix", "home", "-name-marker", "B-t"), nil, "-name-marker"},
		{"bad placement", append(base, "-name-marker-at", "middle"), nil, "-name-marker-at"},
		{"no online cap", append(base, "-max-online", "0"), nil, "-max-online"},
		{"import count without prefix", append(base, "-import", "3"), nil, "go together"},
		{"import prefix without count", append(base, "-import-prefix", "hz"), nil, "go together"},
		{"import marker without import", append(base, "-import-marker", "Npc"), nil, "go with -import-prefix"},
		{"bad import marker", append(base, "-import-prefix", "hz", "-import", "2", "-import-marker", "B-t"), nil, "-import-marker"},
		{"bad import placement", append(base, "-import-marker-at", "middle"), nil, "-import-marker-at"},
		{"bad import prefix", append(base, "-import-prefix", "h-z", "-import", "2"), nil, "-import-prefix"},
		{"server id 0", append(base, "-server-id", "0"), nil, "-server-id"},
		{"panel address", append(base, "-listen", "[::1]:9000"), func(o uiOptions) bool { return o.listen == "[::1]:9000" }, ""},
		{"panel on every interface", append(base, "-listen", "0.0.0.0:8090"), nil, "-listen"},
		{"panel on a host name", append(base, "-listen", "localhost:8090"), nil, "-listen"},
		{"stray argument", append(base, "extra"), nil, "unexpected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseUIFlags(tt.args, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !tt.check(o) {
				t.Fatalf("options %+v", o)
			}
		})
	}
}

func TestPrepareRegistry(t *testing.T) {
	dir := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	open := func() *botregistry.Registry {
		reg, err := botregistry.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		return reg
	}
	reg := open()
	if err := prepareRegistry(reg, uiOptions{}, log); err == nil || !strings.Contains(err.Error(), "-prefix") {
		t.Fatalf("new registry without -prefix: %v", err)
	}
	o := uiOptions{prefix: "home", marker: botnames.DefaultMarker, importPrefix: "home", importCount: 3, importMarker: botnames.DefaultMarker}
	if err := prepareRegistry(reg, o, log); err != nil {
		t.Fatal(err)
	}
	if b, err := reg.NewBot(botregistry.Bot{}); err != nil || b.Account != "home4" {
		t.Fatalf("new bot after importing the panel's own prefix: %+v, %v", b, err)
	}
	_ = reg.Close()

	reg = open()
	defer reg.Close()
	if err := prepareRegistry(reg, uiOptions{}, log); err != nil {
		t.Fatalf("restart without -prefix: %v", err)
	}
	if err := prepareRegistry(reg, uiOptions{prefix: "other", marker: botnames.DefaultMarker}, log); err == nil {
		t.Fatal("another panel prefix accepted")
	}
}

func TestUIRunnerRefusesWhatItCannotRun(t *testing.T) {
	u := uiRunner{cfg: botclient.SessionConfig{Clock: botclient.SystemClock{}}, shared: shared{claims: botclient.NewPartyClaims()}}
	tests := []struct {
		name    string
		run     botregistry.Run
		wantErr string
	}{
		{"unknown preset", botregistry.Run{Account: "hz1", Preset: "bogus", Character: botclient.Character{Name: "AldenBot"}}, "-persona"},
		{"preset needs the datapack", botregistry.Run{Account: "hz1", Preset: "town", Character: botclient.Character{Name: "AldenBot"}}, "-datapack"},
		{"bad character", botregistry.Run{Account: "hz1", Preset: "support", Character: botclient.Character{Name: "A-b"}}, "character name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := u.run(t.Context(), tt.run)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunUIRefusesALockedDataDir(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	args := []string{"-secret-file", secret, "-data-dir", dir, "-prefix", "home"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the panel stops at once: no bot is online

	if code := runUI(ctx, args, io.Discard); code != 0 {
		t.Fatalf("first run: exit %d", code)
	}
	reg, err := botregistry.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	var stderr bytes.Buffer
	if code := runUI(ctx, args, &stderr); code != 1 || !strings.Contains(stderr.String(), "in use by another process") {
		t.Fatalf("second panel on a held data dir: exit %d, %s", code, stderr.String())
	}
}

func TestOfferedPresets(t *testing.T) {
	// names lists the offered presets, and the disabled ones as name:reason.
	names := func(ps []botpanel.Preset) (out []string) {
		for _, p := range ps {
			if p.Disabled != "" {
				out = append(out, p.Name+":"+p.Disabled)
				continue
			}
			out = append(out, p.Name)
		}
		return out
	}
	if got := names(offeredPresets("")); !slices.Equal(got, []string{"support", "town:needs -datapack", "merchant:needs -datapack"}) {
		t.Fatalf("without a datapack offered %q, want support only, the others disabled", got)
	}
	if got := names(offeredPresets("dp")); !slices.Equal(got, []string{"support", "town", "merchant"}) {
		t.Fatalf("with a datapack offered %q", got)
	}
}
