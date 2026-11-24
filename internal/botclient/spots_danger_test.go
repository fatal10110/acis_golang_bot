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

// squire is a human fighter (class 0, row HF, margin 2) of level in
// Squire's Sword, Shirt and Pants, as its UserInfo carries it, unbuffed:
// squire14 the one TestLivePackSpot logs once strip lifts its death penalty
// (#294), hurt14 the one that died at the werewolves with the penalty on,
// squire6 TestLiveSupplyTrip's, weak6 that one with its death penalty.
type squire struct {
	level, pAtk, pDef, evasion, accuracy, atkSpd, maxHP int32
}

var (
	squire14 = squire{level: 14, pAtk: 7, pDef: 99, evasion: 46, accuracy: 46, atkSpd: 416, maxHP: 520}
	hurt14   = squire{level: 14, pAtk: 7, pDef: 86, evasion: 46, accuracy: 46, atkSpd: 416, maxHP: 385}
	squire6  = squire{level: 6, pAtk: 6, pDef: 79, evasion: 38, accuracy: 38, atkSpd: 416, maxHP: 221}
	weak6    = squire{level: 6, pAtk: 2, pDef: 42, evasion: -1, accuracy: -1, atkSpd: 170, maxHP: 221}
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
	self.Level, self.PAtk, self.PDef, self.Evasion, self.Accuracy, self.PAtkSpd = s.level, s.pAtk, s.pDef, s.evasion, s.accuracy, s.atkSpd
	self.HP, self.MaxHP = s.maxHP, s.maxHP
	b.cfg.Travel = &TravelMap{heals: map[int32]potionHeal{lesserHealingPotion: {total: 112, rate: 8}}}
	b.world.Apply(ItemList{Items: []Item{{ObjectID: 0x30000011, TemplateID: lesserHealingPotion, Count: potions}}}, t0)
}

// TestPackFit checks the model bound of a human fighter (margin 2) on the
// packs of #294: the werewolves pass over a level 14 squire with its death
// penalty before any death there, and the one without it once its potions
// spread over the pack do not carry it; the orcs fit while its potions
// last; a level 6 one keeps the toad spots without a potion, and its death
// penalty loses them. kill and die are packFight's seconds: a Werewolf
// Chieftain (level 12: 314 HP, P.Atk 30, P.Def 60) takes the level 14
// squire's 7 P.Atk 37.6 hits 1.2 s apart, and lands 86 % of its swings for
// 23.7 on P.Def 99 every 1.84 s; 13 potions over the 4.9 werewolves of the
// likely pack restore 296 HP each fight.
func TestPackFit(t *testing.T) {
	with := func(s squire, f func(*squire)) squire { f(&s); return s }
	tests := []struct {
		name      string
		self      squire
		potions   int32
		spot      datapack.Spot
		kill, die float64
		fit       bool
	}{
		{"level 14 at the werewolves, 13 potions", squire14, 13, tiWolves, 38.1, 83.9, true},
		{"level 14 at the werewolves, 3 potions", squire14, 3, tiWolves, 38.1, 60.4, false},
		{"level 14 at the werewolves, no potion", squire14, 0, tiWolves, 38.1, 53.5, false},
		{"level 14 with its death penalty at the werewolves, 13 potions", hurt14, 13, tiWolves, 38.1, 60.8, false},
		{"level 14 at the werewolves in better armour", with(squire14, func(s *squire) { s.pDef = 160 }), 0, tiWolves, 38.1, 86.4, true},
		{"level 14 with its death penalty at the orcs, 13 potions", hurt14, 13, tiOrcs01, 36.5, 82.9, true},
		{"level 14 with its death penalty at the orcs, potions run out", hurt14, 0, tiOrcs01, 36.5, 45.1, false},
		{"level 14 at the orcs, 3 potions", squire14, 3, tiOrcs01, 36.5, 80.2, true},
		{"level 6 at five toads, no potion", squire6, 0, tiToads27, 16.0, 36.3, true},
		{"level 6 at four toads, no potion", squire6, 0, tiToads28, 16.0, 36.3, true},
		{"level 6 with its death penalty at five toads, 12 potions", weak6, 12, tiToads27, 360.9, 38.4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, tt.self, tt.potions)
			bd := b.build()
			kill, die, ok := b.packFight(&tt.spot, &bd, bd.Kit.SpotMargin)
			if !ok || math.Abs(kill-tt.kill) > 0.1 || math.Abs(die-tt.die) > 0.1 {
				t.Errorf("packFight = %.2f s, %.2f s, %v; want %.1f, %.1f", kill, die, ok, tt.kill, tt.die)
			}
			if got := b.spotFits(&tt.spot, at(time.Second)); got != tt.fit {
				t.Errorf("spotFits = %v, want %v", got, tt.fit)
			}
		})
	}
	for _, tt := range []struct {
		name string
		self squire
		spot datapack.Spot
		set  func(b *Brain)
		fit  bool
	}{
		{"stats not in yet", with(squire14, func(s *squire) { s.pAtk = 0 }), tiWolves, nil, true},
		{"no monster with stats", squire14, datapack.Spot{MinLevel: 11, MaxLevel: 13}, nil, true},
		{"a mystic line has no margin until S7", squire14, tiWolves, func(b *Brain) { b.world.Self.ClassID = 10 }, true},
		{"SpotPolicy.Margin below 0 turns it off", squire14, tiWolves, func(b *Brain) { b.persona.Spots.Margin = -1 }, true},
		{"SpotPolicy.Margin replaces the row's", squire14, tiWolves, func(b *Brain) { b.persona.Spots.Margin = 1.25 }, true},
		{"SpotPolicy.Margin gives a mystic one", squire14, tiWolves, func(b *Brain) { b.world.Self.ClassID, b.persona.Spots.Margin = 10, 2 }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, tt.self, 0)
			if tt.set != nil {
				tt.set(b)
			}
			if got := b.spotFits(&tt.spot, at(time.Second)); got != tt.fit {
				t.Fatalf("spotFits = %v, want %v", got, tt.fit)
			}
		})
	}
}

