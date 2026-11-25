package botclient

import (
	"slices"
	"testing"
	"time"
)

// The ledger fixture's valuables, each with a supply of one an hour (R 2,
// so a first ask of 125 % of the reference price): Sword of Watershadow
// (no grade, no scroll), Conjuror's Staff (D grade, Scroll: Enchant Weapon
// (Grade D) at 50000 a level) and a stackable recipe scroll.
const (
	ledgerSword  int32 = 121
	ledgerStaff  int32 = 184
	ledgerRecipe int32 = 5458
	ledgerScroll int32 = 955
)

// newLedgerBrain is newStoreBrain (a 140 % store markup) with the default
// ValuablePolicy and the ledger fixture's valuables on its travel map.
func newLedgerBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newStoreBrain(t, 1000000, items...)
	b.persona.Valuables = defaultValuablePolicy()
	m := b.cfg.Travel
	m.prices = map[int32]int32{ledgerSword: 136000, ledgerStaff: 644000, ledgerRecipe: 366000, ledgerScroll: 50000}
	m.supply = map[int32]itemSupply{
		ledgerSword:  {perHour: 1, safe: 3},
		ledgerStaff:  {perHour: 1, safe: 3, scroll: ledgerScroll},
		ledgerRecipe: {perHour: 1},
	}
	m.stackable = map[int32]bool{ledgerRecipe: true}
	return b
}

func swordUnit(obj int32, enchant uint16) Item {
	return Item{ObjectID: obj, TemplateID: ledgerSword, Count: 1, BodySlot: partRHand, Enchant: enchant}
}

// entryOf is the ledger's entry of template at enchant, failing the test
// when there is none.
func entryOf(t *testing.T, b *Brain, template int32, enchant uint16) valuableEntry {
	t.Helper()
	e, ok := b.cfg.valuables.entries[valuableKey{template: template, enchant: enchant}]
	if !ok {
		t.Fatalf("no entry for %d +%d; entries %v", template, enchant, b.cfg.valuables.entries)
	}
	return *e
}

