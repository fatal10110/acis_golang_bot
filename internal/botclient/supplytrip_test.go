package botclient

import (
	"testing"
	"time"
)

// The supply trip tests' items and NPCs.
const (
	// junkMat is loot the bot sells, keptMat loot it keeps (Shop.Keep).
	junkMat int32 = 1864
	keptMat int32 = 1865
	// soeObj is the carried Scroll of Escape stack.
	soeObj = int32(50)
	// mob is a monster template.
	mob    int32 = 20102
	mobObj       = int32(0x20000031)
	// townGK is the town's gatekeeper template.
	townGK int32 = 30006
)

// town is where the supply trip tests' town gatekeeper stands, on the
// spot's side of the split navigation.
var town = Point{X: 4000}

// newTripBrain is a level 5 farmer at its spot's centre (the origin) with
// the default supply policy and a weight limit of 100000, carrying items:
// HP potions a spawned merchant sells, the town's gatekeeper at town.
func newTripBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newSpotBrain(t, 5, testSpot("s", 0, 0, 3, 6, 9))
	b.cfg.Spots.claim([]int{0})
	b.spot.idx, b.spot.held, b.spot.arrived = 0, true, true
	b.world.Objects = map[int32]*Object{}
	b.persona.Supply = defaultSupplyPolicy()
	b.persona.Potions = []int32{lesserHealingPotion}
	b.cfg.Travel = &TravelMap{
		gatekeepers: []gatekeeper{{npcID: townGK, at: town}},
		weights:     map[int32]int32{lesserHealingPotion: 5, scrollOfEscape: 120, junkMat: 10, keptMat: 10},
		spawnedSold: map[int32]bool{lesserHealingPotion: true},
	}
	b.world.Apply(ItemList{Items: items}, t0)
	b.world.Self.MaxLoad = 100000
	return b
}

func soeStack(n int32) Item {
	return Item{ObjectID: soeObj, TemplateID: scrollOfEscape, Count: n, SubCategory: 5}
}

func lootStack(obj, id, n int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: n, SubCategory: 5}
}

