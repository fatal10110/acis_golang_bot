package botclient

import (
	"encoding/binary"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Templates of the shot tests: made-up weapons of grades none and D, and
// the shots of aCis_datapack/data/xml/items.
const (
	ngSword  int32 = 9001
	dSword   int32 = 9002
	dBlade   int32 = 9003
	ngWand   int32 = 9004
	bareWand int32 = 9005

	beginnerSoul   int32 = 5789
	beginnerSpirit int32 = 5790
	dSoulshot      int32 = 1463
	dSpiritshot    int32 = 2510
	blessedNoGrade int32 = 3947

	// shotGrocer is a spawned grocer selling no-grade shots; salesCat
	// sells D-grade ones from list 5720 and has no spawn unless a test
	// gives it one.
	shotGrocer int32 = 30004
	salesCat   int32 = 31075
)

// shotWeaponTemplates are the weapons of the shot tests: a D-grade sword
// scoring less than twice the no-grade one, a D-grade blade more, a
// no-grade wand, and one with no spiritshot room.
var shotWeaponTemplates = map[int32]datapack.Item{
	ngSword:  {ID: ngSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Price: 100000, Soulshots: 1, Spiritshots: 1, Stats: datapack.Stats{PAtk: 30, MAtk: 20, PAtkSpd: 379}},
	dSword:   {ID: dSword, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 1, Price: 150000, Soulshots: 1, Spiritshots: 1, Stats: datapack.Stats{PAtk: 45, MAtk: 30, PAtkSpd: 379}, Enchants: []string{"pAtk", "mAtk"}},
	dBlade:   {ID: dBlade, Type: "Weapon", BodyPart: "rhand", Kind: "SWORD", Grade: 1, Price: 250000, Soulshots: 1, Spiritshots: 1, Stats: datapack.Stats{PAtk: 75, MAtk: 50, PAtkSpd: 379}, Enchants: []string{"pAtk", "mAtk"}},
	ngWand:   {ID: ngWand, Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Magical: true, Price: 50000, Soulshots: 1, Spiritshots: 1, Stats: datapack.Stats{PAtk: 10, MAtk: 20, PAtkSpd: 379}},
	bareWand: {ID: bareWand, Type: "Weapon", BodyPart: "rhand", Kind: "BLUNT", Magical: true, Price: 60000, Soulshots: 1, Stats: datapack.Stats{PAtk: 12, MAtk: 24, PAtkSpd: 379}},
}

// newShotBrain is newGearBrain for class with D-grade expertise, auto
// shots on and the shot weapons, the spawned grocer and gear merchant
// (selling the D-grade sword), and the Sales Cat, spawned when catSpawned,
// on its travel map.
func newShotBrain(t *testing.T, class int32, catSpawned bool, items ...Item) *Brain {
	t.Helper()
	b := newGearBrain(t, 1)
	b.world.Self.ClassID = class
	b.persona.AutoShots = true
	b.persona.Shop.Soulshots, b.persona.Shop.Spiritshots = fixedSoulshots, fixedSpiritshots
	shops, cheapest := indexShops([]datapack.BuyList{
		{ID: 1, NPCID: shotGrocer, Products: []datapack.Product{{ItemID: soulshotNoGrade, Price: 8}, {ItemID: spiritshotNoGrade, Price: 17}, {ItemID: blessedNoGrade, Price: 35}}},
		{ID: 5720, NPCID: salesCat, Products: []datapack.Product{{ItemID: dSoulshot, Price: 20}, {ItemID: dSpiritshot, Price: 40}}},
		{ID: 2, NPCID: gearMerchant, Products: []datapack.Product{{ItemID: dSword, Price: 150000}}},
	})
	spawns := map[int32][]Point{shotGrocer: {{X: 5000}}, gearMerchant: {{X: 5000}}}
	if catSpawned {
		spawns[salesCat] = []Point{{X: 6000}}
	}
	gear, shopGear := indexGear(shotWeaponTemplates, cheapest)
	b.cfg.Travel = &TravelMap{
		spawns: spawns, shops: shops, cheapest: cheapest, gear: gear, shopGear: shopGear,
		shots: builtinShots, weapons: indexWeapons(shotWeaponTemplates), spawnedSold: spawnedSold(shops, spawns),
	}
	b.world.Apply(ItemList{Items: items}, t0)
	return b
}

// Weapons and shots carried by the shot tests.
func wielded(obj, id int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: 1, Equipped: true, BodySlot: partRHand}
}

