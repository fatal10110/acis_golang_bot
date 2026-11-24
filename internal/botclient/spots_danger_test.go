package botclient

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The combat stats of Talking Island's aggressive monsters (npcs
// 20000-20999.xml): each one's level, its spawn total at the spot and its
// template's hp, pAtk, pDef, atkSpd, crit, str, con and dex.
var (
	werewolfHunter    = datapack.Combat{HP: 163, PAtk: 21, PDef: 55, AtkSpd: 247.42, Crit: 1, STR: 40, CON: 43, DEX: 30}
	werewolfChieftain = datapack.Combat{HP: 199, PAtk: 25.2, PDef: 60, AtkSpd: 247.42, Crit: 1, STR: 40, CON: 43, DEX: 30}
	goblinScout       = datapack.Combat{HP: 125, PAtk: 15.12, PDef: 55, AtkSpd: 247.42, Crit: 1, STR: 40, CON: 43, DEX: 30}
	orcFighter        = datapack.Combat{HP: 163, PAtk: 18.48, PDef: 61, AtkSpd: 247.42, Crit: 4, STR: 40, CON: 43, DEX: 30}
	orcLieutenant     = datapack.Combat{HP: 185, PAtk: 19.32, PDef: 70, AtkSpd: 247.42, Crit: 4, STR: 40, CON: 43, DEX: 30}
	giantToad         = datapack.Combat{HP: 80, PAtk: 10.92, PDef: 48, AtkSpd: 247.42, Crit: 8, STR: 40, CON: 43, DEX: 30}
)

// The pack spots of #294 (spawnlist 16_25.xml, 17_25.xml): the werewolves
// of gludio31_1625_10m1 that killed a level 14 fighter, the orcs of
// gludio31_1625_01m1 it farmed until its potions ran out, and the toads of
// gludio32_1725_27m1 and gludio32_1725_28m1 a level 6 one farms.
var (
	tiWolves = func() datapack.Spot {
		s := tiPack
		s.Mobs = []datapack.Mob{{Level: 10, Count: 5, Combat: werewolfHunter}, {Level: 12, Count: 5, Combat: werewolfChieftain}}
		return s
	}()
	tiOrcs01 = datapack.Spot{
		MinLevel: 8, MaxLevel: 11, AggroLevel: 11, Aggressive: 11, AggroRange: 1000, ClanRange: 300, RunSpeed: 108.9, Area: 1.297e7,
		Mobs: []datapack.Mob{{Level: 8, Count: 3, Combat: goblinScout}, {Level: 10, Count: 4, Combat: orcFighter}, {Level: 11, Count: 4, Combat: orcLieutenant}},
	}
	tiToads27 = datapack.Spot{
		MinLevel: 5, MaxLevel: 5, AggroLevel: 5, Aggressive: 5, AggroRange: 1000, ClanRange: 300, RunSpeed: 99, Area: 3.15e6,
		Mobs: []datapack.Mob{{Level: 5, Count: 5, Combat: giantToad}},
	}
	tiToads28 = datapack.Spot{
		MinLevel: 5, MaxLevel: 5, AggroLevel: 5, Aggressive: 4, AggroRange: 1000, ClanRange: 300, RunSpeed: 99, Area: 2.528e6,
		Mobs: []datapack.Mob{{Level: 5, Count: 4, Combat: giantToad}},
	}
)

// squire is a human fighter (class 0: STR 40, CON 43, DEX 30, base pAtk 4
// and pDef 80) of level in Squire's Sword, Shirt and Pants, as its
// UserInfo carries it (the server's character stats: the sword's pAtk 6
// and attack speed 379, the shirt's and pants' pDef 33 and 20 less the 31
// and 18 a fighter's chest and legs take off, by STR, DEX and level).
type squire struct {
	level, pAtk, pDef, evasion, maxHP int32
}

var (
	squire14 = squire{level: 14, pAtk: 7, pDef: 86, evasion: 46, maxHP: 385}
	squire6  = squire{level: 6, pAtk: 6, pDef: 79, evasion: 38, maxHP: 221}
)

// newSquireBrain is a brain of the default persona playing s, carrying
// potions Lesser Healing Potions (skill 2031: 16 HP every 2 seconds, 7
// times).
func newSquireBrain(t *testing.T, s squire, potions int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	p.Potions = []int32{lesserHealingPotion}
	b, _ := newTestBrain(t, p)
	setSquire(b, s, potions)
	return b
}

