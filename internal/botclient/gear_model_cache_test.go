package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// cacheSymbol is the dye symbol of the gear model cache tests: INT +1.
const cacheSymbol int32 = 9001

// cacheUserInfo is the UserInfo of the gear model cache tests' bot: a level
// 20 Human Mystic, at level when it is set.
func cacheUserInfo(level int32) UserInfo {
	return UserInfo{
		ObjectID: selfID, Name: "bot7", ClassID: classHumanMystic, BaseClass: classHumanMystic, Level: level,
		RunSpeed: 100, WalkSpeed: 50, Running: true, HP: 10, MaxHP: 20, MP: 30, MaxMP: 40,
	}
}

// cacheSkills are the skills of the gear model cache tests' bot: Spellcraft
// (a passive the model counts) and the heal its support role values.
var cacheSkills = []KnownSkill{{ID: skillSpellcraft, Level: 1, Passive: true}, {ID: skillHeal, Level: 1}}

// newCacheBrain is a model brain of a level 20 Human Mystic support
// knowing cacheSkills, wearing a robe top and carrying robe legs, with its
// gear model made and one more arrangement valued: the model and the
// number of values it holds.
func newCacheBrain(t *testing.T) (*Brain, *gearModel, int) {
	t.Helper()
	b := newModelBrain(t, classHumanMystic, 20, RoleSupport, cacheSkills, nil, wornItem(301, robeTop), carriedPiece(312, robeBottom))
	b.cfg.Travel.setHennas(map[int32]datapack.Henna{cacheSymbol: {SymbolID: cacheSymbol, INT: 1}})
	b.world.Apply(cacheUserInfo(20), t0)
	v := b.valuer()
	if v == nil {
		t.Fatal("no gear model")
	}
	w := b.rawWorn()
	w[slotLegs] = wornItem(312, robeBottom)
	v.value(b.cfg.Travel, &w, false, v.build.Dyes)
	return b, v, len(v.values)
}

// TestGearValuerKeep checks that an event that changes none of the gear
// model's inputs keeps the model and the values it holds (#494).
func TestGearValuerKeep(t *testing.T) {
	keep := []struct {
		name string
		ev   Event
	}{
		{"own HP and MP StatusUpdate", StatusUpdate{ObjectID: selfID, Set: StatusHP | StatusMP | StatusCP, HP: 5, MP: 7, CP: 3}},
		{"own MaxHP StatusUpdate of the same max", StatusUpdate{ObjectID: selfID, Set: StatusHP | StatusMaxHP, HP: 12, MaxHP: 20}},
		{"other's StatusUpdate", StatusUpdate{ObjectID: npcID, Set: StatusMaxHP, MaxHP: 80}},
		{"own move", MoveToLocation{ObjectID: selfID, Dest: Point{X: 100}}},
		{"other's move", MoveToLocation{ObjectID: pcID, Dest: Point{X: 100}}},
		{"CharInfo", CharInfo{ObjectID: pcID, Name: "other"}},
		{"NPCInfo", NPCInfo{ObjectID: npcID, NPCID: 20001}},
		{"SpawnItem", SpawnItem{Item: GroundItem{ObjectID: 600, TemplateID: 57, Count: 10}}},
		{"DropItem", DropItem{DropperID: npcID, Item: GroundItem{ObjectID: 601, TemplateID: 57, Count: 10}}},
		{"DeleteObject", DeleteObject{ObjectID: npcID}},
		{"chat", CreatureSay{ObjectID: pcID, Name: "other", Text: "hi"}},
		{"system message", SystemMessage{ID: 46}},
		{"unequipped InventoryUpdate", InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: carriedPiece(401, robeTop)}}}},
		{"SkillList unchanged", SkillList{Skills: cacheSkills}},
		{"HennaInfo unchanged", HennaInfo{MaxSlots: 3}},
		{"UserInfo unchanged", cacheUserInfo(20)},
	}
	for _, tt := range keep {
		t.Run(tt.name, func(t *testing.T) {
			b, v, n := newCacheBrain(t)
			b.world.Apply(tt.ev, t0)
			if got := b.valuer(); got != v {
				t.Fatalf("gear model after %s: %p, want the same %p", tt.name, got, v)
			}
			if len(v.values) != n {
				t.Errorf("values after %s: %d, want the %d it held", tt.name, len(v.values), n)
			}
		})
	}
}

// TestGearValuerRebuild checks that an event or edit that changes an input
// of the gear model rebuilds it (#494).
func TestGearValuerRebuild(t *testing.T) {
	skills := func(edit func(s []KnownSkill) []KnownSkill) Event {
		return SkillList{Skills: edit(append([]KnownSkill(nil), cacheSkills...))}
	}
	modified := func(it Item) Event { return InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: it}}} }
	enchanted := wornItem(301, robeTop)
	enchanted.Enchant = 2
	rebuild := []struct {
		name string
		ev   Event
		edit func(b *Brain)
	}{
		{name: "equip", ev: modified(wornItem(312, robeBottom))},
		{name: "unequip", ev: modified(carriedPiece(301, robeTop))},
		{name: "enchant", ev: modified(enchanted)},
		{name: "level up", ev: cacheUserInfo(21)},
		{name: "passive added", ev: skills(func(s []KnownSkill) []KnownSkill { return append(s, KnownSkill{ID: 9999, Level: 1, Passive: true}) })},
		{name: "passive disabled", ev: skills(func(s []KnownSkill) []KnownSkill { s[0].Disabled = true; return s })},
		{name: "passive level", ev: skills(func(s []KnownSkill) []KnownSkill { s[0].Level = 2; return s })},
		{name: "heal forgotten", ev: skills(func(s []KnownSkill) []KnownSkill { return s[:1] })},
		{name: "MaxHP", ev: StatusUpdate{ObjectID: selfID, Set: StatusMaxHP, MaxHP: 30}},
		{name: "MaxMP", ev: StatusUpdate{ObjectID: selfID, Set: StatusMaxMP, MaxMP: 50}},
		{name: "MaxLoad", ev: StatusUpdate{ObjectID: selfID, Set: StatusMaxLoad, MaxLoad: 70000}},
		{name: "dye", ev: HennaInfo{MaxSlots: 3, Worn: []WornSymbol{{SymbolID: cacheSymbol, Active: 1}}}},
		{name: "class", edit: func(b *Brain) { b.world.Self.ClassID = 11 }},
		{name: "level", edit: func(b *Brain) { b.world.Self.Level = 21 }},
		{name: "role", edit: func(b *Brain) { b.persona.Role = RoleDamage }},
	}
	for _, tt := range rebuild {
		t.Run(tt.name, func(t *testing.T) {
			b, v, _ := newCacheBrain(t)
			if tt.ev != nil {
				b.world.Apply(tt.ev, t0)
			}
			if tt.edit != nil {
				tt.edit(b)
			}
			if got := b.valuer(); got == v || got == nil {
				t.Errorf("gear model after %s: %p, want a new one (was %p)", tt.name, got, v)
			}
		})
	}
}
