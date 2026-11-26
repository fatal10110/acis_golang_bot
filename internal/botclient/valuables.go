package botclient

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// anchorArrive is how near the round's anchor every walk back of rule
// "valuables" ends (docs/valuables/round.md "Back to the anchor").
const anchorArrive = 100

// valuableKey names a ledger entry: a template at an enchant level.
type valuableKey struct {
	template int32
	enchant  uint16
}

// valuableEntry is one entry of the valuables ledger
// (docs/valuables/ledger.md "Entries and units"): what the bot cannot read
// back (rounds advertised unsold, the current ask, the release mark) and
// the units the last decision derived.
type valuableEntry struct {
	rounds int
	// ask is the current ask of one unit.
	ask int64
	// released is set by the cap (docs/valuables/release.md "The cap"):
	// the entry is in no round, its rounds and ask do not step, and it goes
	// to the store (docs/valuables/ledger.md "Released entries").
	released bool
	// carried are the units the bot carries and objects their objects, in
	// object order; stored the units its private warehouse holds.
	carried int64
	objects []int32
	stored  int64
	// storedAt is when a decision first saw stored units of the entry,
	// zero while the private status reports none.
	storedAt time.Time
}

// LedgerEntry is one valuables ledger entry as a bot's owner keeps it
// across process restarts (docs/valuables/ledger.md "Across restarts"):
// what the bot cannot read back, not its units, which the next session
// derives from its inventory and its look.
type LedgerEntry struct {
	Template int32  `json:"template"`
	Enchant  uint16 `json:"enchant,omitempty"`
	// Rounds are the rounds the entry was advertised unsold, Ask the ask
	// of one unit, Released the cap's release mark.
	Rounds   int   `json:"rounds,omitempty"`
	Ask      int64 `json:"ask"`
	Released bool  `json:"released,omitempty"`
	// StoredAt is when its units were first seen stored, zero for none.
	StoredAt time.Time `json:"stored_at,omitzero"`
}

// valuableRound is the state of a valuables round (docs/valuables/round.md):
// open or not, its start, its anchor, its ad lines (adTurn) and its trade
// requests (inviteTurn). The replies, counters and prices per partner are
// the responder's (whisperTalk), which outlive endRound.
type valuableRound struct {
	open   bool
	start  time.Time
	anchor Point
	// ads are the ad lines the round claimed, the first at first and the
	// last at adAt; named are the entries a line named since the round
	// last named every entry.
	ads   int
	first time.Time
	adAt  time.Time
	named map[valuableKey]bool
	// invite is the round's trade request under way; asked are the
	// partners the round asked (or set out to), each once.
	invite valuableInvite
	asked  map[int32]bool
	// back is when the walk back after the round's withdrawal ended, zero
	// for none: FirstLineWait counts from it, else from start. extended is
	// set once the end is put off for Grace (roundOver).
	back     time.Time
	extended bool
}

// valuables is a bot's valuables ledger, its round and its noPrivate
// keeper marks (docs/valuables/ledger.md "The valuables value"). It
// outlives a session: a Bot hands it to the brain of each session it runs,
// one at a time, like the chat log, so its only owner is the current
// session's brain goroutine.
type valuables struct {
	entries map[valuableKey]*valuableEntry
	// stored reports the units the private warehouse holds per entry and
	// whether its status is known; nil reports none stored and the status
	// known. Each session's brain wires it to its private status
	// (wireValuables).
	stored func() (map[valuableKey]int64, bool)
	round  valuableRound
	// talk is the whisper responder's state of the latest round.
	talk whisperTalk
	// noPrivate marks the keeper templates that refused a private request
	// (docs/valuables/warehouse.md "Keeper marks").
	noPrivate map[int32]bool
	// lastStart is the latest round's start, zero before any of the
	// process; the next starts RoundGap later at the earliest.
	lastStart time.Time
	// deposited are the carried valuables, by object, and their counts
	// when a deposit of them was last tried, nil before any: the off-round
	// deposit waits for a valuable picked up since (newFind).
	deposited map[int32]int32
	// capDue is set when a round an earlier session left open was cut
	// after its first line: its cap runs once this session's look is done
	// (endStaleRound).
	capDue bool
}

