package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The Dusk Priestess's other validation pages and the stone exchange's
// (data/html/seven_signs, %objectId% filled in): the recorded frames the
// priest stop is tested on, as no live scenario reaches a validation week
// on demand (X19, owner approval on D43).
var (
	// duskPriest2b is dusk_priest_2b.htm: Dusk lost.
	duskPriest2b = `<html><body>Dusk Priestess:<br>We suffered a humiliating defeat in the recent competition.<br>` +
		`<font color="FF0000">[ The seal validation period is in effect. ]</font><br>` +
		priestLink("SevenSignsDesc 20", "What is the Seven Signs?") + priestLink("SevenSignsDesc 13", "Purchase Record of Seven Signs.") + `</body></html>`
	// duskPriest2d is dusk_priest_2d.htm: a tie.
	duskPriest2d = `<html><body>Dusk Priestess:<br>There was no decisive victory in the recent competition.<br>` +
		`<font color="FF0000">[ The seal validation period is in effect. ]</font><br>` +
		priestLink("SevenSignsDesc 20", "What is the Seven Signs?") + priestLink("SevenSignsDesc 13", "Purchase Record of Seven Signs.") + `</body></html>`
	// duskPriest3 is dusk_priest_3.htm: a member of Dawn.
	duskPriest3 = `<html><body>Dusk Priestess:<br>Your arrogant expression makes me want to puke!<br>Get away from me!</body></html>`
	// duskPriest4 is dusk_priest_4.htm: not a member, Dusk the winner.
	duskPriest4 = `<html><body>Dusk Priestess:<br>Long live the revolution! The brave Revolutionaries of Dusk have vanquished the arrogant pigs of dawn!<br>` +
		`<font color="FF0000">[ Seal validation period is underway. ]</font><br>` +
		priestLink("SevenSignsDesc 20", "What is the Seven Signs?") + priestLink("SevenSignsDesc 13", "Purchase Record of Seven Signs") + `</body></html>`
	// signs16Dusk is signs_16_dusk.htm, the stone exchange.
	signs16Dusk = `<html><body>Dusk Priestess:<br><font color="LEVEL">[ Exchange Seal Stones ]</font><br>` +
		priestLink("SevenSigns 17 1", "Exchange Blue Seal Stones.") + priestLink("SevenSigns 17 2", "Exchange Green Seal Stones.") +
		priestLink("SevenSigns 17 3", "Exchange Red Seal Stones.") + priestLink(priestAllStone, "Exchange all seal stones.") + `</body></html>`
	// signs18Dusk is signs_18_dusk.htm, the exchange's answer: no link.
	signs18Dusk = `<html><body>Dusk Priestess:<br><font color="LEVEL">[ Exchange Seal Stones for Ancient Adena ]</font><br>` +
		`I will exchange as many seal stones for Ancient Adena as you want.</body></html>`
)

// Multisell 500's CP rows as a priestess of an owned castle at a 10 % tax
// shows them (multisell/500.xml, server multisell.Entry.prepare): 240 and
// 600 Ancient Adena, and the tax on 200 and 500 adena as the last
// ingredient.
var (
	priestCPRow = MultiSellEntry{
		ID: 33, Stackable: true,
		Products:    []MultiSellItem{{ItemID: itemCPPotion, Type2: 5, Count: 1}},
		Ingredients: []MultiSellItem{{ItemID: itemAncientAdena, Type2: 4, Count: 240}, {ItemID: adenaID, Type2: 4, Count: 20}},
	}
	priestGreaterRow = MultiSellEntry{
		ID: 34, Stackable: true,
		Products:    []MultiSellItem{{ItemID: itemGreaterCPPotion, Type2: 5, Count: 1}},
		Ingredients: []MultiSellItem{{ItemID: itemAncientAdena, Type2: 4, Count: 600}, {ItemID: adenaID, Type2: 4, Count: 50}},
	}
	priestSoulRow = MultiSellEntry{
		ID: 1, Stackable: true,
		Products:    []MultiSellItem{{ItemID: 1835, Type2: 5, Count: 100}},
		Ingredients: []MultiSellItem{{ItemID: itemAncientAdena, Type2: 4, Count: 10}},
	}
	priestShopPage = MultiSellList{ListID: priestListID, Page: 1, Finished: true, PageSize: 40, Entries: []MultiSellEntry{priestSoulRow, priestCPRow, priestGreaterRow}}
)

