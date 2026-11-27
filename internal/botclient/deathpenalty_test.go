package botclient

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Scroll: Recovery templates of the tests (data/xml/items/8500-8599.xml).
const (
	recoveryNoGrade int32 = 8594
	recoveryD       int32 = 8595
)

// TestDeathPenaltyLevel checks where the world takes the bot's level
// from: the session's first EtcStatusUpdate, sent ahead of the spawn, then
// only the self-only level messages, since a later EtcStatusUpdate may be
// another player's Charm of Courage broadcast.
func TestDeathPenaltyLevel(t *testing.T) {
	added := func(n int32) Event { return SystemMessage{ID: msgDeathPenaltyLevelAdded, Number: n} }
	lifted := SystemMessage{ID: msgDeathPenaltyLifted}
	tests := []struct {
		name   string
		events []Event
		want   int32
	}{
		{"entry status", []Event{EtcStatusUpdate{DeathPenalty: 3}}, 3},
		{"another player's status", []Event{EtcStatusUpdate{DeathPenalty: 3}, EtcStatusUpdate{}}, 3},
		{"another player's penalty", []Event{EtcStatusUpdate{}, EtcStatusUpdate{DeathPenalty: 5}}, 0},
		{"raised: status, then message", []Event{EtcStatusUpdate{DeathPenalty: 1}, EtcStatusUpdate{DeathPenalty: 2}, added(2)}, 2},
		{"reduced: message, then status", []Event{EtcStatusUpdate{DeathPenalty: 2}, added(1), EtcStatusUpdate{DeathPenalty: 1}}, 1},
		{"lifted", []Event{EtcStatusUpdate{DeathPenalty: 1}, lifted, EtcStatusUpdate{}}, 0},
		{"another player's penalty after a lift", []Event{EtcStatusUpdate{DeathPenalty: 1}, lifted, EtcStatusUpdate{DeathPenalty: 4}}, 0},
		{"another message's number", []Event{EtcStatusUpdate{DeathPenalty: 2}, SystemMessage{ID: 1147, Number: 9}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, ev := range tt.events {
				w.Apply(ev, t0)
			}
			if w.Self.DeathPenalty != tt.want {
				t.Errorf("DeathPenalty = %d, want %d", w.Self.DeathPenalty, tt.want)
			}
		})
	}
}

// TestRecoveryScroll checks the level bands of skill 2286's condition: at
// least the band's floor, under the next one.
func TestRecoveryScroll(t *testing.T) {
	tests := []struct{ level, want int32 }{
		{1, 8594},
		{19, 8594},
		{20, 8595},
		{39, 8595},
		{40, 8596},
		{51, 8596},
		{52, 8597},
		{60, 8597},
		{61, 8598},
		{75, 8598},
		{76, 8599},
		{80, 8599},
	}
	for _, tt := range tests {
		if got := recoveryScroll(tt.level); got != tt.want {
			t.Errorf("recoveryScroll(%d) = %d, want %d", tt.level, got, tt.want)
		}
	}
}

// newRecoveryBrain is a default-persona brain of level 10 standing out of
// combat with death penalty level dp, carrying items.
func newRecoveryBrain(t *testing.T, dp int32, items ...Item) *Brain {
	t.Helper()
	b, _ := newTestBrain(t, DefaultPersona())
	b.world.Self.Level = 10
	b.world.Apply(EtcStatusUpdate{DeathPenalty: dp}, t0)
	b.world.Apply(ItemList{Items: items}, t0)
	return b
}

// TestRuleRecover checks when the bot reads a Scroll: Recovery: with a
// death penalty level and a scroll of its band, standing out of combat.
func TestRuleRecover(t *testing.T) {
	noGrade := Item{ObjectID: 0x30000031, TemplateID: recoveryNoGrade, Count: 2}
	gradeD := Item{ObjectID: 0x30000032, TemplateID: recoveryD, Count: 2}
	tests := []struct {
		name  string
		dp    int32
		items []Item
		setup func(b *Brain)
		want  int32 // the object read; 0 for none
	}{
		{name: "reads its band's scroll", dp: 2, items: []Item{gradeD, noGrade}, want: noGrade.ObjectID},
		{name: "no death penalty", dp: 0, items: []Item{noGrade}},
		{name: "only another band's scroll", dp: 1, items: []Item{gradeD}},
		{name: "none carried", dp: 1},
		{name: "in combat", dp: 1, items: []Item{noGrade}, setup: func(b *Brain) { b.world.Self.InCombat = true }},
		{name: "sitting", dp: 1, items: []Item{noGrade}, setup: func(b *Brain) { b.world.Self.Sitting = true }},
		{name: "dead", dp: 1, items: []Item{noGrade}, setup: func(b *Brain) { b.world.Self.Dead = true }},
		{name: "casting", dp: 1, items: []Item{noGrade}, setup: func(b *Brain) { b.world.Self.castEnd = at(11 * time.Second) }},
		{
			name: "attacked", dp: 1, items: []Item{noGrade},
			setup: func(b *Brain) {
				b.world.Apply(Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, at(9*time.Second))
			},
		},
		{
			name: "the persona keeps its penalty", dp: 1, items: []Item{noGrade},
			setup: func(b *Brain) { b.persona.Recover.LiftDeathPenalty = false },
		},
		{
			name: "level 20 reads grade D", dp: 1, items: []Item{noGrade, gradeD},
			setup: func(b *Brain) { b.world.Self.Level = 20 },
			want:  gradeD.ObjectID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newRecoveryBrain(t, tt.dp, tt.items...)
			if tt.setup != nil {
				tt.setup(b)
			}
			now := at(10 * time.Second)
			a, ok := b.ruleRecover(now)
			if tt.want == 0 {
				if ok {
					t.Fatalf("ruleRecover = %v, want none", a)
				}
				return
			}
			if !ok || a.kind != actUseItem || !bytes.Equal(a.payload, encodeUseItem(tt.want)) {
				t.Fatalf("ruleRecover = %v, %v, want a UseItem of %#x", a, ok, tt.want)
			}
			if rule, _ := b.decide(now); rule != "recover" {
				t.Fatalf("decided rule %q, want recover", rule)
			}
		})
	}
}