func newValuables() *valuables {
	return &valuables{entries: make(map[valuableKey]*valuableEntry), noPrivate: make(map[int32]bool)}
}

// restoreValuables is a new value holding the kept entries rows, with no
// units: the first decisions merge them with what the bot carries and its
// look finds (sync). A row the ledger could not have written (no template,
// no ask) is left out, so its units start a new entry.
func restoreValuables(rows []LedgerEntry) *valuables {
	v := newValuables()
	for _, r := range rows {
		if r.Template <= 0 || r.Ask <= 0 {
			continue
		}
		v.entries[valuableKey{template: r.Template, enchant: r.Enchant}] = &valuableEntry{
			rounds: r.Rounds, ask: r.Ask, released: r.Released, storedAt: r.StoredAt,
		}
	}
	return v
}

// ledger is the entries as kept across restarts, by template and enchant;
// nil for none or no value.
func (v *valuables) ledger() []LedgerEntry {
	if v == nil || len(v.entries) == 0 {
		return nil
	}
	out := make([]LedgerEntry, 0, len(v.entries))
	for k, e := range v.entries {
		out = append(out, LedgerEntry{
			Template: k.template, Enchant: k.enchant, Rounds: e.rounds, Ask: e.ask, Released: e.released, StoredAt: e.storedAt,
		})
	}
	slices.SortFunc(out, func(a, b LedgerEntry) int {
		return cmp.Or(cmp.Compare(a.Template, b.Template), cmp.Compare(a.Enchant, b.Enchant))
	})
	return out
}

// openRound opens a round at now, anchored at anchor: every walk of the
// round ends back within anchorArrive of it until endRound.
func (v *valuables) openRound(anchor Point, now time.Time) {
	v.round = valuableRound{open: true, start: now, anchor: anchor}
	v.talk = whisperTalk{}
	v.lastStart = now
}

// endRound ends the round at now; its responder's caps and deals hold for
// AnswerAfter (docs/valuables/whispers.md "After a round").
func (v *valuables) endRound(now time.Time) {
	v.round = valuableRound{}
	v.talk.ended = now
}

// carried reports whether an entry not released has carried units: what
// a round starts for (docs/valuables/ledger.md "Released entries").
func (v *valuables) carried() bool {
	for _, e := range v.entries {
		if e.carried > 0 && !e.released {
			return true
		}
	}
	return false
}

// releasedStored reports whether a released entry has stored units: what
// the release withdrawal takes out (docs/valuables/release.md "Release to
// the store").
func (v *valuables) releasedStored() bool {
	for _, e := range v.entries {
		if e.stored > 0 && e.released {
			return true
		}
	}
	return false
}

// released reports whether entry k is released.
func (v *valuables) released(k valuableKey) bool {
	e, ok := v.entries[k]
	return ok && e.released
}

// capEntries runs the cap (docs/valuables/release.md "The cap", D33) over
// every entry once, released or not, counting carried and stored units:
// all are released when they take more than HoldRows rows; all that each
// ask at most HoldValue when their asks times units pass HoldValue; and
// each unsold through MaxRounds rounds, alone. It returns how many entries
// it released that were not.
func (v *valuables) capEntries(p ValuablePolicy, stackable map[int32]bool) int {
	n := 0
	release := func(e *valuableEntry) {
		if !e.released {
			e.released = true
			n++
		}
	}
	if v.rows(stackable) > p.HoldRows {
		for _, e := range v.entries {
			release(e)
		}
		return n
	}
	var sum int64
	for _, e := range v.entries {
		if e.ask <= p.HoldValue {
			sum += e.ask * (e.carried + e.stored)
		}
	}
	for _, e := range v.entries {
		if sum > p.HoldValue && e.ask <= p.HoldValue || e.rounds >= p.MaxRounds {
			release(e)
		}
	}
	return n
}

// heldUnits are the units the ledger holds, carried and stored.
func (v *valuables) heldUnits() int64 {
	var n int64
	for _, e := range v.entries {
		n += e.carried + e.stored
	}
	return n
}