// redStones, ancientAA and priestAdena are stacks of the priest stop's
// items.
func redStones(n int32) Item {
	return Item{ObjectID: 801, TemplateID: itemRedSealStone, Count: n}
}

func ancientAA(n int32) Item {
	return Item{ObjectID: 802, TemplateID: itemAncientAdena, Count: n}
}

func priestAdena(n int32) Item {
	return Item{ObjectID: 803, TemplateID: adenaID, Count: n}
}

// priestBrain is newSignsBrain stocking 40 CP Potions (refilled below half
// of it), the priestess in sight, carrying items.
func priestBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newSignsBrain(t)
	b.persona.PvP.CPStock, b.persona.Supply.Refill, b.persona.Supply.Extra = 40, 50, nil
	b.persona.Shop.KeepAdena = 0
	priestInSight(b)
	carrying(b, items...)
	return b
}

// validated makes the errand know the bot a member of Dusk, the winner of
// the validation, as a 2a read at t0 told it.
func validated(s *signsErrand) {
	s.member, s.period, s.readAt = true, signsValidation, t0
}

func choose(entry, amount int32) []byte {
	return botwire.EncodeMultiSellChoose(priestListID, entry, amount)
}

func changed(it Item) Event {
	return InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: it}}}
}

// TestSevenSignsPriestTrip walks a member holding 30 Red Seal Stones and
// 2400 Ancient Adena, no CP potion, through the priest stop: the stone
// exchange, the talk again, multisell 500, then 4 Greater CP Potions
// (2700 Ancient Adena buys 4 of the 10 the stock lacks) and 1 CP Potion for
// the 300 left, each after the answer to the one before and never within
// multiSellGap of it; then the stop is over and not due again.
func TestSevenSignsPriestTrip(t *testing.T) {
	b := priestBrain(t, redStones(30), ancientAA(2400), priestAdena(10000))
	page := func(html string) []Event { return []Event{NpcHtmlMessage{ObjectID: priestObj, HTML: html}} }
	runTrainerSteps(t, b, []trainerStep{
		{name: "talk", at: time.Second, wantRule: "signs", wantKind: actTalk},
		{name: "exchange stones", at: 2 * time.Second, events: page(duskPriest2a), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(priestExchange)},
		{name: "all of them", at: 3 * time.Second, events: page(signs16Dusk), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(priestAllStone)},
		{
			name: "talk again", at: 4 * time.Second, wantRule: "signs", wantKind: actTalk,
			events: []Event{
				NpcHtmlMessage{ObjectID: priestObj, HTML: signs18Dusk},
				InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: redStones(0)}, {State: ItemModified, Item: ancientAA(2700)}}},
			},
		},
		{name: "wait for the greeting", at: 5 * time.Second, wantRule: "signs"},
		{name: "open the shop", at: 6 * time.Second, events: page(duskPriest2c), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(priestShop)},
		{name: "wait for the list", at: 6500 * time.Millisecond, wantRule: "signs"},
		{name: "Greater CP", at: 7 * time.Second, events: []Event{priestShopPage}, wantRule: "signs", wantKind: actMultiSellChoose, wantPayload: choose(34, 4)},
		{name: "one per answer", at: 7200 * time.Millisecond, wantRule: "signs"},
	})
	s := &b.signs
	now := at(7300 * time.Millisecond)
	if err := b.handle([]Event{changed(cpOf(itemGreaterCPPotion, 4)), changed(ancientAA(300)), changed(priestAdena(9800))}, now); err != nil {
		t.Fatal(err)
	}
	rule, a := b.decide(now)
	if rule != "signs" || a.kind != actMultiSellChoose || !bytes.Equal(a.payload, choose(33, 1)) {
		t.Fatalf("after the Greater CP: decide = %q %d % x, want the CP Potion row for 1", rule, a.kind, a.payload)
	}
	if sent, err := b.send(a, now); err != nil || sent {
		t.Fatalf("a choice 300 ms after the last: sent %v, %v, want it held by the multisell gate", sent, err)
	}
	// The held choice is decided again, its row not passed.
	now = at(7500 * time.Millisecond)
	if rule, a = b.decide(now); rule != "signs" || a.kind != actMultiSellChoose || !bytes.Equal(a.payload, choose(33, 1)) {
		t.Fatalf("after the held choice: decide = %q %d % x, want the CP Potion row for 1 again", rule, a.kind, a.payload)
	}
	if sent, err := b.send(a, now); err != nil || !sent {
		t.Fatalf("a choice multiSellGap after the last: sent %v, %v", sent, err)
	}
	if err := b.handle([]Event{changed(cpOf(itemCPPotion, 1)), changed(ancientAA(60))}, at(8*time.Second)); err != nil {
		t.Fatal(err)
	}
	if rule, _ := b.decide(at(9 * time.Second)); rule == "signs" || s.v.active() || s.chooses != 0 {
		t.Fatalf("after two choices: rule %q visit %+v, want the stop over", rule, s.v)
	}
	if b.priestDue(at(time.Hour)) || b.startSigns(at(time.Hour)) {
		t.Fatal("a priest stop due again within signsFresh")
	}
}

