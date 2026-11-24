package botclient

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Enchant scroll templates of the enchant tests (enchant/service.go
// scrolls).
const (
	scrollWD        int32 = 955  // Scroll: Enchant Weapon (Grade D)
	scrollAD        int32 = 956  // Scroll: Enchant Armor (Grade D)
	scrollWC        int32 = 951  // Scroll: Enchant Weapon (Grade C)
	blessedScrollWD int32 = 6575 // Blessed Scroll: Enchant Weapon (Grade D)
)

// Object ids of the enchant tests' items.
const (
	enchWeaponObj int32 = 0x30000201
	enchBodyObj   int32 = 0x30000202
	enchBootsObj  int32 = 0x30000203
	enchScrollObj int32 = 0x30000210
	enchArmorScr  int32 = 0x30000211
	enchBlessObj  int32 = 0x30000212
)

// newEnchantBrain is a D-grade fighter standing in a town (a gatekeeper
// spawned 100 away) with the score templates indexed, carrying items.
func newEnchantBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newScoreBrain(t, classHumanFight, 1, items...)
	b.cfg.Travel.gatekeepers = []gatekeeper{{npcID: 30006, at: Point{X: 100}}}
	return b
}

// scrollStack is n of scroll template id as object obj.
func scrollStack(obj, id, n int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: n, SubCategory: 5}
}

// TestEnchantPlan checks the scroll and the worn piece the next enchant
// takes: the weapon first, then the armor piece gaining most, a scroll of
// the piece's grade and kind, plain before blessed, below +3 (+4 for full
// armor) unless Risk is set.
func TestEnchantPlan(t *testing.T) {
	weapon := func(e uint16) Item { return enchanted(enchWeaponObj, artisansSword, partRHand, e) }
	body := func(e uint16) Item { return enchanted(enchBodyObj, salamanderMail, partFullArmor, e) }
	boots := func(e uint16) Item { return enchanted(enchBootsObj, leatherBoots, partFeet, e) }
	wd, ad := scrollStack(enchScrollObj, scrollWD, 5), scrollStack(enchArmorScr, scrollAD, 5)
	tests := []struct {
		name       string
		items      []Item
		risk       bool
		wantScroll int32
		wantTarget int32
	}{
		{name: "D weapon at +0", items: []Item{weapon(0), wd}, wantScroll: enchScrollObj, wantTarget: enchWeaponObj},
		{name: "D weapon at +2", items: []Item{weapon(2), wd}, wantScroll: enchScrollObj, wantTarget: enchWeaponObj},
		{name: "D weapon at +3: safe level reached", items: []Item{weapon(3), wd}},
		{name: "D weapon at +3 with Risk", items: []Item{weapon(3), wd}, risk: true, wantScroll: enchScrollObj, wantTarget: enchWeaponObj},
		{name: "weapon first", items: []Item{body(0), weapon(0), ad, wd}, wantScroll: enchScrollObj, wantTarget: enchWeaponObj},
		{name: "armor once the weapon is safe", items: []Item{body(0), weapon(3), ad, wd}, wantScroll: enchArmorScr, wantTarget: enchBodyObj},
		{name: "full armor at +3: one more is safe", items: []Item{body(3), ad}, wantScroll: enchArmorScr, wantTarget: enchBodyObj},
		{name: "full armor at +4", items: []Item{body(4), ad}},
		{name: "boots at +3", items: []Item{boots(3), ad}},
		{name: "equal gain: the body piece first", items: []Item{boots(0), body(0), ad}, wantScroll: enchArmorScr, wantTarget: enchBodyObj},
		{name: "a weapon scroll never goes on armor", items: []Item{body(0), wd}},
		{name: "an armor scroll never goes on the weapon", items: []Item{weapon(0), ad}},
		{name: "a C scroll never goes on a D weapon", items: []Item{weapon(0), scrollStack(enchScrollObj, scrollWC, 5)}},
		{
			name:       "plain before blessed",
			items:      []Item{weapon(0), scrollStack(enchBlessObj, blessedScrollWD, 1), wd},
			wantScroll: enchScrollObj, wantTarget: enchWeaponObj,
		},
		{
			name:       "blessed when no plain one",
			items:      []Item{weapon(0), scrollStack(enchBlessObj, blessedScrollWD, 1)},
			wantScroll: enchBlessObj, wantTarget: enchWeaponObj,
		},
		{name: "a carried weapon is not enchanted", items: []Item{carried(enchWeaponObj, artisansSword, partRHand), wd}},
		{name: "a no-grade weapon is not enchanted", items: []Item{wornAt(enchWeaponObj, longSword, partRHand), wd}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newEnchantBrain(t, tt.items...)
			b.persona.Enchant.Risk = tt.risk
			scroll, target, ok := b.enchantPlan()
			if ok != (tt.wantScroll != 0) || scroll.ObjectID != tt.wantScroll || target.ObjectID != tt.wantTarget {
				t.Fatalf("enchantPlan = %d on %d, %v; want %d on %d", scroll.ObjectID, target.ObjectID, ok, tt.wantScroll, tt.wantTarget)
			}
		})
	}
}