// TestRecoverGate checks the scroll's pacing: one UseItem per
// recoveryReuse, its reuse delay, whether or not the level dropped.
func TestRecoverGate(t *testing.T) {
	b := newRecoveryBrain(t, 3, Item{ObjectID: 0x30000031, TemplateID: recoveryNoGrade, Count: 3})
	read := func(d time.Duration, want bool) {
		t.Helper()
		now := at(d)
		a, ok := b.ruleRecover(now)
		if ok != want {
			t.Fatalf("at %v: ruleRecover reads %v, want %v", d, ok, want)
		}
		if ok {
			if sent, err := b.send(a, now); err != nil || !sent {
				t.Fatalf("at %v: send = %v, %v", d, sent, err)
			}
		}
	}
	read(10*time.Second, true)
	b.world.Apply(SystemMessage{ID: msgDeathPenaltyLevelAdded, Number: 2}, at(10*time.Second+100*time.Millisecond))
	read(10*time.Second+time.Second, false)
	read(10*time.Second+recoveryReuse-time.Millisecond, false)
	read(10*time.Second+recoveryReuse, true)
	b.world.Apply(SystemMessage{ID: msgDeathPenaltyLifted}, at(10*time.Second+recoveryReuse+100*time.Millisecond))
	read(10*time.Second+3*recoveryReuse, false)
}

// TestRecoveryWants checks the supplies tier's scroll purchase: one of the
// bot's band per death penalty level, less those carried, when sold; and
// that the band's scroll is kept while another band's is junk.
func TestRecoveryWants(t *testing.T) {
	tm := &TravelMap{}
	tm.shops, tm.cheapest = indexShops([]datapack.BuyList{
		{ID: 14, NPCID: 30004, Products: []datapack.Product{{ItemID: recoveryNoGrade, Price: 2300}, {ItemID: recoveryD, Price: 5520}}},
	})
	tests := []struct {
		name  string
		dp    int32
		level int32
		items []Item
		edit  func(b *Brain)
		want  []buyRow
	}{
		{name: "one per level", dp: 3, level: 10, want: []buyRow{{item: recoveryNoGrade, count: 3}}},
		{name: "less those carried", dp: 3, level: 10, items: []Item{{ObjectID: 0x30000031, TemplateID: recoveryNoGrade, Count: 2}}, want: []buyRow{{item: recoveryNoGrade, count: 1}}},
		{name: "enough carried", dp: 2, level: 10, items: []Item{{ObjectID: 0x30000031, TemplateID: recoveryNoGrade, Count: 2}}},
		{name: "its level's band", dp: 1, level: 25, want: []buyRow{{item: recoveryD, count: 1}}},
		{name: "no penalty", dp: 0, level: 10},
		{name: "not sold", dp: 1, level: 45},
		{name: "the persona keeps its penalty", dp: 1, level: 10, edit: func(b *Brain) { b.persona.Recover.LiftDeathPenalty = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newRecoveryBrain(t, tt.dp, tt.items...)
			b.cfg.Travel = tm
			b.world.Self.Level = tt.level
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.recoveryWants(); !slices.Equal(got, tt.want) {
				t.Fatalf("recoveryWants = %v, want %v", got, tt.want)
			}
			if tt.want != nil && !slices.ContainsFunc(b.wants(false), func(w want) bool { return w.buyRow == tt.want[0] && w.tier == tierSupplies }) {
				t.Fatalf("wants = %v, want %v in the supplies tier", b.wants(false), tt.want[0])
			}
		})
	}

	noGrade := Item{ObjectID: 0x30000031, TemplateID: recoveryNoGrade, Count: 1}
	gradeD := Item{ObjectID: 0x30000032, TemplateID: recoveryD, Count: 1}
	b := newRecoveryBrain(t, 0, noGrade, gradeD)
	b.persona.Shop.SellAt = 1
	junk := b.junk()
	if _, sold := junk[noGrade.ObjectID]; sold {
		t.Error("the band's Scroll: Recovery is junk")
	}
	if _, sold := junk[gradeD.ObjectID]; !sold {
		t.Error("another band's Scroll: Recovery is not junk")
	}
}
