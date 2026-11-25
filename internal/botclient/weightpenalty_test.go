package botclient

import (
	"reflect"
	"testing"
	"time"
)

// TestWeightBand pins weightBand to the server's bands
// (character_weight_penalty.go refreshWeightPenalty): under 50 %, 66.6 %,
// 80 % and 100 %, then 4; 0 with no weight limit.
func TestWeightBand(t *testing.T) {
	tests := []struct {
		load, maxLoad, want int32
	}{
		{0, 0, 0},
		{500, 0, 0},
		{0, 1000, 0},
		{499, 1000, 0},
		{500, 1000, 1},
		{665, 1000, 1},
		{666, 1000, 2},
		{799, 1000, 2},
		{800, 1000, 3},
		{999, 1000, 3},
		{1000, 1000, 4},
		{1500, 1000, 4},
	}
	for _, tt := range tests {
		if got := weightBand(tt.load, tt.maxLoad); got != tt.want {
			t.Errorf("weightBand(%d, %d) = %d, want %d", tt.load, tt.maxLoad, got, tt.want)
		}
	}
}

// TestSupplyCauseSlowBand checks the trip home in the half-speed band: a
// farmer from 66.6 % goes home with anything to sell, even under ShedMin,
// since every step there is at half speed; under the band ShedMin still
// holds, with nothing to shed it goes to stash its kept or unsold items
// (#449), and with the load trigger off it stays.
func TestSupplyCauseSlowBand(t *testing.T) {
	tests := []struct {
		name string
		item Item
		load int32
		edit func(b *Brain)
		want tripCause
	}{
		{name: "67 % with a little junk", item: lootStack(60, junkMat, 100), load: 67000, want: causeLoad},
		{name: "66.6 % with a little junk", item: lootStack(60, junkMat, 100), load: 66600, want: causeLoad},
		{name: "66.5 % with a little junk", item: lootStack(60, junkMat, 100), load: 66500},
		{name: "67 % with nothing to shed", item: lootStack(60, keptMat, 100), load: 67000, want: causeStash},
		{name: "67 % with no sale", item: lootStack(60, junkMat, 100), load: 67000, edit: func(b *Brain) { b.persona.Shop.SellAt = 0 }, want: causeStash},
		{name: "ReturnLoad 0", item: lootStack(60, junkMat, 100), load: 90000, edit: func(b *Brain) { b.persona.Supply.ReturnLoad = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTripBrain(t, potionStack(20), tt.item)
			b.persona.Shop.Keep = []int32{keptMat}
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

// Worn D-grade pieces of the grade penalty tests.
var (
	wornMail    = Item{ObjectID: 230, TemplateID: salamanderMail, Count: 1, Equipped: true, BodySlot: partFullArmor}
	wornArtisan = Item{ObjectID: 231, TemplateID: artisansSword, Count: 1, Equipped: true, BodySlot: partRHand}
	spareSword  = Item{ObjectID: 232, TemplateID: shortSword, Count: 1, BodySlot: partRHand}
)

// newPenaltyBrain is newGearBrain for a sword wielder whose SkillList
// gives Expertise at level expertise and, when penalty, the grade
// penalty skill.
func newPenaltyBrain(t *testing.T, expertise int32, penalty bool, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, 0, items...)
	b.persona.Gear.Classes = []GearClass{{Weapons: []string{"SWORD"}, Armor: "LIGHT"}}
	var skills []KnownSkill
	if expertise > 0 {
		skills = append(skills, KnownSkill{ID: expertiseSkill, Level: expertise, Passive: true})
	}
	if penalty {
		skills = append(skills, KnownSkill{ID: gradePenaltySkill, Level: 1, Passive: true})
	}
	b.world.Apply(SkillList{Skills: skills}, t0)
	return b
}

// TestGearGradePenalty checks what ruleGear takes off for the grade
// penalty: an over-grade armor piece while the server holds the penalty,
// an over-grade weapon only with a carried one of the bot's grade to
// replace it, nothing within Expertise, without the server's penalty or
// for a piece the server would not take off.
func TestGearGradePenalty(t *testing.T) {
	tests := []struct {
		name      string
		expertise int32
		penalty   bool
		items     []Item
		edit      func(b *Brain)
		want      int32 // the object used, 0 for none
	}{
		{name: "over-grade armor", penalty: true, items: []Item{wornMail}, want: wornMail.ObjectID},
		{name: "no penalty from the server", items: []Item{wornMail}},
		{name: "within Expertise", expertise: 1, penalty: true, items: []Item{wornMail}},
		{name: "weapon with nothing to replace it", penalty: true, items: []Item{wornArtisan}},
		{name: "weapon with a replacement", penalty: true, items: []Item{wornArtisan, spareSword}, want: wornArtisan.ObjectID},
		{name: "armor before the weapon", penalty: true, items: []Item{wornArtisan, spareSword, wornMail}, want: wornMail.ObjectID},
		{name: "refused take-off", penalty: true, items: []Item{wornMail}, edit: func(b *Brain) { b.gear.refused = map[int32]bool{salamanderMail: true} }},
		{name: "gear off", penalty: true, items: []Item{wornMail}, edit: func(b *Brain) { b.persona.Gear.Equip = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newPenaltyBrain(t, tt.expertise, tt.penalty, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			var got int32
			if a, ok := b.ruleGear(at(time.Second)); ok {
				if a.kind != actUseItem {
					t.Fatalf("ruleGear = action %d, want UseItem", a.kind)
				}
				got = a.object
			}
			if got != tt.want {
				t.Fatalf("ruleGear uses %d, want %d", got, tt.want)
			}
		})
	}
}

// TestBrainGearGradePenaltySwap takes an over-grade sword off, waits for
// it to come off, then wears the carried sword of the bot's grade; a
// take-off the server ignores past gearTimeout is given up for good.
func TestBrainGearGradePenaltySwap(t *testing.T) {
	off := InventoryUpdate{Changes: []ItemChange{{ItemModified, Item{ObjectID: wornArtisan.ObjectID, TemplateID: artisansSword, Count: 1, BodySlot: partRHand}}}}
	t.Run("swap", func(t *testing.T) {
		b := newPenaltyBrain(t, 0, true, wornArtisan, spareSword)
		runTrainerSteps(t, b, []trainerStep{
			{name: "take off", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(wornArtisan.ObjectID)},
			{name: "waiting", at: 2 * time.Second},
			{name: "wear the short sword", at: 3 * time.Second, events: []Event{off}, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(spareSword.ObjectID)},
		})
	})
	t.Run("ignored", func(t *testing.T) {
		b := newPenaltyBrain(t, 0, true, wornArtisan, spareSword)
		runTrainerSteps(t, b, []trainerStep{
			{name: "take off", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(wornArtisan.ObjectID)},
			{name: "given up", at: time.Second + gearTimeout},
			{name: "not again", at: time.Minute},
		})
		if !b.gear.refused[artisansSword] {
			t.Errorf("refused = %v, want the artisan's sword", b.gear.refused)
		}
	})
}

// TestGearWantsUnderSlowBand checks that the gear the bot buys never takes
// its load into the half-speed band: a piece that would is left out, the
// lighter ones after it still bought; with no weights known all are.
func TestGearWantsUnderSlowBand(t *testing.T) {
	all := []buyRow{{falchion, 1}, {longSword, 1}, {leatherShirt22, 1}, {woodenGaiters, 1}, {leatherHelmet, 1}}
	tests := []struct {
		name    string
		weights map[int32]int32
		want    []buyRow
	}{
		{
			// 60000 of 100000: 6599 to go, so the 4000 shirt after the two
			// 2000 swords is left out.
			name:    "a piece past the band",
			weights: map[int32]int32{falchion: 2000, longSword: 2000, leatherShirt22: 4000, woodenGaiters: 1000, leatherHelmet: 1000},
			want:    []buyRow{{falchion, 1}, {longSword, 1}, {woodenGaiters, 1}, {leatherHelmet, 1}},
		},
		{name: "all fit", weights: map[int32]int32{falchion: 1}, want: all},
		{name: "no weights", want: all},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGearBrain(t, 0, wornSword, wornShirt, wornPants, adenaStack(300000))
			b.persona.Gear.Buy = true
			b.cfg.Travel.weights = tt.weights
			b.world.Self.Load, b.world.Self.MaxLoad = 60000, 100000
			if got := b.gearWants(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("gearWants = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGearWantsSetUnderSlowBand checks that the planned set is bought only
// when all of it fits under the half-speed band: a set past the band is
// left out whole, never in part.
func TestGearWantsSetUnderSlowBand(t *testing.T) {
	const keep = 10000
	set := []buyRow{{setShield, 1}, {setChest, 1}, {setLegs, 1}, {setHead, 1}}
	tests := []struct {
		name   string
		weight int32
		want   []buyRow
	}{
		// 60000 of 100000: 6599 to go.
		{name: "the set fits", weight: 1500, want: set},
		{name: "each piece fits, the set does not", weight: 2000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSetBrain(t, setShop, wornAt(1, oldShirt, partChest), wornAt(2, oldPants, partLegs), adenaStack(keep+2900))
			b.cfg.Travel.weights = map[int32]int32{setShield: tt.weight, setChest: tt.weight, setLegs: tt.weight, setHead: tt.weight}
			b.world.Self.Load, b.world.Self.MaxLoad = 60000, 100000
			if got := b.gearWants(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("gearWants = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGearPaidUnderSlowBand checks that a gear goal past the half-speed
// band sends the bot to no town visit, as the shop would not buy it.
func TestGearPaidUnderSlowBand(t *testing.T) {
	tests := []struct {
		name    string
		weights map[int32]int32
		want    bool
	}{
		// 60000 of 100000: 6599 to go.
		{name: "the goal fits", weights: map[int32]int32{falchion: 2000}, want: true},
		{name: "the goal is past the band", weights: map[int32]int32{falchion: 7000}},
		{name: "no weights", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGearBrain(t, 0, wornSword, wornShirt, wornPants, adenaStack(300000))
			b.persona.Gear.Buy = true
			b.cfg.Travel.weights = tt.weights
			b.world.Self.Load, b.world.Self.MaxLoad = 60000, 100000
			if goal, _ := b.gearGoal(); !reflect.DeepEqual(goal, []int32{falchion}) {
				t.Fatalf("gearGoal = %v, want [falchion]", goal)
			}
			if got := b.gearPaid(); got != tt.want {
				t.Fatalf("gearPaid = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBrainStatusPenalties checks the load and penalty fields of the
// snapshot (#436, read by the panel, #437): the load, its percent and
// band, the run speed and mode and the grade and death penalties.
func TestBrainStatusPenalties(t *testing.T) {
	tests := []struct {
		name          string
		load, maxLoad int32
		penalty       bool
		want          Status
	}{
		{name: "unknown limit", load: 500, want: Status{Load: 500}},
		{name: "light", load: 30000, maxLoad: 100000, want: Status{Load: 30000, MaxLoad: 100000, Weight: 30}},
		{
			name: "slowed", load: 70500, maxLoad: 100000, penalty: true,
			want: Status{Load: 70500, MaxLoad: 100000, Weight: 70, WeightPenalty: 2, GradePenalty: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newCommandBrain(t, DefaultPersona())
			l := linkBrain(b, standing{})
			s := &b.world.Self
			s.Load, s.MaxLoad, s.RunSpeed, s.Running, s.DeathPenalty = tt.load, tt.maxLoad, 63, true, 2
			if tt.penalty {
				b.world.Apply(SkillList{Skills: []KnownSkill{{ID: gradePenaltySkill, Level: 1, Passive: true}}}, t0)
			}
			b.publishStatus(t0, true)
			st := l.board.load()
			want := tt.want
			want.RunSpeed, want.Running, want.DeathPenalty = 63, true, 2
			got := Status{
				Load: st.Load, MaxLoad: st.MaxLoad, Weight: st.Weight, WeightPenalty: st.WeightPenalty,
				GradePenalty: st.GradePenalty, DeathPenalty: st.DeathPenalty, RunSpeed: st.RunSpeed, Running: st.Running,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("status %+v, want %+v", got, want)
			}
		})
	}
}