// TestSevenSignsPriestPages checks which validation greeting the stop goes
// on from, for a short member holding stones: only the pages linking
// multisell 500 and the stone exchange (2a, 2c); any other ends the visit
// with no link, and its period ends the stop until a 2a or 2c is read again.
func TestSevenSignsPriestPages(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		items   []Item
		wantCmd string
	}{
		{name: "2a: exchange the stones", html: duskPriest2a, items: []Item{redStones(3)}, wantCmd: priestExchange},
		{name: "2c: exchange the stones", html: duskPriest2c, items: []Item{redStones(3)}, wantCmd: priestExchange},
		{name: "2c, Ancient Adena only: the shop", html: duskPriest2c, items: []Item{ancientAA(600)}, wantCmd: priestShop},
		{name: "2a, neither: nothing", html: duskPriest2a, items: []Item{priestAdena(1000)}},
		{name: "2a, the stock full: nothing", html: duskPriest2a, items: []Item{redStones(3), cpOf(itemGreaterCPPotion, 10)}},
		{name: "2b, Dusk lost", html: duskPriest2b, items: []Item{redStones(3)}},
		{name: "2d, a tie", html: duskPriest2d, items: []Item{redStones(3)}},
		{name: "3, a member of Dawn", html: duskPriest3, items: []Item{redStones(3)}},
		{name: "4, not a member", html: duskPriest4, items: []Item{redStones(3)}},
		{name: "a validation page without the shop link", html: strings2a(), items: []Item{redStones(3)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := priestBrain(t, tt.items...)
			s := &b.signs
			s.v = npcVisit{npcID: 31085, at: Point{X: 200}, obj: priestObj}
			b.handleOne(t, NpcHtmlMessage{ObjectID: priestObj, HTML: tt.html}, t0)
			a, ok := b.ruleSigns(t0)
			switch {
			case tt.wantCmd == "" && (ok || s.v.active()):
				t.Fatalf("ruleSigns = %d %v, visit %+v, want the visit over", a.kind, ok, s.v)
			case tt.wantCmd != "" && (!ok || a.kind != actBypass || !bytes.Equal(a.payload, priestBypass(tt.wantCmd))):
				t.Fatalf("ruleSigns = %d %v % x, want the bypass %q", a.kind, ok, a.payload, tt.wantCmd)
			}
			if tt.wantCmd == "" && s.period != signsValidation && b.priestDue(at(time.Minute)) {
				t.Fatal("a priest stop due after a page other than 2a or 2c")
			}
		})
	}
}

