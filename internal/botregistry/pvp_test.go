package botregistry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestRegistryPvPRoundTrip checks a bot's PvP switches survive a reopen and
// a bot with none writes no pvp field (docs/pvp/modes.md).
func TestRegistryPvPRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	set, err := r.NewBot(Bot{Preset: "pvp"})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := r.NewBot(Bot{Preset: "pvp"})
	if err != nil {
		t.Fatal(err)
	}
	want := PvPModes{Duels: true, PK: true}
	if err := r.Update(set.Account, func(b *Bot) { b.PvP = &want }); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), `"pvp":`); n != 1 {
		t.Fatalf("registry file holds %d pvp fields, want the switched bot's only:\n%s", n, raw)
	}
	r = openPanel(t, dir)
	if b, ok := r.Bot(set.Account); !ok || b.PvP == nil || *b.PvP != want {
		t.Fatalf("switched bot after reopen %+v", b)
	}
	if b, ok := r.Bot(plain.Account); !ok || b.PvP != nil {
		t.Fatalf("bot with no switches after reopen %+v", b)
	}
}

// TestSupervisorSetPvP checks the PvP switches: an unknown bot refused; a
// change respawns an online bot whose next run carries the switches, the
// same switches again do not, nil gives back the preset's defaults; an
// offline bot's change keeps it offline.
func TestSupervisorSetPvP(t *testing.T) {
	reg := openPanel(t, t.TempDir())
	h := startSupervisor(t, reg, newClock(), 3)
	_, r1 := h.create(t, "support", ptr(botclient.HumanFighter.ID))
	if r1.PvP != nil {
		t.Fatalf("first run switches %+v, want the defaults", r1.PvP)
	}
	if err := h.sup.SetPvP(t.Context(), "hz9", &PvPModes{}); !errors.Is(err, ErrNoBot) {
		t.Fatalf("SetPvP of an unknown bot = %v, want %v", err, ErrNoBot)
	}
	h.noRun(t)

	respawn := func(prev *fakeRun, modes *PvPModes) *fakeRun {
		t.Helper()
		if err := h.sup.SetPvP(t.Context(), "hz1", modes); err != nil {
			t.Fatal(err)
		}
		<-prev.ctx.Done()
		prev.end <- nil
		next := h.next(t)
		got := h.list(t)["hz1"].PvP
		if next.Account != "hz1" || (modes == nil) != (next.PvP == nil) || modes != nil && (*next.PvP != *modes || got == nil || *got != *modes) || modes == nil && got != nil {
			t.Fatalf("run after switching %+v: %s run %+v, registry %+v", modes, next.Account, next.PvP, got)
		}
		return next
	}
	pk := &PvPModes{Duels: true, Hunt: true, Arenas: true, PK: true}
	r2 := respawn(r1, pk)
	if err := h.sup.SetPvP(t.Context(), "hz1", &PvPModes{Duels: true, Hunt: true, Arenas: true, PK: true}); err != nil {
		t.Fatal(err)
	}
	h.noRun(t)
	pk.Arenas = false // the registry holds its own copy
	if got := h.list(t)["hz1"].PvP; got == nil || !got.Arenas {
		t.Fatalf("registry switches %+v changed with the caller's", got)
	}
	r3 := respawn(r2, nil)
	if err := h.sup.SetPvP(t.Context(), "hz1", nil); err != nil {
		t.Fatal(err)
	}
	h.noRun(t)

	if err := h.sup.Despawn(t.Context(), "hz1"); err != nil {
		t.Fatal(err)
	}
	<-r3.ctx.Done()
	r3.end <- nil
	if err := h.sup.SetPvP(t.Context(), "hz1", &PvPModes{Duels: true}); err != nil {
		t.Fatal(err)
	}
	h.noRun(t)
	if s := h.list(t)["hz1"]; s.PvP == nil || !s.PvP.Duels || s.Online {
		t.Fatalf("offline bot after switching %+v", s.Bot)
	}
	h.stop(t)
}
