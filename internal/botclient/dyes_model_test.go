package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// modelHennas are hennas.xml's first-profession symbols 1-12 (37000
// adena, +1/-3) and 16 and 20 (87000, +1/-2), with their class lists.
var modelHennas = func() map[int32]datapack.Henna {
	all := []int32{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54, 56}
	wiz := []int32{11, 26, 39}
	notHeal := []int32{1, 4, 7, 11, 19, 22, 26, 32, 35, 39, 45, 47, 54, 56}
	heal := []int32{11, 15, 26, 29, 39, 42, 50}
	m := make(map[int32]datapack.Henna)
	add := func(h datapack.Henna, classes []int32) {
		h.DyeID, h.Classes = 4444+h.SymbolID, classes
		if h.Price == 0 {
			h.Price = 37000
		}
		m[h.SymbolID] = h
	}
	add(datapack.Henna{SymbolID: 1, STR: 1, CON: -3}, all)
	add(datapack.Henna{SymbolID: 2, STR: 1, DEX: -3}, all)
	add(datapack.Henna{SymbolID: 3, STR: -3, CON: 1}, all)
	add(datapack.Henna{SymbolID: 4, CON: 1, DEX: -3}, all)
	add(datapack.Henna{SymbolID: 5, STR: -3, DEX: 1}, all)
	add(datapack.Henna{SymbolID: 6, CON: -3, DEX: 1}, all)
	add(datapack.Henna{SymbolID: 7, INT: 1, MEN: -3}, wiz)
	add(datapack.Henna{SymbolID: 8, INT: 1, WIT: -3}, notHeal)
	add(datapack.Henna{SymbolID: 9, INT: -3, MEN: 1}, wiz)
	add(datapack.Henna{SymbolID: 10, MEN: 1, WIT: -3}, heal)
	add(datapack.Henna{SymbolID: 11, INT: -3, WIT: 1}, notHeal)
	add(datapack.Henna{SymbolID: 12, MEN: -3, WIT: 1}, heal)
	add(datapack.Henna{SymbolID: 16, Price: 87000, CON: 1, DEX: -2}, all)
	add(datapack.Henna{SymbolID: 20, Price: 87000, INT: 1, WIT: -2}, notHeal)
	return m
}()

// newDyeModelBrain is newModelBrain with the model hennas, every dye sold
// at 100 adena, and HennaInfo's slots and symbols.
func newDyeModelBrain(t *testing.T, class int32, role Role, skills []KnownSkill, slots int32, worn []WornSymbol, items ...Item) *Brain {
	t.Helper()
	b := newModelBrain(t, class, 20, role, skills, nil, items...)
	b.cfg.Travel.setHennas(modelHennas)
	for _, h := range modelHennas {
		b.cfg.Travel.cheapest[h.DyeID] = 100
	}
	b.world.Symbols = Symbols{Known: true, MaxSlots: slots, Worn: worn}
	return b
}

