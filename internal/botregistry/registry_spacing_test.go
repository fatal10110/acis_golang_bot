package botregistry

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// fileSpacing reads the login spacing bots.json in dir holds, in ms.
func fileSpacing(t *testing.T, dir string) int64 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f.LoginSpacing
}

func TestRegistryLoginSpacing(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	if got := r.LoginSpacing(); got != 0 {
		t.Fatalf("new registry spacing %s, want 0", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, FileName)); strings.Contains(string(b), "login_spacing_ms") {
		t.Fatalf("unset spacing written: %s", b)
	}
	for _, d := range []time.Duration{-time.Millisecond, botclient.MaxLoginSpacing + time.Millisecond} {
		if err := r.SetLoginSpacing(d); err == nil {
			t.Fatalf("SetLoginSpacing(%s) accepted", d)
		}
	}
	writes := r.writes
	if err := r.SetLoginSpacing(2500*time.Millisecond + 300*time.Microsecond); err != nil {
		t.Fatal(err)
	}
	if err := r.SetLoginSpacing(2500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if r.writes != writes+1 {
		t.Fatalf("%d writes, want 1: an unchanged spacing is not saved again", r.writes-writes)
	}
	if got := fileSpacing(t, dir); got != 2500 {
		t.Fatalf("file holds %d ms, want 2500 (whole milliseconds)", got)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if got := r.LoginSpacing(); got != 2500*time.Millisecond {
		t.Fatalf("reopened spacing %s, want 2.5s", got)
	}
}

func TestRegistryRefusesABadLoginSpacing(t *testing.T) {
	for _, ms := range []string{"-1", "60001"} {
		dir := t.TempDir()
		content := `{"version": 1, "login_spacing_ms": ` + ms + `, "prefixes": [], "bots": []}`
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		r, err := Open(dir)
		if err == nil {
			_ = r.Close()
			t.Fatalf("Open accepted login spacing %s ms", ms)
		}
		if !strings.Contains(err.Error(), "login spacing") {
			t.Fatalf("Open err = %v, want the login spacing named", err)
		}
	}
}

func TestSupervisorSetLoginSpacing(t *testing.T) {
	dir := t.TempDir()
	reg := openPanel(t, dir)
	h := startSupervisor(t, reg, newClock(), 1)
	ctx := t.Context()
	if err := h.sup.SetLoginSpacing(ctx, 4*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := h.gate.Spacing(); got != 4*time.Second {
		t.Fatalf("gate spacing %s, want 4s", got)
	}
	if got := fileSpacing(t, dir); got != 4000 {
		t.Fatalf("file holds %d ms, want 4000", got)
	}
	// Under the spaced gate's floor or over the cap: refused, nothing kept.
	for _, d := range []time.Duration{time.Second, 0, botclient.MaxLoginSpacing + time.Second} {
		if err := h.sup.SetLoginSpacing(ctx, d); !errors.Is(err, botclient.ErrSpacing) {
			t.Fatalf("SetLoginSpacing(%s) = %v, want ErrSpacing", d, err)
		}
	}
	if got, ms := h.gate.Spacing(), fileSpacing(t, dir); got != 4*time.Second || ms != 4000 {
		t.Fatalf("after refusals gate %s, file %d ms, want 4s kept", got, ms)
	}
}

func TestSupervisorAppliesKeptLoginSpacing(t *testing.T) {
	floor := botclient.NewLoginGate(botclient.SystemClock{}, nil).MinSpacing()
	tests := []struct {
		name        string
		allowlisted bool
		kept        time.Duration
		want        time.Duration
	}{
		{"none kept, spaced", false, 0, floor},
		{"none kept, allowlisted", true, 0, 0},
		{"raised, spaced", false, 7 * time.Second, 7 * time.Second},
		{"raised, allowlisted", true, 300 * time.Millisecond, 300 * time.Millisecond},
		// Kept under an allowlisted gate, under the spaced floor: the
		// spaced gate keeps its floor.
		{"under the floor, spaced", false, 300 * time.Millisecond, floor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := openPanel(t, t.TempDir())
			if err := reg.SetLoginSpacing(tt.kept); err != nil {
				t.Fatal(err)
			}
			clock := newClock()
			gate := botclient.NewLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
			if tt.allowlisted {
				gate = botclient.NewAllowlistedLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
			}
			_, err := New(Config{
				Registry:  reg,
				Run:       func(context.Context, Run) error { return nil },
				Gate:      gate,
				Clock:     clock,
				MaxOnline: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := gate.Spacing(); got != tt.want {
				t.Fatalf("gate spacing %s, want %s", got, tt.want)
			}
		})
	}
}
