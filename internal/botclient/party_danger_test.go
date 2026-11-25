package botclient

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The class templates and skills of a leader's bot members (classes/*.xml,
// skills/1000-1099.xml, 1200-1299.xml): a human mystic (10) learns 1011
// Heal level 4 at 14 (power 83, 4 + 13 MP, hit time 5 s, reuse 10 s), a human
// fighter (0) learns nothing, an Orc Shaman (50) 1229 Chant of Life level
// 1 at 20 (15 ticks of 12 HP, 12 + 47 MP, hit time 7 s, reuse 20 s).
var (
	humanMysticTmpl = datapack.Class{
		ID: 10, Parent: -1, STR: 22, CON: 27, DEX: 21, INT: 41, WIT: 20, MEN: 39, PAtk: 3, PDef: 54, MAtk: 6, MDef: 41, RunSpeed: 120,
		Skills: []datapack.SkillGrant{{ID: skillHeal, Level: 3, MinLevel: 7}, {ID: skillHeal, Level: 4, MinLevel: 14}},
	}
	humanFighterTmpl = datapack.Class{ID: 0, Parent: -1, STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25, PAtk: 4, PDef: 72, MAtk: 2, MDef: 47, RunSpeed: 115}
	orcShamanTmpl    = datapack.Class{
		ID: 50, Parent: 49, Race: 3, STR: 27, CON: 31, DEX: 24, INT: 31, WIT: 15, MEN: 42, PAtk: 4, PDef: 54, MAtk: 6, MDef: 41, RunSpeed: 121,
		Skills: []datapack.SkillGrant{{ID: skillChantLife, Level: 1, MinLevel: 20}},
	}
	healLevel4 = datapack.Skill{Type: "HEAL", Power: 83, MPInitialConsume: 4, MPConsume: 13, HitTime: 5 * time.Second, ReuseDelay: 10 * time.Second}
	chantLife1 = datapack.Skill{
		Type: "HOT", MPInitialConsume: 12, MPConsume: 47, HitTime: 7 * time.Second, ReuseDelay: 20 * time.Second,
		Effects: []datapack.Effect{{Name: "HealOverTime", Count: 15, Value: 12}},
	}
)