// raised are the base stats symbol h raises.
func raised(h datapack.Henna) []string {
	var out []string
	for _, s := range []string{"STR", "CON", "DEX", "INT", "WIT", "MEN"} {
		if dyeStat(h, s) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// TestRowDyes checks the dyes the model draws per row (column 11): a
// Human Wizard draws INT symbols, paying with the stat a nuke needs least;
// a Knight, a Support tank, draws CON symbols; the budget holds the draws
// to what the adena past the gear floor pays; a symbol already worn
// counts towards the +5 cap and the next draw; a persona table still wins.
func TestRowDyes(t *testing.T) {
	nuke := []KnownSkill{{ID: skillSpellcraft, Level: 1, Passive: true}, {ID: skillWindStrike, Level: 1}}
	robes := []Item{wornItem(301, robeTop), wornItem(302, robeBottom)}
	tests := []struct {
		name   string
		class  int32
		role   Role
		skills []KnownSkill
		adena  int32
		worn   []WornSymbol
		table  map[int32][]int32
		stat   string
		want   int
	}{
		{"wizard", 11, RoleDamage, nuke, 1000000, nil, nil, "INT", 2},
		{"wizard, one symbol worn", 11, RoleDamage, nuke, 1000000, []WornSymbol{{SymbolID: 8, Active: 8}}, nil, "INT", 1},
		{"wizard, one fee", 11, RoleDamage, nuke, 60000, nil, nil, "INT", 1},
		{"wizard, no fee", 11, RoleDamage, nuke, 20000, nil, nil, "INT", 0},
		{"knight", 4, RoleSupport, nil, 1000000, nil, nil, "CON", 2},
		{"knight, persona table", 4, RoleSupport, nil, 1000000, nil, map[int32][]int32{4: {1, 2}}, "STR", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newDyeModelBrain(t, tt.class, tt.role, tt.skills, 2, tt.worn, append(slices.Clone(robes), adenaStack(tt.adena))...)
			b.persona.Dyes.Symbols = tt.table
			_, draw := b.dyePlan()
			if len(draw) != tt.want {
				t.Fatalf("draws %v, want %d symbols", draw, tt.want)
			}
			for _, h := range draw {
				if !slices.Contains(raised(h), tt.stat) || !slices.Contains(h.Classes, tt.class) {
					t.Errorf("draws symbol %d (raises %v, classes %v), want a %s symbol of class %d", h.SymbolID, raised(h), h.Classes, tt.stat, tt.class)
				}
			}
			rows, _ := b.dyeWants()
			var dyes int32
			for _, r := range rows {
				dyes += r.count
			}
			if want := int32(hennaDrawDyes * len(draw)); dyes != want {
				t.Errorf("buys %+v for draws %v, want %d dyes", rows, draw, want)
			}
		})
	}
}

// TestDyePlanWithValuables: a merchant with the valuables policy, a free
// dye slot and a carried valuable answers its deposit fees, its valuables
// and its dye plan. The plan's budget reads the gear floor, whose deposit
// fees classify every carried item, whose keep set asked the plan again
// for each of them: the swarm's bot manager died of the stack overflow.
// A non-dye template now asks no plan; a planned dye is still kept.
func TestDyePlanWithValuables(t *testing.T) {
	robes := []Item{wornItem(301, robeTop), wornItem(302, robeBottom)}
	blade := spare(20, dBlade)
	b := newDyeModelBrain(t, 4, RoleSupport, nil, 2, nil, append(slices.Clone(robes), adenaStack(1000000), blade)...)
	m := b.cfg.Travel
	m.crystals = map[int32]crystalYield{dBlade: {grade: 1, crystal: crystalD, count: 200}}
	m.weights = map[int32]int32{dBlade: 1700}
	m.prices = map[int32]int32{dBlade: 250000}
	m.supply = map[int32]itemSupply{dBlade: {perHour: 1}}
	b.persona.Valuables = DefaultValuablePolicy()
	if !b.valuableTemplate(dBlade, 0) {
		t.Fatal("the D blade fails the valuable test's numeric part: no valuable to classify")
	}
	if fees := b.valuableFees(); fees <= 0 {
		t.Errorf("deposit fees %d, want a row's", fees)
	}
	_, draw := b.dyePlan()
	if len(draw) == 0 {
		t.Fatal("no draw planned, want the free slots' symbols")
	}
	if !b.plannedDye(draw[0].DyeID) || !b.keeps(draw[0].DyeID) {
		t.Errorf("planned dye %d not kept", draw[0].DyeID)
	}
	if b.plannedDye(dBlade) {
		t.Error("the D blade is a planned dye")
	}

	// A dye no shop sells, priced and rare enough to pass the numeric
	// test, is still no valuable: the keep test would ask the plan again.
	dye := draw[0].DyeID
	delete(m.cheapest, dye)
	m.prices[dye], m.supply[dye] = 250000, itemSupply{perHour: 1}
	b.world.Apply(ItemList{Items: append(slices.Clone(robes), adenaStack(1000000), blade, Item{ObjectID: 30, TemplateID: dye, Count: 1, SubCategory: 5})}, t0)
	if b.valuableTemplate(dye, 0) {
		t.Error("a dye passes the valuable test")
	}
	b.valuableFees()
	b.dyePlan()
}