// markDeposited records rows, the carried valuables a deposit tries, as
// the ones the off-round deposit leaves until a new find.
func (v *valuables) markDeposited(rows []WarehouseRow) {
	v.deposited = make(map[int32]int32, len(rows))
	for _, r := range rows {
		v.deposited[r.Object] += r.Count
	}
}

// newFind reports whether rows, the carried valuables, hold one picked up
// since the last deposit tried (markDeposited): an object it did not
// carry, or more of a stack; any before the first.
func (v *valuables) newFind(rows []WarehouseRow) bool {
	if v.deposited == nil {
		return len(rows) > 0
	}
	for _, r := range rows {
		if r.Count > v.deposited[r.Object] {
			return true
		}
	}
	return false
}

// storedUnits are the units the private warehouse holds per entry and
// whether its status is known.
func (v *valuables) storedUnits() (map[valuableKey]int64, bool) {
	if v.stored == nil {
		return nil, true
	}
	return v.stored()
}

// carriedUnits are the units of one entry the bot carries: their count and
// objects.
type carriedUnits struct {
	count   int64
	objects []int32
}

// sync derives every entry's units from carried and the stored units
// (docs/valuables/ledger.md "Entries and units", "Stored rows with no
// entry"): a carried or stored key with no entry adds one at firstAsk's
// ask, rounds 0; an entry with none carried is dropped once the private
// status is known with none stored, and kept with its stored units while
// it is not known. An entry kept across a restart (restoreValuables) is
// one more entry: its units join it, and the look drops it once none are
// left. With the status known, storedAt is set at now when stored units
// are first seen and cleared once none are.
func (v *valuables) sync(carried map[valuableKey]carriedUnits, firstAsk func(valuableKey) int64, now time.Time) {
	stored, known := v.storedUnits()
	for k, u := range carried {
		e, ok := v.entries[k]
		if !ok {
			e = &valuableEntry{ask: firstAsk(k)}
			v.entries[k] = e
		}
		e.carried, e.objects = u.count, u.objects
	}
	for k, n := range stored {
		if _, ok := v.entries[k]; !ok && n > 0 {
			v.entries[k] = &valuableEntry{ask: firstAsk(k)}
		}
	}
	for k, e := range v.entries {
		if _, ok := carried[k]; !ok {
			e.carried, e.objects = 0, nil
		}
		if known {
			e.stored = stored[k]
			switch {
			case e.stored == 0:
				e.storedAt = time.Time{}
			case e.storedAt.IsZero():
				e.storedAt = now
			}
		}
		if e.carried == 0 && e.stored == 0 && known {
			delete(v.entries, k)
		}
	}
}

// rows are the warehouse rows the entries' units take, counted as the
// server's storeSlots counts them (inventory/warehouse.go): one per unit
// of a template that does not stack, one per template that does.
func (v *valuables) rows(stackable map[int32]bool) int {
	n := 0
	stacks := make(map[int32]bool)
	for k, e := range v.entries {
		units := e.carried + e.stored
		switch {
		case units == 0:
		case !stackable[k.template]:
			n += int(units)
		case !stacks[k.template]:
			stacks[k.template] = true
			n++
		}
	}
	return n
}

// indexShadows are the shadow item templates of items: those with a mana
// Duration, as indexCrystals reads them.
func indexShadows(items map[int32]datapack.Item) map[int32]bool {
	out := make(map[int32]bool)
	for id, it := range items {
		if it.Duration > 0 {
			out[id] = true
		}
	}
	return out
}

// ownHold is the persona's own keep hook's answer for carried item it,
// never the clan member's hold that wireMember wraps around it.
func (b *Brain) ownHold(it Item) Hold {
	if !b.member.wired {
		return b.hold(it)
	}
	if b.member.holds == nil {
		return Hold{}
	}
	return b.member.holds(b.world, it)
}

