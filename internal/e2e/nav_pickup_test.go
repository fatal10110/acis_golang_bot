package e2e

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

func TestKillOf(t *testing.T) {
	const self, mob, other = 1, 900, 901
	t0 := time.Unix(0, 0)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	swing := func(by, on int32, x int32) botclient.Attack {
		return botclient.Attack{AttackerID: by, Pos: botclient.Point{X: x}, Hits: []botclient.AttackHit{{TargetID: on}}}
	}
	drop := func(by, id int32) botclient.DropItem {
		return botclient.DropItem{DropperID: by, Item: botclient.GroundItem{ObjectID: id}}
	}
	tests := []struct {
		name  string
		seen  []Seen
		want  wolfKill
		found bool
	}{
		{"last own swing on the mob, its drops, the packets by the Die", []Seen{
			{At: at(0), Event: swing(self, mob, 10)},
			{At: at(1), Event: swing(self, mob, 20)},
			{At: at(2), Event: swing(self, other, 30)},
			{At: at(3), Event: swing(mob, self, 40)},
			{At: at(4), Event: drop(other, 7)},
			{At: at(5), Event: drop(mob, 8), SentBefore: 4},
			{At: at(6), Event: botclient.Die{ObjectID: mob}, SentBefore: 5},
			{At: at(7), Event: drop(mob, 9)},
		}, wolfKill{from: botclient.Point{X: 20}, drops: []botclient.DropItem{drop(mob, 8)}, sentAt: 5}, true},
		{"no own swing", []Seen{
			{At: at(0), Event: swing(other, mob, 10)},
			{At: at(1), Event: botclient.Die{ObjectID: mob}},
		}, wolfKill{}, false},
		{"no Die", []Seen{{At: at(0), Event: swing(self, mob, 10)}}, wolfKill{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := killOf(tt.seen, self, mob)
			if ok != tt.found || got.from != tt.want.from || got.sentAt != tt.want.sentAt || len(got.drops) != len(tt.want.drops) {
				t.Fatalf("killOf = %+v, %v; want %+v, %v", got, ok, tt.want, tt.found)
			}
			for i := range got.drops {
				if got.drops[i] != tt.want.drops[i] {
					t.Fatalf("drop %d = %+v, want %+v", i, got.drops[i], tt.want.drops[i])
				}
			}
		})
	}
}

