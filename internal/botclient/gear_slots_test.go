package botclient

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Shield, jewelry and class templates of the gear slot tests
// (aCis_datapack/data/xml/items), and one made-up sword.
const (
	leatherShield    int32 = 18
	smallShield      int32 = 19
	buckler          int32 = 20
	necklaceOfMagic  int32 = 118
	necklaceOfKnow   int32 = 906
	apprenticeEar    int32 = 112
	mysticsEar       int32 = 113
	earringStrength  int32 = 114
	magicRing        int32 = 116
	ringOfKnowledge  int32 = 875
	ringOfAnguish    int32 = 876
	willowStaff      int32 = 8
	dagger           int32 = 10
	redSunsetSword   int32 = 981
	cottonRobe       int32 = 427
	testSword24000   int32 = 90001
	classHumanFight  int32 = 0
	classWarrior     int32 = 1
	classRogue       int32 = 7
	classMysticHuman int32 = 10
)

// slotTemplates are gearTemplates with the shields, jewelry and class
// pieces of the gear slot tests.
var slotTemplates = func() map[int32]datapack.Item {
	m := maps.Clone(gearTemplates)
	for _, it := range []datapack.Item{
		{ID: leatherShield, Type: "Armor", BodyPart: "lhand", Price: 39},
		{ID: smallShield, Type: "Armor", BodyPart: "lhand", Price: 638},
		{ID: buckler, Type: "Armor", BodyPart: "lhand", Price: 2780},
		{ID: necklaceOfMagic, Type: "Armor", BodyPart: "neck", Price: 66},
		{ID: necklaceOfKnow, Type: "Armor", BodyPart: "neck", Price: 1080},
		{ID: apprenticeEar, Type: "Armor", BodyPart: "rear;lear", Price: 49},
		{ID: mysticsEar, Type: "Armor", BodyPart: "rear;lear", Price: 811},
		{ID: earringStrength, Type: "Armor", BodyPart: "rear;lear", Price: 3510},
		{ID: magicRing, Type: "Armor", BodyPart: "rfinger;lfinger", Price: 33},
		{ID: ringOfKnowledge, Type: "Armor", BodyPart: "rfinger;lfinger", Price: 540},
		{ID: ringOfAnguish, Type: "Armor", BodyPart: "rfinger;lfinger", Price: 2340},
		{ID: willowStaff, Type: "Weapon", BodyPart: "lrhand", Kind: "BIGBLUNT", Magical: true, Price: 12500},
		{ID: dagger, Type: "Weapon", BodyPart: "rhand", Kind: "DAGGER", Price: 138},
		{ID: redSunsetSword, Type: "Weapon", BodyPart: "lrhand", Kind: "BIGSWORD", Price: 25500},
		{ID: cottonRobe, Type: "Armor", BodyPart: "fullarmor", Kind: "MAGIC", Price: 3550},
		{ID: testSword24000, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 24000},
	} {
		m[it.ID] = it
	}
	return m
}()

// newSlotsBrain is newGearBrain over slotTemplates, of class class.
func newSlotsBrain(t *testing.T, class int32, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, 0, items...)
	b.cfg.Travel.gear, b.cfg.Travel.shopGear = indexGear(slotTemplates, b.cfg.Travel.cheapest)
	b.world.Self.ClassID = class
	return b
}

// wornAt is template id worn, object obj, in body part mask part.
func wornAt(obj, id, part int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: 1, Equipped: true, BodySlot: part}
}

// carried is template id carried, object obj, of body part mask part.
func carried(obj, id, part int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: 1, BodySlot: part}
}

