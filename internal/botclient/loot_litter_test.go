package botclient

import (
	"slices"
	"testing"
	"time"
)

// litter is ground item obj at x on the bot's axis, seen at t0.
func litter(obj int32, x int32) timed {
	return timed{0, SpawnItem{Item: GroundItem{ObjectID: obj, TemplateID: 2000, Pos: Point{X: x}, Count: 1}}}
}

// TestLootFailed checks the ActionFailed reply to a pickup click
// (lootFailed): with the bot within pick-up range of the clicked item and
// the click not held, the item leaves the queue at once and counts no
// stall miss; a click from farther, which the server answers when it
// starts the walk, one the server defers (the pickup lock, the bot's own
// cast or swing, its stand-up), an ActionFailed with no click in flight
// and one after another action went out (a refused cast or potion) leave
// the queue alone.
func TestLootFailed(t *testing.T) {
	const near, far = 20, 100
	tests := []struct {
		name string
		x    int32
		// picked, when set, is how long before the click the bot saw its
		// own pickup of another item.
		picked  time.Duration
		noClick bool
		// before edits the brain before the click at now, after between the
		// click and the ActionFailed.
		before, after func(b *Brain, now time.Time)
		retired       bool
	}{
		{name: "within range", x: near, retired: true},
		{name: "from farther: the server walks", x: far},
		{name: "no click in flight", x: near, noClick: true},
		{name: "during the pickup lock", x: near, picked: pickupLock - time.Millisecond},
		{name: "after the pickup lock", x: near, picked: pickupLock, retired: true},
		{name: "during the bot's cast", x: near, before: func(b *Brain, now time.Time) { b.world.Self.castEnd = now.Add(time.Second) }},
		{name: "after the bot's cast", x: near, retired: true, before: func(b *Brain, now time.Time) { b.world.Self.castEnd = now }},
		{name: "during the bot's swing", x: near, before: func(b *Brain, now time.Time) { b.world.Self.swingEnd = now.Add(time.Millisecond) }},
		{name: "during the stand-up", x: near, before: func(b *Brain, now time.Time) {
			b.store.stoodAt = now.Add(-storeStandSettle + time.Millisecond)
		}},
		{name: "after the stand-up", x: near, retired: true, before: func(b *Brain, now time.Time) {
			b.store.stoodAt = now.Add(-storeStandSettle)
		}},
		{name: "another action after the click", x: near, after: func(b *Brain, now time.Time) {
			b.loot.sent(Action{kind: actUseItem}, now.Add(10*time.Millisecond))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSlotLimitBrain(t, 0, 10)
			b.persona.Loot.Ground = GroundAny
			const first, second = 0x40000100, 0x40000101
			handleAll(t, b, []timed{litter(first, tt.x), litter(second, tt.x)})
			now := at(lootGrace)
			if tt.picked > 0 {
				handleAll(t, b, []timed{{lootGrace - tt.picked, GetItem{PickerID: selfID, ObjectID: 0x40000999}}})
			}
			if tt.before != nil {
				tt.before(b, now)
			}
			if tt.noClick {
				if id := b.nextLoot(now); id != first {
					t.Fatalf("queue head %d, want %d", id, first)
				}
			} else if id, _ := pickedUp(b, now); id != first {
				t.Fatalf("picks up %d, want %d", id, first)
			}
			if tt.after != nil {
				tt.after(b, now)
			}
			handleAll(t, b, []timed{{lootGrace + 50*time.Millisecond, ActionFailed{}}})
			want := int32(first)
			if tt.retired {
				want = second
			}
			if len(b.loot.items) == 0 || b.loot.items[0] != want {
				t.Fatalf("queue %v, want %d at its head", b.loot.items, want)
			}
			if b.loot.misses != 0 {
				t.Fatalf("%d misses, want none", b.loot.misses)
			}
		})
	}
}