func spare(obj, id int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: 1, BodySlot: partRHand}
}

func shotStack(obj, id, n int32) Item {
	return Item{ObjectID: obj, TemplateID: id, Count: n, SubCategory: 5}
}

// toggle is a RequestAutoSoulShot the bot sent.
type toggle struct {
	id     int32
	enable bool
}

// runShots sends what ruleShots decides at now until it holds, and
// returns the toggles it sent.
func runShots(t *testing.T, b *Brain, out *recordSender, now time.Time) []toggle {
	t.Helper()
	out.frames = nil
	for range 5 {
		a, ok := b.ruleShots(now)
		if !ok {
			break
		}
		if _, err := b.send(a, now); err != nil {
			t.Fatal(err)
		}
	}
	var got []toggle
	for _, f := range out.frames {
		if f[0] != opGameClientExtended || binary.LittleEndian.Uint16(f[1:]) != opGameRequestAutoSoulShot {
			continue
		}
		got = append(got, toggle{int32(binary.LittleEndian.Uint32(f[3:])), binary.LittleEndian.Uint32(f[7:]) == 1})
	}
	return got
}

// TestShotToggles checks the shot whose auto use the bot turns on, beginners'
// ones first, blessed spiritshots before plain ones, and the ones it turns
// off.
func TestShotToggles(t *testing.T) {
	tests := []struct {
		name  string
		class int32
		items []Item
		on    []int32
		want  []toggle
	}{
		{
			name: "D weapon: D shots on, no-grade ones off", class: classHumanFight,
			items: []Item{wielded(1, dSword), shotStack(10, soulshotNoGrade, 500), shotStack(11, dSoulshot, 500)},
			on:    []int32{soulshotNoGrade},
			want:  []toggle{{dSoulshot, true}, {soulshotNoGrade, false}},
		},
		{
			name: "beginners' shots first", class: classHumanFight,
			items: []Item{wielded(1, ngSword), shotStack(10, soulshotNoGrade, 500), shotStack(11, beginnerSoul, 200)},
			want:  []toggle{{beginnerSoul, true}},
		},
		{
			name: "mystic: blessed before plain", class: classHumanMystic,
			items: []Item{wielded(1, ngWand), shotStack(10, spiritshotNoGrade, 100), shotStack(11, blessedNoGrade, 100)},
			on:    []int32{spiritshotNoGrade},
			want:  []toggle{{blessedNoGrade, true}, {spiritshotNoGrade, false}},
		},
		{
			name: "mystic: plain without blessed", class: classHumanMystic,
			items: []Item{wielded(1, ngWand), shotStack(10, spiritshotNoGrade, 100), shotStack(11, soulshotNoGrade, 100)},
			want:  []toggle{{spiritshotNoGrade, true}},
		},
		{
			name: "mystic weapon without spiritshot room", class: classHumanMystic,
			items: []Item{wielded(1, bareWand), shotStack(10, spiritshotNoGrade, 100)},
			on:    []int32{spiritshotNoGrade},
			want:  []toggle{{spiritshotNoGrade, false}},
		},
		{
			name: "unarmed", class: classHumanFight,
			items: []Item{shotStack(10, soulshotNoGrade, 500)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newShotBrain(t, tt.class, false, tt.items...)
			out := b.out.(*recordSender)
			for _, id := range tt.on {
				b.world.Apply(ExAutoSoulShot{TemplateID: id, Enabled: true}, t0)
			}
			if got := runShots(t, b, out, at(time.Second)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toggles = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShotToggleBack: a template the bot turned off is turned on again
// when the weapon it charges is wielded again, with no wait for shotRetry,
// and an answered request is not repeated.
func TestShotToggleBack(t *testing.T) {
	soul, dSoul := shotStack(10, soulshotNoGrade, 500), shotStack(11, dSoulshot, 500)
	b := newShotBrain(t, classHumanFight, false, wielded(1, dSword), spare(2, ngSword), soul, dSoul)
	out := b.out.(*recordSender)
	b.world.Apply(ExAutoSoulShot{TemplateID: soulshotNoGrade, Enabled: true}, t0)
	if got, want := runShots(t, b, out, at(time.Second)), []toggle{{dSoulshot, true}, {soulshotNoGrade, false}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toggles = %v, want %v", got, want)
	}
	b.world.Apply(ExAutoSoulShot{TemplateID: dSoulshot, Enabled: true}, at(time.Second))
	b.world.Apply(ExAutoSoulShot{TemplateID: soulshotNoGrade}, at(time.Second))
	if got := runShots(t, b, out, at(time.Minute)); got != nil {
		t.Fatalf("answered toggles repeated: %v", got)
	}
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, spare(1, dSword)}, {ItemModified, wielded(2, ngSword)}}}, at(time.Minute))
	if got, want := runShots(t, b, out, at(time.Minute+time.Second)), []toggle{{soulshotNoGrade, true}, {dSoulshot, false}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toggles after the switch = %v, want %v", got, want)
	}
}

// TestShotWants checks the shots the shop errand buys: the ones the
// wielded weapon and the one worn next charge, from a spawned merchant,
// and no shot neither can use.
func TestShotWants(t *testing.T) {
	tests := []struct {
		name       string
		class      int32
		catSpawned bool
		items      []Item
		edit       func(*Brain)
		want       []buyRow
	}{
		{
			name: "no-grade weapon", class: classHumanFight,
			items: []Item{wielded(1, ngSword), shotStack(10, soulshotNoGrade, 50)},
			want:  []buyRow{{soulshotNoGrade, 450}},
		},
		{
			name: "beginners' shots count", class: classHumanFight,
			items: []Item{wielded(1, ngSword), shotStack(11, beginnerSoul, 200)},
		},
		{
			name: "D weapon, D shots on an unspawned list only", class: classHumanFight,
			items: []Item{wielded(1, dSword)},
		},
		{
			name: "D weapon, D shots from a spawned merchant", class: classHumanFight, catSpawned: true,
			items: []Item{wielded(1, dSword)},
			want:  []buyRow{{dSoulshot, 500}},
		},
		{
			name: "next weapon's shots too", class: classHumanFight, catSpawned: true,
			items: []Item{wielded(1, ngSword), spare(2, dSword), shotStack(10, soulshotNoGrade, 500)},
			want:  []buyRow{{dSoulshot, 500}},
		},
		{
			name: "persona restock of unusable shots", class: classHumanFight, catSpawned: true,
			items: []Item{wielded(1, ngSword), shotStack(10, soulshotNoGrade, 500)},
			edit: func(b *Brain) {
				b.persona.Shop.Restock = []Restock{{Item: dSoulshot, Min: 100, Max: 500}, {Item: spiritshotNoGrade, Min: 100, Max: 500}}
			},
		},
		{
			name: "mystic: blessed spiritshots", class: classHumanMystic,
			items: []Item{wielded(1, ngWand)},
			want:  []buyRow{{blessedNoGrade, 200}},
		},
		{
			name: "mystic: plain spiritshots where no blessed are sold", class: classHumanMystic,
			items: []Item{wielded(1, ngWand), shotStack(10, spiritshotNoGrade, 20)},
			edit:  func(b *Brain) { delete(b.cfg.Travel.spawnedSold, blessedNoGrade) },
			want:  []buyRow{{spiritshotNoGrade, 180}},
		},
		{
			name: "mystic weapon without spiritshot room", class: classHumanMystic,
			items: []Item{wielded(1, bareWand)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newShotBrain(t, tt.class, tt.catSpawned, tt.items...)
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.shopWants(true); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("shopWants = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShotJunk checks which carried shots are junk: the ones no weapon the
// bot wields or wears next charges, beginners' ones and the persona's
// Shots aside.
func TestShotJunk(t *testing.T) {
	carriedShots := []Item{
		shotStack(10, soulshotNoGrade, 500), shotStack(11, beginnerSoul, 100), shotStack(12, beginnerSpirit, 100),
		shotStack(13, dSoulshot, 20), shotStack(14, spiritshotNoGrade, 100),
	}
	tests := []struct {
		name       string
		catSpawned bool
		items      []Item
		shots      []int32
		want       []int32
	}{
		{name: "default persona", items: []Item{wielded(1, ngSword)}, want: []int32{dSoulshot, spiritshotNoGrade}},
		{name: "persona keeps its shots", items: []Item{wielded(1, ngSword)}, shots: []int32{dSoulshot}, want: []int32{spiritshotNoGrade}},
		{name: "D weapon worn next", catSpawned: true, items: []Item{wielded(1, ngSword), spare(2, dSword)}, want: []int32{spiritshotNoGrade}},
		{name: "D weapon wielded", catSpawned: true, items: []Item{wielded(1, dSword)}, want: []int32{soulshotNoGrade, spiritshotNoGrade}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newShotBrain(t, classHumanFight, tt.catSpawned, append(slices.Clone(tt.items), carriedShots...)...)
			b.persona.Shots = tt.shots
			var got []int32
			for _, it := range b.junk() {
				if _, shot := builtinShots[it.TemplateID]; shot {
					got = append(got, it.TemplateID)
				}
			}
			slices.Sort(got)
			if want := slices.Sorted(slices.Values(tt.want)); !slices.Equal(got, want) {
				t.Errorf("junk shots = %v, want %v", got, want)
			}
		})
	}
}

// TestShotFactor checks the shot factor in weapon choice: a D-grade weapon
// scoring less than twice the no-grade one wielded wins only when the bot
// can keep it charged, never from an unspawned merchant's list or a few
// looted shots.
func TestShotFactor(t *testing.T) {
	tests := []struct {
		name       string
		catSpawned bool
		items      []Item
		id         int32
		want       bool
	}{
		{name: "no D shots obtainable", items: []Item{wielded(1, ngSword)}, id: dSword},
		{name: "D shots from a spawned merchant", catSpawned: true, items: []Item{wielded(1, ngSword)}, id: dSword, want: true},
		{name: "a few looted D shots", items: []Item{wielded(1, ngSword), shotStack(11, dSoulshot, 99)}, id: dSword},
		{name: "D shots carried at the target", items: []Item{wielded(1, ngSword), shotStack(11, dSoulshot, 100)}, id: dSword, want: true},
		{name: "D weapon wins on its own", items: []Item{wielded(1, ngSword)}, id: dBlade, want: true},
		{name: "a better weapon firing the wielded one's shots", items: []Item{wielded(1, dSword), shotStack(11, dSoulshot, 50)}, id: dBlade, want: true},
		{name: "no factor", items: []Item{wielded(1, ngSword)}, id: dSword, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newShotBrain(t, classHumanFight, tt.catSpawned, tt.items...)
			if tt.name == "no factor" {
				b.persona.Gear.ShotFactor = 0
			}
			w := b.worn()
			if got := b.beats(tt.id, &w); got != tt.want {
				t.Errorf("beats(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestShotFactorBuy: the gear errand buys no D-grade weapon it cannot keep
// charged, the Sales Cat's unspawned list counting for nothing.
func TestShotFactorBuy(t *testing.T) {
	for _, spawned := range []bool{false, true} {
		b := newShotBrain(t, classHumanFight, spawned, wielded(1, ngSword), adenaStack(1000000))
		got := slices.ContainsFunc(b.gearWants(), func(r buyRow) bool { return r.item == dSword })
		if got != spawned {
			t.Errorf("Sales Cat spawned %v: buys the D sword %v, want %v", spawned, got, spawned)
		}
	}
}

// TestShotFallback: with D shots carried at the target the bot wears the D
// weapon, keeps the no-grade one and its shots, fires the D shots to the
// last one, and switches back once they are gone; a few looted D shots
// switch nothing.
func TestShotFallback(t *testing.T) {
	soul := shotStack(10, soulshotNoGrade, 500)
	b := newShotBrain(t, classHumanFight, false, wielded(1, ngSword), spare(2, dSword), soul, shotStack(11, dSoulshot, 50))
	if a, ok := b.ruleGear(at(time.Second)); ok {
		t.Fatalf("a few looted D shots: gear %+v, want none", a)
	}

	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, shotStack(11, dSoulshot, 100)}}}, at(2*time.Second))
	a, ok := b.ruleGear(at(2 * time.Second))
	if !ok || a.object != 2 {
		t.Fatalf("D shots at the target: gear %+v %v, want the D sword used", a, ok)
	}
	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, spare(1, ngSword)}, {ItemModified, wielded(2, dSword)}}}, at(3*time.Second))
	junk := slices.Collect(maps.Keys(b.junk()))
	if slices.Contains(junk, 1) || slices.Contains(junk, 10) {
		t.Errorf("junk %v holds the no-grade sword or its shots", junk)
	}
	if a, ok := b.ruleGear(at(4 * time.Second)); ok {
		t.Fatalf("D shots still at the target: gear %+v, want none", a)
	}

	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, shotStack(11, dSoulshot, 99)}}}, at(5*time.Second))
	if a, ok := b.ruleGear(at(5 * time.Second)); ok {
		t.Fatalf("D shots under the target, D sword wielded: gear %+v, want none", a)
	}
	junk = slices.Collect(maps.Keys(b.junk()))
	for _, obj := range []int32{1, 2, 10, 11} {
		if slices.Contains(junk, obj) {
			t.Errorf("D shots under the target: junk %v holds object %d", junk, obj)
		}
	}

	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemRemoved, shotStack(11, dSoulshot, 0)}}}, at(6*time.Second))
	a, ok = b.ruleGear(at(6 * time.Second))
	if !ok || a.object != 1 {
		t.Fatalf("D shots gone: gear %+v %v, want the no-grade sword used", a, ok)
	}
}