// TestValuablesLedger checks the ledger (docs/valuables/ledger.md): entries
// per template and enchant with their first ask and derived units, rows as
// the server counts them, units leaving and joining, the drop once the
// private status is known with none stored, the prices, and the round's
// anchor with rule "valuables"'s walk back.
func TestValuablesLedger(t *testing.T) {
	t.Run("entries, units and rows", func(t *testing.T) {
		b := newLedgerBrain(t, swordUnit(101, 0), swordUnit(102, 0), swordUnit(104, 5), lootStack(103, ledgerRecipe, 5))
		b.decide(t0)
		if got := len(b.cfg.valuables.entries); got != 3 {
			t.Fatalf("entries = %d, want 3 (+0 and +5 swords, the recipe)", got)
		}
		if e := entryOf(t, b, ledgerSword, 0); e.rounds != 0 || e.ask != 170000 || e.carried != 2 || !slices.Equal(e.objects, []int32{101, 102}) {
			t.Errorf("+0 sword entry = %+v, want rounds 0, ask 170000, two units 101 and 102", e)
		}
		if e := entryOf(t, b, ledgerSword, 5); e.ask != 178500 || e.carried != 1 {
			t.Errorf("+5 sword entry = %+v, want ask 178500 (R 2.5, no scroll), one unit", e)
		}
		if e := entryOf(t, b, ledgerRecipe, 0); e.ask != 457500 || e.carried != 5 || len(e.objects) != 1 {
			t.Errorf("recipe entry = %+v, want ask 457500, five units in one stack", e)
		}
		if got := b.cfg.valuables.rows(b.cfg.Travel.stackable); got != 4 {
			t.Errorf("rows = %d, want 4: a row per sword, one for the recipe stack", got)
		}
		b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: swordUnit(102, 0)}}}}, at(time.Second))
		b.decide(at(time.Second))
		if e := entryOf(t, b, ledgerSword, 0); e.carried != 1 || !slices.Equal(e.objects, []int32{101}) {
			t.Errorf("+0 sword entry after a unit left = %+v, want one unit 101", e)
		}
	})

	t.Run("a found unit joins its entry", func(t *testing.T) {
		b := newLedgerBrain(t, swordUnit(101, 0))
		b.decide(t0)
		e := b.cfg.valuables.entries[valuableKey{template: ledgerSword}]
		e.rounds, e.ask = 2, 150000
		b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: swordUnit(105, 0)}}}}, at(time.Second))
		b.decide(at(time.Second))
		if got := entryOf(t, b, ledgerSword, 0); got.rounds != 2 || got.ask != 150000 || got.carried != 2 {
			t.Errorf("entry = %+v, want rounds 2, ask 150000, two units", got)
		}
	})

	t.Run("none carried", func(t *testing.T) {
		tests := []struct {
			name   string
			stored map[valuableKey]int64
			known  bool
			kept   bool
		}{
			{"status not known", nil, false, true},
			{"known, none stored", nil, true, false},
			{"known, one stored", map[valuableKey]int64{{template: ledgerSword}: 1}, true, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				b := newLedgerBrain(t, swordUnit(101, 0))
				b.decide(t0)
				b.cfg.valuables.entries[valuableKey{template: ledgerSword}].rounds = 1
				b.cfg.valuables.stored = func() (map[valuableKey]int64, bool) { return tt.stored, tt.known }
				b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: swordUnit(101, 0)}}}}, at(time.Second))
				b.decide(at(time.Second))
				e, ok := b.cfg.valuables.entries[valuableKey{template: ledgerSword}]
				if ok != tt.kept {
					t.Fatalf("entry kept = %v, want %v", ok, tt.kept)
				}
				if ok && (e.carried != 0 || e.stored != tt.stored[valuableKey{template: ledgerSword}] || e.rounds != 1) {
					t.Errorf("kept entry = %+v, want none carried, the stored units, its rounds", *e)
				}
				if ok {
					return
				}
				b.handle([]Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: swordUnit(106, 0)}}}}, at(2*time.Second))
				b.decide(at(2 * time.Second))
				if got := entryOf(t, b, ledgerSword, 0); got.rounds != 0 || got.ask != 170000 || got.carried != 1 {
					t.Errorf("entry found after the drop = %+v, want a new one: rounds 0, the first ask", got)
				}
			})
		}
	})

	t.Run("prices", func(t *testing.T) {
		tests := []struct {
			name       string
			key        valuableKey
			floorPct   int
			ask, floor int64
		}{
			{"no grade", valuableKey{template: ledgerSword}, 100, 170000, 136000},
			{"no grade +5: the reference price as floor", valuableKey{template: ledgerSword, enchant: 5}, 100, 178500, 136000},
			{"+5 Conjuror's Staff: five scrolls", valuableKey{template: ledgerStaff, enchant: 5}, 100, 845250 + 250000, 894000},
			{"store price over the worth share", valuableKey{template: ledgerSword}, 50, 170000, 95200},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				b := newLedgerBrain(t)
				b.persona.Valuables.FloorPct = tt.floorPct
				if got := b.firstAsk(tt.key); got != tt.ask {
					t.Errorf("first ask = %d, want %d", got, tt.ask)
				}
				if got := b.valuableFloor(tt.key); got != tt.floor {
					t.Errorf("floor = %d, want %d", got, tt.floor)
				}
			})
		}
		p := defaultValuablePolicy()
		if got := p.lowered(170000, 136000, 136000); got != 156400 {
			t.Errorf("lowered once = %d, want 156400: 10 points of the reference price", got)
		}
		if got := p.lowered(140000, 136000, 136000); got != 136000 {
			t.Errorf("lowered past the floor = %d, want the floor 136000", got)
		}
	})

	t.Run("round and walk back", func(t *testing.T) {
		b := newLedgerBrain(t, swordUnit(101, 0))
		if rule, _ := b.decide(t0); rule == "valuables" {
			t.Fatal(`rule "valuables" matched with no round open`)
		}
		anchor := Point{X: 3000}
		v := b.cfg.valuables
		v.openRound(anchor, t0)
		if rule, a := b.decide(t0); rule != "valuables" || a.kind != actHold {
			t.Errorf("decide with no route to the anchor = %q, %d; want rule valuables staying", rule, a.kind)
		}
		walkOn(t, b)
		now := at(time.Second)
		rule, a := b.decide(now)
		if rule != "valuables" || a.kind != actMove {
			t.Fatalf("decide away from the anchor = %q, %d; want rule valuables walking", rule, a.kind)
		}
		if to := moveTarget(a.payload); to.X <= 0 || b.walk.arrive != anchorArrive || horizontal2(b.walk.dest, anchor) > 0 {
			t.Errorf("leg to %+v of a walk to %+v within %d, want toward the anchor %+v within %d", to, b.walk.dest, b.walk.arrive, anchor, anchorArrive)
		}
		now = at(time.Minute)
		b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 2950}}}, now)
		now = now.Add(time.Second)
		if rule, a := b.decide(now); rule != "valuables" || a.kind != actHold {
			t.Errorf("decide at the anchor = %q, %d; want rule valuables holding the turn, no leg", rule, a.kind)
		}
		if v.round.anchor != anchor || !v.round.open || !v.round.start.Equal(t0) {
			t.Errorf("round = %+v, want open since t0 at the anchor", v.round)
		}
		v.endRound(t0)
		if v.round.open || v.round.anchor != (Point{}) {
			t.Errorf("round after endRound = %+v, want none", v.round)
		}
		if rule, _ := b.decide(now.Add(time.Second)); rule == "valuables" {
			t.Error(`rule "valuables" matched after endRound`)
		}
	})
}