// TestLootFailedSlotsFull checks a pickup refused for want of a slot:
// ActionFailed, then SystemMessage 129 (network/pickup.go). The clicked
// item leaves the queue once, and the next one, adena, which needs no
// slot, is picked up next.
func TestLootFailedSlotsFull(t *testing.T) {
	b := newSlotLimitBrain(t, 0, 80)
	sword := GroundItem{ObjectID: junkDrop, TemplateID: 2000, Pos: Point{X: 20}, Count: 1}
	adena := GroundItem{ObjectID: adenaDrop, TemplateID: adenaID, Pos: Point{X: 20}, Stackable: true, Count: 5}
	handleAll(t, b, kill(sword, adena))
	now := at(time.Second)
	if id, _ := pickedUp(b, now); id != junkDrop {
		t.Fatalf("picks up %d, want %d", id, junkDrop)
	}
	handleAll(t, b, []timed{{time.Second + 50*time.Millisecond, ActionFailed{}}, {time.Second + 50*time.Millisecond, SystemMessage{ID: msgSlotsFull}}})
	if id, _ := pickedUp(b, now.Add(100*time.Millisecond)); id != adenaDrop {
		t.Fatalf("after the refusal picks up %d, want %d", id, adenaDrop)
	}
}

// TestLootLitter checks the town litter of #686: ground items whose every
// click the server refuses cost the bot one click each, not Loot.Timeout,
// and pause no ground pickup however many there are, so a ground item
// queued behind them is picked up within a second.
func TestLootLitter(t *testing.T) {
	b := newSlotLimitBrain(t, 0, 10)
	b.persona.Loot.Ground = GroundAny
	const drop = 0x40000200
	var evs []timed
	for i := range int32(lootMisses + 1) {
		evs = append(evs, litter(0x40000100+i, 20))
	}
	handleAll(t, b, append(evs, litter(drop, 20)))
	now := at(lootGrace)
	for i := range int32(lootMisses + 1) {
		if id, _ := pickedUp(b, now); id != 0x40000100+i {
			t.Fatalf("litter %d: picks up %d, want %d", i, id, 0x40000100+i)
		}
		now = now.Add(100 * time.Millisecond)
		handleAll(t, b, []timed{{now.Sub(t0), ActionFailed{}}})
	}
	if id, _ := pickedUp(b, now); id != drop {
		t.Fatalf("after the litter picks up %d, want %d", id, drop)
	}
	if !b.loot.pauseUntil.IsZero() {
		t.Fatalf("ground pickups paused until %v, want no pause", b.loot.pauseUntil)
	}
}

// TestLootPauseDefers checks that a loot stall's pause sends the queued
// ground items back to the ground record (groundSeen) rather than dropping
// them, and that the first scan after the pause queues them again in the
// order they were queued.
func TestLootPauseDefers(t *testing.T) {
	b := newSlotLimitBrain(t, 0, 10)
	b.persona.Loot.Ground = GroundAny
	const first, second = 0x40000100, 0x40000101
	handleAll(t, b, []timed{litter(first, 100), litter(second, 100)})
	now := at(lootGrace)
	if id, _ := pickedUp(b, now); id != first {
		t.Fatalf("picks up %d, want %d", id, first)
	}
	b.loot.pauseUntil = now.Add(lootPause)
	if id, ok := pickedUp(b, now); id != 0 || ok {
		t.Fatalf("paused: picks up %d (holds %v), want nothing", id, ok)
	}
	if len(b.loot.items) != 0 {
		t.Fatalf("paused: queue %v, want it empty", b.loot.items)
	}
	for _, id := range []int32{first, second} {
		if _, ok := b.loot.ground[id]; !ok {
			t.Fatalf("paused: %d left the ground record", id)
		}
	}
	if id, _ := pickedUp(b, now.Add(lootPause)); id != first {
		t.Fatalf("pause over: picks up %d, want %d", id, first)
	}
	if !slices.Equal(b.loot.items, []int32{first, second}) {
		t.Fatalf("pause over: queue %v, want [%d %d]", b.loot.items, first, second)
	}
}
