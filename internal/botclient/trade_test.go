package botclient

import (
	"bytes"
	"errors"
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

const (
	goodsID = int32(0x40000001) // the bot's goods for sale
	potID   = int32(0x40000002) // an unpriced item
	coinID  = int32(0x40000003) // the partner's adena
	bladeID = int32(0x40000004) // a blade the partner offers
)

var (
	goodsRow = TradeItem{ObjectID: goodsID, TemplateID: 1864, Count: 10, Category: 4, SubCategory: 5}
	potRow   = TradeItem{ObjectID: potID, TemplateID: 1061, Count: 3, Category: 4, SubCategory: 5}
	bladeRow = TradeItem{ObjectID: bladeID, TemplateID: 1, Count: 1, CustomType1: 1, BodySlot: 128, Enchant: 3, CustomType2: 2}
)

func adenaRow(n int32) TradeItem {
	return TradeItem{ObjectID: coinID, TemplateID: adenaID, Count: n, Category: 4, SubCategory: 4}
}

// tradeRow is a trade item row's fields in the server's order
// (writeTradeItem).
func tradeRow(it TradeItem) []any {
	return []any{
		it.Category, it.ObjectID, it.TemplateID, it.Count, it.SubCategory, it.CustomType1,
		it.BodySlot, it.Enchant, it.CustomType2, uint16(0),
	}
}

func TestDecodeTradeEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"SendTradeRequest bytes", []byte{0x5e, 2, 0, 0, 0x20}, SendTradeRequest{SenderID: pcID}},
		{"SendTradeDone success", packet(opGameSendTradeDone, 1), SendTradeDone{Success: true}},
		{"SendTradeDone cancelled", packet(opGameSendTradeDone, 0), SendTradeDone{}},
		{"TradePressOwnOk", packet(opGameTradePressOwnOk), TradePressOwnOk{}},
		{"TradePressOtherOk", packet(opGameTradePressOtherOk), TradePressOtherOk{}},
		{
			"TradeStart", packet(opGameTradeStart, append(append([]any{pcID, uint16(2)}, tradeRow(goodsRow)...), tradeRow(bladeRow)...)...),
			TradeStart{PartnerID: pcID, Items: []TradeItem{goodsRow, bladeRow}},
		},
		{"TradeStart empty", packet(opGameTradeStart, pcID, uint16(0)), TradeStart{PartnerID: pcID}},
		{"TradeOwnAdd", packet(opGameTradeOwnAdd, append([]any{uint16(1)}, tradeRow(goodsRow)...)...), TradeOwnAdd{Item: goodsRow}},
		{"TradeOtherAdd", packet(opGameTradeOtherAdd, append([]any{uint16(1)}, tradeRow(adenaRow(500))...)...), TradeOtherAdd{Item: adenaRow(500)}},
		{
			"TradeUpdate", packet(opGameTradeItemUpdate, append([]any{uint16(1), uint16(3)}, tradeRow(goodsRow)...)...),
			TradeItemUpdate{Items: []TradeAvail{{Mode: 3, Item: goodsRow}}},
		},
		{
			"TradeItemUpdate", packet(opGameTradeItemUpdate, append(append([]any{uint16(2), uint16(3)}, tradeRow(goodsRow)...), append([]any{uint16(2)}, tradeRow(bladeRow)...)...)...),
			TradeItemUpdate{Items: []TradeAvail{{Mode: 3, Item: goodsRow}, {Mode: 2, Item: bladeRow}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v", ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeTradeEventsRejects(t *testing.T) {
	start := packet(opGameTradeStart, append([]any{pcID, uint16(1)}, tradeRow(goodsRow)...)...)
	add := packet(opGameTradeOtherAdd, append([]any{uint16(1)}, tradeRow(goodsRow)...)...)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"SendTradeRequest short", []byte{opGameSendTradeRequest, 1, 0}, wire.ErrShortPacket},
		{"TradeStart short of its count", packet(opGameTradeStart, append([]any{pcID, uint16(2)}, tradeRow(goodsRow)...)...), wire.ErrShortPacket},
		{"TradeStart trailing", append(start, 0), errTrailingBytes},
		{"TradeOtherAdd without its zero", add[:len(add)-2], wire.ErrShortPacket},
		{"TradeOtherAdd trailing", append(add, 0), errTrailingBytes},
		{"TradeItemUpdate short row", packet(opGameTradeItemUpdate, uint16(1), uint16(3), uint16(4)), wire.ErrShortPacket},
		{"TradePressOtherOk trailing", packet(opGameTradePressOtherOk, 0), errTrailingBytes},
		{"SendTradeDone short", packet(opGameSendTradeDone, uint16(1)), wire.ErrShortPacket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTradeEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"TradeRequest", encodeTradeRequest(pcID), []byte{0x15, 2, 0, 0, 0x20}},
		{"AnswerTradeRequest accept", encodeAnswerTradeRequest(true), []byte{0x44, 1, 0, 0, 0}},
		{"AnswerTradeRequest decline", encodeAnswerTradeRequest(false), []byte{0x44, 0, 0, 0, 0}},
		{"AddTradeItem", encodeAddTradeItem(goodsID, 10), []byte{0x16, 0, 0, 0, 0, 1, 0, 0, 0x40, 10, 0, 0, 0}},
		{"TradeDone confirm", encodeTradeDone(true), []byte{0x17, 1, 0, 0, 0}},
		{"TradeDone cancel", encodeTradeDone(false), []byte{0x17, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

// TestWorldTrade applies a trade's life, from the request to the settled
// trade, and checks the prompt and the window after each step.
func TestWorldTrade(t *testing.T) {
	w := newTestWorld(t)
	steps := []struct {
		name   string
		ev     Event
		prompt TradeInvite
		trade  Trade
	}{
		{"request", SendTradeRequest{SenderID: pcID}, TradeInvite{Open: true, PartnerID: pcID, At: at(1)}, Trade{}},
		{"add before a window", TradeOwnAdd{Item: goodsRow}, TradeInvite{Open: true, PartnerID: pcID, At: at(1)}, Trade{}},
		{"start", TradeStart{PartnerID: pcID, Items: []TradeItem{goodsRow}}, TradeInvite{}, Trade{PartnerID: pcID, Offerable: []TradeItem{goodsRow}, At: at(2)}},
		{"own add", TradeOwnAdd{Item: goodsRow}, TradeInvite{}, Trade{PartnerID: pcID, Offerable: []TradeItem{goodsRow}, Own: []TradeItem{goodsRow}, At: at(2)}},
		{"availability ignored", TradeItemUpdate{Items: []TradeAvail{{Mode: 3, Item: goodsRow}}}, TradeInvite{}, Trade{PartnerID: pcID, Offerable: []TradeItem{goodsRow}, Own: []TradeItem{goodsRow}, At: at(2)}},
		{
			"other add",
			TradeOtherAdd{Item: adenaRow(100)},
			TradeInvite{},
			Trade{PartnerID: pcID, Offerable: []TradeItem{goodsRow}, Own: []TradeItem{goodsRow}, Other: []TradeItem{adenaRow(100)}, At: at(2)},
		},
		{
			"other ok",
			TradePressOtherOk{},
			TradeInvite{},
			Trade{PartnerID: pcID, Offerable: []TradeItem{goodsRow}, Own: []TradeItem{goodsRow}, Other: []TradeItem{adenaRow(100)}, OtherOk: true, At: at(2)},
		},
		{
			"own ok",
			TradePressOwnOk{},
			TradeInvite{},
			Trade{PartnerID: pcID, Offerable: []TradeItem{goodsRow}, Own: []TradeItem{goodsRow}, Other: []TradeItem{adenaRow(100)}, OwnOk: true, OtherOk: true, At: at(2)},
		},
		{"done", SendTradeDone{Success: true}, TradeInvite{}, Trade{}},
		{"ok after the end", TradePressOtherOk{}, TradeInvite{}, Trade{}},
	}
	for i, s := range steps {
		w.Apply(s.ev, at(time.Duration(min(i+1, 2))))
		if w.Prompts.Trade != s.prompt {
			t.Fatalf("%s: prompt = %+v, want %+v", s.name, w.Prompts.Trade, s.prompt)
		}
		if !reflect.DeepEqual(w.Trade, s.trade) {
			t.Fatalf("%s: trade = %+v, want %+v", s.name, w.Trade, s.trade)
		}
	}
}

// tradeBrain is a test brain selling goodsRow at 50 adena a unit, carrying
// it and potRow, with the player pcID standing 100 units away.
func tradeBrain(t *testing.T, edit func(*Persona)) *Brain {
	t.Helper()
	p := Persona{Trade: defaultTradePolicy()}
	p.Trade.Prices = map[int32]int32{goodsRow.TemplateID: 50}
	if edit != nil {
		edit(&p)
	}
	b, _ := newTestBrain(t, p)
	b.world.Apply(ItemList{Items: []Item{
		{ObjectID: goodsID, TemplateID: goodsRow.TemplateID, Count: 10, Category: 4, SubCategory: 5},
		{ObjectID: potID, TemplateID: potRow.TemplateID, Count: 3, Category: 4, SubCategory: 5},
	}}, t0)
	near := Point{Y: 100}
	b.world.Objects[pcID].move = motion{from: near, to: near}
	return b
}

// openTrade opens a trade with pcID at t0, offering goodsRow and potRow,
// then applies evs.
func openTrade(b *Brain, evs ...Event) {
	b.world.Apply(TradeStart{PartnerID: pcID, Items: []TradeItem{potRow, goodsRow}}, t0)
	for _, ev := range evs {
		b.world.Apply(ev, t0)
	}
}

func TestTradeRule(t *testing.T) {
	now := at(2 * time.Second)
	gm := func(p *Persona) { p.Trade.Exempt = func(_ *World, id int32) bool { return id == pcID } }
	request := func(b *Brain) { b.world.Apply(SendTradeRequest{SenderID: pcID}, at(time.Second)) }
	paid := func(b *Brain) {
		openTrade(b, TradeOwnAdd{Item: goodsRow}, TradeOtherAdd{Item: adenaRow(300)}, TradeOtherAdd{Item: adenaRow(200)})
	}
	accept, decline := encodeAnswerTradeRequest(true), encodeAnswerTradeRequest(false)
	confirm, cancel := encodeTradeDone(true), encodeTradeDone(false)
	tests := []struct {
		name    string
		persona func(*Persona)
		setup   func(b *Brain)
		// want is the packet sent; nil holds. wantRule "" is no match.
		want     []byte
		wantRule string
	}{
		{name: "no request", wantRule: ""},
		{name: "request with goods accepted", setup: request, want: accept, wantRule: "prompt"},
		{name: "nothing priced declines", persona: func(p *Persona) { p.Trade.Prices = nil }, setup: request, want: decline, wantRule: "prompt"},
		{
			name: "equipped goods declines", setup: func(b *Brain) {
				it := b.world.Inventory.Items[goodsID]
				it.Equipped = true
				b.world.Inventory.Items[goodsID] = it
				request(b)
			}, want: decline, wantRule: "prompt",
		},
		{
			name: "quest goods declines", setup: func(b *Brain) {
				it := b.world.Inventory.Items[goodsID]
				it.SubCategory = subCategoryQuest
				b.world.Inventory.Items[goodsID] = it
				request(b)
			}, want: decline, wantRule: "prompt",
		},
		{name: "store mode declines", setup: func(b *Brain) { b.world.Self.StoreType = 1; request(b) }, want: decline, wantRule: "prompt"},
		{name: "store persona declines", persona: func(p *Persona) { p.Store = true }, setup: request, want: decline, wantRule: "prompt"},
		{name: "in combat declines", setup: func(b *Brain) { b.world.Self.InCombat = true; request(b) }, want: decline, wantRule: "prompt"},
		{name: "partner out of sight declines", setup: func(b *Brain) { delete(b.world.Objects, pcID); request(b) }, want: decline, wantRule: "prompt"},
		{name: "partner traded recently declines", setup: func(b *Brain) { b.tradedAt[pcID] = t0; request(b) }, want: decline, wantRule: "prompt"},
		{name: "partner traded long ago accepted", setup: func(b *Brain) { b.tradedAt[pcID] = now.Add(-time.Minute); request(b) }, want: accept, wantRule: "prompt"},
		{
			name: "exempt partner accepted without goods or wait", persona: func(p *Persona) { gm(p); p.Trade.Prices = nil },
			setup: func(b *Brain) { b.tradedAt[pcID] = t0; request(b) }, want: accept, wantRule: "prompt",
		},
		{name: "expired request", setup: func(b *Brain) { b.world.Apply(SendTradeRequest{SenderID: pcID}, now.Add(-tradeRequestTimeout)) }, wantRule: ""},
		{name: "open trade offers the goods", setup: func(b *Brain) { openTrade(b) }, want: encodeAddTradeItem(goodsID, 10), wantRule: "prompt"},
		{name: "add sent waits for its echo", setup: func(b *Brain) { openTrade(b); b.world.Trade.added = true }, wantRule: "prompt"},
		{name: "goods gone cancels", setup: func(b *Brain) { delete(b.world.Inventory.Items, goodsID); openTrade(b) }, want: cancel, wantRule: "prompt"},
		{name: "offers locked before the bot's cancels", setup: func(b *Brain) { openTrade(b, TradePressOtherOk{}) }, want: cancel, wantRule: "prompt"},
		{name: "short of the price waits", setup: func(b *Brain) { openTrade(b, TradeOwnAdd{Item: goodsRow}, TradeOtherAdd{Item: adenaRow(499)}) }, wantRule: "prompt"},
		{
			name: "short of the price at the partner's confirm cancels",
			setup: func(b *Brain) {
				openTrade(b, TradeOwnAdd{Item: goodsRow}, TradeOtherAdd{Item: adenaRow(499)}, TradePressOtherOk{})
			}, want: cancel, wantRule: "prompt",
		},
		{name: "paid within range confirms", setup: paid, want: confirm, wantRule: "prompt"},
		{
			name: "paid out of range waits", setup: func(b *Brain) {
				paid(b)
				far := Point{Y: tradeRange - tradeRangeMargin + 1}
				b.world.Objects[pcID].move = motion{from: far, to: far}
			}, wantRule: "prompt",
		},
		{name: "paid, partner out of sight waits", setup: func(b *Brain) { paid(b); delete(b.world.Objects, pcID) }, wantRule: "prompt"},
		{name: "confirm sent waits", setup: func(b *Brain) { paid(b); b.world.Trade.confirmed = true }, wantRule: "prompt"},
		{name: "item offered cancels", setup: func(b *Brain) { paid(b); b.world.Apply(TradeOtherAdd{Item: bladeRow}, t0) }, want: cancel, wantRule: "prompt"},
		{name: "in combat cancels", setup: func(b *Brain) { paid(b); b.world.Self.InCombat = true }, want: cancel, wantRule: "prompt"},
		{name: "timed out cancels", setup: func(b *Brain) { openTrade(b); b.world.Trade.At = now.Add(-time.Minute) }, want: cancel, wantRule: "prompt"},
		{name: "cancel sent waits", setup: func(b *Brain) { openTrade(b); b.world.Trade.cancelAt = now }, wantRule: "prompt"},
		{
			name: "cancel unanswered is forgotten",
			setup: func(b *Brain) {
				openTrade(b)
				b.world.Trade.cancelAt = now.Add(-tradeForget)
			}, wantRule: "prompt",
		},
		{name: "exempt waits for the partner's confirm", persona: gm, setup: func(b *Brain) { openTrade(b) }, wantRule: "prompt"},
		{
			name: "exempt gift confirmed", persona: gm,
			setup: func(b *Brain) {
				b.world.Self.Load, b.world.Self.MaxLoad = 50, 100
				openTrade(b, TradeOtherAdd{Item: bladeRow}, TradePressOtherOk{})
			}, want: confirm, wantRule: "prompt",
		},
		{
			name: "exempt gift over the load cancels", persona: gm,
			setup: func(b *Brain) {
				b.world.Self.Load, b.world.Self.MaxLoad = 51, 100
				openTrade(b, TradeOtherAdd{Item: bladeRow}, TradePressOtherOk{})
			}, want: cancel, wantRule: "prompt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tradeBrain(t, tt.persona)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(now)
			if rule != tt.wantRule || !bytes.Equal(a.payload, tt.want) {
				t.Fatalf("decide = rule %q, % x; want rule %q, % x", rule, a.payload, tt.wantRule, tt.want)
			}
		})
	}
}

// TestTradeRuleForgetsCancelled checks that a cancel the server never
// answered stops holding the bot once tradeForget passed.
func TestTradeRuleForgetsCancelled(t *testing.T) {
	b := tradeBrain(t, nil)
	openTrade(b)
	b.world.Trade.cancelAt = t0
	if b.decide(at(tradeForget - time.Millisecond)); !b.world.Trade.Open() {
		t.Fatal("trade forgotten before tradeForget")
	}
	if b.decide(at(tradeForget)); b.world.Trade.Open() {
		t.Fatal("trade still open after tradeForget")
	}
}

// TestBrainTrade plays a sale through the brain loop: the request is
// accepted, the goods offered, the trade confirmed once paid, and the same
// partner's next request inside TradePolicy.Every declined.
func TestBrainTrade(t *testing.T) {
	b := tradeBrain(t, nil)
	out := b.out.(*recordSender)
	steps := []struct {
		name string
		evs  []Event
		want []byte
	}{
		{"request", []Event{SendTradeRequest{SenderID: pcID}}, encodeAnswerTradeRequest(true)},
		{"window opens", []Event{TradeStart{PartnerID: pcID, Items: []TradeItem{potRow, goodsRow}}}, encodeAddTradeItem(goodsID, 10)},
		{"partner pays", []Event{TradeOwnAdd{Item: goodsRow}, TradeOtherAdd{Item: adenaRow(500)}}, encodeTradeDone(true)},
		{"partner confirms", []Event{TradePressOwnOk{}, TradePressOtherOk{}}, nil},
		{"settled", []Event{SendTradeDone{Success: true}}, nil},
		{"next request", []Event{SendTradeRequest{SenderID: pcID}}, encodeAnswerTradeRequest(false)},
	}
	now := t0
	for _, s := range steps {
		now = now.Add(5 * time.Second)
		if err := b.handle(s.evs, now); err != nil {
			t.Fatal(err)
		}
		sent := len(out.frames)
		for _, w := range []time.Time{b.decisionAt(), b.decisionAt().Add(time.Second)} {
			if err := b.wake(w); err != nil {
				t.Fatal(err)
			}
			now = w
		}
		var got []byte
		for _, f := range out.frames[sent:] {
			if f[0] == opGameAnswerTradeRequest || f[0] == opGameAddTradeItem || f[0] == opGameTradeDone {
				if got != nil {
					t.Fatalf("%s: sent % x, then % x", s.name, got, f)
				}
				got = f
			}
		}
		if !bytes.Equal(got, s.want) {
			t.Fatalf("%s: sent % x, want % x", s.name, got, s.want)
		}
	}
	if b.world.Trade.Open() || b.world.Prompts.Trade.Open {
		t.Fatalf("trade %+v, prompt %+v after the end", b.world.Trade, b.world.Prompts.Trade)
	}
}

// TestTradeSentPrunesTradedAt checks that an accepted trade request sent
// through Brain.send forgets partners whose Every passed, keeps the rest
// and records the new partner, and that a decline records nothing.
func TestTradeSentPrunesTradedAt(t *testing.T) {
	const stale, fresh, partner int32 = 0x5001, 0x5002, 0x5003
	now := at(5 * time.Minute)
	tests := []struct {
		name   string
		accept bool
		want   map[int32]time.Time
	}{
		{
			name: "accept prunes stale and records partner", accept: true,
			want: map[int32]time.Time{fresh: now.Add(-time.Minute + time.Millisecond), partner: now},
		},
		{
			name: "decline leaves map unchanged",
			want: map[int32]time.Time{stale: now.Add(-time.Minute), fresh: now.Add(-time.Minute + time.Millisecond)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tradeBrain(t, nil)
			every := b.persona.Trade.Every
			if every != time.Minute {
				t.Fatalf("Every = %v, want 1m", every)
			}
			b.tradedAt[stale] = now.Add(-every)
			b.tradedAt[fresh] = now.Add(-every + time.Millisecond)
			if sent, err := b.send(answerTradeAction(tt.accept, partner), now); err != nil || !sent {
				t.Fatalf("send = %v, %v; want sent", sent, err)
			}
			if !maps.Equal(b.tradedAt, tt.want) {
				t.Fatalf("tradedAt = %v, want %v", b.tradedAt, tt.want)
			}
		})
	}
}