// valuable reports whether carried item it is a valuable
// (docs/valuables/score.md "Valuable test", D31): it passes the persona's
// numeric test (ValuablePolicy.valuable) and is not equipped, a quest
// item, augmented, a shadow item or sold by a counted buylist, nor kept
// for the bot's use (keeps), by the persona's own keep hook (ownHold), worn
// next or fallen back to by the gear errand, nor a piece of the set it buys
// towards. The bot's keeps, the hook and the gear errand go first; the
// clan member's hold comes after it (memberKeep). The zero policy finds
// none.
func (b *Brain) valuable(it Item) bool {
	if it.Equipped || it.SubCategory == subCategoryQuest || it.Augmentation != 0 || !b.valuableTemplate(it.TemplateID, it.Enchant) {
		return false
	}
	if b.keeps(it.TemplateID) || b.ownHold(it).Keep {
		return false
	}
	w := b.worn()
	return !b.upgrade(it, &w) && !b.fallsBack(it, &w) && !b.keepsForSet(it, &w)
}

// valuableTemplate reports whether template at enchant passes the parts of
// the valuable test that read neither the bot's carry nor the unit's own
// classes: the persona's numeric test (ValuablePolicy.valuable), not a
// shadow item, not a dye and not sold by a counted buylist. The zero policy
// passes none. A dye is a supply of the dye errand: the keep test asks the
// dye plan about it (plannedDye), and the plan's budget asks the valuables
// back (valuableFees), so a dye that passed would recurse without end
// (#622).
func (b *Brain) valuableTemplate(template int32, enchant uint16) bool {
	m, p := b.cfg.Travel, b.persona.Valuables
	if m == nil || !p.Enabled || m.shadows[template] || m.dyes[template] {
		return false
	}
	if _, shopSells := m.cheapest[template]; shopSells {
		return false
	}
	return p.valuable(m.valueScore(p, template, int(enchant)))
}

// valuableRow reports whether private warehouse row r passes the warehouse
// row test (docs/valuables/ledger.md "Stored rows with no entry"): the
// valuable test at the row's enchant without the carry checks, which
// decide what the bot wants to carry, not what it stored.
func (b *Brain) valuableRow(r WarehouseItem) bool {
	return r.SubCategory != subCategoryQuest && r.Augmentation == 0 && b.valuableTemplate(r.TemplateID, r.Enchant)
}

// carriedValuables are the units of every valuable the bot carries, by
// entry key, their objects in object order.
func (b *Brain) carriedValuables() map[valuableKey]carriedUnits {
	out := make(map[valuableKey]carriedUnits)
	for _, it := range b.world.Inventory.Items {
		if !b.valuable(it) {
			continue
		}
		k := valuableKey{template: it.TemplateID, enchant: it.Enchant}
		u := out[k]
		u.count += int64(it.Count)
		u.objects = append(u.objects, it.ObjectID)
		out[k] = u
	}
	for k, u := range out {
		slices.Sort(u.objects)
		out[k] = u
	}
	return out
}

// syncValuables derives the ledger's units on each decision of a persona
// whose ValuablePolicy is enabled (valuables.sync), and sets the held
// valuables gauge from it.
func (b *Brain) syncValuables(now time.Time) {
	if !b.persona.Valuables.Enabled || b.cfg.Travel == nil {
		return
	}
	b.cfg.valuables.sync(b.carriedValuables(), b.firstAsk, now)
	b.metrics.ValuablesHeld(b.cfg.valuables.heldUnits())
}

// firstAsk is the first ask of one unit of entry k
// (docs/valuables/whispers.md "Prices", D32): the reference price times
// min(MaxAsk, 100 + AskPerRarity × R) percent, plus the enchant scrolls at
// their reference price, never below the floor.
func (b *Brain) firstAsk(k valuableKey) int64 {
	p := b.persona.Valuables
	sc := b.cfg.Travel.valueScore(p, k.template, int(k.enchant))
	pct := min(p.MaxAsk, 100+p.AskPerRarity*sc.r)
	ask := int64(math.Ceil(float64(sc.ref)*pct/100)) + int64(sc.w) - int64(sc.ref)
	return max(ask, b.valuableFloor(k))
}

// valuableFloor is the least price of one unit of entry k (D32): its worth
// W times FloorPct percent or the store's price of the bare template,
// whichever is higher.
func (b *Brain) valuableFloor(k valuableKey) int64 {
	p := b.persona.Valuables
	sc := b.cfg.Travel.valueScore(p, k.template, int(k.enchant))
	return max(int64(math.Ceil(sc.w*float64(p.FloorPct)/100)), int64(b.storePrice(sc.ref)))
}