// TestShotReOn: a template the bot turned off and then asks on again is
// asked again after shotRetry while the server leaves the request
// unanswered, and once the server reports it on, a stock-out disable keeps
// it off.
func TestShotReOn(t *testing.T) {
	soul, dSoul := shotStack(10, soulshotNoGrade, 500), shotStack(11, dSoulshot, 500)
	b := newShotBrain(t, classHumanFight, false, wielded(1, dSword), spare(2, ngSword), soul, dSoul)
	out := b.out.(*recordSender)
	answer := func(id int32, on bool, now time.Time) {
		t.Helper()
		if err := b.handle([]Event{ExAutoSoulShot{TemplateID: id, Enabled: on}}, now); err != nil {
			t.Fatal(err)
		}
	}
	answer(soulshotNoGrade, true, t0)
	answer(dSoulshot, true, t0)
	if got, want := runShots(t, b, out, at(time.Second)), []toggle{{soulshotNoGrade, false}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toggles = %v, want %v", got, want)
	}
	answer(soulshotNoGrade, false, at(time.Second))

	b.world.Apply(InventoryUpdate{Changes: []ItemChange{{ItemModified, spare(1, dSword)}, {ItemModified, wielded(2, ngSword)}}}, at(time.Minute))
	switched := at(time.Minute)
	if got, want := runShots(t, b, out, switched), []toggle{{soulshotNoGrade, true}, {dSoulshot, false}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toggles after the switch = %v, want %v", got, want)
	}
	answer(dSoulshot, false, switched)
	if got := runShots(t, b, out, switched.Add(shotRetry-time.Millisecond)); got != nil {
		t.Fatalf("unanswered re-on repeated before shotRetry: %v", got)
	}
	if got, want := runShots(t, b, out, switched.Add(shotRetry)), []toggle{{soulshotNoGrade, true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unanswered re-on at shotRetry: toggles %v, want %v", got, want)
	}

	answer(soulshotNoGrade, true, switched.Add(shotRetry))
	answer(soulshotNoGrade, false, switched.Add(2*shotRetry))
	if got := runShots(t, b, out, switched.Add(3*shotRetry)); got != nil {
		t.Fatalf("server-disabled template asked again: %v", got)
	}
}