func setSquire(b *Brain, s squire, potions int32) {
	self := &b.world.Self
	self.Level, self.PAtk, self.PDef, self.Evasion, self.Accuracy, self.PAtkSpd = s.level, s.pAtk, s.pDef, s.evasion, s.evasion, 416
	self.HP, self.MaxHP = s.maxHP, s.maxHP
	b.cfg.Travel = &TravelMap{heals: map[int32]potionHeal{lesserHealingPotion: {total: 112, rate: 8}}}
	b.world.Apply(ItemList{Items: []Item{{ObjectID: 0x30000011, TemplateID: lesserHealingPotion, Count: potions}}}, t0)
}

// TestMobFight checks one monster's fight against a level 14 squire: a
// Werewolf Chieftain (level 12) has 314 HP (199 by CON 43's 1.58), P.Atk
// 30.5 (25.2 by STR 40's 1.20 and level 12's 1.01) and P.Def 60.6, swings
// at 272 (247.42 by DEX 30's 1.10), lands 87.7% of its swings on evasion
// 46 and 1.1% of them critical: 27.3 a hit on P.Def 86, 13.2 a second.
// The squire's 7 P.Atk hits it for 8.9, 92.3% of the time, every 1.2 s.
func TestMobFight(t *testing.T) {
	tests := []struct {
		name      string
		self      squire
		mob       datapack.Mob
		kill, dps float64
	}{
		{"werewolf chieftain", squire14, datapack.Mob{Level: 12, Combat: werewolfChieftain}, 46.05, 13.20},
		{"giant toad", squire6, datapack.Mob{Level: 5, Combat: giantToad}, 16.4, 6.4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			self := &SelfState{Level: tt.self.level, PAtk: tt.self.pAtk, PDef: tt.self.pDef, Evasion: tt.self.evasion, Accuracy: tt.self.evasion, PAtkSpd: 416}
			kill, dps := mobFight(self, &tt.mob)
			if math.Abs(kill-tt.kill) > 0.1 || math.Abs(dps-tt.dps) > 0.1 {
				t.Fatalf("mobFight = %.2f s, %.2f a second, want %.2f s, %.2f", kill, dps, tt.kill, tt.dps)
			}
		})
	}
}

// TestPackRisk checks SpotPolicy.PackRisk passes over the werewolf pack a
// level 14 squire died at before any death there, keeps its orcs while its
// potions last, and keeps the toad spots for a level 6 one with none
// (#294).
func TestPackRisk(t *testing.T) {
	with := func(s squire, f func(*squire)) squire { f(&s); return s }
	tests := []struct {
		name    string
		self    squire
		potions int32
		spot    datapack.Spot
		// risk is the spot's packRisk, fit whether it fits at
		// defaultSpotPolicy's PackRisk.
		risk float64
		fit  bool
	}{
		{"level 14 at the werewolves, 13 potions", squire14, 13, tiWolves, 3.31, false},
		{"level 14 at the orcs, 13 potions", squire14, 13, tiOrcs01, 2.28, true},
		{"level 14 at the orcs, 3 potions", squire14, 3, tiOrcs01, 2.28, true},
		{"level 14 at the orcs, potions run out", squire14, 0, tiOrcs01, 4.04, false},
		{"level 14 at the werewolves in better armour", with(squire14, func(s *squire) { s.pDef = 120 }), 13, tiWolves, 2.37, true},
		{"level 6 at five toads, no potion", squire6, 0, tiToads27, 2.37, true},
		{"level 6 at four toads, no potion", squire6, 0, tiToads28, 1.90, true},
		{"level 6 at four toads, 12 potions", squire6, 12, tiToads28, 1.19, true},
		{"stats not in yet", with(squire14, func(s *squire) { s.pAtk = 0 }), 0, tiWolves, 0, true},
		{"no aggressive monster", squire14, 0, datapack.Spot{MinLevel: 11, MaxLevel: 13}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, tt.self, tt.potions)
			if got := b.packRisk(&tt.spot); math.Abs(got-tt.risk) > 0.01 {
				t.Errorf("packRisk = %.3f, want %.2f", got, tt.risk)
			}
			if got := b.spotFits(&tt.spot, at(time.Second)); got != tt.fit {
				t.Errorf("spotFits = %v, want %v", got, tt.fit)
			}
		})
	}
	t.Run("PackRisk 0 passes over none", func(t *testing.T) {
		b := newSquireBrain(t, squire14, 0)
		b.persona.Spots.PackRisk = 0
		if !b.spotFits(&tiWolves, at(time.Second)) {
			t.Fatal("the werewolves do not fit at PackRisk 0")
		}
	})
}