// TestSupplyCause checks each trigger of the supply trip: a consumable
// that runs out within ReturnMinutes of fighting (two HP potions last the
// first estimate's 1.7 of ten minutes, one does not), HP potions gone once
// carried, a load past ReturnLoad that a visit sheds ShedMin of (not one
// between BuyLoad and ReturnLoad, nor one of kept items with nowhere to
// go), and nothing for what the bot could never buy.
func TestSupplyCause(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		load  int32
		edit  func(b *Brain)
		want  tripCause
	}{
		{name: "stocked", items: []Item{potionStack(20)}},
		{name: "potions run out within ReturnMinutes", items: []Item{potionStack(1)}, want: causeRunOut},
		{name: "potions last past ReturnMinutes", items: []Item{potionStack(2)}},
		{name: "ReturnMinutes 0 never", items: []Item{potionStack(1)}, edit: func(b *Brain) { b.persona.Supply.ReturnMinutes = 0 }},
		{name: "potions gone", edit: func(b *Brain) { b.supplyTrip.potions = true }, want: causePotions},
		{name: "potions never carried", items: nil},
		{
			name: "potions gone, no merchant sells them",
			edit: func(b *Brain) { b.supplyTrip.potions = true; b.cfg.Travel.spawnedSold = nil },
		},
		{name: "past ReturnLoad with ShedMin to sell", items: []Item{potionStack(20), lootStack(60, junkMat, 1000)}, load: 60001, want: causeLoad},
		{name: "past BuyLoad, under ReturnLoad", items: []Item{potionStack(20), lootStack(60, junkMat, 1000)}, load: 60000},
		{name: "past ReturnLoad, under ShedMin to sell", items: []Item{potionStack(20), lootStack(60, junkMat, 999)}, load: 90000},
		{
			name: "past ReturnLoad, kept with nowhere to go", items: []Item{potionStack(20), lootStack(60, keptMat, 5000)}, load: 90000,
			edit: func(b *Brain) { b.persona.Shop.Keep = []int32{keptMat} },
		},
		{
			name: "past ReturnLoad, selling off", items: []Item{potionStack(20), lootStack(60, junkMat, 1000)}, load: 90000,
			edit: func(b *Brain) { b.persona.Shop.SellAt = 0 },
		},
		{
			name: "ReturnLoad 0 never", items: []Item{potionStack(20), lootStack(60, junkMat, 5000)}, load: 99000,
			edit: func(b *Brain) { b.persona.Supply.ReturnLoad = 0 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, tt.items...)
			b.world.Self.Load = tt.load
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.supplyCause(); got != tt.want {
				t.Fatalf("supplyCause = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSupplyCauseShots checks the run-out trigger on shots: the no-grade
// sword spends 60 a minute at first, so 500 run out within ten minutes and
// 700 do not; the Extra supplies count for nothing.
func TestSupplyCauseShots(t *testing.T) {
	tests := []struct {
		name  string
		shots int32
		want  tripCause
	}{
		{"run out", 500, causeRunOut},
		{"last", 700, causeNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMeterBrain(t, classHumanFight, wielded(1, ngSword), shotStack(2, soulshotNoGrade, tt.shots), lootStack(3, soulOre, 1))
			b.persona.Supply.ReturnMinutes = 10
			b.persona.Supply.Extra = func(*World) map[int32]int32 { return map[int32]int32{soulOre: 500} }
			if got := b.supplyCause(); got != tt.want {
				t.Fatalf("supplyCause = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSupplyCauseGear checks the gear trigger: adena past the gear floor
// (GearPolicy.KeepAdena) that pays the goal's cheapest price sends the bot
// home; saving towards it does not, nor a bot that buys no gear.
func TestSupplyCauseGear(t *testing.T) {
	const gearKeep = 10000
	tests := []struct {
		name  string
		adena int32
		buy   bool
		want  tripCause
	}{
		{"goal paid", gearKeep + 244000, true, causeGear},
		{"saving", gearKeep + 243999, true, causeNone},
		{"buys no gear", gearKeep + 244000, false, causeNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBudgetBrain(t, tt.adena)
			b.persona.Gear.Buy = tt.buy
			if goal, _ := b.gearGoal(); tt.buy && (len(goal) != 1 || goal[0] != falchion) {
				t.Fatalf("gearGoal = %v, want the falchion", goal)
			}
			if got := b.supplyCause(); got != tt.want {
				t.Fatalf("supplyCause = %d, want %d", got, tt.want)
			}
		})
	}
}

// decideSend runs the decision table at now and sends what it decided.
func decideSend(t *testing.T, b *Brain, now time.Time) (string, Action) {
	t.Helper()
	rule, a := b.decide(now)
	if a.kind != actHold {
		if _, err := b.send(a, now); err != nil {
			t.Fatal(err)
		}
	}
	return rule, a
}

// TestSupplyTripScroll checks the trip by Scroll of Escape: the trigger
// releases the spot and reads the scroll once; while the recall runs every
// rule is held, a fight, a critical run away and a loot alike, and no second
// scroll goes out; the teleport ends the trip and the bot picks a spot
// again.
func TestSupplyTripScroll(t *testing.T) {
	b := newTripBrain(t, soeStack(2))
	b.supplyTrip.potions = true
	now := at(time.Minute)
	rule, a := decideSend(t, b, now)
	if rule != "supply" || a.kind != actUseItem || a.item != scrollOfEscape {
		t.Fatalf("decide = rule %q action %d item %d, want supply UseItem of the scroll", rule, a.kind, a.item)
	}
	if b.spot.held || b.cfg.Spots.Held(0) != 0 {
		t.Fatalf("spot held %v, registry holds %d: want it released", b.spot.held, b.cfg.Spots.Held(0))
	}
	b.world.Apply(MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: 2013, HitTime: 20 * time.Second}, now.Add(300*time.Millisecond))

	held := []struct {
		name  string
		after time.Duration
		edit  func(b *Brain, now time.Time)
		rule  string
	}{
		{name: "idle", after: time.Second, rule: "escape"},
		{
			name: "attacked", after: 5 * time.Second, rule: "escape",
			edit: func(b *Brain, now time.Time) {
				b.world.Apply(NPCInfo{ObjectID: mobObj, NPCID: mob, Attackable: true, Pos: Point{X: 100}}, now)
				b.world.Apply(Attack{AttackerID: mobObj, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, now)
				b.world.Self.InCombat = true
			},
		},
		{
			name: "critical with no potion: no run away", after: 6 * time.Second, rule: "critical",
			edit: func(b *Brain, _ time.Time) { b.world.Self.HP = 5 },
		},
		{
			name: "past the cast, a monster in sight: waits for the teleport", after: 22 * time.Second, rule: "escape",
			edit: func(b *Brain, _ time.Time) {
				b.world.Attackers = map[int32]*Attacker{}
				b.world.Self.InCombat, b.world.Self.HP = false, 100
			},
		},
	}
	for _, h := range held {
		when := now.Add(h.after)
		if h.edit != nil {
			h.edit(b, when)
		}
		if rule, a := decideSend(t, b, when); rule != h.rule || a.kind != actHold {
			t.Fatalf("%s: decide = rule %q action %d, want a hold by %q", h.name, rule, a.kind, h.rule)
		}
	}
	b.world.Objects = map[int32]*Object{}
	land := now.Add(21 * time.Second)
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: town}}, land); err != nil {
		t.Fatal(err)
	}
	if b.supplyTrip.active() || b.supplyTrip.endedAt != land {
		t.Fatalf("trip active %v ended %v after the teleport, want ended at %v", b.supplyTrip.active(), b.supplyTrip.endedAt, land)
	}
	if rule, _ := b.decide(land.Add(supplyTripGap - time.Second)); rule == "supply" || rule == "escape" {
		t.Fatalf("decide after the trip = rule %q, want the trip over", rule)
	}
}

// TestSupplyTripStandsUp checks that a seated bot stands before it reads
// the scroll.
func TestSupplyTripStandsUp(t *testing.T) {
	b := newTripBrain(t, soeStack(1))
	b.supplyTrip.potions = true
	b.world.Self.Sitting = true
	if rule, a := b.decide(at(time.Minute)); rule != "supply" || a.kind != actSitStand {
		t.Fatalf("decide = rule %q action %d, want supply stand up", rule, a.kind)
	}
}

// TestSupplyTripSafety checks that the scroll waits for a safe moment, no
// attacker; a monster close by that does not attack, as at a starter spot
// where nearly every monster would attack on sight (#251), or one whose
// last hit is past attackerWindow does not stop it, and with no safe
// moment within SafeWait the bot walks home instead.
func TestSupplyTripSafety(t *testing.T) {
	start := at(time.Minute)
	tests := []struct {
		name string
		// mob puts an attackable monster close by.
		mob bool
		// hit is how long before each decide the monster hits the bot, 0
		// for never.
		hit   time.Duration
		after time.Duration
		want  actionKind
	}{
		{name: "safe", want: actUseItem},
		{name: "monster close by, not attacking (#251)", mob: true, want: actUseItem},
		{name: "attacker", mob: true, hit: time.Second, want: actHold},
		{name: "last hit past attackerWindow", mob: true, hit: attackerWindow + time.Second, want: actUseItem},
		{name: "attacker past SafeWait: walks", mob: true, hit: time.Second, after: 2 * time.Minute, want: actMove},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, soeStack(1))
			b.supplyTrip.potions = true
			if tt.mob {
				b.world.Apply(NPCInfo{ObjectID: mobObj, NPCID: mob, Attackable: true, Pos: Point{X: 100}}, start)
			}
			decide := func(now time.Time) (string, Action) {
				if tt.hit > 0 {
					b.world.Apply(Attack{AttackerID: mobObj, Hits: []AttackHit{{TargetID: selfID, Damage: 1}}}, now.Add(-tt.hit))
				}
				return b.decide(now)
			}
			if rule, a := decide(start); rule != "supply" {
				t.Fatalf("decide = rule %q action %d, want supply", rule, a.kind)
			}
			if tt.after > 0 {
				if rule, a := decide(start.Add(tt.after - time.Second)); rule != "supply" || a.kind != actHold {
					t.Fatalf("before SafeWait: decide = rule %q action %d, want a supply hold", rule, a.kind)
				}
			}
			if rule, a := decide(start.Add(tt.after)); rule != "supply" || a.kind != tt.want {
				t.Fatalf("decide = rule %q action %d, want supply %d", rule, a.kind, tt.want)
			}
		})
	}
}

// TestSupplyTripNoTeleport checks a recall that took the bot nowhere: held
// for escapeRetry past the cast's end, or past the UseItem when no cast
// started, then the bot walks home; no second scroll goes out.
func TestSupplyTripNoTeleport(t *testing.T) {
	tests := []struct {
		name string
		cast time.Duration // the recall's hit time, 0 for none
	}{
		{"cast ended", 20 * time.Second},
		{"refused", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, soeStack(2))
			b.supplyTrip.potions = true
			now := at(time.Minute)
			if _, a := decideSend(t, b, now); a.kind != actUseItem {
				t.Fatalf("decide = action %d, want UseItem", a.kind)
			}
			if tt.cast > 0 {
				b.world.Apply(MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: 2013, HitTime: tt.cast}, now)
			}
			wait := now.Add(tt.cast + escapeRetry)
			if rule, a := b.decide(wait.Add(-time.Second)); rule != "escape" || a.kind != actHold {
				t.Fatalf("before escapeRetry: decide = rule %q action %d, want an escape hold", rule, a.kind)
			}
			if rule, a := b.decide(wait); rule != "supply" || a.kind != actMove {
				t.Fatalf("after escapeRetry: decide = rule %q action %d, want a supply walk", rule, a.kind)
			}
		})
	}
}

