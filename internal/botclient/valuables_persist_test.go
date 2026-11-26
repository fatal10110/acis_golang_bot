package botclient

import (
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// TestValuablesLedgerKept checks the ledger kept across a process restart
// (docs/valuables/ledger.md "Across restarts"): rows round trip, rows the
// ledger could not have written are left out, and a new Bot publishes the
// ledger it starts from.
func TestValuablesLedgerKept(t *testing.T) {
	stored := at(-24 * time.Hour)
	rows := []LedgerEntry{
		{Template: ledgerSword, Rounds: 2, Ask: 150000, StoredAt: stored},
		{Template: ledgerStaff, Enchant: 5, Rounds: 3, Ask: 900000, Released: true},
	}
	tests := []struct {
		name string
		in   []LedgerEntry
		want []LedgerEntry
	}{
		{"none", nil, nil},
		{"rows in any order come back by template and enchant", []LedgerEntry{rows[1], rows[0]}, rows},
		{"a row with no template or no ask is left out", []LedgerEntry{rows[0], {Ask: 1}, {Template: ledgerRecipe}}, rows[:1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := restoreValuables(tt.in).ledger(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ledger %+v, want %+v", got, tt.want)
			}
		})
	}
	t.Run("a new Bot publishes it", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(1, 2))
		ch, err := NewCharacter(botnames.DefaultMarker, "bot1", rng)
		if err != nil {
			t.Fatal(err)
		}
		b, err := NewBot(SessionConfig{Clock: newFakeClock(t0)}, "bot1", "not-a-secret", BotConfig{Character: ch, Ledger: rows}, rng)
		if err != nil {
			t.Fatal(err)
		}
		if got := b.Status().Ledger; !reflect.DeepEqual(got, rows) {
			t.Fatalf("status ledger %+v, want %+v", got, rows)
		}
		b.publish(StateStopped, nil)
		if got := b.Status().Ledger; !reflect.DeepEqual(got, rows) {
			t.Fatalf("status ledger after a run %+v, want %+v", got, rows)
		}
	})
}

// TestValuablesLedgerMerge checks the kept entries merge with the units
// (valuables.sync): kept unchanged while the private status is unknown,
// carried units joining theirs; once it is known, a stored unit keeps its
// entry's rounds, ask, release and storage time, an entry with no unit
// left is dropped, and a stored unit with no kept entry starts one at
// rounds 0 and the first ask, stored at now.
func TestValuablesLedgerMerge(t *testing.T) {
	stored := at(-24 * time.Hour)
	sword, staff, recipe := valuableKey{template: ledgerSword}, valuableKey{template: ledgerStaff, enchant: 5}, valuableKey{template: ledgerRecipe}
	rows := []LedgerEntry{
		{Template: ledgerSword, Rounds: 2, Ask: 150000, StoredAt: stored},
		{Template: ledgerStaff, Enchant: 5, Rounds: 3, Ask: 900000, Released: true, StoredAt: stored},
		{Template: ledgerRecipe, Rounds: 1, Ask: 400000},
	}
	firstAsk := func(valuableKey) int64 { return 111 }
	carried := map[valuableKey]carriedUnits{recipe: {count: 1, objects: []int32{401}}}
	tests := []struct {
		name   string
		stored map[valuableKey]int64
		known  bool
		want   map[valuableKey]valuableEntry
	}{
		{
			name: "status unknown", known: false,
			want: map[valuableKey]valuableEntry{
				sword:  {rounds: 2, ask: 150000, storedAt: stored},
				staff:  {rounds: 3, ask: 900000, released: true, storedAt: stored},
				recipe: {rounds: 1, ask: 400000, carried: 1, objects: []int32{401}},
			},
		},
		{
			name: "the look", known: true, stored: map[valuableKey]int64{sword: 1, {template: ledgerStaff}: 1},
			want: map[valuableKey]valuableEntry{
				sword:                   {rounds: 2, ask: 150000, stored: 1, storedAt: stored},
				{template: ledgerStaff}: {ask: 111, stored: 1, storedAt: t0},
				recipe:                  {rounds: 1, ask: 400000, carried: 1, objects: []int32{401}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := restoreValuables(rows)
			v.stored = func() (map[valuableKey]int64, bool) { return tt.stored, tt.known }
			v.sync(carried, firstAsk, t0)
			got := make(map[valuableKey]valuableEntry, len(v.entries))
			for k, e := range v.entries {
				got[k] = *e
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("entries %+v, want %+v", got, tt.want)
			}
		})
	}
	t.Run("a withdrawn entry loses its storage time", func(t *testing.T) {
		v := restoreValuables(rows[:1])
		v.stored = func() (map[valuableKey]int64, bool) { return nil, true }
		v.sync(map[valuableKey]carriedUnits{sword: {count: 1, objects: []int32{301}}}, firstAsk, t0)
		if e := v.entries[sword]; e.rounds != 2 || e.carried != 1 || !e.storedAt.IsZero() {
			t.Fatalf("withdrawn entry %+v, want rounds 2, carried, no storage time", *e)
		}
	})
}

// TestValuablesLedgerLookAfterRestart checks the session's first look after
// a process restart merges the kept ledger with the warehouse: the +0
// sword still stored keeps its rounds, ask and storage time; the released
// +5 staff, gone, is dropped; the +0 staff, not kept, starts at rounds 0.
func TestValuablesLedgerLookAfterRestart(t *testing.T) {
	stored := at(-24 * time.Hour)
	b := newKeeperLookBrain(t)
	b.cfg.valuables = restoreValuables([]LedgerEntry{
		{Template: ledgerSword, Rounds: 2, Ask: 150000, StoredAt: stored},
		{Template: ledgerStaff, Enchant: 5, Rounds: 3, Ask: 900000, Released: true, StoredAt: stored},
	})
	b.wireValuables()
	runTrainerSteps(t, b, lookSteps(0)[:1])
	if got := len(b.cfg.valuables.entries); got != 2 {
		t.Fatalf("entries before the look %v, want both kept", b.cfg.valuables.entries)
	}
	runTrainerSteps(t, b, append(lookSteps(0)[1:], trainerStep{
		name: "list", at: 5 * time.Second, events: []Event{privateList(
			WarehouseItem{ObjectID: 301, TemplateID: ledgerSword, Count: 1, BodySlot: partRHand},
			WarehouseItem{ObjectID: 302, TemplateID: ledgerStaff, Count: 1, BodySlot: partRHand},
		)},
	}))
	if _, ok := b.cfg.valuables.entries[valuableKey{template: ledgerStaff, enchant: 5}]; ok {
		t.Errorf("the gone +5 staff kept its entry: %v", b.cfg.valuables.entries)
	}
	if e := entryOf(t, b, ledgerSword, 0); e.rounds != 2 || e.ask != 150000 || e.stored != 1 || !e.storedAt.Equal(stored) {
		t.Errorf("+0 sword entry = %+v, want rounds 2, ask 150000, one stored unit, stored at %v", e, stored)
	}
	staff := valuableKey{template: ledgerStaff}
	if e := entryOf(t, b, ledgerStaff, 0); e.rounds != 0 || e.ask != b.firstAsk(staff) || e.released || e.stored != 1 || e.storedAt.IsZero() {
		t.Errorf("+0 staff entry = %+v, want rounds 0, the first ask %d, one stored unit, a storage time", e, b.firstAsk(staff))
	}
}
