package botregistry

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// onDisk reads dir's registry file as a new process would, without the lock.
func onDisk(t *testing.T, dir string) file {
	t.Helper()
	r := &Registry{dir: dir, data: file{Version: Version}}
	if err := r.load(); err != nil {
		t.Fatal(err)
	}
	return r.data
}

// diskBots maps the bots of dir's registry file by account.
func diskBots(t *testing.T, dir string) map[string]Bot {
	t.Helper()
	m := make(map[string]Bot)
	for _, b := range onDisk(t, dir).Bots {
		m[b.Account] = b
	}
	return m
}

func TestRegistryUpdateLaterWaitsForFlush(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	if _, err := r.NewBot(Bot{Preset: "town"}); err != nil {
		t.Fatal(err)
	}
	w := r.writes
	for _, reason := range []string{"one", "two", "three"} {
		if err := r.UpdateLater("hz1", func(b *Bot) { b.Online, b.StopReason = true, reason }); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.UpdateLater("hz9", func(b *Bot) { b.Online = true }); err == nil {
		t.Fatal("UpdateLater of an unknown bot succeeded")
	}
	if b, _ := r.Bot("hz1"); !b.Online || b.StopReason != "three" {
		t.Fatalf("bot in memory %+v: want the last deferred change", b)
	}
	if r.writes != w || !r.Dirty() {
		t.Fatalf("writes %d dirty %v after deferred changes, want %d and dirty", r.writes, r.Dirty(), w)
	}
	if b := diskBots(t, dir)["hz1"]; b.Online || b.StopReason != "" {
		t.Fatalf("bot on disk %+v before the flush", b)
	}
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if r.writes != w+1 || r.Dirty() {
		t.Fatalf("writes %d dirty %v after two flushes, want %d and clean", r.writes, r.Dirty(), w+1)
	}
	if b := diskBots(t, dir)["hz1"]; !b.Online || b.StopReason != "three" {
		t.Fatalf("bot on disk %+v after the flush", b)
	}
}

// TestRegistryDurableChangesSaveAtOnce: a name and class, a new bot and its
// counter, a tombstone and an import are on disk when the call returns, with
// every deferred change before them, in one write.
func TestRegistryDurableChangesSaveAtOnce(t *testing.T) {
	ix := botnames.Marker{Text: "ix", Placement: botnames.DefaultMarker.Placement}
	tests := []struct {
		name  string
		do    func(*Registry) error
		check func(t *testing.T, f file, bots map[string]Bot)
	}{
		{
			name: "learned name and class",
			do: func(r *Registry) error {
				return r.Update("hz1", func(b *Bot) { b.Name, b.Class = "Pinned", ptr(3) })
			},
			check: func(t *testing.T, _ file, bots map[string]Bot) {
				if b := bots["hz1"]; b.Name != "Pinned" || b.Class == nil || *b.Class != 3 {
					t.Fatalf("bot on disk %+v: want Pinned of class 3", b)
				}
			},
		},
		{
			name: "new bot",
			do:   func(r *Registry) error { _, err := r.NewBot(Bot{Preset: "town"}); return err },
			check: func(t *testing.T, f file, bots map[string]Bot) {
				if _, ok := bots["hz3"]; !ok || f.Prefixes[0].Counter != 3 {
					t.Fatalf("disk holds bots %v counter %d: want hz3 and 3", bots, f.Prefixes[0].Counter)
				}
			},
		},
		{
			name: "drop",
			do:   func(r *Registry) error { return r.Drop("hz1", "gone", t0) },
			check: func(t *testing.T, f file, bots map[string]Bot) {
				if _, ok := bots["hz1"]; ok || len(f.Tombstones) != 1 || f.Tombstones[0].Account != "hz1" {
					t.Fatalf("disk holds bots %v tombstones %+v: want hz1 dropped", bots, f.Tombstones)
				}
			},
		},
		{
			name: "import",
			do:   func(r *Registry) error { _, err := r.Import("ix", ix, 2, ImportPreset); return err },
			check: func(t *testing.T, _ file, bots map[string]Bot) {
				if _, ok := bots["ix2"]; !ok {
					t.Fatalf("disk holds bots %v: want ix1 and ix2", bots)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			r := openPanel(t, dir)
			for range 2 {
				if _, err := r.NewBot(Bot{Preset: "town"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.UpdateLater("hz2", func(b *Bot) { b.Online, b.StopReason = true, "deferred" }); err != nil {
				t.Fatal(err)
			}
			w := r.writes
			if err := tt.do(r); err != nil {
				t.Fatal(err)
			}
			if r.writes != w+1 || r.Dirty() {
				t.Fatalf("writes %d dirty %v, want %d and clean", r.writes, r.Dirty(), w+1)
			}
			bots := diskBots(t, dir)
			tt.check(t, onDisk(t, dir), bots)
			if b := bots["hz2"]; !b.Online || b.StopReason != "deferred" {
				t.Fatalf("deferred change not written with the save: %+v", b)
			}
		})
	}
}

func TestRegistryCloseFlushes(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetPanel("hz", botnames.DefaultMarker); err != nil {
		t.Fatal(err)
	}
	if _, err := r.NewBot(Bot{Preset: "town"}); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateLater("hz1", func(b *Bot) { b.Level = 9 }); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if b := diskBots(t, dir)["hz1"]; b.Level != 9 {
		t.Fatalf("bot on disk %+v after Close: want level 9", b)
	}
}

// TestRegistryCrashLosesOnlyDeferred: a process that dies with a deferred
// change unsaved loses that change only; the next process never reuses an
// account.
func TestRegistryCrashLosesOnlyDeferred(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetPanel("hz", botnames.DefaultMarker); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := r.NewBot(Bot{Preset: "town"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Update("hz1", func(b *Bot) { b.Name = "Pinned" }); err != nil {
		t.Fatal(err)
	}
	if err := r.Drop("hz2", "gone", t0); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateLater("hz1", func(b *Bot) { b.Online, b.StopReason = true, "late" }); err != nil {
		t.Fatal(err)
	}
	// The crash: the lock goes with the process, nothing is flushed.
	if err := r.lock.Close(); err != nil {
		t.Fatal(err)
	}

	r2 := openPanel(t, dir)
	b, _ := r2.Bot("hz1")
	if b.Name != "Pinned" || b.Online || b.StopReason != "" {
		t.Fatalf("bot after the crash %+v: want the name kept and the deferred stop lost", b)
	}
	if !slices.ContainsFunc(r2.Tombstones(), func(t Tombstone) bool { return t.Account == "hz2" }) {
		t.Fatalf("tombstones after the crash %+v: want hz2", r2.Tombstones())
	}
	nb, err := r2.NewBot(Bot{Preset: "town"})
	if err != nil {
		t.Fatal(err)
	}
	if nb.Account != "hz3" {
		t.Fatalf("new bot after the crash on %s, want hz3", nb.Account)
	}
}
