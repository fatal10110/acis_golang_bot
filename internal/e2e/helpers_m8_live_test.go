package e2e

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Helpers of the M8 scenarios (#180): farmers with the travel map, the
// consumables they carry and use, and the class master and Symbol Maker
// visits.

// Templates, skills and messages of the M8 scenarios (aCis_datapack/data/xml).
const (
	// falchion is a no-grade one-handed sword (P.Atk 31), 2 soulshots a
	// hit like the D-grade saber (P.Atk 40, crystallize_live_test.go):
	// charged, the falchion's score doubles (GearPolicy.ShotFactor 2) past
	// the saber's, so a fighter with no D shots, which no merchant sells
	// (soulshotD, D14 b), keeps it.
	falchion int32 = 68
	// scrollOfEscape is Scroll of Escape (weight 120, price 400).
	scrollOfEscape int32 = 736
	// alacrityPotion is Potion of Alacrity: skill 2012 level 1, stack
	// type attack_time_down order 1, 20 minutes, the fighters' buff potion
	// (GearClass.BuffPotion).
	alacrityPotion int32 = 735
	alacritySkill  int32 = 2012
	// skillHaste is Haste: level 1 is of the potion's stack type and
	// order, target one, 39 MP a cast (skills/1000-1099.xml).
	skillHaste int32 = 1086
	// classMaster is Hasha, the class master (D15, acis_golang#5419).
	classMaster int32 = 50000
	// The Human Fighter's first and second professions of the default
	// build table (supportBuilds): Warrior, then Gladiator.
	classWarrior   int32 = 1
	classGladiator int32 = 2
	// The dye table the dye scenario pins for a Warrior (1 STR+1 CON-3, 6
	// DEX+1 CON-3, fee 37000) and a Gladiator (37 and 42, the same at
	// 87000), and their dyes, 10 a draw (hennas.xml, henna.go).
	symbolStr1, symbolDex1 int32 = 1, 6
	symbolStr3, symbolDex3 int32 = 37, 42
	dyeStr1, dyeDex1       int32 = 4445, 4450
	dyeStr3, dyeDex3       int32 = 4481, 4486
	dyesPerDraw            int32 = 10
	// dyesReturned is how many of its dye a symbol's removal hands back
	// (server symbolmaker.go, henna.RemoveAmount).
	dyesReturned int32 = 5
	// msgSymbolAdded and msgSymbolDeleted are SystemMessages 877 and 878
	// (network/henna.go).
	msgSymbolAdded   int32 = 877
	msgSymbolDeleted int32 = 878
)

var (
	// tiMaster is the class master's spawn in Talking Island village
	// (acis_golang PR #5452, spawnlist/custom_classmaster.xml).
	tiMaster = botclient.Point{X: -83326, Y: 242864, Z: -3720}
	// gludinSymbols stands between Colleen (30209, -81908;152077), who
	// sells the dyes, and Marsden (31046, -81520;152256), the Symbol Maker
	// nearest Talking Island (spawnlist/17_22.xml).
	gludinSymbols = botclient.Point{X: -81700, Y: 152150, Z: -3122}
)

// with is items and a placeholder entry for each of ids: strip destroys
// every carried item of a template items holds.
func with(items map[int32]datapack.Item, ids ...int32) map[int32]datapack.Item {
	out := maps.Clone(items)
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			out[id] = datapack.Item{Name: fmt.Sprint("item ", id)}
		}
	}
	return out
}

// consumables are the templates a supply scenario starts without: shots,
// healing potions, Scrolls of Escape, buff potions and adena.
var consumables = []int32{soulshotNG, soulshotD, healingPotion, lesserHealingPotion, scrollOfEscape, alacrityPotion, adena}

// runFieldFarmer runs a human fighter of account farming the datapack's
// spots with the travel map, as cmd/bots' farmers do: the supply trip, the
// town preparation and the buff potion need both. tune adjusts
// DefaultPersona.
func runFieldFarmer(t *testing.T, ctx context.Context, tg *Target, account string, tune func(*botclient.Persona)) *farmer {
	t.Helper()
	travel := travelMap(t, tg)
	spots := spotRegistry(t, tg)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			if tune != nil {
				tune(&p)
			}
			return p
		},
		brain: func(cfg *botclient.BrainConfig) { cfg.Spots, cfg.Travel = spots, travel },
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(f.digest())
			t.Log(f.visits())
		}
	})
	return f
}

