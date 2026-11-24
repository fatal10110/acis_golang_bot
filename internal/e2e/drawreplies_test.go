package e2e

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestDrawReplies(t *testing.T) {
	const sym, other, dye = int32(1), int32(6), int32(4445)
	dyes := map[int32]int32{sym: dye, other: 4450}
	equip := func(s int32) Sent { return Sent{Op: botclient.OpRequestHennaEquip, Target: s} }
	// after is ev read once sent packets went out.
	after := func(sent int, ev botclient.Event) Seen { return Seen{Event: ev, SentBefore: sent} }
	info := func(worn ...botclient.WornSymbol) botclient.Event { return botclient.HennaInfo{Worn: worn} }
	active := func(s int32) botclient.WornSymbol { return botclient.WornSymbol{SymbolID: s, Active: s} }
	used := func(tmpl int32) botclient.Event {
		return botclient.InventoryUpdate{Changes: []botclient.ItemChange{{State: botclient.ItemModified, Item: botclient.Item{TemplateID: tmpl}}}}
	}
	added := botclient.SystemMessage{ID: msgSymbolAdded}
	full := func(sent int, s int32) []Seen {
		return []Seen{after(sent, info(active(s))), after(sent, used(dyes[s])), after(sent, added)}
	}
	tests := []struct {
		name string
		from int // packets sent before the window opens
		seen []Seen
		sent []Sent
		want []drawReply
	}{
		{name: "none"},
		{
			name: "one draw answered",
			seen: full(1, sym),
			sent: []Sent{equip(sym)},
			want: []drawReply{{symbol: sym, info: true, dye: true, added: true}},
		},
		{
			name: "two draws, each answered before the next",
			seen: append(full(1, sym), full(2, other)...),
			sent: []Sent{equip(sym), equip(other)},
			want: []drawReply{{symbol: sym, info: true, dye: true, added: true}, {symbol: other, info: true, dye: true, added: true}},
		},
		{
			name: "the second sent before the first's answer",
			seen: append(full(2, sym), full(2, other)...),
			sent: []Sent{equip(sym), equip(other)},
			want: []drawReply{{symbol: sym}, {symbol: other, info: true, dye: true, added: true}},
		},
		{
			name: "replies read before the request",
			seen: full(0, sym),
			sent: []Sent{equip(sym)},
			want: []drawReply{{symbol: sym}},
		},
		{
			name: "the symbol inactive, another dye",
			seen: []Seen{after(1, info(botclient.WornSymbol{SymbolID: sym})), after(1, used(57)), after(1, added)},
			sent: []Sent{equip(sym)},
			want: []drawReply{{symbol: sym, added: true}},
		},
		{
			name: "other packets in between",
			seen: append([]Seen{after(1, botclient.SystemMessage{ID: 1})}, full(2, sym)...),
			sent: []Sent{{Op: 0x2f}, equip(sym)},
			want: []drawReply{{symbol: sym, info: true, dye: true, added: true}},
		},
		{
			name: "a draw before the window",
			from: 1,
			seen: full(2, other),
			sent: []Sent{equip(sym), equip(other)},
			want: []drawReply{{symbol: other, info: true, dye: true, added: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The window opens with the draw window, as draws' does.
			seen := append([]Seen{after(tt.from, botclient.HennaEquipList{})}, tt.seen...)
			if got := drawReplies(seen, tt.sent, dyes); !slices.Equal(got, tt.want) {
				t.Fatalf("drawReplies = %+v, want %+v", got, tt.want)
			}
		})
	}
}
