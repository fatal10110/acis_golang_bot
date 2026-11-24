package botregistry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestRegistryBranchRoundTrip checks a pinned branch survives a reopen, a
// bot with none writes no branch field, and a record stored before the pin
// existed (no branch field, the support preset) reopens unchanged (#331).
func TestRegistryBranchRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	pinned, err := r.NewBot(Bot{Preset: "farmer"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := r.NewBot(Bot{Preset: "support"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Update(pinned.Account, func(b *Bot) { b.Branch = 7 }); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), `"branch"`); n != 1 {
		t.Fatalf("registry file holds %d branch fields, want the pinned bot's only:\n%s", n, raw)
	}
	r = openPanel(t, dir)
	if b, ok := r.Bot(pinned.Account); !ok || b.Branch != 7 || b.Preset != "farmer" {
		t.Fatalf("pinned bot after reopen %+v", b)
	}
	if b, ok := r.Bot(old.Account); !ok || b.Branch != 0 || b.Preset != "support" {
		t.Fatalf("stored support bot after reopen %+v, want its preset kept and no pin", b)
	}
}

// TestImportPresetIsFarmer checks imported bots get the farmer preset (D21).
func TestImportPresetIsFarmer(t *testing.T) {
	if ImportPreset != "farmer" {
		t.Fatalf("ImportPreset = %q, want farmer", ImportPreset)
	}
	r := openPanel(t, t.TempDir())
	added, err := r.Import("old", npcMarker, 2, ImportPreset)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range added {
		if b.Preset != "farmer" || b.Branch != 0 {
			t.Fatalf("imported %+v, want farmer with no pin", b)
		}
	}
}

// TestSupervisorSetBranch checks the branch pin: refusals for an unknown
// bot, a class that is no first profession and one of another base class; a
// change respawns an online bot whose next run carries the pin, the same pin
// again does not, 0 clears it; an imported bot of unknown class takes any
// table's branch and stays offline.
func TestSupervisorSetBranch(t *testing.T) {
	reg := openPanel(t, t.TempDir())
	imported, err := reg.Import("old", npcMarker, 1, ImportPreset)
	if err != nil {
		t.Fatal(err)
	}
	h := startSupervisor(t, reg, newClock(), 3)
	_, r1 := h.create(t, "support", ptr(botclient.HumanFighter.ID))
	if r1.Branch != 0 {
		t.Fatalf("first run branch %d, want none", r1.Branch)
	}
	refused := []struct {
		name    string
		account string
		branch  int32
		want    error
	}{
		{"unknown bot", "hz9", 7, ErrNoBot},
		{"second profession", "hz1", 2, ErrBranch},
		{"another base class's branch", "hz1", 19, ErrBranch},
		{"not a class", "hz1", 500, ErrBranch},
	}
	for _, tt := range refused {
		if err := h.sup.SetBranch(t.Context(), tt.account, tt.branch); !errors.Is(err, tt.want) {
			t.Fatalf("%s: SetBranch = %v, want %v", tt.name, err, tt.want)
		}
	}
	h.noRun(t)

	respawn := func(prev *fakeRun, branch int32) *fakeRun {
		t.Helper()
		if err := h.sup.SetBranch(t.Context(), "hz1", branch); err != nil {
			t.Fatal(err)
		}
		<-prev.ctx.Done()
		prev.end <- nil
		next := h.next(t)
		if next.Account != "hz1" || next.Branch != branch || h.list(t)["hz1"].Branch != branch {
			t.Fatalf("run after pinning %d: %s branch %d, registry %d", branch, next.Account, next.Branch, h.list(t)["hz1"].Branch)
		}
		return next
	}
	r2 := respawn(r1, 7) // Rogue, a Human Fighter's damage branch
	if err := h.sup.SetBranch(t.Context(), "hz1", 7); err != nil {
		t.Fatal(err)
	}
	h.noRun(t)
	r3 := respawn(r2, 0)

	if err := h.sup.SetBranch(t.Context(), imported[0].Account, 19); err != nil {
		t.Fatalf("pin of an imported bot: %v", err)
	}
	h.noRun(t)
	if s := h.list(t)[imported[0].Account]; s.Branch != 19 || s.Online {
		t.Fatalf("imported bot after the pin %+v", s.Bot)
	}
	h.stop(t, r3)
}