// lowSpots lets a level 20 bot farm the newbie spots next to its village:
// the scenarios level a bot up with the GM, and a spot of its level is a
// gatekeeper trip away.
func lowSpots(p *botclient.Persona) {
	p.Spots.Above, p.Spots.Outlevel = 40, 60
}

// errandsOff leaves a level 20 farmer to its spot and supplies: no class
// master, dyes, trainer, Newbie Guide or gear purchase.
func errandsOff(p *botclient.Persona) {
	// Empty tables, not nil: a nil one is the build's (Persona).
	p.ClassChange = botclient.ClassChangePolicy{Builds: map[int32]int32{}}
	p.Dyes = botclient.DyePolicy{Symbols: map[int32][]int32{}}
	p.Newbie = botclient.NewbiePolicy{}
	p.LearnSkills = map[int32][]int32{}
	p.Gear.Buy = false
}

// levelTo has the GM set f's level to level unless it has it already, and
// waits until the bot's status shows it.
func levelTo(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, level int32) {
	t.Helper()
	if f.bot.Status().Level == level {
		return
	}
	gmOn(t, ctx, gm, f, "set level "+strconv.Itoa(int(level)))
	waitFor(t, ctx, fmt.Sprintf("%s at level %d", f.name, level), func() bool { return f.bot.Status().Level == level })
}

// objectsOf are the object ids every inventory event of seen showed of
// template tmpl.
func objectsOf(seen []Seen, tmpl int32) map[int32]bool {
	ids := make(map[int32]bool)
	for _, s := range seen {
		one := make(map[int32]botclient.Item)
		applyInventory(one, s.Event)
		for id, it := range one {
			if it.TemplateID == tmpl {
				ids[id] = true
			}
		}
	}
	return ids
}

// usedSince is the UseItem packets f sent of an item of template tmpl at
// or after from.
func (f *farmer) usedSince(tmpl int32, from time.Time) []Sent {
	ids := objectsOf(f.watch.Seen(0), tmpl)
	var out []Sent
	for _, p := range sentOf(f.watch.Sent(), botclient.OpUseItem) {
		if ids[p.Target] && !p.At.Before(from) {
			out = append(out, p)
		}
	}
	return out
}

// effect reports whether f's last AbnormalStatusUpdate lists skill.
func (f *farmer) effect(skill int32) bool {
	seen := f.watch.Seen(0)
	for i := len(seen) - 1; i >= 0; i-- {
		if u, ok := seen[i].Event.(botclient.AbnormalStatusUpdate); ok {
			for _, e := range u.Effects {
				if e.SkillID == skill {
					return true
				}
			}
			return false
		}
	}
	return false
}

// autoShot reports whether the server's last ExAutoSoulShot to f for tmpl
// turned its auto use on.
func (f *farmer) autoShot(tmpl int32) bool {
	seen := f.watch.Seen(0)
	for i := len(seen) - 1; i >= 0; i-- {
		if e, ok := seen[i].Event.(botclient.ExAutoSoulShot); ok && e.TemplateID == tmpl {
			return e.Enabled
		}
	}
	return false
}

// wields reports whether f's inventory shows an item of tmpl worn.
func (f *farmer) wields(tmpl int32) bool {
	return worn(inventory(f.watch.Seen(0)))[tmpl] > 0
}

// carries is how many of tmpl f's inventory shows.
func (f *farmer) carries(tmpl int32) int32 {
	return carriedOf(inventory(f.watch.Seen(0)), tmpl)
}

// gmGiveAdena tops f, the GM's target, up to at least want adena in one
// give, which the GM's scope bounds (maxAdenaGive): every fee here is
// under it.
func gmGiveAdena(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, want int32) {
	t.Helper()
	if have := f.carries(adena); have < want {
		gmGive(t, ctx, gm, f, adena, min(want-have, maxAdenaGive))
	}
}

// freshAccount is an account of group no earlier run used: a class change
// cannot be undone, so a scenario that changes class starts from a new
// character each run.
func freshAccount(tg *Target, group string) string {
	name, _ := tg.Account(group, int(time.Now().Unix()%10_000_000))
	return name
}