// TestPartyFitHealer checks a leader's spot fit with its bot members
// (docs/parties/model.md): the level 14 squire with three potions that
// fails the werewolves alone fits them with a level 14 human mystic healer
// at full MP beside it; a healer with no MP left, dead, away beyond
// FarAway, not reported yet, or a player adds nothing; a Damage member
// kills faster, and an Orc Shaman's Chant of Life heals by its ticks.
// kill and die are packFight's seconds, the margin the squire's 2: the
// healer's 17 casts of 17 MP (4 up front, 13 on landing) over the 4.9
// fights of the likely pack, 3.5 a fight of 85.6 HP (83 + sqrt(7 M. Atk)),
// add 298 HP each fight; the Orc Shaman's 6 Chants of Life of 59 MP, 1.2
// a fight of 180 HP, add 221; the naked
// fighter's fists cut a kill from 38.1 s to 25.9 s, so the squire need
// last only 51.7 s.
func TestPartyFitHealer(t *testing.T) {
	healer := func(id, mp int32) PartyMember {
		m := mate(id, 14)
		m.ClassID, m.MP, m.MaxMP = 10, mp, 300
		return m
	}
	fighter := mate(bethID, 14)
	shaman := mate(abelID, 20)
	shaman.ClassID, shaman.MP, shaman.MaxMP = 50, 400, 400
	near := func(id int32) func(b *Brain) {
		return func(b *Brain) { b.world.Party.Members[id].Pos, b.world.Party.Members[id].PosAt = Point{X: 500}, t0 }
	}
	tests := []struct {
		name      string
		ms        []PartyMember
		set       func(b *Brain)
		kill, die float64
		fit       bool
	}{
		{name: "alone", kill: 38.1, die: 60.4},
		{name: "a healer at full MP", ms: []PartyMember{healer(abelID, 300)}, set: near(abelID), kill: 38.1, die: 91.0, fit: true},
		{name: "a healer with no MP left", ms: []PartyMember{healer(abelID, 0)}, set: near(abelID), kill: 38.1, die: 60.4},
		{name: "a dead healer", ms: []PartyMember{func() PartyMember { m := healer(abelID, 300); m.HP = 0; return m }()}, set: near(abelID), kill: 38.1, die: 60.4},
		{name: "a healer away on a trip", ms: []PartyMember{healer(abelID, 300)}, set: func(b *Brain) {
			b.world.Party.Members[abelID].Pos, b.world.Party.Members[abelID].PosAt = Point{X: 20000}, t0
		}, kill: 38.1, die: 60.4},
		{name: "a healer not reported yet", ms: []PartyMember{healer(abelID, 300)}, kill: 38.1, die: 60.4},
		{name: "a healer in sight", ms: []PartyMember{healer(abelID, 300)}, set: func(b *Brain) {
			b.world.Objects[abelID] = &Object{}
		}, kill: 38.1, die: 91.0, fit: true},
		{name: "a dead healer in sight", ms: []PartyMember{healer(abelID, 300)}, set: func(b *Brain) {
			b.world.Objects[abelID] = &Object{Dead: true}
		}, kill: 38.1, die: 60.4},
		{name: "a player healer", ms: []PartyMember{healer(talbotID, 300)}, set: near(talbotID), kill: 38.1, die: 60.4},
		{name: "a fighter", ms: []PartyMember{fighter}, set: near(bethID), kill: 25.9, die: 60.5, fit: true},
		{name: "an Orc Shaman's Chant of Life", ms: []PartyMember{shaman}, set: near(abelID), kill: 38.1, die: 83.1, fit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, squire14, 3)
			b.cfg.Travel.classes = map[int32]datapack.Class{0: humanFighterTmpl, 10: humanMysticTmpl, 50: orcShamanTmpl}
			b.cfg.Travel.skills = map[datapack.SkillRef]datapack.Skill{{ID: skillHeal, Level: 4}: healLevel4, {ID: skillChantLife, Level: 1}: chantLife1}
			b.cfg.link = newSlotLink(standing{})
			b.setPartySlot(lead("Abel Beth Cole"))
			if len(tt.ms) > 0 {
				feed(t, b, 0, leadWindow(tt.ms...))
			}
			if tt.set != nil {
				tt.set(b)
			}
			now := at(time.Second)
			bd, c := b.build(), b.partyCrew(now)
			kill, die, ok := b.packFight(&tiWolves, &bd, bd.Kit.SpotMargin, &c)
			if !ok || math.Abs(kill-tt.kill) > 0.1 || math.Abs(die-tt.die) > 0.1 {
				t.Errorf("packFight = %.2f s, %.2f s, %v; want %.1f, %.1f", kill, die, ok, tt.kill, tt.die)
			}
			if got := b.spotFits(&tiWolves, now); got != tt.fit {
				t.Errorf("spotFits = %v, want %v", got, tt.fit)
			}
		})
	}
}

// TestPartyFitHealerShipped checks TestPartyFitHealer against the shipped
// datapack when BOT_DATAPACK_XML names its data/xml directory: the level
// 14 squire with three potions that passes over gludio31_1625_10m1 alone
// fits it with a level 14 human mystic healer at full MP, its class tree
// and 1011 Heal as the datapack ships them.
func TestPartyFitHealerShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewTravelMap(dp, DefaultSupplyConfig())
	var spot datapack.Spot
	for _, s := range dp.Spots() {
		if s.Maker == "gludio31_1625_10m1" {
			spot = s
		}
	}
	for _, tt := range []struct {
		name string
		mp   int32
		fit  bool
	}{{"healer at full MP", 300, true}, {"healer with no MP left", 0, false}} {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, squire14, 3)
			b.cfg.Travel = m
			b.cfg.link = newSlotLink(standing{})
			b.setPartySlot(lead("Abel Beth Cole"))
			h := mate(abelID, 14)
			h.ClassID, h.MP, h.MaxMP = 10, tt.mp, 300
			feed(t, b, 0, leadWindow(h))
			b.world.Party.Members[abelID].Pos, b.world.Party.Members[abelID].PosAt = Point{X: 500}, t0
			now := at(time.Second)
			if got := b.spotFits(&spot, now); got != tt.fit {
				bd, c := b.build(), b.partyCrew(now)
				kill, die, _ := b.packFight(&spot, &bd, bd.Kit.SpotMargin, &c)
				t.Fatalf("spotFits = %v at kill %.1f s, die %.1f s, want %v", got, kill, die, tt.fit)
			}
		})
	}
}
