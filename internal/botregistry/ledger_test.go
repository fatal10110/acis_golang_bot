package botregistry

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// ledgerRows are a merchant's kept ledger: a +0 sword stored a day, a
// released +5 staff carried.
var ledgerRows = []botclient.LedgerEntry{
	{Template: 5310, Rounds: 2, Ask: 150000, StoredAt: time.Date(2026, 11, 25, 9, 30, 0, 0, time.UTC)},
	{Template: 5311, Enchant: 5, Rounds: 3, Ask: 900000, Released: true},
}

// TestRegistryLedgerRoundTrip checks a bot's valuables ledger survives a
// reopen and a bot with none writes no ledger field
// (docs/valuables/ledger.md "Across restarts").
func TestRegistryLedgerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := openPanel(t, dir)
	kept, err := r.NewBot(Bot{Preset: "merchant"})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := r.NewBot(Bot{Preset: "merchant"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateLater(kept.Account, func(b *Bot) { b.Ledger = ledgerRows }); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), `"ledger":`); n != 1 {
		t.Fatalf("registry file holds %d ledger fields, want the merchant's only:\n%s", n, raw)
	}
	r = openPanel(t, dir)
	if b, ok := r.Bot(kept.Account); !ok || !reflect.DeepEqual(b.Ledger, ledgerRows) {
		t.Fatalf("ledger after reopen %+v, want %+v", b.Ledger, ledgerRows)
	}
	if b, ok := r.Bot(plain.Account); !ok || b.Ledger != nil {
		t.Fatalf("bot with no ledger after reopen %+v", b)
	}
}

// TestKeepProgressLedger checks a run's last snapshot replaces the kept
// ledger once the bot entered the world, an empty one clearing it, and a
// snapshot of a bot that never entered keeps it.
func TestKeepProgressLedger(t *testing.T) {
	sold := []botclient.LedgerEntry{ledgerRows[1]}
	tests := []struct {
		name string
		st   botclient.Status
		want []botclient.LedgerEntry
	}{
		{"never entered keeps the stored one", botclient.Status{State: botclient.StateStopped}, ledgerRows},
		{"entered copies the snapshot's", botclient.Status{Level: 20, Ledger: sold}, sold},
		{"entered with none clears it", botclient.Status{Level: 20}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := Bot{Ledger: ledgerRows}
			keepProgress(&b, tt.st)
			if !reflect.DeepEqual(b.Ledger, tt.want) {
				t.Fatalf("ledger %+v, want %+v", b.Ledger, tt.want)
			}
		})
	}
}

// TestSupervisorRunLedger checks a restored bot's run carries its kept
// ledger, as its own copy.
func TestSupervisorRunLedger(t *testing.T) {
	reg := openPanel(t, t.TempDir())
	if _, err := reg.NewBot(Bot{Class: ptr(0), Preset: "merchant", Online: true, Ledger: ledgerRows}); err != nil {
		t.Fatal(err)
	}
	h := startSupervisor(t, reg, newClock(), 3)
	r := h.next(t)
	if !reflect.DeepEqual(r.Ledger, ledgerRows) {
		t.Fatalf("run ledger %+v, want %+v", r.Ledger, ledgerRows)
	}
	if &r.Ledger[0] == &ledgerRows[0] {
		t.Fatal("run ledger shares the registry's rows")
	}
}