// TestEnchantFree checks where and when an enchant may start: in town,
// out of combat, standing, unattacked, with no store or trade open.
func TestEnchantFree(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *Brain)
		want bool
	}{
		{name: "in town", want: true},
		{name: "out of town", edit: func(b *Brain) { b.cfg.Travel.gatekeepers[0].at = Point{X: storeTownRadius + 200} }},
		{name: "no travel map", edit: func(b *Brain) { b.cfg.Travel = nil }},
		{name: "in combat", edit: func(b *Brain) { b.world.Self.InCombat = true }},
		{name: "sitting", edit: func(b *Brain) { b.world.Self.Sitting = true }},
		{name: "trade open", edit: func(b *Brain) { b.world.Trade.PartnerID = 7 }},
		{name: "dead", edit: func(b *Brain) { b.world.Self.HP = 0 }},
		{name: "private store", edit: func(b *Brain) { b.store.step = storeOpen }},
		{
			name: "attacked",
			edit: func(b *Brain) {
				b.world.Apply(NPCInfo{ObjectID: mobObj, NPCID: mob, Attackable: true, Pos: Point{X: 100}}, at(0))
				b.world.Attackers = map[int32]*Attacker{mobObj: {Damage: 5, Last: at(time.Second)}}
			},
		},
		{name: "gear UseItem awaited", edit: func(b *Brain) { b.gear.until = at(time.Minute) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newEnchantBrain(t)
			if tt.edit != nil {
				tt.edit(b)
			}
			b.towns = nil
			if got := b.enchantFree(at(time.Second)); got != tt.want {
				t.Fatalf("enchantFree = %v, want %v", got, tt.want)
			}
		})
	}
}

// enchantStep is one step of an enchant exchange: events in at at, then
// ruleEnchant's answer, sent when it is a packet.
type enchantTestStep struct {
	name    string
	at      time.Duration
	events  []Event
	want    actionKind
	payload []byte
	// none is set when the rule must not match.
	none bool
}

func runEnchantSteps(t *testing.T, b *Brain, steps []enchantTestStep) {
	t.Helper()
	for _, st := range steps {
		now := at(st.at)
		if err := b.handle(st.events, now); err != nil {
			t.Fatalf("%s: handle: %v", st.name, err)
		}
		a, ok := b.ruleEnchant(now)
		if ok == st.none || a.kind != st.want {
			t.Fatalf("%s: ruleEnchant = %d, %v; want %d, %v", st.name, a.kind, ok, st.want, !st.none)
		}
		if st.payload != nil && !bytes.Equal(a.payload, st.payload) {
			t.Fatalf("%s: payload % x, want % x", st.name, a.payload, st.payload)
		}
		if a.kind != actHold {
			if sent, err := b.send(a, now); err != nil || !sent {
				t.Fatalf("%s: send = %v, %v", st.name, sent, err)
			}
		}
	}
}

// weaponAt is the InventoryUpdate after a success on the D weapon: the
// weapon at level e and the scroll stack left with n.
func weaponAt(e uint16, n int32) InventoryUpdate {
	st := ItemModified
	if n == 0 {
		st = ItemRemoved
	}
	return InventoryUpdate{Changes: []ItemChange{
		{State: ItemModified, Item: enchanted(enchWeaponObj, artisansSword, partRHand, e)},
		{State: st, Item: scrollStack(enchScrollObj, scrollWD, n)},
	}}
}