func TestIndexGearSlots(t *testing.T) {
	gear, _ := indexGear(slotTemplates, nil)
	for id, want := range map[int32]gearItem{
		buckler:         {part: partLHand, price: 2780},
		necklaceOfKnow:  {part: partNeck, price: 1080},
		mysticsEar:      {part: partLREar, price: 811},
		ringOfAnguish:   {part: partLRFinger, price: 2340},
		willowStaff:     {part: partLRHand, kind: "BIGBLUNT/MAGIC", price: 12500},
		redSunsetSword:  {part: partLRHand, kind: "BIGSWORD", price: 25500},
		cottonRobe:      {part: partFullArmor, kind: "MAGIC", price: 3550},
		leatherShield:   {part: partLHand, price: 39},
		necklaceOfMagic: {part: partNeck, price: 66},
	} {
		if gear[id] != want {
			t.Errorf("gear[%d] = %+v, want %+v", id, gear[id], want)
		}
	}
	for part, want := range map[int32]gearSlot{
		partLHand: slotShield, partNeck: slotNeck, partLREar: slotEar, partREar: slotEar, partLEar: slotEar,
		partLRFinger: slotFinger, partRFinger: slotFinger, partLFinger: slotFinger,
	} {
		if got := slotOf(part); got != want {
			t.Errorf("slotOf(%#x) = %d, want %d", part, got, want)
		}
	}
}

