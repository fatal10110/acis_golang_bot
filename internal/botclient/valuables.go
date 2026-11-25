package botclient

import (
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
// back (rounds advertised unsold, the current ask) and the units the last
// decision derived. The release mark lands with the cap that sets it
// (V7c).
type valuableEntry struct {
	rounds int
	// ask is the current ask of one unit.
	ask int64
	// carried are the units the bot carries and objects their objects, in
	// object order; stored the units its private warehouse holds.
	carried int64
	objects []int32
	stored  int64
}

// valuableRound is the state of a valuables round (docs/valuables/round.md):
// open or not, its start and its anchor. The ad lines said, the replies,
// trade requests, counters and prices per partner land with the steps that
// read them (V4-V6), reset by openRound and endRound as these are.
type valuableRound struct {
	open   bool
	start  time.Time
	anchor Point
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
	// noPrivate marks the keeper templates that refused a private request
	// (docs/valuables/warehouse.md "Keeper marks").
	noPrivate map[int32]bool
}

func newValuables() *valuables {
	return &valuables{entries: make(map[valuableKey]*valuableEntry), noPrivate: make(map[int32]bool)}
}

// openRound opens a round at now, anchored at anchor: every walk of the
// round ends back within anchorArrive of it until endRound.
func (v *valuables) openRound(anchor Point, now time.Time) {
	v.round = valuableRound{open: true, start: now, anchor: anchor}
}

// endRound ends the round.
func (v *valuables) endRound() {
	v.round = valuableRound{}
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
// it is not known.
func (v *valuables) sync(carried map[valuableKey]carriedUnits, firstAsk func(valuableKey) int64) {
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
// shadow item and not sold by a counted buylist. The zero policy passes
// none.
func (b *Brain) valuableTemplate(template int32, enchant uint16) bool {
	m, p := b.cfg.Travel, b.persona.Valuables
	if m == nil || p == (ValuablePolicy{}) || m.shadows[template] {
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
// whose ValuablePolicy is enabled (valuables.sync).
func (b *Brain) syncValuables() {
	if b.persona.Valuables == (ValuablePolicy{}) || b.cfg.Travel == nil {
		return
	}
	b.cfg.valuables.sync(b.carriedValuables(), b.firstAsk)
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

// ruleValuables holds the turn for the session's look at its private
// warehouse (lookTurn) and while a valuables round is open
// (docs/valuables/round.md "The rule"), so no later rule walks the bot
// away: it walks back to within anchorArrive of the round's anchor (goTo,
// walkPlace) and sends nothing once there or when no route there holds.
// Only tests open a round until the round's start lands (V7a).
//
// Pacing: the walk's MoveBackwardToLocation segments, paced as every goTo;
// the look's as rule "warehouse"'s.
func (b *Brain) ruleValuables(now time.Time) (Action, bool) {
	if a, ok := b.lookTurn(now); ok {
		return a, true
	}
	r := &b.cfg.valuables.round
	if !r.open {
		return Action{}, false
	}
	if a, st := b.goTo(r.anchor, anchorArrive, walkPlace, now); st == walkWalking {
		return a, true
	}
	return Action{}, true
}

// gradeLetters are the crystal grades as an ad names them, by grade.
var gradeLetters = [...]string{1: "D", 2: "C", 3: "B", 4: "A", 5: "S"}

// itemLabel is how an ad names units of entry k at price each
// (docs/valuables/ads.md "Lines"): "2x " for several units, "+5 " for an
// enchanted one, the template's name ("item <id>" for none), " (C)" for
// graded gear, and the price as players write it (priceText).
func (m *TravelMap) itemLabel(k valuableKey, units, price int64) string {
	var sb strings.Builder
	if units > 1 {
		fmt.Fprintf(&sb, "%dx ", units)
	}
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
	sb.WriteString(" " + priceText(price))
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