// TestBrainEnchantToSafe enchants a worn D weapon from +0 with 4 D weapon
// scrolls: one exchange per answer, the next only once the inventory shows
// the new level (whichever of the update and EnchantResult comes first),
// ending at +3 with the fourth scroll kept.
func TestBrainEnchantToSafe(t *testing.T) {
	b := newEnchantBrain(t, enchanted(enchWeaponObj, artisansSword, partRHand, 0), scrollStack(enchScrollObj, scrollWD, 4))
	use, req := encodeUseItem(enchScrollObj), encodeRequestEnchantItem(enchWeaponObj)
	runEnchantSteps(t, b, []enchantTestStep{
		{name: "use the scroll", at: time.Second, want: actUseItem, payload: use},
		{name: "wait for the window", at: 2 * time.Second, want: actHold},
		{name: "request on the weapon", at: 3 * time.Second, events: []Event{ChooseInventoryItem{ScrollItemID: scrollWD}}, want: actEnchant, payload: req},
		{name: "wait for the result", at: 4 * time.Second, want: actHold},
		{name: "wait for the inventory", at: 5 * time.Second, events: []Event{EnchantResult{Code: EnchantSuccess}}, want: actHold},
		{name: "+1: the second scroll", at: 6 * time.Second, events: []Event{weaponAt(1, 3)}, want: actUseItem, payload: use},
		{name: "request again", at: 7 * time.Second, events: []Event{ChooseInventoryItem{ScrollItemID: scrollWD}}, want: actEnchant, payload: req},
		// The inventory update first, then the result: the next scroll
		// goes at once.
		{name: "+2: the third scroll", at: 8 * time.Second, events: []Event{weaponAt(2, 2), EnchantResult{Code: EnchantSuccess}}, want: actUseItem, payload: use},
		{name: "request a third time", at: 9 * time.Second, events: []Event{ChooseInventoryItem{ScrollItemID: scrollWD}}, want: actEnchant, payload: req},
		{name: "+3: done", at: 10 * time.Second, events: []Event{EnchantResult{Code: EnchantSuccess}, weaponAt(3, 1)}, none: true},
		{name: "still done", at: time.Hour, none: true},
	})
	if b.ench.step != enchantIdle || !b.ench.retryAt.IsZero() {
		t.Fatalf("exchange %+v, want it idle with no back-off", b.ench)
	}
	if _, junk := b.junk()[enchScrollObj]; junk {
		t.Fatal("the last D weapon scroll is junk, want it kept")
	}
}

// TestBrainEnchantBacksOff ends an exchange that cannot finish for
// RetryAfter: no window, no result, a result other than a success, or no
// inventory update showing the new level.
func TestBrainEnchantBacksOff(t *testing.T) {
	use, req := encodeUseItem(enchScrollObj), encodeRequestEnchantItem(enchWeaponObj)
	choose := []Event{ChooseInventoryItem{ScrollItemID: scrollWD}}
	gone := InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: enchanted(enchWeaponObj, artisansSword, partRHand, 0)}}}
	tests := []struct {
		name  string
		steps []enchantTestStep
		// gone is set when the target is gone, so no scroll is used again.
		gone bool
	}{
		{
			name: "no window",
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "another scroll's window is not ours", at: 2 * time.Second, events: []Event{ChooseInventoryItem{ScrollItemID: scrollAD}}, want: actHold},
				{name: "timeout", at: time.Second + enchantReplyTimeout, none: true},
			},
		},
		{
			name: "no result",
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "request", at: 2 * time.Second, events: choose, want: actEnchant, payload: req},
				{name: "timeout", at: 2*time.Second + enchantReplyTimeout, none: true},
			},
		},
		{
			name: "cancelled",
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "request", at: 2 * time.Second, events: choose, want: actEnchant, payload: req},
				{name: "cancelled", at: 3 * time.Second, events: []Event{EnchantResult{Code: EnchantCancelled}}, none: true},
			},
		},
		{
			// A higher rule's move cancelled the selection before the
			// request went: the request would go unanswered.
			name: "cancelled before the request",
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "cancelled", at: 2 * time.Second, events: []Event{ChooseInventoryItem{ScrollItemID: scrollWD}, EnchantResult{Code: EnchantCancelled}}, none: true},
			},
		},
		{
			// An older selection's cancel does not end this exchange.
			name: "an older cancel before the window",
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "older cancel", at: 2 * time.Second, events: []Event{EnchantResult{Code: EnchantCancelled}}, want: actHold},
				{name: "request", at: 3 * time.Second, events: choose, want: actEnchant, payload: req},
				{name: "cancelled", at: 4 * time.Second, events: []Event{EnchantResult{Code: EnchantCancelled}}, none: true},
			},
		},
		{
			// The request still goes, so the server clears the selection.
			name: "target gone",
			gone: true,
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "request on the gone target", at: 2 * time.Second, events: []Event{ChooseInventoryItem{ScrollItemID: scrollWD}, gone}, want: actEnchant, payload: req},
				{name: "cancelled", at: 3 * time.Second, events: []Event{EnchantResult{Code: EnchantCancelled}}, none: true},
			},
		},
		{
			// No request: the scroll would go past the planned level.
			name: "target level changed",
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "no request", at: 2 * time.Second, events: []Event{ChooseInventoryItem{ScrollItemID: scrollWD}, weaponAt(1, 4)}, none: true},
			},
		},
		{
			name: "no inventory update",
			steps: []enchantTestStep{
				{name: "use", at: time.Second, want: actUseItem, payload: use},
				{name: "request", at: 2 * time.Second, events: choose, want: actEnchant, payload: req},
				{name: "success", at: 3 * time.Second, events: []Event{EnchantResult{Code: EnchantSuccess}}, want: actHold},
				{name: "timeout", at: 3*time.Second + enchantReplyTimeout, none: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newEnchantBrain(t, enchanted(enchWeaponObj, artisansSword, partRHand, 0), scrollStack(enchScrollObj, scrollWD, 4))
			runEnchantSteps(t, b, tt.steps)
			last := at(tt.steps[len(tt.steps)-1].at)
			if want := last.Add(b.persona.Enchant.RetryAfter); !b.ench.retryAt.Equal(want) {
				t.Fatalf("retryAt %v, want %v", b.ench.retryAt, want)
			}
			if _, ok := b.ruleEnchant(last.Add(b.persona.Enchant.RetryAfter - time.Second)); ok {
				t.Fatal("ruleEnchant matched before RetryAfter")
			}
			if tt.gone {
				if _, ok := b.ruleEnchant(last.Add(b.persona.Enchant.RetryAfter)); ok {
					t.Fatal("ruleEnchant matched with the target gone")
				}
				return
			}
			if a, ok := b.ruleEnchant(last.Add(b.persona.Enchant.RetryAfter)); !ok || a.kind != actUseItem {
				t.Fatalf("ruleEnchant after RetryAfter = %d, %v; want the scroll used again", a.kind, ok)
			}
		})
	}
}