// TestPackRiskLeave checks a bot leaves the orc spot it holds for one that
// fits once its potions run out, and stays while they last.
func TestPackRiskLeave(t *testing.T) {
	orcs := tiOrcs01
	orcs.Maker, orcs.Center, orcs.MaximumNPCs = "orcs", datapack.Point{X: 1000}, 11
	tests := []struct {
		name    string
		potions int32
		want    int
	}{
		{"potions left: stays", 3, 0},
		{"potions run out: leaves", 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 14, orcs, testSpot("calm", 0, 3000, 11, 13, 9))
			b.persona.Potions = []int32{lesserHealingPotion}
			setSquire(b, squire14, 13)
			b.decide(at(time.Second))
			if !b.spot.held || b.spot.idx != 0 {
				t.Fatalf("spot %d held %v, want the orcs", b.spot.idx, b.spot.held)
			}
			b.world.Apply(ItemList{Items: []Item{{ObjectID: 0x30000011, TemplateID: lesserHealingPotion, Count: tt.potions}}}, at(2*time.Second))
			b.decide(at(2 * time.Second))
			if !b.spot.held || b.spot.idx != tt.want {
				t.Fatalf("spot %d held %v, want %d", b.spot.idx, b.spot.held, tt.want)
			}
		})
	}
}

// TestIndexPotionHeals checks what a healing potion restores comes off its
// HealOverTime: val a tick, count ticks, time seconds apart.
func TestIndexPotionHeals(t *testing.T) {
	etc := map[int32]datapack.EtcItem{
		lesserHealingPotion: {Kind: datapack.EtcPotion, Skills: []datapack.SkillRef{{ID: 2031, Level: 1}}},
		735:                 {Kind: datapack.EtcPotion, Skills: []datapack.SkillRef{{ID: 2012, Level: 1}}},
	}
	skills := map[datapack.SkillRef]datapack.Skill{
		{ID: 2031, Level: 1}: {Effects: []datapack.Effect{{Name: "HealOverTime", StackType: hpRecover, Time: 2, Count: 7, Value: 16}}},
		{ID: 2012, Level: 1}: {Effects: []datapack.Effect{{Name: "Buff", StackType: "attack_time_down", Time: 1200, Count: 1}}},
	}
	got := indexPotionHeals(etc, skills)
	if len(got) != 1 || got[lesserHealingPotion] != (potionHeal{total: 112, rate: 8}) {
		t.Fatalf("indexPotionHeals = %+v, want only %d at 112 HP, 8 a second", got, lesserHealingPotion)
	}
}

// TestPackRiskShipped checks the fit on the shipped datapack, when
// BOT_DATAPACK_XML names its data/xml directory: a level 14 squire with 13
// Lesser Healing Potions passes over gludio31_1625_10m1, and a level 6 one
// without a potion keeps every toad spot of Talking Island.
func TestPackRiskShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewTravelMap(dp)
	tests := []struct {
		maker   string
		self    squire
		potions int32
		fit     bool
	}{
		{"gludio31_1625_10m1", squire14, 13, false},
		{"gludio32_1725_27m1", squire6, 0, true},
		{"gludio32_1725_28m1", squire6, 0, true},
		{"gludio32_1725_29m1", squire6, 0, true},
		{"gludio32_1725_30m1", squire6, 0, true},
	}
	spots := make(map[string]datapack.Spot)
	for _, s := range dp.Spots() {
		spots[s.Maker] = s
	}
	for _, tt := range tests {
		t.Run(tt.maker, func(t *testing.T) {
			s, ok := spots[tt.maker]
			if !ok {
				t.Fatalf("no spot %s", tt.maker)
			}
			b := newSquireBrain(t, tt.self, tt.potions)
			b.cfg.Travel = m
			if got := b.spotFits(&s, at(time.Second)); got != tt.fit {
				t.Fatalf("spotFits = %v at risk %.2f, want %v", got, b.packRisk(&s), tt.fit)
			}
		})
	}
}