// strings2a is dusk_priest_2a's text with its multisell link left out.
func strings2a() string {
	return `<html><body>Dusk Priestess:<br>The Revolutionaries of Dusk have vanquished the arrogant Lords of Dawn!<br>[ The seal validation period is underway. ]<br>` +
		priestLink("SevenSigns 16", "Exchange seal stones.") + `</body></html>`
}

// TestSevenSignsPriestAmount checks the units a CP row buys: what the
// stock lacks in whole units, bounded by the Ancient Adena, the adena past
// the reserve (the tax ingredient), the weight and the server's caps.
func TestSevenSignsPriestAmount(t *testing.T) {
	tests := []struct {
		name   string
		items  []Item
		setup  func(b *Brain)
		row    struct{ item, units int32 }
		wantID int32
		want   int32
	}{
		{name: "Greater: the lack in whole Greater", items: []Item{ancientAA(100000), priestAdena(100000)}, row: priestRows[0], wantID: 34, want: 10},
		{name: "Greater: bounded by Ancient Adena", items: []Item{ancientAA(1799), priestAdena(100000)}, row: priestRows[0], wantID: 34, want: 2},
		{name: "Greater: bounded by the tax", items: []Item{ancientAA(100000), priestAdena(149)}, row: priestRows[0], wantID: 34, want: 2},
		{
			name: "Greater: the adena past the reserve", items: []Item{ancientAA(100000), priestAdena(1149)}, row: priestRows[0], wantID: 34, want: 2,
			setup: func(b *Brain) { b.persona.Shop.KeepAdena = 1000 },
		},
		{name: "plain: what is left", items: []Item{ancientAA(100000), priestAdena(100000), cpOf(itemGreaterCPPotion, 9)}, row: priestRows[1], wantID: 33, want: 4},
		{name: "plain: above the refill threshold still", items: []Item{ancientAA(100000), priestAdena(100000), cpOf(itemCPPotion, 37)}, row: priestRows[1], wantID: 33, want: 3},
		{name: "Greater: less than one Greater short", items: []Item{ancientAA(100000), priestAdena(100000), cpOf(itemCPPotion, 37)}, row: priestRows[0], wantID: 34},
		{name: "no Ancient Adena", items: []Item{priestAdena(100000)}, row: priestRows[1], wantID: 33},
		{
			name: "weight under BuyLoad", items: []Item{ancientAA(100000), priestAdena(100000)}, row: priestRows[0], wantID: 34, want: 3,
			setup: func(b *Brain) {
				b.persona.Supply.BuyLoad = 50
				b.world.Self.Load, b.world.Self.MaxLoad = 700, 2000
				b.cfg.Travel.weights = map[int32]int32{itemCPPotion: 25, itemGreaterCPPotion: 100}
			},
		},
		{
			name: "weight under the limit", items: []Item{ancientAA(100000), priestAdena(100000)}, row: priestRows[1], wantID: 33, want: 8,
			setup: func(b *Brain) {
				b.persona.Supply.BuyLoad = 0
				b.world.Self.Load, b.world.Self.MaxLoad = 1800, 2000
				b.cfg.Travel.weights = map[int32]int32{itemCPPotion: 25, itemGreaterCPPotion: 100}
			},
		},
		{
			name: "the server's cap", items: []Item{ancientAA(1 << 30), priestAdena(1 << 30)}, row: priestRows[1], wantID: 33, want: multiSellMaxAmount,
			setup: func(b *Brain) { b.persona.PvP.CPStock = 20000 },
		},
		{
			name: "no such row", items: []Item{ancientAA(100000), priestAdena(100000)}, row: priestRows[0],
			setup: func(b *Brain) { b.signs.list = []MultiSellEntry{priestCPRow} },
		},
		{
			name: "a non-stackable row: one", items: []Item{ancientAA(100000), priestAdena(100000)}, row: priestRows[1], wantID: 33, want: 1,
			setup: func(b *Brain) {
				r := priestCPRow
				r.Stackable = false
				b.signs.list = []MultiSellEntry{r}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := priestBrain(t, tt.items...)
			b.signs.list = priestShopPage.Entries
			if tt.setup != nil {
				tt.setup(b)
			}
			if id, n := b.priestAmount(tt.row.item, tt.row.units); n != tt.want || (n > 0 && id != tt.wantID) {
				t.Fatalf("priestAmount = entry %d, %d, want entry %d, %d", id, n, tt.wantID, tt.want)
			}
		})
	}
}