// TestBrainEnchantRuleOrder checks that the rule table runs "enchant" for
// a bot in town with a scroll for its weapon and nothing else to do.
func TestBrainEnchantRuleOrder(t *testing.T) {
	b := newEnchantBrain(t, enchanted(enchWeaponObj, artisansSword, partRHand, 0), scrollStack(enchScrollObj, scrollWD, 1))
	b.persona.Gear.Buy = false
	if rule, a := b.decide(at(time.Second)); rule != "enchant" || a.kind != actUseItem {
		t.Fatalf("decide = %q %d, want enchant UseItem", rule, a.kind)
	}
}

// TestKeepsScroll checks which enchant scrolls are not junk: one of a grade
// and kind the bot wears.
func TestKeepsScroll(t *testing.T) {
	weapon := enchanted(enchWeaponObj, artisansSword, partRHand, 3)
	body := enchanted(enchBodyObj, salamanderMail, partFullArmor, 0)
	tests := []struct {
		name  string
		items []Item
		id    int32
		want  bool
	}{
		{name: "D weapon scroll, D weapon worn", items: []Item{weapon}, id: scrollWD, want: true},
		{name: "blessed D weapon scroll, D weapon worn", items: []Item{weapon}, id: blessedScrollWD, want: true},
		{name: "C weapon scroll, D weapon worn", items: []Item{weapon}, id: scrollWC},
		{name: "D armor scroll, only a weapon worn", items: []Item{weapon}, id: scrollAD},
		{name: "D armor scroll, D armor worn", items: []Item{weapon, body}, id: scrollAD, want: true},
		{name: "D weapon scroll, the weapon only carried", items: []Item{carried(enchWeaponObj, artisansSword, partRHand)}, id: scrollWD},
		{name: "not a scroll", items: []Item{weapon}, id: artisansSword},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newEnchantBrain(t, tt.items...)
			if got := b.keepsScroll(tt.id); got != tt.want {
				t.Fatalf("keepsScroll(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestEnchantScrollsShipped checks the bot's scroll table against the
// shipped etc items when BOT_DATAPACK_XML names a real data/xml directory,
// and skips otherwise: each is an enchant scroll whose name says the
// table's grade (the files set no crystal_type on scrolls), weapon or
// armor, and blessed or not.
func TestEnchantScrollsShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	_, etc, _, err := datapack.LoadItems(filepath.Join(dir, "items"))
	if err != nil {
		t.Fatal(err)
	}
	for id, s := range enchantScrolls {
		e, ok := etc[id]
		grade := "(Grade " + string("?DCBAS"[s.grade]) + ")"
		if !ok || e.Kind != datapack.EtcEnchantScroll || !strings.HasSuffix(e.Name, grade) ||
			strings.Contains(e.Name, "Weapon") != s.weapon || strings.HasPrefix(e.Name, "Blessed") != s.blessed {
			t.Errorf("scroll %d = %+v, want an enchant scroll %+v", id, e, s)
		}
	}
}