// TestGearSlotsBeats checks shields, jewelry, the two-handed trade-off and
// the class table's empty slots.
func TestGearSlotsBeats(t *testing.T) {
	sword := wornAt(201, shortSword, partRHand)
	wand := wornAt(201, apprenticeWand, partRHand)
	twoHander := wornAt(201, redSunsetSword, partLRHand)
	shield := func(id int32) Item { return wornAt(204, id, partLHand) }
	ear := func(obj, id int32) Item { return wornAt(obj, id, partLREar) }
	tests := []struct {
		name      string
		class     int32
		noJewelry bool
		noTable   bool
		items     []Item
		id        int32
		want      bool
	}{
		{name: "unarmed fighter takes a sword", class: classHumanFight, id: longSword, want: true},
		{name: "unarmed fighter, a dagger", class: classHumanFight, id: dagger},
		{name: "unarmed rogue takes a dagger", class: classRogue, id: dagger, want: true},
		{name: "unarmed rogue, a sword", class: classRogue, id: longSword},
		{name: "unarmed mystic takes a wand", class: classMysticHuman, id: wandOfAdept, want: true},
		{name: "unarmed mystic, a sword", class: classMysticHuman, id: longSword},
		{name: "unarmed, no class table", class: classHumanFight, noTable: true, id: longSword},
		{name: "no shield without a class table", class: classHumanFight, noTable: true, items: []Item{sword}, id: smallShield},
		{name: "bare mystic takes a robe", class: classMysticHuman, items: []Item{wand}, id: cottonRobe, want: true},
		{name: "bare mystic, heavy armor", class: classMysticHuman, items: []Item{wand}, id: bronzePlate},
		{name: "bare rogue takes a light shirt", class: classRogue, id: leatherShirt22, want: true},
		{name: "bare warrior takes heavy armor", class: classWarrior, id: bronzePlate, want: true},
		{name: "shield with a sword", class: classHumanFight, items: []Item{sword}, id: smallShield, want: true},
		{name: "shield unarmed", class: classHumanFight, id: smallShield, want: true},
		{name: "no shield for a mystic", class: classMysticHuman, items: []Item{wand}, id: smallShield},
		{name: "no shield for a rogue", class: classRogue, id: smallShield},
		{name: "no shield over a two-hander", class: classWarrior, items: []Item{twoHander}, id: smallShield},
		{name: "better shield", class: classHumanFight, items: []Item{sword, shield(leatherShield)}, id: buckler, want: true},
		{name: "cheaper shield", class: classHumanFight, items: []Item{sword, shield(buckler)}, id: smallShield},
		{name: "enchanted shield kept", class: classHumanFight, items: []Item{sword, {ObjectID: 204, TemplateID: leatherShield, Equipped: true, BodySlot: partLHand, Enchant: 1}}, id: buckler},
		{name: "two-hander over sword and shield", class: classWarrior, items: []Item{sword, shield(buckler)}, id: redSunsetSword, want: true},
		{name: "two-hander under sword and carried shield", class: classWarrior, items: []Item{wornAt(201, testSword24000, partRHand), carried(220, buckler, partLHand)}, id: redSunsetSword},
		{name: "two-hander under long sword", class: classWarrior, items: []Item{wornAt(201, longSword, partRHand)}, id: redSunsetSword},
		{name: "two-hander under sword and enchanted shield", class: classWarrior, items: []Item{sword, {ObjectID: 204, TemplateID: buckler, Equipped: true, BodySlot: partLHand, Enchant: 2}}, id: redSunsetSword},
		{name: "no two-hander for a first fighter", class: classHumanFight, items: []Item{sword}, id: redSunsetSword},
		{name: "sword under a two-hander", class: classWarrior, items: []Item{twoHander}, id: testSword24000},
		{name: "sword and carried shield over a two-hander", class: classWarrior, items: []Item{twoHander, carried(220, buckler, partLHand)}, id: testSword24000, want: true},
		{name: "staff over a wand", class: classMysticHuman, items: []Item{wand}, id: willowStaff, want: true},
		{name: "jewelry fills the neck", class: classMysticHuman, id: necklaceOfMagic, want: true},
		{name: "jewelry off", class: classMysticHuman, noJewelry: true, id: necklaceOfMagic},
		{name: "better necklace", class: classMysticHuman, items: []Item{wornAt(205, necklaceOfMagic, partNeck)}, id: necklaceOfKnow, want: true},
		{name: "cheaper necklace", class: classMysticHuman, items: []Item{wornAt(205, necklaceOfKnow, partNeck)}, id: necklaceOfMagic},
		{name: "earring in the free side", class: classMysticHuman, items: []Item{ear(206, earringStrength)}, id: apprenticeEar, want: true},
		{name: "earring over the lesser of a pair", class: classMysticHuman, items: []Item{ear(206, earringStrength), ear(207, apprenticeEar)}, id: mysticsEar, want: true},
		{name: "earring under both of a pair", class: classMysticHuman, items: []Item{ear(206, earringStrength), ear(207, mysticsEar)}, id: apprenticeEar},
		{name: "enchanted lesser earring kept", class: classMysticHuman, items: []Item{ear(206, earringStrength), {ObjectID: 207, TemplateID: apprenticeEar, Equipped: true, BodySlot: partLREar, Enchant: 1}}, id: mysticsEar},
		{name: "ring over the lesser of a pair", class: classMysticHuman, items: []Item{wornAt(208, magicRing, partLRFinger), wornAt(209, ringOfAnguish, partLRFinger)}, id: ringOfKnowledge, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSlotsBrain(t, tt.class, tt.items...)
			b.persona.Gear.Jewelry = !tt.noJewelry
			if tt.noTable {
				b.persona.Gear.Classes = nil
			}
			w := b.worn()
			if got := b.beats(tt.id, &w); got != tt.want {
				t.Fatalf("beats(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestBrainGearPairSwap replaces the lesser ring of a full pair: it takes
// that ring off, then wears the new one in the freed side.
func TestBrainGearPairSwap(t *testing.T) {
	low, high := wornAt(208, magicRing, partLRFinger), wornAt(209, ringOfAnguish, partLRFinger)
	ring := carried(210, ringOfKnowledge, partLRFinger)
	b := newSlotsBrain(t, classMysticHuman, high, low, ring)
	if _, junk := b.junk()[ring.ObjectID]; junk {
		t.Fatal("the ring to wear is junk")
	}
	runTrainerSteps(t, b, []trainerStep{
		{name: "take the lesser ring off", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(low.ObjectID)},
		{name: "wait for it", at: 2 * time.Second},
		{
			name: "wear the new ring", at: 3 * time.Second,
			events:   []Event{InventoryUpdate{Changes: []ItemChange{{ItemModified, carried(208, magicRing, partLRFinger)}}}},
			wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(ring.ObjectID),
		},
		{
			name: "worn", at: 4 * time.Second,
			events: []Event{InventoryUpdate{Changes: []ItemChange{{ItemModified, wornAt(210, ringOfKnowledge, partLRFinger)}}}},
		},
	})
	junk := b.junk()
	if _, ok := junk[low.ObjectID]; !ok || len(junk) != 1 {
		t.Errorf("junk = %v, want the magic ring", junk)
	}
}

// TestBrainGearPairStuck gives up on a ring when the server does not take
// the lesser one off within gearTimeout.
func TestBrainGearPairStuck(t *testing.T) {
	low, high := wornAt(208, magicRing, partLRFinger), wornAt(209, ringOfAnguish, partLRFinger)
	b := newSlotsBrain(t, classMysticHuman, high, low, carried(210, ringOfKnowledge, partLRFinger))
	runTrainerSteps(t, b, []trainerStep{
		{name: "take off", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(low.ObjectID)},
		{name: "waiting", at: time.Second + gearTimeout - time.Millisecond},
		{name: "still worn", at: time.Second + gearTimeout},
		{name: "not again", at: time.Minute},
	})
	if !b.gear.refused[ringOfKnowledge] || b.gear.refused[magicRing] {
		t.Errorf("refused = %v, want the ring of knowledge", b.gear.refused)
	}
}

// TestBrainGearTwoHander wears a two-handed sword over a sword and shield:
// the server takes the shield off, and both are junk.
func TestBrainGearTwoHander(t *testing.T) {
	sword, shield := wornAt(201, shortSword, partRHand), wornAt(204, buckler, partLHand)
	b := newSlotsBrain(t, classWarrior, sword, shield, carried(220, redSunsetSword, partLRHand))
	runTrainerSteps(t, b, []trainerStep{
		{name: "wear the two-hander", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: encodeUseItem(220)},
		{
			name: "worn, shield off", at: 2 * time.Second,
			events: []Event{InventoryUpdate{Changes: []ItemChange{
				{ItemModified, carried(204, buckler, partLHand)},
				{ItemModified, carried(201, shortSword, partRHand)},
				{ItemModified, wornAt(220, redSunsetSword, partLRHand)},
			}}},
		},
	})
	junk := b.junk()
	if _, ok := junk[201]; !ok {
		t.Errorf("junk = %v, want the sword", junk)
	}
	if _, ok := junk[204]; !ok || len(junk) != 2 {
		t.Errorf("junk = %v, want the sword and the shield", junk)
	}
}

// TestGearClassesDefault checks the default class table: no class in two
// entries, the catch-all last, and only armor types the bot indexes.
func TestGearClassesDefault(t *testing.T) {
	classes := defaultGearClasses()
	seen := make(map[int32]int)
	for i, c := range classes {
		if len(c.Classes) == 0 && i != len(classes)-1 {
			t.Errorf("entry %d is for every class but not last", i)
		}
		for _, id := range c.Classes {
			if j, ok := seen[id]; ok {
				t.Errorf("class %d in entries %d and %d", id, j, i)
			}
			seen[id] = i
		}
		if !slices.Contains([]string{"LIGHT", "HEAVY", "MAGIC"}, c.Armor) {
			t.Errorf("entry %d armor %q", i, c.Armor)
		}
		if len(c.Weapons) == 0 {
			t.Errorf("entry %d weapons %v: want some", i, c.Weapons)
		}
		bows := slices.Contains(c.Weapons, "BOW")
		for _, id := range c.Classes {
			if archer := slices.Contains([]int32{9, 24, 37, 92, 102, 109}, id); archer != bows {
				t.Errorf("class %d: bow %v, want %v (archers only)", id, bows, archer)
			}
		}
	}
}

// TestGearClassesShipped checks every class id and weapon and armor type
// of the default class table against a real datapack when
// BOT_DATAPACK_XML names its data/xml directory, and skips otherwise.
func TestGearClassesShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewTravelMap(dp)
	kinds := make(map[string]bool)
	for _, g := range m.gear {
		kinds[g.kind] = true
	}
	files, err := filepath.Glob(filepath.Join(dir, "classes", "*.xml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("class files: %v, %v", files, err)
	}
	classID := regexp.MustCompile(`<class>(?:\s*<!--[^>]*-->)?\s*<set id="(\d+)"`)
	known := make(map[int32]bool)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range classID.FindAllSubmatch(data, -1) {
			id, _ := strconv.Atoi(string(m[1]))
			known[int32(id)] = true
		}
	}
	for i, c := range defaultGearClasses() {
		for _, id := range c.Classes {
			if !known[id] {
				t.Errorf("entry %d: class %d not in the datapack", i, id)
			}
		}
		for _, k := range append([]string{c.Armor}, c.Weapons...) {
			if !kinds[k] {
				t.Errorf("entry %d: no wearable %q in the datapack", i, k)
			}
		}
	}
	t.Logf("classes=%d kinds=%d", len(known), len(kinds))
}