// lowered is the ask after an unsold round: RoundStep points of the
// reference price ref lower, never below floor.
func (p ValuablePolicy) lowered(ask int64, ref int32, floor int64) int64 {
	return max(ask-int64(p.RoundStep)*int64(ref)/100, floor)
}

// ruleValuables is rule "valuables" (docs/valuables/round.md "The rule"):
// the session's look at its private warehouse (lookTurn), then the
// valuables round. Once roundDue holds it opens a round anchored where the
// bot stands and runs it (roundStep), holding the turn on every decision
// while it runs, so no later rule walks the bot away. With the budget
// spent it runs the off-round deposit instead (offRoundDue); with no round
// due and released valuables stored, the off-round release withdrawal
// (releaseDue); each holds the turn while it and its walk back run. A
// round or release withdrawal pending on timers only holds the turn with a
// wait for StoreHold at most (holdTurn).
//
// Pacing: the look's, the round's and the off-round errands' private
// requests as rule "warehouse"'s (one open per warehouseOpenGap, openDue);
// the walks' MoveBackwardToLocation segments as every goTo; the ad lines
// as adTurn's; the trade requests as inviteTurn's; one round per RoundGap;
// one off-round deposit per new find.
func (b *Brain) ruleValuables(now time.Time) (Action, bool) {
	b.trackHold(now)
	if a, ok := b.lookTurn(now); ok {
		b.held.pause(now)
		return a, true
	}
	b.held.resume(now)
	if b.round.phase == roundIdle && !b.cfg.valuables.round.open {
		switch {
		case b.offRoundDue(now):
			if !b.startOffRound(roundDeposit, now) {
				return Action{}, false
			}
		case b.roundDue(now):
			b.startRound(now)
		case b.releaseDue(now):
			if !b.startOffRound(roundWithdraw, now) {
				return Action{}, false
			}
		default:
			return Action{}, b.holdTurn(now)
		}
	}
	return b.roundStep(now)
}

// gradeLetters are the crystal grades as an ad names them, by grade.
var gradeLetters = [...]string{1: "D", 2: "C", 3: "B", 4: "A", 5: "S"}

// itemLabel is how an ad names units of entry k at price each
// (docs/valuables/ads.md "Lines"): "2x " for several units, "+5 " for an
// enchanted one, the template's name ("item <id>" for none), " (C)" for
// graded gear, and the price as players write it (priceText).
func (m *TravelMap) itemLabel(k valuableKey, units, price int64) string {
	label := m.itemName(k) + " " + priceText(price)
	if units > 1 {
		return fmt.Sprintf("%dx %s", units, label)
	}
	return label
}

// itemName is how a line names entry k: itemLabel's without units or
// price.
func (m *TravelMap) itemName(k valuableKey) string {
	var sb strings.Builder
	if k.enchant > 0 {
		fmt.Fprintf(&sb, "+%d ", k.enchant)
	}
	if name := m.itemNames[k.template]; name != "" {
		sb.WriteString(name)
	} else {
		fmt.Fprintf(&sb, "item %d", k.template)
	}
	if y, ok := m.crystals[k.template]; ok && y.grade > 0 && y.grade < len(gradeLetters) {
		fmt.Fprintf(&sb, " (%s)", gradeLetters[y.grade])
	}
	return sb.String()
}

// priceText writes price as players do: "173k" in thousands, "1.2kk" in
// millions to a tenth, rounded up to the unit shown so it is never below
// price; under a thousand in plain digits.
func priceText(price int64) string {
	if price < 1000 {
		return strconv.FormatInt(price, 10)
	}
	if k := ceilDiv(price, 1000); k < 1000 {
		return strconv.FormatInt(k, 10) + "k"
	}
	tenths := ceilDiv(price, 100000)
	if tenths%10 == 0 {
		return strconv.FormatInt(tenths/10, 10) + "kk"
	}
	return fmt.Sprintf("%d.%dkk", tenths/10, tenths%10)
}

// ceilDiv is a / b rounded up, for a ≥ 0 and b > 0.
func ceilDiv(a, b int64) int64 {
	return (a + b - 1) / b
}