func TestPickedUp(t *testing.T) {
	const self, item, tmpl = 1, 8, 57
	t0 := time.Unix(0, 0)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	d := botclient.DropItem{DropperID: 900, Item: botclient.GroundItem{ObjectID: item, TemplateID: tmpl}}
	gain := func(tm int32, st botclient.ItemState) botclient.InventoryUpdate {
		return botclient.InventoryUpdate{Changes: []botclient.ItemChange{{State: st, Item: botclient.Item{TemplateID: tm}}}}
	}
	tests := []struct {
		name string
		seen []Seen
		want time.Time
		ok   bool
	}{
		{"GetItem then the gain", []Seen{
			{At: at(0), Event: gain(tmpl, botclient.ItemModified)},
			{At: at(1), Event: botclient.GetItem{PickerID: self, ObjectID: item}},
			{At: at(2), Event: gain(tmpl+1, botclient.ItemModified)},
			{At: at(3), Event: gain(tmpl, botclient.ItemModified)},
		}, at(3), true},
		{"gain before the GetItem only", []Seen{
			{At: at(0), Event: gain(tmpl, botclient.ItemModified)},
			{At: at(1), Event: botclient.GetItem{PickerID: self, ObjectID: item}},
		}, time.Time{}, false},
		{"another picker", []Seen{
			{At: at(0), Event: botclient.GetItem{PickerID: 2, ObjectID: item}},
			{At: at(1), Event: gain(tmpl, botclient.ItemModified)},
		}, time.Time{}, false},
		{"a removal is no gain", []Seen{
			{At: at(0), Event: botclient.GetItem{PickerID: self, ObjectID: item}},
			{At: at(1), Event: gain(tmpl, botclient.ItemRemoved)},
		}, time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := pickedUp(tt.seen, self, d); ok != tt.ok || !got.Equal(tt.want) {
				t.Fatalf("pickedUp = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPickUpClick(t *testing.T) {
	action := func(id, x, y, z int32) Sent {
		p := make([]byte, 18)
		p[0] = opAction
		for i, v := range []int32{id, x, y, z} {
			binary.LittleEndian.PutUint32(p[1+4*i:], uint32(v))
		}
		return Sent{Op: opAction, Target: id, Payload: p}
	}
	sent := []Sent{
		{Op: opMoveBackwardToLocation, Target: 5},
		action(7, 1, 2, 3),
		action(8, 4, 5, 6),
		action(8, 9, 9, 9),
	}
	tests := []struct {
		name string
		id   int32
		want botclient.Point
		ok   bool
	}{
		{"first Action on the item", 8, botclient.Point{X: 4, Y: 5, Z: 6}, true},
		{"none on the item", 5, botclient.Point{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got, ok := pickUpClick(sent, tt.id); ok != tt.ok || got != tt.want {
				t.Fatalf("pickUpClick = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLootTurns(t *testing.T) {
	pt := func(x int32) botclient.Point { return botclient.Point{X: x} }
	action := func(id, x int32) Sent {
		p := make([]byte, 18)
		p[0] = opAction
		for i, v := range []int32{id, x, 0, 0} {
			binary.LittleEndian.PutUint32(p[1+4*i:], uint32(v))
		}
		return Sent{Op: opAction, Target: id, Payload: p}
	}
	move := func(from int32) Sent {
		return Sent{Op: opMoveBackwardToLocation, Payload: botwire.EncodeMoveBackwardToLocation(pt(from+50), pt(from))}
	}
	tests := []struct {
		name string
		sent []Sent
		want []lootTurn
	}{
		{"straight clicks, each from the drop before", []Sent{action(1, 10), action(2, 20)}, []lootTurn{
			{target: 1, from: pt(10), click: action(1, 10)},
			{target: 2, from: pt(20), click: action(2, 20)},
		}},
		{"a walk to a stand point turns from its first move", []Sent{action(1, 10), move(20), move(30), action(2, 40)}, []lootTurn{
			{target: 1, from: pt(10), click: action(1, 10)},
			{target: 2, from: pt(20), moves: 2, click: action(2, 40)},
		}},
		{"a retried click stays in its turn", []Sent{action(1, 10), move(10), action(1, 12), action(2, 20)}, []lootTurn{
			{target: 1, from: pt(10), click: action(1, 10)},
			{target: 2, from: pt(20), click: action(2, 20)},
		}},
		{"moves with no click yet are left out", []Sent{action(1, 10), move(20)}, []lootTurn{
			{target: 1, from: pt(10), click: action(1, 10)},
		}},
		{"other packets ignored", []Sent{{Op: 0x0a, Target: 1}, action(1, 10)}, []lootTurn{
			{target: 1, from: pt(10), click: action(1, 10)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lootTurns(tt.sent)
			if len(got) != len(tt.want) {
				t.Fatalf("lootTurns = %+v, want %+v", got, tt.want)
			}
			for i, g := range got {
				w := tt.want[i]
				if g.target != w.target || g.from != w.from || g.moves != w.moves || !bytes.Equal(g.click.Payload, w.click.Payload) {
					t.Fatalf("turn %d = %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

func TestClosedTurn(t *testing.T) {
	const herbTmpl, adena = 8600, 57
	drop := func(id, tmpl, x int32) botclient.DropItem {
		return botclient.DropItem{DropperID: 900, Item: botclient.GroundItem{ObjectID: id, TemplateID: tmpl, Pos: botclient.Point{X: x}}}
	}
	turn := func(id, from int32) lootTurn { return lootTurn{target: id, from: botclient.Point{X: from}} }
	// A spot is closed from 100 or more away.
	closed := func(from, q botclient.Point) bool { return q.X-from.X >= 100 || from.X-q.X >= 100 }
	drops := []botclient.DropItem{drop(1, adena, 0), drop(2, adena, 100), drop(3, herbTmpl, 300), drop(4, adena, 400)}
	tests := []struct {
		name  string
		turns []lootTurn
		want  int32
		ok    bool
	}{
		{"closed from the kill spot", []lootTurn{turn(2, 0)}, 2, true},
		{"open from the drop before, closed from the kill spot", []lootTurn{turn(1, 0), turn(2, 0+50)}, 0, false},
		{"closed from the drop before", []lootTurn{turn(1, 0), turn(2, 50), turn(4, 100)}, 4, true},
		{"closed herb left out", []lootTurn{turn(3, 0)}, 0, false},
		{"turn onto no drop of the kill", []lootTurn{turn(9, 1000)}, 0, false},
		{"no turns", nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _, ok := closedTurn(tt.turns, drops, closed); ok != tt.ok || got.Item.ObjectID != tt.want {
				t.Fatalf("closedTurn = item %d, %v; want item %d, %v", got.Item.ObjectID, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestClickedAll(t *testing.T) {
	drops := []botclient.DropItem{{Item: botclient.GroundItem{ObjectID: 1}}, {Item: botclient.GroundItem{ObjectID: 2}}}
	tests := []struct {
		name  string
		turns []lootTurn
		want  bool
	}{
		{"every drop clicked", []lootTurn{{target: 2}, {target: 9}, {target: 1}}, true},
		{"one drop left", []lootTurn{{target: 1}}, false},
		{"none", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clickedAll(tt.turns, drops); got != tt.want {
				t.Fatalf("clickedAll = %v, want %v", got, tt.want)
			}
		})
	}
}