// TestSupplyTripWalk checks the trip with no Scroll of Escape: it walks to
// the nearest town at once and ends there, the spot released.
func TestSupplyTripWalk(t *testing.T) {
	b := newTripBrain(t)
	b.supplyTrip.potions = true
	now := at(time.Minute)
	if rule, a := b.decide(now); rule != "supply" || a.kind != actMove {
		t.Fatalf("decide = rule %q action %d, want a supply walk", rule, a.kind)
	}
	if b.spot.held || b.supplyTrip.town != town {
		t.Fatalf("spot held %v, walking to %v: want released, to %v", b.spot.held, b.supplyTrip.town, town)
	}
	b.world.Self.move = stopAt(Point{X: town.X - 500})
	arrive := now.Add(time.Minute)
	if rule, _ := b.decide(arrive); rule == "supply" {
		t.Fatalf("decide in town = rule %q, want the trip over", rule)
	}
	if b.supplyTrip.active() || b.supplyTrip.endedAt != arrive {
		t.Fatalf("trip active %v ended %v in town, want ended at %v", b.supplyTrip.active(), b.supplyTrip.endedAt, arrive)
	}
}

// TestSupplyTripOnlyAtSpot checks that the triggers are read only at the
// bot's spot, every supplyCheck, and not within supplyTripGap of the last
// trip.
func TestSupplyTripOnlyAtSpot(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *Brain)
	}{
		{"not arrived", func(b *Brain) { b.spot.arrived = false }},
		{"no spot", func(b *Brain) { b.leaveSpot() }},
		{"within the gap", func(b *Brain) { b.supplyTrip.endedAt = at(time.Minute).Add(time.Second - supplyTripGap) }},
		{"checked lately", func(b *Brain) { b.supplyTrip.checkAt = at(time.Minute + time.Second) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, soeStack(1))
			b.supplyTrip.potions = true
			tt.edit(b)
			if got := b.supplyDue(at(time.Minute)); got != causeNone {
				t.Fatalf("supplyDue = %d, want none", got)
			}
		})
	}
}
