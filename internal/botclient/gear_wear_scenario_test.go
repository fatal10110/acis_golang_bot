package botclient

import (
	"maps"
	"testing"
	"time"
)

// TestBrainGearWearScenario plays TestLiveGearWear's steps against the
// equip rules of the server (itemcontainer/inventory.go equipItemLocked,
// equipPaired): a human fighter (class 0) wearing its starting set is
// given a shield, a necklace and two Magic Rings, then a Ring of
// Knowledge, then a two-handed sword. The scenario pinned the warrior's
// build entry (#455): since the Warrior's own row (#443) that entry is for
// class 1 only, so the class 0 bot has none, wears the jewelry and never
// the shield, and the scenario waited on it until its bound. The entry the
// scenario pins now (fighterGear, the default table's last entry) wears
// all of it.
func TestBrainGearWearScenario(t *testing.T) {
	warrior := CompileBuild(0, classWarrior, 20, RoleDamage, BranchChoice{}).Kit.Gear
	tests := []struct {
		name  string
		table []GearClass
		// steps are what the GM gives, one step at a time, and what the
		// bot wears by template once it has nothing left to use.
		steps []scenarioStep
	}{
		{
			name:  "warrior's build entry: no shield (#455)",
			table: []GearClass{warrior},
			steps: []scenarioStep{{
				give: []Item{carried(301, smallShield, partLHand), carried(302, necklaceOfMagic, partNeck), carried(303, magicRing, partLRFinger), carried(304, magicRing, partLRFinger)},
				want: map[int32]int{shortSword: 1, squireShirt: 1, squirePants: 1, necklaceOfMagic: 1, magicRing: 2},
			}},
		},
		{
			name:  "the scenario's fighter entry",
			table: swapTable,
			steps: []scenarioStep{
				{
					give: []Item{carried(301, smallShield, partLHand), carried(302, necklaceOfMagic, partNeck), carried(303, magicRing, partLRFinger), carried(304, magicRing, partLRFinger)},
					want: map[int32]int{shortSword: 1, squireShirt: 1, squirePants: 1, smallShield: 1, necklaceOfMagic: 1, magicRing: 2},
				},
				{
					give: []Item{carried(305, ringOfKnowledge, partLRFinger)},
					want: map[int32]int{shortSword: 1, squireShirt: 1, squirePants: 1, smallShield: 1, necklaceOfMagic: 1, magicRing: 1, ringOfKnowledge: 1},
				},
				{
					give: []Item{carried(306, redSunsetSword, partLRHand)},
					want: map[int32]int{redSunsetSword: 1, squireShirt: 1, squirePants: 1, necklaceOfMagic: 1, magicRing: 1, ringOfKnowledge: 1},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSlotsBrain(t, classHumanFight, wornSword, wornShirt, wornPants)
			b.persona.Gear.Jewelry, b.persona.Gear.Buy = true, false
			b.persona.Gear.Classes = tt.table
			now := at(time.Second)
			for i, st := range tt.steps {
				b.world.Apply(InventoryUpdate{Changes: changes(ItemAdded, st.give...)}, now)
				for n := 0; ; n++ {
					if n == 10 {
						t.Fatalf("step %d: still using items after %d UseItems", i, n)
					}
					now = now.Add(time.Second)
					a, ok := b.ruleGear(now)
					if !ok {
						break
					}
					if a.kind != actUseItem {
						t.Fatalf("step %d: ruleGear = %+v, want a UseItem", i, a)
					}
					now = now.Add(time.Second)
					b.world.Apply(InventoryUpdate{Changes: changes(ItemModified, serveUseItem(b.world.Inventory.Items, a.object)...)}, now)
				}
				if got := wornTemplates(b.world.Inventory.Items); !maps.Equal(got, st.want) {
					t.Fatalf("step %d: wears %v, want %v", i, got, st.want)
				}
			}
		})
	}
}

// scenarioStep is a GM's gift and what the bot wears after it.
type scenarioStep struct {
	give []Item
	want map[int32]int
}

// changes are items as InventoryUpdate changes of kind k.
func changes(k ItemState, items ...Item) []ItemChange {
	out := make([]ItemChange, len(items))
	for i, it := range items {
		out[i] = ItemChange{State: k, Item: it}
	}
	return out
}

// serveUseItem is the server's answer to a UseItem of obj: the items whose
// Equipped flag changes. A worn item is taken off; a carried one is worn,
// taking off what holds its slot: a two-handed weapon takes the weapon and
// the shield off, a shield a two-handed weapon, a ring the left one of two
// worn (inventory.go equipItemLocked, equipPaired).
func serveUseItem(inv map[int32]Item, obj int32) []Item {
	it := inv[obj]
	if it.Equipped {
		it.Equipped = false
		return []Item{it}
	}
	it.Equipped = true
	out := []Item{it}
	off := func(match func(Item) bool) {
		for _, w := range inv {
			if w.Equipped && match(w) {
				w.Equipped = false
				out = append(out, w)
				return
			}
		}
	}
	hand := func(w Item) bool { return w.BodySlot == partRHand || w.BodySlot == partLRHand }
	switch it.BodySlot {
	case partRHand:
		off(hand)
	case partLRHand:
		off(hand)
		off(func(w Item) bool { return w.BodySlot == partLHand })
	case partLHand:
		off(func(w Item) bool { return w.BodySlot == partLHand || w.BodySlot == partLRHand })
	case partLRFinger:
		n := 0
		for _, w := range inv {
			if w.Equipped && w.BodySlot == partLRFinger {
				n++
			}
		}
		if n == 2 {
			off(func(w Item) bool { return w.BodySlot == partLRFinger })
		}
	default:
		off(func(w Item) bool { return w.BodySlot == it.BodySlot })
	}
	return out
}

// wornTemplates counts the worn items of inv by template.
func wornTemplates(inv map[int32]Item) map[int32]int {
	out := make(map[int32]int)
	for _, it := range inv {
		if it.Equipped {
			out[it.TemplateID]++
		}
	}
	return out
}