// TestSevenSignsPriestSilentDrop checks that a MultiSellChoose answered by
// no InventoryUpdate within trainerReplyTimeout (the server dropped the
// list in silence, or refused with a SystemMessage) ends the stop: no
// second choice, no stop due again within signsFresh. An InventoryUpdate
// of another item is no answer.
func TestSevenSignsPriestSilentDrop(t *testing.T) {
	b := priestBrain(t, ancientAA(6000), priestAdena(10000))
	s := &b.signs
	validated(s)
	s.v = npcVisit{npcID: 31085, at: Point{X: 200}, obj: priestObj}
	b.handleOne(t, NpcHtmlMessage{ObjectID: priestObj, HTML: duskPriest2a}, t0)
	sendSigns := func(now time.Time, want actionKind) {
		t.Helper()
		a, ok := b.ruleSigns(now)
		if !ok || a.kind != want {
			t.Fatalf("ruleSigns = %d %v, want %d", a.kind, ok, want)
		}
		if sent, err := b.send(a, now); err != nil || !sent {
			t.Fatalf("send = %v, %v", sent, err)
		}
	}
	sendSigns(at(time.Second), actBypass)
	b.handleOne(t, priestShopPage, at(2*time.Second))
	sendSigns(at(2*time.Second), actMultiSellChoose)
	b.handleOne(t, changed(priestAdena(9000)), at(3*time.Second))
	if a, ok := b.ruleSigns(at(6 * time.Second)); !ok || a.kind != actHold {
		t.Fatalf("within the reply timeout: ruleSigns = %d %v, want a hold", a.kind, ok)
	}
	if _, ok := b.ruleSigns(at(8 * time.Second)); ok || s.v.active() || s.chooses != 0 {
		t.Fatalf("after the reply timeout: visit %+v, want the stop over", s.v)
	}
	if b.priestDue(at(time.Hour)) {
		t.Fatal("a priest stop due again within signsFresh after a silent drop")
	}
}

// TestSevenSignsPriestDue checks when a visit starts for a priest stop
// before the errand's rest is over: a member in a validation Dusk won,
// short of CP potions, holding stones or Ancient Adena; never in another
// period, at a full stock, or empty-handed.
func TestSevenSignsPriestDue(t *testing.T) {
	tests := []struct {
		name  string
		setup func(s *signsErrand)
		items []Item
		want  bool
	}{
		{name: "stones", setup: validated, items: []Item{redStones(1)}, want: true},
		{name: "Ancient Adena", setup: validated, items: []Item{ancientAA(1)}, want: true},
		{name: "empty-handed", setup: validated, items: []Item{priestAdena(1000)}},
		{name: "stock above the refill threshold", setup: validated, items: []Item{redStones(1), cpOf(itemCPPotion, 30)}},
		{name: "the competition", setup: competition, items: []Item{redStones(1)}},
		{name: "another period", setup: func(s *signsErrand) { validated(s); s.period = signsOther }, items: []Item{redStones(1)}},
		{name: "not a member", setup: func(s *signsErrand) { validated(s); s.member = false }, items: []Item{redStones(1)}},
		{name: "a stop lately", setup: func(s *signsErrand) { validated(s); s.stopUntil = at(time.Hour) }, items: []Item{redStones(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := priestBrain(t, tt.items...)
			s := &b.signs
			tt.setup(s)
			s.restUntil = at(signsFresh)
			if got := b.startSigns(at(time.Minute)); got != tt.want {
				t.Fatalf("startSigns = %v, want %v", got, tt.want)
			}
		})
	}
}