// TestPackFitPick checks which spot a level 14 squire claims, the
// werewolves the nearest: the orcs while three potions last, none once
// they run out, and with the model bound off the nearest.
func TestPackFitPick(t *testing.T) {
	place := func(s datapack.Spot, name string, x, y int32) datapack.Spot {
		s.Maker, s.Center, s.MaximumNPCs = name, datapack.Point{X: x, Y: y}, 11
		return s
	}
	wolves, orcs := place(tiWolves, "wolves", 1000, 0), place(tiOrcs01, "orcs", 0, 3000)
	tests := []struct {
		name    string
		margin  float64
		potions int32
		// want is the spot held, -1 for none.
		want int
	}{
		{"3 potions: the orcs", 0, 3, 1},
		{"no potion: none", 0, 0, -1},
		{"bound off: the nearest", -1, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 14, wolves, orcs)
			b.persona.Potions = []int32{lesserHealingPotion}
			b.persona.Spots.Margin = tt.margin
			setSquire(b, squire14, tt.potions)
			b.decide(at(time.Second))
			got := -1
			if b.spot.held {
				got = b.spot.idx
			}
			if got != tt.want {
				t.Fatalf("spot %d, want %d", got, tt.want)
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

// TestPackFitShipped checks the fit on the shipped datapack, when
// BOT_DATAPACK_XML names its data/xml directory: a level 14 squire passes
// over gludio31_1625_10m1 with its death penalty and 13 Lesser Healing
// Potions, or without it and 3 potions, and a level 6 one without a potion
// keeps every toad spot of Talking Island.
func TestPackFitShipped(t *testing.T) {
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
		{"gludio31_1625_10m1", hurt14, 13, false},
		{"gludio31_1625_10m1", squire14, 3, false},
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
				bd := b.build()
				kill, die, _ := b.packFight(&s, &bd, bd.Kit.SpotMargin)
				t.Fatalf("spotFits = %v at kill %.1f s, die %.1f s, want %v", got, kill, die, tt.fit)
			}
		})
	}
}

// TestPackFitRepick checks a bot that took the werewolves before its
// stats came in moves to a calm spot once they do, and one holding the
// orcs keeps them when its potions run out with no spot that fits.
func TestPackFitRepick(t *testing.T) {
	wolves, orcs := tiWolves, tiOrcs01
	wolves.Maker, wolves.Center, wolves.MaximumNPCs = "wolves", datapack.Point{X: 1000}, 10
	orcs.Maker, orcs.Center, orcs.MaximumNPCs = "orcs", datapack.Point{X: 1000}, 11
	type step struct {
		name    string
		self    squire
		potions int32
		want    int
	}
	tests := []struct {
		name  string
		spots []datapack.Spot
		steps []step
	}{
		{
			name:  "stats come in",
			spots: []datapack.Spot{wolves, testSpot("calm", 0, 3000, 11, 13, 9)},
			steps: []step{
				{name: "no stats yet: the nearest", want: 0},
				{name: "stats in: the calm spot", self: squire14, potions: 3, want: 1},
				{name: "stays", self: squire14, potions: 3, want: 1},
			},
		},
		{
			name:  "potions run out",
			spots: []datapack.Spot{orcs},
			steps: []step{
				{name: "the orcs fit", self: squire14, potions: 3, want: 0},
				{name: "none left: kept", self: squire14, want: 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 14, tt.spots...)
			b.persona.Potions = []int32{lesserHealingPotion}
			for i, s := range tt.steps {
				if s.self.level > 0 {
					setSquire(b, s.self, s.potions)
				}
				b.decide(at(time.Duration(i+1) * time.Second))
				if !b.spot.held || b.spot.idx != s.want {
					t.Fatalf("%s: spot %d held %v, want %d", s.name, b.spot.idx, b.spot.held, s.want)
				}
			}
		})
	}
}
